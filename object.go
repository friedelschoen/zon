package main

import (
	"errors"
	"fmt"
	"io"
	"iter"
	"maps"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
)

/* unresolved value */
type Expression interface {
	position() string
	hashValue(w io.Writer)
	resolve(scope map[string]Value, ev *Evaluator) (Value, error)
}

/* resolved value */
type Value interface {
	position() string
	encodeEnviron(root bool) (string, error)
	symlink(resultname string) error
	jsonObject() any
}

type BaseExpr struct {
	filename string
	line     int
	offset   int
}

func (o BaseExpr) String() string {
	return o.position()
}

func (o BaseExpr) position() string {
	if o.filename == "" {
		return "<unknown>"
	}

	return fmt.Sprintf("%s:%d:%d", path.Base(o.filename), o.line, o.offset)
}

func (o BaseExpr) symlink(string) error {
	return fmt.Errorf("%s: unable to symlink object of type: %T", o.position(), o)
}

func (obj BaseExpr) encodeEnviron(bool) (string, error) {
	return "", fmt.Errorf("%s: unable to encode %T", obj.position(), obj)
}

type MapExpr struct {
	BaseExpr

	extends []Expression
	expr    []Expression
}

type MapValue struct {
	BaseExpr

	values map[string]Value
}

func (o MapValue) jsonObject() any {
	result := make(map[string]any)
	for k, v := range o.values {
		result[k] = v.jsonObject()
	}
	return result
}

