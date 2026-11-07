// Example: a kind "job" plugin (apiVersion 2).
//
// meta.kind says what the host may use the plugin for. Omitted, it is "task",
// the asynchronous relay driver of the other examples. "job" instead
// contributes a scheduled system task, so it has no relay surface at all:
//
//   * apiVersion must be 2 — a non-task kind on apiVersion 1 is rejected;
//   * routes, protocols and channelTypes are rejected when declared, because a
//     route, a host protocol claim, or a channel binding would be silently
//     ignored for a plugin that never drives a relay request;
//   * models and fetchMode are not required (they describe how a driver is
//     reached), so this manifest declares neither;
//   * none of the task driver hooks are required.
//
// The host side (service.SystemTaskHandler) keys a handler by a system task
// type. Plugin types live in the "plugin:" namespace and are built from the
// plugin key, so this plugin owns "plugin:example-nightly" and can neither
// impersonate nor collide with a built-in type such as "log_cleanup".
export const meta = {
  apiVersion: 2,
  kind: "job",
  key: "example-nightly",
  name: "Example Nightly",
  icon: "text",
  description: {
    en: "Scheduled system task example",
    zh: "定时系统任务示例",
  },
  version: "1.0.0",
  author: { name: "Example Author" },
};

// The task type this plugin owns. The scheduler creates one task of this type
// every interval when it is enabled; the runner then claims it and calls the
// hook below with the payload NewPayload produced.
export function taskType() {
  return "plugin:" + meta.key;
}

// Whether the scheduler should create this task periodically. A job that is
// only triggered on demand returns false here and is created by the host.
export function enabled() {
  return true;
}

// Seconds between runs. A scheduling host runs its existing task when the
// configured interval has elapsed since the last run of this type.
export function intervalSeconds() {
  return 24 * 60 * 60;
}

// The payload stored with a scheduled task. It must be plain JSON; the runner
// hands it back to run() unchanged.
export function newPayload() {
  return { batch_size: 100 };
}

// Runs one claimed task. A job hook must not expect relay or channel context:
// there is no request, no channel and no model. Return a result describing what
// happened. A job reports failure by throwing; the runner records the task as
// failed and the next scheduled run creates a new one.
//
// Keep the work bounded and resumable: the runner holds a lease for the task and
// cancels the run when the lease is lost.
export function run(payload) {
  const limit = Number((payload || {}).batch_size);
  const batchSize = Number.isFinite(limit) && limit > 0 ? Math.min(limit, 1000) : 100;
  return {
    ok: true,
    // A real job would do its work here and report counts.
    scanned: batchSize,
    changed: 0,
  };
}

// Optional: a short human-readable summary surfaced with the task result.
export function describe(result) {
  const value = result || {};
  return String(value.scanned || 0) + " scanned, " + String(value.changed || 0) + " changed";
}
