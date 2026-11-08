package providerregistry

import "github.com/QuantumNous/new-api/common"

func init() {
	common.SetChannelAPITypeResolver(lookupAPIType)
}
