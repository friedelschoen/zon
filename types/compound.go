package types

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"strings"
	"sync"
)

func parallelResolve(exprs []Expression, scope map[string]Value, ev *Evaluator) ([]Value, error) {
	// var errs []error
	// if !ev.Serial {
	// 	var (
	// 		wg sync.WaitGroup
	// 		mu sync.Mutex
	// 	)
	// 	mu.Lock()
	// 	for k, v := range values {
	// 		wg.Add(1)
	// 		go func() {
	// 			val, err := v.Resolve(scope, ev)
	// 			mu.Lock()
	// 			if err == nil {
	// 				set(k, val)
	// 			}
	// 			errs = append(errs, err)
	// 			mu.Unlock()
	// 			wg.Done()
	// 		}()
	// 	}
	// 	mu.Unlock()
	// 	wg.Wait()
	// } else {
	// 	for k, v := range values {
	// 		val, err := v.Resolve(scope, ev)
	// 		if err == nil {
	// 			set(k, val)
	// 		}
	// 		errs = append(errs, err)
	// 	}
	// }

	var (
		mu     sync.Mutex
		wg     sync.WaitGroup
		errs   = make([]error, len(exprs))
		values = make([]Value, len(exprs))
	)
	for i, v := range exprs {
		wg.Add(1)
		ev.Submit(v, func(v Value, err error) {
			mu.Lock()
			errs[i] = err
			values[i] = v
			mu.Unlock()
			wg.Done()
		}, scope)
	}

	wg.Wait()
	return values, errors.Join(errs...)
}

type MapExpr struct {
	Position

	Extends []Expression
	Exprs   []Expression
}

func (obj MapExpr) Resolve(scope map[string]Value, ev *Evaluator) (Value, error) {
	values, err := parallelResolve(obj.Exprs, scope, ev)
	if err != nil {
		return nil, err
	}

	res := MapValue{
		Position: obj.Position,
		values:   make(map[string]Value),
	}

	for i := 0; i < len(values); i += 2 {
		key, value := values[i], values[i+1]
		keyStr, ok := key.(StringValue)
		if !ok {
			return nil, fmt.Errorf("%s: expected string-key, got %T", key.Pos(), key)
		}
		res.values[keyStr.Content] = value
	}

	for _, extname := range obj.Extends {
		othervalue, err := extname.Resolve(scope, ev)
		if err != nil {
			return nil, err
		}
		otherast, ok := othervalue.(MapValue)
		if !ok {
			return nil, fmt.Errorf("%s: unable to extend %T", obj.Pos(), othervalue)
		}
		maps.Copy(res.values, otherast.values)
	}

	return res, nil
}

func (obj MapExpr) hashValue(w io.Writer) {
	fmt.Fprint(w, "map")
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

func (obj ArrayExpr) Resolve(scope map[string]Value, ev *Evaluator) (Value, error) {
	res := ArrayValue{
		Position: obj.Position,
	}
	var err error
	res.Values, err = parallelResolve(obj.Exprs, scope, ev)
	return res, err
}

func (obj ArrayExpr) hashValue(w io.Writer) {
	fmt.Fprint(w, "list")
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
