// Package osexitcheck запрещает прямой вызов os.Exit в функции main пакета main.
//
// Прямой выход из main обрывает выполнение отложенных вызовов: буферы логгера
// не сбрасываются, соединения с базой и файлы аудита остаются незакрытыми.
// Вместо os.Exit функция main должна завершаться штатно.
package osexitcheck

import (
	"go/ast"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/types/typeutil"
)

// Analyzer ищет прямые вызовы os.Exit в функции main пакета main.
var Analyzer = &analysis.Analyzer{
	Name: "osexitcheck",
	Doc:  "запрещает прямой вызов os.Exit в функции main пакета main",
	Run:  run,
}

func run(pass *analysis.Pass) (any, error) {
	if pass.Pkg.Name() != "main" {
		return nil, nil
	}

	fn := mainFunc(pass.Files)
	if fn == nil {
		return nil, nil
	}

	ast.Inspect(fn.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if ok && isOsExit(pass, call) {
			pass.Reportf(call.Pos(), "прямой вызов os.Exit в функции main запрещён")
		}

		return true
	})

	return nil, nil
}

func mainFunc(files []*ast.File) *ast.FuncDecl {
	for _, file := range files {
		if ast.IsGenerated(file) {
			continue
		}

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && fn.Recv == nil && fn.Name.Name == "main" && fn.Body != nil {
				return fn
			}
		}
	}

	return nil
}

func isOsExit(pass *analysis.Pass, call *ast.CallExpr) bool {
	fn := typeutil.StaticCallee(pass.TypesInfo, call)

	return fn != nil && fn.FullName() == "os.Exit"
}
