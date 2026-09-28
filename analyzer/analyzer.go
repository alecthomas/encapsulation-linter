// Package analyzer checks access to the private fields of encapsulated structs.
package analyzer

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Config controls exemptions for private-field access and construction.
type Config struct {
	AllowReads                 string
	AllowWrites                string
	AllowFactory               string
	AllowGeneratedConstruction bool
}

// Analyzer enforces construction and private-field access through a type's API.
var Analyzer = NewAnalyzer(Config{})

// NewAnalyzer returns an independent analyzer with configurable access rules.
func NewAnalyzer(config Config) *analysis.Analyzer {
	var reads, writes, factories string
	var generatedConstruction bool
	a := &analysis.Analyzer{
		Name:      "encapsulation",
		Doc:       "check access to encapsulated struct fields and construction",
		FactTypes: []analysis.Fact{new(encapsulatedFact)},
	}
	a.Flags.StringVar(&reads, "allow-reads", config.AllowReads, `comma-separated module-relative writer:target pairs allowed to read private fields; targets may be interfaces; use "all" for either side`)
	a.Flags.StringVar(&writes, "allow-writes", config.AllowWrites, `comma-separated module-relative writer:target pairs allowed to write private fields; targets may be interfaces; use "all" for either side`)
	a.Flags.StringVar(&factories, "allow-factory", config.AllowFactory, `comma-separated module-relative factory:type pairs whose methods may return newly constructed values; targets may be interfaces; use "all" for either side`)
	a.Flags.BoolVar(&generatedConstruction, "allow-generated-construction", config.AllowGeneratedConstruction, "allow construction of structs declared in generated files from anywhere")
	a.Run = func(pass *analysis.Pass) (any, error) {
		allowedReads, err := parseAllowlist(reads)
		if err != nil {
			return nil, fmt.Errorf("allow-reads: %w", err)
		}
		allowedWrites, err := parseAllowlist(writes)
		if err != nil {
			return nil, fmt.Errorf("allow-writes: %w", err)
		}
		allowedFactories, err := parseAllowlist(factories)
		if err != nil {
			return nil, fmt.Errorf("allow-factory: %w", err)
		}
		return run(pass, allowedReads, allowedWrites, allowedFactories, generatedConstruction)
	}
	return a
}

type accessRule struct {
	writer string
	target string
}

type allowlist []accessRule

func parseAllowlist(raw string) (allowlist, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var list allowlist
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		writer, target, ok := strings.Cut(entry, ":")
		writer, target = strings.TrimSpace(writer), strings.TrimSpace(target)
		if !ok || !validAccessName(writer) || !validAccessName(target) {
			return nil, fmt.Errorf("invalid writer:target pair %q", entry)
		}
		list = append(list, accessRule{writer: writer, target: target})
	}
	return list, nil
}

func validAccessName(name string) bool {
	if name == "all" {
		return true
	}
	if dot := strings.LastIndexByte(name, '.'); dot >= 0 {
		name = name[dot+1:]
	}
	return token.IsIdentifier(name)
}

func contextWriter(ctx context) types.Object {
	var writer types.Object
	// A method's writer is its receiver type, not its individual method name.
	if ctx.method != nil {
		writer = ctx.method
	} else if ctx.fn != nil {
		writer = ctx.fn
	}
	return writer
}

func (c *checker) allows(list allowlist, writer types.Object, owner *types.TypeName, interfaces bool) bool {
	for _, rule := range list {
		if !matchesAccessName(rule.writer, writer, c.modulePath) {
			continue
		}
		if matchesAccessName(rule.target, owner, c.modulePath) || (interfaces && c.matchesInterfaceTarget(rule.target, owner)) {
			return true
		}
	}
	return false
}

