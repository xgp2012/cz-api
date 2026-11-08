package providerregistry_test

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/providerregistry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// removedAPITypeSlot is the `_` gap kept in the constant.APIType* block for the
// retired AIProxyLibrary API type: it is persisted nowhere, but the value stayed
// reserved so older deployment notes stay unambiguous.
const removedAPITypeSlot = 7

// TestEveryAPITypeHasExactlyOneProvider is the guard that makes the provider
// table the single registration point: adding an API type without an adaptor, or
// forgetting the entry for a new channel type, fails here instead of silently
// falling back to the OpenAI adaptor at runtime.
func TestEveryAPITypeHasExactlyOneProvider(t *testing.T) {
	covered := make(map[int]int, constant.APITypeDummy)
	for _, entry := range providerregistry.Entries() {
		covered[entry.APIType]++
	}
	for apiType := range constant.APITypeDummy {
		if apiType == removedAPITypeSlot {
			assert.Zero(t, covered[apiType], "the retired API type slot must stay unregistered")
			continue
		}
		assert.Positive(t, covered[apiType], "api type %d has no registered adaptor", apiType)
	}
	assert.NotContains(t, covered, constant.APITypeDummy, "the sentinel API type must not be registered")
}

// TestChannelType2APITypeUsesTheProviderTable pins the seam between the provider
// table and the API type lookup shared by the controller and middleware layers.
func TestChannelType2APITypeUsesTheProviderTable(t *testing.T) {
	for _, entry := range providerregistry.Entries() {
		apiType, ok := common.ChannelType2APIType(entry.ChannelType)
		require.True(t, ok, "channel type %d should resolve", entry.ChannelType)
		assert.Equal(t, entry.APIType, apiType, "channel type %d", entry.ChannelType)
		assert.NotNil(t, entry.NewAdaptor(), "channel type %d must construct an adaptor", entry.ChannelType)
	}

	for _, unregistered := range []int{
		6,  // retired OpenAIMax slot
		28, // unassigned
		constant.ChannelPluginTypeStart,
	} {
		apiType, ok := common.ChannelType2APIType(unregistered)
		assert.False(t, ok, "channel type %d must report ok=false", unregistered)
		assert.Equal(t, constant.APITypeOpenAI, apiType, "channel type %d keeps the OpenAI fallback", unregistered)
	}

	apiType, ok := common.ChannelType2APIType(constant.ChannelTypeTaskPlugin)
	assert.False(t, ok, "the task plugin channel has no adaptor")
	assert.Equal(t, -1, apiType, "the task plugin channel must not fall back to an adaptor")
}

// TestAPITypeLookupIsDeterministic documents that several channel types may
// share one API type and that the representative entry is the lowest channel
// type, so relay.GetAdaptor does not depend on map iteration order.
func TestAPITypeLookupIsDeterministic(t *testing.T) {
	for apiType := range constant.APITypeDummy {
		entry, ok := providerregistry.LookupByAPIType(apiType)
		if !ok {
			continue
		}
		for _, other := range providerregistry.Entries() {
			if other.APIType == apiType {
				assert.LessOrEqual(t, entry.ChannelType, other.ChannelType,
					"api type %d should be represented by its lowest channel type", apiType)
			}
		}
	}
}

// TestBuiltinAdaptorAPITypesMatchProviderRegistry keeps the constant package's
// local adaptor index in step with this table. The index exists only because
// model cannot import this package (the provider packages depend on model), and
// it is duplicated knowledge, so it must be checked rather than trusted.
func TestBuiltinAdaptorAPITypesMatchProviderRegistry(t *testing.T) {
	for _, entry := range providerregistry.Entries() {
		apiType, ok := constant.BuiltinAdaptorAPIType(entry.ChannelType)
		require.True(t, ok, "channel type %d is registered here but missing from constant.channel_adaptors.go", entry.ChannelType)
		assert.Equal(t, entry.APIType, apiType, "channel type %d disagrees on its API type", entry.ChannelType)
	}

	registered := make(map[int]bool, len(providerregistry.Entries()))
	for _, entry := range providerregistry.Entries() {
		registered[entry.ChannelType] = true
	}
	for _, metadata := range constant.ChannelCatalog() {
		assert.Equal(t, registered[metadata.Type], constant.HasBuiltinAdaptor(metadata.Type),
			"channel type %d disagrees between the catalog and the adaptor table", metadata.Type)
	}
}
