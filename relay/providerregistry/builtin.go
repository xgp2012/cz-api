package providerregistry

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/advancedcustom"
	"github.com/QuantumNous/new-api/relay/channel/ali"
	"github.com/QuantumNous/new-api/relay/channel/aws"
	"github.com/QuantumNous/new-api/relay/channel/baidu"
	"github.com/QuantumNous/new-api/relay/channel/baidu_v2"
	"github.com/QuantumNous/new-api/relay/channel/claude"
	"github.com/QuantumNous/new-api/relay/channel/cloudflare"
	"github.com/QuantumNous/new-api/relay/channel/codex"
	"github.com/QuantumNous/new-api/relay/channel/cohere"
	"github.com/QuantumNous/new-api/relay/channel/coze"
	"github.com/QuantumNous/new-api/relay/channel/deepseek"
	"github.com/QuantumNous/new-api/relay/channel/dify"
	"github.com/QuantumNous/new-api/relay/channel/gemini"
	"github.com/QuantumNous/new-api/relay/channel/jimeng"
	"github.com/QuantumNous/new-api/relay/channel/jina"
	"github.com/QuantumNous/new-api/relay/channel/minimax"
	"github.com/QuantumNous/new-api/relay/channel/mistral"
	"github.com/QuantumNous/new-api/relay/channel/mokaai"
	"github.com/QuantumNous/new-api/relay/channel/moonshot"
	"github.com/QuantumNous/new-api/relay/channel/newapi"
	"github.com/QuantumNous/new-api/relay/channel/ollama"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	"github.com/QuantumNous/new-api/relay/channel/palm"
	"github.com/QuantumNous/new-api/relay/channel/perplexity"
	"github.com/QuantumNous/new-api/relay/channel/replicate"
	"github.com/QuantumNous/new-api/relay/channel/siliconflow"
	"github.com/QuantumNous/new-api/relay/channel/sub2api"
	"github.com/QuantumNous/new-api/relay/channel/submodel"
	"github.com/QuantumNous/new-api/relay/channel/tencent"
	"github.com/QuantumNous/new-api/relay/channel/vertex"
	"github.com/QuantumNous/new-api/relay/channel/volcengine"
	"github.com/QuantumNous/new-api/relay/channel/xai"
	"github.com/QuantumNous/new-api/relay/channel/xunfei"
	"github.com/QuantumNous/new-api/relay/channel/zhipu"
	"github.com/QuantumNous/new-api/relay/channel/zhipu_4v"
)

