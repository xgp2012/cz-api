package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestChannelTypeDirectoryDescribesEveryKnownType pins the contract the console
// depends on: every channel type the gateway knows appears exactly once, sorts by
// type, and carries the name, base URL and protocol a channel form needs.
func TestChannelTypeDirectoryDescribesEveryKnownType(t *testing.T) {
	entries := ChannelTypeDirectory()
	require.NotEmpty(t, entries)

	byType := make(map[int]ChannelTypeDirectoryEntry, len(entries))
	for i, entry := range entries {
		if i > 0 {
			assert.Less(t, entries[i-1].Type, entry.Type, "entries must be sorted and unique by type")
		}
		byType[entry.Type] = entry
	}

	for _, metadata := range constant.ChannelCatalog() {
		entry, ok := byType[metadata.Type]
		require.True(t, ok, "channel type %d is missing from the directory", metadata.Type)
		assert.Equal(t, metadata.Deprecated, entry.Deprecated, "channel type %d", metadata.Type)
		assert.NotEmpty(t, entry.Name, "channel type %d has no name", metadata.Type)
		if !metadata.Deprecated {
			assert.Equal(t, entry.Name, metadata.Name, "channel type %d", metadata.Type)
		}
	}
}

// TestChannelTypeDirectoryMarksWhatAClientMayOffer separates the types a console
// may create from the ones it must only keep for stored rows. Getting this wrong
// is what let the old frontend list offer retired providers.
func TestChannelTypeDirectoryMarksWhatAClientMayOffer(t *testing.T) {
	byType := make(map[int]ChannelTypeDirectoryEntry)
	for _, entry := range ChannelTypeDirectory() {
		byType[entry.Type] = entry
	}

	openAI := byType[constant.ChannelTypeOpenAI]
	assert.True(t, openAI.Configurable)
	assert.Equal(t, constant.ChannelTypeKindChat, openAI.Kind)
	assert.Equal(t, "openai", openAI.Protocol)
	assert.Equal(t, constant.GetChannelBaseURL(constant.ChannelTypeOpenAI), openAI.BaseURL)

	// A retired slot stays visible so stored rows still resolve, but is not
	// offerable and has no protocol because no adaptor serves it.
	retired := byType[6]
	assert.False(t, retired.Configurable)
	assert.True(t, retired.Deprecated)
	assert.Empty(t, retired.Kind)
	assert.Empty(t, retired.Protocol)

	// A channel served only by the task relay must never claim a native protocol.
	taskPlugin := byType[constant.ChannelTypeTaskPlugin]
	assert.False(t, taskPlugin.Deprecated)

	for _, channelType := range []int{constant.ChannelTypeVLLM, constant.ChannelTypeSGLang, constant.ChannelTypeAdvancedCustom} {
		assert.Equal(t, "advanced_custom", byType[channelType].Protocol, "channel type %d shares the custom protocol", channelType)
	}
}
