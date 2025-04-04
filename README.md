# bake

`bake` is a minimal, declarative build tool inspired by the Nix language and functional build systems. It evaluates JSON expressions with `@define`, `@include`, `@expand`, and `@output` constructs, performs dependency resolution, and executes builds in isolated output directories using content-based hashing.

## Features

- Declarative JSON-based build descriptions
- Functional, reproducible builds
- Content-addressed caching
- Parallel or serial evaluation
- Dry-run and graph generation support
- Per-output logging
- Minimal dependencies, suitable for bootstrapping

## Installation

Requires Go 1.21 or newer.

```sh
go install github.com/friedelschoen/bake@latest
```

## Usage

```sh
bake [flags] file.json [key=value ...]
```

### Example

```json
{
  "@define": {
    "version": "1.0.0"
  },
  "build": {
    "@name": "build",
    "@output": "echo Building version {{version}} > $out/build.txt"
  }
}
```

Then run:

```sh
bake example.json
```

The output is stored in a content-hashed directory under `cache/`, and a symlink named `result` points to it.

## Flags

```
  -cache dir        directory for output cache (default "cache")
  -dry              dry-run; do not build anything
  -force            force rebuild even if output exists
  -graph file.dot   write DOT-formatted dependency graph to file
  -interpreter sh   interpreter to use for @output scripts (default "sh")
  -json             print final result as JSON
  -log dir          directory for log files (default "cache/log")
  -no-eval-output   do not evaluate @output; treat it as data
  -no-result        do not create result symlink
  -result name      name for result symlink (default "result")
  -serial           evaluate nodes serially instead of in parallel
```

## Short Guide

A bake file is a JSON expression. It can be a map, list, string, number, or boolean. Special keys are interpreted:

- `@define`: defines variables for reuse
- `@include`: includes another JSON file
- `@expand`: merges in another value from the scope
- `@output`: a script to execute in a temporary directory with `$out` set
- `$key`: environment variable to pass to the script
- `@name`: optional name used for diagnostics and graph generation

Example with includes and environment:

```json
{
  "@include": "./common.json",
  "build": {
    "@name": "example",
    "@output": "cp $src $out/hello",
    "$src": "./hello.txt"
  }
}
```

## References

- [Nix: Functional package manager](https://nixos.org/)
- [dot(1): Graphviz DOT tool](https://graphviz.org/doc/info/lang.html)
- [Go](https://go.dev/)

## License

This software is distributed under the terms of the zlib license. See [LICENSE](./LICENSE) for details.
