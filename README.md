# bake - A minimal JSON-based declarative build system

**Bake** is a lightweight, reproducible, and parallel build executor. It builds artifacts based on declarative JSON files and content-addressed caching. Bake is designed to be simple and fast.

---

## Features

- Declarative builds using plain JSON
- Pure and impure build support (content hashing or randomization)
- Parallel or serial evaluation
- Content-addressed caching (store by input hash)
- DOT graph output of dependency relationships
- Cleanup of orphaned outputs
- Minimal runtime dependencies (just Go + POSIX shell)
- Variable resolution, includes, and extensions
- Inline string interpolation (`{{var}}`)
- Output selection via result symlink or JSON

---

## Quick Example

```json
{
  "@name": "hello",
  "@output": "echo hello > $out"
}
```

Build it:

```sh
bake hello.json
```

Result:

- Creates a cached directory like `cache/store/5e2c34...`
- Prints its path
- Optionally symlinks it as `result`

---

## Installation

Bake requires Go 1.21+.

```sh
go build -o bake .
```

Or install globally:

```sh
go install github.com/friedelschoen/bake@latest
```

---

## Usage

```sh
bake [options] file.json [var=value ...]
```

### Options

| Flag              | Description                                            |
| ----------------- | ------------------------------------------------------ |
| `-cache dir`      | Output directory (default: `cache/store`)              |
| `-log dir`        | Log directory (default: `cache/log`)                   |
| `-result name`    | Symlink name (default: `result`)                       |
| `-no-result`      | Disable result symlink creation                        |
| `-force`          | Force rebuild even if output exists                    |
| `-dry`            | Simulate without building                              |
| `-serial`         | Evaluate objects one-by-one                            |
| `-interpreter`    | Default shell (default: `sh`)                          |
| `-no-eval-output` | Skip evaluation of `@output`, return the object itself |
| `-graph path`     | Output a DOT graph of dependencies                     |
| `-clean`          | Delete orphaned cached outputs                         |
| `-json`           | Print final object as formatted JSON                   |

---

## Concepts

### Object Types

Bake supports:

- `ObjectMap` — standard JSON maps with special keys like `@output`, `@define`
- `ObjectArray` — lists of values, including `@multiline` support
- `ObjectString` — string values with `@var` and `{{interpolation}}`
- `ObjectNumber`, `ObjectBoolean` — literal values

---

## Special Keys

| Key            | Purpose                                                              |
| -------------- | -------------------------------------------------------------------- |
| `@name`        | Human-readable name used in graphs/logs                              |
| `@output`      | Shell snippet that builds into `$out`                                |
| `@interpreter` | Optional override for the default shell interpreter                  |
| `@impure`      | If true, disables hash-based caching (useful for timestamped builds) |
| `@define`      | Defines scoped variables for expansion                               |
| `@include`     | Path to another JSON file to merge in                                |
| `@expand`      | Extend from a named object in scope                                  |
| `@`            | If a map contains only `@`, unwrap its value as the object           |

---

## Includes and Extensions

Include files:

```json
{
  "@include": "./base.json",
  "@output": "echo hello > $out"
}
```

Extend a defined object:

```json
{
  "@define": {
    "base": {
      "@name": "base",
      "@output": "echo base > $out"
    }
  },
  "@expand": "@base"
}
```

---

## Multiline Arrays

For multi-line string values:

```json
["@multiline", "line 1", "line 2", "line 3"]
```

Expands into one `ObjectString` with newline separators.

---

## Environment Variables

Keys starting with `$` are exposed to the shell environment:

```json
{
  "$version": "1.2.3",
  "@output": "echo $version > $out/version"
}
```

---

## Output Caching

- Deterministic builds are hashed using FNV64
- Cached outputs are stored in `cache/store/<hash>/`
- Logs are stored as `cache/log/<hash>.log`
- Impure builds skip hashing

---

## Cleanup

```sh
bake -clean file.json
```

Removes all output directories in the cache that were **not part of this build run**. Safe for CI and long-term caching.

---

## DOT Graph Output

Generate a visual graph of build dependencies:

```sh
bake -graph graph.dot file.json
dot -Tpng graph.dot -o graph.png
```

---

## Example: Full Build Recipe

```json
{
  "@name": "build-myapp",
  "@define": {
    "src": "./src"
  },
  "@output": "cp -r {{src}} $out"
}
```

---

## License

Zlib Licensed
