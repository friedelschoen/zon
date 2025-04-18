package types

import (
	"fmt"
	"io"
	"path"
	"sync"
)

type WorkerItem struct {
	expr   Expression
	setter func(Value, error)
	scope  map[string]Value
}

type Evaluator struct {
	Config

	wg    sync.WaitGroup
	queue chan WorkerItem
}

func (ev *Evaluator) Start(num int) {
	ev.queue = make(chan WorkerItem, num)
	for range num {
		ev.wg.Add(1)
		go func() {
			for job := range ev.queue {
				job.setter(job.expr.Resolve(job.scope, ev))
			}
			ev.wg.Done()
		}()
	}
}

func (ev *Evaluator) Stop() {
	close(ev.queue)
	ev.wg.Wait()
}

func (ev *Evaluator) Submit(expr Expression, setter func(Value, error), scope map[string]Value) {
	ev.queue <- WorkerItem{expr, setter, scope}
}

type Config struct {
	Force        bool
	DryRun       bool
	CacheDir     string
	LogDir       string
	Serial       bool
	Interpreter  string
	NoEvalOutput bool

	ParseFile func(filename PathExpr) (Expression, error)

	Outputs []string
}

/* unresolved value */
type Expression interface {
	Pos() string
	hashValue(w io.Writer)
	Resolve(scope map[string]Value, ev *Evaluator) (Value, error)
}

/* resolved value */
type Value interface {
	Pos() string
	encodeEnviron(root bool) (string, error)
	Link(resultname string) error
	JSON() any
}

type Position struct {
	Filename string
	Line     int
	Offset   int
}

func (obj Position) String() string {
	return obj.Pos()
}

func (obj Position) Pos() string {
	if obj.Filename == "" {
		return "<unknown>"
	}

	return fmt.Sprintf("%s:%d:%d", path.Base(obj.Filename), obj.Line, obj.Offset)
}
