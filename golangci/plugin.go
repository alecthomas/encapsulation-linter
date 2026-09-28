// Package golangci registers the analyzer as a golangci-lint module plugin.
package golangci

import (
	"fmt"

	"github.com/alecthomas/encapsulation-linter/analyzer"
	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"
)

func init() {
	register.Plugin("encapsulation", newPlugin)
}

type settings struct {
	AllowReads                 string `json:"allow-reads"`
	AllowWrites                string `json:"allow-writes"`
	AllowFactory               string `json:"allow-factory"`
	AllowGeneratedConstruction bool   `json:"allow-generated-construction"`
}

type plugin struct {
	settings settings
}

func newPlugin(conf any) (register.LinterPlugin, error) {
	if conf == nil {
		return plugin{}, nil
	}
	settings, err := register.DecodeSettings[settings](conf)
	if err != nil {
		return nil, fmt.Errorf("encapsulation settings: %w", err)
	}
	return plugin{settings: settings}, nil
}

func (p plugin) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{analyzer.NewAnalyzer(analyzer.Config{
		AllowReads:                 p.settings.AllowReads,
		AllowWrites:                p.settings.AllowWrites,
		AllowFactory:               p.settings.AllowFactory,
		AllowGeneratedConstruction: p.settings.AllowGeneratedConstruction,
	})}, nil
}

func (plugin) GetLoadMode() string {
	return register.LoadModeTypesInfo
}
