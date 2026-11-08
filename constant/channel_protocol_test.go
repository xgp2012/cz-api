package constant

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestChannelTypeProtocolsCoverEveryAPIType keeps the protocol names complete: a
// channel type directory entry with an empty protocol would tell the console
// nothing about what a channel speaks.
func TestChannelTypeProtocolsCoverEveryAPIType(t *testing.T) {
	seen := make(map[string]int, len(ChannelTypeProtocols))
	for apiType := range APITypeDummy {
		if apiType == 7 {
			continue
		}
		name, ok := ChannelTypeProtocols[apiType]
		require.True(t, ok, "api type %d has no protocol name", apiType)
		assert.NotEmpty(t, name)
		if other, duplicate := seen[name]; duplicate {
			t.Fatalf("protocol name %q is used by api types %d and %d", name, other, apiType)
		}
		seen[name] = apiType
	}
}

// TestChannelTypeProtocolsCoverEveryBuiltinAdaptor keeps a channel from
// advertising no protocol because its API type was added to the adaptor index
// without a protocol name.
func TestChannelTypeProtocolsCoverEveryBuiltinAdaptor(t *testing.T) {
	for channelType, apiType := range builtinAdaptorAPITypes {
		assert.NotEmpty(t, ChannelTypeProtocols[apiType], "channel type %d has no protocol name", channelType)
	}
}
