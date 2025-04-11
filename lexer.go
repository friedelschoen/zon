package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"unicode"
)

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
)

func (t Token) String() string {
	switch t {
	case TokenLBrace:
		return "'{'"
	case TokenRBrace:
		return "'}'"
	case TokenLBracket:
		return "'['"
	case TokenRBracket:
		return "']'"
	case TokenColon:
		return "':'"
	case TokenComma:
		return "','"
	case TokenString:
		return "'\"'"
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
	case TokenTrue:
		return "'true'"
	case TokenFalse:
		return "'false'"
	case TokenEOF:
		return "end-of-file"
	}
	return "<unknown>"
}

type Mode int

const (
	ModeIllegal Mode = -1
	ModeRoot    Mode = iota
	ModeString
	ModeStringEscape
	ModeInterp
	ModeIdent
)

type Scanner struct {
	scanner *bufio.Scanner
	runes   []rune
	current string
	stack   []Mode

	Linenr int /* incremented by scan */
	End    int /* incremented by consume */
	Start  int
	Token  Token
}

func NewScanner(r io.Reader) *Scanner {
	return &Scanner{
		scanner: bufio.NewScanner(r),
		stack:   []Mode{ModeRoot},
	}
}

var symbols = map[rune]Token{
	'{': TokenLBrace,
	'}': TokenRBrace,
	'[': TokenLBracket,
	']': TokenRBracket,
	':': TokenColon,
	',': TokenComma,
}

var keywords = map[string]Token{
	"true":  TokenTrue,
	"false": TokenFalse,
}

func isSymbol(r rune) bool {
	_, ok := symbols[r]
	return ok
}

var lastkeyword string

func isKeyword(rs []rune) bool {
	str := string(rs)
	for key := range keywords {
		if strings.HasPrefix(str, key) {
			lastkeyword = key
			return true
		}
	}
	return false
}

func (s *Scanner) Next() error {
	for {
		var chr rune = -1
		if len(s.runes) == 0 {
			if s.scanner.Scan() {
				s.current = s.scanner.Text()
				s.runes = []rune(s.current)
				s.Linenr++
				s.End = 0
			}
		}
		if len(s.runes) > 0 {
			chr = s.runes[0]
		}
		switch s.mode() {
		case ModeIllegal:
			s.Start = s.End
			return fmt.Errorf("stack-underflow")
		case ModeRoot, ModeInterp:
			switch {
			case chr == -1:
				s.Token = TokenEOF
				s.Start = s.End
				return nil
			case chr == ' ' || chr == '\t':
				s.consume(1)
			case isSymbol(chr):
				s.Token = symbols[chr]
				s.consume(1)
				s.Start = s.End - 1
				return nil
			case chr == '"':
				s.Token = TokenString
				s.consume(1)
				s.Start = s.End - 1
				s.push(ModeString)
				return nil
			case s.mode() == ModeInterp && chr == ')':
				s.Token = TokenInterpEnd
				s.consume(1)
				s.Start = s.End - 1
				s.pop()
				return nil
			case isKeyword(s.runes):
				s.Token = keywords[lastkeyword]
				s.consume(len(lastkeyword))
				s.Start = s.End - len(lastkeyword)
				return nil
			case unicode.IsLetter(chr):
				s.push(ModeIdent)
				s.Start = s.End
			default:
				s.Start = s.End
				return fmt.Errorf("illegal token: `%c`", chr)
			}
		case ModeString:
			switch chr {
			case '\\':
				s.Start = s.End
				s.consume(1)
				s.push(ModeStringEscape)
			case '"':
				s.Token = TokenStringEnd
				s.consume(1)
				s.Start = s.End - 1
				s.pop()
				return nil
			default:
				s.Token = TokenStringChar
				s.consume(1)
				s.Start = s.End - 1
				return nil
			}
		case ModeStringEscape:
			switch chr {
			case '"', '\\', 'b', 'f', 'n', 'r', 't':
				s.Token = TokenStringEscape
				s.consume(1)
				s.pop()
				return nil
			case '(':
				s.Token = TokenInterp
				s.consume(2)
				s.pop()
				s.push(ModeInterp)
				return nil
			case 'u':
				if len(s.runes) < 5 {
					s.Start = s.End
					return fmt.Errorf("illegal unicode-escape: `\\%c`", chr)
				}
				hex := s.runes[1:4]
				if strings.ContainsFunc(string(hex), func(r rune) bool {
					return !unicode.Is(unicode.Hex_Digit, r)
				}) {
					return fmt.Errorf("illegal unicode-escape: `\\%c`", chr)
				}
				s.Token = TokenStringEscape
				s.consume(5)
				s.pop()
				return nil
			default:
				return fmt.Errorf("illegal escape: `\\%c`", chr)
			}
		case ModeIdent:
			if unicode.IsLetter(chr) || unicode.IsDigit(chr) {
				s.consume(1)
			} else {
				s.Token = TokenIdent
				s.pop()
				return nil
			}
		}
	}
}

func (s *Scanner) Text() string {
	return s.current[s.Start:s.End]
}

func (s *Scanner) consume(n int) {
	s.runes = s.runes[n:]
	s.End += n
}

func (s *Scanner) mode() Mode {
	if len(s.stack) == 0 {
		return ModeIllegal
	}
	return s.stack[len(s.stack)-1]
}

func (s *Scanner) pop() {
	s.stack = s.stack[:len(s.stack)-1]
}

func (s *Scanner) push(m Mode) {
	s.stack = append(s.stack, m)
}
