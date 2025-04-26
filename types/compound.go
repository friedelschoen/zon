package types

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"strings"
	"sync"
)

func parallelResolve(exprs []Expression, scope Scope, ev *Evaluator) ([]Value, []PathExpr, error) {
	var (
		values = make([]Value, len(exprs))
		errs   = make([]error, len(exprs))
		deps   = make([]PathExpr, 0, len(exprs))
	)
	if !ev.Serial {
		var (
			wg sync.WaitGroup
			mu sync.Mutex
		)
		mu.Lock()
		for i, v := range exprs {
			wg.Add(1)
			go func() {
				val, paths, err := v.Resolve(scope, ev)
				mu.Lock()
				values[i] = val
				errs[i] = err
				deps = append(deps, paths...)
				mu.Unlock()
				wg.Done()
			}()
		}
		mu.Unlock()
		wg.Wait()
	} else {
		for i, v := range exprs {
			val, paths, err := v.Resolve(scope, ev)
			values[i] = val
			errs[i] = err
			deps = append(deps, paths...)
		}
	}
	return values, deps, errors.Join(errs...)
}

type MapExpr struct {
	Position

	Extends []Expression
	Exprs   []Expression
}

func (obj MapExpr) Resolve(scope Scope, ev *Evaluator) (Value, []PathExpr, error) {
	values, deps, err := parallelResolve(obj.Exprs, scope, ev)
	if err != nil {
		return nil, nil, err
	}

	res := MapValue{
		Position: obj.Position,
		values:   make(map[string]Value),
	}

	for i := 0; i < len(values); i += 2 {
		key, value := values[i], values[i+1]
		keyStr, ok := key.(StringValue)
		if !ok {
			return nil, nil, fmt.Errorf("%s: expected string-key, got %T", key.Pos(), key)
		}
		res.values[keyStr.Content] = value
	}

	for _, extname := range obj.Extends {
		othervalue, otherdeps, err := extname.Resolve(scope, ev)
		if err != nil {
			return nil, nil, err
		}
		otherast, ok := othervalue.(MapValue)
		if !ok {
			return nil, nil, fmt.Errorf("%s: unable to extend %T", obj.Pos(), othervalue)
		}
		maps.Copy(res.values, otherast.values)
		deps = append(deps, otherdeps...)
	}

	return res, deps, nil
}

func (obj MapExpr) hashValue(w io.Writer) {
	fmt.Fprint(w, "map")
	for _, k := range obj.Extends {
		k.hashValue(w)
	}
	for _, k := range obj.Exprs {
		k.hashValue(w)
	}
}

type MapValue struct {
	Position

	values map[string]Value
}

func (obj MapValue) JSON() any {
	result := make(map[string]any)
	for k, v := range obj.values {
		result[k] = v.JSON()
	}
	return result
}

func (obj MapValue) Link(string) error {
	return fmt.Errorf("%s: unable to symlink object of type: %T", obj.Pos(), obj)
}

func (obj MapValue) encodeEnviron(root bool) (string, error) {
	if !root {
		return "", fmt.Errorf("%s: unable to encode nested %T", obj.Pos(), obj.values)
	}
	var builder strings.Builder
	first := true
	for key, elem := range obj.values {
		if !first {
			builder.WriteByte(' ')
		}
		first = false
		builder.WriteString(key)
		builder.WriteByte('=')
		enc, err := elem.encodeEnviron(false)
		if err != nil {
			return "", err
		}
		builder.WriteString(enc)
	}
	return builder.String(), nil
}

type ArrayExpr struct {
	Position

	Exprs []Expression
}

type ArrayValue struct {
	Position

	Values []Value
}

func (obj ArrayValue) JSON() any {
	result := make([]any, len(obj.Values))
	for i, v := range obj.Values {
		result[i] = v.JSON()
	}
	return result
}

func (obj ArrayExpr) Resolve(scope Scope, ev *Evaluator) (Value, []PathExpr, error) {
	res := ArrayValue{
		Position: obj.Position,
	}
	var err error
	var deps []PathExpr
	res.Values, deps, err = parallelResolve(obj.Exprs, scope, ev)
	return res, deps, err
}

func (obj ArrayExpr) hashValue(w io.Writer) {
	fmt.Fprint(w, "array")
	for _, elem := range obj.Exprs {
		elem.hashValue(w)
	}
}

func (obj ArrayValue) encodeEnviron(root bool) (string, error) {
	if !root {
		return "", fmt.Errorf("%s: unable to encode nested %T", obj.Pos(), obj.Values)
	}
	var builder strings.Builder
	for i, elem := range obj.Values {
		if i > 0 {
			builder.WriteByte(' ')
		}
		enc, err := elem.encodeEnviron(false)
		if err != nil {
			return "", err
		}
		builder.WriteString(enc)
	}
	return builder.String(), nil
}

func (obj ArrayValue) Link(resname string) error {
	var errs []error
	if resname != "" {
		for i, r := range obj.Values {
			errs = append(errs, r.Link(fmt.Sprintf("%s-%d", resname, i)))
		}
	}
	return errors.Join(errs...)
}
