package switaculous

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
)

type Pos struct {
	File      string
	Line, Col int
}

func (p Pos) String() string {
	return fmt.Sprintf("%s:%d:%d", filepath.Base(p.File), p.Line, p.Col)
}

type TK int

const (
	tEOF TK = iota
	tIdent
	tInt
	tFloat
	tString
	tOp
	tPunct
	tAttr
	tPound
)

type StrSeg struct {
	Lit      string
	IsInterp bool
	Src      string
	IPos     Pos
}

type Token struct {
	K       TK
	S       string
	Pos     Pos
	NL      bool // newline before token
	SpB     bool // whitespace before
	SpA     bool // whitespace after
	Esc     bool // backtick-escaped identifier
	Postfix bool // postfix ? or !
	Segs    []StrSeg
	IntV    int64
	FloatV  float64
}

type SwiftError struct {
	Pos Pos
	Msg string
}

func (e *SwiftError) Error() string { return e.Pos.String() + ": " + e.Msg }

type lexer struct {
	src       []rune
	i         int
	line, col int
	file      string
	toks      []Token
	lineBase  int
	colBase   int
}

const opChars = "/=-+!*%<>&|^~?"

func isOpChar(r rune) bool { return strings.ContainsRune(opChars, r) }

func isIdentStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || r > 0x2000 && !unicode.IsSpace(r) && !unicode.IsPunct(r) && !unicode.IsSymbol(r)
}
func isIdentChar(r rune) bool { return isIdentStart(r) || unicode.IsDigit(r) }

func Lex(file, src string) (toks []Token, err error) {
	return lexAt(file, src, 1, 1)
}

func lexAt(file, src string, line, col int) (toks []Token, err error) {
	lx := &lexer{src: []rune(src), file: file, line: line, col: col}
	defer func() {
		if r := recover(); r != nil {
			if se, ok := r.(*SwiftError); ok {
				err = se
				return
			}
			panic(r)
		}
	}()
	lx.run()
	return lx.toks, nil
}

func (lx *lexer) pos() Pos { return Pos{lx.file, lx.line, lx.col} }

func (lx *lexer) errf(p Pos, f string, a ...interface{}) {
	panic(&SwiftError{p, fmt.Sprintf(f, a...)})
}

func (lx *lexer) peekr(n int) rune {
	if lx.i+n < len(lx.src) {
		return lx.src[lx.i+n]
	}
	return 0
}

func (lx *lexer) adv() rune {
	r := lx.src[lx.i]
	lx.i++
	if r == '\n' {
		lx.line++
		lx.col = 1
	} else {
		lx.col++
	}
	return r
}

func (lx *lexer) run() {
	nl := true
	sp := true
	for {
		for lx.i < len(lx.src) {
			r := lx.src[lx.i]
			if r == '\n' {
				nl = true
				sp = true
				lx.adv()
			} else if r == ' ' || r == '\t' || r == '\r' || r == '\f' {
				sp = true
				lx.adv()
			} else if r == '/' && lx.peekr(1) == '/' {
				for lx.i < len(lx.src) && lx.src[lx.i] != '\n' {
					lx.adv()
				}
				sp = true
			} else if r == '/' && lx.peekr(1) == '*' {
				p := lx.pos()
				depth := 0
				for {
					if lx.i >= len(lx.src) {
						lx.errf(p, "unterminated '/*' comment")
					}
					if lx.src[lx.i] == '/' && lx.peekr(1) == '*' {
						depth++
						lx.adv()
						lx.adv()
					} else if lx.src[lx.i] == '*' && lx.peekr(1) == '/' {
						depth--
						lx.adv()
						lx.adv()
						if depth == 0 {
							break
						}
					} else {
						if lx.src[lx.i] == '\n' {
							nl = true
						}
						lx.adv()
					}
				}
				sp = true
			} else {
				break
			}
		}
		if len(lx.toks) > 0 {
			lx.toks[len(lx.toks)-1].SpA = sp
		}
		if lx.i >= len(lx.src) {
			lx.toks = append(lx.toks, Token{K: tEOF, Pos: lx.pos(), NL: true, SpB: true, SpA: true})
			return
		}
		t := lx.next(sp)
		t.NL = nl
		t.SpB = sp
		lx.toks = append(lx.toks, t)
		nl = false
		sp = false
	}
}

