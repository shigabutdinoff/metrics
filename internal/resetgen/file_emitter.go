package resetgen

import (
	"bytes"
	"fmt"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/imports"
)

type generationContext struct {
	marked      map[*types.TypeName]bool
	traversal   map[*types.TypeName]bool
	constraints *constraintChecker
	metadata    []*packages.Package
	dispatches  map[resetTypeID][]int
}

func resetFile(pkg *packages.Package, structs []*types.Named, context *generationContext) ([]byte, error) {
	names := make(nameAllocator)
	for _, scope := range []*types.Scope{pkg.Types.Scope(), types.Universe} {
		for _, name := range scope.Names() {
			names[name] = true
		}
	}
	for _, file := range pkg.Syntax {
		for _, spec := range file.Imports {
			if name := pkg.TypesInfo.PkgNameOf(spec); name != nil && name.Name() != "_" && name.Name() != "." {
				names[name.Name()] = true
			}
		}
	}
	declarations, err := reservePackageNames(pkg, context.metadata, names)
	if err != nil {
		return nil, err
	}
	names["_"] = true

	emitter := &fileEmitter{
		pkg:          pkg,
		marked:       context.marked,
		traversal:    context.traversal,
		names:        names,
		declarations: declarations,
		constraints:  context.constraints,
		dispatches:   context.dispatches,
	}
	for _, n := range structs {
		if err := emitter.emitStruct(n); err != nil {
			return nil, err
		}
	}

	return emitter.source()
}

type fileEmitter struct {
	pkg          *packages.Package
	marked       map[*types.TypeName]bool
	traversal    map[*types.TypeName]bool
	names        nameAllocator
	helpers      resetHelpers
	declarations map[string]bool
	constraints  *constraintChecker
	dispatches   map[resetTypeID][]int
	body         bytes.Buffer
}

func (e *fileEmitter) emitStruct(n *types.Named) error {
	e.helpers.parameters = make(map[*types.TypeParam]bool)
	name := n.Obj().Name()
	if e.declarations["nil"] {
		return fmt.Errorf("%s: предопределённый nil затенён объявлением пакета", name)
	}
	recv := e.names.take(strings.ToLower(string([]rune(name)[:1])))
	receiverType := n
	if params := n.TypeParams(); params.Len() > 0 {
		args := make([]types.Type, params.Len())
		for i := range args {
			obj := types.NewTypeName(token.NoPos, e.pkg.Types, e.names.take("resetT"), nil)
			args[i] = types.NewTypeParam(obj, params.At(i).Constraint())
		}
		instance, err := types.Instantiate(nil, n, args, false)
		if err != nil {
			return err
		}
		receiverType = instance.(*types.Named)
	}

	method := resetMethodContext{
		receiverName: recv,
		receiverType: types.TypeString(receiverType, types.RelativeTo(e.pkg.Types)),
	}
	if e.traversal[n.Obj()] {
		method.visited = e.names.take("resetVisited")
		method.original = e.names.take("resetOriginal")
		method.seen = e.names.take("resetSeen")
	}
	statements := statementEmitter{
		marked:       e.marked,
		traversal:    e.traversal,
		names:        e.names,
		helpers:      &e.helpers,
		declarations: e.declarations,
		constraints:  e.constraints,
		visited:      method.visited,
	}
	var fields bytes.Buffer
	for f := range receiverType.Underlying().(*types.Struct).Fields() {
		if f.Name() == "_" {
			continue
		}
		if !f.Exported() && f.Pkg() != e.pkg.Types {
			return fmt.Errorf("%s.%s: недоступное поле пакета %s", name, f.Name(), f.Pkg().Path())
		}
		if err := e.constraints.checkObject(f); err != nil {
			return fmt.Errorf("%s.%s: %w", name, f.Name(), err)
		}
		stmt, err := statements.emit(recv+"."+f.Name(), f.Type(), make(map[*types.Pointer]bool))
		if err != nil {
			return fmt.Errorf("%s.%s: %w", name, f.Name(), err)
		}
		fmt.Fprintln(&fields, stmt)
	}
	e.emitResetMethod(name, method, fields.Bytes())
	if method.visited != "" {
		e.emitResetWithVisitedMethod(method, fields.Bytes())
	}
	var used []int
	for i := 0; i < receiverType.TypeArgs().Len(); i++ {
		if param, ok := receiverType.TypeArgs().At(i).(*types.TypeParam); ok && e.helpers.parameters[param] {
			used = append(used, i)
		}
	}
	e.dispatches[resetTypeName(n)] = used
	return nil
}

type resetMethodContext struct {
	receiverName string
	receiverType string
	visited      string
	original     string
	seen         string
}

