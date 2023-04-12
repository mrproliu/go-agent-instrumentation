package main

import (
	"bytes"
	"fmt"
	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
	"github.com/dave/dst/dstutil"
	"github.com/mrproliu/go-agent-instrumentation/framework/core"
	"github.com/mrproliu/go-agent-instrumentation/frameworks/gin"
	"html/template"
	"io"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var frameworkInstruments []core.Instrument
var frameworkGeneratePrefix = "_skywalking_enhance_"

func init() {
	frameworkInstruments = append(frameworkInstruments, &gin.Instrument{})
}

type FrameworkInstrument struct {
	points       []*InstrumentPoint
	enhances     []FrameworkEnhanceInfo
	replacements map[string]map[string]string
}

func NewFrameworkInstrument() *FrameworkInstrument {
	points := make([]*InstrumentPoint, 0)
	result := &FrameworkInstrument{}
	replacements := make(map[string]map[string]string)
	for _, inst := range frameworkInstruments {
		for _, point := range inst.Points() {
			points = append(points, func(p *core.InstrumentPoint, i core.Instrument) *InstrumentPoint {
				return &InstrumentPoint{
					Package: filepath.Join(inst.BasePackage(), point.PackagePath),
					File:    point.FileName,
					FilterAndEdit: func(cursor *dstutil.Cursor) bool {
						if p.EnhanceStruct != nil && p.EnhanceStruct(cursor) {
							spec := cursor.Node().(*dst.TypeSpec)
							enhanceInfo := NewFrameworkEnhanceTypeInfo(p, i, spec)
							result.enhances = append(result.enhances, enhanceInfo)

							enhanceInfo.EnhanceField()
						}
						if p.FilterMethod != nil && p.FilterMethod(cursor) {
							decl := cursor.Node().(*dst.FuncDecl)
							methodInfo := NewFrameworkEnhanceMethodInfo(p, i, decl)
							result.enhances = append(result.enhances, methodInfo)

							curFileReplacement := methodInfo.BuildForInvoker()

							replacementsTmp := replacements[point.FileName]
							if replacementsTmp == nil {
								replacementsTmp = make(map[string]string)
								replacements[point.FileName] = replacementsTmp
							}
							for k, v := range curFileReplacement {
								replacementsTmp[k] = v
								decl.Body.Decs.Lbrace.Prepend("\n", k)
							}
							return true
						}
						return false
					},
				}
			}(point, inst))
		}
	}
	result.points = points
	result.replacements = replacements
	return result
}

func (f *FrameworkInstrument) HookPoints() []*InstrumentPoint {
	return f.points
}

func (f *FrameworkInstrument) WriteExtraFiles(basePath string) ([]string, error) {
	if len(f.enhances) == 0 {
		return nil, nil
	}
	packageName := ""
	if f.enhances[0].GetPoint().PackagePath == "" {
		packageName = filepath.Base(f.enhances[0].GetInstrument().BasePackage())
	} else {
		packageName = filepath.Base(f.enhances[0].GetPoint().PackagePath)
	}
	file := &dst.File{
		Name: dst.NewIdent(packageName),
	}

	for _, m := range f.enhances {
		for _, fu := range m.BuildForAdapter() {
			file.Decls = append(file.Decls, fu)
		}
	}

	adapterFile := filepath.Join(basePath, "skywalking_adapter.go")
	output, err := os.Create(adapterFile)
	if err != nil {
		return nil, err
	}
	defer output.Close()
	if err := writeFile(file, output); err != nil {
		return nil, err
	}

	// intercepter file should write to the go2sky
	intercepter := filepath.Join(basePath, "sw_intercepter.go")
	if err := ioutil.WriteFile(intercepter, []byte(fmt.Sprintf(`package %s
import _ "unsafe"

var (
	GetGLS = func() interface{} { return nil }
	SetGLS = func(interface{}) {}
)

//go:linkname _skywalking_tls_get _skywalking_tls_get
var _skywalking_tls_get func() interface{}

//go:linkname _skywalking_tls_set _skywalking_tls_set
var _skywalking_tls_set func(interface{})

func init() {
	if _skywalking_tls_get != nil && _skywalking_tls_set != nil {
		GetGLS = _skywalking_tls_get
		SetGLS = _skywalking_tls_set
	}
}

type EnhancedInstance interface {
	GetSkyWalkingDynamicField() interface{}
	SetSkyWalkingDynamicField(interface{})
}

type Invocation struct {
	CallerInstance interface{}
	Args           []interface{}

	Continue bool
	Return   []interface{}
}
`, packageName)), 0644); err != nil {
		return nil, err
	}

	// temporary only process the root dir
	insFS := f.enhances[0].GetInstrument().FS()
	dirEntries, err := fs.ReadDir(insFS, ".")
	if err != nil {
		panic(err)
	}

	// import interceptors
	writedFiles := make([]string, 0)
	writedFiles = append(writedFiles, adapterFile, intercepter)
	for _, entry := range dirEntries {
		if entry.IsDir() {
			continue
		}
		if entry.Name() == "go.mod" || entry.Name() == "go.sum" || entry.Name() == "instrument.go" {
			continue
		}

		readFile, err := fs.ReadFile(insFS, entry.Name())
		if err != nil {
			return nil, err
		}

		parse, err := decorator.Parse(readFile)
		if err != nil {
			return nil, err
		}
		var currentPackageImportPath = filepath.Join(f.enhances[0].GetInstrument().BasePackage(), f.enhances[0].GetPoint().PackagePath)
		var shouldRemovePkgRef = []string{"core"}
		dstutil.Apply(parse, func(cursor *dstutil.Cursor) bool {
			node := cursor.Node()
			switch x := node.(type) {
			case *dst.ImportSpec:
				if filepath.Base(x.Path.Value) == "core\"" { // delete core import, in real case, it should be renamed to the go2sky
					cursor.Delete()
					return true
				}
				currentPackage := "\"" + currentPackageImportPath + "\""
				if x.Path.Value == currentPackage {
					if x.Name != nil {
						shouldRemovePkgRef = append(shouldRemovePkgRef, x.Name.Name)
					} else {
						shouldRemovePkgRef = append(shouldRemovePkgRef, filepath.Base(currentPackageImportPath))
					}
					cursor.Delete()
				}
			case *dst.FuncDecl: // delete core package use
				if (x.Name.Name == "BeforeInvoke" || x.Name.Name == "AfterInvoke") && len(x.Type.Params.List) > 0 {
					ref, ok := x.Type.Params.List[0].Type.(*dst.StarExpr)
					if !ok {
						return true
					}
					ref.X = dst.NewIdent("Invocation")
				}
			case *dst.SelectorExpr:
				pkgRefName, ok := x.X.(*dst.Ident)
				if !ok {
					return true
				}
				for _, ref := range shouldRemovePkgRef {
					if pkgRefName.Name == ref {
						switch p := cursor.Parent().(type) {
						case *dst.CallExpr:
							p.Fun = x.Sel
						case *dst.StarExpr:
							p.X = x.Sel
						case *dst.TypeAssertExpr:
							p.Type = x.Sel
						}
						//cursor.Parent().(dst.Expr).X = dst.NewIdent(x.Sel.Name)
					}
				}

				return true
			}
			return true
		}, func(cursor *dstutil.Cursor) bool {
			return true
		})

		path := filepath.Join(basePath, fmt.Sprintf("sw_enhance_%s", entry.Name()))
		output, err := os.Create(path)
		if err != nil {
			return nil, err
		}
		defer output.Close()
		if e := writeFile(parse, output); e != nil {
			return nil, e
		}
		writedFiles = append(writedFiles, path)
	}
	return writedFiles, nil
}

func buildFrameworkFuncID(pkgPath string, node *dst.FuncDecl) string {
	var receiver string
	if node.Recv != nil {
		expr, ok := node.Recv.List[0].Type.(*dst.StarExpr)
		if !ok {
			return ""
		}
		ident, ok := expr.X.(*dst.Ident)
		if !ok {
			return ""
		}
		receiver = ident.Name
	}
	return fmt.Sprintf("%s_%s%s",
		regexp.MustCompile(`[/.\-@]`).ReplaceAllString(pkgPath, "_"), receiver, node.Name)
}

type FrameworkEnhanceInfo interface {
	GetPoint() *core.InstrumentPoint
	GetInstrument() core.Instrument
	BuildForAdapter() []*dst.FuncDecl
}

type FrameworkEnhanceTypeInfo struct {
	Instrument core.Instrument
	Point      *core.InstrumentPoint
	TypeSpec   *dst.TypeSpec
}

func NewFrameworkEnhanceTypeInfo(p *core.InstrumentPoint, i core.Instrument, typeSpec *dst.TypeSpec) *FrameworkEnhanceTypeInfo {
	return &FrameworkEnhanceTypeInfo{Instrument: i, Point: p, TypeSpec: typeSpec}
}

func (f *FrameworkEnhanceTypeInfo) GetInstrument() core.Instrument {
	return f.Instrument
}

func (f *FrameworkEnhanceTypeInfo) GetPoint() *core.InstrumentPoint {
	return f.Point
}

func (f *FrameworkEnhanceTypeInfo) EnhanceField() {
	structType := f.TypeSpec.Type.(*dst.StructType)
	structType.Fields.List = append(structType.Fields.List, &dst.Field{
		Names: []*dst.Ident{dst.NewIdent("skywalking_dynamic_field")},
		Type:  dst.NewIdent("interface{}"),
	})
}

func (f *FrameworkEnhanceTypeInfo) BuildForAdapter() []*dst.FuncDecl {
	return []*dst.FuncDecl{
		{
			Name: &dst.Ident{Name: "GetSkyWalkingDynamicField"},
			Recv: &dst.FieldList{
				List: []*dst.Field{
					{
						Names: []*dst.Ident{dst.NewIdent("receiver")},
						Type:  &dst.StarExpr{X: dst.NewIdent(f.TypeSpec.Name.Name)},
					},
				},
			},
			Type: &dst.FuncType{
				Params: &dst.FieldList{},
				Results: &dst.FieldList{
					List: []*dst.Field{
						{Type: dst.NewIdent("interface{}")},
					},
				},
			},
			Body: &dst.BlockStmt{
				List: goStringToStmts("return receiver.skywalking_dynamic_field", false),
			},
		},
		{
			Name: &dst.Ident{Name: "SetSkyWalkingDynamicField"},
			Recv: &dst.FieldList{
				List: []*dst.Field{
					{
						Names: []*dst.Ident{dst.NewIdent("receiver")},
						Type:  &dst.StarExpr{X: dst.NewIdent(f.TypeSpec.Name.Name)},
					},
				},
			},
			Type: &dst.FuncType{
				Params: &dst.FieldList{
					List: []*dst.Field{
						{
							Names: []*dst.Ident{dst.NewIdent("param")},
							Type:  dst.NewIdent("interface{}"),
						},
					},
				},
				Results: &dst.FieldList{},
			},
			Body: &dst.BlockStmt{
				List: goStringToStmts("receiver.skywalking_dynamic_field = param", false),
			},
		},
	}
}

type FrameworkEnhanceMethodInfo struct {
	Point          *core.InstrumentPoint
	Instrument     core.Instrument
	FuncDecl       *dst.FuncDecl
	FuncParameters []*ParameterInfo
	FuncRecvs      []*ParameterInfo
	FuncResults    []*ParameterInfo

	adapterPreFuncName  string
	adapterPostFuncName string
}

func NewFrameworkEnhanceMethodInfo(p *core.InstrumentPoint, i core.Instrument, f *dst.FuncDecl) *FrameworkEnhanceMethodInfo {
	info := &FrameworkEnhanceMethodInfo{
		Point:      p,
		Instrument: i,
		FuncDecl:   f,
	}
	info.FuncParameters = enhanceParameterNames(f.Type.Params)
	info.FuncResults = enhanceParameterNames(f.Type.Results)
	if f.Recv != nil {
		info.FuncRecvs = enhanceParameterNames(f.Recv)
	}

	funcID := buildFrameworkFuncID(filepath.Join(i.BasePackage(), p.PackagePath), f)
	info.adapterPreFuncName = fmt.Sprintf("%s%s", frameworkGeneratePrefix, funcID)
	info.adapterPostFuncName = fmt.Sprintf("%s%s_ret", frameworkGeneratePrefix, funcID)
	return info
}

func (e *FrameworkEnhanceMethodInfo) GetInstrument() core.Instrument {
	return e.Instrument
}

func (e *FrameworkEnhanceMethodInfo) GetPoint() *core.InstrumentPoint {
	return e.Point
}

func (e *FrameworkEnhanceMethodInfo) BuildForInvoker() map[string]string {
	invokerResultParams := ""
	if len(e.FuncResults) > 0 {
		beforeFuncInvokeResultParams := make([]string, 0)
		for inx := range e.FuncResults {
			beforeFuncInvokeResultParams = append(beforeFuncInvokeResultParams, fmt.Sprintf("_sw_inv_res%d", inx))
		}
		invokerResultParams = strings.Join(beforeFuncInvokeResultParams, ", ") + ", "
	}

	invokerParams := ""
	if len(e.FuncRecvs) > 0 {
		receiverRefs := make([]string, 0)
		for _, n := range e.FuncRecvs {
			receiverRefs = append(receiverRefs, fmt.Sprintf("&%s", n.Name))
		}
		invokerParams = strings.Join(receiverRefs, ", ")
	}
	if len(e.FuncParameters) > 0 {
		if len(invokerParams) > 0 {
			invokerParams += ", "
		}
		paramRefs := make([]string, 0)
		for _, n := range e.FuncParameters {
			paramRefs = append(paramRefs, fmt.Sprintf("&%s", n.Name))
		}
		invokerParams += strings.Join(paramRefs, ", ")
	}

	invokerSkipReturn := invokerResultParams
	if len(invokerSkipReturn) > 0 {
		invokerSkipReturn = strings.TrimSuffix(invokerSkipReturn, ", ")
	}

	invokerRealResult := ""
	if len(e.FuncResults) > 0 {
		invokerRealResult += ", "
		paramRefs := make([]string, 0)
		for _, n := range e.FuncResults {
			paramRefs = append(paramRefs, fmt.Sprintf("&%s", n.Name))
		}
		invokerRealResult += strings.Join(paramRefs, ", ")
	}

	result := fmt.Sprintf(`if %s_sw_invocation, _sw_keep := %s(%s); !_sw_keep { return %s } else { defer %s(_sw_invocation%s) };`, invokerResultParams,
		e.adapterPreFuncName,
		invokerParams,
		invokerSkipReturn,
		e.adapterPostFuncName,
		invokerRealResult,
	)
	replacedName := fmt.Sprintf("//goagent:enhance_%s\n", e.FuncDecl.Name.Name)
	goStringToStmts(replacedName, false)
	return map[string]string{replacedName: result}
}

func (e *FrameworkEnhanceMethodInfo) BuildForAdapter() []*dst.FuncDecl {
	preFunc := &dst.FuncDecl{
		Name: &dst.Ident{Name: e.adapterPreFuncName},
		Type: &dst.FuncType{
			Params:  &dst.FieldList{},
			Results: &dst.FieldList{},
		},
	}
	for i, recv := range e.FuncRecvs {
		preFunc.Type.Params.List = append(preFunc.Type.Params.List, &dst.Field{
			Names: []*dst.Ident{dst.NewIdent(fmt.Sprintf("recv_%d", i))},
			Type:  &dst.StarExpr{X: dst.Clone(recv.Type).(dst.Expr)},
		})
	}
	for i, parameter := range e.FuncParameters {
		preFunc.Type.Params.List = append(preFunc.Type.Params.List, &dst.Field{
			Names: []*dst.Ident{dst.NewIdent(fmt.Sprintf("param_%d", i))},
			Type:  &dst.StarExpr{X: dst.Clone(parameter.Type).(dst.Expr)},
		})
	}
	for i, result := range e.FuncResults {
		preFunc.Type.Results.List = append(preFunc.Type.Results.List, &dst.Field{
			Names: []*dst.Ident{dst.NewIdent(fmt.Sprintf("ret_%d", i))},
			Type:  &dst.StarExpr{X: dst.Clone(result.Type).(dst.Expr)},
		})
	}
	preFunc.Type.Results.List = append(preFunc.Type.Results.List, &dst.Field{
		Names: []*dst.Ident{dst.NewIdent("inv")},
		Type:  &dst.StarExpr{X: dst.NewIdent("Invocation")},
	})
	preFunc.Type.Results.List = append(preFunc.Type.Results.List, &dst.Field{
		Names: []*dst.Ident{dst.NewIdent("keep")},
		Type:  dst.NewIdent("bool"),
	})

	parse, err := template.New("").Parse(`invocation := &Invocation{}
{{if .FuncRecvs -}}
invocation.CallerInstance = *recv_0	// for caller if exist
{{- end}}
invocation.Args = make([]interface{}, {{len .FuncParameters}})
{{- range $index, $value := .FuncParameters}}
invocation.Args[{{$index}}] = *param_{{$index}}
{{- end}}

inter := &{{.Point.InterceptorName}}{}
// real invoke
if err := inter.BeforeInvoke(invocation); err != nil {
	// using go2sky log error
	return {{ range $index, $value := .FuncResults -}}
{{- if ne .index 0}}, {{end}}$value.DefaultValueAsString
{{- end}}{{if .FuncResults}}, {{- end}}invocation, true
}
if (invocation.Continue) {
	return {{ range $index, $value := .FuncResults -}}
{{- if ne .index 0}}, {{end}}$value.DefaultValueAsString
{{- end}}{{if .FuncResults}}, {{- end}}invocation, false
}
return {{ range $index, $value := .FuncResults -}}
{{- if ne .index 0}}, {{end}}$value.DefaultValueAsString
{{- end}}{{if .FuncResults}}, {{- end}}invocation, true`)
	if err != nil {
		panic(fmt.Errorf("parse pre funtion failure: %v", err))
	}
	var buffer bytes.Buffer
	writer := io.Writer(&buffer)
	err = parse.Execute(writer, e)
	if err != nil {
		panic(fmt.Errorf("write pre function tmplate failure: %v", err))
	}
	preFunc.Body = &dst.BlockStmt{
		List: goStringToStmts(buffer.String(), false),
	}

	postFunc := &dst.FuncDecl{
		Name: &dst.Ident{Name: e.adapterPostFuncName},
		Type: &dst.FuncType{
			Params:  &dst.FieldList{},
			Results: &dst.FieldList{},
		},
	}
	postFunc.Type.Params.List = append(postFunc.Type.Params.List, &dst.Field{
		Names: []*dst.Ident{dst.NewIdent("invocation")},
		Type:  &dst.StarExpr{X: dst.NewIdent("Invocation")},
	})
	for inx, f := range e.FuncResults {
		postFunc.Type.Params.List = append(postFunc.Type.Params.List, &dst.Field{
			Names: []*dst.Ident{dst.NewIdent(fmt.Sprintf("ret_%d", inx))},
			Type:  &dst.StarExpr{X: dst.Clone(f.Type).(dst.Expr)},
		})
	}
	parse, err = template.New("").Parse(`inter := &{{.Point.InterceptorName}}{}
inter.AfterInvoke(invocation{{ range $index, $value := .FuncResults -}}
{{- if ne .index 0}}, {{end}}ret_$index
{{- end}})`)
	if err != nil {
		panic(fmt.Errorf("parse pre funtion failure: %v", err))
	}
	buffer.Reset()
	writer = io.Writer(&buffer)
	err = parse.Execute(writer, e)
	if err != nil {
		panic(fmt.Errorf("write pre function tmplate failure: %v", err))
	}
	postFunc.Body = &dst.BlockStmt{
		List: goStringToStmts(buffer.String(), false),
	}
	return []*dst.FuncDecl{preFunc, postFunc}
}

func (r *FrameworkInstrument) ExtraChangesForEnhancedFile(f string) error {
	filePath := filepath.Base(f)
	replacements := r.replacements[filePath]
	if replacements == nil {
		return nil
	}
	contentBytes, err := os.ReadFile(f)
	if err != nil {
		return err
	}
	contentString := string(contentBytes)
	for k, v := range replacements {
		contentString = strings.ReplaceAll(contentString, k, v)
	}
	return os.WriteFile(f, []byte(contentString), 0644)
}