func matchesAccessName(name string, obj types.Object, modulePath string) bool {
	if name == "all" {
		return true
	}
	if obj == nil || obj.Pkg() == nil {
		return false
	}
	path := obj.Pkg().Path()
	if name == path+"."+obj.Name() {
		return true
	}
	if path == modulePath {
		return name == obj.Name()
	}
	if strings.HasPrefix(path, modulePath+"/") {
		return name == strings.TrimPrefix(path, modulePath+"/")+"."+obj.Name()
	}
	return false
}

// A type fact carries construction policy to importing packages. Go's type
// checker already prevents those packages from naming private fields.
type encapsulatedFact struct {
	DirectConstructor bool
	// Generated records the declaring file rather than the construction
	// policy, so each analyzing package applies its own configuration.
	Generated bool
	// Module is empty when the declaring package's module is unknown.
	Module string
}

func (*encapsulatedFact) AFact() {}

type checker struct {
	pass             *analysis.Pass
	module           string
	modulePath       string
	facts            map[*types.TypeName]encapsulatedFact
	standard         map[string]bool
	allowedReads     allowlist
	allowedWrites    allowlist
	allowedFactories allowlist
	factoryNodes     map[ast.Node]bool
	factoryOwners    map[*types.TypeName]bool
	// Generated files are excluded from checking but still declare types.
	generatedFiles             map[*token.File]bool
	allowGeneratedConstruction bool
}

type context struct {
	method *types.TypeName
	fn     *types.Func
	target *types.Var
	option *types.TypeName
}

func run(pass *analysis.Pass, reads, writes, factories allowlist, generatedConstruction bool) (any, error) {
	c := newChecker(pass, reads, writes, factories, generatedConstruction)
	if c.isStandardPackage(pass.Pkg) {
		return nil, nil
	}
	files := c.checkedFiles()
	c.collectFactoryConstructions(files)
	c.collectTypes()
	for _, file := range files {
		c.checkFile(file)
	}
	return nil, nil
}

func (c *checker) checkedFiles() []*ast.File {
	files := make([]*ast.File, 0, len(c.pass.Files))
	for _, file := range c.pass.Files {
		name := c.pass.Fset.PositionFor(file.Pos(), false).Filename
		if ast.IsGenerated(file) {
			c.generatedFiles[c.pass.Fset.File(file.Pos())] = true
			continue
		}
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		files = append(files, file)
	}
	return files
}

func newChecker(pass *analysis.Pass, reads, writes, factories allowlist, generatedConstruction bool) *checker {
	module := enclosingModule(pass)
	return &checker{
		pass:                       pass,
		module:                     module,
		modulePath:                 modulePath(pass, module),
		facts:                      make(map[*types.TypeName]encapsulatedFact),
		standard:                   make(map[string]bool),
		allowedReads:               reads,
		allowedWrites:              writes,
		allowedFactories:           factories,
		factoryNodes:               make(map[ast.Node]bool),
		factoryOwners:              make(map[*types.TypeName]bool),
		generatedFiles:             make(map[*token.File]bool),
		allowGeneratedConstruction: generatedConstruction,
	}
}

func (c *checker) isStandardPackage(pkg *types.Package) bool {
	if pkg == nil {
		return false
	}
	path := pkg.Path()
	if standard, ok := c.standard[path]; ok {
		return standard
	}
	// Check GOROOT instead of inferring standard-library ownership from the
	// import path, which would also exclude packages in dotless modules.
	built, err := build.Import(path, "", build.FindOnly)
	standard := err == nil && built.Goroot
	c.standard[path] = standard
	return standard
}