func (e *fileEmitter) emitResetMethod(name string, method resetMethodContext, fields []byte) {
	fmt.Fprintf(&e.body, "// Reset сбрасывает %s к начальному состоянию.\n", name)
	fmt.Fprintf(&e.body, "func (%s *%s) %s() {\n", method.receiverName, method.receiverType, resetMethodName)
	fmt.Fprintf(&e.body, "if %s == nil {\nreturn\n}\n", method.receiverName)
	if method.visited != "" {
		fmt.Fprintf(&e.body, "%s.%s(map[interface{}]struct{}{}, %s)\n}\n\n",
			method.receiverName, resetWithVisitedMethodName, method.receiverName)
		return
	}
	e.body.Write(fields)
	e.body.WriteString("}\n\n")
}

func (e *fileEmitter) emitResetWithVisitedMethod(method resetMethodContext, fields []byte) {
	e.body.WriteString("// ResetWithVisited продолжает сброс с общей картой посещённых объектов.\n")
	e.body.WriteString("// original сохраняет вызов пользовательского Reset при встраивании типа.\n")
	e.body.WriteString("// Тип необязательного параметра отличает собственный протокол от унаследованного.\n")
	fmt.Fprintf(&e.body, "func (%s *%s) %s(%s map[interface{}]struct{}, %s interface{ %s() }, _ ...*%s) {\n",
		method.receiverName, method.receiverType, resetWithVisitedMethodName, method.visited,
		method.original, resetMethodName, method.receiverType,
	)
	fmt.Fprintf(&e.body, "if %s == nil {\nreturn\n}\n", method.original)
	fmt.Fprintf(&e.body, "if %s != %s {\n%s.%s()\nreturn\n}\n", method.original, method.receiverName, method.original, resetMethodName)
	fmt.Fprintf(&e.body, "if %s == nil {\nreturn\n}\n", method.receiverName)
	fmt.Fprintf(&e.body, "if %s == nil {\n%s = map[interface{}]struct{}{}\n}\n", method.visited, method.visited)
	fmt.Fprintf(&e.body, "if _, %s := %s[%s]; %s {\nreturn\n}\n", method.seen, method.visited, method.receiverName, method.seen)
	fmt.Fprintf(&e.body, "%s[%s] = struct{}{}\n\n", method.visited, method.receiverName)
	e.body.Write(fields)
	e.body.WriteString("}\n\n")
}

func (e *fileEmitter) source() ([]byte, error) {
	if e.helpers.zero != "" {
		fmt.Fprintf(&e.body, "func %s[T interface{}](value *T) {\nvar zero T\n*value = zero\n}\n", e.helpers.zero)
	}

	var src bytes.Buffer
	fmt.Fprintf(&src, "%s\n\npackage %s\n\n", genHeader, e.pkg.Name)
	if e.helpers.value != "" {
		reflectName := e.names.take("resetReflect")
		fmt.Fprintf(&src, "import %s \"reflect\"\n\n", reflectName)
		e.emitResetValueHelper(reflectName)
	}
	src.Write(e.body.Bytes())

	opt := &imports.Options{FormatOnly: true, Comments: true, TabIndent: true, TabWidth: 8}

	return imports.Process("", src.Bytes(), opt)
}

const resetValueHelperTemplate = `func {{helper}}(value interface{ {{reset}}() }, visited map[interface{}]struct{}) {
if value == nil {
return
}
v := {{reflect}}.ValueOf(value)
switch v.Kind() {
case {{reflect}}.Chan, {{reflect}}.Func, {{reflect}}.Map, {{reflect}}.Pointer, {{reflect}}.Slice:
if v.IsNil() {
return
}
}
if resetter, ok := value.(interface {
{{resetWithVisited}}(map[interface{}]struct{}, interface{ {{reset}}() })
}); ok {
resetter.{{resetWithVisited}}(visited, value)
return
}
if method := v.MethodByName("{{resetWithVisited}}"); method.IsValid() {
signature := method.Type()
if signature.NumIn() == 3 && signature.NumOut() == 0 && signature.IsVariadic() &&
signature.In(0) == {{reflect}}.TypeFor[map[interface{}]struct{}]() &&
signature.In(1) == {{reflect}}.TypeFor[interface{ {{reset}}() }]() &&
signature.In(2) == {{reflect}}.SliceOf(v.Type()) {
method.Call([]{{reflect}}.Value{{{reflect}}.ValueOf(visited), v})
return
}
}
value.{{reset}}()
}
`

var resetValueHelperBase = strings.NewReplacer(
	"{{reset}}", resetMethodName,
	"{{resetWithVisited}}", resetWithVisitedMethodName,
).Replace(resetValueHelperTemplate)

func (e *fileEmitter) emitResetValueHelper(reflectName string) {
	e.body.WriteString(strings.NewReplacer(
		"{{helper}}", e.helpers.value,
		"{{reflect}}", reflectName,
	).Replace(resetValueHelperBase))
}

type nameAllocator map[string]bool

func (names nameAllocator) take(base string) string {
	name := base
	for suffix := 2; names[name]; suffix++ {
		name = fmt.Sprintf("%s%d", base, suffix)
	}
	names[name] = true
	return name
}

type resetHelpers struct {
	zero       string
	value      string
	parameters map[*types.TypeParam]bool
}
