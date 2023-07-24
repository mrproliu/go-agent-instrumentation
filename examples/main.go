package main

import (
	"time"
)

// go build -a -work -toolexec D:\Project\Golang\go-agent-instrumentation\cmd\cmd.exe .
func main() {
	go ServerGinBefore()
	go CheckAddressGinBefore()
	time.Sleep(time.Hour)
}
