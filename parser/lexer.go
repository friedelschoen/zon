package parser

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"
)

type State int

const (
	StateRoot State = iota
	StateString
	StateStringEscape
	StateMultilineString
	StateInterp
	StateIdent
	StatePath
	StateComment
)

type Scanner struct {
	scanner *bufio.Scanner
	runes   []rune
	current string
	stack   []State

	Linenr int /* incremented by scan */
	End    int /* incremented by consume */
	Start  int
	Token  Token
}

func NewScanner(r io.Reader) *Scanner {
	return &Scanner{
		scanner: bufio.NewScanner(r),
		stack:   []State{StateRoot},
	}
}

func isSymbol(r rune) bool {
	_, ok := symbols[r]
	return ok
}

func isPathPrefix(rs []rune) bool {
	str := string(rs)
	for _, pre := range []string{"/", "./", "../"} {
		if strings.HasPrefix(str, pre) {
			return true
		}
	}
	return false
}

func (s *Scanner) Text() string {
	return s.current[s.Start:s.End]
}

func (s *Scanner) consume(n int) {
	s.runes = s.runes[n:]
	s.End += n
}

func (s *Scanner) pop() {
	s.stack = s.stack[:len(s.stack)-1]
}

func (s *Scanner) push(m State) {
	s.stack = append(s.stack, m)
}

func (s *Scanner) Next() error {
	for {
		if len(s.stack) == 0 {
			s.Start = s.End
			return fmt.Errorf("stack-underflow")
		}

		var chr rune = -1
		if len(s.runes) == 0 {
			if s.scanner.Scan() {
				s.current = s.scanner.Text() + "\n"
				s.runes = []rune(s.current)
				s.Linenr++
				s.End = 0
			}
		}
		if len(s.runes) > 0 {
			chr = s.runes[0]
		}

		var (
			cont bool
			err  error
		)
		mode := s.stack[len(s.stack)-1]
		switch mode {
		case StateRoot, StateInterp:
			cont, err = s.scanRoot(chr, mode)
		case StateString:
			cont, err = s.scanString(chr)
		case StateMultilineString:
			cont, err = s.scanMultiString(chr)
		case StateStringEscape:
			cont, err = s.scanStringEscape(chr)
		case StateIdent:
			cont, err = s.scanIdent(chr)
		case StatePath:
			cont, err = s.scanPath(chr)
		case StateComment:
			cont, err = s.scanComment()
		}
		if !cont {
			return err
		}
		if chr == -1 {
			s.Start = s.End
			return fmt.Errorf("illegal token: end-of-line")
		}
	}
}

func (s *Scanner) scanComment() (bool, error) {
	if len(s.runes) == 0 {
		s.pop()
	}
	cons := strings.Index(string(s.runes), "*/")
	if cons == -1 {
		/* no comment end yet */
		s.runes = s.runes[:0]
	} else {
		s.consume(cons + 2)
		s.pop()
	}
	return true, nil
}

func (s *Scanner) scanPath(chr rune) (bool, error) {
	if !unicode.IsSpace(chr) && !strings.ContainsRune(",{}[]()'\"", chr) {
		s.consume(1)
	} else {
		s.Token = TokenPath
		s.pop()
		return false, nil
	}
	return true, nil
}

func (s *Scanner) scanIdent(chr rune) (bool, error) {
	if unicode.IsLetter(chr) || unicode.IsDigit(chr) {
		s.consume(1)
	} else {
		if tok, ok := keywords[s.current[s.Start:s.End]]; ok {
			s.Token = tok
		} else {
			s.Token = TokenIdent
		}
		s.pop()
		return false, nil
	}
	return true, nil
}

