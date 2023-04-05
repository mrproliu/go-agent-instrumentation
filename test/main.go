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
	//http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
	//	defer request.Body.Close()
	//	writer.Write([]byte("ok"))
	//})
	//
	//err := http.ListenAndServe(":9999", nil)
	//log.Fatal(err)
}
