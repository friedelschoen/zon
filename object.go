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

type Evaluator struct {
	Force        bool
	DryRun       bool
	CacheDir     string
	LogDir       string
	Serial       bool
	Interpreter  string
	NoEvalOutput bool

	Outputs []string
}

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

type Position struct {
	filename string
	line     int
	offset   int
}

func (obj Position) String() string {
	return obj.position()
}

func (obj Position) position() string {
	if obj.filename == "" {
		return "<unknown>"
	}

	return fmt.Sprintf("%s:%d:%d", path.Base(obj.filename), obj.line, obj.offset)
}

type MapExpr struct {
	Position

	extends []Expression
	expr    []Expression
}

type MapValue struct {
	Position

	values map[string]Value
}

func (obj MapValue) jsonObject() any {
	result := make(map[string]any)
	for k, v := range obj.values {
		result[k] = v.jsonObject()
	}
	return result
}

func (obj MapValue) symlink(string) error {
	return fmt.Errorf("%s: unable to symlink object of type: %T", obj.position(), obj)
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

func (obj MapExpr) resolve(scope map[string]Value, ev *Evaluator) (Value, error) {
	values := make([]Value, len(obj.expr))
	err := parallelResolve(slices.All(obj.expr), func(k int, v Value) { values[k] = v }, scope, ev)
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
			return nil, fmt.Errorf("%s: expected string-key, got %T", key.position(), key)
		}
		res.values[keyStr.content] = value
	}

	for _, extname := range obj.extends {
		othervalue, err := extname.resolve(scope, ev)
		if err != nil {
			return nil, err
		}
		otherast, ok := othervalue.(MapValue)
		if !ok {
			return nil, fmt.Errorf("%s: unable to extend %T", obj.position(), othervalue)
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
	Position

	values []Expression
}

type ArrayValue struct {
	Position

	values []Value
}

func (obj ArrayValue) jsonObject() any {
	result := make([]any, len(obj.values))
	for i, v := range obj.values {
		result[i] = v.jsonObject()
	}
	return result
}

func (obj ArrayExpr) resolve(scope map[string]Value, ev *Evaluator) (Value, error) {
	res := ArrayValue{
		Position: obj.Position,
		values:   make([]Value, len(obj.values)),
	}
	return res, parallelResolve(slices.All(obj.values), func(i int, v Value) { res.values[i] = v }, scope, ev)
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

func (obj ArrayValue) symlink(resname string) error {
	var errs []error
	if resname != "" {
		for i, r := range obj.values {
			errs = append(errs, r.symlink(fmt.Sprintf("%s-%d", resname, i)))
		}
	}
	return errors.Join(errs...)
}

type StringValue struct {
	Position

	content string
}

func (obj StringValue) jsonObject() any {
	return obj.content
}

func (obj StringValue) hashValue(w io.Writer) {
	fmt.Fprintf(w, "%T", obj.content)
	fmt.Fprint(w, obj.content)
}

func (obj StringValue) encodeEnviron(root bool) (string, error) {
	return obj.content, nil
}

func (obj StringValue) symlink(string) error {
	return fmt.Errorf("%s: unable to symlink object of type: %T", obj.position(), obj)
}

type StringExpr struct {
	Position

	content []string
	interp  []Expression
}

func (obj StringExpr) jsonObject() any {
	return obj.content
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
			res.WriteString(intp.name)
		default:
			return nil, fmt.Errorf("%s: unable to interpolate %T", obj.position(), intp)
		}
	}
	return StringValue{
		obj.Position,
		res.String(),
	}, nil
}

func (obj StringExpr) hashValue(w io.Writer) {
	fmt.Fprintf(w, "%T", obj.content)
	fmt.Fprint(w, obj.content)
}

type NumberExpr struct {
	Position

	value float64
}

func (obj NumberExpr) jsonObject() any {
	return obj.value
}

func (obj NumberExpr) resolve(scope map[string]Value, ev *Evaluator) (Value, error) {
	return obj, nil
}

