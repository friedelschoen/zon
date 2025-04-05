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

type ObjectBase struct {
	parent   Object
	filename string
	offset   int64
}

type ObjectMap struct {
	ObjectBase

	defines  map[string]Object
	includes []ObjectString
	extends  []ObjectString
	values   map[string]Object
	unwrap   Object
}

type ObjectArray struct {
	ObjectBase

	values []Object
}

type ObjectString struct {
	ObjectBase

	value string
}

type ObjectNumber struct {
	ObjectBase

	value float64
}

type ObjectBoolean struct {
	ObjectBase

	value bool
}

type Object interface {
	jsonObject() any
	Parent() Object
	encodeEnviron(root bool) (string, error)
	hashValue(w io.Writer)
	position() string
	resolve(scope map[string]Object, ev *Evaluator) (Object, error)
	symlink(resultname string) error
}

func (o ObjectBase) Parent() Object {
	return o.parent
}

func (o ObjectBase) position() string {
	if o.filename == "" {
		return "<unknown>"
	}
	file, err := os.Open(o.filename)
	if err != nil {
		return fmt.Sprintf("%s:1:%d", o.filename, o.offset)
	}
	defer file.Close()

	var (
		line       = 1
		lineOffset = int64(0)
		buf        = make([]byte, 4096)
		total      = int64(0)
	)

	for {
		n, err := file.Read(buf)
		if n == 0 && err != nil {
			break
		}
		for i := range n {
			if total == o.offset {
				return fmt.Sprintf("%s:%d:%d", path.Base(o.filename), line, int(o.offset-lineOffset))
			}
			if buf[i] == '\n' {
				line++
				lineOffset = total + 1
			}
			total++
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "<unknown>"
		}
	}

	// If offset is beyond EOF, fallback to last known position
	return fmt.Sprintf("%s:%d:%d", path.Base(o.filename), line, int(o.offset-lineOffset))
}

func (o ObjectBase) symlink(string) error {
	return fmt.Errorf("%s: unable to symlink object of type: %T", o.position(), o)
}

func (o ObjectMap) jsonObject() any {
	result := make(map[string]any)
	for k, v := range o.values {
		result[k] = v.jsonObject()
	}
	if len(o.defines) > 0 {
		definesmap := make(map[string]any)
		for k, v := range o.defines {
			definesmap[k] = v.jsonObject()
		}
		result["@define"] = definesmap
	}
	if len(o.includes) > 0 {
		incllist := make([]any, len(o.includes))
		for i, v := range o.includes {
			incllist[i] = v.jsonObject()
		}
		result["@include"] = incllist
	}
	if len(o.extends) > 0 {
		explist := make([]any, len(o.extends))
		for i, v := range o.extends {
			explist[i] = v.jsonObject()
		}
		result["@expand"] = explist
	}
	return result
}

func (o ObjectArray) jsonObject() any {
	result := make([]any, len(o.values))
	for i, v := range o.values {
		result[i] = v.jsonObject()
	}
	return result
}

func (o ObjectString) jsonObject() any {
	return o.value
}

func (o ObjectNumber) jsonObject() any {
	return o.value
}

func (o ObjectBoolean) jsonObject() any {
	return o.value
}

func copyMapKeep[K comparable, V Object](dest map[K]V, source map[K]V) {
	for k, v := range source {
		_, ok := dest[k]
		if !ok {
			dest[k] = v
		}
	}
}

func parallelResolve[K any](values iter.Seq2[K, Object], set func(K, Object), scope map[string]Object, ev *Evaluator) error {
	if !ev.Serial {
		var (
			wg   sync.WaitGroup
			mu   sync.Mutex
			errs []error
		)
		for k, v := range values {
			wg.Add(1)
			go func() {
				defer wg.Done()
				val, err := v.resolve(scope, ev)
				mu.Lock()
				set(k, val)
				errs = append(errs, err)
				mu.Unlock()
			}()
		}
		wg.Wait()
		if err := errors.Join(errs...); err != nil {
			return err
		}
	} else {
		for k, v := range values {
			val, err := v.resolve(scope, ev)
			if err != nil {
				return err
			}
			set(k, val)
		}
	}
	return nil
}

func (o ObjectMap) resolve(scope map[string]Object, ev *Evaluator) (Object, error) {
	scope = maps.Clone(scope)
	maps.Copy(scope, o.defines)
	for len(o.includes) > 0 || len(o.extends) > 0 {
		var otherast ObjectMap
		if len(o.includes) > 0 {
			inclpath := o.includes[0]
			o.includes = o.includes[1:]
			otherastAny, err := parseFile(inclpath, o)
			if err != nil {
				return nil, err
			}
			var ok bool
			otherast, ok = otherastAny.(ObjectMap)
			if !ok {
				return nil, fmt.Errorf("%s: unable to include non-map", inclpath.position())
			}
		} else if len(o.extends) > 0 {
			extname := o.extends[0]
			o.extends = o.extends[1:]
			otherastAny, ok := scope[extname.value]
			if !ok {
				return nil, fmt.Errorf("%s: not in scope: %s", extname.position(), extname.value)
			}
			otherast, ok = otherastAny.(ObjectMap)
			if !ok {
				return nil, fmt.Errorf("%s: unable to expand non-map", extname.position())
			}
		}
		copyMapKeep(o.defines, otherast.defines)
		o.includes = append(o.includes, otherast.includes...)
		o.extends = append(o.extends, otherast.extends...)

		if len(otherast.defines) > 0 {
			scope = maps.Clone(scope)
			copyMapKeep(scope, otherast.defines)
		}
		copyMapKeep(o.values, otherast.values)
	}

	parallelResolve(maps.All(o.values), func(k string, v Object) { o.values[k] = v }, scope, ev)

	if _, ok := o.values["@output"]; ok && !ev.NoEvalOutput {
		return ev.output(o)
	}
	if o.unwrap != nil {
		return o.unwrap, nil
	}
	return o, nil
}

