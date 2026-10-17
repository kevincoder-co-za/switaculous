package switaculous

import (
	"fmt"
	"strconv"
	"strings"
)

type Parser struct {
	toks    []Token
	i       int
	noTrail int
	file    string
}

func ParseFile(file, src string) (stmts []Stmt, err error) {
	src = strings.TrimPrefix(src, "\uFEFF")
	src = strings.ReplaceAll(src, "\r\n", "\n")
	toks, err := Lex(file, src)
	if err != nil {
		return nil, err
	}
	toks = preprocess(toks)
	p := &Parser{toks: toks, file: file}
	defer func() {
		if r := recover(); r != nil {
			if se, ok := r.(*SwiftError); ok {
				err = se
				return
			}
			panic(r)
		}
	}()
	for p.t().K != tEOF {
		if p.isP(";") {
			p.next()
			continue
		}
		s := p.parseStmt()
		if s != nil {
			stmts = append(stmts, s)
		}
	}
	return stmts, nil
}

// ---------- #if preprocessing ----------

func preprocess(toks []Token) []Token {
	var out []Token
	type frame struct{ active, taken, parentActive bool }
	var stack []frame
	active := true
	i := 0
	for i < len(toks) {
		t := toks[i]
		if t.K == tPound && (t.S == "if" || t.S == "elseif" || t.S == "else" || t.S == "endif") {
			j := i + 1
			for j < len(toks) && !toks[j].NL && toks[j].K != tEOF {
				j++
			}
			cond := toks[i+1 : j]
			switch t.S {
			case "if":
				c := evalPPCond(cond)
				stack = append(stack, frame{active: active && c, taken: c, parentActive: active})
				active = active && c
			case "elseif":
				if len(stack) > 0 {
					f := &stack[len(stack)-1]
					c := !f.taken && evalPPCond(cond)
					if c {
						f.taken = true
					}
					f.active = f.parentActive && c
					active = f.active
				}
			case "else":
				if len(stack) > 0 {
					f := &stack[len(stack)-1]
					c := !f.taken
					f.taken = true
					f.active = f.parentActive && c
					active = f.active
				}
				j = i + 1
			case "endif":
				if len(stack) > 0 {
					active = stack[len(stack)-1].parentActive
					stack = stack[:len(stack)-1]
				}
				j = i + 1
			}
			if j < len(toks) {
				toks[j].NL = true
			}
			i = j
			continue
		}
		if active || t.K == tEOF {
			out = append(out, t)
		}
		i++
	}
	return out
}

func evalPPCond(ts []Token) bool {
	i := 0
	var or func() bool
	var prim func() bool
	prim = func() bool {
		if i >= len(ts) {
			return false
		}
		t := ts[i]
		if t.K == tOp && t.S == "!" {
			i++
			return !prim()
		}
		if t.K == tPunct && t.S == "(" {
			i++
			v := or()
			if i < len(ts) && ts[i].S == ")" {
				i++
			}
			return v
		}
		i++
		name := t.S
		if i < len(ts) && ts[i].S == "(" {
			depth := 0
			arg := ""
			for i < len(ts) {
				if ts[i].S == "(" {
					depth++
				} else if ts[i].S == ")" {
					depth--
					if depth == 0 {
						i++
						break
					}
				} else if depth == 1 && arg == "" {
					arg = ts[i].S
				}
				i++
			}
			switch name {
			case "os":
				return arg == "iOS"
			case "canImport", "swift", "compiler", "targetEnvironment":
				return true
			}
			return false
		}
		return name == "DEBUG" || name == "true"
	}
	and := func() bool {
		v := prim()
		for i < len(ts) && ts[i].S == "&&" {
			i++
			w := prim()
			v = v && w
		}
		return v
	}
	or = func() bool {
		v := and()
		for i < len(ts) && ts[i].S == "||" {
			i++
			w := and()
			v = v || w
		}
		return v
	}
	return or()
}

// ---------- helpers ----------

func (p *Parser) t() *Token { return &p.toks[p.i] }
func (p *Parser) peek(n int) *Token {
	if p.i+n < len(p.toks) {
		return &p.toks[p.i+n]
	}
	return &p.toks[len(p.toks)-1]
}
func (p *Parser) next() *Token {
	t := &p.toks[p.i]
	if p.i < len(p.toks)-1 {
		p.i++
	}
	return t
}
func (p *Parser) isP(s string) bool  { t := p.t(); return t.K == tPunct && t.S == s }
func (p *Parser) isOp(s string) bool { t := p.t(); return t.K == tOp && t.S == s }
func (p *Parser) isKw(s string) bool { t := p.t(); return t.K == tIdent && !t.Esc && t.S == s }
func (p *Parser) errf(pos Pos, f string, a ...interface{}) {
	panic(&SwiftError{pos, fmt.Sprintf(f, a...)})
}
func (p *Parser) describe(t *Token) string {
	switch t.K {
	case tEOF:
		return "end of file"
	case tString:
		return "string literal"
	}
	return "'" + t.S + "'"
}
func (p *Parser) expectP(s string) *Token {
	if !p.isP(s) {
		p.errf(p.t().Pos, "expected '%s' but found %s", s, p.describe(p.t()))
	}
	return p.next()
}
func (p *Parser) expectKw(s string) {
	if !p.isKw(s) {
		p.errf(p.t().Pos, "expected '%s' but found %s", s, p.describe(p.t()))
	}
	p.next()
}
func (p *Parser) expectIdent() string {
	t := p.t()
	if t.K != tIdent {
		p.errf(t.Pos, "expected identifier but found %s", p.describe(t))
	}
	p.next()
	return t.S
}

func (p *Parser) attempt(f func()) (ok bool) {
	save := p.i
	saved := make([]Token, 0)
	_ = saved
	defer func() {
		if r := recover(); r != nil {
			if _, isSE := r.(*SwiftError); isSE {
				p.i = save
				ok = false
				return
			}
			panic(r)
		}
	}()
	f()
	return true
}

var declKeywords = map[string]bool{
	"struct": true, "class": true, "enum": true, "protocol": true, "extension": true,
	"func": true, "init": true, "deinit": true, "var": true, "let": true, "typealias": true,
	"subscript": true, "import": true, "actor": true, "associatedtype": true, "case": true,
	"operator": true, "precedencegroup": true,
}
var declModifiers = map[string]bool{
	"private": true, "fileprivate": true, "internal": true, "public": true, "open": true,
	"static": true, "final": true, "override": true, "mutating": true, "nonmutating": true,
	"lazy": true, "weak": true, "unowned": true, "required": true, "convenience": true,
	"dynamic": true, "indirect": true, "nonisolated": true, "optional": true, "class": true,
	"prefix": true, "postfix": true, "infix": true, "consuming": true, "borrowing": true,
	"package": true,
}

func (p *Parser) atDeclStart() bool {
	t := p.t()
	if t.K == tAttr {
		return true
	}
	if t.K != tIdent || t.Esc {
		return false
	}
	if declKeywords[t.S] {
		if t.S == "class" {
			return true
		}
		return true
	}
	if declModifiers[t.S] {
		n := p.peek(1)
		if t.S == "private" || t.S == "fileprivate" || t.S == "public" || t.S == "internal" || t.S == "open" || t.S == "package" {
			if n.K == tPunct && n.S == "(" {
				return true
			}
		}
		return n.K == tAttr || (n.K == tIdent && !n.Esc && (declKeywords[n.S] || declModifiers[n.S]))
	}
	return false
}

