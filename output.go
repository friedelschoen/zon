package main

import (
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"time"
)

type Edge [2]string

type Evaluator struct {
	Force        bool
	DryRun       bool
	CacheDir     string
	LogDir       string
	Serial       bool
	Interpreter  string
	NoEvalOutput bool

	Edges   []Edge
	Outputs []string
}

func (o OutputExpr) resolve(scope map[string]Value, ev *Evaluator) (Value, error) {
	attrsAny, err := o.attrs.resolve(scope, ev)
	if err != nil {
		return nil, err
	}
	result, ok := attrsAny.(MapValue)
	if !ok {
		return nil, fmt.Errorf("%s: unable to output non-map: %T", o.position(), attrsAny)
	}

	impure := false
	if impureAny, ok := result.values["@impure"]; ok {
		if impureVal, ok := impureAny.(BooleanExpr); ok {
			impure = impureVal.value
		}
	}

	var hashsum []byte
	hashlib := fnv.New128()
	if impure {
		hashsum = make([]byte, hashlib.Size())
		for i := range hashsum {
			hashsum[i] = byte(rand.Int())
		}
	} else {
		o.attrs.hashValue(hashlib)
		hashsum = hashlib.Sum(nil)
	}
	hashstr := hex.EncodeToString(hashsum)

	ev.Outputs = append(ev.Outputs, hashstr)

	cwd, _ := os.Getwd()
	outdir := path.Join(cwd, ev.CacheDir, hashstr)
	if _, err := os.Stat(outdir); (ev.DryRun || err == nil) && !ev.Force {
		return StringValue{content: outdir}, nil
	}

	start := time.Now()

	os.RemoveAll(outdir)
	success := false
	defer func() {
		if !success {
			os.RemoveAll(outdir)
		}
	}()

	var cmdline []string
	var token Value

	if installAny, ok := result.values["@output"]; ok {
		token = installAny
		install, ok := installAny.(StringValue)
		if !ok {
			return nil, fmt.Errorf("%s: @output must be a string", installAny.position())
		}

		interpreter := ev.Interpreter
		if interpreterAny, ok := result.values["@interpreter"]; ok {
			if str, ok := interpreterAny.(StringValue); ok {
				interpreter = str.content
			} else {
				return nil, fmt.Errorf("%s: @interpreter must be a string", interpreterAny.position())
			}
		}
		cmdline = []string{interpreter, "-e", "-c", install.content, "builder"}
	} else if builderAny, ok := result.values["@builder"]; ok {
		token = builderAny
		builder, ok := builderAny.(StringValue)
		if !ok {
			return nil, fmt.Errorf("%s: @builder must be a string", builderAny.position())
		}
		cmdline = []string{builder.content}
	} else {
		return nil, fmt.Errorf("%s: missing @output or @builder", o.position())
	}

	if argsAny, ok := result.values["@args"]; ok {
		args, ok := argsAny.(ArrayValue)
		if !ok {
			return nil, fmt.Errorf("%s: @args must be an array", argsAny.position())
		}
		for _, elem := range args.values[1:] {
			arg, ok := elem.(StringValue)
			if !ok {
				return nil, fmt.Errorf("%s: non-string in @args: %T", elem.position(), elem)
			}
			cmdline = append(cmdline, string(arg.content))
		}
	}

	var builddir string
	var deletebuilddir bool

	if sourcedirAny, ok := result.values["@source"]; ok {
		sourcedir, ok := sourcedirAny.(PathExpr)
		if !ok {
			return nil, fmt.Errorf("%s: @source must be a string", sourcedirAny.position())
		}
		builddir = sourcedir.value
	} else {
		var err error
		builddir, err = os.MkdirTemp("", "bake-")
		if err != nil {
			return nil, err
		}
		deletebuilddir = true
	}

	defer func() {
		if deletebuilddir {
			os.RemoveAll(builddir)
		}
	}()

	environ := append(os.Environ(), "out="+outdir)
	for key, value := range result.values {
		if key != "" && key[0] != '@' {
			enc, err := value.encodeEnviron(true)
			if err != nil {
				return nil, err
			}
			environ = append(environ, key+"="+enc)
		}
	}

	logpath := path.Join(ev.LogDir, hashstr+".log")
	logfile, err := os.Create(logpath)
	if err != nil {
		logfile = os.Stdout
	}
	defer logfile.Close()

	cmd := exec.Command(cmdline[0], cmdline[1:]...)
	cmd.Env = environ
	cmd.Dir = builddir
	cmd.Stdin = nil
	cmd.Stdout = logfile
	cmd.Stderr = logfile
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s: building %s failed, for logs look in %s: %w", token.position(), hashstr, logpath, err)
	}

	dur := time.Since(start).Round(time.Millisecond)
	fmt.Fprintf(os.Stderr, "%s (%v)\n", hashstr, dur)

	success = true
	return PathExpr{value: outdir}, nil
}
