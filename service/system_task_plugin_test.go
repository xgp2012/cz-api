package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
)

const pluginJobTestSource = `
export const meta = {
  key: 'nightly_report',
  name: 'Nightly Report',
  version: '1.0.0',  author: { name: 'Example' },
  apiVersion: 2,
  kind: 'job',
}

export function taskType() { return 'plugin:nightly_report' }
export function enabled() { return true }
export function intervalSeconds() { return 3600 }
export function newPayload() { return { since: 'last' } }
export function run(request) { return { handled: request.type } }
`

func compilePluginJob(t *testing.T, source string) *jsplugin.LoadedPlugin {
	t.Helper()
	plugin, err := jsplugin.CompilePlugin(source, jsplugin.Options{})
	require.NoError(t, err)
	return plugin
}

// The scheduler asks a job for its interval and payload before running it, so a
// job bound to the wrong namespace would silently fire under another plugin's
// lock. Registration is the only place that can catch that.
func TestRegisterPluginJobHandlerBindsPluginNamespace(t *testing.T) {
	plugin := compilePluginJob(t, pluginJobTestSource)
	t.Cleanup(func() { unregisterSystemTaskHandlerForTest(plugin) })

	require.NoError(t, RegisterPluginJobHandler(plugin))

	handlers := map[string]SystemTaskHandler{}
	for _, handler := range registeredSystemTaskHandlers() {
		handlers[handler.Type()] = handler
	}
	handler, ok := handlers["plugin:nightly_report"]
	require.True(t, ok, "job plugin must register under its own namespace")

	scheduled, ok := handler.(ScheduledSystemTaskHandler)
	require.True(t, ok, "job plugin must be schedulable")
	assert.True(t, scheduled.Enabled())
	assert.Equal(t, int64(3600), int64(scheduled.Interval().Seconds()))
	assert.Equal(t, map[string]any{"since": "last"}, scheduled.NewPayload())
}

// A job is untrusted code: it must not be able to claim a built-in task type,
// and a plugin whose run export is missing must fail at registration rather
// than at the first scheduled fire.
func TestRegisterPluginJobHandlerRejectsInvalidJobs(t *testing.T) {
	nonJob := compilePluginJob(t, `
export const meta = {
  key: 'plain_task',
  name: 'Plain Task',
  version: '1.0.0',  author: { name: 'Example' },
  apiVersion: 2,
  kind: 'task',
  models: ['m'],
  fetchMode: 'per_task',
}
export function buildSubmitRequest() { return { url: 'https://example.test', method: 'POST' } }
export function parseSubmitResponse() { return { taskId: 't' } }
export function parseTaskResult() { return { status: 'SUCCESS' } }
export function buildQueryRequest() { return { url: 'https://example.test', method: 'GET' } }
`)
	assert.ErrorContains(t, RegisterPluginJobHandler(nonJob), "not a job plugin")

	noRun := compilePluginJob(t, `
export const meta = {
  key: 'lazy_job',
  name: 'Lazy Job',
  version: '1.0.0',  author: { name: 'Example' },
  apiVersion: 2,
  kind: 'job',
}
export function enabled() { return true }
`)
	assert.ErrorContains(t, RegisterPluginJobHandler(noRun), "must export run")

	wrongNamespace := compilePluginJob(t, `
export const meta = {
  key: 'sneaky_job',
  name: 'Sneaky Job',
  version: '1.0.0',  author: { name: 'Example' },
  apiVersion: 2,
  kind: 'job',
}
export function taskType() { return 'plugin:someone_else' }
export function run() { return null }
`)
	assert.ErrorContains(t, RegisterPluginJobHandler(wrongNamespace), "namespace")

	assert.Error(t, RegisterPluginJobHandler(nil))
}

// A job that cannot report Enabled must stay off. Defaulting an unreadable flag
// to "scheduled" would run untrusted code the operator never turned on.
func TestPluginJobHandlerFailsClosed(t *testing.T) {
	plugin := compilePluginJob(t, `
export const meta = {
  key: 'quiet_job',
  name: 'Quiet Job',
  version: '1.0.0',  author: { name: 'Example' },
  apiVersion: 2,
  kind: 'job',
}
export function enabled() { throw new Error('nope') }
export function intervalSeconds() { return 0 }
export function run() { return null }
`)
	t.Cleanup(func() { unregisterSystemTaskHandlerForTest(plugin) })
	require.NoError(t, RegisterPluginJobHandler(plugin))

	handler := &pluginJobHandler{plugin: plugin, taskType: "plugin:quiet_job"}
	assert.False(t, handler.Enabled())
	assert.Equal(t, pluginJobDefaultFreq, handler.Interval())
}

func unregisterSystemTaskHandlerForTest(plugin *jsplugin.LoadedPlugin) {
	taskType := "plugin:" + plugin.Meta.Key
	systemTaskHandlersMu.Lock()
	defer systemTaskHandlersMu.Unlock()
	delete(systemTaskHandlers, taskType)
}

// The job request projection must not leak the lease holder, which is a node
// identity, or anything else the plugin has no business seeing.
func TestPluginJobRequestOmitsLease(t *testing.T) {
	task := &model.SystemTask{
		TaskID:   "task-7",
		Type:     "plugin:nightly_report",
		LockedBy: "lease-runner-7",
	}
	request := pluginJobRequest(task)

	assert.Equal(t, task.TaskID, request["task_id"])
	assert.Equal(t, task.Type, request["type"])
	for key := range request {
		assert.NotContains(t, strings.ToLower(key), "runner")
		assert.NotContains(t, strings.ToLower(key), "lock")
	}
}
