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

	visited := e.names.take("resetVisited")
	original := e.names.take("resetOriginal")
	seen := e.names.take("resetSeen")
	receiver := types.TypeString(receiverType, types.RelativeTo(e.pkg.Types))
	method := resetMethodContext{
		receiverName: recv,
		receiverType: receiver,
		visited:      visited,
		original:     original,
		seen:         seen,
	}
	e.emitResetMethod(name, method)
	e.emitResetWithVisitedMethod(method)
	statements := statementEmitter{
		marked:       e.marked,
		names:        e.names,
		helpers:      &e.helpers,
		declarations: e.declarations,
		constraints:  e.constraints,
		visited:      visited,
	}
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
		fmt.Fprintln(&e.body, stmt)
	}
	e.body.WriteString("}\n\n")
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

func (e *fileEmitter) emitResetMethod(name string, method resetMethodContext) {
	fmt.Fprintf(&e.body, "// Reset сбрасывает %s к начальному состоянию.\n", name)
	fmt.Fprintf(&e.body, "func (%s *%s) %s() {\n", method.receiverName, method.receiverType, resetMethodName)
	fmt.Fprintf(&e.body, "if %s == nil {\nreturn\n}\n", method.receiverName)
	fmt.Fprintf(&e.body, "%s.%s(map[interface{}]struct{}{}, %s)\n", method.receiverName, resetWithVisitedMethodName, method.receiverName)
	e.body.WriteString("}\n\n")
}

func (e *fileEmitter) emitResetWithVisitedMethod(method resetMethodContext) {
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

func (e *fileEmitter) emitResetValueHelper(reflectName string) {
	fmt.Fprintf(&e.body, `func %[1]s(value interface{ %[3]s() }, visited map[interface{}]struct{}) {
if value == nil {
return
}
v := %[2]s.ValueOf(value)
switch v.Kind() {
case %[2]s.Chan, %[2]s.Func, %[2]s.Map, %[2]s.Pointer, %[2]s.Slice:
if v.IsNil() {
return
}
}
if resetter, ok := value.(interface {
%[4]s(map[interface{}]struct{}, interface{ %[3]s() })
}); ok {
resetter.%[4]s(visited, value)
return
}
if method := v.MethodByName("%[4]s"); method.IsValid() {
signature := method.Type()
if signature.NumIn() == 3 && signature.NumOut() == 0 && signature.IsVariadic() &&
signature.In(0) == %[2]s.TypeFor[map[interface{}]struct{}]() &&
signature.In(1) == %[2]s.TypeFor[interface{ %[3]s() }]() &&
signature.In(2) == %[2]s.SliceOf(v.Type()) {
method.Call([]%[2]s.Value{%[2]s.ValueOf(visited), v})
return
}
}
value.%[3]s()
}
`, e.helpers.value, reflectName, resetMethodName, resetWithVisitedMethodName)
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
