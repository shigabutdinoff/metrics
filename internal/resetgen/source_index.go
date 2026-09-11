package resetgen

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

type sourceIndex struct {
	fset         *token.FileSet
	sources      map[string]*constraintSource
	packages     map[string]*types.Package
	packageFiles map[string]map[string]bool
	instances    map[string]map[string]types.Type
	rootDirs     map[string]bool
}

type constraintSource struct {
	name    string
	types   map[string]*ast.TypeSpec
	imports []*ast.ImportSpec
	methods []*ast.FuncDecl
}

func newSourceIndex(fset *token.FileSet, pkgs, metadata []*packages.Package) *sourceIndex {
	sources := &sourceIndex{
		fset:         fset,
		sources:      make(map[string]*constraintSource),
		packages:     make(map[string]*types.Package),
		packageFiles: make(map[string]map[string]bool),
		instances:    make(map[string]map[string]types.Type),
		rootDirs:     make(map[string]bool),
	}
	for _, pkg := range pkgs {
		if pkg.Dir != "" {
			sources.rootDirs[filepath.Clean(pkg.Dir)] = true
		}
	}
	for pkg := range packages.Postorder(metadata) {
		sources.addPackageFiles(pkg)
	}
	for pkg := range packages.Postorder(pkgs) {
		sources.addPackageFiles(pkg)
		if pkg.Types != nil && pkg.Syntax != nil {
			sources.addPackageTypes(pkg)
		}
	}

	return sources
}

func (s *sourceIndex) addPackageTypes(pkg *packages.Package) {
	s.packages[pkg.PkgPath] = pkg.Types
	instances := make(map[string]types.Type)
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			decl, ok := decl.(*ast.GenDecl)
			if !ok || decl.Tok != token.TYPE {
				continue
			}
			for _, spec := range decl.Specs {
				spec := spec.(*ast.TypeSpec)
				switch ast.Unparen(spec.Type).(type) {
				case *ast.IndexExpr, *ast.IndexListExpr:
					instances[spec.Name.Name] = pkg.TypesInfo.TypeOf(spec.Type)
				}
			}
		}
	}
	s.instances[pkg.PkgPath] = instances
}

func (s *sourceIndex) addPackageFiles(pkg *packages.Package) {
	files := s.packageFiles[pkg.PkgPath]
	if files == nil {
		files = make(map[string]bool)
		s.packageFiles[pkg.PkgPath] = files
	}
	for _, path := range slices.Concat(pkg.GoFiles, pkg.CompiledGoFiles, pkg.IgnoredFiles) {
		if filepath.IsAbs(path) {
			files[filepath.Clean(path)] = true
		}
	}
}

func (s *sourceIndex) path(obj types.Object) (string, error) {
	path := s.fset.PositionFor(obj.Pos(), false).Filename
	if suffix, ok := strings.CutPrefix(path, "$GOROOT/"); ok {
		return filepath.Join(build.Default.GOROOT, suffix), nil
	}
	if filepath.IsAbs(path) {
		return path, nil
	}

	var resolved string
	for file := range s.packageFiles[obj.Pkg().Path()] {
		if filepath.Base(file) != filepath.Base(path) {
			continue
		}
		if resolved != "" {
			return "", fmt.Errorf("неоднозначный файл объявления зависимости %s в пакете %s", path, obj.Pkg().Path())
		}
		resolved = file
	}
	if resolved == "" {
		return "", fmt.Errorf("не найден файл объявления зависимости %s в пакете %s", path, obj.Pkg().Path())
	}

	return resolved, nil
}

func (s *sourceIndex) loadPackage(pkg *types.Package) (*types.Package, error) {
	if loaded := s.packages[pkg.Path()]; loaded != nil {
		return loaded, nil
	}
	loaded, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo,
		Fset: s.fset,
	}, pkg.Path())
	if err != nil {
		return nil, err
	}
	if len(loaded) != 1 || loaded[0].Types == nil {
		return nil, fmt.Errorf("не удалось загрузить исходный пакет %s", pkg.Path())
	}
	for _, err := range loaded[0].Errors {
		if err.Kind != packages.TypeError && err.Kind != packages.ListError {
			return nil, err
		}
	}
	s.addPackageTypes(loaded[0])
	s.addPackageFiles(loaded[0])

	return loaded[0].Types, nil
}

func (s *sourceIndex) source(path string) (*constraintSource, error) {
	if source := s.sources[path]; source != nil {
		return source, nil
	}
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	source := &constraintSource{
		name:    file.Name.Name,
		types:   make(map[string]*ast.TypeSpec),
		imports: file.Imports,
	}
	for _, decl := range file.Decls {
		if method, ok := decl.(*ast.FuncDecl); ok && method.Recv != nil && len(method.Recv.List) == 1 &&
			(method.Name.Name == resetMethodName || method.Name.Name == resetWithVisitedMethodName) {
			source.methods = append(source.methods, method)
		}
		decl, ok := decl.(*ast.GenDecl)
		if !ok || decl.Tok != token.TYPE {
			continue
		}
		for _, spec := range decl.Specs {
			spec := spec.(*ast.TypeSpec)
			source.types[spec.Name.Name] = spec
		}
	}
	s.sources[path] = source

	return source, nil
}

func (s *constraintSource) dependency(spec *ast.TypeSpec, pkg *types.Package) (*types.TypeName, error) {
	return s.resolveType(ast.Unparen(spec.Type), spec, pkg)
}

func (s *constraintSource) resolveType(expr ast.Expr, spec *ast.TypeSpec, pkg *types.Package) (*types.TypeName, error) {
	switch expr := expr.(type) {
	case *ast.Ident:
		if spec.TypeParams != nil {
			for _, field := range spec.TypeParams.List {
				for _, name := range field.Names {
					if name.Name == expr.Name {
						return nil, nil
					}
				}
			}
		}
		if obj, ok := pkg.Scope().Lookup(expr.Name).(*types.TypeName); ok {
			return obj, nil
		}
		for _, spec := range s.imports {
			if spec.Name == nil || spec.Name.Name != "." {
				continue
			}
			if imported := importedPackage(pkg, spec); imported != nil {
				if obj, ok := imported.Scope().Lookup(expr.Name).(*types.TypeName); ok && obj.Exported() {
					return obj, nil
				}
			}
		}
		if obj, ok := types.Universe.Lookup(expr.Name).(*types.TypeName); ok {
			return obj, nil
		}
		return nil, fmt.Errorf("не найден тип зависимости %s", expr.Name)
	case *ast.SelectorExpr:
		qualifier, ok := expr.X.(*ast.Ident)
		if !ok {
			return nil, fmt.Errorf("не найден пакет зависимости %s", expr.Sel.Name)
		}
		for _, spec := range s.imports {
			imported := importedPackage(pkg, spec)
			if imported == nil {
				continue
			}
			name := imported.Name()
			if spec.Name != nil {
				name = spec.Name.Name
			}
			if name == qualifier.Name {
				if obj, ok := imported.Scope().Lookup(expr.Sel.Name).(*types.TypeName); ok {
					return obj, nil
				}
			}
		}
		return nil, fmt.Errorf("не найден тип зависимости %s.%s", qualifier.Name, expr.Sel.Name)
	}

	return nil, nil
}

func importedPackage(pkg *types.Package, spec *ast.ImportSpec) *types.Package {
	path, err := strconv.Unquote(spec.Path.Value)
	if err != nil {
		return nil
	}
	for _, imported := range pkg.Imports() {
		if imported.Path() == path {
			return imported
		}
	}

	return nil
}
