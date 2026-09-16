package resetgen

import (
	"fmt"
	"go/ast"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type methodIndex struct {
	sources  *sourceIndex
	packages map[constraintMethodKey]*constraintMethods
}

type constraintMethod struct {
	name  string
	paths []string
	reset bool
}

type constraintMethods struct {
	byType map[string][]constraintMethod
	err    error
}

type constraintMethodKey struct {
	packagePath  string
	includeTests bool
}

func newMethodIndex(sources *sourceIndex) *methodIndex {
	return &methodIndex{
		sources:  sources,
		packages: make(map[constraintMethodKey]*constraintMethods),
	}
}

func (m *methodIndex) typeMethods(obj *types.TypeName, includeTests bool) ([]constraintMethod, error) {
	if obj.Pkg() == nil {
		return nil, nil
	}
	pkg := obj.Pkg()
	key := constraintMethodKey{packagePath: pkg.Path(), includeTests: includeTests}
	index := m.packages[key]
	if index == nil {
		index = &constraintMethods{byType: make(map[string][]constraintMethod)}
		m.packages[key] = index
		index.err = m.indexMethods(pkg, index, includeTests)
	}
	return index.byType[obj.Name()], index.err
}

func (m *methodIndex) indexMethods(pkg *types.Package, index *constraintMethods, includeTests bool) error {
	type receiverAlias struct {
		target string
		path   string
	}
	type sourceMethod struct {
		receiver string
		method   constraintMethod
	}
	aliases := make(map[string][]receiverAlias)
	var methods []sourceMethod
	paths := make([]string, 0, len(m.sources.packageFiles[pkg.Path()]))
	for path := range m.sources.packageFiles[pkg.Path()] {
		if filepath.Ext(path) == ".go" && (includeTests || !strings.HasSuffix(path, "_test.go")) {
			paths = append(paths, path)
		}
	}
	slices.Sort(paths)
	for _, path := range paths {
		if filepath.Base(path) == genFile && m.sources.rootDirs[filepath.Dir(path)] {
			src, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if hasGenHeader(src) {
				continue
			}
		}
		source, err := m.sources.source(path)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if source.name != pkg.Name() {
			continue
		}
		for name, spec := range source.types {
			if spec.Assign.IsValid() {
				if target := receiverName(spec.Type); target != "" {
					aliases[name] = append(aliases[name], receiverAlias{target, path})
				}
			}
		}
		for _, decl := range source.methods {
			methods = append(methods, sourceMethod{
				receiver: receiverName(decl.Recv.List[0].Type),
				method: constraintMethod{
					name:  decl.Name.Name,
					paths: []string{path},
					reset: decl.Name.Name == resetMethodName && decl.Type.Params.NumFields() == 0 && decl.Type.Results.NumFields() == 0,
				},
			})
		}
	}
	var add func(string, constraintMethod, map[string]bool)
	add = func(name string, method constraintMethod, seen map[string]bool) {
		if name == "" || seen[name] {
			return
		}
		seen[name] = true
		defer delete(seen, name)
		index.byType[name] = append(index.byType[name], method)
		if targets := aliases[name]; len(targets) > 0 {
			for _, alias := range targets {
				aliased := method
				aliased.paths = append(slices.Clone(method.paths), alias.path)
				add(alias.target, aliased, seen)
			}
			return
		}
	}
	for _, method := range methods {
		add(method.receiver, method.method, make(map[string]bool))
	}
	return nil
}

func receiverName(expr ast.Expr) string {
	switch expr := ast.Unparen(expr).(type) {
	case *ast.Ident:
		return expr.Name
	case *ast.StarExpr:
		return receiverName(expr.X)
	case *ast.IndexExpr:
		return receiverName(expr.X)
	case *ast.IndexListExpr:
		return receiverName(expr.X)
	}
	return ""
}
