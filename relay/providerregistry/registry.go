// Package providerregistry is the single table pairing every built-in channel
// type with the API type it speaks and the adaptor that serves it.
//
// It lives in its own package because of the import graph: the provider packages
// implement the adaptor interface and most of them import common, while common
// cannot import them for the channel API type lookup. The table therefore sits
// between them and installs its lookup into common at startup:
//
//	constant <- relay/channel <- providerregistry <- <provider packages> <- relay <- common
//
// Registering a provider is one line in builtin.go. Nothing else in the codebase
// needs to know the number, the protocol, or the constructor.
package providerregistry

import (
	"github.com/QuantumNous/new-api/relay/channel"
)

// Entry describes which adaptor serves one numeric channel type and which API
// type that adaptor speaks.
type Entry struct {
	// ChannelType is the persisted channels.type value.
	ChannelType int
	// APIType is the protocol family the adaptor implements.
	APIType int
	// NewAdaptor returns a fresh adaptor for one relay attempt. Adaptors carry
	// per-request state, so this must never return a shared instance.
	NewAdaptor func() channel.Adaptor
}