func parallelResolve[K any](values iter.Seq2[K, Expression], set func(K, Value), scope map[string]Value, ev *Evaluator) error {
	var errs []error
	if !ev.Serial {
		var (
			wg sync.WaitGroup
			mu sync.Mutex
		)
		mu.Lock()
		for k, v := range values {
			wg.Add(1)
			go func() {
				val, err := v.resolve(scope, ev)
				mu.Lock()
				if err == nil {
					set(k, val)
				}
				errs = append(errs, err)
				mu.Unlock()
				wg.Done()
			}()
		}
		mu.Unlock()
		wg.Wait()
	} else {
		for k, v := range values {
			val, err := v.resolve(scope, ev)
			if err == nil {
				set(k, val)
			}
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (o MapExpr) resolve(scope map[string]Value, ev *Evaluator) (Value, error) {
	values := make([]Value, len(o.expr))
	err := parallelResolve(slices.All(o.expr), func(k int, v Value) { values[k] = v }, scope, ev)
	if err != nil {
		return nil, err
	}

	res := MapValue{
		BaseExpr: o.BaseExpr,
		values:   make(map[string]Value),
	}

	for i := 0; i < len(values); i += 2 {
		key, value := values[i], values[i+1]
		keyStr, ok := key.(StringValue)
		if !ok {
			return nil, fmt.Errorf("%s: expected string-key, got %T", key.position(), key)
		}
		res.values[keyStr.content] = value
	}

	for _, extname := range o.extends {
		othervalue, err := extname.resolve(scope, ev)
		if err != nil {
			return nil, err
		}
		otherast, ok := othervalue.(MapValue)
		if !ok {
			return nil, fmt.Errorf("%s: unable to extend %T", o.position(), othervalue)
		}
		maps.Copy(res.values, otherast.values)
	}

	return res, nil
}

func (obj MapExpr) hashValue(w io.Writer) {
	fmt.Fprint(w, "map")
	for _, k := range obj.expr {
		k.hashValue(w)
	}
}

func (obj MapValue) encodeEnviron(root bool) (string, error) {
	if !root {
		return "", fmt.Errorf("%s: unable to encode nested %T", obj.position(), obj.values)
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
	BaseExpr

	values []Expression
}

type ArrayValue struct {
	BaseExpr

	values []Value
}

func (o ArrayValue) jsonObject() any {
	result := make([]any, len(o.values))
	for i, v := range o.values {
		result[i] = v.jsonObject()
	}
	return result
}

func (o ArrayExpr) resolve(scope map[string]Value, ev *Evaluator) (Value, error) {
	res := ArrayValue{
		BaseExpr: o.BaseExpr,
		values:   make([]Value, len(o.values)),
	}
	return res, parallelResolve(slices.All(o.values), func(i int, v Value) { res.values[i] = v }, scope, ev)
}

func (obj ArrayExpr) hashValue(w io.Writer) {
	fmt.Fprint(w, "list")
	for _, elem := range obj.values {
		elem.hashValue(w)
	}
}

func (obj ArrayValue) encodeEnviron(root bool) (string, error) {
	if !root {
		return "", fmt.Errorf("%s: unable to encode nested %T", obj.position(), obj.values)
	}
	var builder strings.Builder
	for i, elem := range obj.values {
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

func (o ArrayValue) symlink(resname string) error {
	var errs []error
	if resname != "" {
		for i, r := range o.values {
			errs = append(errs, r.symlink(fmt.Sprintf("%s-%d", resname, i)))
		}
	}
	return errors.Join(errs...)
}

type StringValue struct {
	BaseExpr

	content string
}

func (o StringValue) jsonObject() any {
	return o.content
}

func (obj StringValue) hashValue(w io.Writer) {
	fmt.Fprintf(w, "%T", obj.content)
	fmt.Fprint(w, obj.content)
}

func (obj StringValue) encodeEnviron(root bool) (string, error) {
	return obj.content, nil
}

func (o StringValue) symlink(resname string) error {
	if resname != "" {
		if stat, err := os.Lstat(resname); err == nil && (stat.Mode()&os.ModeType) != os.ModeSymlink {
			return fmt.Errorf("unable to make symlink: exist")
		}
		os.Remove(resname)
		return os.Symlink(o.content, resname)
	}
	return nil
}

type StringExpr struct {
	BaseExpr

	content []string
	interp  []Expression
}

func (o StringExpr) jsonObject() any {
	return o.content
}

func (obj StringExpr) resolve(scope map[string]Value, ev *Evaluator) (Value, error) {
	var res strings.Builder
	for i := range obj.content {
		res.WriteString(obj.content[i])
		if obj.interp[i] == nil {
			continue
		}
		intp, err := obj.interp[i].resolve(scope, ev)
		if err != nil {
			return nil, err
		}
		switch intp := intp.(type) {
		case StringValue:
			res.WriteString(intp.content)
		case PathExpr:
			res.WriteString(intp.value)
		default:
			return nil, fmt.Errorf("%s: unable to interpolate %T", obj.position(), intp)
		}
	}
	return StringValue{
		obj.BaseExpr,
		res.String(),
	}, nil
}

func (obj StringExpr) hashValue(w io.Writer) {
	fmt.Fprintf(w, "%T", obj.content)
	fmt.Fprint(w, obj.content)
}

type NumberExpr struct {
	BaseExpr

	value float64
}

func (o NumberExpr) jsonObject() any {
	return o.value
}

func (o NumberExpr) resolve(scope map[string]Value, ev *Evaluator) (Value, error) {
	return o, nil
}

func (obj NumberExpr) hashValue(w io.Writer) {
	fmt.Fprintf(w, "%T", obj)
	fmt.Fprint(w, obj.value)
}

func (obj NumberExpr) encodeEnviron(root bool) (string, error) {
	return strconv.FormatFloat(obj.value, 'f', -1, 64), nil
}

type BooleanExpr struct {
	BaseExpr

	value bool
}

func (o BooleanExpr) jsonObject() any {
	return o.value
}

func (o BooleanExpr) resolve(scope map[string]Value, ev *Evaluator) (Value, error) {
	return o, nil
}

func (obj BooleanExpr) hashValue(w io.Writer) {
	fmt.Fprintf(w, "%T", obj)
	fmt.Fprint(w, obj.value)
}

func (obj BooleanExpr) encodeEnviron(root bool) (string, error) {
	if obj.value {
		return "1", nil
	}
	return "0", nil
}

type PathExpr struct {
	BaseExpr

	value string
}

func (o PathExpr) jsonObject() any {
	return o.value
}

func (o PathExpr) resolve(scope map[string]Value, ev *Evaluator) (Value, error) {
	return o, nil
}

func (obj PathExpr) hashValue(w io.Writer) {
	fmt.Fprintf(w, "%T", obj)
	fmt.Fprint(w, obj.value)
}

func (obj PathExpr) encodeEnviron(root bool) (string, error) {
	return obj.value, nil
}

type VarExpr struct {
	BaseExpr

	name string
}

func (o VarExpr) resolve(scope map[string]Value, ev *Evaluator) (Value, error) {
	val, ok := scope[o.name]
	if !ok {
		return nil, fmt.Errorf("%s: not in scope: %s", o.position(), o.name)
	}
	return val, nil
	// for _, attr := range o.name[1:] {
	// 	val, err := val.resolve(scope, ev)
	// 	if err != nil {
	// 		return nil, err
	// 	}
	// 	switch obj := val.(type) {
	// 	case MapExpr:
	// 		val, ok = obj.values[attr]
	// 		if !ok {
	// 			return nil, fmt.Errorf("%s: map has no attribute %s", o.position(), attr)
	// 		}
	// 		val, err = val.resolve(scope, ev)
	// 		if err != nil {
	// 			return nil, err
	// 		}
	// 	default:
	// 		return nil, fmt.Errorf("%s: %T has no attributes", o.position(), obj)
	// 	}
	// }
}

func (obj VarExpr) hashValue(w io.Writer) {
	fmt.Fprintf(w, "%T", obj)
	fmt.Fprint(w, obj.name)
}

type AttributeExpr struct {
	BaseExpr

	base Expression
	name string
}

func (o AttributeExpr) resolve(scope map[string]Value, ev *Evaluator) (Value, error) {
	val, err := o.base.resolve(scope, ev)
	if err != nil {
		return nil, err
	}
	switch obj := val.(type) {
	case MapValue:
		val, ok := obj.values[o.name]
		if !ok {
			return nil, fmt.Errorf("%s: map has no attribute %s", o.position(), o.name)
		}
		return val, nil
	default:
		return nil, fmt.Errorf("%s: %T has no attributes", o.position(), obj)
	}
}

func (obj AttributeExpr) hashValue(w io.Writer) {
	fmt.Fprintf(w, "%T", obj)
	fmt.Fprint(w, obj.name)
}

type IncludeExpr struct {
	BaseExpr

	name Expression
}

func (o IncludeExpr) resolve(scope map[string]Value, ev *Evaluator) (Value, error) {
	pathAny, err := o.name.resolve(scope, ev)
	if err != nil {
		return nil, err
	}
	path, ok := pathAny.(PathExpr)
	if !ok {
		return nil, fmt.Errorf("%s: unable to include non-path: %T", o.position(), path)
	}
	expr, err := parseFile(path)
	if err != nil {
		return nil, err
	}
	return expr.resolve(scope, ev)
}

func (obj IncludeExpr) hashValue(w io.Writer) {
	fmt.Fprintf(w, "%T", obj)
	fmt.Fprint(w, obj.name)
}

type DefineExpr struct {
	BaseExpr

	define map[string]Expression
	value  Expression
}

func (o DefineExpr) jsonObject() any {
	return nil
}

func (o DefineExpr) resolve(scope map[string]Value, ev *Evaluator) (Value, error) {
	newscope := maps.Clone(scope)
	var err error
	for k, v := range o.define {
		newscope[k], err = v.resolve(scope, ev)
		if err != nil {
			return nil, err
		}
	}
	return o.value.resolve(newscope, ev)
}

func (obj DefineExpr) hashValue(w io.Writer) {
	fmt.Fprintf(w, "%T", obj)
	obj.value.hashValue(w)
}

type OutputExpr struct {
	BaseExpr

	attrs Expression
}

func (obj OutputExpr) hashValue(w io.Writer) {
	fmt.Fprintf(w, "%T", obj)
	obj.attrs.hashValue(w)
}
