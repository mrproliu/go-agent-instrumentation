package core

import (
	"embed"
	"github.com/dave/dst/dstutil"
)

type InstrumentPoint struct {
	PackagePath     string
	FileName        string
	FilterMethod    func(cursor *dstutil.Cursor) bool
	InterceptorName string
}

type Instrument interface {
	BasePackage() string
	Points() []*InstrumentPoint
	FS() *embed.FS
}
