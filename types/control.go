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

func (obj IncludeExpr) Resolve(scope Scope, ev *Evaluator) (Value, []PathExpr, error) {
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

func (obj DefineExpr) Resolve(scope Scope, ev *Evaluator) (Value, []PathExpr, error) {
	newscope := maps.Clone(scope)
	for name, expr := range obj.Define {
		newscope[name] = Variable{expr, scope}
	}
	return obj.Expr.Resolve(newscope, ev)
}

func (obj DefineExpr) hashValue(w io.Writer) {
	fmt.Fprintf(w, "%T", obj)
	obj.Expr.hashValue(w)
}

type LambdaExpr struct {
	Position

	Args []string
	Expr Expression
}

func (obj LambdaExpr) Resolve(scope Scope, ev *Evaluator) (Value, []PathExpr, error) {
	return obj, nil, nil
}

func (obj LambdaExpr) hashValue(w io.Writer) {
	fmt.Fprintf(w, "fn(")
	for i, a := range obj.Args {
		if i > 0 {
			fmt.Fprint(w, ",")
		}
		fmt.Fprint(w, a)
	}
	fmt.Fprint(w, ")")
	obj.Expr.hashValue(w)
}

func (obj LambdaExpr) encodeEnviron(root bool) (string, error) {
	return "", fmt.Errorf("%s: unable to encode %T to environment", obj.Pos(), obj)
}

func (obj LambdaExpr) Link(resultname string) error {
	return fmt.Errorf("%s: unable to link %T", obj.Pos(), obj)
}

func (obj LambdaExpr) JSON() any {
	return nil
}
