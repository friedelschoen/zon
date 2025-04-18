package main

import (
	"fmt"
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

func (p *Parser) base() Position {
	return Position{
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
	if err := p.s.Next(); err != nil {
		return err
	}
	return nil
}

func (p *Parser) parseString() (Expression, error) {
	obj := StringExpr{
		Position: p.base(),
	}

	var builder strings.Builder
	for {
		if err := p.s.Next(); err != nil {
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
		case TokenInterp:
			obj.content = append(obj.content, builder.String())
			builder.Reset()
			if err := p.s.Next(); err != nil {
				return nil, err
			}
			intp, err := p.parseValue()
			if err != nil {
				return nil, err
			}
			obj.interp = append(obj.interp, intp)
		default:
			err := p.expect(TokenStringChar, TokenStringEnd, TokenInterp)
			return nil, err
		}
	}
exit:
	if err := p.s.Next(); err != nil {
		return nil, err
	}

	obj.content = append(obj.content, builder.String())
	obj.interp = append(obj.interp, nil)
	return obj, nil
}

func (p *Parser) parseBase() (Expression, error) {
	switch p.s.Token {
	case TokenLBrace:
		return p.parseMap()
	case TokenLBracket:
		return p.parseArray()
	case TokenString:
		return p.parseString()
	case TokenIdent:
		return p.parseVar()
	case TokenInclude:
		return p.parseInclude()
	case TokenOutput:
		return p.parseOutput()
	case TokenLParen:
		return p.parseEnclosed()
	case TokenInteger:
		val, _ := strconv.ParseFloat(p.s.Text(), 64)
		obj := NumberExpr{
			p.base(),
			val,
		}
		if err := p.s.Next(); err != nil {
			return nil, err
		}
		return obj, nil
	case TokenPath:
		obj := PathExpr{
			p.base(),
			p.s.Text(),
		}
		if obj.name[0] != '/' {
			obj.name = path.Clean(p.cwd + "/" + obj.name)
		}
		if err := p.s.Next(); err != nil {
			return nil, err
		}
		return obj, nil
	case TokenTrue, TokenFalse:
		obj := BooleanExpr{
			p.base(),
			p.s.Token == TokenTrue,
		}
		if err := p.s.Next(); err != nil {
			return nil, err
		}
		return obj, nil
	case TokenLet:
		return p.parseDefinition()
	}
	return nil, fmt.Errorf("%s: invalid token: %v", p.base().position(), p.s.Token)
}

func (p *Parser) parseValue() (Expression, error) {
	base, err := p.parseBase()
	if err != nil {
		return nil, err
	}

	for p.s.Token == TokenDot {
		if err := p.s.Next(); err != nil {
			return nil, err
		}
		if p.s.Token != TokenIdent {
			return nil, p.expect(TokenIdent)
		}
		base = AttributeExpr{
			Position: p.base(),
			base:     base,
			name:     p.s.Text(),
		}
		if err := p.s.Next(); err != nil {
			return nil, err
		}
	}
	return base, nil
}

func (p *Parser) parseMap() (Expression, error) {
	obj := MapExpr{
		Position: p.base(),
	}

	p.s.Token = TokenComma
	for p.s.Token == TokenComma {
		if err := p.s.Next(); err != nil {
			return nil, err
		}
		if p.s.Token == TokenWith {
			if err := p.s.Next(); err != nil {
				return nil, err
			}
			val, err := p.parseValue()
			if err != nil {
				return nil, err
			}
			obj.extends = append(obj.extends, val)
		} else {
			key, err := p.parseValue()
			if err != nil {
				return nil, err
			}
			obj.expr = append(obj.expr, key)
			if err := p.expect(TokenColon); err != nil {
				return nil, err
			}
			value, err := p.parseValue()
			if err != nil {
				return nil, err
			}
			obj.expr = append(obj.expr, value)
		}
	}

	if err := p.expect(TokenRBrace); err != nil {
		return nil, err
	}

	return obj, nil
}

func (p *Parser) parseDefinition() (Expression, error) {
	obj := DefineExpr{
		Position: p.base(),
		define:   make(map[string]Expression),
	}

	p.s.Token = TokenComma
	for p.s.Token == TokenComma {
		if err := p.s.Next(); err != nil {
			return nil, err
		}
		keyStr := p.s.Text()
		if err := p.expect(TokenIdent); err != nil {
			return nil, err
		}
		if err := p.expect(TokenEquals); err != nil {
			return nil, err
		}
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		obj.define[keyStr] = value
	}

	err := p.expect(TokenIn)
	if err != nil {
		return nil, err
	}

	obj.value, err = p.parseValue()
	if err != nil {
		return nil, err
	}

	return obj, nil
}

func (p *Parser) parseArray() (Expression, error) {
	obj := ArrayExpr{
		Position: p.base(),
	}

	p.s.Token = TokenComma
	for p.s.Token == TokenComma {
		if err := p.s.Next(); err != nil {
			return nil, err
		}
		value, err := p.parseValue()
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

func (p *Parser) parseVar() (Expression, error) {
	obj := VarExpr{
		p.base(),
		p.s.Text(),
	}

	if err := p.s.Next(); err != nil {
		return nil, err
	}

	return obj, nil
}

func (p *Parser) parseInclude() (Expression, error) {
	obj := IncludeExpr{
		p.base(),
		nil,
	}
	if err := p.s.Next(); err != nil {
		return nil, err
	}
	var err error
	obj.name, err = p.parseValue()
	return obj, err
}

func (p *Parser) parseOutput() (Expression, error) {
	obj := OutputExpr{
		p.base(),
		nil,
	}
	if err := p.s.Next(); err != nil {
		return nil, err
	}
	var err error
	obj.attrs, err = p.parseValue()
	return obj, err
}

func (p *Parser) parseEnclosed() (Expression, error) {
	if err := p.s.Next(); err != nil {
		return nil, err
	}
	obj, err := p.parseValue()
	if err != nil {
		return nil, err
	}
	err = p.expect(TokenRParen)
	if err != nil {
		return nil, err
	}
	return obj, err
}

func parseFile(filename PathExpr) (Expression, error) {
	file, err := os.Open(filename.name)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to open file %s: %w", filename.position(), filename.name, err)
	}
	defer file.Close()
	abs, _ := filepath.Abs(filename.name)

	scanner := NewScanner(file)
	err = scanner.Next()
	if err != nil {
		return nil, err
	}
	parser := Parser{s: scanner, cwd: path.Dir(abs), filename: filename.name}
	val, err := parser.parseValue()
	if err != nil {
		return nil, err
	}
	if err := parser.expect(TokenEOF); err != nil {
		return nil, err
	}
	return val, nil
}