// ---------- statements ----------

func (p *Parser) parseBlock() []Stmt {
	p.expectP("{")
	saved := p.noTrail
	p.noTrail = 0
	body := p.parseStmtsUntil()
	p.noTrail = saved
	p.expectP("}")
	return body
}

func (p *Parser) parseStmtsUntil() []Stmt {
	var out []Stmt
	for !p.isP("}") && p.t().K != tEOF {
		if p.isP(";") {
			p.next()
			continue
		}
		s := p.parseStmt()
		if s != nil {
			out = append(out, s)
		}
	}
	return out
}

func (p *Parser) parseStmt() Stmt {
	t := p.t()
	if t.K == tIdent && !t.Esc && p.peek(1).K == tPunct && p.peek(1).S == ":" {
		n := p.peek(2)
		if n.K == tIdent && (n.S == "for" || n.S == "while" || n.S == "repeat" || n.S == "switch" || n.S == "do") {
			label := t.S
			p.next()
			p.next()
			s := p.parseStmt()
			switch st := s.(type) {
			case *ForStmt:
				st.Label = label
			case *WhileStmt:
				st.Label = label
			case *RepeatStmt:
				st.Label = label
			case *SwitchStmt:
				st.Label = label
			}
			return s
		}
	}
	if t.K == tPound {
		switch t.S {
		case "Preview":
			return p.parsePreview()
		}
	}
	if p.atDeclStart() {
		return p.parseDecl()
	}
	if t.K == tIdent && !t.Esc {
		switch t.S {
		case "if":
			return p.parseIf()
		case "guard":
			pos := t.Pos
			p.next()
			conds := p.parseConds()
			p.expectKw("else")
			body := p.parseBlock()
			return &GuardStmt{Pos: pos, Conds: conds, Else: body}
		case "for":
			return p.parseFor()
		case "while":
			pos := t.Pos
			p.next()
			conds := p.parseConds()
			body := p.parseBlock()
			return &WhileStmt{Pos: pos, Conds: conds, Body: body}
		case "repeat":
			pos := t.Pos
			p.next()
			body := p.parseBlock()
			p.expectKw("while")
			p.noTrail++
			c := p.parseExpr()
			p.noTrail--
			return &RepeatStmt{Pos: pos, Body: body, Cond: c}
		case "switch":
			return p.parseSwitch()
		case "return":
			pos := t.Pos
			p.next()
			n := p.t()
			if n.NL || n.K == tEOF || (n.K == tPunct && (n.S == "}" || n.S == ";")) || (n.K == tIdent && (n.S == "case" || n.S == "default") && !n.Esc) {
				return &ReturnStmt{Pos: pos}
			}
			return &ReturnStmt{Pos: pos, X: p.parseExpr()}
		case "break":
			p.next()
			if p.t().K == tIdent && !p.t().NL {
				return &BreakStmt{Label: p.next().S}
			}
			return &BreakStmt{}
		case "continue":
			p.next()
			if p.t().K == tIdent && !p.t().NL {
				return &ContinueStmt{Label: p.next().S}
			}
			return &ContinueStmt{}
		case "fallthrough":
			p.next()
			return &FallthroughStmt{}
		case "throw":
			pos := t.Pos
			p.next()
			return &ThrowStmt{Pos: pos, X: p.parseExpr()}
		case "defer":
			p.next()
			return &DeferStmt{Body: p.parseBlock()}
		case "do":
			if p.peek(1).K == tPunct && p.peek(1).S == "{" {
				return p.parseDo()
			}
		}
	}
	return &ExprStmt{X: p.parseExpr()}
}

func (p *Parser) parseDo() Stmt {
	p.next()
	d := &DoStmt{Body: p.parseBlock()}
	for p.isKw("catch") {
		p.next()
		c := &CatchClause{}
		if !p.isP("{") {
			p.noTrail++
			c.Pat = p.parsePattern(false)
			if p.isKw("where") {
				p.next()
				c.Where = p.parseExpr()
			}
			p.noTrail--
		}
		c.Body = p.parseBlock()
		d.Catches = append(d.Catches, c)
	}
	return d
}

func (p *Parser) parseIf() *IfStmt {
	pos := p.t().Pos
	p.next()
	conds := p.parseConds()
	then := p.parseBlock()
	s := &IfStmt{Pos: pos, Conds: conds, Then: then}
	if p.isKw("else") {
		p.next()
		if p.isKw("if") {
			s.Else = p.parseIf()
		} else {
			s.Else = &BlockStmt{Body: p.parseBlock()}
		}
	}
	return s
}

func (p *Parser) parseConds() []Cond {
	p.noTrail++
	defer func() { p.noTrail-- }()
	var conds []Cond
	for {
		var c Cond
		if p.isKw("let") || p.isKw("var") {
			c.Kind = p.next().S
			c.Pat = p.parsePattern(true)
			if bp, ok := c.Pat.(*BindPat); ok {
				c.Name = bp.Name
			}
			if p.isP(":") {
				p.next()
				c.Type = p.parseType()
			}
			if p.isOp("=") {
				p.next()
				c.X = p.parseExpr()
			} else {
				c.X = &Ident{Pos: p.t().Pos, Name: c.Name}
			}
		} else if p.isKw("case") {
			p.next()
			c.Kind = "case"
			c.Pat = p.parsePattern(false)
			if !p.isOp("=") {
				p.errf(p.t().Pos, "expected '=' in 'case' condition")
			}
			p.next()
			c.X = p.parseExpr()
		} else {
			c.Kind = "expr"
			c.X = p.parseExpr()
		}
		conds = append(conds, c)
		if p.isP(",") {
			p.next()
			continue
		}
		break
	}
	return conds
}

func (p *Parser) parseFor() Stmt {
	pos := p.t().Pos
	p.next()
	// `for try await x in stream` / `for await x in stream`: the sequence is
	// awaited as a whole, the loop itself reads like any other for-in.
	var tryKind string
	isAwait := false
	if p.isKw("try") {
		p.next()
		tryKind = "try"
		if p.t().K == tOp && p.t().Postfix {
			tryKind += p.next().S
		}
	}
	if p.isKw("await") {
		p.next()
		isAwait = true
	}
	if p.isKw("case") {
		p.next()
	}
	pat := p.parsePattern(true)
	if p.isP(":") {
		p.next()
		p.parseType()
	}
	p.expectKw("in")
	p.noTrail++
	seq := p.parseExpr()
	if isAwait {
		seq = &AwaitExpr{Pos: seq.P(), X: seq}
	}
	if tryKind != "" {
		seq = &TryExpr{Pos: seq.P(), Kind: tryKind, X: seq}
	}
	var where Expr
	if p.isKw("where") {
		p.next()
		where = p.parseExpr()
	}
	p.noTrail--
	body := p.parseBlock()
	return &ForStmt{Pos: pos, Pat: pat, Seq: seq, Where: where, Body: body}
}

