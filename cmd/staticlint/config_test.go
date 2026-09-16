package main

import (
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/go/analysis"
	"honnef.co/go/tools/staticcheck"
)

func TestLoadConfig(t *testing.T) {
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig() вернул ошибку: %v", err)
	}

	if len(cfg.Prefixes) == 0 {
		t.Error("prefixes пустой, ни одна проверка staticcheck.io не включится")
	}

	if !slices.Contains(cfg.Required, "SA") {
		t.Error("SA нет в required, класс SA перестал быть обязательным")
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

	for _, a := range staticcheck.Analyzers {
		if !slices.Contains(names, a.Analyzer.Name) {
			t.Errorf("SA-проверка %q не включена", a.Analyzer.Name)
		}
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
		wantMsg string
	}{
		{
			name:    "опечатка в exclude",
			cfg:     checksConfig{Prefixes: []string{"SA"}, Required: []string{"SA"}, Exclude: []string{"QF1O08"}},
			wantErr: true,
		},
		{
			name:    "опечатка в prefixes",
			cfg:     checksConfig{Prefixes: []string{"SA", "ZZ"}, Required: []string{"SA"}},
			wantErr: true,
		},
		{
			name:    "пустой prefixes",
			cfg:     checksConfig{Required: []string{"SA"}},
			wantErr: true,
		},
		{
			name: "класс SA покрыт более широким префиксом",
			cfg:  checksConfig{Prefixes: []string{"S"}, Required: []string{"S"}},
		},
		{
			name:    "класс SA сужен в required",
			cfg:     checksConfig{Prefixes: []string{"SA"}, Required: []string{"SA1"}},
			wantErr: true,
			wantMsg: "класс SA",
		},
		{
			name: "префикс совпал только с исключённой проверкой",
			cfg:  checksConfig{Prefixes: []string{"SA", "QF1008"}, Required: []string{"SA"}, Exclude: []string{"QF1008"}},
		},
		{
			name:    "обязательная проверка в exclude",
			cfg:     checksConfig{Prefixes: []string{"SA"}, Required: []string{"SA"}, Exclude: []string{"SA1019"}},
			wantErr: true,
		},
		{
			name:    "обязательный префикс не включён",
			cfg:     checksConfig{Prefixes: []string{"S1"}, Required: []string{"SA"}},
			wantErr: true,
			wantMsg: "не включена",
		},
		{
			name: "обязательный префикс включён более широким",
			cfg:  checksConfig{Prefixes: []string{"S"}, Required: []string{"SA"}},
		},
		{
			name:    "обязательный префикс сужен в prefixes",
			cfg:     checksConfig{Prefixes: []string{"SA1"}, Required: []string{"SA"}},
			wantErr: true,
			wantMsg: "не включена",
		},
		{
			name:    "неизвестный префикс в required",
			cfg:     checksConfig{Prefixes: []string{"SA"}, Required: []string{"SA", "ZZ"}},
			wantErr: true,
			wantMsg: `"ZZ"`,
		},
		{
			name:    "опечатка в exclude с обязательным префиксом",
			cfg:     checksConfig{Prefixes: []string{"SA"}, Required: []string{"SA"}, Exclude: []string{"SA10190"}},
			wantErr: true,
			wantMsg: "неизвестная проверка",
		},
		{
			name:    "пустой префикс в required",
			cfg:     checksConfig{Prefixes: []string{"SA"}, Required: []string{""}, Exclude: []string{"QF1008"}},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := staticcheckAnalyzers(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("staticcheckAnalyzers() вернул ошибку %v, ждали ошибку: %v, получено %d проверок", err, tt.wantErr, len(res))
			}
			if tt.wantMsg != "" && !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("ошибка %v не содержит %q", err, tt.wantMsg)
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