func (c *checker) collectTypes() {
	for _, name := range c.pass.Pkg.Scope().Names() {
		obj, ok := c.pass.Pkg.Scope().Lookup(name).(*types.TypeName)
		if !ok {
			continue
		}
		named, ok := obj.Type().(*types.Named)
		if !ok {
			continue
		}
		fields, ok := named.Underlying().(*types.Struct)
		if !ok {
			continue
		}
		hasPrivate := false
		for i := range fields.NumFields() {
			field := fields.Field(i)
			if !field.Exported() && field.Name() != "_" {
				hasPrivate = true
			}
		}
		if !hasPrivate || (!obj.Exported() && named.NumMethods() == 0) {
			continue
		}
		fact := encapsulatedFact{
			DirectConstructor: c.hasLocalConstructor(obj),
			Generated:         c.generatedFiles[c.pass.Fset.File(obj.Pos())],
			Module:            c.module,
		}
		c.facts[obj] = fact
		if obj.Exported() {
			c.pass.ExportObjectFact(obj, &fact)
		}
	}
}

func (c *checker) hasLocalConstructor(owner *types.TypeName) bool {
	// Factory sites are collected first and the package scope is complete, so
	// source order cannot change whether a type has a direct constructor.
	if c.factoryOwners[owner] {
		return true
	}
	for _, name := range c.pass.Pkg.Scope().Names() {
		fn, ok := c.pass.Pkg.Scope().Lookup(name).(*types.Func)
		if ok && c.isConstructor(fn, owner) {
			return true
		}
	}
	return false
}

func (c *checker) metadata(obj *types.TypeName) (encapsulatedFact, bool) {
	if obj == nil || c.isStandardPackage(obj.Pkg()) {
		return encapsulatedFact{}, false
	}
	if obj.Pkg() == c.pass.Pkg {
		fact, ok := c.facts[obj]
		return fact, ok
	}
	var fact encapsulatedFact
	if !c.pass.ImportObjectFact(obj, &fact) {
		return encapsulatedFact{}, false
	}
	// Like the standard library, other modules are outside the codebase
	// being linted, so their types are constructed through their public API.
	if fact.Module != "" && c.module != "" && fact.Module != c.module {
		return encapsulatedFact{}, false
	}
	return fact, true
}

func (c *checker) isEncapsulated(obj *types.TypeName) bool {
	_, ok := c.metadata(obj)
	return ok
}

func namedType(t types.Type) *types.Named {
	if t == nil {
		return nil
	}
	t = types.Unalias(t)
	if pointer, ok := t.(*types.Pointer); ok {
		t = types.Unalias(pointer.Elem())
	}
	named, _ := t.(*types.Named)
	if named == nil {
		return nil
	}
	return named.Origin()
}

func typeName(t types.Type) *types.TypeName {
	if named := namedType(t); named != nil {
		return named.Obj()
	}
	return nil
}

func (c *checker) checkFile(file *ast.File) {
	options := make(map[*ast.FuncLit]context)
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if ok {
			c.collectOptions(fn, options)
		}
	}
	stack := []context{{}}
	// Inspect enters parents before children. Keep the ancestor stack in step
	// with it so each construction can be tied to its exact field initializer.
	ancestors := []ast.Node{}
	ast.Inspect(file, func(node ast.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			ancestors = ancestors[:len(ancestors)-1]
			return true
		}
		current := stack[len(stack)-1]
		switch n := node.(type) {
		case *ast.FuncDecl:
			current = context{}
			if fn, ok := c.pass.TypesInfo.Defs[n.Name].(*types.Func); ok {
				current.fn = fn
				if sig, ok := fn.Type().(*types.Signature); ok && sig.Recv() != nil {
					current.method = typeName(sig.Recv().Type())
				}
			}
		case *ast.FuncLit:
			if option, ok := options[n]; ok {
				current.target = option.target
				current.option = option.option
			} else {
				current.target = nil
				current.option = nil
			}
		case *ast.SelectorExpr:
			c.checkSelector(n, current, ancestors)
		case *ast.CompositeLit:
			c.checkConstruction(n, c.pass.TypesInfo.TypeOf(n), current, ancestors)
		case *ast.CallExpr:
			c.checkNew(n, current, ancestors)
		}
		stack = append(stack, current)
		ancestors = append(ancestors, node)
		return true
	})
}

