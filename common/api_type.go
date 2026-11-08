package common

import "github.com/QuantumNous/new-api/constant"

// ChannelType2APIType resolves the API type of a channel type.
//
// It reads the provider registry installed at startup by
// relay/providerregistry, the same table relay.GetAdaptor reads, through the
// function variable below. The indirection is required by the import graph: the
// provider packages implement the adaptor interface and import common, while
// this package cannot import them. relay/providerregistry sets the variable from
// its init, and it is part of every binary that can relay, so a request never
// observes it unset.
//
// An unregistered channel type still falls back to the OpenAI API type so
// existing stored rows keep relaying, but reports ok=false so callers can tell a
// known channel from an unknown one. The task plugin channel type is the one
// exception: it is served by the task relay and must never resolve to an
// adaptor, which is why it keeps the -1 sentinel.
func ChannelType2APIType(channelType int) (int, bool) {
	apiType, ok := constant.APITypeOpenAI, false
	if resolveChannelAPIType != nil {
		apiType, ok = resolveChannelAPIType(channelType)
	}
	if ok {
		return apiType, true
	}
	if channelType == constant.ChannelTypeTaskPlugin {
		return -1, false
	}
	return constant.APITypeOpenAI, false
}

// SetChannelAPITypeResolver installs the provider registry lookup. It must be
// called once at startup and panics on a second call:
// a silent replacement could point a channel at the wrong provider.
func SetChannelAPITypeResolver(resolver func(channelType int) (int, bool)) {
	if resolver == nil {
		panic("common: nil channel API type resolver")
	}
	if resolveChannelAPIType != nil {
		panic("common: channel API type resolver already installed")
	}
	resolveChannelAPIType = resolver
}

var resolveChannelAPIType func(channelType int) (int, bool)

func SupportsResponsesCompact(channelType, apiType int) bool {
	switch apiType {
	case constant.APITypeOpenAI,
		constant.APITypeCodex,
		constant.APITypeAdvancedCustom,
		constant.APITypeSub2API,
		constant.APITypeNewAPI:
		return true
	default:
		return false
	}
}
