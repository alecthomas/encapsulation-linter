# Encapsulation linter

`encapsulation-linter` reports access to private fields of encapsulated structs and construction outside recognized constructors. A struct is encapsulated when it has a private field and is exported or has methods. Generated files, `_test.go` files, and standard-library types are excluded.

Run it standalone:

```sh
go run ./cmd/encapsulation-linter ./...
```

Use `-allow-reads` and `-allow-writes` with comma-separated `writer:target` pairs. A writer is a method receiver type or a package-level function. A target is a concrete type or an interface; an interface matches field-owning types implemented by their value or pointer type when that interface is declared in, or directly imported by, the field owner's package. For example, `visit:node` allows `visit` to access all private fields of types implementing `node`. Names are relative to the analyzed module root: `Config` names a root-package type, `lexer.Config` names one in the `lexer` subpackage, and `example.com/project/lexer.Config` is an exact full-path name. Use `all` on either side to match every writer or target; `all:all` exempts every private-field access. The flags are independent and do not exempt struct construction. Assignments, increments, address-taking, and mutating builtins such as `clear` and `delete` count as writes. An assignment that also reads the field needs both exemptions.

Use `-allow-factory` with comma-separated `factory:type` pairs to permit factory methods to construct encapsulated types. The factory is a method receiver type. The target may be a concrete type or an interface implemented by the constructed type's value or pointer type, using the same interface matching as read/write rules. Module-relative names, full paths, and `all` work on either side. A method must be in the constructed type's package and return that newly constructed value, directly or through a local variable that is not reassigned, as the concrete type or an implemented non-empty interface. Constructing a value only to store it elsewhere is not exempted. An allowed factory counts as a direct constructor, so the parent-constructor fallback does not apply to that type.

```sh
go run ./cmd/encapsulation-linter -allow-factory=lexer.StatefulDefinition:lexer.StatefulLexer -allow-reads=lexer.ActionPop:lexer.StatefulLexer,lexer.ActionPush:lexer.StatefulLexer -allow-writes=lexer.ActionPop:lexer.StatefulLexer,lexer.ActionPush:lexer.StatefulLexer ./...
```

For golangci-lint v2, build a custom binary with the module plugin:

```yaml
# .custom-gcl.yml
version: v2.14.0
plugins:
  - module: github.com/alecthomas/encapsulation-linter
    import: github.com/alecthomas/encapsulation-linter/golangci
    path: .
```

```yaml
# .golangci.yml
version: "2"
linters:
  default: none
  enable:
    - encapsulation
  settings:
    custom:
      encapsulation:
        type: module
        # Optional:
        # settings:
        #   allow-reads: "lexer.ActionPop:lexer.StatefulLexer,lexer.ActionPush:lexer.StatefulLexer"
        #   allow-writes: "lexer.ActionPop:lexer.StatefulLexer,lexer.ActionPush:lexer.StatefulLexer"
        #   allow-factory: "lexer.StatefulDefinition:lexer.StatefulLexer"
```

Inside this repository's Hermit environment, run `golangci-lint run`. The repository wrapper uses Bit to build and cache the custom binary containing the module plugin before invoking it. Use `bit fmt` to format source, `bit fmt-l` to check formatting, and `bit test` to run tests. Outside that environment, run `golangci-lint custom` and use the resulting `./custom-gcl run ./...`.

A recognized constructor is any package-level function that returns the concrete type, a pointer to it, or a non-empty interface implemented by either. If a type has no direct constructor, it may also be constructed inside a field initializer of another type's constructor when that field's type contains it. Methods of a struct that directly embeds such a type may access its private fields through the embedded field. A directly returned closure of a named functional-option type may access private fields through its target parameter. Methods may access their own type's private fields. Exported fields and private structs without methods are unrestricted.
