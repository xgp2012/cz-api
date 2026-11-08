package providerregistry

import (
	"slices"
)

// byChannel and byAPIType are built once from builtinEntries and then read-only.
var (
	byChannel = make(map[int]Entry, len(builtinEntries))
	byAPIType = make(map[int]Entry, len(builtinEntries))
)

func init() {
	for _, entry := range builtinEntries {
		if entry.ChannelType <= 0 {
			panic("providerregistry: channel type must be positive")
		}
		if entry.NewAdaptor == nil {
			panic("providerregistry: channel type has no adaptor constructor")
		}
		if other, exists := byChannel[entry.ChannelType]; exists {
			_ = other
			panic("providerregistry: channel type registered twice")
		}
		byChannel[entry.ChannelType] = entry
		// Several channel types may share one API type. The lowest channel type
		// represents it so the result is deterministic; all of them record the
		// same API type and equivalent adaptor behaviour for the shared protocol.
		if existing, exists := byAPIType[entry.APIType]; !exists || entry.ChannelType < existing.ChannelType {
			byAPIType[entry.APIType] = entry
		}
	}
}

// Lookup returns the entry for a channel type.
func Lookup(channelType int) (Entry, bool) {
	entry, ok := byChannel[channelType]
	return entry, ok
}

// LookupByAPIType returns the entry that represents an API type.
func LookupByAPIType(apiType int) (Entry, bool) {
	entry, ok := byAPIType[apiType]
	return entry, ok
}

// Entries returns every entry in ascending channel type order.
func Entries() []Entry {
	entries := make([]Entry, 0, len(byChannel))
	for _, entry := range byChannel {
		entries = append(entries, entry)
	}
	slices.SortFunc(entries, func(left, right Entry) int {
		return left.ChannelType - right.ChannelType
	})
	return entries
}

func lookupAPIType(channelType int) (int, bool) {
	entry, ok := byChannel[channelType]
	if !ok {
		return 0, false
	}
	return entry.APIType, true
}
