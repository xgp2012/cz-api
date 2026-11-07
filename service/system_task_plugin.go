package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
)

// Plugin job contract. A plugin declaring meta.kind "job" owns the system task
// type "plugin:<key>" and may export these members. Every member is optional
// except "run": a job without it cannot do anything and is rejected at
// registration rather than at the first scheduled fire.
const (
	pluginJobExport       = "run"
	pluginJobTaskType     = "taskType"
	pluginJobEnabled      = "enabled"
	pluginJobInterval     = "intervalSeconds"
	pluginJobNewPayload   = "newPayload"
	pluginJobDefaultFreq  = time.Hour
	pluginJobIntervalUnit = time.Second
)

// pluginJobHandler adapts a kind "job" JavaScript plugin to the scheduler. It
// holds no lock and no database handle of its own: the runner claims the task
// and calls Run, and runPluginJobHook finishes the task through model, so the
// plugin never observes a billing, quota, or credential object.
type pluginJobHandler struct {
	plugin   *jsplugin.LoadedPlugin
	taskType string
}

func (h *pluginJobHandler) Type() string { return h.taskType }

func (h *pluginJobHandler) Enabled() bool {
	var enabled bool
	if err := h.plugin.Engine.CallJSONInto(context.Background(), &enabled, pluginJobEnabled); err != nil {
		// A job that cannot answer Enabled stays off: scheduling is the risky
		// direction, so an unreadable flag must not default to "yes".
		return false
	}
	return enabled
}

func (h *pluginJobHandler) Interval() time.Duration {
	var seconds float64
	if err := h.plugin.Engine.CallJSONInto(context.Background(), &seconds, pluginJobInterval); err != nil {
		return pluginJobDefaultFreq
	}
	if seconds <= 0 {
		return pluginJobDefaultFreq
	}
	return time.Duration(seconds) * pluginJobIntervalUnit
}

func (h *pluginJobHandler) NewPayload() any {
	var payload any
	if err := h.plugin.Engine.CallInto(context.Background(), &payload, pluginJobNewPayload); err != nil {
		return nil
	}
	return payload
}

func (h *pluginJobHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	// A panic inside the JS runtime must fail this one task, never the runner,
	// which also serves every other system task type on this node.
	defer func() {
		if recovered := recover(); recovered != nil {
			message := fmt.Sprintf("plugin job %q panicked: %v", h.plugin.Meta.Key, recovered)
			finishPluginJob(task, runnerID, false, nil, message)
		}
	}()

	var result any
	err := h.plugin.Engine.CallJSONInto(ctx, &result, pluginJobExport, pluginJobRequest(task))
	if err != nil {
		// Context cancellation means the lease was lost or the process is
		// shutting down; reporting a failure would corrupt the task state the
		// runner is already reconciling.
		if ctx.Err() != nil {
			return
		}
		finishPluginJob(task, runnerID, false, nil, err.Error())
		return
	}
	finishPluginJob(task, runnerID, true, result, "")
}

// pluginJobRequest is the non-secret projection a job receives. It identifies
// the task and carries the scheduler payload, but never the runner, a lease
// token, or anything from the channel/user tables.
func pluginJobRequest(task *model.SystemTask) map[string]any {
	request := map[string]any{
		"task_id": task.TaskID,
		"type":    task.Type,
	}
	if task.Payload != "" {
		var payload any
		if err := common.UnmarshalJsonStr(task.Payload, &payload); err == nil {
			request["payload"] = payload
		}
	}
	return request
}

func finishPluginJob(task *model.SystemTask, runnerID string, succeeded bool, result any, message string) {
	status := model.SystemTaskStatusFailed
	if succeeded {
		status = model.SystemTaskStatusSucceeded
		message = ""
	}
	// finishPluginJob runs on the runner's critical path. Losing the lease is a
	// normal race, so the error is deliberately not retried here.
	_ = model.FinishSystemTask(task.TaskID, runnerID, status, result, message)
}

// RegisterPluginJobHandler binds one kind "job" plugin to the scheduler under
// "plugin:<key>". It rejects a plugin that is not a job, one that omits the run
// export, and one that declares a taskType other than its own namespace.
func RegisterPluginJobHandler(plugin *jsplugin.LoadedPlugin) error {
	if plugin == nil {
		return errors.New("plugin must not be nil")
	}
	if plugin.Meta.Kind != jsplugin.PluginKindJob {
		return fmt.Errorf("plugin %q is not a job plugin", plugin.Meta.Key)
	}
	if !plugin.Engine.HasExport(pluginJobExport) {
		return fmt.Errorf("job plugin %q must export %s", plugin.Meta.Key, pluginJobExport)
	}
	taskType := model.PluginSystemTaskType(plugin.Meta.Key)
	if plugin.Engine.HasExport(pluginJobTaskType) {
		var declared string
		if err := plugin.Engine.CallJSONInto(context.Background(), &declared, pluginJobTaskType); err != nil {
			return fmt.Errorf("job plugin %q: taskType must return a string", plugin.Meta.Key)
		}
		if declared != taskType {
			return fmt.Errorf("job plugin %q declares taskType %q but its namespace is %q", plugin.Meta.Key, declared, taskType)
		}
	}
	return RegisterSystemTaskHandler(&pluginJobHandler{plugin: plugin, taskType: taskType})
}