func (p *Parser) parseSwitch() Stmt {
	pos := p.t().Pos
	p.next()
	p.noTrail++
	subj := p.parseExpr()
	p.noTrail--
	p.expectP("{")
	s := &SwitchStmt{Pos: pos, Subj: subj}
	saved := p.noTrail
	p.noTrail = 0
	for !p.isP("}") {
		if p.t().K == tAttr && p.t().S == "unknown" {
			p.next()
		}
		c := &SwitchCase{Pos: p.t().Pos}
		if p.isKw("default") {
			p.next()
			c.Default = true
		} else if p.isKw("case") {
			p.next()
			for {
				p.noTrail++
				c.Pats = append(c.Pats, p.parsePattern(false))
				p.noTrail--
				if p.isP(",") {
					p.next()
					continue
				}
				break
			}
			if p.isKw("where") {
				p.next()
				c.Where = p.parseExpr()
			}
		} else {
			p.errf(p.t().Pos, "expected 'case' or 'default' in switch but found %s", p.describe(p.t()))
		}
		p.expectP(":")
		for !p.isP("}") && !p.isKw("case") && !p.isKw("default") && !(p.t().K == tAttr && p.t().S == "unknown") {
			if p.isP(";") {
				p.next()
				continue
			}
			st := p.parseStmt()
			if st != nil {
				c.Body = append(c.Body, st)
			}
		}
		s.Cases = append(s.Cases, c)
	}
	p.noTrail = saved
	p.expectP("}")
	return s
}

func (p *Parser) parsePreview() Stmt {
	pos := p.t().Pos
	p.next()
	name := ""
	if p.isP("(") {
		args := p.parseArgs("(", ")")
		for _, a := range args {
			if s, ok := a.X.(*StrLit); ok && a.Label == "" && len(s.Pieces) == 1 && s.Pieces[0].X == nil {
				name = s.Pieces[0].Lit
			}
		}
	}
	body := p.parseBlock()
	return &PreviewDecl{Pos: pos, Name: name, Body: body}
}

// ---------- declarations ----------

func (p *Parser) parseAttrs() []*Attr {
	var attrs []*Attr
	for p.t().K == tAttr {
		t := p.next()
		a := &Attr{Name: t.S, Pos: t.Pos}
		if p.isP("(") && !p.t().SpB {
			if a.Name == "available" || a.Name == "objc" {
				p.skipBalanced("(", ")")
			} else {
				a.Args = p.parseArgs("(", ")")
			}
		}
		attrs = append(attrs, a)
	}
	return attrs
}

func (p *Parser) skipBalanced(open, close string) {
	depth := 0
	for {
		t := p.t()
		if t.K == tEOF {
			p.errf(t.Pos, "unbalanced '%s'", open)
		}
		if t.K == tPunct && t.S == open {
			depth++
		} else if t.K == tPunct && t.S == close {
			depth--
			if depth == 0 {
				p.next()
				return
			}
		}
		p.next()
	}
}

func (p *Parser) parseGenericParamNames() []string {
	if !(p.t().K == tOp && strings.HasPrefix(p.t().S, "<")) {
		return nil
	}
	var names []string
	depth := 0
	expectName := false
	for {
		t := p.t()
		if t.K == tEOF {
			return names
		}
		if t.K == tIdent && expectName && depth == 1 {
			names = append(names, t.S)
			expectName = false
		}
		if t.K == tPunct && t.S == "," && depth == 1 {
			expectName = true
		}
		if t.K == tOp {
			for _, c := range t.S {
				if c == '<' {
					depth++
					expectName = depth == 1
				} else if c == '>' {
					depth--
				}
			}
		}
		p.next()
		if t.K == tOp && depth <= 0 {
			return names
		}
	}
}

func (p *Parser) skipAngles() {
	if !(p.t().K == tOp && strings.HasPrefix(p.t().S, "<")) {
		return
	}
	depth := 0
	for {
		t := p.t()
		if t.K == tEOF {
			return
		}
		if t.K == tOp {
			for _, c := range t.S {
				if c == '<' {
					depth++
				} else if c == '>' {
					depth--
				}
			}
			p.next()
			if depth <= 0 {
				return
			}
			continue
		}
		p.next()
	}
}

func (p *Parser) skipWhere() {
	if p.isKw("where") {
		for !p.isP("{") && p.t().K != tEOF {
			p.next()
		}
	}
}

func (p *Parser) parseDecl() Stmt {
	attrs := p.parseAttrs()
	mods := map[string]bool{}
	for {
		t := p.t()
		if t.K == tAttr {
			attrs = append(attrs, p.parseAttrs()...)
			continue
		}
		if t.K == tIdent && !t.Esc && declModifiers[t.S] {
			n := p.peek(1)
			if t.S == "class" && n.K == tIdent && !declKeywords[n.S] && !declModifiers[n.S] {
				break
			}
			if (n.K == tIdent && (declKeywords[n.S] || declModifiers[n.S])) || n.K == tAttr || (n.K == tPunct && n.S == "(") {
				p.next()
				mods[t.S] = true
				if p.isP("(") {
					p.skipBalanced("(", ")")
				}
				continue
			}
		}
		break
	}
	t := p.t()
	if t.K != tIdent {
		p.errf(t.Pos, "expected declaration")
	}
	switch t.S {
	case "struct", "class", "enum", "protocol", "extension", "actor":
		return p.parseTypeDecl(attrs, mods)
	case "func":
		return p.parseFunc(attrs, mods)
	case "init":
		return p.parseInit(attrs, mods)
	case "deinit":
		p.next()
		p.parseBlock()
		return nil
	case "subscript":
		return p.parseSubscript(attrs, mods)
	case "var", "let":
		return p.parseVar(attrs, mods)
	case "typealias":
		p.next()
		name := p.expectIdent()
		p.skipAngles()
		if !p.isOp("=") {
			p.errf(p.t().Pos, "expected '=' in typealias")
		}
		p.next()
		return &TypeAliasDecl{Name: name, T: p.parseType()}
	case "associatedtype":
		p.next()
		p.expectIdent()
		if p.isP(":") {
			p.next()
			p.parseType()
		}
		if p.isOp("=") {
			p.next()
			p.parseType()
		}
		return nil
	case "import":
		p.next()
		for !p.t().NL && p.t().K != tEOF {
			p.next()
		}
		return nil
	case "case":
		return p.parseEnumCase()
	case "operator", "precedencegroup":
		for !p.t().NL && p.t().K != tEOF && !p.isP("{") {
			p.next()
		}
		if p.isP("{") {
			p.skipBalanced("{", "}")
		}
		return nil
	}
	p.errf(t.Pos, "expected declaration but found %s", p.describe(t))
	return nil
}

