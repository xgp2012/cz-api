// Example: fetchMode "batch" — one upstream request polls every running task.
//
// With fetchMode "batch" the host requires buildBatchQueryRequest and
// parseBatchResult in place of buildQueryRequest. It still requires
// buildSubmitRequest, parseSubmitResponse and parseTaskResult, so the driver
// keeps a per-task hook for paths that poll a single task (an openai_image
// request that polls inside the client request, for example) and reuses the
// batch parser for it.
//
// The fictional vendor ("Fanout") accepts one job per request here; a real batch
// driver sends every task id in one body, which is what makes batch polling
// cheaper than per_task polling.
export const meta = {
  apiVersion: 1,
  key: "example-batch",
  name: "Example Batch",
  icon: "text",
  description: {
    en: "Batch polling driver example",
    zh: "批量轮询驱动示例",
  },
  version: "1.0.0",
  author: { name: "Example Author" },
  models: ["fanout-image"],
  fetchMode: "batch",
  usageSchema: {
    // The vendor bills each delivered picture.
    image_count: {
      type: "number",
      unit: "count",
      unitLabel: { en: "image", zh: "张" },
      description: { en: "Image generation unit price", zh: "图片生成单价" },
    },
    // The sample size belongs to the vendor's request format, not to billing;
    // an enum fact is a price selector, so it needs no unit price wording.
    size: {
      enum: ["512", "1024"],
      enumLabels: {
        512: { en: "512 pixels", zh: "512 像素" },
        1024: { en: "1024 pixels", zh: "1024 像素" },
      },
      description: { en: "Output image size", zh: "输出图片尺寸" },
    },
  },
  routes: [{ method: "POST", path: "/v1/fanout/images", type: "submit", decode: "decodeSubmit", render: "renderSubmit" }],
};

function decodeSubmit(ctx) {
  if (!ctx.body || ctx.body.kind !== "json") throw new Error("JSON body required");
  const body = ctx.body.value || {};
  if (typeof body.prompt !== "string" || body.prompt.trim() === "") throw new Error("prompt is required");
  const size = body.size === undefined ? "1024" : String(body.size);
  if (size !== "512" && size !== "1024") throw new Error("size must be 512 or 1024");
  return { kind: "submit", model: ctx.body.value.model || "fanout-image", requestBody: { prompt: body.prompt, size: size } };
}

export function buildSubmitRequest(ctx) {
  const body = ctx.requestBody || {};
  return {
    url: ctx.baseUrl + "/v1/fanout/images",
    method: "POST",
    headers: { "Content-Type": "application/json", Authorization: ctx.authHeader },
    body: { prompt: body.prompt, size: body.size },
    action: "image",
  };
}

// The vendor answers synchronously with the finished pictures, so the task
// completes immediately: immediate SUCCESS skips polling entirely and the host
// never persists task.data unless a route keeps retainResult (the default).
export function parseSubmitResponse(ctx, response) {
  const body = response.body || {};
  const images = Array.isArray(body.images) ? body.images : [];
  if (images.length === 0) throw new Error("upstream returned no image");
  const urls = [];
  for (const image of images) {
    const url = String((image && image.url) || "").trim();
    if (!url) throw new Error("upstream returned an image without a url");
    urls.push(url);
  }
  return {
    taskId: String(body.request_id || ""),
    taskData: { images: urls },
    immediate: { status: "SUCCESS", data: { images: urls } },
  };
}

// Facts sent with the submission; the terminal facts below replace them.
export function extractUsage(ctx) {
  const body = ctx.requestBody || {};
  if (ctx.usagePurpose === "billing_ratios") return null;
  return { image_count: 1, size: body.size === "512" ? "512" : "1024" };
}

// batch: one descriptor for every task the host is currently polling. tasks is
// a TaskQueryContext[], the same shape buildQueryRequest receives.
export function buildBatchQueryRequest(ctx, tasks) {
  const ids = (tasks || []).map(function (task) {
    return task.taskId;
  });
  return {
    url: ctx.baseUrl + "/v1/fanout/jobs:get",
    method: "POST",
    headers: { "Content-Type": "application/json", Authorization: ctx.authHeader },
    body: { ids: ids },
  };
}

// The reply is one entry per requested id. Each entry keeps the per-task
// `data` and `state` that the host persists, and `taskId` matches the id the
// host asked about.
export function parseBatchResult(ctx, body, response) {
  const jobs = (body || {}).jobs;
  if (!Array.isArray(jobs)) throw new Error("upstream batch reply has no jobs array");
  const results = [];
  for (const job of jobs) {
    if (!job || typeof job.id !== "string") throw new Error("upstream batch entry has no id");
    results.push({
      taskId: job.id,
      status: job.state === "finished" ? "SUCCESS" : job.state === "failed" ? "FAILURE" : "IN_PROGRESS",
      reason: String(job.error || ""),
      data: job,
    });
  }
  return results;
}

// Batch plugins still export parseTaskResult: a host path that polls one task
// (the openai_image in-request poller) calls it. Reusing the batch parser keeps
// both paths agreeing on status mapping.
export function parseTaskResult(ctx, body, response) {
  const result = parseBatchResult(ctx, { jobs: [body || {}] }, response)[0];
  if (!result) return { status: "UNKNOWN" };
  return { status: result.status, reason: result.reason, data: result.data };
}

export function extractUsageOnComplete(ctx, result, data) {
  const images = (data || {}).images;
  return { image_count: Array.isArray(images) ? images.length : 0, size: "1024" };
}

export const native = {
  decodeSubmit: decodeSubmit,
  renderSubmit: function (ctx, task) {
    const data = task.data || {};
    return { request_id: task.task_id, images: Array.isArray(data.images) ? data.images : [] };
  },
  error: function (ctx, error) {
    return { error: { message: error.message } };
  },
};