func (c *checker) checkSelector(sel *ast.SelectorExpr, ctx context, ancestors []ast.Node) {
	selection := c.pass.TypesInfo.Selections[sel]
	if selection == nil || selection.Kind() != types.FieldVal {
		return
	}
	field, ok := selection.Obj().(*types.Var)
	if !ok || field.Exported() {
		return
	}
	owner := fieldOwner(selection)
	fact, encapsulated := c.metadata(owner)
	if !encapsulated {
		return
	}
	if ctx.method == owner || c.isConstructor(ctx.fn, owner) {
		return
	}
	// A type without its own constructor can be owned by the struct that
	// directly embeds it, but only through that struct's field selection.
	if !fact.DirectConstructor && c.isEmbeddedFieldAccess(sel, selection, ctx.method, owner) {
		return
	}
	if ctx.option == owner && c.isTarget(sel.X, ctx.target) {
		return
	}
	if c.isWrite(sel, ancestors) {
		if c.allows(c.allowedWrites, contextWriter(ctx), owner, true) {
			return
		}
	} else if c.allows(c.allowedReads, contextWriter(ctx), owner, true) {
		return
	}
	c.pass.Reportf(sel.Sel.Pos(), "private field %s.%s.%s may only be accessed by its methods, constructor, a direct functional option, or an eligible embedding type's methods", owner.Pkg().Path(), owner.Name(), field.Name())
}

func (c *checker) isEmbeddedFieldAccess(sel *ast.SelectorExpr, selection *types.Selection, method, owner *types.TypeName) bool {
	if method == nil {
		return false
	}
	if typeName(selection.Recv()) == method {
		return len(selection.Index()) == 2 && c.isEmbeddedField(method, selection.Index()[0], owner)
	}
	// An explicitly selected anonymous field, such as p.Checkpoint.private,
	// still belongs to the embedding method's receiver.
	embedded, ok := sel.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	parent := c.pass.TypesInfo.Selections[embedded]
	return parent != nil && parent.Kind() == types.FieldVal && typeName(parent.Recv()) == method &&
		len(parent.Index()) == 1 && c.isEmbeddedField(method, parent.Index()[0], owner)
}

func (c *checker) isEmbeddedField(method *types.TypeName, index int, owner *types.TypeName) bool {
	fields, ok := method.Type().Underlying().(*types.Struct)
	if !ok || index >= fields.NumFields() {
		return false
	}
	field := fields.Field(index)
	return field.Anonymous() && typeName(field.Type()) == owner
}

func (c *checker) isWrite(sel *ast.SelectorExpr, ancestors []ast.Node) bool {
	for _, ancestor := range ancestors {
		switch node := ancestor.(type) {
		case *ast.AssignStmt:
			for _, lhs := range node.Lhs {
				if writeTargetContains(lhs, sel) {
					return true
				}
			}
		case *ast.IncDecStmt:
			if writeTargetContains(node.X, sel) {
				return true
			}
		case *ast.RangeStmt:
			if node.Tok == token.ASSIGN && (writeTargetContains(node.Key, sel) || writeTargetContains(node.Value, sel)) {
				return true
			}
		case *ast.UnaryExpr:
			if node.Op == token.AND && writeTargetContains(node.X, sel) {
				return true
			}
		case *ast.CallExpr:
			if len(node.Args) > 0 && c.isMutatingBuiltin(node.Fun) && writeTargetContains(node.Args[0], sel) {
				return true
			}
		}
	}
	return false
}

func (c *checker) isMutatingBuiltin(expr ast.Expr) bool {
	id, ok := expr.(*ast.Ident)
	if !ok {
		return false
	}
	builtin, ok := c.pass.TypesInfo.Uses[id].(*types.Builtin)
	if !ok {
		return false
	}
	switch builtin.Name() {
	case "clear", "close", "copy", "delete":
		return true
	default:
		return false
	}
}

