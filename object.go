package main

import (
	"encoding/json"
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
	line     int
	offset   int
}

func GetScope[T Object](scope map[string]Object, name ObjectString) (result T, err error) {
	otherastAny, ok := scope[name.content]
	if !ok {
		return result, fmt.Errorf("%s: not in scope: %s", name.position(), name.content)
	}
	result, ok = otherastAny.(T)
	if !ok {
		return result, fmt.Errorf("%s: %s must be a %T, got %T", otherastAny.position(), name.content, result, otherastAny)
	}
	return result, nil
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

	content string
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

func (o ObjectBase) String() string {
	return o.position()
}

func (o ObjectBase) position() string {
	if o.filename == "" {
		return "<unknown>"
	}

	return fmt.Sprintf("%s:%d:%d", path.Base(o.filename), o.line, o.offset)
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
	return o.content
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
			var err error
			otherast, err = GetScope[ObjectMap](scope, extname)
			if err != nil {
				return nil, err
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

	err := parallelResolve(maps.All(o.values), func(k string, v Object) { o.values[k] = v }, scope, ev)
	if err != nil {
		return nil, err
	}

	if !ev.NoEvalOutput {
		_, hasoutput := o.values["@output"]
		_, hasbuilder := o.values["@builder"]
		if hasoutput || hasbuilder {
			return ev.output(o)
		}
	}
	if o.unwrap != nil {
		return o.unwrap, nil
	}
	return o, nil
}

func (o ObjectArray) resolve(scope map[string]Object, ev *Evaluator) (Object, error) {
	err := parallelResolve(slices.All(o.values), func(i int, v Object) { o.values[i] = v }, scope, ev)
	if err != nil {
		return nil, err
	}

	if len(o.values) > 0 {
		if head, ok := o.values[0].(ObjectString); ok && head.content == "@multiline" {
			var builder strings.Builder
			for i, elem := range o.values[1:] {
				selem, ok := elem.(ObjectString)
				if !ok {
					return nil, fmt.Errorf("%s: non-string in @multiline-array: %T", elem.position(), elem)
				}
				if i > 0 {
					builder.WriteByte('\n')
				}
				builder.WriteString(selem.content)
			}
			return ObjectString{o.ObjectBase, builder.String()}, nil
		}
	}
	return o, nil
}

const (
	InterpBegin = "{{"
	InterpEnd   = "}}"
)

func (obj ObjectString) resolve(scope map[string]Object, ev *Evaluator) (Object, error) {
	str := obj.content
	if str == "" {
		return obj, nil
	}
	if str[0] == '@' && str != "@multiline" {
		doEncode := false
		str = str[1:]
		if str[0] == '#' {
			doEncode = true
			str = str[1:]
		}
		varName := str
		replacement, err := GetScope[Object](scope, ObjectString{obj.ObjectBase, varName})
		if err != nil {
			return nil, err
		}
		if doEncode {
			enc, err := json.Marshal(replacement.jsonObject())
			if err != nil {
				return nil, err
			}
			obj.content = string(enc)
			return obj, nil
		}
		return replacement.resolve(scope, ev)
	}
	if !strings.Contains(str, InterpBegin) {
		/* no interpolation required */
		return obj, nil
	}
	var builder strings.Builder
	for len(str) > 0 {
		startIdx := strings.Index(str, InterpBegin)
		if startIdx == -1 {
			break
		}
		if startIdx > 0 && str[startIdx-1] == '\\' {
			/* escape sequence: write until, not including escaping `\` and `{{`, continue after `{{`  */
			builder.WriteString(str[:startIdx-1])
			builder.WriteString(InterpBegin)
			str = str[startIdx+len(InterpBegin):]
			continue
		}
		builder.WriteString(str[:startIdx])
		str = str[startIdx:]
		endIdx := strings.Index(str, InterpEnd)
		if endIdx == -1 {
			/* unmatched beginning */
			builder.WriteString(InterpBegin)
			str = str[len(InterpBegin):]
		}
		varName := str[len(InterpBegin):endIdx]
		doEncode := false
		if varName[0] == '#' {
			doEncode = true
			varName = varName[1:]
		}

		var replacementStr ObjectString

		if doEncode {
			replacement, err := GetScope[Object](scope, ObjectString{obj.ObjectBase, varName})
			if err != nil {
				return nil, err
			}
			enc, err := json.Marshal(replacement.jsonObject())
			if err != nil {
				return nil, err
			}
			replacementStr = ObjectString{content: string(enc)}
		} else {
			var err error
			replacementStr, err = GetScope[ObjectString](scope, ObjectString{obj.ObjectBase, varName})
			if err != nil {
				return nil, err
			}
		}

		builder.WriteString(replacementStr.content)
		str = str[endIdx+len(InterpEnd):]
	}
	builder.WriteString(str)
	obj.content = builder.String()
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
		obj.values[k].hashValue(w)
	}
}

func (obj ObjectArray) hashValue(w io.Writer) {
	fmt.Fprint(w, "list")
	for _, elem := range obj.values {
		elem.hashValue(w)
	}
}

func (obj ObjectString) hashValue(w io.Writer) {
	fmt.Fprintf(w, "%T", obj.content)
	fmt.Fprint(w, obj.content)
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
	return obj.content, nil
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
	fmt.Printf("%s\n", o.content)
	if resname != "" {
		if stat, err := os.Lstat(resname); err == nil && (stat.Mode()&os.ModeType) != os.ModeSymlink {
			return fmt.Errorf("unable to make symlink: exist")
		}
		os.Remove(resname)
		return os.Symlink(o.content, resname)
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
