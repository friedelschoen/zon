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
	fmt.Fprintf(w, "include")
	obj.Name.hashValue(w)
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
	fmt.Fprintf(w, "define")
	for k, v := range obj.Define {
		fmt.Fprint(w, k)
		v.hashValue(w)
	}
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
	fmt.Fprint(w, "fn")
	for _, a := range obj.Args {
		fmt.Fprint(w, a)
	}
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

func (obj LambdaExpr) Boolean() (bool, error) {
	return false, fmt.Errorf("lamba's do not have an boolean expression")
}

type ConditionExpr struct {
	Position

	Cond  Expression
	Truly Expression
	Falsy Expression
}

func (obj ConditionExpr) JSON() any {
	return nil
}

func (obj ConditionExpr) Resolve(scope Scope, ev *Evaluator) (Value, []PathExpr, error) {
	cond, deps, err := obj.Cond.Resolve(scope, ev)
	if err != nil {
		return nil, nil, err
	}
	var expr Expression
	b, err := cond.Boolean()
	if err != nil {
		return nil, nil, err
	}
	if b {
		expr = obj.Truly
	} else {
		expr = obj.Falsy
	}
	val, vdeps, err := expr.Resolve(scope, ev)
	return val, append(deps, vdeps...), err
}

func (obj ConditionExpr) hashValue(w io.Writer) {
	fmt.Fprintf(w, "condition")
	obj.Cond.hashValue(w)
	obj.Truly.hashValue(w)
	obj.Falsy.hashValue(w)
}

type OperationExpr struct {
	Position

	Operator string
	Left     Expression
	Right    Expression
}

func (obj OperationExpr) JSON() any {
	return nil
}

func (obj OperationExpr) Resolve(scope Scope, ev *Evaluator) (Value, []PathExpr, error) {
	return nil, nil, fmt.Errorf("not implemented")
}

func (obj OperationExpr) hashValue(w io.Writer) {
	fmt.Fprint(w, "operation")
	fmt.Fprint(w, obj.Operator)
	obj.Left.hashValue(w)
	obj.Right.hashValue(w)
}