func (s *Scanner) scanString(chr rune) (bool, error) {
	switch chr {
	case '\\':
		s.Start = s.End
		s.consume(1)
		s.push(StateStringEscape)
	case '"':
		s.Token = TokenStringEnd
		s.consume(1)
		s.Start = s.End - 1
		s.pop()
		return false, nil
	case '\n':
		s.Start = s.End
		return false, fmt.Errorf("illegal token: `\n`")
	case -1:
		s.Start = s.End
		return false, fmt.Errorf("illegal token: end-of-line")
	default:
		s.Token = TokenStringChar
		s.consume(1)
		s.Start = s.End - 1
		return false, nil
	}

	return true, nil
}

func (s *Scanner) scanMultiString(chr rune) (bool, error) {
	if strings.HasPrefix(string(s.runes), "''") {
		s.Token = TokenStringEnd
		s.consume(2)
		s.Start = s.End - 2
		s.pop()
		return false, nil
	}
	switch chr {
	case '\\':
		s.Start = s.End
		s.consume(1)
		s.push(StateStringEscape)
	case -1:
		s.Start = s.End
		return false, fmt.Errorf("illegal token: end-of-line")
	default:
		s.Token = TokenStringChar
		s.consume(1)
		s.Start = s.End - 1
		return false, nil
	}
	return true, nil
}

var numberPattern = regexp.MustCompile(`^-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?`)

func (s *Scanner) scanStringEscape(chr rune) (bool, error) {
	switch chr {
	case '"', '\'', '\\', 'b', 'f', 'n', 'r', 't', '\n':
		s.Token = TokenStringEscape
		s.consume(1)
		s.pop()
		return false, nil
	case '(':
		s.Token = TokenInterp
		s.consume(1)
		s.pop()
		s.push(StateInterp)
		return false, nil
	case -1:
		s.Start = s.End
		return false, fmt.Errorf("illegal token: end-of-line")
	case 'u':
		if len(s.runes) < 5 {
			s.Start = s.End
			return false, fmt.Errorf("illegal unicode-escape: `\\%c`", chr)
		}
		hex := s.runes[1:4]
		if strings.ContainsFunc(string(hex), func(r rune) bool {
			return !unicode.Is(unicode.Hex_Digit, r)
		}) {
			return false, fmt.Errorf("illegal unicode-escape: `\\%c`", chr)
		}
		s.Token = TokenStringEscape
		s.consume(5)
		s.pop()
		return false, nil
	default:
		return false, fmt.Errorf("illegal escape: `\\%c`", chr)
	}
}

func (s *Scanner) scanRoot(chr rune, mode State) (bool, error) {
	switch {
	case chr == -1:
		s.Token = TokenEOF
		s.Start = s.End
		return false, nil
	case unicode.IsSpace(chr):
		s.consume(1)
	case strings.HasPrefix(string(s.runes), "//"):
		/* consume rest of the line */
		s.runes = s.runes[:0]
	case strings.HasPrefix(string(s.runes), "/*"):
		s.consume(2)
		s.push(StateComment)
	case isPathPrefix(s.runes):
		s.push(StatePath)
		s.Start = s.End
	case chr == '"':
		s.Token = TokenString
		s.consume(1)
		s.Start = s.End - 1
		s.push(StateString)
		return false, nil
	case strings.HasPrefix(string(s.runes), "''"):
		s.Token = TokenString
		s.consume(2)
		s.Start = s.End - 2
		s.push(StateMultilineString)
		return false, nil
	case mode == StateInterp && chr == ')':
		s.Token = TokenInterpEnd
		s.consume(1)
		s.Start = s.End - 1
		s.pop()
		return false, nil
	case isSymbol(chr):
		s.Token = symbols[chr]
		s.consume(1)
		s.Start = s.End - 1
		return false, nil
	case unicode.IsLetter(chr):
		s.push(StateIdent)
		s.Start = s.End
	default:
		if m := numberPattern.FindStringIndex(string(s.runes)); m != nil {
			s.Token = TokenNumber
			s.Start = s.End
			s.consume(m[1])
			return false, nil
		}
		s.Start = s.End
		return false, fmt.Errorf("illegal token: `%c`", chr)
	}
	return true, nil
}
