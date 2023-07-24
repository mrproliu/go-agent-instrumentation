package main

import (
	"fmt"
	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
	"github.com/dave/dst/dstutil"
	"go/parser"
	"go/printer"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// InstrumentPoint 函数拦截点。
type InstrumentPoint struct {
	Package       string
	File          string
	FilterAndEdit func(cursor *dstutil.Cursor) bool
}

// Instrument 增强方法接口，对于每一个待自动埋点的库/函数创建一个结构体。在本项目中创建了runtime和gin的增强实例。
type Instrument interface {
	HookPoints() []*InstrumentPoint
	ExtraChangesForEnhancedFile(filepath string) error
	WriteExtraFiles(basePath string) ([]string, error)
}

// fileInfo 待增强文件信息。
type fileInfo struct {
	argsIndex int
	// Decorated Syntax Tree 修饰语法树。
	dstFile *dst.File
	// InstrumentPoint 函数拦截点。
	instPoint []*InstrumentPoint
}

// instrument 增强方法插入过程。
func instrument(args []string, opt *compileOptions) ([]string, error) {
	var inst Instrument
	//判断编译指令中包含的库是否存在对应的增强方法，分别执行。
	switch opt.Package {
	case "runtime":
		inst = NewRuntimeInstrument()
	default:
		inst = NewFrameworkInstrument()
	}

	var buildDir = filepath.Dir(opt.Output)

	//创建拦截后的待增强文件。
	fileWithInfo := make(map[string]*fileInfo)
	//过滤构建待增强文件。
	for inx, path := range args {
		//如果文件有.go后缀，跳过。
		if !strings.HasSuffix(path, ".go") {
			continue
		}
		//遍历函数拦截点。
		for _, hp := range inst.HookPoints() {
			//如果不在同一个包内，跳过。
			if hp.Package != opt.Package {
				continue
			}
			baseName := filepath.Base(path)
			//如果同包不同文件，跳过。
			if baseName != hp.File {
				continue
			}
			//剩下的是成功匹配的同包同文件，确为需要增强改造的文件。使用修饰语法树拦截。
			file, err := decorator.ParseFile(nil, path, nil, parser.ParseComments)
			if err != nil {
				return nil, err
			}
			//往拦截后的待增强文件中写入本次改造。
			info := fileWithInfo[path]
			if info == nil {
				info = &fileInfo{
					argsIndex: inx,
					dstFile:   file,
				}
				fileWithInfo[path] = info
			}
			info.instPoint = append(info.instPoint, hp)
		}
	}

	instruments := make(map[string]bool)
	//遍历待增强文件。
	for path, info := range fileWithInfo {
		hasInstruted := false
		//覆盖写入修饰语法树。
		dstutil.Apply(info.dstFile, func(cursor *dstutil.Cursor) bool {
			for _, p := range info.instPoint {
				if p.FilterAndEdit(cursor) {
					hasInstruted = true
				}
			}
			return true
		}, func(cursor *dstutil.Cursor) bool {
			return true
		})
		if hasInstruted {
			instruments[path] = true
		}

	}

	//把增强后的文件写入构建文件夹。
	for updateFileSrc := range instruments {
		fileInfo := fileWithInfo[updateFileSrc]
		filename := filepath.Base(updateFileSrc)
		dest := filepath.Join(buildDir, filename)
		output, err := os.Create(dest)
		if err != nil {
			return nil, err
		}
		defer output.Close()
		output.WriteString(fmt.Sprintf("//line %s:1\n", updateFileSrc))
		if err := writeFile(fileInfo.dstFile, output); err != nil {
			return nil, err
		}
		if err := inst.ExtraChangesForEnhancedFile(dest); err != nil {
			return nil, err
		}
		args[fileInfo.argsIndex] = dest
	}

	//如果有其他辅助构建需要的文件，一并写入。
	files, err := inst.WriteExtraFiles(buildDir)
	if err != nil {
		return nil, err
	}
	if len(files) > 0 {
		args = append(args, files...)
	}

	return args, nil
}

func writeFile(file *dst.File, w io.Writer) error {
	fset, af, err := decorator.RestoreFile(file)
	if err != nil {
		return err
	}
	return printer.Fprint(w, fset, af)
}

func goStringToStmts(goString string, minimized bool) []dst.Stmt {
	data := fmt.Sprintf(`
package main
func main() {
%s
}`, goString)
	parsed, err := decorator.ParseFile(nil, "builder.go", data, parser.ParseComments)
	if err != nil {
		panic(fmt.Sprintf("parsing go failure: %v\n%s", err, goString))
	}

	return parsed.Decls[0].(*dst.FuncDecl).Body.List
}

type ParameterInfo struct {
	Name                 string
	Type                 dst.Expr
	DefaultValueAsString string
}

func enhanceParameterNames(fields *dst.FieldList) []*ParameterInfo {
	if fields == nil {
		return nil
	}
	result := make([]*ParameterInfo, 0)
	for i, f := range fields.List {
		defineName := fmt.Sprintf("sw_param_%d", i)
		if len(f.Names) == 0 {
			f.Names = []*dst.Ident{{Name: defineName}}
			result = append(result, NewParameterInfo(defineName, f.Type))
		} else {
			for _, n := range f.Names {
				if n.Name == "_" {
					*n = *dst.NewIdent(defineName)
					break
				}
			}

			for _, n := range f.Names {
				result = append(result, NewParameterInfo(n.Name, f.Type))
				break
			}
		}
	}
	return result
}

func NewParameterInfo(name string, tp dst.Expr) *ParameterInfo {
	result := &ParameterInfo{
		Name: name,
		Type: tp,
	}
	switch n := tp.(type) {
	case *dst.StarExpr:
		result.DefaultValueAsString = "nil"
	case *dst.UnaryExpr:
		if n.Op == token.INT || n.Op == token.FLOAT {
			result.DefaultValueAsString = "0"
		} else {
			result.DefaultValueAsString = "nil"
		}
	default:
		result.DefaultValueAsString = "nil"
	}

	return result
}
