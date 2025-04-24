package types

import (
	"fmt"
	"io"
	"path"
)

type Evaluator struct {
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

type Variable struct {
	Expr  Expression
	Args  []string
	Scope Scope
}

type Scope map[string]Variable

/* unresolved value */
type Expression interface {
	Pos() string
	hashValue(w io.Writer)
	Resolve(scope Scope, ev *Evaluator) (Value, []PathExpr, error)
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
