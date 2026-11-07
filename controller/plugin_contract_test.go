package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const contractProbeTaskSource = `
export const meta = {
  apiVersion: 2, kind: "task", key: "contract-probe-task", name: "Contract Probe", version: "1.2.3",
  author: {name: "Probe", url: "https://example.com/probe"},
  description: {en: "Contract probe"},
  requiredCapabilities: ["json-order@1"],
  channelTypes: [7781],
  models: ["contract-model"], fetchMode: "per_task",
  allowedHosts: ["api.vendor.example"],
  upstreams: ["vendor", "new_api"],
  auth: {type: "api_key"},
  protocols: [{name: "openai_responses", supports: ["stream"]}],
  usageSchema: {seconds: {type: "number", unit: "second"}}
};
export function buildSubmitRequest() { return {}; }
export function parseSubmitResponse() { return {}; }
export function buildQueryRequest() { return {}; }
export function parseTaskResult() { return {}; }
export const protocols = {openai_responses: {decodeRequest: function () { return {}; }, renderEvents: function () { return {}; }}};
`

const contractProbeJobSource = `
export const meta = {
  apiVersion: 2, kind: "job", key: "contract-probe-job", name: "Probe Job", version: "0.1.0",
  author: {name: "Probe"}
};
export function run() { return {}; }
`

// pluginContractSecrets are manifest members the read-only contract routes must
// never publish: they name a host, a credential, or a pricing vector.
var pluginContractSecrets = []string{"api.vendor.example", "new_api", "allowedHosts", "usageSchema", "api_key", `"auth"`, "baseUrl", "upstreams"}

func runPluginContractHandler(t *testing.T, handler gin.HandlerFunc, params gin.Params, path string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Params = params
	context.Request = httptest.NewRequest(http.MethodGet, path, nil)
	handler(context)
	return recorder
}

func decodePluginContractData(t *testing.T, recorder *httptest.ResponseRecorder, target any) {
	t.Helper()
	var response struct {
		Success bool `json:"success"`
		Data    any  `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	encoded, err := common.Marshal(response.Data)
	require.NoError(t, err)
	require.NoError(t, common.Unmarshal(encoded, target))
}

// The list and detail routes publish the manifest projection a client may read:
// identity, kind, version, and the relay surfaces, and nothing that names a host
// or a credential.
func TestPluginContractRoutesPublishNonSensitiveMetadata(t *testing.T) {
	setupTaskPluginControllerTest(t)
	cleanupTaskPluginControllerRuntime(t, "contract-probe-task")
	cleanupTaskPluginControllerRuntime(t, "contract-probe-job")
	_, err := jsplugin.DefaultRegistry.Register(contractProbeTaskSource, jsplugin.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { jsplugin.DefaultRegistry.Unregister("contract-probe-task") })
	_, err = jsplugin.DefaultRegistry.RegisterFactory(contractProbeJobSource, jsplugin.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { jsplugin.DefaultRegistry.Unregister("contract-probe-job") })

	listRecorder := runPluginContractHandler(t, ListPlugins, nil, "/api/plugin")
	assert.Equal(t, http.StatusOK, listRecorder.Code)
	var listed []pluginMetadataSummary
	decodePluginContractData(t, listRecorder, &listed)
	byKey := make(map[string]pluginMetadataSummary, len(listed))
	for _, item := range listed {
		byKey[item.Key] = item
	}
	task, ok := byKey["contract-probe-task"]
	require.True(t, ok)
	assert.Equal(t, "task", task.Kind)
	assert.Equal(t, jsplugin.APIVersion2, task.APIVersion)
	assert.Equal(t, "1.2.3", task.Version)
	assert.Equal(t, jsplugin.AuthorMeta{Name: "Probe", URL: "https://example.com/probe"}, task.Author)
	assert.Equal(t, jsplugin.LocalizedText{"en": "Contract probe"}, task.Description)
	assert.Equal(t, []string{"json-order@1"}, task.Capabilities)
	assert.Equal(t, []int{7781}, task.ChannelTypes)
	assert.Equal(t, []string{"contract-model"}, task.Models)
	require.Len(t, task.Protocols, 1)
	assert.Equal(t, "openai_responses", task.Protocols[0].Name)
	assert.Equal(t, []string{"stream"}, task.Protocols[0].Supports)
	job, ok := byKey["contract-probe-job"]
	require.True(t, ok)
	assert.Equal(t, jsplugin.PluginKindJob, job.Kind)
	assert.Empty(t, job.Models, "a job plugin declares no relay models")
	assert.Empty(t, job.ChannelTypes)

	detailRecorder := runPluginContractHandler(t, GetPlugin, gin.Params{{Key: "key", Value: "contract-probe-task"}}, "/api/plugin/contract-probe-task")
	assert.Equal(t, http.StatusOK, detailRecorder.Code)
	var detail pluginMetadataSummary
	decodePluginContractData(t, detailRecorder, &detail)
	assert.Equal(t, task, detail)

	for _, body := range []string{listRecorder.Body.String(), detailRecorder.Body.String()} {
		for _, secret := range pluginContractSecrets {
			assert.NotContains(t, body, secret, "the contract must not publish %s", secret)
		}
	}
}

func TestGetPluginAnswersNotFoundForUnknownKey(t *testing.T) {
	setupTaskPluginControllerTest(t)

	recorder := runPluginContractHandler(t, GetPlugin, gin.Params{{Key: "key", Value: "no-such-plugin"}}, "/api/plugin/no-such-plugin")

	assert.Equal(t, http.StatusNotFound, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"success":false`)
	assert.NotContains(t, recorder.Body.String(), "controller/", "the not-found body must not leak a source path")
	assert.NotContains(t, recorder.Body.String(), `\\`, "the not-found body must not leak a filesystem path")
}