func (p *Parser) parseTypeDecl(attrs []*Attr, mods map[string]bool) Stmt {
	kw := p.next()
	kind := kw.S
	if kind == "actor" {
		kind = "class"
	}
	d := &TypeDecl{Pos: kw.Pos, Kind: kind, Attrs: attrs, Mods: mods}
	if kind == "extension" {
		ty := p.parseType()
		d.Name = ty.Name
		if ty.Kind == tyArray {
			d.Name = "Array"
		} else if ty.Kind == tyDict {
			d.Name = "Dictionary"
		}
	} else {
		d.Name = p.expectIdent()
		p.skipAngles()
	}
	if p.isP(":") {
		p.next()
		for {
			ty := p.parseType()
			d.Inherits = append(d.Inherits, ty.Name)
			for p.isOp("&") {
				p.next()
				d.Inherits = append(d.Inherits, p.parseType().Name)
			}
			if p.isP(",") {
				p.next()
				continue
			}
			break
		}
	}
	p.skipWhere()
	p.expectP("{")
	saved := p.noTrail
	p.noTrail = 0
	for !p.isP("}") && p.t().K != tEOF {
		if p.isP(";") {
			p.next()
			continue
		}
		s := p.parseStmt()
		if s != nil {
			d.Members = append(d.Members, s)
		}
	}
	p.noTrail = saved
	p.expectP("}")
	return d
}

func (p *Parser) parseEnumCase() Stmt {
	pos := p.next().Pos
	d := &EnumCaseDecl{Pos: pos}
	for {
		c := &EnumCase{Pos: p.t().Pos, Name: p.expectIdent()}
		if p.isP("(") {
			p.next()
			for !p.isP(")") {
				prm := &Param{}
				if p.t().K == tIdent && p.peek(1).K == tPunct && p.peek(1).S == ":" {
					prm.Label = p.next().S
					prm.Name = prm.Label
					p.next()
				} else if p.t().K == tIdent && p.t().S == "_" && p.peek(1).K == tIdent && p.peek(2).S == ":" {
					p.next()
					prm.Name = p.next().S
					p.next()
				}
				prm.Type = p.parseType()
				if p.isOp("=") {
					p.next()
					prm.Default = p.parseExpr()
				}
				c.Params = append(c.Params, prm)
				if p.isP(",") {
					p.next()
				}
			}
			p.expectP(")")
		}
		if p.isOp("=") {
			p.next()
			c.Raw = p.parseExpr()
		}
		d.Cases = append(d.Cases, c)
		if p.isP(",") {
			p.next()
			continue
		}
		break
	}
	return d
}

func (p *Parser) parseParams() []*Param {
	p.expectP("(")
	var params []*Param
	for !p.isP(")") {
		prm := &Param{}
		prm.Attrs = p.parseAttrs()
		t := p.t()
		if t.K != tIdent {
			p.errf(t.Pos, "expected parameter name but found %s", p.describe(t))
		}
		first := p.next().S
		if p.t().K == tIdent {
			second := p.next().S
			prm.Label = first
			prm.Name = second
		} else {
			prm.Label = first
			prm.Name = first
		}
		if prm.Label == "_" {
			prm.Label = ""
		}
		p.expectP(":")
		prm.Attrs = append(prm.Attrs, p.parseAttrs()...)
		if p.isKw("inout") {
			p.next()
			prm.InOut = true
		}
		prm.Attrs = append(prm.Attrs, p.parseAttrs()...)
		prm.Type = p.parseType()
		if p.isOp("...") {
			p.next()
			prm.Variadic = true
		}
		if p.isOp("=") {
			p.next()
			prm.Default = p.parseExpr()
		}
		params = append(params, prm)
		if p.isP(",") {
			p.next()
		} else if !p.isP(")") {
			p.errf(p.t().Pos, "expected ',' or ')' in parameter list")
		}
	}
	p.expectP(")")
	return params
}

func (p *Parser) parseFuncTail(f *FuncDecl) {
	for {
		if p.isKw("async") || p.isKw("throws") || p.isKw("rethrows") || p.isKw("reasync") {
			if p.t().S != "async" {
				f.Throws = true
			} else {
				f.Async = true
			}
			p.next()
			if p.isP("(") {
				p.skipBalanced("(", ")")
			}
			continue
		}
		break
	}
	if p.isP("->") {
		p.next()
		f.Result = p.parseType()
	}
	p.skipWhere()
	if p.isP("{") {
		f.Body = p.parseBlock()
		f.HasBody = true
	}
}

func (p *Parser) parseFunc(attrs []*Attr, mods map[string]bool) Stmt {
	pos := p.next().Pos
	t := p.t()
	name := ""
	if t.K == tIdent || t.K == tOp {
		name = t.S
		p.next()
	} else {
		p.errf(t.Pos, "expected function name")
	}
	generics := p.parseGenericParamNames()
	f := &FuncDecl{Pos: pos, Name: name, Attrs: attrs, Mods: mods, Generics: generics}
	f.Params = p.parseParams()
	if t.K == tOp {
		for _, prm := range f.Params {
			prm.Label = ""
		}
	}
	p.parseFuncTail(f)
	return f
}

func (p *Parser) parseInit(attrs []*Attr, mods map[string]bool) Stmt {
	pos := p.next().Pos
	f := &FuncDecl{Pos: pos, Name: "init", IsInit: true, Attrs: attrs, Mods: mods}
	if p.t().K == tOp && p.t().Postfix {
		f.Failable = p.t().S == "?"
		p.next()
	}
	p.skipAngles()
	f.Params = p.parseParams()
	p.parseFuncTail(f)
	return f
}

func (p *Parser) parseSubscript(attrs []*Attr, mods map[string]bool) Stmt {
	pos := p.next().Pos
	f := &FuncDecl{Pos: pos, Name: "subscript", IsSubscript: true, Attrs: attrs, Mods: mods}
	p.skipAngles()
	f.Params = p.parseParams()
	if p.isP("->") {
		p.next()
		f.Result = p.parseType()
	}
	p.skipWhere()
	if p.isP("{") {
		v := &VarDecl{}
		p.parseAccessors(v)
		f.Body = v.Get
		f.SetBody = v.Set
		f.SetName = v.SetName
		f.HasBody = true
	}
	return f
}

func (p *Parser) isAccessorStart() bool {
	t := p.t()
	if t.K != tIdent || t.Esc {
		return false
	}
	switch t.S {
	case "get", "set", "willSet", "didSet", "_modify", "_read":
		n := p.peek(1)
		return (n.K == tPunct && (n.S == "{" || n.S == "(" || n.S == "}")) || (n.K == tIdent && (n.S == "async" || n.S == "throws" || n.S == "set" || n.S == "get"))
	case "mutating", "nonmutating":
		n := p.peek(1)
		return n.K == tIdent && (n.S == "get" || n.S == "set")
	}
	return false
}

