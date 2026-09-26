# Encapsulation linter

`encapsulation-linter` is a Go linter that prevents encapsulation violations
by reporting access to private fields outside methods, and construction outside
recognized constructors.

Run it standalone:

```sh
go run ./cmd/encapsulation-linter ./...
```

## Private-field access exceptions

Use `-allow-reads` and `-allow-writes` with comma-separated
`writer:target` pairs.

- A writer is a method receiver type or a package-level function.
- A target is a concrete type or an interface.
- An interface target matches types implemented by its value or pointer type.
  The interface must be declared in, or directly imported by, the target's
  package.

For example, `visit:node` allows the `visit` function to access private fields
of every type implementing `node`.

Names are relative to the analyzed module:

- `Config` identifies a type in the module's root package.
- `lexer.Config` identifies a type in the `lexer` subpackage.
- `example.com/project/lexer.Config` identifies a type by its full package
  path.

Use `all` on either side to match every writer or target. `all:all` exempts
every private-field access.

The two flags are independent and do not exempt struct construction.
Assignments, increments, address-taking, and mutating builtins such as `clear`
and `delete` count as writes. An assignment that also reads a field needs both
exceptions.

```sh
go run ./cmd/encapsulation-linter \
  -allow-reads=visit:node \
  -allow-writes=lexer.ActionPop:lexer.StatefulLexer,lexer.ActionPush:lexer.StatefulLexer \
  ./...
```

## Factory exceptions

Use `-allow-factory` with comma-separated `factory:type` pairs. This permits
methods on the factory type to construct the target type.

The target may be a concrete type or an interface. Interface targets use the
same matching rules as read and write exceptions. Module-relative names, full
package paths, and `all` work on either side.

To qualify, a factory method must:

- Be declared in the constructed type's package.
- Return the newly constructed value directly or through a local variable that
  is not reassigned.
- Return the concrete type or an implemented non-empty interface.

Constructing a value only to store it elsewhere does not qualify. An allowed
factory counts as a direct constructor, so the parent-constructor fallback does
not apply to that type.

```sh
go run ./cmd/encapsulation-linter \
  -allow-factory=lexer.StatefulDefinition:lexer.StatefulLexer \
  ./...
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
        #   allow-reads: >-
        #     lexer.ActionPop:lexer.StatefulLexer,
        #     lexer.ActionPush:lexer.StatefulLexer
        #   allow-writes: >-
        #     lexer.ActionPop:lexer.StatefulLexer,
        #     lexer.ActionPush:lexer.StatefulLexer
        #   allow-factory: "lexer.StatefulDefinition:lexer.StatefulLexer"
```

Inside this repository's Hermit environment, run `golangci-lint run`. The repository wrapper uses Bit to build and cache the custom binary containing the module plugin before invoking it. Use `bit fmt` to format source, `bit fmt-l` to check formatting, and `bit test` to run tests. Outside that environment, run `golangci-lint custom` and use the resulting `./custom-gcl run ./...`.

## Example violations

Assume `Config` is declared in the `config` package of the
`example.com/project` module:

```go
package config

type Config struct {
	secret string
}

func (c *Config) Secret() string {
	return c.secret // Allowed: methods may access their receiver's fields.
}
```

Reading the private field from an ordinary function is a violation:

```go
func reveal(config *Config) string {
	return config.secret
}
```

```text
private field example.com/project/config.Config.secret may only be accessed by its methods, constructor, a direct functional option, or an eligible embedding type's methods
```

Constructing the type outside a constructor is also a violation:

```go
func reset() {
	_ = &Config{}
}
```

```text
encapsulated struct example.com/project/config.Config may only be constructed in its constructor, returned from an allowed factory method, or used as a field in an eligible parent constructor
```
