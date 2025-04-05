package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type Parser struct {
	dec      *json.Decoder
	cwd      string
	filename string
}

func (p *Parser) base(parent Object) ObjectBase {
	return ObjectBase{
		parent:   parent,
		filename: p.filename,
		offset:   p.dec.InputOffset(),
	}
}

func (p *Parser) parseValue(parent Object) (Object, error) {
	token, err := p.dec.Token()
	if err != nil {
		return nil, err
	}

	switch tok := token.(type) {
	case json.Delim:
		switch tok {
		case '{':
			return p.parseMap(parent)
		case '[':
			return p.parseArray(parent)
		default:
			return nil, nil
		}
	case string:
		if strings.HasPrefix(tok, "./") {
			tok = path.Join(p.cwd, tok)
		}
		return ObjectString{
			p.base(parent),
			tok,
		}, nil
	case float64:
		return ObjectNumber{
			p.base(parent),
			tok,
		}, nil
	case bool:
		return ObjectBoolean{
			p.base(parent),
			tok,
		}, nil
	}
	return nil, fmt.Errorf("%s: invalid type %T", p.base(nil).position(), token)
}

func (p *Parser) parseMap(parent Object) (Object, error) {
	result := ObjectMap{
		ObjectBase: p.base(parent),
		defines:    make(map[string]Object),
		values:     make(map[string]Object),
	}

	for p.dec.More() {
		key, err := p.dec.Token()
		if err != nil {
			return nil, err
		}
		keyStr, ok := key.(string)
		if !ok {
			return nil, fmt.Errorf("%s: expected string-key, got %T", result.position(), key)
		}
		value, err := p.parseValue(result)
		if err != nil {
			return nil, err
		}
		switch keyStr {
		case "@define":
			defs, ok := value.(ObjectMap)
			if !ok {
				return nil, fmt.Errorf("%s: @define must be a map, got %T", value.position(), value)
			}
			maps.Copy(result.defines, defs.values)
		case "@expand":
			str, ok := value.(ObjectString)
			if !ok {
				return nil, fmt.Errorf("%s: @expand variable must be string, got %T", value.position(), value)
			}
			result.extends = append(result.extends, str)
		case "@include":
			str, ok := value.(ObjectString)
			if !ok {
				return nil, fmt.Errorf("%s: @include must be a string, got %T", value.position(), value)
			}
			result.includes = append(result.includes, str)
		default:
			result.values[keyStr] = value
		}
	}
	_, err := p.dec.Token() // Consume '}'

	if attr, ok := result.values["@"]; ok {
		if len(result.values) != 1 {
			return nil, fmt.Errorf("%s: map with @ has more than 1 value", result.values["@"].position())
		}
		result.unwrap = attr
	}

	return result, err
}

func (p *Parser) parseArray(parent Object) (Object, error) {
	obj := ObjectArray{
		ObjectBase: p.base(parent),
	}
	for p.dec.More() {
		value, err := p.parseValue(parent)
		if err != nil {
			return nil, err
		}
		obj.values = append(obj.values, value)
	}
	_, err := p.dec.Token() // Consume ']'
	return obj, err
}

func parseFile(filename ObjectString, parent Object) (Object, error) {
	file, err := os.Open(filename.value)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to open file %s: %w", filename.position(), filename.value, err)
	}
	defer file.Close()
	abs, _ := filepath.Abs(filename.value)
	parser := Parser{dec: json.NewDecoder(file), cwd: path.Dir(abs), filename: filename.value}
	return parser.parseValue(parent)
}