func (p *Parser) parseAccessors(v *VarDecl) {
	p.expectP("{")
	if !p.isAccessorStart() {
		saved := p.noTrail
		p.noTrail = 0
		v.Get = p.parseStmtsUntil()
		p.noTrail = saved
		p.expectP("}")
		v.Computed = true
		return
	}
	for !p.isP("}") {
		for p.isKw("mutating") || p.isKw("nonmutating") {
			p.next()
		}
		kw := p.expectIdent()
		name := ""
		if p.isP("(") {
			p.next()
			name = p.expectIdent()
			p.expectP(")")
		}
		for p.isKw("async") || p.isKw("throws") {
			p.next()
		}
		var body []Stmt
		hasBody := false
		if p.isP("{") {
			body = p.parseBlock()
			hasBody = true
		}
		switch kw {
		case "get":
			v.Get = body
			v.Computed = true
			if !hasBody {
				v.Computed = false
			}
		case "set":
			v.Set = body
			if name == "" {
				name = "newValue"
			}
			v.SetName = name
		case "willSet":
			v.WillSet = body
			if name == "" {
				name = "newValue"
			}
			v.SetName = name
		case "didSet":
			v.DidSet = body
			if name != "" {
				v.SetName = name
			}
		}
	}
	p.expectP("}")
}

func (p *Parser) parseVar(attrs []*Attr, mods map[string]bool) Stmt {
	kw := p.next()
	var out []Stmt
	for {
		v := &VarDecl{Pos: p.t().Pos, Let: kw.S == "let", Attrs: attrs, Mods: mods}
		if p.isP("(") {
			v.Pat = p.parsePattern(true)
		} else {
			v.Name = p.expectIdent()
		}
		if p.isP(":") {
			p.next()
			v.Type = p.parseType()
		}
		if p.isOp("=") {
			p.next()
			v.Init = p.parseExpr()
		}
		if p.isP("{") && !p.t().NL || (p.isP("{") && v.Init == nil && v.Type != nil) {
			p.parseAccessors(v)
		}
		out = append(out, v)
		if p.isP(",") {
			p.next()
			continue
		}
		break
	}
	if len(out) == 1 {
		return out[0]
	}
	return &BlockStmt{Body: out}
}

// ---------- patterns ----------

func (p *Parser) parsePattern(bind bool) Pattern {
	t := p.t()
	var pat Pattern
	switch {
	case t.K == tIdent && !t.Esc && (t.S == "let" || t.S == "var"):
		p.next()
		pat = p.parsePattern(true)
	case t.K == tIdent && t.S == "_" && !t.Esc:
		p.next()
		pat = &WildPat{}
	case t.K == tIdent && !t.Esc && t.S == "is":
		p.next()
		pat = &IsPat{T: p.parseType()}
	case t.K == tPunct && t.S == "(":
		p.next()
		tp := &TuplePat{}
		for !p.isP(")") {
			label := ""
			if p.t().K == tIdent && p.peek(1).K == tPunct && p.peek(1).S == ":" {
				label = p.next().S
				p.next()
			}
			tp.Labels = append(tp.Labels, label)
			tp.Elems = append(tp.Elems, p.parsePattern(bind))
			if p.isP(",") {
				p.next()
			}
		}
		p.expectP(")")
		if len(tp.Elems) == 1 && tp.Labels[0] == "" {
			pat = tp.Elems[0]
		} else {
			pat = tp
		}
	case t.K == tPunct && t.S == ".":
		p.next()
		ep := &EnumPat{Case: p.expectIdent()}
		p.parseEnumSubs(ep, bind)
		pat = ep
	case t.K == tIdent && bind && !(p.peek(1).K == tPunct && (p.peek(1).S == "." || p.peek(1).S == "(")):
		p.next()
		pat = &BindPat{Name: t.S}
	case t.K == tIdent && len(t.S) > 0 && t.S[0] >= 'A' && t.S[0] <= 'Z' && p.peek(1).K == tPunct && p.peek(1).S == "." && p.peek(2).K == tIdent && !(p.peek(3).K == tPunct && p.peek(3).S == "."):
		typ := p.next().S
		p.next()
		ep := &EnumPat{Type: typ, Case: p.expectIdent()}
		p.parseEnumSubs(ep, bind)
		pat = ep
	default:
		pat = &ExprPat{X: p.parseBinary(3)}
	}
	for {
		if p.t().K == tOp && p.t().Postfix && p.t().S == "?" {
			p.next()
			pat = &OptPat{Sub: pat}
			continue
		}
		if p.isKw("as") {
			p.next()
			pat = &AsPat{Sub: pat, T: p.parseType()}
			continue
		}
		break
	}
	return pat
}

func (p *Parser) parseEnumSubs(ep *EnumPat, bind bool) {
	if p.isP("(") && !p.t().NL {
		ep.HasSub = true
		p.next()
		for !p.isP(")") {
			label := ""
			if p.t().K == tIdent && p.peek(1).K == tPunct && p.peek(1).S == ":" {
				label = p.next().S
				p.next()
			}
			ep.Labels = append(ep.Labels, label)
			ep.Subs = append(ep.Subs, p.parsePattern(bind))
			if p.isP(",") {
				p.next()
			}
		}
		p.expectP(")")
	}
}

// ---------- types ----------

func (p *Parser) closeAngle() {
	t := p.t()
	if t.K == tOp && strings.HasPrefix(t.S, ">") {
		if len(t.S) == 1 {
			p.next()
		} else {
			t.S = t.S[1:]
			t.SpB = false
			if t.S == "?" || t.S == "!" {
				t.Postfix = true
			}
		}
		return
	}
	p.errf(t.Pos, "expected '>' in generic argument list")
}

func (p *Parser) parseType() *TypeRef {
	for p.t().K == tAttr {
		p.parseAttrs()
	}
	for p.isKw("inout") || p.isKw("borrowing") || p.isKw("consuming") || p.isKw("sending") {
		p.next()
	}
	if p.isKw("some") || p.isKw("any") {
		p.next()
		t := p.parseType()
		t.Some = true
		return t
	}
	var ty *TypeRef
	t := p.t()
	switch {
	case t.K == tPunct && t.S == "(":
		p.next()
		var elems []*TypeRef
		for !p.isP(")") {
			if p.t().K == tIdent && p.peek(1).K == tPunct && p.peek(1).S == ":" {
				p.next()
				p.next()
			} else if p.t().K == tIdent && p.peek(1).K == tIdent && p.peek(2).K == tPunct && p.peek(2).S == ":" {
				p.next()
				p.next()
				p.next()
			}
			elems = append(elems, p.parseType())
			if p.isOp("...") {
				p.next()
			}
			if p.isP(",") {
				p.next()
			}
		}
		p.expectP(")")
		isFunc := false
		for p.isKw("async") || p.isKw("throws") || p.isKw("rethrows") {
			p.next()
			isFunc = true
		}
		if p.isP("->") {
			p.next()
			ty = &TypeRef{Kind: tyFunc, Params: elems, Result: p.parseType()}
			return ty
		} else if isFunc {
			ty = &TypeRef{Kind: tyFunc, Params: elems, Result: &TypeRef{Name: "Void"}}
		} else if len(elems) == 1 {
			ty = elems[0]
		} else if len(elems) == 0 {
			ty = &TypeRef{Name: "Void"}
		} else {
			ty = &TypeRef{Kind: tyTuple, Params: elems}
		}
	case t.K == tPunct && t.S == "[":
		p.next()
		k := p.parseType()
		if p.isP(":") {
			p.next()
			v := p.parseType()
			p.expectP("]")
			ty = &TypeRef{Kind: tyDict, Key: k, Elem: v, Name: "Dictionary"}
		} else {
			p.expectP("]")
			ty = &TypeRef{Kind: tyArray, Elem: k, Name: "Array"}
		}
	case t.K == tIdent:
		name := p.next().S
		ty = &TypeRef{Name: name}
		if p.t().K == tOp && strings.HasPrefix(p.t().S, "<") && !p.t().SpB {
			ty.Args = p.parseGenericArgs()
		}
		for p.isP(".") && p.peek(1).K == tIdent {
			p.next()
			n := p.next().S
			if n == "Type" || n == "Protocol" {
				continue
			}
			ty.Name += "." + n
			if p.t().K == tOp && strings.HasPrefix(p.t().S, "<") && !p.t().SpB {
				ty.Args = p.parseGenericArgs()
			}
		}
		switch ty.Name {
		case "Array":
			if len(ty.Args) == 1 {
				ty = &TypeRef{Kind: tyArray, Elem: ty.Args[0], Name: "Array"}
			}
		case "Dictionary":
			if len(ty.Args) == 2 {
				ty = &TypeRef{Kind: tyDict, Key: ty.Args[0], Elem: ty.Args[1], Name: "Dictionary"}
			}
		case "Optional":
			if len(ty.Args) == 1 {
				ty = ty.Args[0]
				ty.Opt = true
			}
		}
	default:
		p.errf(t.Pos, "expected type but found %s", p.describe(t))
	}
	for {
		t := p.t()
		if t.K == tOp && t.Postfix && (t.S == "?" || t.S == "!") {
			p.next()
			ty.Opt = true
			continue
		}
		if t.K == tOp && (t.S == "?" || t.S == "!") && !t.SpB {
			p.next()
			ty.Opt = true
			continue
		}
		if t.K == tOp && t.S == "&" && p.peek(1).K == tIdent {
			p.next()
			p.parseType()
			continue
		}
		break
	}
	return ty
}

