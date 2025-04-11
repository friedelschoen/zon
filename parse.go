package main

import (
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

type Parser struct {
	s        *Scanner
	cwd      string
	filename string
}

func (p *Parser) base(parent Object) ObjectBase {
	return ObjectBase{
		parent:   parent,
		filename: p.filename,
		offset:   p.s.Start,
		line:     p.s.Linenr,
	}
}

func (p *Parser) expect(toks ...Token) error {
	if !slices.Contains(toks, p.s.Token) {
		var expected strings.Builder
		for i, t := range toks {
			if i > 0 {
				expected.WriteString(", ")
			}
			expected.WriteString(t.String())
		}
		return fmt.Errorf("%s:%d:%d-%d: expected %s, got '%s' (type %v)", path.Base(p.filename), p.s.Linenr, p.s.Start+1, p.s.End+1, expected.String(), p.s.Text(), p.s.Token)
	}
	err := p.s.Next()
	if err != nil {
		return err
	}
	return nil
}

func (p *Parser) parseString(parent Object) (Object, error) {
	obj := ObjectString{
		ObjectBase: p.base(parent),
	}

	var builder strings.Builder
	for {
		err := p.s.Next()
		if err != nil {
			return nil, err
		}

		switch p.s.Token {
		case TokenStringChar:
			builder.WriteString(p.s.Text())
		case TokenStringEscape:
			text := p.s.Text()
			/* text is including \, so we want the second char */
			switch text[1] {
			case '"':
				builder.WriteByte('"')
			case '\\':
				builder.WriteByte('\\')
			case 'b':
				builder.WriteByte('\b')
			case 'f':
				builder.WriteByte('\f')
			case 'n':
				builder.WriteByte('\n')
			case 'r':
				builder.WriteByte('\r')
			case 't':
				builder.WriteByte('\t')
			case 'u':
				code, err := strconv.ParseInt(text[2:6], 16, 16)
				if err != nil {
					return nil, err
				}
				builder.WriteRune(rune(code))
			}
		case TokenStringEnd:
			goto exit
		default:
			err := p.expect(TokenStringChar, TokenStringEnd)
			return nil, err
		}
	}
exit:
	err := p.s.Next()
	if err != nil {
		return nil, err
	}

	obj.content = builder.String()
	if strings.HasPrefix(obj.content, "./") {
		obj.content = path.Join(p.cwd, obj.content)
	}
	return obj, nil
}

func (p *Parser) parseValue(parent Object) (Object, error) {
	switch p.s.Token {
	case TokenLBrace:
		return p.parseMap(parent)
	case TokenLBracket:
		return p.parseArray(parent)
	case TokenString:
		return p.parseString(parent)
	case TokenInteger:
		val, _ := strconv.ParseFloat(p.s.Text(), 64)
		obj := ObjectNumber{
			p.base(parent),
			val,
		}
		if err := p.s.Next(); err != nil {
			return nil, err
		}
		return obj, nil
	case TokenTrue, TokenFalse:
		obj := ObjectBoolean{
			p.base(parent),
			p.s.Token == TokenTrue,
		}
		if err := p.s.Next(); err != nil {
			return nil, err
		}
		return obj, nil
	}
	return nil, fmt.Errorf("%s: invalid token: %v", p.base(nil).position(), p.s.Token)
}

func (p *Parser) parseMap(parent Object) (Object, error) {
	obj := ObjectMap{
		ObjectBase: p.base(parent),
		defines:    make(map[string]Object),
		values:     make(map[string]Object),
	}

	p.s.Token = TokenComma
	for p.s.Token == TokenComma {
		err := p.s.Next()
		if err != nil {
			return nil, err
		}
		key, err := p.parseValue(obj)
		if err != nil {
			return nil, err
		}
		keyStr, ok := key.(ObjectString)
		if !ok {
			return nil, fmt.Errorf("%s: expected string-key, got %T", obj.position(), key)
		}
		if err := p.expect(TokenColon); err != nil {
			return nil, err
		}
		value, err := p.parseValue(obj)
		if err != nil {
			return nil, err
		}
		switch keyStr.content {
		case "@define":
			defs, ok := value.(ObjectMap)
			if !ok {
				return nil, fmt.Errorf("%s: @define must be a map, got %T", value.position(), value)
			}
			maps.Copy(obj.defines, defs.values)
		case "@expand":
			str, ok := value.(ObjectString)
			if !ok {
				return nil, fmt.Errorf("%s: @expand variable must be string, got %T", value.position(), value)
			}
			obj.extends = append(obj.extends, str)
		case "@include":
			str, ok := value.(ObjectString)
			if !ok {
				return nil, fmt.Errorf("%s: @include must be a string, got %T", value.position(), value)
			}
			obj.includes = append(obj.includes, str)
		default:
			obj.values[keyStr.content] = value
		}
	}

	if err := p.expect(TokenRBrace); err != nil {
		return nil, err
	}

	if attr, ok := obj.values["@"]; ok {
		if len(obj.values) != 1 {
			return nil, fmt.Errorf("%s: map with @ has more than 1 value", obj.values["@"].position())
		}
		obj.unwrap = attr
	}

	return obj, nil
}

func (p *Parser) parseArray(parent Object) (Object, error) {
	obj := ObjectArray{
		ObjectBase: p.base(parent),
	}

	p.s.Token = TokenComma
	for p.s.Token == TokenComma {
		err := p.s.Next()
		if err != nil {
			return nil, err
		}
		value, err := p.parseValue(obj)
		if err != nil {
			return nil, err
		}
		obj.values = append(obj.values, value)
	}

	if err := p.expect(TokenRBracket); err != nil {
		return nil, err
	}

	return obj, nil
}

func parseFile(filename ObjectString, parent Object) (Object, error) {
	file, err := os.Open(filename.content)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to open file %s: %w", filename.position(), filename.content, err)
	}
	defer file.Close()
	abs, _ := filepath.Abs(filename.content)

	scanner := NewScanner(file)
	err = scanner.Next()
	if err != nil {
		return nil, err
	}
	parser := Parser{s: scanner, cwd: path.Dir(abs), filename: filename.content}
	val, err := parser.parseValue(parent)
	if err != nil {
		return nil, err
	}
	if err := parser.expect(TokenEOF); err != nil {
		return nil, err
	}
	return val, nil
}