func (lx *lexer) prevEndsExpr() bool {
	if len(lx.toks) == 0 {
		return false
	}
	t := lx.toks[len(lx.toks)-1]
	switch t.K {
	case tIdent, tInt, tFloat, tString:
		return true
	case tPunct:
		return t.S == ")" || t.S == "]" || t.S == "}"
	case tOp:
		return t.Postfix || t.S == ">"
	}
	return false
}

func (lx *lexer) next(spaceBefore bool) Token {
	p := lx.pos()
	r := lx.src[lx.i]
	switch {
	case r == '"' || (r == '#' && (lx.peekr(1) == '"' || (lx.peekr(1) == '#' && lx.peekr(2) == '"'))):
		return lx.lexString(p)
	case r == '`':
		lx.adv()
		start := lx.i
		for lx.i < len(lx.src) && lx.src[lx.i] != '`' {
			lx.adv()
		}
		name := string(lx.src[start:lx.i])
		lx.adv()
		return Token{K: tIdent, S: name, Pos: p, Esc: true}
	case r == '$' || isIdentStart(r):
		start := lx.i
		lx.adv()
		for lx.i < len(lx.src) && isIdentChar(lx.src[lx.i]) {
			lx.adv()
		}
		return Token{K: tIdent, S: string(lx.src[start:lx.i]), Pos: p}
	case unicode.IsDigit(r):
		return lx.lexNumber(p)
	case r == '@':
		lx.adv()
		start := lx.i
		for lx.i < len(lx.src) && isIdentChar(lx.src[lx.i]) {
			lx.adv()
		}
		return Token{K: tAttr, S: string(lx.src[start:lx.i]), Pos: p}
	case r == '#':
		lx.adv()
		start := lx.i
		for lx.i < len(lx.src) && isIdentChar(lx.src[lx.i]) {
			lx.adv()
		}
		return Token{K: tPound, S: string(lx.src[start:lx.i]), Pos: p}
	case r == '.':
		if lx.peekr(1) == '.' && (lx.peekr(2) == '.' || lx.peekr(2) == '<') {
			lx.adv()
			lx.adv()
			lx.adv()
			s := "..."
			if lx.src[lx.i-1] == '<' {
				s = "..<"
			}
			return Token{K: tOp, S: s, Pos: p}
		}
		lx.adv()
		return Token{K: tPunct, S: ".", Pos: p}
	case r == '-' && lx.peekr(1) == '>':
		lx.adv()
		lx.adv()
		return Token{K: tPunct, S: "->", Pos: p}
	case strings.ContainsRune("()[]{},:;\\", r):
		lx.adv()
		return Token{K: tPunct, S: string(r), Pos: p}
	case isOpChar(r):
		if (r == '?' || r == '!') && !spaceBefore && lx.prevEndsExpr() {
			n := lx.peekr(1)
			if !(r == '!' && n == '=') && !(r == '?' && n == '?') {
				lx.adv()
				return Token{K: tOp, S: string(r), Pos: p, Postfix: true}
			}
		}
		start := lx.i
		for lx.i < len(lx.src) && isOpChar(lx.src[lx.i]) {
			if lx.i > start && lx.src[lx.i] == '/' && (lx.peekr(1) == '/' || lx.peekr(1) == '*') {
				break
			}
			lx.adv()
		}
		return Token{K: tOp, S: string(lx.src[start:lx.i]), Pos: p}
	}
	lx.errf(p, "unexpected character '%c'", r)
	return Token{}
}

