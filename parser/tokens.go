package parser

import "fmt"

type Token int

const (
	TokenEOF          Token = iota /* end of file */
	TokenLBrace                    /* { */
	TokenRBrace                    /* } */
	TokenLBracket                  /* [ */
	TokenRBracket                  /* ] */
	TokenColon                     /* : */
	TokenComma                     /* , */
	TokenString                    /* "hello */
	TokenInterp                    /* \( */
	TokenInterpEnd                 /* ) */
	TokenStringEnd                 /* " */
	TokenStringChar                /* char */
	TokenStringEscape              /* \n, \t, ... */
	TokenInteger                   /* 10 */
	TokenFloat                     /* 10.12 */
	TokenIdent                     /* identifier123 */
	TokenTrue                      /* true */
	TokenFalse                     /* false */
	TokenLet                       /* let */
	TokenIn                        /* in */
	TokenEquals                    /* = */
	TokenPath                      /* ../hello, ./foo */
	TokenInclude                   /* include */
	TokenDot                       /* . */
	TokenWith                      /* with */
	TokenOutput                    /* output */
	TokenLParen                    /* ( */
	TokenRParen                    /* ) */
)

func (t Token) String() string {
	for k, v := range symbols {
		if v == t {
			return fmt.Sprintf("'%c'", k)
		}
	}
	for k, v := range keywords {
		if v == t {
			return fmt.Sprintf("'%s'", k)
		}
	}

	switch t {
	case TokenString:
		return "'\"' or '\\'\\''"
	case TokenInterp:
		return "'\\('"
	case TokenInterpEnd:
		return "')'"
	case TokenStringEnd:
		return "'\"'"
	case TokenStringChar:
		return "string-character"
	case TokenStringEscape:
		return "string-escape"
	case TokenInteger:
		return "integer"
	case TokenFloat:
		return "float"
	case TokenIdent:
		return "identifier"
	case TokenPath:
		return "path"
	case TokenInclude:
		return "'include'"
	case TokenEOF:
		return "end-of-file"
	}
	return "<unknown>"
}

var symbols = map[rune]Token{
	'{': TokenLBrace,
	'}': TokenRBrace,
	'[': TokenLBracket,
	']': TokenRBracket,
	'(': TokenLParen,
	')': TokenRParen,
	':': TokenColon,
	',': TokenComma,
	'=': TokenEquals,
	'.': TokenDot,
}

var keywords = map[string]Token{
	"true":    TokenTrue,
	"false":   TokenFalse,
	"let":     TokenLet,
	"include": TokenInclude,
	"in":      TokenIn,
	"with":    TokenWith,
	"output":  TokenOutput,
}
