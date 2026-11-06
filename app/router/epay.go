package router

import (
	"github.com/gin-gonic/gin"
	"github.com/v03413/bepusdt/app/handler/epay"
)

func epayInit(engine *gin.Engine) {
	epHdr := new(epay.Epay)
	{
		engine.POST("/submit.php", epHdr.Submit)
		engine.GET("/submit.php", epHdr.Submit)
		// 查单接口由下游服务端直接调用，不带后台 session，故不使用 GetRegister/PostRegister
		engine.GET("/api.php", epHdr.Api)
		engine.POST("/api.php", epHdr.Api)
	}
}
