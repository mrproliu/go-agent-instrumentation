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

type InstrumentPoint struct {
	Package       string
	File          string
	FilterAndEdit func(cursor *dstutil.Cursor) bool
}

type Instrument interface {
	HookPoints() []*InstrumentPoint
	ExtraChangesForEnhancedFile(filepath string) error
	WriteExtraFiles(basePath string) ([]string, error)
}

func instrument(args []string, opt *compileOptions) ([]string, error) {
	var inst Instrument
	switch opt.Package {
	case "runtime":
		inst = NewRuntimeInstrument()
	default:
		inst = NewFrameworkInstrument()
	}

	var buildDir = filepath.Dir(opt.Output)

	// basic filter matched files
	fileWithInfo := make(map[string]*fileInfo)
	for inx, path := range args {
		if !strings.HasSuffix(path, ".go") {
			continue
		}
		for _, hp := range inst.HookPoints() {
			if hp.Package != opt.Package {
				continue
			}
			baseName := filepath.Base(path)
			if baseName != hp.File {
				continue
			}

			file, err := decorator.ParseFile(nil, path, nil, parser.ParseComments)
			if err != nil {
				return nil, err
			}

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

	// try to filter and edit file
	instruments := make(map[string]bool)
	for path, info := range fileWithInfo {
		hasInstruted := false
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

	// write instrumented files to the build directory
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

	// write extra files if exist
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

type fileInfo struct {
	argsIndex int
	dstFile   *dst.File
	instPoint []*InstrumentPoint
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
