package analyzer

import (
	"go/types"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/tools/go/analysis"
)

func modulePath(pass *analysis.Pass) string {
	if pass.Pkg == nil {
		return ""
	}
	path := pass.Pkg.Path()
	if pass.Fset == nil {
		return path
	}
	for _, file := range pass.Files {
		filename := pass.Fset.PositionFor(file.Pos(), false).Filename
		if absolute, err := filepath.Abs(filename); err == nil {
			filename = absolute
		}
		for dir := filepath.Dir(filename); dir != "."; dir = filepath.Dir(dir) {
			data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
			if err == nil {
				module := modfile.ModulePath(data)
				if module != "" && (path == module || strings.HasPrefix(path, module+"/")) {
					return module
				}
				// The nearest go.mod is the module boundary, even when this
				// package was loaded under a different import path.
				return path
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
		}
	}
	// Packages without a matching go.mod still accept names relative to
	// their own package, as in GOPATH-based analysistest fixtures.
	return path
}

func (c *checker) matchesInterfaceTarget(name string, owner *types.TypeName) bool {
	if owner == nil || owner.Pkg() == nil {
		return false
	}
	named, ok := owner.Type().(*types.Named)
	if !ok {
		return false
	}
	packages := append([]*types.Package{owner.Pkg()}, owner.Pkg().Imports()...)
	for _, pkg := range packages {
		for _, symbol := range pkg.Scope().Names() {
			candidate, ok := pkg.Scope().Lookup(symbol).(*types.TypeName)
			if !ok || !matchesAccessName(name, candidate, c.modulePath) {
				continue
			}
			iface, ok := types.Unalias(candidate.Type()).Underlying().(*types.Interface)
			if ok && (types.Implements(named, iface) || types.Implements(types.NewPointer(named), iface)) {
				return true
			}
		}
	}
	return false
}
