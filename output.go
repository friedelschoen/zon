package main

import (
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"io"
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
	hashlib := fnv.New64()
	if impure {
		hashsum = make([]byte, hashlib.Size())
		for i := range hashsum {
			hashsum[i] = byte(rand.Int())
		}
	} else {
		result.hashValue(hashlib)
		hashsum = hashlib.Sum(nil)
	}
	hashstr := hex.EncodeToString(hashsum[:])

	var names []string
	for node := Object(result); node != nil; node = node.Parent() {
		if mapv, ok := node.(ObjectMap); ok {
			if nameAny, ok := mapv.values["@name"]; ok {
				if name, ok := nameAny.(ObjectString); ok && (len(names) == 0 || names[len(names)-1] != name.value) {
					names = append(names, name.value)
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
		return ObjectString{value: outdir}, nil
	}

	start := time.Now()

	os.RemoveAll(outdir)
	success := false
	defer func() {
		if !success {
			os.RemoveAll(outdir)
		}
	}()

	install, ok := result.values["@output"].(ObjectString)
	if !ok {
		return nil, fmt.Errorf("%s: @output must be a string", result.values["@output"].position())
	}

	interpreter := ev.Interpreter
	if interpreterAny, ok := result.values["@interpreter"]; ok {
		if str, ok := interpreterAny.(ObjectString); ok {
			interpreter = str.value
		} else {
			return nil, fmt.Errorf("%s: @interpreter must be a string", interpreterAny.position())
		}
	}

	builddir, err := os.MkdirTemp("", "bake-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(builddir)
	environ := append(os.Environ(), "out="+outdir)
	for key, value := range result.values {
		if key != "" && key[0] == '$' {
			enc, err := value.encodeEnviron(true)
			if err != nil {
				return nil, err
			}
			environ = append(environ, key[1:]+"="+enc)
		}
	}

	logfile, err := os.Create(path.Join(ev.LogDir, hashstr+".log"))
	if err != nil {
		logfile = os.Stdout
	}
	logbuf := &RingBuffer{Content: make([]byte, 1024)}
	stdout := io.MultiWriter(logfile, logbuf)

	cmd := exec.Command(interpreter, "-e", "-c", install.value)
	cmd.Env = environ
	cmd.Dir = builddir
	cmd.Stdin = nil
	cmd.Stdout = stdout
	cmd.Stderr = stdout
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s: %w\n", install.position(), err)
	}

	dur := time.Since(start).Round(time.Millisecond)
	if len(names) > 0 {
		fmt.Fprintf(os.Stderr, "%s %s (%v)\n", hashstr, strings.Join(names, " > "), dur)
	} else {
		fmt.Fprintf(os.Stderr, "%s (%v)\n", hashstr, dur)
	}

	success = true
	return ObjectString{value: outdir}, nil
}
