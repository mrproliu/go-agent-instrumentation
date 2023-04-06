package gin

import (
	"embed"
	"github.com/dave/dst"
	"github.com/dave/dst/dstutil"
	"github.com/gin-gonic/gin"
	"github.com/mrproliu/go-agent-instrumentation/framework/core"
)

//go:embed *
var assets embed.FS

type Instrument struct {
}

func (i *Instrument) BasePackage() string {
	return "github.com/gin-gonic/gin"
}

func (i *Instrument) FS() *embed.FS {
	return &assets
}

func (i *Instrument) Points() []*core.InstrumentPoint {
	return []*core.InstrumentPoint{
		{
			PackagePath: "",
			FileName:    "gin.go",
			FilterMethod: func(cursor *dstutil.Cursor) bool {
				switch n := cursor.Node().(type) {
				case *dst.FuncDecl:
					if n.Name.Name == "handleHTTPRequest" && len(n.Recv.List) > 0 {
						expr, ok := n.Recv.List[0].Type.(*dst.StarExpr)
						if !ok {
							return false
						}
						ident, ok := expr.X.(*dst.Ident)
						if !ok {
							return false
						}
						return ident.Name == "Engine"
					}
				}
				return false
			},
			InterceptorName: "ServerHTTPInterceptor",
			EnhanceStruct: func(cursor *dstutil.Cursor) bool {
				switch n := cursor.Node().(type) {
				case *dst.TypeSpec:
					if n.Name.Name == "Engine" {
						return true
					}
				}
				return false
			},
		},
	}
}

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
