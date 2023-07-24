package main

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
)

func CheckAddressGinBefore() {
	r := gin.Default()
	r.GET("/address", func(c *gin.Context) {
		response, _ := http.Get("http://127.0.0.1:2025/province")
		province, _ := io.ReadAll(response.Body)

		response, _ = http.Get("http://127.0.0.1:2025/city")
		city, _ := io.ReadAll(response.Body)

		c.String(http.StatusOK, " Address : "+string(province)+" Province "+string(city)+" City ")
		fmt.Println(" Address : " + string(province) + " Province " + string(city) + " City ")
	})
	err := r.Run("127.0.0.1:2024")
	if err != nil {
		fmt.Println(err)
	}
}
