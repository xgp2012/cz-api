package constant

// channelTypeKindChat is the kind of every built-in channel type that speaks a
// chat-completion style protocol through a relay adaptor. It is the only kind
// the built-in adaptor table declares today; plugin channel types declare their
// own kind in the plugin manifest.
const channelTypeKindChat = "chat"

// ChannelTypeProtocols names the API protocol family of each API type. The
// channel type directory publishes these names so the console can describe a
// channel without keeping its own copy of the mapping.
//
// A new API type must be added here: the entry is what turns a registry number
// into a name a human and a client can act on.
var ChannelTypeProtocols = map[int]string{
	APITypeOpenAI:         "openai",
	APITypeAnthropic:      "anthropic",
	APITypePaLM:           "palm",
	APITypeBaidu:          "baidu",
	APITypeZhipu:          "zhipu",
	APITypeAli:            "ali",
	APITypeXunfei:         "xunfei",
	APITypeTencent:        "tencent",
	APITypeGemini:         "gemini",
	APITypeZhipuV4:        "zhipu_v4",
	APITypeOllama:         "ollama",
	APITypePerplexity:     "perplexity",
	APITypeAws:            "aws",
	APITypeCohere:         "cohere",
	APITypeDify:           "dify",
	APITypeJina:           "jina",
	APITypeCloudflare:     "cloudflare",
	APITypeSiliconFlow:    "siliconflow",
	APITypeVertexAi:       "vertexai",
	APITypeMistral:        "mistral",
	APITypeDeepSeek:       "deepseek",
	APITypeMokaAI:         "mokaai",
	APITypeVolcEngine:     "volcengine",
	APITypeBaiduV2:        "baidu_v2",
	APITypeOpenRouter:     "openrouter",
	APITypeXinference:     "xinference",
	APITypeXai:            "xai",
	APITypeCoze:           "coze",
	APITypeJimeng:         "jimeng",
	APITypeMoonshot:       "moonshot",
	APITypeSubmodel:       "submodel",
	APITypeMiniMax:        "minimax",
	APITypeReplicate:      "replicate",
	APITypeCodex:          "codex",
	APITypeAdvancedCustom: "advanced_custom",
	APITypeSub2API:        "sub2api",
	APITypeNewAPI:         "new_api",
}
