package main

import (
	"fmt"
	"github.com/dave/dst"
	"github.com/dave/dst/dstutil"
	"io/ioutil"
	"path/filepath"
)

type RuntimeInstrument struct {
}

func NewRuntimeInstrument() *RuntimeInstrument {
	return &RuntimeInstrument{}
}

func (r *RuntimeInstrument) HookPoints() []*InstrumentPoint {
	return []*InstrumentPoint{
		{
			Package: "runtime",
			File:    "runtime2.go",
			FilterAndEdit: func(cursor *dstutil.Cursor) bool {
				switch n := cursor.Node().(type) {
				case *dst.TypeSpec:
					// append tls into goroutine
					if n.Name != nil && n.Name.Name != "g" {
						return false
					}
					st, ok := n.Type.(*dst.StructType)
					if !ok {
						return false
					}
					st.Fields.List = append(st.Fields.List, &dst.Field{
						Names: []*dst.Ident{dst.NewIdent("swtls")},
						Type:  dst.NewIdent("interface{}"),
					})
					return true
				}
				return false
			},
		},
		{
			Package: "runtime",
			File:    "proc.go",
			FilterAndEdit: func(cursor *dstutil.Cursor) bool {
				switch n := cursor.Node().(type) {
				case *dst.FuncDecl:
					if n.Name.Name != "newproc1" {
						return false
					}

					if len(n.Type.Results.List) != 1 {
						return false
					}
					parameterNames := enhanceParameterNames(n.Type.Params)
					// enhance the result names
					resultNames := enhanceParameterNames(n.Type.Results)
					n.Body.List = append(goStringToStmts(fmt.Sprintf(`defer func() {
	if %s != nil && %s != nil {
		%s.swtls = %s.swtls
	}
}()`, resultNames[0].Name, parameterNames[1].Name, resultNames[0].Name, parameterNames[1].Name), false), n.Body.List...)
					return true
				}

				return false
			},
		},
	}
}

func (r *RuntimeInstrument) ExtraChangesForEnhancedFile(filepath string) error {
	return nil
}

func (r *RuntimeInstrument) WriteExtraFiles(basePath string) ([]string, error) {
	//if p1, p2, inv, keep := _sw_write_extra_file(&r, &basePath); !keep {
	//	return p1, p2
	//} else {
	//	defer _sw_write_extra_file_ret(inv, r1, r2)
	//}
	tlsExt := filepath.Join(basePath, "skywalking.go")
	if err := ioutil.WriteFile(tlsExt, []byte(`package runtime

import (
	_ "unsafe"
)

//go:linkname _skywalking_tls_get _skywalking_tls_get
var _skywalking_tls_get = _skywalking_tls_get_impl

//go:linkname _skywalking_tls_set _skywalking_tls_set
var _skywalking_tls_set = _skywalking_tls_set_impl

//go:nosplit
func _skywalking_tls_get_impl() interface{} {
	return getg().m.curg.swtls
}

//go:nosplit
func _skywalking_tls_set_impl(v interface{}) {
	getg().m.curg.swtls = v
}
`), 0644); err != nil {
		return nil, err
	}
	return []string{tlsExt}, nil
}
