package core

import (
	"embed"
	"github.com/dave/dst/dstutil"
)

type InstrumentPoint struct {
	PackagePath     string
	FileName        string
	FilterMethod    func(cursor *dstutil.Cursor) bool // Define which method needs intercept
	InterceptorName string                            // Interceptor struct name, execute when method intercepted
	EnhanceStruct   func(cursor *dstutil.Cursor) bool // Define which struct needs enhance
}

type Instrument interface {
	BasePackage() string
	Points() []*InstrumentPoint
	FS() *embed.FS
}
