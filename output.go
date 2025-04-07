package main

import (
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"math/rand"
	"os"
	"os/exec"
	"path"
	"strings"
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

func (ev *Evaluator) output(result ObjectMap) (Object, error) {
	impure := false
	if impureAny, ok := result.values["@impure"]; ok {
		if impureVal, ok := impureAny.(ObjectBoolean); ok {
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
		result.hashValue(hashlib)
		hashsum = hashlib.Sum(nil)
	}
	hashstr := hex.EncodeToString(hashsum)

	var names []string
	for node := Object(result); node != nil; node = node.Parent() {
		if mapv, ok := node.(ObjectMap); ok {
			if nameAny, ok := mapv.values["@name"]; ok {
				if name, ok := nameAny.(ObjectString); ok && (len(names) == 0 || names[len(names)-1] != name.content) {
					names = append(names, name.content)
				}
			}
		}
	}

	if len(names) >= 2 {
		ev.Edges = append(ev.Edges, [2]string{names[1], names[0]})
	}

	ev.Outputs = append(ev.Outputs, hashstr)

	cwd, _ := os.Getwd()
	outdir := path.Join(cwd, ev.CacheDir, hashstr)
	if _, err := os.Stat(outdir); (ev.DryRun || err == nil) && !ev.Force {
		return ObjectString{content: outdir}, nil
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
	var token Object

	if installAny, ok := result.values["@output"]; ok {
		token = installAny
		install, ok := installAny.(ObjectString)
		if !ok {
			return nil, fmt.Errorf("%s: @output must be a string", installAny.position())
		}

		interpreter := ev.Interpreter
		if interpreterAny, ok := result.values["@interpreter"]; ok {
			if str, ok := interpreterAny.(ObjectString); ok {
				interpreter = str.content
			} else {
				return nil, fmt.Errorf("%s: @interpreter must be a string", interpreterAny.position())
			}
		}
		cmdline = []string{interpreter, "-e", "-c", install.content, "builder"}
	} else if builderAny, ok := result.values["@builder"]; ok {
		token = builderAny
		builder, ok := builderAny.(ObjectString)
		if !ok {
			return nil, fmt.Errorf("%s: @builder must be a string", builderAny.position())
		}
		cmdline = []string{builder.content}
	}

	if argsAny, ok := result.values["@args"]; ok {
		args, ok := argsAny.(ObjectArray)
		if !ok {
			return nil, fmt.Errorf("%s: @args must be an array", argsAny.position())
		}
		for _, elem := range args.values[1:] {
			arg, ok := elem.(ObjectString)
			if !ok {
				return nil, fmt.Errorf("%s: non-string in @args: %T", elem.position(), elem)
			}
			cmdline = append(cmdline, string(arg.content))
		}
	}

	var builddir string
	var deletebuilddir bool

	if sourcedirAny, ok := result.values["@source"]; ok {
		sourcedir, ok := sourcedirAny.(ObjectString)
		if !ok {
			return nil, fmt.Errorf("%s: @source must be a string", sourcedirAny.position())
		}
		builddir = sourcedir.content
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
	if len(names) > 0 {
		fmt.Fprintf(os.Stderr, "%s %s (%v)\n", hashstr, strings.Join(names, " > "), dur)
	} else {
		fmt.Fprintf(os.Stderr, "%s (%v)\n", hashstr, dur)
	}

	success = true
	return ObjectString{content: outdir}, nil
}
