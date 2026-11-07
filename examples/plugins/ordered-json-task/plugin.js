// Example: apiVersion 2 with kind "task" spelled out.
//
// apiVersion 2 with kind "task" means exactly what apiVersion 1 does: the plugin
// is an asynchronous task driver and needs the same hooks. Declaring the kind
// makes that explicit, and apiVersion 2 is what a manifest needs before it can
// declare a non-task kind later.
//
// This example also shows the two extensions that change what a hook receives:
//   requiredCapabilities: ["json-order@1"] — the hook receives request bodies
//   with the members in the order the client sent them, which matters for a
//   model that reads a JSON object as text. Omit it and the host sorts members.
//
// It also returns `state` from parseSubmitResponse and parseTaskResult: state is
// the only value that survives between poll rounds unchanged by the upstream
// snapshot, so a driver keeps its own bookkeeping there.
export const meta = {
  apiVersion: 2,
  kind: "task",
  key: "example-ordered",
  name: "Example Ordered",
  icon: "text",
  description: {
    en: "apiVersion 2 task driver with ordered JSON",
    zh: "使用有序 JSON 的 apiVersion 2 任务驱动",
  },
  version: "1.0.0",
  author: { name: "Example Author" },
  requiredCapabilities: ["json-order@1"],
  models: ["ordered-json", "ordered-json-large"],
  fetchMode: "per_task",
  usageSchema: {
    // Priced per call for the small model and per produced item for the large
    // one; usageProfiles below swaps the schema per model.
    request_count: {
      type: "number",
      unit: "count",
      description: { en: "Request unit price", zh: "请求单价" },
    },
    item_count: {
      type: "number",
      unit: "count",
      unitLabel: { en: "item", zh: "项" },
      description: { en: "Item processing unit price", zh: "项目处理单价" },
    },
    prompt_tokens: {
      type: "number",
      unit: "token",
      description: { en: "Input token unit price", zh: "输入词元单价" },
    },
  },
  // Required once usageSchema declares a token unit: a plugin-level example must
  // exist even though both models are covered by the profiles below.
  usageExamples: [{ label: "ordered model · one request", facts: { request_count: 1, item_count: 0, prompt_tokens: 200 } }],
  // A profile replaces usageSchema and usageExamples completely for its models;
  // nothing is merged or inherited, so each profile is a complete description.
  usageProfiles: [
    {
      models: ["ordered-json"],
      schema: {
        request_count: {
          type: "number",
          unit: "count",
          description: { en: "Request unit price", zh: "请求单价" },
        },
        prompt_tokens: {
          type: "number",
          unit: "token",
          description: { en: "Input token unit price", zh: "输入词元单价" },
        },
      },
      examples: [{ label: "ordered-json · one request", facts: { request_count: 1, prompt_tokens: 200 } }],
    },
    {
      models: ["ordered-json-large"],
      schema: {
        item_count: {
          type: "number",
          unit: "count",
          unitLabel: { en: "item", zh: "项" },
          description: { en: "Item processing unit price", zh: "项目处理单价" },
        },
        prompt_tokens: {
          type: "number",
          unit: "token",
          description: { en: "Input token unit price", zh: "输入词元单价" },
        },
      },
      examples: [{ label: "ordered-json-large · three items", facts: { item_count: 3, prompt_tokens: 900 } }],
    },
  ],
  routes: [
    { method: "POST", path: "/v1/ordered/tasks", type: "submit", decode: "decodeSubmit", render: "renderSubmit" },
    { method: "GET", path: "/v1/ordered/tasks/:task_id", type: "query", render: "renderTask" },
  ],
};