func writeTargetContains(expr ast.Expr, target *ast.SelectorExpr) bool {
	if expr == target {
		return true
	}
	switch expr := expr.(type) {
	case *ast.ParenExpr:
		return writeTargetContains(expr.X, target)
	case *ast.StarExpr:
		return writeTargetContains(expr.X, target)
	case *ast.SelectorExpr:
		return writeTargetContains(expr.X, target)
	case *ast.IndexExpr:
		return writeTargetContains(expr.X, target)
	case *ast.SliceExpr:
		return writeTargetContains(expr.X, target)
	default:
		return false
	}
}

func fieldOwner(selection *types.Selection) *types.TypeName {
	t := selection.Recv()
	index := selection.Index()
	for step, i := range index {
		t = types.Unalias(t)
		if pointer, ok := t.(*types.Pointer); ok {
			t = types.Unalias(pointer.Elem())
		}
		owner := typeName(t)
		fields, ok := t.Underlying().(*types.Struct)
		if !ok || i >= fields.NumFields() {
			return nil
		}
		if step == len(index)-1 {
			return owner
		}
		t = fields.Field(i).Type()
	}
	return nil
}

func (c *checker) isTarget(expr ast.Expr, target *types.Var) bool {
	if target == nil {
		return false
	}
	for {
		switch x := expr.(type) {
		case *ast.ParenExpr:
			expr = x.X
		case *ast.StarExpr:
			expr = x.X
		case *ast.Ident:
			return c.pass.TypesInfo.Uses[x] == target
		default:
			return false
		}
	}
}

func (c *checker) checkConstruction(node ast.Node, t types.Type, ctx context, ancestors []ast.Node) {
	owner := typeName(t)
	fact, encapsulated := c.metadata(owner)
	if !encapsulated || (fact.Generated && c.allowGeneratedConstruction) || c.isConstructor(ctx.fn, owner) || c.factoryNodes[node] {
		return
	}
	if !fact.DirectConstructor && c.inParentField(owner, ctx, ancestors, node) {
		return
	}
	c.pass.Reportf(node.Pos(), "encapsulated struct %s.%s may only be constructed in its constructor, returned from an allowed factory method, or used as a field in an eligible parent constructor", owner.Pkg().Path(), owner.Name())
}

func (c *checker) checkNew(call *ast.CallExpr, ctx context, ancestors []ast.Node) {
	t := c.newType(call)
	if t == nil {
		return
	}
	c.checkConstruction(call, t, ctx, ancestors)
}

func (c *checker) newType(call *ast.CallExpr) types.Type {
	id, ok := call.Fun.(*ast.Ident)
	if !ok || len(call.Args) != 1 {
		return nil
	}
	builtin, ok := c.pass.TypesInfo.Uses[id].(*types.Builtin)
	if !ok || builtin.Name() != "new" {
		return nil
	}
	return c.pass.TypesInfo.TypeOf(call.Args[0])
}

func (c *checker) inParentField(owner *types.TypeName, ctx context, ancestors []ast.Node, node ast.Node) bool {
	for i := len(ancestors) - 1; i >= 0; i-- {
		lit, ok := ancestors[i].(*ast.CompositeLit)
		if !ok || !c.isConstructor(ctx.fn, typeName(c.pass.TypesInfo.TypeOf(lit))) {
			continue
		}
		element := node
		if i+1 < len(ancestors) {
			element = ancestors[i+1]
		}
		field := c.literalField(lit, element)
		if field != nil && fieldContainsType(field.Type(), owner) {
			return true
		}
	}
	return false
}

func (c *checker) literalField(lit *ast.CompositeLit, element ast.Node) *types.Var {
	t := c.pass.TypesInfo.TypeOf(lit)
	if t == nil {
		return nil
	}
	fields, ok := types.Unalias(t).Underlying().(*types.Struct)
	if !ok {
		return nil
	}
	if keyed, ok := element.(*ast.KeyValueExpr); ok {
		key, ok := keyed.Key.(*ast.Ident)
		if !ok {
			return nil
		}
		field, _ := c.pass.TypesInfo.Uses[key].(*types.Var)
		return field
	}
	for i, expr := range lit.Elts {
		if expr == element && i < fields.NumFields() {
			return fields.Field(i)
		}
	}
	return nil
}

