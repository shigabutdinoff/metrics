package main

import (
	"slices"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestLoadConfig(t *testing.T) {
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() вернул ошибку: %v", err)
	}

	if len(cfg.Prefixes) == 0 {
		t.Error("prefixes пустой, ни одна проверка staticcheck.io не включится")
	}
}

func TestStaticcheckAnalyzers(t *testing.T) {
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() вернул ошибку: %v", err)
	}

	res, err := staticcheckAnalyzers(cfg)
	if err != nil {
		t.Fatalf("staticcheckAnalyzers() вернул ошибку: %v", err)
	}

	names := analyzerNames(res)

	if !slices.Contains(names, "SA1000") {
		t.Error("SA1000 не включён, хотя префикс SA есть в конфиге")
	}

	if slices.Contains(names, "QF1008") {
		t.Error("QF1008 включён, хотя перечислен в exclude")
	}
}

func TestStaticcheckAnalyzersConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     checksConfig
		wantErr bool
	}{
		{
			name:    "опечатка в exclude",
			cfg:     checksConfig{Prefixes: []string{"SA"}, Exclude: []string{"QF1O08"}},
			wantErr: true,
		},
		{
			name:    "опечатка в prefixes",
			cfg:     checksConfig{Prefixes: []string{"SA", "ZZ"}},
			wantErr: true,
		},
		{
			name:    "пустой prefixes",
			cfg:     checksConfig{},
			wantErr: true,
		},
		{
			name: "префикс совпал только с исключённой проверкой",
			cfg:  checksConfig{Prefixes: []string{"SA", "QF1008"}, Exclude: []string{"QF1008"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := staticcheckAnalyzers(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("staticcheckAnalyzers() вернул ошибку %v, ждали ошибку: %v, получено %d проверок", err, tt.wantErr, len(res))
			}
		})
	}
}

func TestAllAnalyzers(t *testing.T) {
	all, err := allAnalyzers()
	if err != nil {
		t.Fatalf("allAnalyzers() вернул ошибку: %v", err)
	}

	names := analyzerNames(all)

	for _, want := range []string{"printf", "bodyclose", "nilerr", "osexitcheck", "SA1000"} {
		if !slices.Contains(names, want) {
			t.Errorf("анализатор %q не попал в набор", want)
		}
	}
}

func analyzerNames(analyzers []*analysis.Analyzer) []string {
	names := make([]string, 0, len(analyzers))
	for _, a := range analyzers {
		names = append(names, a.Name)
	}

	return names
}