func (p *Parser) parseGenericArgs() []*TypeRef {
	t := p.t()
	if t.S == "<" {
		p.next()
	} else {
		t.S = t.S[1:]
	}
	var args []*TypeRef
	for {
		args = append(args, p.parseType())
		if p.isP(",") {
			p.next()
			continue
		}
		break
	}
	p.closeAngle()
	return args
}

// ---------- expressions ----------

type opInfo struct {
	prec  int
	right bool
}

var binOps = map[string]opInfo{
	"=": {1, true}, "+=": {1, true}, "-=": {1, true}, "*=": {1, true}, "/=": {1, true}, "%=": {1, true},
	"&=": {1, true}, "|=": {1, true}, "^=": {1, true}, "<<=": {1, true}, ">>=": {1, true},
	"?":  {2, true},
	"||": {3, false}, "&&": {4, false},
	"==": {5, false}, "!=": {5, false}, "<": {5, false}, ">": {5, false}, "<=": {5, false}, ">=": {5, false},
	"===": {5, false}, "!==": {5, false}, "~=": {5, false},
	"??": {6, true},
	"is": {7, false}, "as": {7, false},
	"...": {8, false}, "..<": {8, false},
	"+": {9, false}, "-": {9, false}, "|": {9, false}, "^": {9, false}, "&+": {9, false}, "&-": {9, false},
	"*": {10, false}, "/": {10, false}, "%": {10, false}, "&": {10, false}, "&*": {10, false},
	"<<": {11, false}, ">>": {11, false},
}

func (p *Parser) parseExpr() Expr { return p.parseBinary(0) }

func (p *Parser) parseBinary(minPrec int) Expr {
	left := p.parseUnary()
	for {
		t := p.t()
		var op string
		isCast := false
		if t.K == tOp && !t.Postfix {
			op = t.S
		} else if t.K == tIdent && !t.Esc && (t.S == "is" || t.S == "as") {
			op = t.S
			isCast = true
		} else {
			break
		}
		if t.NL && !t.SpA && !isCast {
			break
		}
		if (op == "..." || op == "..<") && !t.SpB {
			n := p.peek(1)
			if n.K == tPunct && (n.S == "]" || n.S == ")" || n.S == "," || n.S == "}" || n.S == ":" || n.S == ";") || n.K == tEOF || n.NL {
				p.next()
				left = &PostfixRange{Pos: t.Pos, X: left}
				continue
			}
		}
		info, ok := binOps[op]
		if !ok {
			p.errf(t.Pos, "unknown operator '%s'", op)
		}
		if info.prec < minPrec {
			break
		}
		p.next()
		switch {
		case op == "?":
			mid := p.parseBinary(2)
			p.expectP(":")
			rhs := p.parseBinary(2)
			left = &TernaryExpr{Pos: t.Pos, C: left, T: mid, F: rhs}
		case isCast:
			kind := op
			if p.t().K == tOp && (p.t().S == "?" || p.t().S == "!") {
				kind += p.next().S
			}
			left = &CastExpr{Pos: t.Pos, X: left, Kind: kind, T: p.parseType()}
		case info.prec == 1:
			rhs := p.parseBinary(1)
			left = &AssignExpr{Pos: t.Pos, Op: op, L: left, R: rhs}
		default:
			np := info.prec + 1
			if info.right {
				np = info.prec
			}
			rhs := p.parseBinary(np)
			left = &BinaryExpr{Pos: t.Pos, Op: op, L: left, R: rhs}
		}
	}
	return left
}

func (p *Parser) parseUnary() Expr {
	t := p.t()
	if t.K == tOp {
		switch t.S {
		case "-", "+", "!", "~", "&", "...", "..<":
			p.next()
			if t.S == "-" {
				n := p.t()
				if n.K == tInt && !n.SpB {
					p.next()
					return p.parsePostfix(&IntLit{Pos: t.Pos, V: -n.IntV})
				}
				if n.K == tFloat && !n.SpB {
					p.next()
					return p.parsePostfix(&FloatLit{Pos: t.Pos, V: -n.FloatV})
				}
			}
			x := p.parseUnary()
			if t.S == "&" {
				return &InOutExpr{Pos: t.Pos, X: x}
			}
			return &UnaryExpr{Pos: t.Pos, Op: t.S, X: x}
		}
	}
	if t.K == tIdent && !t.Esc && (t.S == "try" || t.S == "await") {
		p.next()
		kind := t.S
		if kind == "try" && p.t().K == tOp && p.t().Postfix {
			kind += p.next().S
		}
		x := p.parseBinary(2)
		if kind == "await" {
			return &AwaitExpr{Pos: t.Pos, X: x}
		}
		return &TryExpr{Pos: t.Pos, Kind: kind, X: x}
	}
	return p.parsePostfix(p.parsePrimary())
}

func canTrail(x Expr) bool {
	switch x.(type) {
	case *Ident, *MemberExpr, *CallExpr, *ImplicitMember, *SubscriptExpr, *OptMark, *ForceUnwrap:
		return true
	}
	return false
}

