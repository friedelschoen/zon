package types

import (
	"fmt"
	"io"
	"maps"
)

type IncludeExpr struct {
	Position

	Name Expression
}

func (obj IncludeExpr) Resolve(scope map[string]Value, ev *Evaluator) (Value, error) {
	pathAny, err := obj.Name.Resolve(scope, ev)
	if err != nil {
		return nil, err
	}
	path, ok := pathAny.(PathExpr)
	if !ok {
		return nil, fmt.Errorf("%s: unable to include non-path: %T", obj.Pos(), path)
	}
	expr, err := ev.ParseFile(path)
	if err != nil {
		return nil, err
	}
	return expr.Resolve(scope, ev)
}

func (obj IncludeExpr) hashValue(w io.Writer) {
	fmt.Fprintf(w, "%T", obj)
	fmt.Fprint(w, obj.Name)
}

type DefineExpr struct {
	Position

	Define map[string]Expression
	Expr   Expression
}

func (obj DefineExpr) JSON() any {
	return nil
}

func (obj DefineExpr) Resolve(scope map[string]Value, ev *Evaluator) (Value, error) {
	newscope := maps.Clone(scope)
	var err error
	for k, v := range obj.Define {
		newscope[k], err = v.Resolve(scope, ev)
		if err != nil {
			return nil, err
		}
	}
	return obj.Expr.Resolve(newscope, ev)
}

func (obj DefineExpr) hashValue(w io.Writer) {
	fmt.Fprintf(w, "%T", obj)
	obj.Expr.hashValue(w)
}