func (obj NumberExpr) hashValue(w io.Writer) {
	fmt.Fprintf(w, "%T", obj)
	fmt.Fprint(w, obj.value)
}

func (obj NumberExpr) encodeEnviron(root bool) (string, error) {
	return strconv.FormatFloat(obj.value, 'f', -1, 64), nil
}

func (obj NumberExpr) symlink(string) error {
	return fmt.Errorf("%s: unable to symlink object of type: %T", obj.position(), obj)
}

type BooleanExpr struct {
	Position

	value bool
}

func (obj BooleanExpr) jsonObject() any {
	return obj.value
}

func (obj BooleanExpr) resolve(scope map[string]Value, ev *Evaluator) (Value, error) {
	return obj, nil
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

func (obj BooleanExpr) symlink(string) error {
	return fmt.Errorf("%s: unable to symlink object of type: %T", obj.position(), obj)
}

type PathExpr struct {
	Position

	name string
}

func (obj PathExpr) jsonObject() any {
	return obj.name
}

func (obj PathExpr) resolve(scope map[string]Value, ev *Evaluator) (Value, error) {
	return obj, nil
}

func (obj PathExpr) hashValue(w io.Writer) {
	fmt.Fprintf(w, "%T", obj)
	fmt.Fprint(w, obj.name)
}

func (obj PathExpr) encodeEnviron(root bool) (string, error) {
	return obj.name, nil
}

func (obj PathExpr) symlink(resname string) error {
	if resname != "" {
		if stat, err := os.Lstat(resname); err == nil && (stat.Mode()&os.ModeType) != os.ModeSymlink {
			return fmt.Errorf("unable to make symlink: exist")
		}
		os.Remove(resname)
		return os.Symlink(obj.name, resname)
	}
	return nil
}

type VarExpr struct {
	Position

	name string
}

func (obj VarExpr) resolve(scope map[string]Value, ev *Evaluator) (Value, error) {
	val, ok := scope[obj.name]
	if !ok {
		return nil, fmt.Errorf("%s: not in scope: %s", obj.position(), obj.name)
	}
	return val, nil
}

func (obj VarExpr) hashValue(w io.Writer) {
	fmt.Fprintf(w, "%T", obj)
	fmt.Fprint(w, obj.name)
}

type AttributeExpr struct {
	Position

	base Expression
	name string
}

func (obj AttributeExpr) resolve(scope map[string]Value, ev *Evaluator) (Value, error) {
	val, err := obj.base.resolve(scope, ev)
	if err != nil {
		return nil, err
	}
	switch mapval := val.(type) {
	case MapValue:
		val, ok := mapval.values[obj.name]
		if !ok {
			return nil, fmt.Errorf("%s: map has no attribute %s", mapval.position(), obj.name)
		}
		return val, nil
	default:
		return nil, fmt.Errorf("%s: %T has no attributes", mapval.position(), mapval)
	}
}

func (obj AttributeExpr) hashValue(w io.Writer) {
	fmt.Fprintf(w, "%T", obj)
	fmt.Fprint(w, obj.name)
}

type IncludeExpr struct {
	Position

	name Expression
}

func (obj IncludeExpr) resolve(scope map[string]Value, ev *Evaluator) (Value, error) {
	pathAny, err := obj.name.resolve(scope, ev)
	if err != nil {
		return nil, err
	}
	path, ok := pathAny.(PathExpr)
	if !ok {
		return nil, fmt.Errorf("%s: unable to include non-path: %T", obj.position(), path)
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
	Position

	define map[string]Expression
	value  Expression
}

func (obj DefineExpr) jsonObject() any {
	return nil
}

func (obj DefineExpr) resolve(scope map[string]Value, ev *Evaluator) (Value, error) {
	newscope := maps.Clone(scope)
	var err error
	for k, v := range obj.define {
		newscope[k], err = v.resolve(scope, ev)
		if err != nil {
			return nil, err
		}
	}
	return obj.value.resolve(newscope, ev)
}

func (obj DefineExpr) hashValue(w io.Writer) {
	fmt.Fprintf(w, "%T", obj)
	obj.value.hashValue(w)
}
