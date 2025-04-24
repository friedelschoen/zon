package parser

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/friedelschoen/zon/types"
)

type Parser struct {
	s        *Scanner
	cwd      string
	filename string
}

func (p *Parser) base() types.Position {
	return types.Position{
		Filename: p.filename,
		Offset:   p.s.Start,
		Line:     p.s.Linenr,
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

func (p *Parser) parseString() (types.Expression, error) {
	obj := types.StringExpr{
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
			obj.Content = append(obj.Content, builder.String())
			builder.Reset()
			if err := p.s.Next(); err != nil {
				return nil, err
			}
			intp, err := p.parseValue()
			if err != nil {
				return nil, err
			}
			obj.Interp = append(obj.Interp, intp)
		default:
			err := p.expect(TokenStringChar, TokenStringEnd, TokenInterp)
			return nil, err
		}
	}
exit:
	if err := p.s.Next(); err != nil {
		return nil, err
	}

	obj.Content = append(obj.Content, builder.String())
	obj.Interp = append(obj.Interp, nil)
	return obj, nil
}

func (p *Parser) parseBase() (types.Expression, error) {
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
	case TokenNumber:
		val, _ := strconv.ParseFloat(p.s.Text(), 64)
		obj := types.NumberExpr{
			Position: p.base(),
			Value:    val,
		}
		if err := p.s.Next(); err != nil {
			return nil, err
		}
		return obj, nil
	case TokenPath:
		obj := types.PathExpr{
			Position: p.base(),
			Name:     p.s.Text(),
		}
		if obj.Name[0] != '/' {
			obj.Name = path.Clean(p.cwd + "/" + obj.Name)
		}
		if err := p.s.Next(); err != nil {
			return nil, err
		}
		return obj, nil
	case TokenTrue, TokenFalse:
		obj := types.BooleanExpr{
			Position: p.base(),
			Value:    p.s.Token == TokenTrue,
		}
		if err := p.s.Next(); err != nil {
			return nil, err
		}
		return obj, nil
	case TokenLet:
		return p.parseDefinition()
	}
	return nil, fmt.Errorf("%s: invalid token: %v", p.base(), p.s.Token)
}

func (p *Parser) parseValue() (types.Expression, error) {
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
		base = types.AttributeExpr{
			Position: p.base(),
			Base:     base,
			Name:     p.s.Text(),
		}
		if err := p.s.Next(); err != nil {
			return nil, err
		}
	}
	return base, nil
}

func (p *Parser) parseMap() (types.Expression, error) {
	obj := types.MapExpr{
		Position: p.base(),
	}

	if err := p.expect(TokenLBrace); err != nil {
		return nil, err
	}

	for p.s.Token != TokenRBrace {
		if p.s.Token == TokenWith {
			if err := p.s.Next(); err != nil {
				return nil, err
			}
			val, err := p.parseValue()
			if err != nil {
				return nil, err
			}
			obj.Extends = append(obj.Extends, val)
		} else {
			key, err := p.parseValue()
			if err != nil {
				return nil, err
			}
			obj.Exprs = append(obj.Exprs, key)
			if err := p.expect(TokenColon); err != nil {
				return nil, err
			}
			value, err := p.parseValue()
			if err != nil {
				return nil, err
			}
			obj.Exprs = append(obj.Exprs, value)
		}
		if err := p.expect(TokenComma); err != nil {
			break
		}
	}

	if err := p.expect(TokenRBrace); err != nil {
		return nil, err
	}

	return obj, nil
}

func (p *Parser) parseDefinition() (types.Expression, error) {
	obj := types.DefineExpr{
		Position: p.base(),
	}

	err := p.expect(TokenLet)
	if err != nil {
		return nil, err
	}

	for p.s.Token != TokenIn {
		keyStr := p.s.Text()
		if err := p.expect(TokenIdent); err != nil {
			return nil, err
		}
		var args []string
		if p.s.Token == TokenIdent {
			for p.s.Token != TokenEquals {
				txt := p.s.Text()
				if err := p.expect(TokenIdent); err != nil {
					return nil, err
				}
				args = append(args, txt)
				if err := p.expect(TokenComma); err != nil {
					break
				}
			}
		}
		if err := p.expect(TokenEquals); err != nil {
			return nil, err
		}
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		obj.Define = append(obj.Define, types.Definition{Name: keyStr, Expr: value, Args: args})
		err = p.expect(TokenComma)
		if err != nil {
			break
		}
	}

	err = p.expect(TokenIn)
	if err != nil {
		return nil, err
	}

	obj.Expr, err = p.parseValue()
	if err != nil {
		return nil, err
	}

	return obj, nil
}

func (p *Parser) parseArray() (types.Expression, error) {
	obj := types.ArrayExpr{
		Position: p.base(),
	}

	err := p.expect(TokenLBracket)
	if err != nil {
		return nil, err
	}

	for p.s.Token != TokenRBracket {
		value, err := p.parseValue()
		if err != nil {
			return nil, err
		}
		obj.Exprs = append(obj.Exprs, value)
		if err := p.expect(TokenComma); err != nil {
			break
		}
	}

	if err := p.expect(TokenRBracket); err != nil {
		return nil, err
	}

	return obj, nil
}

func (p *Parser) parseVar() (types.Expression, error) {
	obj := types.VarExpr{
		Position: p.base(),
		Name:     p.s.Text(),
	}
	if err := p.expect(TokenIdent); err != nil {
		return nil, err
	}
	if p.s.Token == TokenLParen {
		if err := p.s.Next(); err != nil {
			return nil, err
		}
		for p.s.Token != TokenRParen {
			expr, err := p.parseValue()
			if err != nil {
				return nil, err
			}
			obj.Args = append(obj.Args, expr)
			if err := p.expect(TokenComma); err != nil {
				break
			}
		}
		if err := p.expect(TokenRParen); err != nil {
			return nil, err
		}
	}

	return obj, nil
}

func (p *Parser) parseInclude() (types.Expression, error) {
	obj := types.IncludeExpr{
		Position: p.base(),
		Name:     nil,
	}
	if err := p.s.Next(); err != nil {
		return nil, err
	}
	var err error
	obj.Name, err = p.parseValue()
	return obj, err
}

func (p *Parser) parseOutput() (types.Expression, error) {
	obj := types.OutputExpr{
		Position: p.base(),
		Attrs:    nil,
	}
	if err := p.s.Next(); err != nil {
		return nil, err
	}
	var err error
	obj.Attrs, err = p.parseValue()
	return obj, err
}

func (p *Parser) parseEnclosed() (types.Expression, error) {
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

func ParseFile(filename types.PathExpr) (types.Expression, error) {
	file, err := os.Open(filename.Name)
	if err != nil {
		return nil, fmt.Errorf("%s: failed to open file %s: %w", filename.Pos(), filename.Name, err)
	}
	defer file.Close()
	abs, _ := filepath.Abs(filename.Name)

	scanner := NewScanner(file)
	err = scanner.Next()
	if err != nil {
		return nil, err
	}
	parser := Parser{s: scanner, cwd: path.Dir(abs), filename: filename.Name}
	val, err := parser.parseValue()
	if err != nil {
		return nil, err
	}
	if err := parser.expect(TokenEOF); err != nil {
		return nil, err
	}
	return val, nil
}
