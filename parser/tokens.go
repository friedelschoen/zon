package parser

import "fmt"

type Token int

const (
	TokenEOF          Token = iota /* end of file */
	TokenAssign                    /* = */
	TokenColon                     /* : */
	TokenComma                     /* , */
	TokenDot                       /* . */
	TokenElse                      /* else */
	TokenEquals                    /* == */
	TokenFalse                     /* false */
	TokenFloat                     /* 10.12 */
	TokenFunction                  /* fn */
	TokenIdent                     /* identifier123 */
	TokenIf                        /* if */
	TokenIn                        /* in */
	TokenInclude                   /* include */
	TokenInterp                    /* \( */
	TokenInterpEnd                 /* ) */
	TokenLBrace                    /* { */
	TokenLBracket                  /* [ */
	TokenLParen                    /* ( */
	TokenLet                       /* let */
	TokenNumber                    /* 10 */
	TokenOutput                    /* output */
	TokenPath                      /* ../hello, ./foo */
	TokenRBrace                    /* } */
	TokenRBracket                  /* ] */
	TokenRParen                    /* ) */
	TokenString                    /* "hello */
	TokenStringChar                /* char */
	TokenStringEnd                 /* " */
	TokenStringEscape              /* \n, \t, ... */
	TokenThen                      /* then */
	TokenTrue                      /* true */
	TokenUnequals                  /* != */
	TokenWith                      /* with */
)

type tokenMatch struct {
	text  string
	token Token
}

var symbols = []tokenMatch{
	{"==", TokenEquals},
	{"!=", TokenUnequals},
	{"{", TokenLBrace},
	{"}", TokenRBrace},
	{"[", TokenLBracket},
	{"]", TokenRBracket},
	{"(", TokenLParen},
	{")", TokenRParen},
	{":", TokenColon},
	{",", TokenComma},
	{"=", TokenAssign},
	{".", TokenDot},
}

var keywords = map[string]Token{
	"true":    TokenTrue,
	"false":   TokenFalse,
	"let":     TokenLet,
	"include": TokenInclude,
	"in":      TokenIn,
	"with":    TokenWith,
	"output":  TokenOutput,
	"fn":      TokenFunction,
	"if":      TokenIf,
	"then":    TokenThen,
	"else":    TokenElse,
}

var operators = []Token{
	TokenEquals, TokenUnequals,
}

func (t Token) String() string {
	for _, v := range symbols {
		if t == v.token {
			return fmt.Sprintf("'%s'", v.text)
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
	case TokenNumber:
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
