package types

import (
	"fmt"
	"io"
	"maps"
)

type VarExpr struct {
	Position

	Name string
	Args []Expression
}

func (obj VarExpr) Resolve(scope Scope, ev *Evaluator) (Value, []PathExpr, error) {
	expr, ok := scope[obj.Name]
	if !ok {
		return nil, nil, fmt.Errorf("%s: not in scope: %s", obj.Pos(), obj.Name)
	}
	return expr.Expr.Resolve(expr.Scope, ev)
}

func (obj VarExpr) hashValue(w io.Writer) {
	fmt.Fprintf(w, "var")
	fmt.Fprint(w, obj.Name)
	for _, a := range obj.Args {
		a.hashValue(w)
	}
}

type AttributeExpr struct {
	Position

	Base Expression
	Name string
}

func (obj AttributeExpr) Resolve(scope Scope, ev *Evaluator) (Value, []PathExpr, error) {
	val, deps, err := obj.Base.Resolve(scope, ev)
	if err != nil {
		return nil, nil, err
	}
	switch mapval := val.(type) {
	case MapValue:
		val, ok := mapval.Values[obj.Name]
		if !ok {
			return nil, nil, fmt.Errorf("%s: map has no attribute %s", mapval.Pos(), obj.Name)
		}
		return val, deps, nil
	default:
		return nil, nil, fmt.Errorf("%s: %T has no attributes", mapval.Pos(), mapval)
	}
}

func (obj AttributeExpr) hashValue(w io.Writer) {
	fmt.Fprintf(w, "attribute")
	fmt.Fprint(w, obj.Name)
	obj.Base.hashValue(w)
}

type CallExpr struct {
	Position

	Base Expression
	Args []Expression
}

func (obj CallExpr) Resolve(scope Scope, ev *Evaluator) (Value, []PathExpr, error) {
	value, deps, err := obj.Base.Resolve(scope, ev)
	if err != nil {
		return nil, nil, err
	}
	lambda, ok := value.(LambdaExpr)
	if !ok {
		return nil, nil, fmt.Errorf("%s: unable to call %T", obj.Pos(), value)
	}
	if len(lambda.Args) != len(obj.Args) {
		return nil, nil, fmt.Errorf("%s: variable expecting %d arguments, got %d", obj.Pos(), len(lambda.Args), len(obj.Args))
	}
	newscope := scope
	if len(lambda.Args) > 0 {
		newscope = maps.Clone(newscope)
		for i, name := range lambda.Args {
			newscope[name] = Variable{obj.Args[i], scope}
		}
	}
	res, paths, err := lambda.Expr.Resolve(newscope, ev)
	deps = append(deps, paths...)
	return res, deps, err
}

func (obj CallExpr) hashValue(w io.Writer) {
	fmt.Fprintf(w, "call")
	obj.Base.hashValue(w)
	for _, a := range obj.Args {
		a.hashValue(w)
	}
}
