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

func TestInterfaceTargets(t *testing.T) {
	a := analyzer.NewAnalyzer(analyzer.Config{AllowReads: "visit:node", AllowWrites: "visit:node"})
	analysistest.Run(t, analysistest.TestData(), a, "example/nodes")
}

func TestImportedInterfaceTarget(t *testing.T) {
	a := analyzer.NewAnalyzer(analyzer.Config{AllowReads: "inspect:example/iface.Node"})
	analysistest.Run(t, analysistest.TestData(), a, "example/iface", "example/impl")
}

func TestAllowFactory(t *testing.T) {
	a := analyzer.NewAnalyzer(analyzer.Config{AllowFactory: "example/factorynarrow.Factory:Target"})
	analysistest.Run(t, analysistest.TestData(), a, "example/factorynarrow", "example/factoryconsumer")
}

func TestAllowInterfaceFactory(t *testing.T) {
	a := analyzer.NewAnalyzer(analyzer.Config{AllowFactory: "Maker:node,Maker:example/iface.Node"})
	analysistest.Run(t, analysistest.TestData(), a, "example/iface", "example/factoryinterface")
}

func TestAllowAllFactories(t *testing.T) {
	a := analyzer.NewAnalyzer(analyzer.Config{AllowFactory: "all:all"})
	analysistest.Run(t, analysistest.TestData(), a, "example/factoryall")
}