func (lx *lexer) lexNumber(p Pos) Token {
	start := lx.i
	isFloat := false
	if lx.src[lx.i] == '0' && (lx.peekr(1) == 'x' || lx.peekr(1) == 'b' || lx.peekr(1) == 'o') {
		base := 16
		switch lx.peekr(1) {
		case 'b':
			base = 2
		case 'o':
			base = 8
		}
		lx.adv()
		lx.adv()
		var v int64
		for lx.i < len(lx.src) {
			c := lx.src[lx.i]
			d := -1
			switch {
			case c >= '0' && c <= '9':
				d = int(c - '0')
			case c >= 'a' && c <= 'f':
				d = int(c-'a') + 10
			case c >= 'A' && c <= 'F':
				d = int(c-'A') + 10
			case c == '_':
				lx.adv()
				continue
			}
			if d < 0 || d >= base {
				break
			}
			v = v*int64(base) + int64(d)
			lx.adv()
		}
		return Token{K: tInt, S: string(lx.src[start:lx.i]), Pos: p, IntV: v}
	}
	for lx.i < len(lx.src) && (unicode.IsDigit(lx.src[lx.i]) || lx.src[lx.i] == '_') {
		lx.adv()
	}
	if lx.i < len(lx.src) && lx.src[lx.i] == '.' && unicode.IsDigit(lx.peekr(1)) {
		isFloat = true
		lx.adv()
		for lx.i < len(lx.src) && (unicode.IsDigit(lx.src[lx.i]) || lx.src[lx.i] == '_') {
			lx.adv()
		}
	}
	if lx.i < len(lx.src) && (lx.src[lx.i] == 'e' || lx.src[lx.i] == 'E') {
		n := lx.peekr(1)
		if unicode.IsDigit(n) || ((n == '-' || n == '+') && unicode.IsDigit(lx.peekr(2))) {
			isFloat = true
			lx.adv()
			if n == '-' || n == '+' {
				lx.adv()
			}
			for lx.i < len(lx.src) && unicode.IsDigit(lx.src[lx.i]) {
				lx.adv()
			}
		}
	}
	s := strings.ReplaceAll(string(lx.src[start:lx.i]), "_", "")
	if isFloat {
		var f float64
		fmt.Sscanf(s, "%g", &f)
		return Token{K: tFloat, S: s, Pos: p, FloatV: f}
	}
	var v int64
	fmt.Sscanf(s, "%d", &v)
	return Token{K: tInt, S: s, Pos: p, IntV: v}
}