func (p *Parser) parsePostfix(x Expr) Expr {
	hasOpt := false
	for {
		t := p.t()
		switch {
		case t.K == tPunct && t.S == ".":
			p.next()
			n := p.t()
			var name string
			if n.K == tInt {
				name = n.S
				p.next()
			} else if n.K == tFloat {
				parts := strings.SplitN(n.S, ".", 2)
				p.next()
				x = &MemberExpr{Pos: n.Pos, X: x, Name: parts[0]}
				name = parts[1]
			} else if n.K == tIdent {
				name = n.S
				p.next()
			} else {
				p.errf(n.Pos, "expected member name following '.'")
			}
			x = &MemberExpr{Pos: n.Pos, X: x, Name: name}
		case t.K == tPunct && t.S == "(" && !t.NL && p.isCompoundNameRef():
			// `AlarmTone.init(rawValue:)` / `UUID.init(uuidString:)` / `foo(a:b:)`:
			// a reference to a function by its full name, not a call. Desugar to
			// `{ p0, p1 in foo(a: p0, b: p1) }` so it can be passed to map/flatMap.
			x = p.parseCompoundNameRef(x)
		case t.K == tPunct && t.S == "(" && !t.NL:
			args := p.parseArgs("(", ")")
			x = &CallExpr{Pos: t.Pos, Fn: x, Args: args}
			x = p.parseTrailing(x)
		case t.K == tPunct && t.S == "[" && !t.NL:
			args := p.parseArgs("[", "]")
			x = &SubscriptExpr{Pos: t.Pos, X: x, Args: args}
		case t.K == tPunct && t.S == "{" && !t.NL && p.noTrail == 0 && canTrail(x) && !p.isObserverBlock():
			x = p.parseTrailing(x)
		case t.K == tOp && t.Postfix && t.S == "!":
			p.next()
			x = &ForceUnwrap{Pos: t.Pos, X: x}
		case t.K == tOp && t.Postfix && t.S == "?":
			p.next()
			x = &OptMark{Pos: t.Pos, X: x}
			hasOpt = true
		case t.K == tOp && strings.HasPrefix(t.S, "<") && !t.SpB && isTypeish(x):
			save := p.i
			saveTok := p.toks[p.i]
			var typeArgs []*TypeRef
			ok := p.attempt(func() {
				typeArgs = p.parseGenericArgs()
				if !(p.isP("(") || p.isP(".")) {
					p.errf(p.t().Pos, "not generic")
				}
			})
			if !ok {
				p.i = save
				p.toks[p.i] = saveTok
				goto done
			}
			x = &GenericTypeExpr{Pos: t.Pos, X: x, Args: typeArgs}
		default:
			goto done
		}
	}
done:
	if hasOpt {
		x = &OptChain{Pos: x.P(), X: x}
	}
	return x
}

// isCompoundNameRef reports whether the tokens at the current '(' spell a
// compound function name like `(rawValue:)` or `(a:b:)`: only labels, each
// followed directly by ':', with no expressions between them.
func (p *Parser) isCompoundNameRef() bool {
	if !p.isP("(") {
		return false
	}
	i := 1
	labels := 0
	for {
		t := p.peek(i)
		if t.K == tPunct && t.S == ")" {
			return labels > 0
		}
		if t.K != tIdent {
			return false
		}
		c := p.peek(i + 1)
		if c.K != tPunct || c.S != ":" {
			return false
		}
		labels++
		i += 2
	}
}

func (p *Parser) parseCompoundNameRef(fn Expr) Expr {
	open := p.expectP("(")
	c := &ClosureExpr{Pos: open.Pos, HasSig: true}
	call := &CallExpr{Pos: open.Pos, Fn: fn}
	for !p.isP(")") {
		label := p.expectIdent()
		p.expectP(":")
		name := fmt.Sprintf("__ref%d", len(c.Params))
		c.Params = append(c.Params, &Param{Name: name})
		argLabel := label
		if argLabel == "_" {
			argLabel = ""
		}
		call.Args = append(call.Args, Arg{Label: argLabel, X: &Ident{Pos: open.Pos, Name: name}})
	}
	p.expectP(")")
	c.Body = []Stmt{&ExprStmt{X: call}}
	return c
}

func isTypeish(x Expr) bool {
	if id, ok := x.(*Ident); ok {
		return len(id.Name) > 0 && id.Name[0] >= 'A' && id.Name[0] <= 'Z'
	}
	return false
}

func (p *Parser) isObserverBlock() bool {
	n := p.peek(1)
	if n.K == tIdent && (n.S == "didSet" || n.S == "willSet") {
		return true
	}
	return false
}

func (p *Parser) parseTrailing(x Expr) Expr {
	var args []Arg
	if p.isP("{") && !p.t().NL && p.noTrail == 0 && !p.isObserverBlock() {
		args = append(args, Arg{X: p.parseClosure(), Trailing: true})
		for {
			t := p.t()
			if t.K == tIdent && !t.NL && p.peek(1).K == tPunct && p.peek(1).S == ":" && p.peek(2).K == tPunct && p.peek(2).S == "{" {
				label := p.next().S
				p.next()
				args = append(args, Arg{Label: label, X: p.parseClosure(), Trailing: true})
				continue
			}
			break
		}
	}
	if len(args) == 0 {
		return x
	}
	if c, ok := x.(*CallExpr); ok {
		c.Args = append(c.Args, args...)
		return c
	}
	return &CallExpr{Pos: x.P(), Fn: x, Args: args}
}

func (p *Parser) parseArgs(open, close string) []Arg {
	p.expectP(open)
	saved := p.noTrail
	p.noTrail = 0
	var args []Arg
	for !p.isP(close) {
		a := Arg{}
		t := p.t()
		if t.K == tIdent && p.peek(1).K == tPunct && p.peek(1).S == ":" {
			a.Label = t.S
			p.next()
			p.next()
		}
		t = p.t()
		if t.K == tOp && (p.peek(1).S == "," || p.peek(1).S == close) && p.peek(1).K == tPunct {
			p.next()
			a.X = &Ident{Pos: t.Pos, Name: t.S}
		} else {
			a.X = p.parseExpr()
		}
		args = append(args, a)
		if p.isP(",") {
			p.next()
		} else if !p.isP(close) {
			p.errf(p.t().Pos, "expected ',' or '%s' in argument list but found %s", close, p.describe(p.t()))
		}
	}
	p.noTrail = saved
	p.expectP(close)
	return args
}

func (p *Parser) parseStrLit(t *Token) *StrLit {
	s := &StrLit{Pos: t.Pos}
	for _, seg := range t.Segs {
		if !seg.IsInterp {
			s.Pieces = append(s.Pieces, StrPiece{Lit: seg.Lit})
			continue
		}
		toks, err := lexAt(p.file, seg.Src, seg.IPos.Line, seg.IPos.Col)
		if err != nil {
			panic(err)
		}
		sub := &Parser{toks: toks, file: p.file}
		x := sub.parseExpr()
		var extra []Arg
		for sub.isP(",") {
			sub.next()
			a := Arg{}
			if sub.t().K == tIdent && sub.peek(1).K == tPunct && sub.peek(1).S == ":" {
				a.Label = sub.next().S
				sub.next()
			}
			a.X = sub.parseExpr()
			extra = append(extra, a)
		}
		if sub.t().K != tEOF {
			p.errf(sub.t().Pos, "unexpected %s in string interpolation", sub.describe(sub.t()))
		}
		s.Pieces = append(s.Pieces, StrPiece{X: x, Args: extra})
	}
	return s
}