// With json-order@1 the body members keep the client's order. The host writes
// the descriptor body with JSON.stringify semantics, so member order in the
// objects this plugin builds is what the upstream receives.
function decodeSubmit(ctx) {
  if (!ctx.body || ctx.body.kind !== "json") throw new Error("JSON body required");
  const body = ctx.body.value || {};
  if (typeof body.model !== "string" || body.model === "") throw new Error("model is required");
  if (body.document === undefined) throw new Error("document is required");
  const items = Array.isArray(body.document) ? body.document.length : 1;
  return { kind: "submit", model: body.model, requestBody: { document: body.document, items: items } };
}

export function buildSubmitRequest(ctx) {
  const body = ctx.requestBody || {};
  const upstreamModel = ctx.upstreamModel || ctx.model;
  return {
    url: ctx.baseUrl + "/v1/ordered/tasks",
    method: "POST",
    headers: { "Content-Type": "application/json", Authorization: ctx.authHeader },
    // Order matters only when the model reads the JSON as text; this example
    // builds the members in the order the vendor documents.
    body: { model: upstreamModel, document: body.document, items: body.items },
    action: upstreamModel === "ordered-json-large" ? "batch" : "single",
  };
}

// state carries the plugin's own bookkeeping across polls. The host writes it
// only when the hook returns one, and rejects oversized state with a warning
// rather than truncating it, so keep it small.
export function parseSubmitResponse(ctx, response) {
  const body = response.body || {};
  if (typeof body.task_id !== "string" || body.task_id === "") throw new Error("upstream returned no task id");
  return {
    taskId: body.task_id,
    taskData: { task_id: body.task_id, status: String(body.status || "queued") },
    state: { polls: 0 },
  };
}

export function extractUsage(ctx) {
  if (ctx.usagePurpose === "billing_ratios") return null;
  const body = ctx.requestBody || {};
  if ((ctx.upstreamModel || ctx.model) === "ordered-json-large") {
    return { item_count: Number(body.items) || 0, prompt_tokens: 0 };
  }
  return { request_count: 1, prompt_tokens: 0 };
}

export function buildQueryRequest(ctx) {
  return {
    url: ctx.baseUrl + "/v1/ordered/tasks/" + encodeURIComponent(ctx.taskId),
    method: "GET",
    headers: { Authorization: ctx.authHeader },
  };
}

export function parseTaskResult(ctx, body, response) {
  const data = body || {};
  // ctx.state is whatever the previous hook returned; it is null before the
  // first poll that set it.
  const previous = ctx.state && typeof ctx.state.polls === "number" ? ctx.state.polls : 0;
  const state = { polls: previous + 1 };
  const status = String(data.status || "").toLowerCase();
  switch (status) {
    case "queued":
      return { status: "QUEUED", data: data, state: state };
    case "running":
      return { status: "IN_PROGRESS", progress: String(data.percent || "") + "%", data: data, state: state };
    case "succeeded":
      return { status: "SUCCESS", data: data, state: state };
    case "failed":
      return { status: "FAILURE", reason: String(data.error || "task failed"), data: data, state: state };
    default:
      // Never substitute a guessed status: an unrecognized reply must stay
      // UNKNOWN so the host counts a poll failure.
      return { status: "UNKNOWN", data: data, state: state };
  }
}

export function extractUsageOnComplete(ctx, result, data) {
  const usage = (data || {}).usage || {};
  const profile = (data || {}).items_processed;
  if ((ctx.upstreamModel || ctx.model) === "ordered-json-large") {
    return { item_count: Number(profile) || 0, prompt_tokens: Number(usage.prompt_tokens) || 0 };
  }
  return { request_count: 1, prompt_tokens: Number(usage.prompt_tokens) || 0 };
}

export const native = {
  decodeSubmit: decodeSubmit,
  renderSubmit: function (ctx, task) {
    return { task_id: task.task_id, status: String((task.data || {}).status || "queued") };
  },
  renderTask: function (ctx, task) {
    return {
      task_id: task.task_id,
      status: String(task.status || ""),
      reason: String(task.fail_reason || ""),
      data: task.data === undefined ? null : task.data,
    };
  },
  error: function (ctx, error) {
    return { error: { message: error.message } };
  },
};
