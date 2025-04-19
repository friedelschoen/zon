package types

import (
	"fmt"
	"hash/fnv"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"time"
)

func GetValue[T Value](resultname string, result MapValue, name string) (ret T, err error) {
	valueAny, ok := result.values[name]
	if !ok {
		return ret, fmt.Errorf("%s: %s has no attribute '%s'", result.Pos(), resultname, name)
	}
	value, ok := valueAny.(T)
	if !ok {
		return ret, fmt.Errorf("%s: %s attribute '%s' should be a %T, got %T", result.Pos(), resultname, name, ret, valueAny)
	}
	return value, nil
}

type OutputExpr struct {
	Position

	Attrs Expression
}

func (obj OutputExpr) hashValue(w io.Writer) {
	fmt.Fprintf(w, "%T", obj)
	obj.Attrs.hashValue(w)
}

func (obj OutputExpr) Resolve(scope map[string]Value, ev *Evaluator) (Value, []PathExpr, error) {
	attrsAny, deps, err := obj.Attrs.Resolve(scope, ev)
	if err != nil {
		return nil, nil, err
	}
	result, ok := attrsAny.(MapValue)
	if !ok {
		return nil, nil, fmt.Errorf("%s: unable to output non-map: %T", obj.Pos(), attrsAny)
	}

	impure := false
	if impureAny, ok := result.values["impure"]; ok {
		if impureVal, ok := impureAny.(BooleanExpr); ok {
			impure = impureVal.Value
		}
	}

	hashlib := fnv.New128()
	var hashsum []byte
	if impure {
		hashsum = make([]byte, hashlib.Size())
		for i := range hashsum {
			hashsum[i] = byte(rand.Int())
		}
	} else {
		obj.Attrs.hashValue(hashlib)
		hashsum = hashlib.Sum(nil)
	}

	name, err := GetValue[StringValue]("output", result, "name")
	if err != nil {
		return nil, nil, err
	}

	hashstr := fmt.Sprintf("%x-%s", hashsum, name.Content)

	ev.Outputs = append(ev.Outputs, hashstr)

	cwd, _ := os.Getwd()
	outdir := path.Join(cwd, ev.CacheDir, hashstr)
	if _, err := os.Stat(outdir); (ev.DryRun || err == nil) && !ev.Force {
		res := PathExpr{Name: outdir, Depends: deps}
		return res, []PathExpr{res}, nil
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

	if outputAny, ok := result.values["output"]; ok {
		token = outputAny
		install, err := GetValue[StringValue]("output", result, "output")
		if err != nil {
			return nil, nil, err
		}

		exec := ev.Interpreter
		if _, ok := result.values["interpreter"]; ok {
			execValue, err := GetValue[StringValue]("output", result, "interpreter")
			if err != nil {
				return nil, nil, err
			}
			exec = execValue.Content
		}
		cmdline = []string{exec, "-e", "-c", install.Content, "builder"}
	} else if builderAny, ok := result.values["builder"]; ok {
		token = builderAny
		builder, err := GetValue[StringValue]("output", result, "builder")
		if err != nil {
			return nil, nil, err
		}
		cmdline = []string{builder.Content}
	} else {
		return nil, nil, fmt.Errorf("%s: missing output or builder", obj.Pos())
	}

	if _, ok := result.values["args"]; ok {
		args, err := GetValue[ArrayValue]("output", result, "args")
		if err != nil {
			return nil, nil, err
		}
		for _, elem := range args.Values[1:] {
			arg, ok := elem.(StringValue)
			if !ok {
				return nil, nil, fmt.Errorf("%s: non-string in args: %T", elem.Pos(), elem)
			}
			cmdline = append(cmdline, string(arg.Content))
		}
	}

	var builddir string
	var deletebuilddir bool

	if _, ok := result.values["source"]; ok {
		sourcedir, err := GetValue[PathExpr]("output", result, "source")
		if err != nil {
			return nil, nil, err
		}
		builddir = sourcedir.Name
	} else {
		var err error
		builddir, err = os.MkdirTemp("", "zon-")
		if err != nil {
			return nil, nil, err
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
		enc, err := value.encodeEnviron(true)
		if err != nil {
			return nil, nil, err
		}
		environ = append(environ, key+"="+enc)
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
		return nil, nil, fmt.Errorf("%s: building %s failed, for logs look in %s: %w", token.Pos(), hashstr, logpath, err)
	}

	dur := time.Since(start).Round(time.Millisecond)
	fmt.Fprintf(os.Stderr, "%s (%v)\n", hashstr, dur)

	success = true
	res := PathExpr{Name: outdir, Depends: deps}
	return res, []PathExpr{res}, nil
}