func (p *Parser) parsePrimary() Expr {
	t := p.t()
	switch t.K {
	case tInt:
		p.next()
		return &IntLit{Pos: t.Pos, V: t.IntV}
	case tFloat:
		p.next()
		return &FloatLit{Pos: t.Pos, V: t.FloatV}
	case tString:
		p.next()
		return p.parseStrLit(t)
	case tIdent:
		if !t.Esc {
			switch t.S {
			case "true", "false":
				p.next()
				return &BoolLit{Pos: t.Pos, V: t.S == "true"}
			case "nil":
				p.next()
				return &NilLit{Pos: t.Pos}
			case "if":
				s := p.parseIf()
				return &IfExpr{Pos: t.Pos, S: s}
			case "switch":
				s := p.parseSwitch().(*SwitchStmt)
				return &SwitchExpr{Pos: t.Pos, S: s}
			}
		}
		p.next()
		return &Ident{Pos: t.Pos, Name: t.S}
	case tPunct:
		switch t.S {
		case "(":
			args := p.parseArgs("(", ")")
			if len(args) == 1 && args[0].Label == "" {
				return args[0].X
			}
			te := &TupleExpr{Pos: t.Pos}
			for _, a := range args {
				te.Elems = append(te.Elems, a.X)
				te.Labels = append(te.Labels, a.Label)
			}
			return te
		case "[":
			p.next()
			saved := p.noTrail
			p.noTrail = 0
			defer func() { p.noTrail = saved }()
			if p.isP("]") {
				p.next()
				return &ArrayLit{Pos: t.Pos}
			}
			if p.isP(":") {
				p.next()
				p.expectP("]")
				return &DictLit{Pos: t.Pos}
			}
			first := p.parseExpr()
			if p.isP(":") {
				d := &DictLit{Pos: t.Pos}
				p.next()
				d.Keys = append(d.Keys, first)
				d.Vals = append(d.Vals, p.parseExpr())
				for p.isP(",") {
					p.next()
					if p.isP("]") {
						break
					}
					d.Keys = append(d.Keys, p.parseExpr())
					p.expectP(":")
					d.Vals = append(d.Vals, p.parseExpr())
				}
				p.expectP("]")
				return d
			}
			a := &ArrayLit{Pos: t.Pos, Elems: []Expr{first}}
			for p.isP(",") {
				p.next()
				if p.isP("]") {
					break
				}
				a.Elems = append(a.Elems, p.parseExpr())
			}
			p.expectP("]")
			return a
		case "{":
			return p.parseClosure()
		case ".":
			p.next()
			n := p.t()
			if n.K != tIdent {
				p.errf(n.Pos, "expected identifier after '.'")
			}
			p.next()
			return &ImplicitMember{Pos: t.Pos, Name: n.S}
		case "\\":
			p.next()
			kp := &KeyPathExpr{Pos: t.Pos}
			if p.t().K == tIdent {
				kp.Root = p.next().S
			}
			for p.isP(".") {
				p.next()
				n := p.t()
				if n.K == tIdent || n.K == tInt {
					kp.Path = append(kp.Path, n.S)
					p.next()
				} else {
					break
				}
			}
			return kp
		}
	case tPound:
		p.next()
		switch t.S {
		case "available":
			if p.isP("(") {
				p.skipBalanced("(", ")")
			}
			return &BoolLit{Pos: t.Pos, V: true}
		case "unavailable":
			if p.isP("(") {
				p.skipBalanced("(", ")")
			}
			return &BoolLit{Pos: t.Pos, V: false}
		case "file", "filePath", "fileID":
			return &StrLit{Pos: t.Pos, Pieces: []StrPiece{{Lit: t.Pos.File}}}
		case "line":
			return &IntLit{Pos: t.Pos, V: int64(t.Pos.Line)}
		case "function":
			return &StrLit{Pos: t.Pos, Pieces: []StrPiece{{Lit: "function"}}}
		case "colorLiteral":
			args := p.parseArgs("(", ")")
			return &CallExpr{Pos: t.Pos, Fn: &Ident{Pos: t.Pos, Name: "Color"}, Args: args}
		case "Predicate", "Expression":
			if n := p.t(); n.K == tOp && strings.HasPrefix(n.S, "<") {
				p.parseGenericArgs()
			}
			if !p.isP("{") {
				p.errf(p.t().Pos, "expected '{' to start the #%s closure", t.S)
			}
			return p.parseClosure()
		}
		p.errf(t.Pos, "unsupported directive '#%s'", t.S)
	}
	p.errf(t.Pos, "expected expression but found %s", p.describe(t))
	return nil
}

// Finds the largest $n in a token, looking inside string interpolations too.
func highestShorthandArgument(tok Token) int {
	highest := 0
	if tok.K == tIdent && strings.HasPrefix(tok.S, "$") {
		if n, err := strconv.Atoi(tok.S[1:]); err == nil {
			highest = n
		}
	}
	for _, seg := range tok.Segs {
		if !seg.IsInterp {
			continue
		}
		for _, field := range strings.FieldsFunc(seg.Src, func(r rune) bool { return r != '$' && (r < '0' || r > '9') }) {
			if n, err := strconv.Atoi(strings.TrimPrefix(field, "$")); err == nil && strings.HasPrefix(field, "$") {
				highest = max(highest, n)
			}
		}
	}
	return highest
}

func (p *Parser) parseClosure() *ClosureExpr {
	t := p.expectP("{")
	c := &ClosureExpr{Pos: t.Pos}
	saved := p.noTrail
	p.noTrail = 0
	save := p.i
	ok := p.attempt(func() {
		if p.isP("[") {
			p.skipBalanced("[", "]")
		}
		var params []*Param
		if p.isP("(") {
			p.next()
			for !p.isP(")") {
				prm := &Param{}
				n1 := p.expectIdent()
				prm.Name = n1
				if p.t().K == tIdent {
					prm.Name = p.next().S
				}
				if p.isP(":") {
					p.next()
					prm.Type = p.parseType()
				}
				params = append(params, prm)
				if p.isP(",") {
					p.next()
				} else if !p.isP(")") {
					p.errf(p.t().Pos, "bad closure params")
				}
			}
			p.next()
		} else if p.t().K == tIdent && !p.isKw("in") {
			for {
				params = append(params, &Param{Name: p.expectIdent()})
				if p.isP(",") {
					p.next()
					continue
				}
				break
			}
		}
		for p.isKw("async") || p.isKw("throws") {
			p.next()
		}
		if p.isP("->") {
			p.next()
			p.parseType()
		}
		if !p.isKw("in") {
			p.errf(p.t().Pos, "no in")
		}
		p.next()
		c.Params = params
		c.HasSig = true
	})
	if !ok {
		p.i = save
	}
	bodyStart := p.i
	c.Body = p.parseStmtsUntil()
	for _, tok := range p.toks[bodyStart:p.i] {
		c.MaxDollar = max(c.MaxDollar, highestShorthandArgument(tok))
	}
	p.noTrail = saved
	p.expectP("}")
	return c
}
