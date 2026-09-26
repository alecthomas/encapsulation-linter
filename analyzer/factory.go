package analyzer

import (
	"go/ast"
	"go/token"
	"go/types"
)

// Collect factory sites before type facts so a later method declaration still
// suppresses the parent-constructor fallback for its returned type.
func (c *checker) collectFactoryConstructions(files []*ast.File) {
	if len(c.allowedFactories) == 0 {
		return
	}
	for _, file := range files {
		for _, declaration := range file.Decls {
			method, ok := declaration.(*ast.FuncDecl)
			if !ok || method.Body == nil {
				continue
			}
			fn, ok := c.pass.TypesInfo.Defs[method.Name].(*types.Func)
			if !ok {
				continue
			}
			sig, ok := fn.Type().(*types.Signature)
			if !ok || sig.Recv() == nil {
				continue
			}
			factory := typeName(sig.Recv().Type())
			if factory == nil {
				continue
			}
			ast.Inspect(method.Body, func(node ast.Node) bool {
				if _, ok := node.(*ast.FuncLit); ok {
					return false
				}
				var owner *types.TypeName
				switch node := node.(type) {
				case *ast.CompositeLit:
					owner = typeName(c.pass.TypesInfo.TypeOf(node))
				case *ast.CallExpr:
					owner = typeName(c.newType(node))
				}
				if owner == nil || owner.Pkg() != c.pass.Pkg || !c.allowedFactories.allowsPair(factory, owner) {
					return true
				}
				if c.factoryReturnsConstruction(method, sig, owner, node) {
					c.factoryNodes[node] = true
					c.factoryOwners[owner] = true
				}
				return true
			})
		}
	}
}

func (c *checker) factoryReturnsConstruction(method *ast.FuncDecl, sig *types.Signature, owner *types.TypeName, construction ast.Node) bool {
	returned := make(map[*types.Var]token.Pos)
	direct := false
	ast.Inspect(method.Body, func(node ast.Node) bool {
		if _, ok := node.(*ast.FuncLit); ok {
			return false
		}
		ret, ok := node.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		if len(ret.Results) == 0 {
			for i := range sig.Results().Len() {
				result := sig.Results().At(i)
				if result.Name() != "" && constructorResult(result.Type(), owner) {
					returned[result] = ret.Pos()
				}
			}
			return true
		}
		if len(ret.Results) != sig.Results().Len() {
			return true
		}
		for i, expr := range ret.Results {
			if !constructorResult(sig.Results().At(i).Type(), owner) {
				continue
			}
			value := c.unwrapFactoryValue(expr)
			if value == construction {
				direct = true
			}
			if id, ok := value.(*ast.Ident); ok {
				if variable := c.variable(id); variable != nil && ret.Pos() > returned[variable] {
					returned[variable] = ret.Pos()
				}
			}
		}
		return true
	})
	if direct {
		return true
	}
	for variable, retPos := range returned {
		if retPos > construction.Pos() && c.singleFactoryAssignment(method.Body, variable, construction) {
			return true
		}
	}
	return false
}

func (c *checker) unwrapFactoryValue(expr ast.Expr) ast.Expr {
	for {
		switch value := expr.(type) {
		case *ast.ParenExpr:
			expr = value.X
		case *ast.UnaryExpr:
			if value.Op != token.AND && value.Op != token.MUL {
				return expr
			}
			expr = value.X
		case *ast.CallExpr:
			if len(value.Args) != 1 || !c.pass.TypesInfo.Types[value.Fun].IsType() {
				return expr
			}
			expr = value.Args[0]
		default:
			return expr
		}
	}
}

func (c *checker) variable(id *ast.Ident) *types.Var {
	if variable, ok := c.pass.TypesInfo.Defs[id].(*types.Var); ok {
		return variable
	}
	variable, _ := c.pass.TypesInfo.Uses[id].(*types.Var)
	return variable
}

func (c *checker) singleFactoryAssignment(body *ast.BlockStmt, variable *types.Var, construction ast.Node) bool {
	writes := 0
	matched := false
	ast.Inspect(body, func(node ast.Node) bool {
		if _, ok := node.(*ast.FuncLit); ok {
			return false
		}
		switch node := node.(type) {
		case *ast.AssignStmt:
			for i, lhs := range node.Lhs {
				id, ok := lhs.(*ast.Ident)
				if !ok || c.variable(id) != variable {
					continue
				}
				writes++
				if len(node.Lhs) == len(node.Rhs) && c.unwrapFactoryValue(node.Rhs[i]) == construction {
					matched = true
				}
			}
		case *ast.ValueSpec:
			for i, id := range node.Names {
				if c.variable(id) != variable || len(node.Values) == 0 {
					continue
				}
				writes++
				if len(node.Names) == len(node.Values) && c.unwrapFactoryValue(node.Values[i]) == construction {
					matched = true
				}
			}
		case *ast.RangeStmt:
			if node.Tok == token.ASSIGN {
				for _, expr := range []ast.Expr{node.Key, node.Value} {
					if id, ok := expr.(*ast.Ident); ok && c.variable(id) == variable {
						writes++
					}
				}
			}
		case *ast.IncDecStmt:
			if id, ok := node.X.(*ast.Ident); ok && c.variable(id) == variable {
				writes++
			}
		}
		return true
	})
	return writes == 1 && matched
}
