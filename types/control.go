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

func (obj IncludeExpr) Resolve(scope map[string]Value, ev *Evaluator) (Value, []PathExpr, error) {
	pathAny, deps, err := obj.Name.Resolve(scope, ev)
	if err != nil {
		return nil, nil, err
	}
	path, ok := pathAny.(PathExpr)
	if !ok {
		return nil, nil, fmt.Errorf("%s: unable to include non-path: %T", obj.Pos(), path)
	}
	expr, err := ev.ParseFile(path)
	if err != nil {
		return nil, nil, err
	}
	val, paths, err := expr.Resolve(scope, ev)
	deps = append(deps, paths...)
	return val, deps, err
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

func (obj DefineExpr) Resolve(scope map[string]Value, ev *Evaluator) (Value, []PathExpr, error) {
	newscope := maps.Clone(scope)
	var deps []PathExpr
	var err error
	for k, v := range obj.Define {
		var paths []PathExpr
		newscope[k], paths, err = v.Resolve(scope, ev)
		if err != nil {
			return nil, nil, err
		}
		deps = append(deps, paths...)
	}
	val, paths, err := obj.Expr.Resolve(newscope, ev)
	deps = append(deps, paths...)
	return val, deps, err
}

func (obj DefineExpr) hashValue(w io.Writer) {
	fmt.Fprintf(w, "%T", obj)
	obj.Expr.hashValue(w)
}
