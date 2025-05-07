package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/friedelschoen/zon/parser"
	"github.com/friedelschoen/zon/types"
	flag "github.com/spf13/pflag"
)

func PrintPathTree(p types.PathExpr, indent string) {
	fmt.Println(indent + "- " + path.Base(p.Name))

	for _, dep := range p.Depends {
		PrintPathTree(dep, indent+"  ")
	}
}

func main() {
	var (
		ev         types.Evaluator
		resultName string
		noResult   bool
		jsonOutput bool
		cleanup    bool
	)

	ev.ParseFile = parser.ParseFile

	flag.BoolVarP(&ev.Force, "force", "f", false, "force building all outputs")
	flag.BoolVarP(&ev.DryRun, "dry", "d", false, "do not build anything")
	flag.StringVarP(&ev.CacheDir, "cache", "c", "cache/store", "destination of outputs")
	flag.StringVarP(&ev.LogDir, "log", "l", "cache/log", "destination of logs of outputs")
	flag.StringVarP(&resultName, "output", "o", "result", "name of result-symlink")
	flag.BoolVar(&noResult, "no-result", false, "disables creation of result-symlink")
	flag.BoolVarP(&ev.Serial, "serial", "s", false, "do not build output asynchronous")
	flag.StringVar(&ev.Interpreter, "interpreter", "sh", "default interpreter for output")
	flag.BoolVar(&ev.NoEvalOutput, "no-eval-output", false, "skip evaluation of output")
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
	scope := make(types.Scope)
	for _, arg := range flag.Args() {
		if name, value, ok := strings.Cut(arg, "="); ok {
			scope[name] = types.Variable{Expr: types.StringConstant(value, "<commandline>"), Scope: make(types.Scope)}
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

	ast, err := parser.ParseFile(types.PathExpr{Position: types.Position{Filename: "<commandline>"}, Name: filename})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	if !ev.DryRun {
		os.MkdirAll(ev.CacheDir, 0755)
		os.MkdirAll(ev.LogDir, 0755)
	}
	res, deps, err := ast.Resolve(scope, &ev)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	_ = deps
	// for _, d := range deps {
	// 	PrintPathTree(d, "")
	// }

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

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "\t")
		enc.Encode(res.JSON())
	} else if err := res.Link(resultName); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
