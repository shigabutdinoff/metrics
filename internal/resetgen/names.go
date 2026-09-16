package resetgen

import (
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
	importNames := make(map[string][]string)
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
				if _, loaded := importNames[importPath]; !loaded {
					importNames[importPath], err = loadImportNames(pkg.Dir, importPath)
					if err != nil {
						return nil, fmt.Errorf("%s: %w", path, err)
					}
				}
				for _, name := range importNames[importPath] {
					names[name] = true
				}
			}
		}
	}
	return declarations, nil
}

func loadImportNames(dir, importPath string) ([]string, error) {
	pkgs, err := packages.Load(&packages.Config{
		Mode: packages.NeedName | packages.NeedFiles,
		Dir:  dir,
	}, importPath)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", importPath, err)
	}
	var names []string
	fset := token.NewFileSet()
	for _, pkg := range pkgs {
		if pkg.Name != "" {
			names = append(names, pkg.Name)
		}
		for _, path := range slices.Concat(pkg.GoFiles, pkg.IgnoredFiles) {
			if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
				continue
			}
			file, err := parser.ParseFile(fset, path, nil, parser.PackageClauseOnly)
			if err != nil {
				return nil, err
			}
			names = append(names, file.Name.Name)
		}
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("не удалось определить имя импортируемого пакета %s", importPath)
	}
	slices.Sort(names)
	return slices.Compact(names), nil
}
