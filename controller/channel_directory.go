package controller

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

// GetChannelTypeDirectory serves the channel types this gateway can offer,
// joined from the adaptor registry, the channel metadata registry, and the
// loaded task plugins.
//
// It is the one source of truth for the console's channel type list: the
// providers, their default base URL, the protocol each speaks, whether a new
// channel may use the type, and which task plugins a type can bind. A console
// that reads this no longer needs a hand-maintained copy, which is what let the
// old frontend table offer retired channel types and miss plugin ones.
func GetChannelTypeDirectory(c *gin.Context) {
	common.ApiSuccess(c, model.ChannelTypeDirectory())
}
