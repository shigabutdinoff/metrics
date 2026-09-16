package resetgen

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"maps"
	"path/filepath"
	"slices"

	"golang.org/x/tools/go/packages"
)

type resetTypeID struct {
	path string
	name string
}

func resetTypeName(n *types.Named) resetTypeID {
	return resetTypeID{n.Obj().Pkg().Path(), n.Obj().Name()}
}

type genericResetChecker struct {
	sources    *sourceIndex
	overlay    map[string][]byte
	dispatches map[resetTypeID][]int
	legacy     map[resetTypeID]bool
	owned      map[string]bool
	checked    map[types.Type]bool
	protocol   *types.Interface
}

func checkGenericResets(pattern string, result *generation, roots, metadata []*packages.Package, dispatches map[resetTypeID][]int) error {
	overlay := maps.Clone(result.files)
	for _, pkg := range roots {
		path := filepath.Join(pkg.Dir, genFile)
		if slices.Contains(result.obsolete, path) {
			overlay[path] = []byte(genHeader + "\n\npackage " + pkg.Name + "\n")
		}
	}
	fset := token.NewFileSet()
	pkgs, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports,
		Fset:    fset,
		Tests:   true,
		Overlay: overlay,
	}, pattern)
	if err != nil {
		return err
	}
	if err := checkPackageErrors(pkgs, packages.TypeError, packages.ListError); err != nil {
		return err
	}
	c := &genericResetChecker{
		sources:    newSourceIndex(fset, pkgs, metadata),
		overlay:    overlay,
		dispatches: dispatches,
		legacy:     make(map[resetTypeID]bool),
		owned:      make(map[string]bool),
		checked:    make(map[types.Type]bool),
		protocol:   newTraversalInterface(nil),
	}
	for _, pkg := range pkgs {
		if pkg.IllTyped || pkg.TypesInfo == nil {
			continue
		}
		instances := make([]*ast.Ident, 0, len(pkg.TypesInfo.Instances))
		for id := range pkg.TypesInfo.Instances {
			instances = append(instances, id)
		}
		slices.SortFunc(instances, func(a, b *ast.Ident) int { return int(a.Pos() - b.Pos()) })
		for _, id := range instances {
			if err := c.checkType(pkg.TypesInfo.Instances[id].Type); err != nil {
				return fmt.Errorf("%s: %w", fset.PositionFor(id.Pos(), false), err)
			}
		}
		for _, file := range pkg.Syntax {
			var checkErr error
			ast.Inspect(file, func(node ast.Node) bool {
				if checkErr != nil {
					return false
				}
				if expr, ok := node.(ast.Expr); ok {
					if err := c.checkType(pkg.TypesInfo.TypeOf(expr)); err != nil {
						checkErr = fmt.Errorf("%s: %w", fset.PositionFor(expr.Pos(), false), err)
					}
				}
				return checkErr == nil
			})
			if checkErr != nil {
				return checkErr
			}
		}
	}
	return nil
}

func (c *genericResetChecker) checkType(t types.Type) error {
	return walkTypeGraph(t, c.checked, c.checkGenericInstantiation)
}

func (c *genericResetChecker) checkGenericInstantiation(t types.Type) error {
	n, ok := t.(*types.Named)
	if !ok || n.TypeArgs().Len() == 0 {
		return nil
	}
	used, err := c.resetParameters(n)
	if err != nil {
		return err
	}
	legacy := c.legacy[resetTypeName(n)]
	for _, index := range used {
		if index >= n.TypeArgs().Len() {
			continue
		}
		argument := n.TypeArgs().At(index)
		err := checkLegacyEmbedding(argument)
		if err == nil {
			err = c.checkGeneratedArgument(argument, legacy)
		}
		if err == nil && legacy {
			err = c.checkLegacyArgument(argument)
		}
		if err != nil {
			return fmt.Errorf("%s: параметр %s: %w", types.TypeString(n, nil), n.TypeParams().At(index).Obj().Name(), err)
		}
	}
	return nil
}

func (c *genericResetChecker) resetParameters(n *types.Named) ([]int, error) {
	if obj := n.Obj(); obj.Pkg() == nil || obj.Parent() != obj.Pkg().Scope() {
		return nil, nil
	}
	id := resetTypeName(n)
	if used, ok := c.dispatches[id]; ok {
		return used, nil
	}
	c.dispatches[id] = nil
	origin := n.Origin()
	pointer := types.NewPointer(n)
	legacy := types.Implements(pointer, c.protocol)
	if !legacy && !types.Implements(pointer, newTraversalInterface(pointer)) {
		return nil, nil
	}
	owned, err := c.generatedProtocol(pointer)
	if err != nil || !owned {
		return nil, err
	}
	c.legacy[id] = legacy
	structure, ok := origin.Underlying().(*types.Struct)
	if !ok {
		return nil, nil
	}
	parameters := make(map[*types.TypeParam]bool)
	for field := range structure.Fields() {
		if field.Name() == "_" {
			continue
		}
		seen := make(map[types.Type]bool)
		for t := field.Type(); !seen[t]; {
			seen[t] = true
			if resetMethod(t) != nil {
				if param, ok := types.Unalias(t).(*types.TypeParam); ok {
					parameters[param] = true
				}
				break
			}
			pointer, ok := t.Underlying().(*types.Pointer)
			if !ok {
				break
			}
			t = pointer.Elem()
		}
	}
	var used []int
	for i := 0; i < origin.TypeParams().Len(); i++ {
		if parameters[origin.TypeParams().At(i)] {
			used = append(used, i)
		}
	}
	c.dispatches[id] = used
	return used, nil
}

func (c *genericResetChecker) generatedProtocol(t types.Type) (bool, error) {
	methods := types.NewMethodSet(t)
	for _, name := range resetProtocolMethodNames {
		selection := methods.Lookup(nil, name)
		if selection == nil || len(selection.Index()) != 1 {
			return false, nil
		}
		owned, err := c.generatedMethod(selection.Obj().(*types.Func))
		if err != nil || !owned {
			return false, err
		}
	}
	return true, nil
}