func (o ObjectArray) resolve(scope map[string]Object, ev *Evaluator) (Object, error) {
	parallelResolve(slices.All(o.values), func(k int, v Object) { o.values[k] = v }, scope, ev)

	if len(o.values) > 0 {
		if head, ok := o.values[0].(ObjectString); ok && head.value == "@multiline" {
			var builder strings.Builder
			for i, elem := range o.values[1:] {
				selem, ok := elem.(ObjectString)
				if !ok {
					return nil, fmt.Errorf("%s: non-string in @multiline-array: %T", elem.position(), elem)
				}
				if i > 0 {
					builder.WriteByte('\n')
				}
				builder.WriteString(selem.value)
			}
			return ObjectString{o.ObjectBase, builder.String()}, nil
		}
	}
	return o, nil
}

func (obj ObjectString) resolve(scope map[string]Object, ev *Evaluator) (Object, error) {
	str := obj.value
	if str == "" {
		return obj, nil
	}
	if str[0] == '@' && str != "@multiline" {
		varName := str[1:]
		replacement, found := scope[varName]
		if !found {
			return nil, fmt.Errorf("%s: undefined variable: %s", obj.position(), varName)
		}
		return replacement.resolve(scope, ev)
	}
	var builder strings.Builder
	for len(str) > 0 {
		startIdx := strings.Index(str, "{{")
		if startIdx == -1 {
			break
		}
		builder.WriteString(str[:startIdx])
		str = str[startIdx:]
		endIdx := strings.Index(str, "}}")
		if endIdx == -1 {
			return nil, fmt.Errorf("%s: unmatched {{ in string: %s", obj.position(), str)
		}
		varName := str[2:endIdx]
		replacement, found := scope[varName]
		if !found {
			return nil, fmt.Errorf("%s: undefined variable: %s", obj.position(), varName)
		}
		replacementStr, valid := replacement.(ObjectString)
		if !valid {
			return nil, fmt.Errorf("%s: variable %s must be a string, got %T", obj.position(), varName, replacement)
		}
		builder.WriteString(replacementStr.value)
		str = str[endIdx+2:]
	}
	builder.WriteString(str)
	obj.value = builder.String()
	return obj, nil
}

func (o ObjectNumber) resolve(scope map[string]Object, ev *Evaluator) (Object, error) {
	return o, nil
}

func (o ObjectBoolean) resolve(scope map[string]Object, ev *Evaluator) (Object, error) {
	return o, nil
}

func (obj ObjectMap) hashValue(w io.Writer) {
	fmt.Fprint(w, "map")
	keys := slices.Collect(maps.Keys(obj.values))
	slices.Sort(keys)
	for _, k := range keys {
		w.Write([]byte(k))
		obj.values[k].hashValue(w)
	}
}

func (obj ObjectArray) hashValue(w io.Writer) {
	fmt.Fprint(w, "list")
	for i, elem := range obj.values {
		fmt.Fprint(w, i)
		elem.hashValue(w)
	}
}

func (obj ObjectString) hashValue(w io.Writer) {
	fmt.Fprintf(w, "%T", obj.value)
	fmt.Fprint(w, obj.value)
}

func (obj ObjectNumber) hashValue(w io.Writer) {
	fmt.Fprintf(w, "%T", obj.value)
	fmt.Fprint(w, obj.value)
}

func (obj ObjectBoolean) hashValue(w io.Writer) {
	fmt.Fprintf(w, "%T", obj.value)
	fmt.Fprint(w, obj.value)
}

func (obj ObjectMap) encodeEnviron(root bool) (string, error) {
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

func (obj ObjectArray) encodeEnviron(root bool) (string, error) {
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

func (obj ObjectString) encodeEnviron(root bool) (string, error) {
	return obj.value, nil
}

func (obj ObjectNumber) encodeEnviron(root bool) (string, error) {
	return strconv.FormatFloat(obj.value, 'f', -1, 64), nil
}

func (obj ObjectBoolean) encodeEnviron(root bool) (string, error) {
	if obj.value {
		return "1", nil
	}
	return "0", nil
}

func (o ObjectString) symlink(resname string) error {
	fmt.Printf("%s\n", o.value)
	if resname != "" {
		if stat, err := os.Lstat(resname); err == nil && (stat.Mode()&os.ModeType) != os.ModeSymlink {
			return fmt.Errorf("unable to make symlink: exist\n")
		}
		os.Remove(resname)
		return os.Symlink(o.value, resname)
	}
	return nil
}

func (o ObjectArray) symlink(resname string) error {
	var errs []error
	if resname != "" {
		for i, r := range o.values {
			errs = append(errs, r.symlink(fmt.Sprintf("%s-%d", resname, i)))
		}
	}
	return errors.Join(errs...)
}
