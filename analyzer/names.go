package analyzer

import (
	"go/types"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/tools/go/analysis"
)

// enclosingModule returns the module containing the analyzed package, or ""
// when neither the driver nor an enclosing go.mod identifies one.
func enclosingModule(pass *analysis.Pass) string {
	// Drivers know the module of vendored packages, which a go.mod search
	// cannot find.
	if pass.Module != nil && pass.Module.Path != "" {
		return pass.Module.Path
	}
	if pass.Pkg == nil || pass.Fset == nil {
		return ""
	}
	path := pass.Pkg.Path()
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
				return ""
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
		}
	}
	return ""
}

// modulePath returns the root for module-relative access names.
func modulePath(pass *analysis.Pass, module string) string {
	if module != "" || pass.Pkg == nil {
		return module
	}
	// Packages without a matching go.mod still accept names relative to
	// their own package, as in GOPATH-based analysistest fixtures.
	return pass.Pkg.Path()
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
