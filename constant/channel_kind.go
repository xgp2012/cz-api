package constant

// ChannelTypeKindChat is the kind of every built-in channel type that relays
// through an adaptor. Plugin channel types declare their own kind in the plugin
// manifest, so a client can tell a relay channel from a task-plugin channel
// without knowing the numbers.
const ChannelTypeKindChat = "chat"
