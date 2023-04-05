package gin

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/mrproliu/go-agent-instrumentation/framework/core"
	"time"
)

type ServerHTTPInterceptor struct {
}

func (s *ServerHTTPInterceptor) BeforeInvoke(invocation *core.Invocation) error {
	context := invocation.Args[0].(*gin.Context)
	fmt.Printf("request: %s\nheaders: %v\n", context.Request.RequestURI, context.Request.Header)
	core.SetGLS("test")
	go func() {
		time.Sleep(time.Second)
		fmt.Printf("go routine TLS: %v\n", core.GetGLS())
	}()
	return nil
}

func (s *ServerHTTPInterceptor) AfterInvoke(invocation *core.Invocation, result ...interface{}) error {
	fmt.Print("after\n")
	return nil
}
