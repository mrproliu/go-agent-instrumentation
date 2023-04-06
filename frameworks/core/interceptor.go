package core

import _ "unsafe"

var (
	GetGLS = func() interface{} { return nil }
	SetGLS = func(interface{}) {}
)

//go:linkname _skywalking_tls_get _skywalking_tls_get
var _skywalking_tls_get func() interface{}

//go:linkname _skywalking_tls_set _skywalking_tls_set
var _skywalking_tls_set func(interface{})

type Invocation struct {
	CallerInstance interface{}
	Args           []interface{}

	Continue bool
	Return   []interface{} // not fully implemented, return default value for now
}

type EnhancedInstance interface {
	GetSkyWalkingDynamicField() interface{}
	SetSkyWalkingDynamicField(interface{})
}

type Interceptor interface {
	BeforeInvoke(invocation *Invocation) error
	AfterInvoke(invocation *Invocation, result ...interface{}) error
}
