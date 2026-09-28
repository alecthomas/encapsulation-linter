package golangci

import (
	"testing"

	"github.com/alecthomas/encapsulation-linter/analyzer"
	"github.com/golangci/plugin-module-register/register"
)

func TestRegistration(t *testing.T) {
	newPlugin, err := register.GetPlugin("encapsulation")
	if err != nil {
		t.Fatal(err)
	}
	plugin, err := newPlugin(nil)
	if err != nil {
		t.Fatal(err)
	}
	analyzers, err := plugin.BuildAnalyzers()
	if err != nil {
		t.Fatal(err)
	}
	if len(analyzers) != 1 || analyzers[0].Name != analyzer.Analyzer.Name {
		t.Fatalf("unexpected analyzers: %v", analyzers)
	}
	if analyzers[0].Flags.Lookup("allow-reads") == nil || analyzers[0].Flags.Lookup("allow-writes") == nil || analyzers[0].Flags.Lookup("allow-factory") == nil || analyzers[0].Flags.Lookup("allow-generated-construction") == nil {
		t.Fatal("access flags are missing")
	}
	if plugin.GetLoadMode() != register.LoadModeTypesInfo {
		t.Fatalf("unexpected load mode: %s", plugin.GetLoadMode())
	}
}

func TestSettings(t *testing.T) {
	newPlugin, err := register.GetPlugin("encapsulation")
	if err != nil {
		t.Fatal(err)
	}
	plugin, err := newPlugin(map[string]any{
		"allow-reads":   "all:example/access.Access",
		"allow-writes":  "all:all",
		"allow-factory": "all:lexer.StatefulLexer",
		"allow-generated-construction": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	analyzers, err := plugin.BuildAnalyzers()
	if err != nil {
		t.Fatal(err)
	}
	if got := analyzers[0].Flags.Lookup("allow-reads").Value.String(); got != "all:example/access.Access" {
		t.Fatalf("allow-reads = %q", got)
	}
	if got := analyzers[0].Flags.Lookup("allow-writes").Value.String(); got != "all:all" {
		t.Fatalf("allow-writes = %q", got)
	}
	if got := analyzers[0].Flags.Lookup("allow-factory").Value.String(); got != "all:lexer.StatefulLexer" {
		t.Fatalf("allow-factory = %q", got)
	}
	if got := analyzers[0].Flags.Lookup("allow-generated-construction").Value.String(); got != "true" {
		t.Fatalf("allow-generated-construction = %q", got)
	}
}
