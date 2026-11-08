package model

import (
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/constant"
	jsplugin "github.com/QuantumNous/new-api/pkg/jsplugin"
)

// PluginChannelBinding names one plugin a channel of a given type can bind.
type PluginChannelBinding struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// ChannelTypeDirectoryEntry is one channel type the console offers, with every
// piece of fixed information the channel form needs. It is computed from the
// adaptor index, the channel metadata registry, and the plugin registry, so the
// console never keeps its own copy of this table.
type ChannelTypeDirectoryEntry struct {
	Type            int    `json:"type"`
	Name            string `json:"name"`
	BaseURL         string `json:"baseURL"`
	RequiresBaseURL bool   `json:"requiresBaseURL"`
	Deprecated      bool   `json:"deprecated"`
	// Plugin reports whether the type is declared by a task plugin rather than
	// built into the gateway.
	Plugin bool   `json:"plugin"`
	Kind   string `json:"kind"`
	// Protocol names the API protocol family the gateway relays for this type,
	// so the console can describe a channel without a second table.
	Protocol string `json:"protocol"`
	// Configurable reports whether a channel of this type can be created: it
	// has a native adaptor or a loaded plugin declares it.
	Configurable bool `json:"configurable"`
	// TaskPlugins lists the plugins a channel of this type can bind.
	TaskPlugins []PluginChannelBinding `json:"taskPlugins,omitempty"`
	// PluginKey names the declaring plugin when Plugin is true.
	PluginKey string `json:"pluginKey,omitempty"`
}

// ChannelTypeDirectory joins the registries that each know part of a channel
// type into the one list the console consumes.
//
// The gateway is the only place that knows which channel types exist, what they
// speak, and which adaptor serves them: constant.ChannelCatalog (name, default
// base URL, deprecation), the built-in adaptor index, and the task plugin
// registry (plugin-declared types and the plugins a type can bind). Publishing
// the join once removes the hand-maintained copy the console used to carry,
// which had already drifted from the backend.
//
// Retired and plugin-only entries stay in the list, marked by Kind, Deprecated
// and Configurable, because stored channels may still use them and the model
// list must keep resolving them.
func ChannelTypeDirectory() []ChannelTypeDirectoryEntry {
	catalog := constant.ChannelCatalog()
	entries := make([]ChannelTypeDirectoryEntry, 0, len(catalog))

	for _, metadata := range catalog {
		entry := ChannelTypeDirectoryEntry{
			Type:            metadata.Type,
			Name:            metadata.Name,
			BaseURL:         metadata.BaseURL,
			RequiresBaseURL: metadata.RequiresBaseURL,
			Deprecated:      metadata.Deprecated,
		}
		if entry.Name == "" {
			entry.Name = constant.GetChannelTypeName(metadata.Type)
		}
		if apiType, ok := constant.BuiltinAdaptorAPIType(metadata.Type); ok {
			entry.Kind = constant.ChannelTypeKindChat
			entry.Protocol = constant.ChannelTypeProtocols[apiType]
			entry.Configurable = !metadata.Deprecated
		}
		if metadata.Type == constant.ChannelTypeTaskPlugin {
			// The placeholder type binds task plugins instead of relaying itself.
			entry.Kind = constant.ChannelTypeKindChat
			entry.Configurable = true
		}
		entries = append(entries, entry)
	}

	snapshot := jsplugin.DefaultRegistry.Snapshot()
	for _, meta := range slices.Concat(snapshot.Override, snapshot.Factory) {
		if len(meta.ChannelTypes) == 0 {
			continue
		}
		for _, channelType := range meta.ChannelTypes {
			if constant.IsPluginChannelType(channelType) {
				entries = append(entries, ChannelTypeDirectoryEntry{
					Type:            channelType,
					Name:            meta.Name,
					BaseURL:         meta.BaseURL,
					RequiresBaseURL: meta.BaseURL == "",
					Plugin:          true,
					PluginKey:       meta.Key,
					Kind:            meta.Kind,
					Configurable:    true,
				})
				continue
			}
			// A plugin may also serve a built-in type; expose that as a bind option
			// on the shared entry rather than as a duplicate channel type.
			for i := range entries {
				if entries[i].Type == channelType && !entries[i].Plugin {
					entries[i].TaskPlugins = append(entries[i].TaskPlugins, PluginChannelBinding{Key: meta.Key, Name: meta.Name})
				}
			}
		}
	}

	for i := range entries {
		slices.SortFunc(entries[i].TaskPlugins, func(left, right PluginChannelBinding) int {
			return strings.Compare(left.Key, right.Key)
		})
	}
	slices.SortStableFunc(entries, func(left, right ChannelTypeDirectoryEntry) int {
		return left.Type - right.Type
	})
	return entries
}
