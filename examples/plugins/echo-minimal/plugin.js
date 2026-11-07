// Example: the smallest loadable task driver (apiVersion 1, fetchMode "per_task").
//
// The manifest declares no `kind`, so the host loads it as kind "task": the
// documented default for every apiVersion 1 manifest. "per_task" polling means
// the host polls one task at a time through buildQueryRequest.
//
// What the host requires from a per_task driver:
//   buildSubmitRequest, parseSubmitResponse, buildQueryRequest, parseTaskResult
//
// The vendor ("Echo") is fictional so the example stays self-contained; it
// answers a submitted job with {"id", "status"} and a polled job with
// {"id", "state", "result_text", "word_count"}.
export const meta = {
  apiVersion: 1,
  key: "example-echo",
  name: "Example Echo",
  // "text" asks for a generated text avatar; a real vendor uses its LobeHub
  // icon name, or ships icon.svg next to this file.
  icon: "text",
  description: {
    en: "Minimal per-task polling driver example",
    zh: "最小化的单任务轮询驱动示例",
  },
  version: "1.0.0",
  author: { name: "Example Author" },
  // Default Base URL for a type-61 "Task Plugin" channel bound to this plugin
  // when the administrator leaves the channel Base URL empty. It must be an
  // absolute http(s) URL without credentials, query, or fragment; the host
  // normalizes it and persists it onto the channel at bind time.
  baseUrl: "https://echo.example.com",
  models: ["echo-1", "echo-1-mini"],
  fetchMode: "per_task",
  // Numeric fields are usage quantities; their descriptions label the price
  // input in the UI, so they name the billing subject and unit price, and the
  // unit itself lives in `unit`.
  usageSchema: {
    // Billed per thousand words. The count unit is capped at 128 facts by the
    // host, so the driver reports whole thousands here and lets unitLabel name
    // the display unit; a real plugin declares the unit its price is quoted in.
    word_count: {
      type: "number",
      unit: "count",
      unitLabel: { en: "thousand words", zh: "千字" },
      description: { en: "Text generation unit price", zh: "文本生成单价" },
    },
    // Every plugin that has prompt tokens to report declares them as tokens.
    prompt_tokens: {
      type: "number",
      unit: "token",
      description: { en: "Input token unit price", zh: "输入词元单价" },
    },
    // Boolean facts name the affirmative state that `true` represents.
    fast: {
      type: "boolean",
      description: { en: "Whether fast mode is requested", zh: "是否请求快速模式" },
    },
  },
  // Required by the host as soon as usageSchema declares a token unit: each
  // label is a spec name, never a price, and each facts object is a complete
  // vector over usageSchema.
  usageExamples: [{ label: "echo-1 · 1k words", facts: { word_count: 1, prompt_tokens: 120, fast: false } }],
  // Native routes are optional for a host-protocol-only plugin but are the
  // vendor's own paths here. Both routes are also served under /example-echo.
  routes: [
    { method: "POST", path: "/v1/echo/jobs", type: "submit", decode: "decodeSubmit", render: "renderSubmit" },
    { method: "GET", path: "/v1/echo/jobs/:task_id", type: "query", render: "renderTask" },
  ],
};

// Decoders reject the body kinds the vendor format does not use and return a
// canonical submit intent. The host never parses vendor request bodies.
function decodeSubmit(ctx) {
  if (!ctx.body || ctx.body.kind !== "json") throw new Error("JSON body required");
  const body = ctx.body.value || {};
  if (typeof body.model !== "string" || body.model === "") throw new Error("model is required");
  if (typeof body.text !== "string" || body.text.trim() === "") throw new Error("text is required");
  return { kind: "submit", model: body.model, requestBody: body };
}

// buildSubmitRequest must not branch on the client path or protocol: it reads
// only the normalized ctx.requestBody that the decoder produced.
export function buildSubmitRequest(ctx) {
  const body = ctx.requestBody || {};
  return {
    url: ctx.baseUrl + "/v1/echo/jobs",
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      // authHeader already carries the channel credential in the vendor's
      // scheme ("Bearer <token>" for api_key auth).
      Authorization: ctx.authHeader,
    },
    body: { input: body.text, model: ctx.upstreamModel || ctx.model, fast: body.fast === true },
    // A native query route answer carries this action; it is persisted with the
    // task and handed back to every query hook.
    action: "text",
  };
}

// Any 2xx reply reaches this hook. Return the upstream task id and keep only
// the snapshot a presenter or artifact hook needs in taskData.
export function parseSubmitResponse(ctx, response) {
  const body = response.body || {};
  if (typeof body.id !== "string" || body.id === "") throw new Error("upstream returned no job id");
  return { taskId: body.id, taskData: { id: body.id, status: String(body.status || "") } };
}

// Reports quantities for billing before submission. Return only facts declared
// in usageSchema; never return a price or a quota.
export function extractUsage(ctx) {
  const body = ctx.requestBody || {};
  // billing_ratios is a legacy per-call hook; this plugin has none.
  if (ctx.usagePurpose === "billing_ratios") return null;
  return { fast: body.fast === true };
}

// TaskQueryContext carries no client HTTP identity: taskId is the upstream id,
// publicTaskId the gateway id, and state the plugin's own persisted state.
export function buildQueryRequest(ctx) {
  return {
    url: ctx.baseUrl + "/v1/echo/jobs/" + encodeURIComponent(ctx.taskId),
    method: "GET",
    headers: { Authorization: ctx.authHeader },
  };
}

// Map the vendor status onto the host's fixed vocabulary. Never fall back to a
// guessed status: "UNKNOWN" is what the host needs to see for a response the
// plugin does not recognize, and enough of them fail the task.
export function parseTaskResult(ctx, body, response) {
  const data = body || {};
  const state = String(data.state || "").toLowerCase();
  switch (state) {
    case "queued":
      return { status: "QUEUED", data: data };
    case "running":
      return { status: "IN_PROGRESS", progress: "50%", data: data };
    case "done":
      return { status: "SUCCESS", data: data, remoteUrl: "" };
    case "error":
      return { status: "FAILURE", reason: String(data.error || "job failed"), data: data };
    default:
      return { status: "UNKNOWN", data: data };
  }
}

// Settlement facts read from the persisted data of a finished task.
export function extractUsageOnComplete(ctx, result, data) {
  const words = Number((data || {}).word_count);
  const wordsPerUnit = 1000;
  return {
    word_count: Number.isFinite(words) ? Math.ceil(words / wordsPerUnit) : 0,
    prompt_tokens: Number((data || {}).prompt_tokens) || 0,
  };
}

// Native presenters own the vendor envelope and run after persistence.
export const native = {
  decodeSubmit: decodeSubmit,
  renderSubmit: function (ctx, task) {
    const data = task.data || {};
    return { id: task.task_id, status: String(data.status || "queued") };
  },
  renderTask: function (ctx, task) {
    return {
      id: task.task_id,
      state: String(task.status || ""),
      reason: String(task.fail_reason || ""),
      data: task.data === undefined ? null : task.data,
    };
  },
  error: function (ctx, error) {
    return { error: { message: error.message } };
  },
};