// builtinEntries is the authoritative channel type -> adaptor table and the
// single registration point for a built-in provider.
//
// Adding a provider means adding one line here. Common ChannelType2APIType and
// relay.GetAdaptor both read this table, so a channel can no longer be mapped to
// an API type without an adaptor to serve it, and a provider can no longer be
// half-registered. registry_test.go fails when a declared API type is missing.
//
// Several channel types legitimately share an adaptor when they speak the same
// protocol: OpenRouter and Xinference are OpenAI-compatible, Moonshot serves the
// Anthropic protocol, and BaiduV2 and the vLLM/SGLang presets reuse an adaptor.
var builtinEntries = []Entry{
	{ChannelType: constant.ChannelTypeOpenAI, APIType: constant.APITypeOpenAI, NewAdaptor: func() channel.Adaptor { return &openai.Adaptor{} }},
	{ChannelType: constant.ChannelTypeAnthropic, APIType: constant.APITypeAnthropic, NewAdaptor: func() channel.Adaptor { return &claude.Adaptor{} }},
	{ChannelType: constant.ChannelTypeBaidu, APIType: constant.APITypeBaidu, NewAdaptor: func() channel.Adaptor { return &baidu.Adaptor{} }},
	{ChannelType: constant.ChannelTypePaLM, APIType: constant.APITypePaLM, NewAdaptor: func() channel.Adaptor { return &palm.Adaptor{} }},
	{ChannelType: constant.ChannelTypeZhipu, APIType: constant.APITypeZhipu, NewAdaptor: func() channel.Adaptor { return &zhipu.Adaptor{} }},
	{ChannelType: constant.ChannelTypeZhipu_v4, APIType: constant.APITypeZhipuV4, NewAdaptor: func() channel.Adaptor { return &zhipu_4v.Adaptor{} }},
	{ChannelType: constant.ChannelTypeAli, APIType: constant.APITypeAli, NewAdaptor: func() channel.Adaptor { return &ali.Adaptor{} }},
	{ChannelType: constant.ChannelTypeXunfei, APIType: constant.APITypeXunfei, NewAdaptor: func() channel.Adaptor { return &xunfei.Adaptor{} }},
	{ChannelType: constant.ChannelTypeTencent, APIType: constant.APITypeTencent, NewAdaptor: func() channel.Adaptor { return &tencent.DispatchAdaptor{} }},
	{ChannelType: constant.ChannelTypeGemini, APIType: constant.APITypeGemini, NewAdaptor: func() channel.Adaptor { return &gemini.Adaptor{} }},
	{ChannelType: constant.ChannelTypeOllama, APIType: constant.APITypeOllama, NewAdaptor: func() channel.Adaptor { return &ollama.Adaptor{} }},
	{ChannelType: constant.ChannelTypePerplexity, APIType: constant.APITypePerplexity, NewAdaptor: func() channel.Adaptor { return &perplexity.Adaptor{} }},
	{ChannelType: constant.ChannelTypeAws, APIType: constant.APITypeAws, NewAdaptor: func() channel.Adaptor { return &aws.Adaptor{} }},
	{ChannelType: constant.ChannelTypeCohere, APIType: constant.APITypeCohere, NewAdaptor: func() channel.Adaptor { return &cohere.Adaptor{} }},
	{ChannelType: constant.ChannelTypeDify, APIType: constant.APITypeDify, NewAdaptor: func() channel.Adaptor { return &dify.Adaptor{} }},
	{ChannelType: constant.ChannelTypeJina, APIType: constant.APITypeJina, NewAdaptor: func() channel.Adaptor { return &jina.Adaptor{} }},
	{ChannelType: constant.ChannelCloudflare, APIType: constant.APITypeCloudflare, NewAdaptor: func() channel.Adaptor { return &cloudflare.Adaptor{} }},
	{ChannelType: constant.ChannelTypeSiliconFlow, APIType: constant.APITypeSiliconFlow, NewAdaptor: func() channel.Adaptor { return &siliconflow.Adaptor{} }},
	{ChannelType: constant.ChannelTypeVertexAi, APIType: constant.APITypeVertexAi, NewAdaptor: func() channel.Adaptor { return &vertex.Adaptor{} }},
	{ChannelType: constant.ChannelTypeMistral, APIType: constant.APITypeMistral, NewAdaptor: func() channel.Adaptor { return &mistral.Adaptor{} }},
	{ChannelType: constant.ChannelTypeDeepSeek, APIType: constant.APITypeDeepSeek, NewAdaptor: func() channel.Adaptor { return &deepseek.Adaptor{} }},
	{ChannelType: constant.ChannelTypeMokaAI, APIType: constant.APITypeMokaAI, NewAdaptor: func() channel.Adaptor { return &mokaai.Adaptor{} }},
	{ChannelType: constant.ChannelTypeVolcEngine, APIType: constant.APITypeVolcEngine, NewAdaptor: func() channel.Adaptor { return &volcengine.Adaptor{} }},
	{ChannelType: constant.ChannelTypeBaiduV2, APIType: constant.APITypeBaiduV2, NewAdaptor: func() channel.Adaptor { return &baidu_v2.Adaptor{} }},
	{ChannelType: constant.ChannelTypeOpenRouter, APIType: constant.APITypeOpenRouter, NewAdaptor: func() channel.Adaptor { return &openai.Adaptor{} }},
	{ChannelType: constant.ChannelTypeXinference, APIType: constant.APITypeXinference, NewAdaptor: func() channel.Adaptor { return &openai.Adaptor{} }},
	{ChannelType: constant.ChannelTypeXai, APIType: constant.APITypeXai, NewAdaptor: func() channel.Adaptor { return &xai.Adaptor{} }},
	{ChannelType: constant.ChannelTypeCoze, APIType: constant.APITypeCoze, NewAdaptor: func() channel.Adaptor { return &coze.Adaptor{} }},
	{ChannelType: constant.ChannelTypeJimeng, APIType: constant.APITypeJimeng, NewAdaptor: func() channel.Adaptor { return &jimeng.Adaptor{} }},
	{ChannelType: constant.ChannelTypeMoonshot, APIType: constant.APITypeMoonshot, NewAdaptor: func() channel.Adaptor { return &moonshot.Adaptor{} }},
	{ChannelType: constant.ChannelTypeSubmodel, APIType: constant.APITypeSubmodel, NewAdaptor: func() channel.Adaptor { return &submodel.Adaptor{} }},
	{ChannelType: constant.ChannelTypeMiniMax, APIType: constant.APITypeMiniMax, NewAdaptor: func() channel.Adaptor { return &minimax.Adaptor{} }},
	{ChannelType: constant.ChannelTypeReplicate, APIType: constant.APITypeReplicate, NewAdaptor: func() channel.Adaptor { return &replicate.Adaptor{} }},
	{ChannelType: constant.ChannelTypeCodex, APIType: constant.APITypeCodex, NewAdaptor: func() channel.Adaptor { return &codex.Adaptor{} }},
	{ChannelType: constant.ChannelTypeAdvancedCustom, APIType: constant.APITypeAdvancedCustom, NewAdaptor: func() channel.Adaptor { return &advancedcustom.Adaptor{} }},
	{ChannelType: constant.ChannelTypeVLLM, APIType: constant.APITypeAdvancedCustom, NewAdaptor: func() channel.Adaptor { return &advancedcustom.Adaptor{} }},
	{ChannelType: constant.ChannelTypeSGLang, APIType: constant.APITypeAdvancedCustom, NewAdaptor: func() channel.Adaptor { return &advancedcustom.Adaptor{} }},
	{ChannelType: constant.ChannelTypeSub2API, APIType: constant.APITypeSub2API, NewAdaptor: func() channel.Adaptor { return &sub2api.Adaptor{} }},
	{ChannelType: constant.ChannelTypeNewAPI, APIType: constant.APITypeNewAPI, NewAdaptor: func() channel.Adaptor { return &newapi.Adaptor{} }},
}
