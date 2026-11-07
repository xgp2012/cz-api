// Example: server-side polling that hides a two-step download.
//
// The vendor hands out a preview after the job finishes and only a signed,
// short-lived download link from a second call. Declaring artifacts lets the
// gateway serve the deliverable itself at a stable, credential-free address:
//
//   listArtifacts        — which deliverables a finished task has
//   buildContentRequest  — how to fetch one of them
//
// The host requires the two hooks together: exporting one without the other
// fails to load. The artifact address is what a client receives from
// GET /v1/videos/:task_id/content (or an openai_responses renderer's
// ctx.artifacts[key].url), so a link that expires upstream never reaches the
// client.
export const meta = {
  apiVersion: 1,
  key: "example-artifacts",
  name: "Example Artifacts",
  icon: "text",
  description: {
    en: "Artifact download driver example",
    zh: "产物下载驱动示例",
  },
  version: "1.0.0",
  author: { name: "Example Author" },
  // The signed download link lives on a CDN host, not on the API host, so the
  // credential boundary for redirects and utils.fetch has to list it.
  baseUrl: "https://api.example-video.com",
  allowedHosts: ["cdn.example-video.com"],
  models: ["clip-video"],
  fetchMode: "per_task",
  usageSchema: {
    // The vendor bills per second of delivered video.
    seconds: {
      type: "number",
      unit: "second",
      description: { en: "Video generation unit price", zh: "视频生成单价" },
    },
    with_audio: {
      type: "boolean",
      description: { en: "Whether audio is generated", zh: "是否生成音频" },
    },
  },
  routes: [{ method: "POST", path: "/v1/clips", type: "submit", decode: "decodeSubmit", render: "renderSubmit" }],
  protocols: ["openai_video"],
};

function decodeSubmit(ctx) {
  if (!ctx.body || ctx.body.kind !== "json") throw new Error("JSON body required");
  const body = ctx.body.value || {};
  if (typeof body.prompt !== "string" || body.prompt.trim() === "") throw new Error("prompt is required");
  const seconds = Number(body.seconds === undefined ? 5 : body.seconds);
  if (!Number.isFinite(seconds) || seconds < 1 || seconds > 60) throw new Error("seconds must be between 1 and 60");
  return { kind: "submit", model: ctx.model, requestBody: { prompt: body.prompt, seconds: seconds, with_audio: body.with_audio === true } };
}

function decodeVideoCreate(ctx) {
  const body = ctx.body || {};
  if (body.kind !== "json" && body.kind !== "multipart") throw new Error("JSON or multipart body required");
  const fields = body.kind === "json" ? body.value || {} : body.fields || {};
  const prompt = Array.isArray(fields.prompt) ? fields.prompt[0] : fields.prompt;
  if (typeof prompt !== "string" || prompt.trim() === "") throw new Error("prompt is required");
  const seconds = Number(Array.isArray(fields.seconds) ? fields.seconds[0] : fields.seconds || 5);
  return {
    kind: "submit",
    model: ctx.model,
    requestBody: { prompt: prompt, seconds: Number.isFinite(seconds) ? seconds : 5, with_audio: false },
  };
}

export function buildSubmitRequest(ctx) {
  const body = ctx.requestBody || {};
  return {
    url: ctx.baseUrl + "/v1/clips",
    method: "POST",
    headers: { "Content-Type": "application/json", Authorization: ctx.authHeader },
    body: body,
    action: "video",
  };
}

export function parseSubmitResponse(ctx, response) {
  const body = response.body || {};
  if (typeof body.job_id !== "string" || body.job_id === "") throw new Error("upstream returned no job id");
  return { taskId: body.job_id, taskData: { job_id: body.job_id, state: String(body.state || "queued") } };
}

export function extractUsage(ctx) {
  const body = ctx.requestBody || {};
  if (ctx.usagePurpose === "billing_ratios") return null;
  return { seconds: Number(body.seconds) || 0, with_audio: body.with_audio === true };
}

export function buildQueryRequest(ctx) {
  return { url: ctx.baseUrl + "/v1/clips/" + encodeURIComponent(ctx.taskId), method: "GET", headers: { Authorization: ctx.authHeader } };
}

export function parseTaskResult(ctx, body) {
  const data = body || {};
  const state = String(data.state || "").toLowerCase();
  if (state === "done") return { status: "SUCCESS", data: data };
  if (state === "error") return { status: "FAILURE", reason: String(data.error || "clip failed"), data: data };
  if (state === "running") return { status: "IN_PROGRESS", data: data };
  if (state === "queued") return { status: "QUEUED", data: data };
  return { status: "UNKNOWN", data: data };
}

export function extractUsageOnComplete(ctx, result, data) {
  const duration = Number((data || {}).duration_seconds);
  return { seconds: Number.isFinite(duration) ? duration : 0, with_audio: (data || {}).audio === true };
}

// Artifact keys must be stable for one deliverable: the key a listing produces
// is the key buildContentRequest later receives as ctx.artifactKey. It must not
// depend on the signed link, which changes on every poll.
export function listArtifacts(task) {
  if (task.status !== "SUCCESS") return [];
  const data = task.data || {};
  const artifacts = [];
  if (String(data.download_path || "").trim()) artifacts.push({ key: "clip", type: "video", mimeType: "video/mp4" });
  if (String(data.cover_path || "").trim()) artifacts.push({ key: "cover", type: "image", mimeType: "image/jpeg" });
  return artifacts;
}

// The signed link is fetched only now, from a hook that runs after channel
// selection. utils.fetch is available in this hook (and in the other post-
// selection driver hooks) and is limited to the channel host and allowedHosts.
export function buildContentRequest(ctx) {
  const data = ctx.data || {};
  const path = ctx.artifactKey === "clip" ? data.download_path : ctx.artifactKey === "cover" ? data.cover_path : "";
  if (!String(path || "").trim()) throw new Error("artifact_not_found");
  const signed = utils.fetch({
    url: ctx.baseUrl + String(path),
    method: "GET",
  });
  if (signed.status !== 200) throw new Error("artifact_not_found");
  const body = signed.body || {};
  const url = String(body.url || "").trim();
  if (!url) throw new Error("artifact_not_found");
  // credentialless: the CDN does not take the channel API key. The host keeps
  // the URL host unchanged and forwards no plugin credentials.
  return { url: url, method: ctx.clientRequest.method, credentialless: true };
}

// openai_video is claimed in bare-string form because it defines no modes; it
// requires decodeRequest and render.
export const protocols = {
  openai_video: {
    decodeRequest: decodeVideoCreate,
    render: function (ctx, task) {
      const data = task.data || {};
      const artifacts = ctx.artifacts || {};
      const video = artifacts.clip;
      return {
        status: String(task.status || ""),
        progress: task.status === "SUCCESS" ? 100 : 0,
        seconds: String(data.duration_seconds || ""),
        // The signed gateway address, not the vendor's expiring link.
        url: video ? video.url : "",
        fail_reason: task.fail_reason || "",
      };
    },
  },
};

export const native = {
  decodeSubmit: decodeSubmit,
  renderSubmit: function (ctx, task) {
    return { job_id: task.task_id, state: String((task.data || {}).state || "queued") };
  },
  error: function (ctx, error) {
    return { error: { message: error.message } };
  },
};
