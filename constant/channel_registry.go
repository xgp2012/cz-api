package constant

import (
	"fmt"
	"slices"
	"strings"
)

// ChannelPluginTypeStart is the first numeric channel type reserved for channels
// declared by task plugins. Values from this number up never belong to a
// built-in adaptor, so a plugin-declared channel cannot be mistaken for one.
const ChannelPluginTypeStart = 1000

// ChannelMetadata describes one known channel type. Numeric values come from
// channel.go and are persisted in the channels.type column, so they must never
// be reused or renumbered.
type ChannelMetadata struct {
	// Type is the persisted numeric channel type.
	Type int `json:"type"`
	// Name is the canonical display name, or empty when the registry has no name
	// for the type and GetChannelTypeName falls back to "Unknown".
	Name string `json:"name"`
	// BaseURL is the built-in default upstream base URL the channel form offers as
	// a placeholder hint. It is empty when the channel has none.
	BaseURL string `json:"base_url"`
	// RequiresBaseURL reports whether the channel cannot relay without an
	// operator-supplied base URL.
	RequiresBaseURL bool `json:"requires_base_url"`
	// Plugin reports whether this type is in the plugin-reserved range.
	Plugin bool `json:"plugin"`
	// Deprecated reports whether the type is kept only for stored rows and should
	// not be offered for new channels.
	Deprecated bool `json:"deprecated"`
}

// channelRegistry is the single authoritative description of built-in channel
// types, sorted ascending by Type so lookups can binary search it. Values and
// names mirror channel.go; plugin types are never listed here.
var channelRegistry = []ChannelMetadata{
	{Type: ChannelTypeUnknown, Name: "Unknown"},
	{Type: ChannelTypeOpenAI, Name: "OpenAI", BaseURL: "https://api.openai.com"},
	{Type: ChannelTypeMidjourney, Name: "Midjourney", BaseURL: "https://oa.api2d.net", Deprecated: true},
	{Type: ChannelTypeAzure, Name: "Azure", RequiresBaseURL: true},
	{Type: ChannelTypeOllama, Name: "Ollama", BaseURL: "http://localhost:11434"},
	{Type: ChannelTypeMidjourneyPlus, Name: "MidjourneyPlus", BaseURL: "https://api.openai-sb.com", Deprecated: true},
	{Type: 6, BaseURL: "https://api.openaimax.com", Deprecated: true},
	{Type: 7, BaseURL: "https://api.ohmygpt.com", Deprecated: true},
	{Type: ChannelTypeCustom, Name: "Custom", RequiresBaseURL: true},
	{Type: 9, BaseURL: "https://api.caipacity.com", Deprecated: true},
	{Type: ChannelTypeAIProxy, Name: "AIProxy", BaseURL: "https://api.aiproxy.io", Deprecated: true},
	{Type: ChannelTypePaLM, Name: "PaLM", RequiresBaseURL: true, Deprecated: true},
	{Type: 12, BaseURL: "https://api.api2gpt.com", Deprecated: true},
	{Type: 13, BaseURL: "https://api.aigc2d.com", Deprecated: true},
	{Type: ChannelTypeAnthropic, Name: "Anthropic", BaseURL: "https://api.anthropic.com"},
	{Type: ChannelTypeBaidu, Name: "Baidu", BaseURL: "https://aip.baidubce.com"},
	{Type: ChannelTypeZhipu, Name: "Zhipu", BaseURL: "https://open.bigmodel.cn"},
	{Type: ChannelTypeAli, Name: "Ali", BaseURL: "https://dashscope.aliyuncs.com"},
	{Type: ChannelTypeXunfei, Name: "Xunfei", Deprecated: true},
	{Type: ChannelType360, Name: "360", BaseURL: "https://api.360.cn"},
	{Type: ChannelTypeOpenRouter, Name: "OpenRouter", BaseURL: "https://openrouter.ai/api"},
	{Type: ChannelTypeAIProxyLibrary, Name: "AIProxyLibrary", BaseURL: "https://api.aiproxy.io", Deprecated: true},
	{Type: 22, BaseURL: "https://fastgpt.run/api/openapi", Deprecated: true},
	{Type: ChannelTypeTencent, Name: "Tencent", BaseURL: "https://hunyuan.tencentcloudapi.com"},
	{Type: ChannelTypeGemini, Name: "Gemini", BaseURL: "https://generativelanguage.googleapis.com"},
	{Type: ChannelTypeMoonshot, Name: "Moonshot", BaseURL: "https://api.moonshot.cn"},
	{Type: ChannelTypeZhipu_v4, Name: "ZhipuV4", BaseURL: "https://open.bigmodel.cn"},
	{Type: ChannelTypePerplexity, Name: "Perplexity", BaseURL: "https://api.perplexity.ai"},
	{Type: ChannelTypeLingYiWanWu, Name: "LingYiWanWu", BaseURL: "https://api.lingyiwanwu.com"},
	{Type: ChannelTypeAws, Name: "AWS", RequiresBaseURL: true},
	{Type: ChannelTypeCohere, Name: "Cohere", BaseURL: "https://api.cohere.ai"},
	{Type: ChannelTypeMiniMax, Name: "MiniMax", BaseURL: "https://api.minimax.chat"},
	{Type: ChannelTypeSunoAPI, Name: "SunoAPI", RequiresBaseURL: true},
	{Type: ChannelTypeDify, Name: "Dify", BaseURL: "https://api.dify.ai"},
	{Type: ChannelTypeJina, Name: "Jina", BaseURL: "https://api.jina.ai"},
	{Type: ChannelCloudflare, Name: "Cloudflare", BaseURL: "https://api.cloudflare.com"},
	{Type: ChannelTypeSiliconFlow, Name: "SiliconFlow", BaseURL: "https://api.siliconflow.cn"},
	{Type: ChannelTypeVertexAi, Name: "VertexAI", RequiresBaseURL: true},
	{Type: ChannelTypeMistral, Name: "Mistral", BaseURL: "https://api.mistral.ai"},
	{Type: ChannelTypeDeepSeek, Name: "DeepSeek", BaseURL: "https://api.deepseek.com"},
	{Type: ChannelTypeMokaAI, Name: "MokaAI", BaseURL: "https://api.moka.ai"},
	{Type: ChannelTypeVolcEngine, Name: "VolcEngine", BaseURL: "https://ark.cn-beijing.volces.com"},
	{Type: ChannelTypeBaiduV2, Name: "BaiduV2", BaseURL: "https://qianfan.baidubce.com"},
	{Type: ChannelTypeXinference, Name: "Xinference", RequiresBaseURL: true},
	{Type: ChannelTypeXai, Name: "xAI", BaseURL: "https://api.x.ai"},
	{Type: ChannelTypeCoze, Name: "Coze", BaseURL: "https://api.coze.cn"},
	{Type: ChannelTypeKling, Name: "Kling", BaseURL: "https://api.klingai.com"},
	{Type: ChannelTypeJimeng, Name: "Jimeng", BaseURL: "https://visual.volcengineapi.com"},
	{Type: ChannelTypeVidu, Name: "Vidu", BaseURL: "https://api.vidu.cn"},
	{Type: ChannelTypeSubmodel, Name: "Submodel", BaseURL: "https://llm.submodel.ai"},
	{Type: ChannelTypeDoubaoVideo, Name: "Doubao", BaseURL: "https://ark.cn-beijing.volces.com"},
	{Type: ChannelTypeSora, Name: "Sora", BaseURL: "https://api.openai.com"},
	{Type: ChannelTypeReplicate, Name: "Replicate", BaseURL: "https://api.replicate.com"},
	{Type: ChannelTypeCodex, Name: "ChatGPT Subscription (Codex)", BaseURL: "https://chatgpt.com"},
	{Type: ChannelTypeAdvancedCustom, Name: "Advanced Custom", RequiresBaseURL: true},
	{Type: ChannelTypeSub2API, Name: "Sub2API", RequiresBaseURL: true},
	{Type: ChannelTypeNewAPI, Name: "New API", RequiresBaseURL: true},
	{Type: ChannelTypeTaskPlugin, Name: "Task Plugin", RequiresBaseURL: true},
	{Type: ChannelTypeVLLM, Name: "vLLM", RequiresBaseURL: true},
	{Type: ChannelTypeSGLang, Name: "SGLang", RequiresBaseURL: true},
}

