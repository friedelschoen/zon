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

type OutputExpr struct {
	Position

	Attrs Expression
}

func (obj OutputExpr) hashValue(w io.Writer) {
	fmt.Fprintf(w, "%T", obj)
	obj.Attrs.hashValue(w)
}

func (obj OutputExpr) Resolve(scope map[string]Value, ev *Evaluator) (Value, error) {
	attrsAny, err := obj.Attrs.Resolve(scope, ev)
	if err != nil {
		return nil, err
	}
	result, ok := attrsAny.(MapValue)
	if !ok {
		return nil, fmt.Errorf("%s: unable to output non-map: %T", obj.Pos(), attrsAny)
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

	nameAny, ok := result.values["name"]
	if !ok {
		return nil, fmt.Errorf("%s: output requires attribute name", result.Pos())
	}
	name, ok := nameAny.(StringValue)
	if !ok {
		return nil, fmt.Errorf("%s: output->name must be string", result.Pos())
	}

	hashstr := fmt.Sprintf("%x-%s", hashsum, name.Content)

	ev.Outputs = append(ev.Outputs, hashstr)

	cwd, _ := os.Getwd()
	outdir := path.Join(cwd, ev.CacheDir, hashstr)
	if _, err := os.Stat(outdir); (ev.DryRun || err == nil) && !ev.Force {
		return PathExpr{Name: outdir}, nil
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

	if installAny, ok := result.values["output"]; ok {
		token = installAny
		install, ok := installAny.(StringValue)
		if !ok {
			return nil, fmt.Errorf("%s: output must be a string", installAny.Pos())
		}

		interpreter := ev.Interpreter
		if interpreterAny, ok := result.values["interpreter"]; ok {
			if str, ok := interpreterAny.(StringValue); ok {
				interpreter = str.Content
			} else {
				return nil, fmt.Errorf("%s: interpreter must be a string", interpreterAny.Pos())
			}
		}
		cmdline = []string{interpreter, "-e", "-c", install.Content, "builder"}
	} else if builderAny, ok := result.values["builder"]; ok {
		token = builderAny
		builder, ok := builderAny.(StringValue)
		if !ok {
			return nil, fmt.Errorf("%s: builder must be a string", builderAny.Pos())
		}
		cmdline = []string{builder.Content}
	} else {
		return nil, fmt.Errorf("%s: missing output or builder", obj.Pos())
	}

	if argsAny, ok := result.values["args"]; ok {
		args, ok := argsAny.(ArrayValue)
		if !ok {
			return nil, fmt.Errorf("%s: args must be an array", argsAny.Pos())
		}
		for _, elem := range args.Values[1:] {
			arg, ok := elem.(StringValue)
			if !ok {
				return nil, fmt.Errorf("%s: non-string in args: %T", elem.Pos(), elem)
			}
			cmdline = append(cmdline, string(arg.Content))
		}
	}

	var builddir string
	var deletebuilddir bool

	if sourcedirAny, ok := result.values["source"]; ok {
		sourcedir, ok := sourcedirAny.(PathExpr)
		if !ok {
			return nil, fmt.Errorf("%s: source must be a path", sourcedirAny.Pos())
		}
		builddir = sourcedir.Name
	} else {
		var err error
		builddir, err = os.MkdirTemp("", "zon-")
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
		enc, err := value.encodeEnviron(true)
		if err != nil {
			return nil, err
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
		return nil, fmt.Errorf("%s: building %s failed, for logs look in %s: %w", token.Pos(), hashstr, logpath, err)
	}

	dur := time.Since(start).Round(time.Millisecond)
	fmt.Fprintf(os.Stderr, "%s (%v)\n", hashstr, dur)

	success = true
	return PathExpr{Name: outdir}, nil
}
