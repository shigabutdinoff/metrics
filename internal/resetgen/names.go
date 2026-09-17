package resetgen

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

func reservePackageNames(pkg *packages.Package, metadata []*packages.Package, names nameAllocator) (map[string]bool, error) {
	declarations := make(map[string]bool)
	reserveDeclaration := func(name string) {
		declarations[name] = true
		names[name] = true
	}
	seen := make(map[string]bool)
	sources := make(map[string]string)
	var order []string
	fset := token.NewFileSet()
	for candidate := range packages.Postorder(metadata) {
		if candidate.Dir != pkg.Dir || candidate.Name != pkg.Name {
			continue
		}
		for _, path := range slices.Concat(candidate.GoFiles, candidate.IgnoredFiles) {
			if filepath.Ext(path) != ".go" || seen[path] {
				continue
			}
			seen[path] = true
			if filepath.Base(path) == genFile {
				src, err := os.ReadFile(path)
				if err != nil {
					return nil, err
				}
				if hasGenHeader(src) {
					continue
				}
			}
			file, err := parser.ParseFile(fset, path, nil, parser.AllErrors)
			if err != nil {
				return nil, err
			}
			if file.Name.Name != pkg.Name {
				continue
			}
			for _, decl := range file.Decls {
				switch decl := decl.(type) {
				case *ast.FuncDecl:
					if decl.Recv == nil {
						reserveDeclaration(decl.Name.Name)
					}
				case *ast.GenDecl:
					for _, spec := range decl.Specs {
						switch spec := spec.(type) {
						case *ast.TypeSpec:
							reserveDeclaration(spec.Name.Name)
						case *ast.ValueSpec:
							for _, name := range spec.Names {
								reserveDeclaration(name.Name)
							}
						}
					}
				}
			}
			for _, spec := range file.Imports {
				if spec.Name != nil {
					if name := spec.Name.Name; name != "_" && name != "." {
						names[name] = true
					}
					continue
				}
				importPath, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					return nil, fmt.Errorf("%s: %w", path, err)
				}
				if importPath == "C" {
					names["C"] = true
					continue
				}
				if imported := candidate.Imports[importPath]; imported != nil && imported.Name != "" &&
					!slices.Contains(candidate.IgnoredFiles, path) {
					names[imported.Name] = true
					continue
				}
				if _, ok := sources[importPath]; !ok {
					sources[importPath] = path
					order = append(order, importPath)
				}
			}
		}
	}
	if len(order) > 0 {
		loaded, err := loadImportNames(pkg.Dir, order, sources)
		if err != nil {
			return nil, err
		}
		for _, name := range loaded {
			names[name] = true
		}
	}
	return declarations, nil
}

func loadImportNames(dir string, importPaths []string, sources map[string]string) ([]string, error) {
	pkgs, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedFiles,
		Dir:  dir,
	}, importPaths...)
	if err != nil {
		return nil, fmt.Errorf("%s: не удалось загрузить импорты: %w", dir, err)
	}
	if len(pkgs) != len(importPaths) {
		return nil, fmt.Errorf("%s: не удалось загрузить импорты: %s",
			dir, strings.Join(importPaths, ", "))
	}
	var names []string
	fset := token.NewFileSet()
	for _, pkg := range pkgs {
		var found []string
		if pkg.Name != "" {
			found = append(found, pkg.Name)
		}
		for _, path := range slices.Concat(pkg.GoFiles, pkg.IgnoredFiles) {
			if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				continue
			}
			file, err := parser.ParseFile(fset, path, nil, parser.PackageClauseOnly)
			if err != nil {
				return nil, err
			}
			found = append(found, file.Name.Name)
		}
		if len(found) == 0 {
			return nil, importNameError(dir, pkg, sources)
		}
		names = append(names, found...)
	}
	slices.Sort(names)
	return slices.Compact(names), nil
}

func importNameError(dir string, pkg *packages.Package, sources map[string]string) error {
	id := pkg.PkgPath
	if id == "" {
		id = pkg.ID
	}
	cause := errors.New("не удалось определить имя импортируемого пакета")
	if len(pkg.Errors) > 0 {
		cause = pkg.Errors[0]
	}
	file := sources[id]
	if file == "" {
		file = dir
	}

	return fmt.Errorf("%s: %s: %w", file, id, cause)
}
