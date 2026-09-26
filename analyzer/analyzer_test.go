package analyzer_test

import (
	"testing"

	"github.com/alecthomas/encapsulation-linter/analyzer"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	analysistest.Run(t, analysistest.TestData(), analyzer.Analyzer, "example/base", "example/consumer")
}

func TestAllowReadsWrites(t *testing.T) {
	a := analyzer.NewAnalyzer(analyzer.Config{})
	if err := a.Flags.Set("allow-reads", "all:example/access.Access,Worker:ReadOnly,permittedHelper:ReadOnly,Worker:Combined"); err != nil {
		t.Fatal(err)
	}
	if err := a.Flags.Set("allow-writes", "all:Access,example/access.Worker:example/access.WriteOnly,Worker:Combined"); err != nil {
		t.Fatal(err)
	}
	analysistest.Run(t, analysistest.TestData(), a, "example/access")
}

func TestAllowAll(t *testing.T) {
	a := analyzer.NewAnalyzer(analyzer.Config{AllowReads: "all:all", AllowWrites: "all:all"})
	analysistest.Run(t, analysistest.TestData(), a, "example/allaccess")
}

func TestAllowFactory(t *testing.T) {
	a := analyzer.NewAnalyzer(analyzer.Config{AllowFactory: "example/factorynarrow.Factory:Target"})
	analysistest.Run(t, analysistest.TestData(), a, "example/factorynarrow", "example/factoryconsumer")
}

func TestAllowAllFactories(t *testing.T) {
	a := analyzer.NewAnalyzer(analyzer.Config{AllowFactory: "all:all"})
	analysistest.Run(t, analysistest.TestData(), a, "example/factoryall")
}
