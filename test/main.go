package main

import (
	"github.com/gin-gonic/gin"
)

func main() {
	engine := gin.New()
	engine.Handle("GET", "/", func(context *gin.Context) {
		context.String(200, "success")
	})

	engine.Run(":9999")
}
