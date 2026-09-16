package osexitcheck_test

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/shigabutdinoff/metrics/cmd/staticlint/osexitcheck"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), osexitcheck.Analyzer,
		"exitaliased",
		"exitdotimport",
		"exitinclosure",
		"exitinfunc",
		"exitinlib",
		"exitinmain",
		"exitparen",
		"exitviavar",
		"shadowedos",
	)
}
