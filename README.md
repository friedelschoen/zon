# zon - _"zon is not json"_

**zon** is a declarative, extensible build tool written in Go. It interprets `.zon` files—JSON-inspired configuration files with support for evaluation, includes, function definitions, and build recipes. It’s designed to be minimal yet powerful, with strong inspiration from tools like Nix and JSON-e, and suitable for building packages, managing dotfiles, or orchestrating complex workflows.

---

## ✨ Features

- Declarative and expressive syntax: inspired by JSON, with extensions for expressions, interpolation, and functions.
- Build caching via content-addressable storage.
- Reproducible output paths, unless explicitly marked `impure`.
- Parallel and serial evaluation modes.
- Native support for:
  - `let/in` bindings,
  - `include` statements,
  - `output { ... }` build expressions,
  - string interpolation and escape sequences.
- Output as symlinks, JSON, or raw directory paths.
- Can optionally generate a DOT dependency graph.

---

## 📦 Example

A simple `.zon` file for building suckless tools:

```zon
let
  util = include ./util.zon
in output {
  with util.merge,
  "name": "apps",
  "prefix": ".local",
  "paths": [
    output {
      with util.unpack,
      "archive": output {
        with util.fetch,
        "url": "https://dl.suckless.org/tools/dmenu-5.2.tar.gz",
        "checksum": "3829528c849db6f903676fe7e6a48f3735505b6d"
      },
      "name": "dmenu",
      "output": ''
        make PREFIX=$out install
      ''
    }
  ]
}
```

You can run this with:

```sh
zon apps.zon
```

This fetches, unpacks, and builds `dmenu`, outputting to a deterministic directory in `cache/store`.

---

## 🔧 Command Line Usage

```sh
zon [options] <file.zon> [key=value ...]
```

### Options

| Flag             | Description                                           |
| ---------------- | ----------------------------------------------------- |
| `-f`, `--force`  | Force rebuilding of all outputs                       |
| `-d`, `--dry`    | Dry-run: do not execute anything                      |
| `-s`, `--serial` | Run builders sequentially instead of in parallel      |
| `-o`, `--output` | Symlink output to given name (default: `result`)      |
| `--no-result`    | Disable symlink creation                              |
| `--json`         | Print result as JSON                                  |
| `-g`, `--clean`  | Clean orphaned outputs in the store                   |
| `--graph`        | Write DOT graph to specified file                     |
| `--cache`        | Cache directory (default: `cache/store`)              |
| `--log`          | Log directory (default: `cache/log`)                  |
| `--interpreter`  | Interpreter to use for inline scripts (default: `sh`) |

---

## 🛠️ Built-in Expressions

- `output { ... }`: defines a build step. Attributes include:
  - `"builder"` or `"output"` (script),
  - `"args"` (array of string args),
  - `"source"` (working directory),
  - `"impure"` (disables caching),
  - custom env vars.
- `include path`: includes and evaluates another `.zon` file.
- `let ... in ...`: scoped variable definitions.
- `with expr, ...`: merges attribute sets (maps).
- Strings support `"\(expression)"` interpolation.

---

## 🧠 Design

- Everything is an expression: there are no statements.
- Outputs are hashed based on the full input expression unless marked `impure`.
- Evaluation is lazy but deterministic.
- Errors include file and position information for debugging.

---

## 📜 License

Beautiful zlib-license