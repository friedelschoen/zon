package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

func writeDot(filename string, edges []Edge) {
	dot, err := os.Create(filename)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer dot.Close()

	fmt.Fprintf(dot, "digraph D {\n")
	for _, e := range edges {
		fmt.Fprintf(dot, "\"%s\" -> \"%s\";\n", e[0], e[1])
	}
	fmt.Fprintf(dot, "}")
}

func main() {
	var (
		ev         Evaluator
		resultName string
		noResult   bool
		dotName    string
		jsonOutput bool
	)

	flag.BoolVar(&ev.Force, "force", false, "force building all outputs")
	flag.BoolVar(&ev.DryRun, "dry", false, "do not build anything")
	flag.StringVar(&ev.CacheDir, "cache", "cache", "`destination` of outputs")
	flag.StringVar(&ev.LogDir, "log", "cache/log", "`destination` of logs of outputs")
	flag.StringVar(&resultName, "result", "result", "`name` of result-symlink")
	flag.BoolVar(&noResult, "no-result", false, "disables creation of result-symlink")
	flag.StringVar(&dotName, "graph", "", "`destination` of dependency graph (DOT formatted)")
	flag.BoolVar(&ev.Serial, "serial", false, "do not build output asynchronous")
	flag.StringVar(&ev.Interpreter, "interpreter", "sh", "default interpreter for @output")
	flag.BoolVar(&ev.NoEvalOutput, "no-eval-output", false, "print @output in result")
	flag.BoolVar(&jsonOutput, "json", false, "print result as JSON")
	flag.Parse()

	if noResult {
		resultName = ""
	}

	if ev.DryRun && ev.Force {
		ev.Force = false
	}

	filename := ""
	scope := make(map[string]Object)
	for _, arg := range flag.Args() {
		if name, value, ok := strings.Cut(arg, "="); ok {
			scope[name] = ObjectString{ObjectBase{filename: "<commandline>"}, value}
		} else if filename == "" {
			filename = arg
		} else {
			fmt.Fprintf(os.Stderr, "obsolete argument: `%s`\n", arg)
			os.Exit(1)
		}
	}

	if filename == "" {
		fmt.Fprintf(os.Stderr, "no file provided\n")
		os.Exit(1)
	}

	ast, err := parseFile(ObjectString{ObjectBase{filename: "<commandline>"}, filename}, nil)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	os.MkdirAll(ev.CacheDir, 0755)
	os.MkdirAll(ev.LogDir, 0755)

	res, err := ast.resolve(scope, &ev)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if dotName != "" {
		writeDot(dotName, ev.Edges)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "\t")
		enc.Encode(res.jsonObject())
	} else if err := res.symlink(resultName); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
