package main

import (
	"log"
	"slices"

	"github.com/gostaticanalysis/nilerr"
	"github.com/timakin/bodyclose/passes/bodyclose"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/multichecker"
	"golang.org/x/tools/go/analysis/suite/vet"

	"github.com/shigabutdinoff/metrics/cmd/staticlint/osexitcheck"
)

var analyzers = slices.Concat(vet.Suite, []*analysis.Analyzer{
	bodyclose.Analyzer,
	nilerr.Analyzer,

	osexitcheck.Analyzer,
})

func main() {
	all, err := allAnalyzers()
	if err != nil {
		log.Fatal(err)
	}

	multichecker.Main(all...)
}

func allAnalyzers() ([]*analysis.Analyzer, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, err
	}

	checks, err := staticcheckAnalyzers(cfg)
	if err != nil {
		return nil, err
	}

	return slices.Concat(analyzers, checks), nil
}
