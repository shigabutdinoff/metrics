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
	Required []string `json:"required"`
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

	for _, prefix := range cfg.Required {
		if prefix == "" {
			return nil, fmt.Errorf("%s: пустой префикс в required", configName)
		}
	}

	if !slices.ContainsFunc(cfg.Required, func(prefix string) bool { return strings.HasPrefix("SA", prefix) }) {
		return nil, fmt.Errorf("%s: класс SA не объявлен обязательным", configName)
	}

	for _, name := range cfg.Exclude {
		if !slices.Contains(names, name) {
			return nil, fmt.Errorf("%s: неизвестная проверка %q в exclude", configName, name)
		}

		if slices.ContainsFunc(cfg.Required, func(prefix string) bool { return strings.HasPrefix(name, prefix) }) {
			return nil, fmt.Errorf("%s: проверка %q обязательна, её нельзя выключить", configName, name)
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

	for _, prefix := range cfg.Required {
		matched := false
		for _, name := range names {
			if !strings.HasPrefix(name, prefix) {
				continue
			}
			matched = true
			if !slices.ContainsFunc(res, func(enabled *analysis.Analyzer) bool { return enabled.Name == name }) {
				return nil, fmt.Errorf("%s: обязательная проверка %q не включена", configName, name)
			}
		}
		if !matched {
			return nil, fmt.Errorf("%s: обязательный префикс %q не совпал ни с одной проверкой", configName, prefix)
		}
	}

	return res, nil
}