func fieldContainsType(t types.Type, owner *types.TypeName) bool {
	switch t := types.Unalias(t).(type) {
	case *types.Named:
		return t.Origin().Obj() == owner
	case *types.Pointer:
		return fieldContainsType(t.Elem(), owner)
	case *types.Slice:
		return fieldContainsType(t.Elem(), owner)
	case *types.Array:
		return fieldContainsType(t.Elem(), owner)
	case *types.Map:
		return fieldContainsType(t.Key(), owner) || fieldContainsType(t.Elem(), owner)
	default:
		return false
	}
}

func (c *checker) isConstructor(fn *types.Func, owner *types.TypeName) bool {
	if fn == nil || owner == nil || fn.Pkg() != c.pass.Pkg {
		return false
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() != nil {
		return false
	}
	for i := range sig.Results().Len() {
		if constructorResult(sig.Results().At(i).Type(), owner) {
			return true
		}
	}
	return false
}

func constructorResult(result types.Type, owner *types.TypeName) bool {
	if typeName(result) == owner {
		return true
	}
	named, ok := owner.Type().(*types.Named)
	if !ok {
		return false
	}
	iface, ok := types.Unalias(result).Underlying().(*types.Interface)
	return ok && iface.NumMethods() > 0 && (types.Implements(named, iface) || types.Implements(types.NewPointer(named), iface))
}

func (c *checker) collectOptions(fn *ast.FuncDecl, options map[*ast.FuncLit]context) {
	obj, ok := c.pass.TypesInfo.Defs[fn.Name].(*types.Func)
	if !ok || fn.Body == nil {
		return
	}
	sig, ok := obj.Type().(*types.Signature)
	if !ok {
		return
	}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if _, ok := node.(*ast.FuncLit); ok {
			return false
		}
		ret, ok := node.(*ast.ReturnStmt)
		if !ok || len(ret.Results) != sig.Results().Len() {
			return true
		}
		for i, expr := range ret.Results {
			option, lit := directOption(expr, sig.Results().At(i).Type(), c.pass.TypesInfo)
			if option == nil || lit == nil {
				continue
			}
			optionSig, ok := option.Underlying().(*types.Signature)
			if !ok || optionSig.Params().Len() != 1 {
				continue
			}
			owner := typeName(optionSig.Params().At(0).Type())
			if !c.isEncapsulated(owner) {
				continue
			}
			if len(lit.Type.Params.List) != 1 || len(lit.Type.Params.List[0].Names) != 1 {
				continue
			}
			target, ok := c.pass.TypesInfo.Defs[lit.Type.Params.List[0].Names[0]].(*types.Var)
			if ok {
				options[lit] = context{target: target, option: owner}
			}
		}
		return true
	})
}

func directOption(expr ast.Expr, result types.Type, info *types.Info) (*types.Named, *ast.FuncLit) {
	for {
		paren, ok := expr.(*ast.ParenExpr)
		if !ok {
			break
		}
		expr = paren.X
	}
	if lit, ok := expr.(*ast.FuncLit); ok {
		option, _ := types.Unalias(result).(*types.Named)
		return option, lit
	}
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return nil, nil
	}
	conversion, ok := info.Types[call.Fun]
	if !ok || !conversion.IsType() {
		return nil, nil
	}
	option, ok := types.Unalias(conversion.Type).(*types.Named)
	if !ok || !types.AssignableTo(option, result) {
		return nil, nil
	}
	arg := call.Args[0]
	for {
		paren, ok := arg.(*ast.ParenExpr)
		if !ok {
			break
		}
		arg = paren.X
	}
	lit, _ := arg.(*ast.FuncLit)
	return option, lit
}
