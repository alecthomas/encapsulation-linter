package analyzer

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestModulePath(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "analyzer.go", "package analyzer", parser.PackageClauseOnly)
	if err != nil {
		t.Fatal(err)
	}
	pass := &analysis.Pass{
		Fset:  fset,
		Pkg:   types.NewPackage("github.com/alecthomas/encapsulation-linter/analyzer", "analyzer"),
		Files: []*ast.File{file},
	}
	if got := modulePath(pass); got != "github.com/alecthomas/encapsulation-linter" {
		t.Fatalf("module path = %q", got)
	}
	pass.Pkg = types.NewPackage("example/access", "access")
	if got := modulePath(pass); got != "example/access" {
		t.Fatalf("unrelated package path = %q", got)
	}
}

func TestModuleRelativeNames(t *testing.T) {
	const module = "example.com/project"
	root := types.NewPackage(module, "project")
	sub := types.NewPackage(module+"/lexer", "lexer")
	other := types.NewPackage("example.com/other/lexer", "lexer")
	cases := []struct {
		name string
		obj  types.Object
		want bool
	}{
		{"visit", types.NewTypeName(token.NoPos, root, "visit", nil), true},
		{"lexer.ActionPop", types.NewTypeName(token.NoPos, sub, "ActionPop", nil), true},
		{"ActionPop", types.NewTypeName(token.NoPos, sub, "ActionPop", nil), false},
		{"lexer.ActionPop", types.NewTypeName(token.NoPos, other, "ActionPop", nil), false},
		{"example.com/other/lexer.ActionPop", types.NewTypeName(token.NoPos, other, "ActionPop", nil), true},
		{"all", types.NewTypeName(token.NoPos, other, "ActionPop", nil), true},
	}
	for _, tc := range cases {
		if got := matchesAccessName(tc.name, tc.obj, module); got != tc.want {
			t.Errorf("matchesAccessName(%q, %s) = %t, want %t", tc.name, tc.obj, got, tc.want)
		}
	}
}
