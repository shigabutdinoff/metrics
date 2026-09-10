package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
	"honnef.co/go/tools/quickfix"
	"honnef.co/go/tools/simple"
	"honnef.co/go/tools/staticcheck"
	"honnef.co/go/tools/stylecheck"
)

const configName = "config.json"

//go:embed config.json
var configData []byte

type checksConfig struct {
	Prefixes []string `json:"prefixes"`
	Exclude  []string `json:"exclude"`
}

func loadConfig() (checksConfig, error) {
	var cfg checksConfig

	dec := json.NewDecoder(bytes.NewReader(configData))
	dec.DisallowUnknownFields()

	if err := dec.Decode(&cfg); err != nil {
		return cfg, fmt.Errorf("не удалось разобрать %s: %w", configName, err)
	}

	return cfg, nil
}

func staticcheckAnalyzers(cfg checksConfig) ([]*analysis.Analyzer, error) {
	all := slices.Concat(staticcheck.Analyzers, simple.Analyzers, stylecheck.Analyzers, quickfix.Analyzers)

	names := make([]string, 0, len(all))
	for _, a := range all {
		names = append(names, a.Analyzer.Name)
	}

	for _, name := range cfg.Exclude {
		if !slices.Contains(names, name) {
			return nil, fmt.Errorf("%s: неизвестная проверка %q в exclude", configName, name)
		}
	}

	for _, prefix := range cfg.Prefixes {
		if !slices.ContainsFunc(names, func(name string) bool { return strings.HasPrefix(name, prefix) }) {
			return nil, fmt.Errorf("%s: префикс %q не совпал ни с одной проверкой", configName, prefix)
		}
	}

	res := make([]*analysis.Analyzer, 0, len(all))

	for _, a := range all {
		name := a.Analyzer.Name
		if slices.Contains(cfg.Exclude, name) {
			continue
		}

		if slices.ContainsFunc(cfg.Prefixes, func(prefix string) bool { return strings.HasPrefix(name, prefix) }) {
			res = append(res, a.Analyzer)
		}
	}

	if len(res) == 0 {
		return nil, fmt.Errorf("%s не включил ни одной проверки staticcheck.io", configName)
	}

	return res, nil
}