type pluginKindContractPayload struct {
	Kind                string   `json:"kind"`
	MinAPIVersion       int      `json:"minApiVersion"`
	RequiredMetaFields  []string `json:"requiredMetaFields"`
	ForbiddenMetaFields []string `json:"forbiddenMetaFields"`
	RequiredHooks       []struct {
		Hook  string `json:"hook"`
		Field string `json:"field"`
		Value string `json:"value"`
	} `json:"requiredHooks"`
	DeclarationVariants []struct {
		Field string   `json:"field"`
		Value string   `json:"value"`
		Hooks []string `json:"hooks"`
	} `json:"declarationVariants"`
}

// The kind contract publishes the hook requirements the compiler enforces. A
// manifest run through the published variant fields must demand exactly the
// hooks the compiler demands, or the endpoint has drifted from the compiler.
func TestGetPluginKindsPublishesCompilerDerivedHookRequirements(t *testing.T) {
	setupTaskPluginControllerTest(t)

	recorder := runPluginContractHandler(t, GetPluginKinds, nil, "/api/plugin/kinds")
	assert.Equal(t, http.StatusOK, recorder.Code)
	var kinds []pluginKindContractPayload
	decodePluginContractData(t, recorder, &kinds)
	require.Len(t, kinds, 3)

	contracts := make(map[string]pluginKindContractPayload, len(kinds))
	for _, kind := range kinds {
		contracts[kind.Kind] = kind
	}
	task := contracts[jsplugin.PluginKindTask]
	require.NotNil(t, task.RequiredHooks)

	// A manifest value replaces the kind's hook set with a derived one. The
	// published variant, applied to a manifest, must yield exactly the hooks the
	// compiler demands for that manifest.
	baseHooks := make([]string, 0, len(task.RequiredHooks))
	for _, requirement := range task.RequiredHooks {
		baseHooks = append(baseHooks, requirement.Hook)
	}
	hooksFor := func(field, value string) []string {
		for _, variant := range task.DeclarationVariants {
			if variant.Field == field && variant.Value == value {
				return variant.Hooks
			}
		}
		t.Fatalf("kind contract does not describe %s=%s", field, value)
		return nil
	}
	for _, testCase := range []struct {
		name      string
		meta      jsplugin.Meta
		delta     bool
		published []string
	}{
		{"json per_task", jsplugin.Meta{SubmitResponseTypes: []string{"json"}, FetchMode: "per_task"}, false, baseHooks},
		{
			"sse delta", jsplugin.Meta{SubmitResponseTypes: []string{"json", "sse"}, FetchMode: "per_task"}, true,
			hooksFor(jsplugin.PluginRequirementFieldSubmitResponseTypes, "sse (delta events)"),
		},
		{
			"json batch", jsplugin.Meta{SubmitResponseTypes: []string{"json"}, FetchMode: "batch"}, false,
			hooksFor(jsplugin.PluginRequirementFieldFetchMode, "batch"),
		},
	} {
		compilerHooks := make([]string, 0, len(testCase.published))
		for _, requirement := range jsplugin.TaskDriverHookRequirements(testCase.meta, testCase.delta) {
			compilerHooks = append(compilerHooks, requirement.Hook)
		}
		assert.ElementsMatch(t, compilerHooks, testCase.published, "%s: kind contract and compiler disagree", testCase.name)
	}

	// Both SSE parsers are reported as variants of submitResponseTypes, and the
	// batch variant drops the single-task query hook the compiler drops.
	assert.Contains(t, hooksFor(jsplugin.PluginRequirementFieldSubmitResponseTypes, "sse"), "parseSubmitEvent")
	assert.Contains(t, hooksFor(jsplugin.PluginRequirementFieldSubmitResponseTypes, "sse (delta events)"), "parseSubmitEventDelta")
	assert.Contains(t, hooksFor(jsplugin.PluginRequirementFieldFetchMode, "batch"), "buildBatchQueryRequest")
	assert.NotContains(t, hooksFor(jsplugin.PluginRequirementFieldFetchMode, "batch"), "buildQueryRequest")

	assert.Equal(t, jsplugin.APIVersion1, task.MinAPIVersion)
	assert.Contains(t, task.RequiredMetaFields, "models")
	for _, kind := range []string{jsplugin.PluginKindJob, jsplugin.PluginKindIdentity} {
		contract := contracts[kind]
		assert.Equal(t, jsplugin.APIVersion2, contract.MinAPIVersion)
		assert.ElementsMatch(t, []string{"routes", "protocols", "channelTypes"}, contract.ForbiddenMetaFields)
		assert.Empty(t, contract.RequiredHooks, "%s requires no driver hooks", kind)
	}
}

