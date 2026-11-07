package constant

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetChannelBaseURLIsBoundsSafe(t *testing.T) {
	assert.Empty(t, GetChannelBaseURL(ChannelTypeTaskPlugin))
	assert.Empty(t, GetChannelBaseURL(9999))
}

func TestGetChannelTypeNameMatchesNameTable(t *testing.T) {
	for channelType, want := range ChannelTypeNames {
		assert.Equal(t, want, GetChannelTypeName(channelType), "channel type %d", channelType)
	}
	assert.Equal(t, "Unknown", GetChannelTypeName(28), "an unassigned value keeps the fallback name")
	assert.Equal(t, "Unknown", GetChannelTypeName(ChannelPluginTypeStart), "a plugin-reserved value has no built-in name")
	assert.Equal(t, "Unknown", GetChannelTypeName(9999))
}

func TestChannelCatalogIsSortedByTypeAndCoversNameTable(t *testing.T) {
	catalog := ChannelCatalog()
	require.NotEmpty(t, catalog)
	for i := 1; i < len(catalog); i++ {
		require.Less(t, catalog[i-1].Type, catalog[i].Type, "the catalog must be strictly ascending by channel type")
	}

	byType := make(map[int]ChannelMetadata, len(catalog))
	for _, metadata := range catalog {
		require.False(t, IsPluginChannelType(metadata.Type), "plugin channel types have no built-in metadata")
		byType[metadata.Type] = metadata
	}
	for channelType, name := range ChannelTypeNames {
		metadata, ok := byType[channelType]
		require.True(t, ok, "channel type %d is missing from the catalog", channelType)
		assert.Equal(t, name, metadata.Name)
	}
}

func TestChannelCatalogRecordsMatchExistingChannelValues(t *testing.T) {
	tests := []struct {
		channelType     int
		name            string
		baseURL         string
		requiresBaseURL bool
		deprecated      bool
	}{
		{channelType: ChannelTypeUnknown, name: "Unknown"},
		{channelType: ChannelTypeOpenAI, name: "OpenAI", baseURL: "https://api.openai.com"},
		{channelType: ChannelTypeAzure, name: "Azure", requiresBaseURL: true},
		{channelType: ChannelTypeCustom, name: "Custom", requiresBaseURL: true},
		{channelType: ChannelTypeAnthropic, name: "Anthropic", baseURL: "https://api.anthropic.com"},
		{channelType: ChannelTypeCodex, name: "ChatGPT Subscription (Codex)", baseURL: "https://chatgpt.com"},
		{channelType: ChannelTypeNewAPI, name: "New API", requiresBaseURL: true},
		{channelType: ChannelTypeTaskPlugin, name: "Task Plugin", requiresBaseURL: true},
		{channelType: ChannelTypeSGLang, name: "SGLang", requiresBaseURL: true},
		{channelType: ChannelTypeXunfei, name: "Xunfei", deprecated: true},
		{channelType: 6, baseURL: "https://api.openaimax.com", deprecated: true},
		{channelType: 9, baseURL: "https://api.caipacity.com", deprecated: true},
	}
	for _, test := range tests {
		metadata, ok := GetChannelMetadata(test.channelType)
		require.True(t, ok, "channel type %d must be registered", test.channelType)
		assert.Equal(t, test.channelType, metadata.Type)
		assert.Equal(t, test.name, metadata.Name, "channel type %d name", test.channelType)
		assert.Equal(t, test.baseURL, metadata.BaseURL, "channel type %d base URL", test.channelType)
		assert.Equal(t, test.requiresBaseURL, metadata.RequiresBaseURL, "channel type %d base URL requirement", test.channelType)
		assert.Equal(t, test.deprecated, metadata.Deprecated, "channel type %d deprecation", test.channelType)
		assert.False(t, metadata.Plugin)
	}

	for _, channelType := range []int{6, 7, 9, 12, 13, 22} {
		metadata, ok := GetChannelMetadata(channelType)
		require.True(t, ok)
		assert.Empty(t, metadata.Name, "reserved slot %d keeps the Unknown name fallback", channelType)
	}

	_, ok := GetChannelMetadata(28)
	assert.False(t, ok, "unassigned values in the built-in range are not described")
}

func TestChannelCatalogBaseURLsMatchLegacyTable(t *testing.T) {
	for _, metadata := range ChannelCatalog() {
		assert.Equal(t, GetChannelBaseURL(metadata.Type), metadata.BaseURL, "channel type %d base URL", metadata.Type)
	}
}

func TestChannelCatalogIsACopy(t *testing.T) {
	catalog := ChannelCatalog()
	require.NotEmpty(t, catalog)
	catalog[0].Name = "mutated"

	refetched, ok := GetChannelMetadata(catalog[0].Type)
	require.True(t, ok)
	assert.NotEqual(t, "mutated", refetched.Name, "callers must not be able to mutate the registry")
}

func TestIsPluginChannelType(t *testing.T) {
	tests := []struct {
		channelType int
		want        bool
	}{
		{channelType: ChannelTypeUnknown, want: false},
		{channelType: ChannelTypeOpenAI, want: false},
		{channelType: ChannelTypeDummy, want: false},
		{channelType: ChannelPluginTypeStart - 1, want: false},
		{channelType: ChannelPluginTypeStart, want: true},
		{channelType: ChannelPluginTypeStart + 1, want: true},
		{channelType: 9999, want: true},
	}
	for _, test := range tests {
		assert.Equal(t, test.want, IsPluginChannelType(test.channelType), "channel type %d", test.channelType)
	}

	_, ok := GetChannelMetadata(ChannelPluginTypeStart)
	assert.False(t, ok, "lookup must not resolve plugin-reserved values to built-in metadata")
}

func TestIsChannelTypeNameTaken(t *testing.T) {
	for channelType, name := range ChannelTypeNames {
		assert.True(t, IsChannelTypeNameTaken(name), "built-in name %q of channel type %d", name, channelType)
	}
	assert.True(t, IsChannelTypeNameTaken("  new api "), "built-in names are reserved regardless of case and padding")
	assert.True(t, IsChannelTypeNameTaken("SGLang"))

	assert.False(t, IsChannelTypeNameTaken("plugin-vendor"))
	assert.False(t, IsChannelTypeNameTaken(""))
	assert.False(t, IsChannelTypeNameTaken("   "))
}