func init() {
	// The registry seeds every name shipped in ChannelTypeNames, so a new channel
	// type added to channel.go must be described here too instead of silently
	// missing from the catalog served to the frontend and to plugins.
	for channelType, name := range ChannelTypeNames {
		if metadata, ok := GetChannelMetadata(channelType); !ok || metadata.Name != name {
			panic(fmt.Sprintf("channel registry is missing channel type %d (%q) from ChannelTypeNames", channelType, name))
		}
	}
}

// ChannelCatalog returns every known channel type sorted by numeric value.
func ChannelCatalog() []ChannelMetadata {
	return slices.Clone(channelRegistry)
}

// GetChannelMetadata looks up a built-in channel type. A value the registry does
// not describe reports false and a zero metadata value.
func GetChannelMetadata(channelType int) (ChannelMetadata, bool) {
	if IsPluginChannelType(channelType) {
		return ChannelMetadata{}, false
	}
	index, found := slices.BinarySearchFunc(channelRegistry, channelType, func(metadata ChannelMetadata, channelType int) int {
		return metadata.Type - channelType
	})
	if !found {
		return ChannelMetadata{}, false
	}
	return channelRegistry[index], true
}

// IsPluginChannelType reports whether a channel type belongs to the
// plugin-reserved range instead of a built-in channel type.
func IsPluginChannelType(channelType int) bool {
	return channelType >= ChannelPluginTypeStart
}

// IsChannelTypeNameTaken reports whether a name already belongs to a known
// channel type, ignoring case. Plugin-declared channels must not reuse a
// built-in name, because the name is how operators see and select a channel.
func IsChannelTypeNameTaken(name string) bool {
	trimmed := strings.TrimSpace(name)
	return slices.ContainsFunc(channelRegistry, func(metadata ChannelMetadata) bool {
		return metadata.Name != "" && strings.EqualFold(metadata.Name, trimmed)
	})
}
