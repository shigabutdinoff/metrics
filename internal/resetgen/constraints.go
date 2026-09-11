package resetgen

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
)

type constraintChecker struct {
	sources *sourceIndex
	files   map[string]error
	types   map[*types.TypeName]error
	methods *methodIndex
}

func newConstraintChecker(fset *token.FileSet, pkgs, metadata []*packages.Package) *constraintChecker {
	sources := newSourceIndex(fset, pkgs, metadata)
	return &constraintChecker{
		sources: sources,
		files:   make(map[string]error),
		types:   make(map[*types.TypeName]error),
		methods: newMethodIndex(sources),
	}
}

func (c *constraintChecker) checkObject(obj types.Object) error {
	if obj == nil || obj.Pkg() == nil || !obj.Pos().IsValid() {
		return nil
	}

	path, err := c.sources.path(obj)
	if err != nil {
		return fmt.Errorf("%s: %w", obj.Name(), err)
	}
	if err := c.checkFile(path); err != nil {
		return fmt.Errorf("%s: %s: %w", obj.Name(), path, err)
	}

	return nil
}

func (c *constraintChecker) checkFile(path string) error {
	err, checked := c.files[path]
	if !checked {
		err = checkBuildConstraints(path)
		c.files[path] = err
	}
	return err
}

func (c *constraintChecker) checkConditionalReset(t types.Type) error {
	n, ok := types.Unalias(t).(*types.Named)
	if !ok || types.IsInterface(n) {
		return nil
	}
	methods, err := c.methods.typeMethods(n.Obj(), false)
	if err != nil {
		return err
	}
	for _, method := range methods {
		if !method.reset {
			continue
		}
		for _, path := range method.paths {
			if err := c.checkFile(path); err != nil {
				return fmt.Errorf("%s.Reset: %s: %w", n.Obj().Name(), path, err)
			}
		}
	}
	return nil
}

func (c *constraintChecker) checkGeneratedMethods(n *types.Named) error {
	methods, err := c.methods.typeMethods(n.Obj(), true)
	if err != nil {
		return err
	}
	if len(methods) > 0 {
		method := methods[0]
		return fmt.Errorf("%s.%s: %s: метод уже объявлен", n.Obj().Name(), method.name, method.paths[0])
	}
	return nil
}

func (c *constraintChecker) checkType(t types.Type) error {
	switch t := t.(type) {
	case *types.Named:
		return c.checkTypeName(t.Obj())
	case *types.Alias:
		if err := c.checkTypeName(t.Obj()); err != nil {
			return err
		}
		return c.checkType(t.Rhs())
	}

	return nil
}

func (c *constraintChecker) checkZeroConstraint(t types.Type) error {
	if err := c.checkType(t); err != nil {
		return err
	}
	if alias, ok := t.(*types.Alias); ok {
		return c.checkZeroConstraint(alias.Rhs())
	}
	if iface, ok := t.Underlying().(*types.Interface); ok {
		for i := 0; i < iface.NumEmbeddeds(); i++ {
			if err := c.checkZeroConstraint(iface.EmbeddedType(i)); err != nil {
				return err
			}
		}
	}

	return nil
}

func (c *constraintChecker) checkResetConstraint(t types.Type) error {
	if err := c.checkType(t); err != nil {
		return err
	}
	if alias, ok := t.(*types.Alias); ok {
		return c.checkResetConstraint(alias.Rhs())
	}
	iface, ok := t.Underlying().(*types.Interface)
	if !ok {
		return nil
	}
	for i := 0; i < iface.NumExplicitMethods(); i++ {
		method := iface.ExplicitMethod(i)
		if method.Name() == resetMethodName {
			return c.checkObject(method)
		}
	}
	var firstErr error
	for i := 0; i < iface.NumEmbeddeds(); i++ {
		embedded := iface.EmbeddedType(i)
		if !types.IsInterface(embedded) {
			continue
		}
		obj, _, _ := types.LookupFieldOrMethod(embedded, false, nil, resetMethodName)
		if _, ok := obj.(*types.Func); !ok {
			continue
		}
		if err := c.checkResetConstraint(embedded); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		return nil
	}

	return firstErr
}

func (c *constraintChecker) checkTypeName(obj *types.TypeName) (err error) {
	if obj == nil || obj.Pkg() == nil || !obj.Pos().IsValid() {
		return nil
	}
	if err, checked := c.types[obj]; checked {
		return err
	}
	c.types[obj] = nil
	defer func() { c.types[obj] = err }()

	if err := c.checkObject(obj); err != nil {
		return err
	}
	path, err := c.sources.path(obj)
	if err != nil {
		return fmt.Errorf("%s: %w", obj.Name(), err)
	}
	source, err := c.sources.source(path)
	if err != nil {
		return fmt.Errorf("%s: %s: %w", obj.Name(), path, err)
	}
	spec := source.types[obj.Name()]
	if spec == nil {
		return fmt.Errorf("%s: %s: не найдено объявление типа", obj.Name(), path)
	}
	switch ast.Unparen(spec.Type).(type) {
	case *ast.IndexExpr, *ast.IndexListExpr:
		if _, err := c.sources.loadPackage(obj.Pkg()); err != nil {
			return fmt.Errorf("%s: %s: %w", obj.Name(), path, err)
		}
		dependency := c.sources.instances[obj.Pkg().Path()][obj.Name()]
		if dependency == nil || dependency == types.Typ[types.Invalid] {
			return fmt.Errorf("%s: %s: не найден тип подставленной зависимости", obj.Name(), path)
		}
		return c.checkType(dependency)
	}
	pkg := obj.Pkg()
	if loaded := c.sources.packages[pkg.Path()]; loaded != nil {
		pkg = loaded
	}
	dependency, err := source.dependency(spec, pkg)
	if err != nil || dependency != nil && dependency.Pkg() == nil && c.sources.packages[pkg.Path()] == nil {
		pkg, loadErr := c.sources.loadPackage(obj.Pkg())
		if loadErr != nil {
			return fmt.Errorf("%s: %s: %w", obj.Name(), path, loadErr)
		}
		dependency, err = source.dependency(spec, pkg)
		if err != nil {
			return fmt.Errorf("%s: %s: %w", obj.Name(), path, err)
		}
	}
	if dependency != nil {
		return c.checkType(dependency.Type())
	}

	return nil
}
