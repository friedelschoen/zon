package types

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type StringValue struct {
	Position

	Content string
}

func (obj StringValue) JSON() any {
	return obj.Content
}

func (obj StringValue) encodeEnviron(root bool) (string, error) {
	return obj.Content, nil
}

func (obj StringValue) Link(string) error {
	return fmt.Errorf("%s: unable to symlink object of type: %T", obj.Pos(), obj)
}

func (obj StringValue) Boolean() (bool, error) {
	return len(obj.Content) > 0, nil
}

type StringExpr struct {
	Position

	Content []string
	Interp  []Expression
}

func (obj StringExpr) JSON() any {
	return obj.Content
}

func (obj StringExpr) Resolve(scope Scope, ev *Evaluator) (Value, []PathExpr, error) {
	var res strings.Builder
	var deps []PathExpr
	for i := range obj.Content {
		res.WriteString(obj.Content[i])
		if obj.Interp[i] == nil {
			continue
		}
		intp, paths, err := obj.Interp[i].Resolve(scope, ev)
		if err != nil {
			return nil, nil, err
		}
		deps = append(deps, paths...)
		switch intp := intp.(type) {
		case StringValue:
			res.WriteString(intp.Content)
		case PathExpr:
			res.WriteString(intp.Name)
		default:
			return nil, nil, fmt.Errorf("%s: unable to interpolate %T", obj.Pos(), intp)
		}
	}
	return StringValue{
		obj.Position,
		res.String(),
	}, deps, nil
}

func (obj StringExpr) hashValue(w io.Writer) {
	fmt.Fprintf(w, "string")
	for i := range obj.Content {
		fmt.Fprint(w, obj.Content[i])
		if obj.Interp[i] != nil {
			obj.Interp[i].hashValue(w)
		}
	}
}

func StringConstant(content string, origin string) StringExpr {
	return StringExpr{Position: Position{Filename: origin}, Content: []string{content}, Interp: []Expression{nil}}
}

type NumberExpr struct {
	Position

	Value float64
}

func (obj NumberExpr) JSON() any {
	return obj.Value
}

func (obj NumberExpr) Resolve(scope Scope, ev *Evaluator) (Value, []PathExpr, error) {
	return obj, nil, nil
}

func (obj NumberExpr) hashValue(w io.Writer) {
	fmt.Fprintf(w, "number")
	fmt.Fprint(w, obj.Value)
}

func (obj NumberExpr) encodeEnviron(root bool) (string, error) {
	return strconv.FormatFloat(obj.Value, 'f', -1, 64), nil
}

func (obj NumberExpr) Link(string) error {
	return fmt.Errorf("%s: unable to symlink object of type: %T", obj.Pos(), obj)
}

func (obj NumberExpr) Boolean() (bool, error) {
	return obj.Value != 0, nil
}

type BooleanExpr struct {
	Position

	Value bool
}

func (obj BooleanExpr) JSON() any {
	return obj.Value
}

func (obj BooleanExpr) Resolve(scope Scope, ev *Evaluator) (Value, []PathExpr, error) {
	return obj, nil, nil
}

func (obj BooleanExpr) Boolean() (bool, error) {
	return obj.Value, nil
}

func (obj BooleanExpr) hashValue(w io.Writer) {
	fmt.Fprintf(w, "boolean")
	fmt.Fprint(w, obj.Value)
}

func (obj BooleanExpr) encodeEnviron(root bool) (string, error) {
	if obj.Value {
		return "1", nil
	}
	return "0", nil
}

func (obj BooleanExpr) Link(string) error {
	return fmt.Errorf("%s: unable to symlink object of type: %T", obj.Pos(), obj)
}

type PathExpr struct {
	Position

	Name    string
	Depends []PathExpr
}

func (obj PathExpr) JSON() any {
	return obj.Name
}

func (obj PathExpr) Resolve(scope Scope, ev *Evaluator) (Value, []PathExpr, error) {
	return obj, nil, nil
}

func (obj PathExpr) Boolean() (bool, error) {
	return true, nil
}

func (obj PathExpr) hashValue(w io.Writer) {
	fmt.Fprintf(w, "%T", obj)
	fmt.Fprint(w, obj.Name)
	s, err := os.Stat(obj.Name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to stat %s: %v\n", obj.Name, err)
	} else {
		fmt.Fprint(w, s.ModTime(), s.Mode())
	}
	for _, dep := range obj.Depends {
		dep.hashValue(w)
	}
}

func (obj PathExpr) encodeEnviron(root bool) (string, error) {
	return obj.Name, nil
}

func (obj PathExpr) Link(resname string) error {
	if resname != "" {
		if stat, err := os.Lstat(resname); err == nil && (stat.Mode()&os.ModeType) != os.ModeSymlink {
			return fmt.Errorf("unable to make symlink: exist")
		}
		os.Remove(resname)
		return os.Symlink(obj.Name, resname)
	}
	return nil
}
