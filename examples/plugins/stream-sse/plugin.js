// Example: a submission that streams its result over SSE.
//
// Declaring meta.submitResponseTypes ["sse"] makes the host accept a
// text/event-stream reply to the submission, and then requires one event hook:
//   parseSubmitEvent      — the plugin keeps the whole accumulated state
//   parseSubmitEventDelta — the host accumulates, the plugin sends changes
// This file uses parseSubmitEvent (the snapshot form). buildSubmitRequest must
// select responseType: "sse" on the descriptor; the host rejects a selected type
// that the manifest does not declare.
//
// When the hook returns done: true the host stops reading and calls the normal
// parseSubmitResponse with the last state as response.body, so the usual
// immediate-completion path persists, settles and presents the result.
export const meta = {
  apiVersion: 1,
  key: "example-sse",
  name: "Example SSE",
  icon: "text",
  description: {
    en: "Streaming submit driver example",
    zh: "流式提交驱动示例",
  },
  version: "1.0.0",
  author: { name: "Example Author" },
  models: ["stream-chat"],
  fetchMode: "per_task",
  submitResponseTypes: ["sse"],
  usageSchema: {
    // The vendor reports its own token counts in the final event.
    prompt_tokens: {
      type: "number",
      unit: "token",
      description: { en: "Input token unit price", zh: "输入词元单价" },
    },
    completion_tokens: {
      type: "number",
      unit: "token",
      description: { en: "Output token unit price", zh: "输出词元单价" },
    },
  },
  // Required once usageSchema declares a token unit; labels are spec names,
  // never prices, and each facts vector covers every declared key.
  usageExamples: [{ label: "stream-chat · short reply", facts: { prompt_tokens: 40, completion_tokens: 120 } }],
  protocols: [{ name: "openai_responses", supports: ["stream"] }],
};

function decodeResponsesRequest(ctx) {
  if (!ctx.body || ctx.body.kind !== "json") throw new Error("JSON body required");
  const request = ctx.body.value || {};
  if (typeof request.input !== "string" || request.input.trim() === "") throw new Error("input must be a non-empty string");
  return { kind: "submit", model: ctx.model, requestBody: { input: request.input } };
}

export function buildSubmitRequest(ctx) {
  const body = ctx.requestBody || {};
  return {
    url: ctx.baseUrl + "/v1/stream/chat",
    method: "POST",
    headers: { "Content-Type": "application/json", Accept: "text/event-stream", Authorization: ctx.authHeader },
    body: { input: body.input, stream: true },
    // Must be declared in submitResponseTypes; the matching event hook is then
    // required at load time.
    responseType: "sse",
  };
}

// The host frames SSE for you: it joins the data: lines of one event, handles
// CRLF and heartbeats, and passes {event, id, data} strings. It does not parse
// the vendor's JSON. Return the complete state every call — pooled runtimes do
// not keep module globals between events, and previousState is read-only.
export function parseSubmitEvent(ctx, event, previousState) {
  const state = previousState || { text: "", usage: null, failed: null };
  let payload;
  try {
    payload = JSON.parse(event.data);
  } catch (error) {
    // A malformed frame is a submission failure; throwing is the right answer.
    throw new Error("upstream sent a data frame that is not JSON");
  }
  // Build owned values rather than mutating the host-backed previousState.
  if (payload.error) {
    return { state: { text: state.text, usage: state.usage, failed: String(payload.error) }, done: true };
  }
  if (payload.delta) {
    return { state: { text: state.text + String(payload.delta), usage: state.usage, failed: state.failed }, done: false };
  }
  if (payload.usage) {
    // The terminal event carries cumulative counts; SSE state is request-local,
    // so durable facts still go through parseSubmitResponse below.
    return {
      state: {
        text: state.text,
        usage: { prompt_tokens: Number(payload.usage.prompt_tokens) || 0, completion_tokens: Number(payload.usage.completion_tokens) || 0 },
        failed: state.failed,
      },
      done: true,
    };
  }
  return { state: state, done: false };
}

// Called once with the last state as response.body after done: true. Because the
// result completed inside the request, return the terminal result immediately.
export function parseSubmitResponse(ctx, response) {
  const state = response.body || {};
  if (state.failed) {
    return { taskId: ctx.publicTaskId, taskData: state, immediate: { status: "FAILURE", reason: state.failed } };
  }
  return {
    taskId: ctx.publicTaskId,
    taskData: state,
    immediate: { status: "SUCCESS", completionTokens: (state.usage || {}).completion_tokens, data: state },
  };
}

export function extractUsageOnComplete(ctx, result, data) {
  const usage = (data || {}).usage || {};
  return { prompt_tokens: Number(usage.prompt_tokens) || 0, completion_tokens: Number(usage.completion_tokens) || 0 };
}

// Required of every task driver even when the submission always completes in
// the request: an image-protocol request that polls inside the client call, or
// a re-poll of a retained task, reaches these two hooks. Because the result is
// already terminal, the query hooks are never the deciding path here.
export function buildQueryRequest(ctx) {
  return { url: ctx.baseUrl + "/v1/stream/tasks/" + encodeURIComponent(ctx.taskId), method: "GET", headers: { Authorization: ctx.authHeader } };
}

export function parseTaskResult(ctx, body) {
  const data = body || {};
  const status = String(data.status || "").toLowerCase();
  if (status === "succeeded") return { status: "SUCCESS", data: data };
  if (status === "failed") return { status: "FAILURE", reason: String(data.error || "task failed"), data: data };
  if (status === "running") return { status: "IN_PROGRESS", data: data };
  if (status === "queued") return { status: "QUEUED", data: data };
  return { status: "UNKNOWN", data: data };
}

// A host protocol plugin that only supports "stream" needs renderEvents and no
// renderFinal. The host calls renderEvents once at terminal SUCCESS for
// retrieval (GET /v1/responses/:response_id) with no previous state.
export const protocols = {
  openai_responses: {
    decodeRequest: decodeResponsesRequest,
    renderEvents: function (ctx, task, previousState) {
      const status = String(task.status || "UNKNOWN");
      const text = String(((task.data || {}).text) || "");
      if (status === "SUCCESS") {
        const events = previousState && previousState.status === status ? [] : [{ type: "output", data: text }];
        return { events: events, state: { status: status }, done: true };
      }
      if (status === "FAILURE") {
        return {
          events: [{ type: "error", code: "task_failed", message: task.fail_reason || "task failed" }],
          state: { status: status },
          done: true,
        };
      }
      return { events: [{ type: "progress", message: status.toLowerCase() }], state: { status: status }, done: false };
    },
  },
};