func (lx *lexer) lexString(p Pos) Token {
	hashes := 0
	for lx.src[lx.i] == '#' {
		hashes++
		lx.adv()
	}
	multi := false
	if lx.peekr(0) == '"' && lx.peekr(1) == '"' && lx.peekr(2) == '"' {
		multi = true
		lx.adv()
		lx.adv()
		lx.adv()
	} else {
		lx.adv()
	}
	closeHashes := strings.Repeat("#", hashes)
	isClose := func() bool {
		if multi {
			if !(lx.peekr(0) == '"' && lx.peekr(1) == '"' && lx.peekr(2) == '"') {
				return false
			}
			for k := 0; k < hashes; k++ {
				if lx.peekr(3+k) != '#' {
					return false
				}
			}
			return true
		}
		if lx.peekr(0) != '"' {
			return false
		}
		for k := 0; k < hashes; k++ {
			if lx.peekr(1+k) != '#' {
				return false
			}
		}
		return true
	}
	_ = closeHashes
	type rr struct {
		r   rune
		pos Pos
	}
	var raw []rr
	for {
		if lx.i >= len(lx.src) {
			lx.errf(p, "unterminated string literal")
		}
		if !multi && lx.src[lx.i] == '\n' {
			lx.errf(p, "unterminated string literal")
		}
		if isClose() {
			n := 1 + hashes
			if multi {
				n = 3 + hashes
			}
			for k := 0; k < n; k++ {
				lx.adv()
			}
			break
		}
		if lx.src[lx.i] == '\\' && hashes == 0 {
			raw = append(raw, rr{lx.src[lx.i], lx.pos()})
			lx.adv()
			if lx.i < len(lx.src) {
				if lx.src[lx.i] == '(' {
					depth := 0
					for lx.i < len(lx.src) {
						c := lx.src[lx.i]
						raw = append(raw, rr{c, lx.pos()})
						lx.adv()
						if c == '(' {
							depth++
						} else if c == ')' {
							depth--
							if depth == 0 {
								break
							}
						} else if c == '"' {
							for lx.i < len(lx.src) && lx.src[lx.i] != '"' {
								if lx.src[lx.i] == '\\' {
									raw = append(raw, rr{lx.src[lx.i], lx.pos()})
									lx.adv()
								}
								raw = append(raw, rr{lx.src[lx.i], lx.pos()})
								lx.adv()
							}
							if lx.i < len(lx.src) {
								raw = append(raw, rr{lx.src[lx.i], lx.pos()})
								lx.adv()
							}
						}
					}
					continue
				}
				raw = append(raw, rr{lx.src[lx.i], lx.pos()})
				lx.adv()
			}
			continue
		}
		raw = append(raw, rr{lx.src[lx.i], lx.pos()})
		lx.adv()
	}
	if multi {
		// Swift multi-line strings drop the opening newline and remove the closing delimiter's indentation from every line.
		s := make([]rr, len(raw))
		copy(s, raw)
		k := 0
		for k < len(s) && (s[k].r == ' ' || s[k].r == '\t') {
			k++
		}
		if k < len(s) && s[k].r == '\n' {
			s = s[k+1:]
		}
		j := len(s)
		for j > 0 && (s[j-1].r == ' ' || s[j-1].r == '\t') {
			j--
		}
		indent := len(s) - j
		if j > 0 && s[j-1].r == '\n' {
			s = s[:j-1]
		} else {
			s = s[:j]
		}
		var out []rr
		atLineStart := true
		removed := 0
		for _, c := range s {
			if atLineStart && removed < indent && (c.r == ' ' || c.r == '\t') {
				removed++
				continue
			}
			atLineStart = false
			out = append(out, c)
			if c.r == '\n' {
				atLineStart = true
				removed = 0
			}
		}
		raw = out
	}
	var segs []StrSeg
	var sb strings.Builder
	for k := 0; k < len(raw); k++ {
		c := raw[k].r
		if c == '\\' && hashes == 0 && k+1 < len(raw) {
			k++
			e := raw[k].r
			switch e {
			case 'n':
				sb.WriteRune('\n')
			case 't':
				sb.WriteRune('\t')
			case 'r':
				sb.WriteRune('\r')
			case '0':
				sb.WriteRune(0)
			case '\\':
				sb.WriteRune('\\')
			case '"':
				sb.WriteRune('"')
			case '\'':
				sb.WriteRune('\'')
			case '\n':
				// A backslash at the end of a line in a multi-line string joins it with the next line.
			case 'u':
				if k+1 < len(raw) && raw[k+1].r == '{' {
					k += 2
					var v rune
					for k < len(raw) && raw[k].r != '}' {
						var d rune
						fmt.Sscanf(string(raw[k].r), "%x", &d)
						v = v*16 + d
						k++
					}
					sb.WriteRune(v)
				}
			case '(':
				if sb.Len() > 0 {
					segs = append(segs, StrSeg{Lit: sb.String()})
					sb.Reset()
				}
				depth := 1
				start := k + 1
				m := start
				inStr := false
				for m < len(raw) {
					ch := raw[m].r
					if inStr {
						if ch == '\\' {
							m += 2
							continue
						}
						if ch == '"' {
							inStr = false
						}
					} else if ch == '"' {
						inStr = true
					} else if ch == '(' {
						depth++
					} else if ch == ')' {
						depth--
						if depth == 0 {
							break
						}
					}
					m++
				}
				var src strings.Builder
				for q := start; q < m && q < len(raw); q++ {
					src.WriteRune(raw[q].r)
				}
				ip := p
				if start < len(raw) {
					ip = raw[start].pos
				}
				segs = append(segs, StrSeg{IsInterp: true, Src: src.String(), IPos: ip})
				k = m
			default:
				lx.errf(raw[k].pos, "invalid escape sequence in literal")
			}
			continue
		}
		sb.WriteRune(c)
	}
	if sb.Len() > 0 || len(segs) == 0 {
		segs = append(segs, StrSeg{Lit: sb.String()})
	}
	return Token{K: tString, Pos: p, Segs: segs}
}
