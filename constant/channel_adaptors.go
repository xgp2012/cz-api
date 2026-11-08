package constant

// builtinAdaptorAPITypes lists, for every built-in channel type that relays
// through a native adaptor, the API type that adaptor speaks.
//
// This is an index into the adaptor table in relay/providerregistry, not a second
// copy of it: the registry is authoritative for which channel type exists and
// which adaptor serves it, and it verifies at startup that this map agrees with
// it once a resolver is installed. The map exists so a package that must not
// import relay/providerregistry (an import cycle, since provider packages depend
// on model) can still describe a channel type.
//
// A new built-in provider adds exactly one line to each place: here, and in
// relay/providerregistry/builtin.go.
var builtinAdaptorAPITypes = map[int]int{
	ChannelTypeOpenAI:         APITypeOpenAI,
	ChannelTypeAnthropic:      APITypeAnthropic,
	ChannelTypeBaidu:          APITypeBaidu,
	ChannelTypePaLM:           APITypePaLM,
	ChannelTypeZhipu:          APITypeZhipu,
	ChannelTypeZhipu_v4:       APITypeZhipuV4,
	ChannelTypeAli:            APITypeAli,
	ChannelTypeXunfei:         APITypeXunfei,
	ChannelTypeTencent:        APITypeTencent,
	ChannelTypeGemini:         APITypeGemini,
	ChannelTypeOllama:         APITypeOllama,
	ChannelTypePerplexity:     APITypePerplexity,
	ChannelTypeAws:            APITypeAws,
	ChannelTypeCohere:         APITypeCohere,
	ChannelTypeDify:           APITypeDify,
	ChannelTypeJina:           APITypeJina,
	ChannelCloudflare:         APITypeCloudflare,
	ChannelTypeSiliconFlow:    APITypeSiliconFlow,
	ChannelTypeVertexAi:       APITypeVertexAi,
	ChannelTypeMistral:        APITypeMistral,
	ChannelTypeDeepSeek:       APITypeDeepSeek,
	ChannelTypeMokaAI:         APITypeMokaAI,
	ChannelTypeVolcEngine:     APITypeVolcEngine,
	ChannelTypeBaiduV2:        APITypeBaiduV2,
	ChannelTypeOpenRouter:     APITypeOpenRouter,
	ChannelTypeXinference:     APITypeXinference,
	ChannelTypeXai:            APITypeXai,
	ChannelTypeCoze:           APITypeCoze,
	ChannelTypeJimeng:         APITypeJimeng,
	ChannelTypeMoonshot:       APITypeMoonshot,
	ChannelTypeSubmodel:       APITypeSubmodel,
	ChannelTypeMiniMax:        APITypeMiniMax,
	ChannelTypeReplicate:      APITypeReplicate,
	ChannelTypeCodex:          APITypeCodex,
	ChannelTypeAdvancedCustom: APITypeAdvancedCustom,
	ChannelTypeVLLM:           APITypeAdvancedCustom,
	ChannelTypeSGLang:         APITypeAdvancedCustom,
	ChannelTypeSub2API:        APITypeSub2API,
	ChannelTypeNewAPI:         APITypeNewAPI,
}

// HasBuiltinAdaptor reports whether a channel type is served by a native relay
// adaptor. It is the cheap local check; the provider registry is authoritative,
// and TestBuiltinAdaptorAPITypesMatchProviderRegistry keeps the two in step.
func HasBuiltinAdaptor(channelType int) bool {
	_, ok := builtinAdaptorAPITypes[channelType]
	return ok
}

// BuiltinAdaptorAPIType returns the API type the native adaptor of a channel type
// speaks, and false when the channel type has no native adaptor.
func BuiltinAdaptorAPIType(channelType int) (int, bool) {
	apiType, ok := builtinAdaptorAPITypes[channelType]
	return apiType, ok
}