func TestGetPluginProtocolsPublishesHostOperationCatalog(t *testing.T) {
	setupTaskPluginControllerTest(t)

	recorder := runPluginContractHandler(t, GetPluginProtocols, nil, "/api/plugin/protocols")
	assert.Equal(t, http.StatusOK, recorder.Code)
	var catalog []pluginProtocolCatalogEntry
	decodePluginContractData(t, recorder, &catalog)
	definitions := jsplugin.HostProtocols()
	require.Len(t, catalog, len(definitions))

	for _, entry := range catalog {
		definition, ok := jsplugin.HostProtocol(entry.Name)
		require.True(t, ok, entry.Name)
		require.Len(t, entry.Operations, len(definition.Operations))
		for i, operation := range entry.Operations {
			want := definition.Operations[i]
			assert.Equal(t, want.Name, operation.Name)
			assert.Equal(t, want.Methods, operation.Methods)
			assert.Equal(t, want.Path, operation.Path)
			assert.Equal(t, want.ModelField, operation.ModelField)
			assert.Equal(t, want.RequiredProtocolMembers, operation.RequiredProtocolMembers)
			assert.Equal(t, want.RequiredDriverHooks, operation.RequiredDriverHooks)
			bodyKinds := make([]string, len(want.BodyKinds))
			for j, bodyKind := range want.BodyKinds {
				bodyKinds[j] = string(bodyKind)
			}
			assert.Equal(t, bodyKinds, operation.BodyKinds)
		}
		assert.Equal(t, len(definition.DefinedModes()), len(entry.Modes), "%s lists each defined mode once", entry.Name)
	}

	// A plugin author reads the responses stream hook from the catalog.
	for _, entry := range catalog {
		if entry.Name != "openai_responses" {
			continue
		}
		require.NotEmpty(t, entry.Modes)
		assert.Equal(t, "stream", entry.Modes[0].Name)
		assert.Equal(t, "renderEvents", entry.Modes[0].Hook)
		return
	}
	t.Fatal("openai_responses missing from the host protocol catalog")
}
