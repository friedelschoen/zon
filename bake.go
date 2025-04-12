package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"slices"
	"strings"

	flag "github.com/spf13/pflag"
)

func writeDot(filename string, edges []Edge) {
	dot, err := os.Create(filename)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	defer dot.Close()

	fmt.Fprintf(dot, "digraph {\n")
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
		cleanup    bool
	)

	flag.BoolVarP(&ev.Force, "force", "f", false, "force building all outputs")
	flag.BoolVarP(&ev.DryRun, "dry", "d", false, "do not build anything")
	flag.StringVarP(&ev.CacheDir, "cache", "c", "cache/store", "destination of outputs")
	flag.StringVarP(&ev.LogDir, "log", "l", "cache/log", "destination of logs of outputs")
	flag.StringVarP(&resultName, "output", "o", "result", "name of result-symlink")
	flag.BoolVar(&noResult, "no-result", false, "disables creation of result-symlink")
	flag.StringVar(&dotName, "graph", "", "destination of dependency graph")
	flag.BoolVarP(&ev.Serial, "serial", "s", false, "do not build output asynchronous")
	flag.StringVar(&ev.Interpreter, "interpreter", "sh", "default interpreter for @output")
	flag.BoolVar(&ev.NoEvalOutput, "no-eval-output", false, "skip evaluation of @output")
	flag.BoolVar(&jsonOutput, "json", false, "print result as JSON, implies --no-result")
	flag.BoolVarP(&cleanup, "clean", "g", false, "clean orphaned results, not used by this build")
	flag.Parse()

	if jsonOutput {
		noResult = true
	}

	if noResult {
		resultName = ""
	}

	if ev.DryRun && ev.Force {
		ev.Force = false
	}

	filename := ""
	scope := make(map[string]Value)
	for _, arg := range flag.Args() {
		if name, value, ok := strings.Cut(arg, "="); ok {
			scope[name] = StringValue{BaseExpr{filename: "<commandline>"}, value}
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

	ast, err := parseFile(PathExpr{BaseExpr{filename: "<commandline>"}, filename})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if !ev.DryRun {
		os.MkdirAll(ev.CacheDir, 0755)
		os.MkdirAll(ev.LogDir, 0755)
	}
	res, err := ast.resolve(scope, &ev)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if cleanup {
		cwd, _ := os.Getwd()
		entries, err := os.ReadDir(path.Join(cwd, ev.CacheDir))
		if err != nil {
			fmt.Println(err)
			entries = nil
		}
		for _, entry := range entries {
			if !slices.Contains(ev.Outputs, entry.Name()) {
				fmt.Printf("clean %s\n", entry.Name())
				os.RemoveAll(path.Join(cwd, ev.CacheDir, entry.Name()))
			}
		}
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
