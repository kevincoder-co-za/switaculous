package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

type Slot struct {
	V      Value
	Let    bool
	T      *TypeRef
	Get    func() Value
	Set    func(Value)
	Uninit bool
}

func (s *Slot) get() Value {
	if s.Get != nil {
		return s.Get()
	}
	return s.V
}

type Env struct {
	vars        map[string]*Slot
	parent      *Env
	self        Value
	selfT       *TypeInfo
	staticT     *TypeInfo
	mutableSelf bool
	inInit      bool
}

func newEnv(parent *Env) *Env { return &Env{vars: map[string]*Slot{}, parent: parent} }

type ctrl int

const (
	cNone ctrl = iota
	cBreak
	cContinue
	cReturn
	cFallthrough
)

type FnCtx struct {
	collect       []Value
	keys          []string
	pos           []string
	ret           Value
	lastExprValue Value
	isInit        bool
	returnedNil   bool
	replacedSelf  Value
	label         string
}

type Interp struct {
	types     map[string]*TypeInfo
	ext       map[string]*TypeInfo
	aliases   map[string]*TypeRef
	declNames map[*TypeDecl]string
	globals   *Env
	previews  []*PreviewInfo
	mainApp   *TypeInfo
	objID     int
	store     *Store
	rt        *Runtime
	render    *Render
	depth     int
	gen       int
	printFn   func(string)
	lastSelf  Value
	inTask    int
	warnFn    func(Pos, string)

	memoryContexts map[string]*ModelContextV
	directCall     bool
	expectedType   *TypeRef
}

func NewInterp(store *Store) *Interp {
	ip := &Interp{
		types:     map[string]*TypeInfo{},
		ext:       map[string]*TypeInfo{},
		aliases:   map[string]*TypeRef{},
		declNames: map[*TypeDecl]string{},
		store:     store,
	}
	ip.globals = newEnv(nil)
	ip.printFn = func(s string) { fmt.Println(s) }
	ip.warnFn = func(Pos, string) {}
	return ip
}

func (ip *Interp) fail(pos Pos, f string, a ...interface{}) {
	panic(&RuntimeError{Pos: pos, Msg: fmt.Sprintf(f, a...)})
}

func (ip *Interp) warn(pos Pos, f string, a ...interface{}) {
	ip.warnFn(pos, fmt.Sprintf(f, a...))
}

// ---------- statements ----------

func (ip *Interp) execBody(body []Stmt, env *Env, fc *FnCtx) Value {
	if len(body) == 1 {
		if es, ok := body[0].(*ExprStmt); ok {
			if _, isAssign := es.X.(*AssignExpr); !isAssign {
				return ip.eval(es.X, env)
			}
		}
	}
	c := ip.execBlock(body, env, fc)
	if c == cReturn {
		return fc.ret
	}
	if len(fc.collect) > 0 {
		return &ViewV{Kind: "Tuple", Kids: fc.collect, Keys: fc.keys}
	}
	// A body that is just an `if` or `switch` whose branches are single
	// expressions is an expression itself (Swift 5.9), so its value is the result.
	if len(body) == 1 && fc.lastExprValue != nil {
		switch body[0].(type) {
		case *IfStmt, *SwitchStmt:
			return fc.lastExprValue
		}
	}
	return Void
}

func (ip *Interp) execBlock(stmts []Stmt, env *Env, fc *FnCtx) (c ctrl) {
	var defers [][]Stmt
	defer func() {
		for i := len(defers) - 1; i >= 0; i-- {
			ip.execBlock(defers[i], env, &FnCtx{})
		}
	}()
	for i, s := range stmts {
		if d, ok := s.(*DeferStmt); ok {
			defers = append(defers, d.Body)
			continue
		}
		fc.pos = append(fc.pos, strconv.Itoa(i))
		c = ip.exec(s, env, fc)
		fc.pos = fc.pos[:len(fc.pos)-1]
		if c != cNone {
			return c
		}
	}
	return cNone
}

func (ip *Interp) exec(s Stmt, env *Env, fc *FnCtx) ctrl {
	switch x := s.(type) {
	case *ExprStmt:
		v := ip.eval(x.X, env)
		if isViewValue(v) {
			fc.collect = append(fc.collect, v)
			fc.keys = append(fc.keys, strings.Join(fc.pos, "."))
		}
		if _, isAssign := x.X.(*AssignExpr); !isAssign {
			fc.lastExprValue = v
		}
		return cNone
	case *VarDecl:
		ip.execVarDecl(x, env)
		return cNone
	case *BlockStmt:
		for _, b := range x.Body {
			if c := ip.exec(b, env, fc); c != cNone {
				return c
			}
		}
		return cNone
	case *FuncDecl:
		env.vars[x.Name] = &Slot{V: &FuncV{Name: x.Name, Params: x.Params, Body: x.Body, Env: env, Decl: x, ResultT: x.Result, HasSig: true}, Let: true}
		return cNone
	case *TypeDecl:
		if x.Kind != "extension" {
			for _, d := range ip.registerType(x, "") {
				ti := ip.types[ip.fullName(d)]
				ip.addMembers(ti, d.Members)
				for _, inh := range d.Inherits {
					ti.Conforms[inh] = true
				}
			}
		}
		return cNone
	case *IfStmt:
		cenv := newEnv(env)
		if ip.evalConds(x.Conds, cenv) {
			fc.pos = append(fc.pos, "t")
			c := ip.execBlock(x.Then, cenv, fc)
			fc.pos = fc.pos[:len(fc.pos)-1]
			return c
		}
		fc.pos = append(fc.pos, "e")
		defer func() { fc.pos = fc.pos[:len(fc.pos)-1] }()
		switch el := x.Else.(type) {
		case *IfStmt:
			return ip.exec(el, env, fc)
		case *BlockStmt:
			return ip.execBlock(el.Body, newEnv(env), fc)
		}
		return cNone
	case *GuardStmt:
		if !ip.evalConds(x.Conds, env) {
			c := ip.execBlock(x.Else, newEnv(env), fc)
			if c == cNone {
				ip.fail(x.Pos, "'guard' body must not fall through, consider using a 'return' or 'throw' to exit the scope")
			}
			return c
		}
		return cNone
	case *ForStmt:
		return ip.execFor(x, env, fc)
	case *WhileStmt:
		for i := 0; ; i++ {
			cenv := newEnv(env)
			if !ip.evalConds(x.Conds, cenv) {
				return cNone
			}
			c := ip.execBlock(x.Body, cenv, fc)
			if c == cBreak {
				if fc.label == "" || fc.label == x.Label {
					fc.label = ""
					return cNone
				}
				return c
			}
			if c == cContinue {
				if fc.label == "" || fc.label == x.Label {
					fc.label = ""
					continue
				}
				return c
			}
			if c == cReturn {
				return c
			}
			if i > 10000000 {
				ip.fail(x.Pos, "infinite loop detected")
			}
		}
	case *RepeatStmt:
		for {
			c := ip.execBlock(x.Body, newEnv(env), fc)
			if c == cBreak {
				if fc.label == "" || fc.label == x.Label {
					fc.label = ""
					return cNone
				}
				return c
			}
			if c == cReturn {
				return c
			}
			if c == cContinue && !(fc.label == "" || fc.label == x.Label) {
				return c
			}
			fc.label = ""
			if !ip.evalBool(x.Cond, env) {
				return cNone
			}
		}
	case *SwitchStmt:
		return ip.execSwitch(x, env, fc)
	case *ReturnStmt:
		if x.X != nil {
			if _, isNil := x.X.(*NilLit); isNil && fc.isInit {
				fc.returnedNil = true
			}
			fc.ret = ip.eval(x.X, env)
		} else {
			fc.ret = Void
		}
		return cReturn
	case *BreakStmt:
		fc.label = x.Label
		return cBreak
	case *ContinueStmt:
		fc.label = x.Label
		return cContinue
	case *FallthroughStmt:
		return cFallthrough
	case *ThrowStmt:
		panic(&ThrowSignal{V: ip.eval(x.X, env), Pos: x.Pos})
	case *DoStmt:
		return ip.execDo(x, env, fc)
	case *TypeAliasDecl:
		ip.aliases[x.Name] = x.T
		return cNone
	case *PreviewDecl, *EnumCaseDecl, nil:
		return cNone
	}
	ip.fail(Pos{}, "unsupported statement %T", s)
	return cNone
}

func (ip *Interp) execVarDecl(x *VarDecl, env *Env) {
	if x.Computed {
		vd := x
		env.vars[x.Name] = &Slot{Get: func() Value {
			return ip.runGetter(vd.Get, env, nil, nil, vd.Type)
		}}
		return
	}
	if x.Pat != nil {
		v := ip.eval(x.Init, env)
		if !ip.matchPattern(x.Pat, v, env) {
			ip.fail(x.Pos, "tuple pattern does not match value")
		}
		return
	}
	if old, ok := env.vars[x.Name]; ok && old.Get == nil && !strings.HasPrefix(x.Name, "_") {
		_ = old
		ip.fail(x.Pos, "invalid redeclaration of '%s'", x.Name)
	}
	if hasAttr(x.Attrs, "State") && ip.store != nil {
		key := "local|" + x.Pos.String() + "|" + x.Name
		box := ip.store.state[key]
		if box == nil {
			var init Value
			if x.Init != nil {
				init = copyValue(ip.evalTyped(x.Init, env, x.Type))
			}
			box = &BoxV{V: init, Kind: "State"}
			ip.store.state[key] = box
		}
		if ip.render != nil {
			ip.render.touched[key] = true
		}
		b := box
		vt := x.Type
		env.vars[x.Name] = &Slot{V: b, Get: func() Value { return b.V }, Set: func(v Value) { b.V = copyValue(ip.coerce(v, vt, nil)) }}
		return
	}
	slot := &Slot{Let: x.Let, T: x.Type}
	if x.Init != nil {
		slot.V = copyValue(ip.evalTyped(x.Init, env, x.Type))
	} else if x.Type != nil && x.Type.Opt && !x.Let {
		slot.V = nil
	} else {
		slot.Uninit = true
	}
	if len(x.DidSet) > 0 || len(x.WillSet) > 0 {
		vd := x
		slot.Get = func() Value { return slot.V }
		slot.Set = func(v Value) {
			old := slot.V
			if len(vd.WillSet) > 0 {
				e := newEnv(env)
				e.vars[nz(vd.SetName, "newValue")] = &Slot{V: v, Let: true}
				ip.execBlock(vd.WillSet, e, &FnCtx{})
			}
			slot.V = copyValue(ip.coerce(v, vd.Type, nil))
			if len(vd.DidSet) > 0 {
				e := newEnv(env)
				e.vars["oldValue"] = &Slot{V: old, Let: true}
				ip.execBlock(vd.DidSet, e, &FnCtx{})
			}
		}
	}
	env.vars[x.Name] = slot
}

func nz(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func (ip *Interp) execFor(x *ForStmt, env *Env, fc *FnCtx) ctrl {
	seq := ip.eval(x.Seq, env)
	items := ip.iterate(seq, x.Pos)
	for _, it := range items {
		lenv := newEnv(env)
		if !ip.matchPattern(x.Pat, it, lenv) {
			continue
		}
		if x.Where != nil && !ip.evalBool(x.Where, lenv) {
			continue
		}
		c := ip.execBlock(x.Body, lenv, fc)
		switch c {
		case cBreak:
			if fc.label == "" || fc.label == x.Label {
				fc.label = ""
				return cNone
			}
			return c
		case cContinue:
			if fc.label == "" || fc.label == x.Label {
				fc.label = ""
				continue
			}
			return c
		case cReturn:
			return c
		}
	}
	return cNone
}

func (ip *Interp) execSwitch(x *SwitchStmt, env *Env, fc *FnCtx) ctrl {
	subj := ip.eval(x.Subj, env)
	fall := false
	for ci, c := range x.Cases {
		cenv := newEnv(env)
		ok := fall || c.Default
		if !ok {
			for _, p := range c.Pats {
				if ip.matchPattern(p, subj, cenv) {
					ok = true
					break
				}
			}
			if ok && c.Where != nil {
				ok = ip.evalBool(c.Where, cenv)
			}
		}
		if !ok {
			continue
		}
		fc.pos = append(fc.pos, "s"+strconv.Itoa(ci))
		r := ip.execBlock(c.Body, cenv, fc)
		fc.pos = fc.pos[:len(fc.pos)-1]
		if r == cFallthrough {
			fall = true
			continue
		}
		if r == cBreak && (fc.label == "" || fc.label == x.Label) {
			fc.label = ""
			return cNone
		}
		return r
	}
	return cNone
}

func (ip *Interp) execDo(x *DoStmt, env *Env, fc *FnCtx) (c ctrl) {
	var thrown *ThrowSignal
	func() {
		defer func() {
			if r := recover(); r != nil {
				if ts, ok := r.(*ThrowSignal); ok {
					thrown = ts
					return
				}
				panic(r)
			}
		}()
		c = ip.execBlock(x.Body, newEnv(env), fc)
	}()
	if thrown == nil {
		return c
	}
	for _, cc := range x.Catches {
		cenv := newEnv(env)
		if cc.Pat == nil {
			cenv.vars["error"] = &Slot{V: thrown.V, Let: true}
		} else if !ip.matchPattern(cc.Pat, thrown.V, cenv) {
			continue
		}
		if cc.Where != nil && !ip.evalBool(cc.Where, cenv) {
			continue
		}
		return ip.execBlock(cc.Body, cenv, fc)
	}
	panic(thrown)
}

func (ip *Interp) evalConds(conds []Cond, env *Env) bool {
	for _, c := range conds {
		switch c.Kind {
		case "expr":
			if !ip.evalBool(c.X, env) {
				return false
			}
		case "let", "var":
			v := ip.eval(c.X, env)
			if v == nil {
				return false
			}
			v = ip.coerce(v, c.Type, nil)
			if !ip.matchPattern(c.Pat, v, env) {
				return false
			}
			if c.Kind == "var" && c.Name != "" {
				env.vars[c.Name].Let = false
			}
		case "case":
			v := ip.eval(c.X, env)
			if !ip.matchPattern(c.Pat, v, env) {
				return false
			}
		}
	}
	return true
}

func (ip *Interp) evalBool(e Expr, env *Env) bool {
	v := ip.eval(e, env)
	b, ok := v.(bool)
	if !ok {
		ip.fail(e.P(), "cannot convert value of type '%s' to expected condition type 'Bool'", typeName(v))
	}
	return b
}

func (ip *Interp) iterate(v Value, pos Pos) []Value {
	switch x := v.(type) {
	case *ArrayV:
		return append([]Value(nil), x.E...)
	case *SetV:
		return append([]Value(nil), x.Keys...)
	case *DictV:
		out := make([]Value, len(x.Keys))
		for i := range x.Keys {
			out[i] = &TupleV{E: []Value{x.Keys[i], x.Vals[i]}, L: []string{"key", "value"}}
		}
		return out
	case *RangeV:
		lo, ok1 := x.Lo.(int)
		hi, ok2 := x.Hi.(int)
		if x.NoHi {
			ip.fail(pos, "cannot iterate over a partial range without an upper bound")
		}
		if !ok1 || !ok2 {
			if c1, ok := x.Lo.(Character); ok {
				c2 := x.Hi.(Character)
				var out []Value
				r1, _ := utf8.DecodeRuneInString(string(c1))
				r2, _ := utf8.DecodeRuneInString(string(c2))
				end := r2
				if !x.Closed {
					end--
				}
				for r := r1; r <= end; r++ {
					out = append(out, Character(string(r)))
				}
				return out
			}
			ip.fail(pos, "Range of type '%s' is not a sequence (only integer ranges can be iterated)", typeName(x.Lo))
		}
		if !x.Closed {
			hi--
		}
		if hi-lo > 5000000 {
			ip.fail(pos, "range too large to iterate")
		}
		out := make([]Value, 0, max(0, hi-lo+1))
		for i := lo; i <= hi; i++ {
			out = append(out, i)
		}
		return out
	case *StrideV:
		var out []Value
		if x.By == 0 {
			ip.fail(pos, "stride by zero")
		}
		for f := x.From; ; f += x.By {
			if x.By > 0 && (f > x.To || (!x.Through && f >= x.To)) {
				break
			}
			if x.By < 0 && (f < x.To || (!x.Through && f <= x.To)) {
				break
			}
			if x.Int {
				out = append(out, int(f))
			} else {
				out = append(out, f)
			}
			if len(out) > 5000000 {
				break
			}
		}
		return out
	case string:
		var out []Value
		for _, g := range graphemes(x) {
			out = append(out, Character(g))
		}
		return out
	case nil:
		ip.fail(pos, "Unexpectedly found nil while iterating an Optional sequence")
	}
	ip.fail(pos, "for-in loop requires '%s' to conform to 'Sequence'", typeName(v))
	return nil
}

// ---------- patterns ----------

func (ip *Interp) matchPattern(p Pattern, v Value, env *Env) bool {
	switch x := p.(type) {
	case *WildPat:
		return true
	case *BindPat:
		env.vars[x.Name] = &Slot{V: copyValue(v), Let: true}
		return true
	case *ExprPat:
		pv := ip.eval(x.X, env)
		switch r := pv.(type) {
		case *RangeV:
			return ip.rangeContains(r, v)
		case *SymV:
			return ip.enumMatch(v, r.Name(), r.Args(), r.Parts[0].Called)
		}
		return ip.equals(pv, v, Pos{})
	case *EnumPat:
		if x.Case == "some" && x.Type == "" {
			if v == nil {
				return false
			}
			if len(x.Subs) == 1 {
				return ip.matchPattern(x.Subs[0], v, env)
			}
			return true
		}
		if x.Case == "none" && x.Type == "" {
			return v == nil
		}
		switch e := v.(type) {
		case *EnumV:
			if e.Case != x.Case {
				return false
			}
			for i, sp := range x.Subs {
				if i >= len(e.Assoc) {
					return false
				}
				if !ip.matchPattern(sp, e.Assoc[i], env) {
					return false
				}
			}
			return true
		case *SymV:
			if e.Name() != x.Case {
				return false
			}
			args := e.Args()
			for i, sp := range x.Subs {
				if i >= len(args) {
					return false
				}
				if !ip.matchPattern(sp, args[i].V, env) {
					return false
				}
			}
			return true
		}
		if x.Type != "" && !x.HasSub {
			tv := ip.evalIdent(&Ident{Name: x.Type}, env)
			pv := ip.getMember(tv, x.Case, Pos{}, env)
			return ip.equals(pv, v, Pos{})
		}
		if x.Type == "" && !x.HasSub {
			return ip.enumMatch(v, x.Case, nil, false)
		}
		return false
	case *TuplePat:
		t, ok := v.(*TupleV)
		if !ok || len(t.E) != len(x.Elems) {
			return false
		}
		for i, sp := range x.Elems {
			if !ip.matchPattern(sp, t.E[i], env) {
				return false
			}
		}
		return true
	case *IsPat:
		return ip.isType(v, x.T)
	case *AsPat:
		if !ip.isType(v, x.T) {
			return false
		}
		return ip.matchPattern(x.Sub, v, env)
	case *OptPat:
		if v == nil {
			return false
		}
		return ip.matchPattern(x.Sub, v, env)
	}
	return false
}

func (ip *Interp) enumMatch(v Value, name string, args []ArgV, called bool) bool {
	switch e := v.(type) {
	case *EnumV:
		if e.Case != name {
			return false
		}
		if called && len(args) > 0 {
			for i := range args {
				if i >= len(e.Assoc) || !valueEquals(args[i].V, e.Assoc[i]) {
					return false
				}
			}
		}
		return true
	case *SymV:
		return e.Name() == name
	case *ColorV:
		return e.Name == name
	case nil:
		return name == "none"
	}
	if name == "some" {
		return v != nil
	}
	sv := ip.resolveSymLoose(sym(name))
	if sv != nil {
		return valueEquals(sv, v)
	}
	return false
}

func (ip *Interp) rangeContains(r *RangeV, v Value) bool {
	if !r.NoLo {
		c, ok := compareValues(v, r.Lo)
		if !ok || c < 0 {
			return false
		}
	}
	if !r.NoHi {
		c, ok := compareValues(v, r.Hi)
		if !ok {
			return false
		}
		if r.Closed && c > 0 || !r.Closed && c >= 0 {
			return false
		}
	}
	return true
}

func (ip *Interp) resolveAlias(name string) string {
	for i := 0; i < 10; i++ {
		t, ok := ip.aliases[name]
		if !ok {
			return name
		}
		name = t.Name
	}
	return name
}

func (ip *Interp) isType(v Value, t *TypeRef) bool {
	if t == nil {
		return true
	}
	if t.Kind == tyArray {
		_, ok := v.(*ArrayV)
		return ok
	}
	if t.Kind == tyDict {
		_, ok := v.(*DictV)
		return ok
	}
	name := ip.resolveAlias(t.Name)
	if name == "Any" || name == "AnyObject" || name == "AnyHashable" {
		return v != nil
	}
	if v == nil {
		return false
	}
	switch name {
	case "Double", "CGFloat", "Float", "TimeInterval":
		_, ok := v.(float64)
		return ok
	case "Error":
		switch x := v.(type) {
		case *EnumV:
			return x.T.conformsTo("Error") || x.T.conformsTo("LocalizedError")
		case *StructV:
			return x.T.conformsTo("Error") || x.T.conformsTo("LocalizedError")
		}
		return false
	}
	tn := typeName(v)
	if tn == name {
		return true
	}
	switch x := v.(type) {
	case *StructV:
		return x.T.conformsTo(name)
	case *ObjectV:
		return x.T.conformsTo(name)
	case *EnumV:
		return x.T.conformsTo(name)
	}
	if name == "View" {
		return isViewValue(v)
	}
	return false
}

// ---------- coercion ----------

func isDoubleType(name string) bool {
	switch name {
	case "Double", "CGFloat", "Float", "TimeInterval", "Float64", "Float32", "CGFloat.NativeType":
		return true
	}
	return false
}

func (ip *Interp) coerce(v Value, t *TypeRef, x Expr) Value {
	if t == nil || v == nil {
		return v
	}
	switch t.Kind {
	case tyNamed:
		name := ip.resolveAlias(t.Name)
		if t.Name != name {
			if at, ok := ip.aliases[t.Name]; ok && at.Kind != tyNamed {
				return ip.coerce(v, at, x)
			}
		}
		if name == "Binding" {
			return constBinding(v)
		}
		switch {
		case isDoubleType(name):
			if i, ok := v.(int); ok {
				return float64(i)
			}
			if s, ok := v.(*SymV); ok {
				if r := ip.resolveSymAs("Double", s); r != nil {
					return r
				}
			}
			return v
		case name == "String":
			if c, ok := v.(Character); ok {
				return string(c)
			}
			return v
		case name == "Character":
			if s, ok := v.(string); ok && utf8.RuneCountInString(s) >= 1 {
				return Character(s)
			}
			return v
		case name == "Set":
			if a, ok := v.(*ArrayV); ok {
				s := newSet()
				for _, e := range a.E {
					if len(t.Args) == 1 {
						e = ip.coerce(e, t.Args[0], nil)
					}
					s.Add(e)
				}
				if len(t.Args) == 1 {
					s.Elem = t.Args[0]
				}
				return s
			}
			return v
		}
		if s, ok := v.(*SymV); ok {
			if ti := ip.lookupTypeInfo(name); ti != nil {
				return ip.symToType(ti, s, Pos{})
			}
			if r := ip.resolveSymAs(name, s); r != nil {
				return r
			}
		}
		if a, ok := v.(*ArrayV); ok && len(a.E) == 1 && name == "GridItem" {
			return v
		}
		return v
	case tyArray:
		a, ok := v.(*ArrayV)
		if !ok {
			return v
		}
		needs := false
		for _, e := range a.E {
			switch e.(type) {
			case int, *SymV, *ArrayV, Character:
				needs = true
			}
		}
		if needs {
			n := &ArrayV{E: make([]Value, len(a.E)), Elem: t.Elem}
			for i, e := range a.E {
				n.E[i] = ip.coerce(e, t.Elem, nil)
			}
			return n
		}
		a.Elem = t.Elem
		return a
	case tyDict:
		d, ok := v.(*DictV)
		if !ok {
			return v
		}
		for i := range d.Vals {
			d.Vals[i] = ip.coerce(d.Vals[i], t.Elem, nil)
			nk := ip.coerce(d.Keys[i], t.Key, nil)
			d.Keys[i] = nk
		}
		d.reindex()
		d.KeyT, d.ValT = t.Key, t.Elem
		return d
	case tyTuple:
		tv, ok := v.(*TupleV)
		if ok && len(tv.E) == len(t.Params) {
			n := &TupleV{E: make([]Value, len(tv.E)), L: tv.L}
			for i := range tv.E {
				n.E[i] = ip.coerce(tv.E[i], t.Params[i], nil)
			}
			return n
		}
	}
	return v
}

// Two types can each declare a nested type with the same name (every UIKit
// wrapper has its own Coordinator), so the enclosing type's own one wins.
func (ip *Interp) nestedTypeInScope(name string, env *Env) *TypeInfo {
	for e := env; e != nil; e = e.parent {
		owner := e.selfT
		if owner == nil {
			owner = e.staticT
		}
		if owner == nil {
			continue
		}
		scope := owner.Name
		if owner.Outer != "" {
			scope = owner.Outer + "." + owner.Name
		}
		for scope != "" {
			if ti, ok := ip.types[scope+"."+name]; ok {
				return ti
			}
			cut := strings.LastIndex(scope, ".")
			if cut < 0 {
				break
			}
			scope = scope[:cut]
		}
		return nil
	}
	return nil
}

func (ip *Interp) lookupTypeInfo(name string) *TypeInfo {
	if ti, ok := ip.types[name]; ok {
		return ti
	}
	if i := strings.LastIndex(name, "."); i >= 0 {
		if ti, ok := ip.types[name[i+1:]]; ok {
			return ti
		}
	}
	return nil
}

func (ip *Interp) symToType(ti *TypeInfo, s *SymV, pos Pos) Value {
	name := s.Name()
	var base Value
	if ti.Kind == "enum" {
		if c, ok := ti.CaseMap[name]; ok {
			p0 := s.Parts[0]
			if len(c.Params) > 0 {
				base = ip.makeEnumCase(ti, c, p0.Args, pos)
			} else {
				base = &EnumV{T: ti, Case: name}
			}
			return ip.applySymRest(base, s, pos)
		}
	}
	tv := &TypeV{Name: ti.Name, T: ti}
	m := ip.typeMemberMaybe(tv, name, pos)
	if m == nil {
		return s
	}
	if s.Parts[0].Called {
		m = ip.callValue(m, s.Parts[0].Args, pos, ip.globals)
	}
	return ip.applySymRest(m, s, pos)
}

func (ip *Interp) applySymRest(base Value, s *SymV, pos Pos) Value {
	for _, p := range s.Parts[1:] {
		if p.Called {
			base = ip.callMethod(base, &Ref{Get: func() Value { return base }, Set: func(Value) {}}, p.Name, p.Args, pos, ip.globals)
		} else {
			base = ip.getMember(base, p.Name, pos, ip.globals)
		}
	}
	return base
}

func (ip *Interp) makeEnumCase(ti *TypeInfo, c *EnumCase, args []ArgV, pos Pos) Value {
	vals, set, err := ip.matchArgs(c.Params, args)
	if err != "" {
		ip.fail(pos, "%s", err)
	}
	e := &EnumV{T: ti, Case: c.Name}
	for i, p := range c.Params {
		v := vals[i]
		if !set[i] && p.Default != nil {
			v = ip.eval(p.Default, ip.globals)
		}
		e.Assoc = append(e.Assoc, copyValue(ip.coerce(v, p.Type, nil)))
		e.Labels = append(e.Labels, p.Label)
	}
	return e
}

func (ip *Interp) evalTyped(e Expr, env *Env, t *TypeRef) Value {
	if t == nil {
		return ip.eval(e, env)
	}
	if t.Kind == tyNamed && isDoubleType(ip.resolveAlias(t.Name)) {
		return ip.evalAsDouble(e, env)
	}
	if t.Kind == tyArray && t.Elem != nil && t.Elem.Kind == tyNamed && isDoubleType(t.Elem.Name) {
		if al, ok := e.(*ArrayLit); ok {
			a := &ArrayV{Elem: t.Elem}
			for _, el := range al.Elems {
				a.E = append(a.E, ip.evalAsDouble(el, env))
			}
			return a
		}
	}
	// A generic function infers its result type from the declaration it is
	// assigned to: `let items: [Item] = Bundle.main.decode("items.json")`.
	if _, isCall := e.(*CallExpr); isCall {
		ip.expectedType = t
	}
	return ip.coerce(ip.eval(e, env), t, e)
}

func (ip *Interp) evalAsDouble(e Expr, env *Env) Value {
	switch x := e.(type) {
	case *IntLit:
		return float64(x.V)
	case *UnaryExpr:
		if x.Op == "-" {
			v := ip.evalAsDouble(x.X, env)
			if f, ok := v.(float64); ok {
				return -f
			}
			return ip.binop("-", 0, v, x.Pos)
		}
	case *BinaryExpr:
		switch x.Op {
		case "+", "-", "*", "/":
			l := ip.evalAsDouble(x.L, env)
			r := ip.evalAsDouble(x.R, env)
			return ip.binop(x.Op, l, r, x.Pos)
		}
	case *TernaryExpr:
		if ip.evalBool(x.C, env) {
			return ip.evalAsDouble(x.T, env)
		}
		return ip.evalAsDouble(x.F, env)
	}
	v := ip.eval(e, env)
	if i, ok := v.(int); ok {
		return float64(i)
	}
	if s, ok := v.(*SymV); ok {
		if r := ip.resolveSymAs("Double", s); r != nil {
			return r
		}
	}
	return v
}

// ---------- expressions ----------

func (ip *Interp) eval(e Expr, env *Env) Value {
	switch x := e.(type) {
	case *IntLit:
		return int(x.V)
	case *FloatLit:
		return x.V
	case *StrLit:
		return ip.evalStr(x, env, false)
	case *BoolLit:
		return x.V
	case *NilLit:
		return nil
	case *Ident:
		return ip.evalIdent(x, env)
	case *ImplicitMember:
		return sym(x.Name)
	case *MemberExpr:
		base := ip.eval(x.X, env)
		return ip.getMember(base, x.Name, x.Pos, env)
	case *CallExpr:
		return ip.evalCall(x, env)
	case *SubscriptExpr:
		return ip.subscriptRef(x, env).Get()
	case *OptChain:
		return ip.evalOptChain(x.X, env)
	case *OptMark:
		v := ip.eval(x.X, env)
		if v == nil {
			panic(&OptNilSignal{})
		}
		return v
	case *ForceUnwrap:
		v := ip.eval(x.X, env)
		if v == nil {
			ip.fail(x.Pos, "Fatal error: Unexpectedly found nil while unwrapping an Optional value")
		}
		return v
	case *UnaryExpr:
		v := ip.eval(x.X, env)
		switch x.Op {
		case "-":
			switch n := v.(type) {
			case int:
				return -n
			case float64:
				return -n
			}
		case "+":
			return v
		case "!":
			if b, ok := v.(bool); ok {
				return !b
			}
			ip.fail(x.Pos, "cannot convert value of type '%s' to expected argument type 'Bool'", typeName(v))
		case "~":
			if n, ok := v.(int); ok {
				return ^n
			}
		case "..<":
			return &RangeV{NoLo: true, Hi: v}
		case "...":
			return &RangeV{NoLo: true, Hi: v, Closed: true}
		}
		ip.fail(x.Pos, "unary operator '%s' cannot be applied to an operand of type '%s'", x.Op, typeName(v))
	case *PostfixRange:
		return &RangeV{Lo: ip.eval(x.X, env), NoHi: true, Closed: true}
	case *BinaryExpr:
		return ip.evalBinary(x, env)
	case *AssignExpr:
		ip.evalAssign(x, env)
		return Void
	case *TernaryExpr:
		if ip.evalBool(x.C, env) {
			return ip.eval(x.T, env)
		}
		return ip.eval(x.F, env)
	case *CastExpr:
		v := ip.eval(x.X, env)
		switch x.Kind {
		case "is":
			return ip.isType(v, x.T)
		case "as?":
			if v == nil || !ip.isType(v, x.T) {
				if i, ok := v.(int); ok && x.T.Kind == tyNamed && isDoubleType(x.T.Name) {
					_ = i
					return nil
				}
				return nil
			}
			return v
		case "as!":
			if !ip.isType(v, x.T) {
				ip.fail(x.Pos, "Could not cast value of type '%s' to '%s'", typeName(v), x.T.String())
			}
			return v
		default:
			return ip.coerce(v, x.T, x.X)
		}
	case *ClosureExpr:
		return &FuncV{Params: x.Params, Body: x.Body, Env: env, IsClosure: true, HasSig: x.HasSig, MaxDollar: x.MaxDollar}
	case *ArrayLit:
		a := &ArrayV{E: make([]Value, len(x.Elems))}
		for i, el := range x.Elems {
			a.E[i] = copyValue(ip.eval(el, env))
		}
		return a
	case *DictLit:
		d := newDict()
		for i := range x.Keys {
			d.Set(ip.eval(x.Keys[i], env), copyValue(ip.eval(x.Vals[i], env)))
		}
		return d
	case *TupleExpr:
		if len(x.Elems) == 0 {
			return Void
		}
		t := &TupleV{E: make([]Value, len(x.Elems)), L: x.Labels}
		for i, el := range x.Elems {
			t.E[i] = copyValue(ip.eval(el, env))
		}
		return t
	case *KeyPathExpr:
		return &KeyPathV{Root: x.Root, Path: x.Path}
	case *GenericTypeExpr:
		v := ip.eval(x.X, env)
		if t, ok := v.(*TypeV); ok && t.Ref == nil {
			return &TypeV{Name: t.Name, T: t.T, Ref: &TypeRef{Name: t.Name, Args: x.Args}}
		}
		return v
	case *TryExpr:
		return ip.evalTry(x, env)
	case *AwaitExpr:
		return ip.eval(x.X, env)
	case *InOutExpr:
		r := ip.ref(x.X, env)
		if r.RO != "" {
			ip.fail(x.Pos, "cannot pass immutable value as inout argument: %s", r.RO)
		}
		return &RefV{R: r}
	case *IfExpr:
		cenv := newEnv(env)
		if ip.evalConds(x.S.Conds, cenv) {
			return ip.branchValue(x.S.Then, cenv)
		}
		switch el := x.S.Else.(type) {
		case *IfStmt:
			return ip.eval(&IfExpr{Pos: x.Pos, S: el}, env)
		case *BlockStmt:
			return ip.branchValue(el.Body, newEnv(env))
		}
		return Void
	case *SwitchExpr:
		subj := ip.eval(x.S.Subj, env)
		for _, c := range x.S.Cases {
			cenv := newEnv(env)
			ok := c.Default
			for _, p := range c.Pats {
				if !ok && ip.matchPattern(p, subj, cenv) {
					ok = true
				}
			}
			if ok && c.Where != nil {
				ok = ip.evalBool(c.Where, cenv)
			}
			if ok {
				return ip.branchValue(c.Body, cenv)
			}
		}
		return Void
	case *TypeExpr:
		return &TypeV{Name: x.T.Name}
	}
	ip.fail(e.P(), "unsupported expression %T", e)
	return nil
}

func (ip *Interp) branchValue(body []Stmt, env *Env) Value {
	fc := &FnCtx{}
	return ip.execBody(body, env, fc)
}

func (ip *Interp) evalOptChain(x Expr, env *Env) (v Value) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(*OptNilSignal); ok {
				v = nil
				return
			}
			panic(r)
		}
	}()
	return ip.eval(x, env)
}

func (ip *Interp) evalTry(x *TryExpr, env *Env) (v Value) {
	if x.Kind == "try" {
		return ip.eval(x.X, env)
	}
	defer func() {
		if r := recover(); r != nil {
			if ts, ok := r.(*ThrowSignal); ok {
				if x.Kind == "try?" {
					v = nil
					return
				}
				ip.fail(x.Pos, "Fatal error: 'try!' expression unexpectedly raised an error: %s", describe(ts.V))
			}
			panic(r)
		}
	}()
	return ip.eval(x.X, env)
}

// Optionals hold their value directly, so whether "\(x)" prints Optional(...)
// comes from the declared type of x rather than from the value.
func (ip *Interp) isDeclaredOptional(e Expr, env *Env) bool {
	switch x := e.(type) {
	case *Ident:
		for scope := env; scope != nil; scope = scope.parent {
			if slot, ok := scope.vars[x.Name]; ok {
				return slot.T != nil && slot.T.Opt
			}
			if scope.self != nil {
				if ti := typeInfoOf(scope.self); ti != nil {
					if f := ti.field(x.Name); f != nil {
						return f.Type != nil && f.Type.Opt
					}
				}
			}
		}
	case *MemberExpr:
		base := ip.eval(x.X, env)
		if ti := typeInfoOf(base); ti != nil {
			if f := ti.field(x.Name); f != nil {
				return f.Type != nil && f.Type.Opt
			}
		}
	}
	return false
}

func (ip *Interp) evalStr(s *StrLit, env *Env, localized bool) string {
	if len(s.Pieces) == 1 && s.Pieces[0].X == nil {
		return s.Pieces[0].Lit
	}
	var sb strings.Builder
	for _, pc := range s.Pieces {
		if pc.X == nil {
			sb.WriteString(pc.Lit)
			continue
		}
		v := ip.eval(pc.X, env)
		if len(pc.Args) > 0 {
			a := pc.Args[0]
			if !localized {
				ip.fail(pc.X.P(), "extraneous argument label '%s:' in call ('%s:' interpolation only works directly inside Text(\"...\"))", a.Label, a.Label)
			}
			switch a.Label {
			case "specifier":
				spec := describe(ip.eval(a.X, env))
				sb.WriteString(cFormat(spec, []Value{v}))
			case "format":
				st := ip.eval(a.X, env)
				sb.WriteString(ip.formatStyle(v, st, pc.X.P()))
			default:
				ip.fail(pc.X.P(), "extraneous argument label '%s:' in string interpolation", a.Label)
			}
			continue
		}
		if v != nil && ip.isDeclaredOptional(pc.X, env) {
			sb.WriteString("Optional(" + describeIn(v, true) + ")")
			continue
		}
		sb.WriteString(ip.describeV(v))
	}
	return sb.String()
}

func (ip *Interp) describeV(v Value) string {
	switch x := v.(type) {
	case *StructV:
		if vd := ip.findComputed(x.T, "description"); vd != nil {
			return describe(ip.runGetter(vd.Get, ip.globals, x, x.T, nil))
		}
	case *ObjectV:
		if vd := ip.findComputed(x.T, "description"); vd != nil {
			return describe(ip.runGetter(vd.Get, ip.globals, x, x.T, nil))
		}
	case *EnumV:
		if vd := ip.findComputed(x.T, "description"); vd != nil {
			return describe(ip.runGetter(vd.Get, ip.globals, x, x.T, nil))
		}
	}
	return describe(v)
}

func (ip *Interp) runGetter(body []Stmt, parent *Env, self Value, selfT *TypeInfo, rt *TypeRef) Value {
	env := newEnv(parent)
	if self != nil {
		env.self = self
		env.selfT = selfT
		if _, isObj := self.(*ObjectV); isObj {
			env.mutableSelf = true
		}
		env.staticT = selfT
	}
	fc := &FnCtx{}
	ip.depth++
	defer func() { ip.depth-- }()
	if ip.depth > 600 {
		ip.fail(Pos{}, "stack overflow (infinite recursion?)")
	}
	v := ip.execBody(body, env, fc)
	if rt != nil {
		v = ip.coerce(v, rt, nil)
	}
	return v
}

func (ip *Interp) evalIdent(x *Ident, env *Env) Value {
	name := x.Name
	switch name {
	case "self":
		for e := env; e != nil; e = e.parent {
			if e.self != nil {
				return e.self
			}
			if e.staticT != nil && e.parent == ip.globals {
				return &TypeV{Name: e.staticT.Name, T: e.staticT}
			}
		}
		ip.fail(x.Pos, "cannot find 'self' in scope")
	case "Self":
		for e := env; e != nil; e = e.parent {
			if e.selfT != nil {
				return &TypeV{Name: e.selfT.Name, T: e.selfT}
			}
			if e.staticT != nil {
				return &TypeV{Name: e.staticT.Name, T: e.staticT}
			}
		}
	case "super":
		for e := env; e != nil; e = e.parent {
			if e.self != nil {
				return &SuperV{Obj: e.self, Owner: e.selfT}
			}
		}
	}
	if v, ok := ip.lookup(name, env); ok {
		return v
	}
	if strings.HasPrefix(name, "$") && len(name) > 1 && !(name[1] >= '0' && name[1] <= '9') {
		return ip.projection(name[1:], env, x.Pos)
	}
	if strings.HasPrefix(name, "$") {
		ip.fail(x.Pos, "anonymous closure argument '%s' not contained in a closure (or closure has fewer parameters)", name)
	}
	if ti := ip.nestedTypeInScope(name, env); ti != nil {
		return &TypeV{Name: ti.Name, T: ti}
	}
	if ti := ip.lookupTypeInfo(name); ti != nil && ip.types[name] != nil {
		return &TypeV{Name: name, T: ti}
	}
	if t, ok := ip.aliases[name]; ok {
		if t.Kind == tyArray || t.Kind == tyDict {
			return &TypeV{Name: t.Name, Ref: t}
		}
		return ip.evalIdent(&Ident{Pos: x.Pos, Name: t.Name}, env)
	}
	if builtinTypeNames[name] {
		return &TypeV{Name: name}
	}
	if fn := ip.builtinGlobal(name); fn != nil {
		return fn
	}
	if _, isOp := binOps[name]; isOp && name != "=" {
		op := name
		return &NativeV{Name: op, Fn: func(c *CallCtx) Value {
			if len(c.Args) == 1 {
				return ip.binop(op, 0, c.Args[0].V, c.Pos)
			}
			return ip.binop(op, argAt(c.Args, 0), argAt(c.Args, 1), c.Pos)
		}}
	}
	ip.fail(x.Pos, "cannot find '%s' in scope", name)
	return nil
}

func (ip *Interp) lookup(name string, env *Env) (Value, bool) {
	for e := env; e != nil; e = e.parent {
		if s, ok := e.vars[name]; ok {
			return s.get(), true
		}
		if e.self != nil {
			if v, ok := ip.implicitSelfMember(e.self, e.selfT, name); ok {
				return v, true
			}
		}
		if e.staticT != nil {
			if v, ok := ip.staticMemberIfExists(e.staticT, name); ok {
				return v, true
			}
		}
	}
	return nil, false
}

func (ip *Interp) staticMemberIfExists(ti *TypeInfo, name string) (Value, bool) {
	if s, ok := ip.staticField(ti, name); ok {
		return s.get(), true
	}
	if ms := ip.findStaticMethods(ti, name); ms != nil {
		return ip.bindMethods(ms, nil, ti, true), true
	}
	if ti.Kind == "enum" {
		if c, ok := ti.CaseMap[name]; ok && len(c.Params) == 0 {
			_ = c
		}
	}
	return nil, false
}

func (ip *Interp) implicitSelfMember(self Value, selfT *TypeInfo, name string) (Value, bool) {
	switch s := self.(type) {
	case *StructV, *ObjectV:
		ti := selfT
		if sv, ok := s.(*StructV); ok {
			ti = sv.T
		} else if ov, ok := s.(*ObjectV); ok {
			ti = ov.T
		}
		if ti.field(name) != nil || ip.findComputed(ti, name) != nil {
			v, _ := ip.getField(self, ti, name, Pos{})
			return v, true
		}
		if ms := ip.findMethods(ti, name); ms != nil {
			return ip.bindMethods(ms, self, ti, false), true
		}
		if strings.HasPrefix(name, "_") && ti.field(name[1:]) != nil {
			return fieldsOf(self)[name[1:]], true
		}
		if ti.IsView() && (knownModifiers[name] || ip.viewExtMethod(name) != nil) {
			return ip.getMember(self, name, Pos{}, nil), true
		}
	case *EnumV:
		if name == "rawValue" && s.T.RawType != "" {
			return s.T.RawVals[s.Case], true
		}
		if vd := ip.findComputed(s.T, name); vd != nil {
			return ip.runGetter(vd.Get, ip.globals, s, s.T, vd.Type), true
		}
		if ms := ip.findMethods(s.T, name); ms != nil {
			return ip.bindMethods(ms, self, s.T, false), true
		}
	case *ViewV, *ModV, *ColorV:
		if knownModifiers[name] || ip.viewExtMethod(name) != nil {
			return ip.getMember(self, name, Pos{}, nil), true
		}
	default:
		if selfT != nil {
			if vd, ok := selfT.Computed[name]; ok {
				return ip.runGetter(vd.Get, ip.globals, self, selfT, vd.Type), true
			}
			if ms, ok := selfT.Methods[name]; ok {
				return ip.bindMethods(ms, self, selfT, false), true
			}
		}
		if _, isPlaceholder := self.(*builtinInitSelf); isPlaceholder {
			return nil, false
		}
		if native, isNative := self.(*NativeObj); isNative {
			// Inside `extension Bundle`, plain names are globals (fatalError, Data)
			// unless the object really has a property of that name.
			v, found := native.Props[name]
			return v, found
		}
		if v, ok := ip.builtinProperty(self, name, Pos{}); ok {
			return v, true
		}
		if hasBuiltinMethod(self, name) {
			return ip.getMember(self, name, Pos{}, nil), true
		}
	}
	return nil, false
}

func fieldsOf(v Value) map[string]Value {
	switch x := v.(type) {
	case *StructV:
		return x.F
	case *ObjectV:
		return x.F
	}
	return nil
}

func typeInfoOf(v Value) *TypeInfo {
	switch x := v.(type) {
	case *StructV:
		return x.T
	case *ObjectV:
		return x.T
	case *EnumV:
		return x.T
	}
	return nil
}

func (ip *Interp) projection(name string, env *Env, pos Pos) Value {
	for e := env; e != nil; e = e.parent {
		if s, ok := e.vars[name]; ok {
			if b, ok := s.V.(*BoxV); ok {
				box := b
				return &BindingV{Get: func() Value { return box.V }, Set: func(v Value) { box.V = copyValue(v) }, Name: name}
			}
			v := s.get()
			switch x := v.(type) {
			case *ObjectV:
				obj := x
				return &ProjV{Get: func() Value { return obj }}
			case *BindingV:
				return x
			}
			ip.fail(pos, "cannot find '$%s' in scope ('%s' is not a property wrapper)", name, name)
		}
		if e.self != nil {
			if f := fieldsOf(e.self); f != nil {
				ti := typeInfoOf(e.self)
				if fi := ti.field(name); fi != nil {
					raw := f[name]
					return ip.projectRaw(raw, fi, pos)
				}
			}
		}
	}
	ip.fail(pos, "cannot find '$%s' in scope", name)
	return nil
}

func (ip *Interp) projectRaw(raw Value, fi *FieldInfo, pos Pos) Value {
	switch r := raw.(type) {
	case *BoxV:
		box := r
		ft := fi.Type
		return &BindingV{Get: func() Value { return box.V }, Set: func(v Value) { box.V = copyValue(ip.coerce(v, ft, nil)) }, Name: fi.Name}
	case *BindingV:
		return r
	case *ObjectV:
		obj := r
		return &ProjV{Get: func() Value { return obj }}
	}
	if fi.Wrapper == "" {
		ip.fail(pos, "cannot find '$%s' in scope ('%s' is not declared with @State, @Binding or another property wrapper)", fi.Name, fi.Name)
	}
	ip.fail(pos, "cannot create a binding to '%s'", fi.Name)
	return nil
}

// ---------- fields ----------

func (ip *Interp) getField(obj Value, ti *TypeInfo, name string, pos Pos) (Value, bool) {
	f := fieldsOf(obj)
	if fi := ti.field(name); fi != nil {
		raw := f[name]
		switch r := raw.(type) {
		case *BoxV:
			return r.V, true
		case *BindingV:
			if fi.Wrapper == "Binding" {
				return r.Get(), true
			}
		case *lazyFieldV:
			env := newEnv(ip.globals)
			env.self, env.selfT, env.staticT = obj, ti, ti
			f[name] = copyValue(ip.evalTyped(fi.Init, env, fi.Type))
			return f[name], true
		}
		if fi.Wrapper == "EnvironmentObject" && raw == nil {
			ip.fail(pos, "No ObservableObject of type %s found. A View.environmentObject(_:) for %s may be missing as an ancestor of this view.", fi.Type.String(), fi.Type.String())
		}
		return raw, true
	}
	if vd := ip.findComputed(ti, name); vd != nil {
		return ip.runGetter(vd.Get, ip.globals, obj, ti, vd.Type), true
	}
	if strings.HasPrefix(name, "_") && ti.field(name[1:]) != nil {
		return f[name[1:]], true
	}
	return nil, false
}

func (ip *Interp) setField(obj Value, ti *TypeInfo, name string, v Value, pos Pos) {
	f := fieldsOf(obj)
	if fi := ti.field(name); fi != nil {
		raw := f[name]
		switch r := raw.(type) {
		case *BoxV:
			r.V = copyValue(ip.coerce(v, fi.Type, nil))
			return
		case *BindingV:
			if fi.Wrapper == "Binding" {
				r.Set(v)
				return
			}
		}
		if fi.Wrapper == "Binding" && raw == nil {
			if b, ok := v.(*BindingV); ok {
				f[name] = b
				return
			}
		}
		decl := fi.Decl
		var old Value
		if decl != nil && (len(decl.WillSet) > 0 || len(decl.DidSet) > 0) {
			old = raw
			if len(decl.WillSet) > 0 {
				e := newEnv(ip.globals)
				e.self, e.selfT, e.mutableSelf = obj, ti, true
				e.vars[nz(decl.SetName, "newValue")] = &Slot{V: v, Let: true}
				ip.execBlock(decl.WillSet, e, &FnCtx{})
			}
		}
		f[name] = copyValue(ip.coerce(v, fi.Type, nil))
		if model, ok := obj.(*ObjectV); ok && model.T.IsModel && model.Model == nil {
			insertIntoRelatedContext(model, f[name])
		}
		if decl != nil && len(decl.DidSet) > 0 {
			e := newEnv(ip.globals)
			e.self, e.selfT, e.mutableSelf = obj, ti, true
			e.vars["oldValue"] = &Slot{V: old, Let: true}
			ip.execBlock(decl.DidSet, e, &FnCtx{})
		}
		return
	}
	if vd := ip.findComputed(ti, name); vd != nil {
		if vd.Set == nil {
			ip.fail(pos, "cannot assign to property: '%s' is a get-only property", name)
		}
		e := newEnv(ip.globals)
		e.self, e.selfT, e.mutableSelf = obj, ti, true
		e.vars[nz(vd.SetName, "newValue")] = &Slot{V: v, Let: true}
		ip.execBlock(vd.Set, e, &FnCtx{})
		return
	}
	if strings.HasPrefix(name, "_") && ti.field(name[1:]) != nil {
		f[name[1:]] = v
		return
	}
	ip.fail(pos, "value of type '%s' has no member '%s'", ti.Name, name)
}

func isWrapperSettable(w string) bool {
	switch w {
	case "State", "Binding", "StateObject", "FocusState", "AppStorage", "SceneStorage", "GestureState":
		return true
	}
	return false
}

// ---------- refs ----------

func (ip *Interp) ref(e Expr, env *Env) *Ref {
	switch x := e.(type) {
	case *Ident:
		return ip.identRef(x, env)
	case *MemberExpr:
		return ip.memberRef(x, env)
	case *SubscriptExpr:
		return ip.subscriptRef(x, env)
	case *ForceUnwrap:
		r := ip.ref(x.X, env)
		return &Ref{Get: func() Value {
			v := r.Get()
			if v == nil {
				ip.fail(x.Pos, "Fatal error: Unexpectedly found nil while unwrapping an Optional value")
			}
			return v
		}, Set: r.Set, RO: r.RO, ROKind: r.ROKind}
	case *OptMark:
		r := ip.ref(x.X, env)
		return &Ref{Get: func() Value {
			v := r.Get()
			if v == nil {
				panic(&OptNilSignal{})
			}
			return v
		}, Set: r.Set, RO: r.RO, ROKind: r.ROKind}
	case *OptChain:
		var r *Ref
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					if _, ok := rec.(*OptNilSignal); ok {
						r = &Ref{Get: func() Value { return nil }, Set: func(Value) {}}
						return
					}
					panic(rec)
				}
			}()
			r = ip.ref(x.X, env)
		}()
		inner := r
		return &Ref{Get: func() (v Value) {
			defer func() {
				if rec := recover(); rec != nil {
					if _, ok := rec.(*OptNilSignal); ok {
						v = nil
						return
					}
					panic(rec)
				}
			}()
			return inner.Get()
		}, Set: func(v Value) {
			defer func() {
				if rec := recover(); rec != nil {
					if _, ok := rec.(*OptNilSignal); ok {
						return
					}
					panic(rec)
				}
			}()
			inner.Set(v)
		}, RO: r.RO}
	}
	v := ip.eval(e, env)
	return &Ref{Get: func() Value { return v }, Set: func(Value) { ip.fail(e.P(), "cannot assign to this expression") }}
}

func (ip *Interp) identRef(x *Ident, env *Env) *Ref {
	name := x.Name
	if name == "self" {
		for e := env; e != nil; e = e.parent {
			if e.self != nil {
				ee := e
				ro := ""
				switch e.self.(type) {
				case *StructV, *EnumV:
					if !e.mutableSelf {
						ro = "'self' is immutable"
					}
				}
				return &Ref{Get: func() Value { return ee.self }, Set: func(v Value) {
					if s, ok := ee.self.(*StructV); ok {
						if n, ok2 := v.(*StructV); ok2 {
							for k, fv := range n.F {
								s.F[k] = copyValue(fv)
							}
							return
						}
					}
					ee.self = v
				}, RO: ro, ROKind: "value", InInit: e.inInit}
			}
		}
	}
	for e := env; e != nil; e = e.parent {
		if s, ok := e.vars[name]; ok {
			slot := s
			ro := ""
			if s.Let && !s.Uninit {
				ro = fmt.Sprintf("'%s' is a 'let' constant", name)
			}
			if s.Get != nil && s.Set == nil {
				ro = fmt.Sprintf("'%s' is a get-only property", name)
			}
			return &Ref{Get: slot.get, Set: func(v Value) {
				if slot.Set != nil {
					slot.Set(v)
					return
				}
				if slot.Let && !slot.Uninit {
					ip.fail(x.Pos, "cannot assign to value: '%s' is a 'let' constant", name)
				}
				slot.Uninit = false
				slot.V = copyValue(ip.coerce(v, slot.T, nil))
			}, RO: ro, ROKind: "value"}
		}
		if e.self != nil {
			ti := typeInfoOf(e.self)
			if ti != nil && strings.HasPrefix(name, "_") && ti.field(name[1:]) != nil && ti.field(name[1:]).Wrapper != "" {
				self := e.self
				return &Ref{Get: func() Value {
					v, _ := ip.getField(self, ti, name, x.Pos)
					return v
				}, Set: func(v Value) {
					ip.setField(self, ti, name, v, x.Pos)
				}, ROKind: "property"}
			}
			if ti != nil && (ti.field(name) != nil || ip.findComputed(ti, name) != nil) {
				self := e.self
				ro := ""
				fi := ti.field(name)
				_, isStruct := self.(*StructV)
				if fi != nil && fi.Wrapper != "" && isWrapperSettable(fi.Wrapper) {
					ro = ""
				} else if fi != nil && fi.Let && !e.inInit {
					ro = fmt.Sprintf("'%s' is a 'let' constant", name)
				} else if isStruct && !e.mutableSelf {
					ro = "'self' is immutable"
				} else if fi == nil {
					if vd := ip.findComputed(ti, name); vd != nil && vd.Set == nil {
						ro = fmt.Sprintf("'%s' is a get-only property", name)
					}
				}
				if _, isEnum := self.(*EnumV); isEnum && !e.mutableSelf {
					ro = "'self' is immutable"
				}
				observed := fi != nil && fi.Decl != nil && (len(fi.Decl.DidSet) > 0 || len(fi.Decl.WillSet) > 0)
				return &Ref{Get: func() Value {
					v, _ := ip.getField(self, ti, name, x.Pos)
					if observed {
						return copyValue(v)
					}
					return v
				}, Set: func(v Value) {
					if ro != "" {
						ip.fail(x.Pos, "cannot assign to property: %s", ro)
					}
					ip.setField(self, ti, name, v, x.Pos)
				}, RO: ro, ROKind: "property"}
			}
		}
		if e.staticT != nil {
			if s, ok := ip.staticField(e.staticT, name); ok {
				slot := s
				ro := ""
				if s.Let {
					ro = fmt.Sprintf("'%s' is a 'let' constant", name)
				}
				return &Ref{Get: slot.get, Set: func(v Value) {
					if slot.Set != nil {
						slot.Set(v)
						return
					}
					slot.V = copyValue(ip.coerce(v, slot.T, nil))
				}, RO: ro, ROKind: "property"}
			}
		}
	}
	v := ip.evalIdent(x, env)
	return &Ref{Get: func() Value { return v }, Set: func(Value) { ip.fail(x.Pos, "cannot assign to '%s'", name) }, RO: fmt.Sprintf("'%s' is not assignable", name)}
}

func (ip *Interp) memberRef(x *MemberExpr, env *Env) *Ref {
	baseRef := ip.ref(x.X, env)
	base := baseRef.Get()
	name := x.Name
	switch b := base.(type) {
	case *StructV, *ObjectV:
		ti := typeInfoOf(b)
		fi := ti.field(name)
		ro := ""
		_, isStruct := b.(*StructV)
		if fi != nil && fi.Wrapper != "" && isWrapperSettable(fi.Wrapper) {
			ro = ""
		} else if fi != nil && fi.Let && !baseRef.InInit {
			ro = fmt.Sprintf("'%s' is a 'let' constant", name)
		} else if isStruct {
			ro = baseRef.RO
		}
		if fi == nil {
			if vd := ip.findComputed(ti, name); vd != nil && vd.Set == nil {
				ro = fmt.Sprintf("'%s' is a get-only property", name)
			}
		}
		observed := fi != nil && fi.Decl != nil && (len(fi.Decl.DidSet) > 0 || len(fi.Decl.WillSet) > 0)
		return &Ref{Get: func() Value {
			v := ip.getMember(b, name, x.Pos, env)
			if observed {
				return copyValue(v)
			}
			return v
		}, Set: func(v Value) {
			if ro != "" {
				ip.fail(x.Pos, "cannot assign to property: %s", ro)
			}
			ip.setField(b, ti, name, v, x.Pos)
		}, RO: ro, ROKind: "property"}
	case *TupleV:
		idx := tupleIndex(b, name)
		if idx < 0 {
			ip.fail(x.Pos, "value of tuple type has no member '%s'", name)
		}
		return &Ref{Get: func() Value { return b.E[idx] }, Set: func(v Value) {
			if baseRef.RO != "" {
				ip.fail(x.Pos, "cannot assign to property: %s", baseRef.RO)
			}
			b.E[idx] = copyValue(v)
		}, RO: baseRef.RO}
	case *BindingV:
		if name == "wrappedValue" {
			return &Ref{Get: b.Get, Set: b.Set}
		}
	case *TypeV:
		if b.T != nil {
			if s, ok := ip.staticField(b.T, name); ok {
				slot := s
				return &Ref{Get: slot.get, Set: func(v Value) {
					if slot.Let {
						ip.fail(x.Pos, "cannot assign to property: '%s' is a 'let' constant", name)
					}
					if slot.Set != nil {
						slot.Set(v)
						return
					}
					slot.V = copyValue(ip.coerce(v, slot.T, nil))
				}}
			}
		}
	case *EnumV:
		return &Ref{Get: func() Value { return ip.getMember(b, name, x.Pos, env) }, Set: func(v Value) {
			ip.fail(x.Pos, "cannot assign to property: '%s' is a get-only property", name)
		}, RO: "enum value is immutable"}
	case *NativeObj:
		return &Ref{Get: func() Value { return ip.getMember(b, name, x.Pos, env) }, Set: func(v Value) {
			b.Props[name] = copyValue(v)
		}}
	case *ModelContextV:
		if name == "autosaveEnabled" {
			return &Ref{Get: func() Value { return b.autosaveEnabled }, Set: func(v Value) { b.autosaveEnabled = v == true }}
		}
	}
	return &Ref{Get: func() Value { return ip.getMember(base, name, x.Pos, env) }, Set: func(v Value) {
		ip.fail(x.Pos, "cannot assign to property: '%s' is a get-only property", name)
	}, RO: baseRef.RO}
}

func tupleIndex(t *TupleV, name string) int {
	if len(name) > 0 && name[0] >= '0' && name[0] <= '9' {
		var i int
		fmt.Sscanf(name, "%d", &i)
		if i < len(t.E) {
			return i
		}
		return -1
	}
	for i, l := range t.L {
		if l == name {
			return i
		}
	}
	return -1
}

func (ip *Interp) subscriptRef(x *SubscriptExpr, env *Env) *Ref {
	baseRef := ip.ref(x.X, env)
	base := baseRef.Get()
	args := ip.evalArgs(x.Args, env, false)
	if len(args) == 0 {
		ip.fail(x.Pos, "missing subscript index")
	}
	switch b := base.(type) {
	case *ArrayV:
		if r, ok := args[0].V.(*RangeV); ok {
			lo, hi := ip.rangeBounds(r, len(b.E), x.Pos)
			return &Ref{Get: func() Value {
				return &ArrayV{E: append([]Value(nil), b.E[lo:hi]...), Elem: b.Elem}
			}, Set: func(v Value) {
				nv := v.(*ArrayV)
				ne := append([]Value(nil), b.E[:lo]...)
				ne = append(ne, nv.E...)
				b.E = append(ne, b.E[hi:]...)
			}, RO: baseRef.RO}
		}
		i, ok := args[0].V.(int)
		if !ok {
			ip.fail(x.Pos, "cannot subscript array with value of type '%s' (use an Int index)", typeName(args[0].V))
		}
		return &Ref{Get: func() Value {
			if i < 0 || i >= len(b.E) {
				ip.fail(x.Pos, "Fatal error: Index out of range (index %d, count %d)", i, len(b.E))
			}
			return b.E[i]
		}, Set: func(v Value) {
			if baseRef.RO != "" {
				ip.fail(x.Pos, "cannot assign through subscript: %s", baseRef.RO)
			}
			if i < 0 || i >= len(b.E) {
				ip.fail(x.Pos, "Fatal error: Index out of range (index %d, count %d)", i, len(b.E))
			}
			b.E[i] = copyValue(ip.coerce(v, b.Elem, nil))
		}, RO: baseRef.RO, ROKind: "subscript"}
	case *DictV:
		key := ip.coerce(args[0].V, b.KeyT, nil)
		var def Value
		hasDef := false
		if len(args) > 1 && args[1].Label == "default" {
			def = args[1].V
			hasDef = true
		}
		return &Ref{Get: func() Value {
			v, ok := b.Get(key)
			if !ok {
				if hasDef {
					nv := copyValue(def)
					return nv
				}
				return nil
			}
			return v
		}, Set: func(v Value) {
			if baseRef.RO != "" {
				ip.fail(x.Pos, "cannot assign through subscript: %s", baseRef.RO)
			}
			if v == nil && !hasDef {
				b.Remove(key)
				return
			}
			b.Set(key, copyValue(ip.coerce(v, b.ValT, nil)))
		}, RO: baseRef.RO, ROKind: "subscript"}
	case string:
		if r, ok := args[0].V.(*RangeV); ok {
			_ = r
		}
		ip.fail(x.Pos, "'subscript(_:)' is unavailable: cannot subscript String with an Int, use a String.Index instead (tip: Array(str)[i])")
	case *StructV, *ObjectV:
		ti := typeInfoOf(b)
		for c := ti; c != nil; c = c.Super {
			for _, d := range c.Subscripts {
				vals, set, err := ip.matchArgs(d.Params, args)
				if err != "" {
					continue
				}
				dd := d
				mk := func() *Env {
					env2 := newEnv(ip.globals)
					env2.self, env2.selfT = b, ti
					ip.bindParams(env2, dd.Params, vals, set, ip.globals)
					return env2
				}
				return &Ref{Get: func() Value {
					return ip.execBody(dd.Body, mk(), &FnCtx{})
				}, Set: func(v Value) {
					e2 := mk()
					e2.mutableSelf = true
					e2.vars[nz(dd.SetName, "newValue")] = &Slot{V: v, Let: true}
					ip.execBlock(dd.SetBody, e2, &FnCtx{})
				}}
			}
		}
	case *BindingV:
		bb := b
		return &Ref{Get: func() Value {
			return ip.derivedSubscriptBinding(bb, args, x.Pos)
		}, Set: func(Value) {}}
	case nil:
		ip.fail(x.Pos, "value of optional type must be unwrapped before subscripting")
	}
	ip.fail(x.Pos, "value of type '%s' has no subscripts", typeName(base))
	return nil
}

func (ip *Interp) derivedSubscriptBinding(b *BindingV, args []ArgV, pos Pos) Value {
	return &BindingV{Get: func() Value {
		base := b.Get()
		return ip.subscriptValue(base, args, pos)
	}, Set: func(v Value) {
		base := b.Get()
		switch bb := base.(type) {
		case *ArrayV:
			i, _ := args[0].V.(int)
			if i >= 0 && i < len(bb.E) {
				bb.E[i] = copyValue(v)
			}
		case *DictV:
			bb.Set(args[0].V, copyValue(v))
		}
		b.Set(base)
	}}
}

func (ip *Interp) subscriptValue(base Value, args []ArgV, pos Pos) Value {
	switch bb := base.(type) {
	case *ArrayV:
		i, _ := args[0].V.(int)
		if i < 0 || i >= len(bb.E) {
			ip.fail(pos, "Fatal error: Index out of range")
		}
		return bb.E[i]
	case *DictV:
		v, _ := bb.Get(args[0].V)
		return v
	}
	return nil
}

func (ip *Interp) rangeBounds(r *RangeV, n int, pos Pos) (int, int) {
	lo, hi := 0, n
	if !r.NoLo {
		lo, _ = toInt(r.Lo)
	}
	if !r.NoHi {
		hi, _ = toInt(r.Hi)
		if r.Closed {
			hi++
		}
	}
	if lo < 0 || hi > n || lo > hi {
		ip.fail(pos, "Fatal error: Array index is out of range")
	}
	return lo, hi
}

// ---------- calls ----------

func (ip *Interp) evalArgs(args []Arg, env *Env, localized bool) []ArgV {
	out := make([]ArgV, len(args))
	for i, a := range args {
		var v Value
		if s, ok := a.X.(*StrLit); ok && localized && i == 0 && a.Label == "" {
			v = ip.evalStr(s, env, true)
		} else {
			v = ip.eval(a.X, env)
		}
		out[i] = ArgV{Label: a.Label, V: v, X: a.X, Trailing: a.Trailing}
	}
	return out
}

func (ip *Interp) evalCall(x *CallExpr, env *Env) Value {
	switch fn := x.Fn.(type) {
	case *MemberExpr:
		if id, ok := fn.X.(*Ident); ok {
			if id.Name == "super" {
				return ip.callSuper(fn.Name, ip.evalArgs(x.Args, env, false), env, x.Pos)
			}
			if id.Name == "self" && fn.Name == "init" {
				return ip.selfInit(ip.evalArgs(x.Args, env, false), env, x.Pos)
			}
		}
		recvRef := ip.ref(fn.X, env)
		recv := recvRef.Get()
		localized := false
		if tv, ok := recv.(*TypeV); ok && localizedTypes[tv.Name] {
			localized = true
		}
		args := ip.evalArgs(x.Args, env, localized)
		ip.directCall = true
		return ip.callMethod(recv, recvRef, fn.Name, args, x.Pos, env)
	case *ImplicitMember:
		args := ip.evalArgs(x.Args, env, false)
		return &SymV{Parts: []SymPart{{Name: fn.Name, Args: args, Called: true}}}
	}
	callee := ip.eval(x.Fn, env)
	localized := false
	if tv, ok := callee.(*TypeV); ok && localizedTypes[tv.Name] && tv.T == nil {
		localized = true
	}
	args := ip.evalArgs(x.Args, env, localized)
	ip.directCall = true
	return ip.callValue(callee, args, x.Pos, env)
}

func (ip *Interp) callValue(callee Value, args []ArgV, pos Pos, env *Env) Value {
	calledDirectly := ip.directCall
	ip.directCall = false
	switch f := callee.(type) {
	case *FuncV:
		return ip.callFunc(f, args, pos, calledDirectly)
	case *OverloadV:
		var firstErr string
		for _, fv := range f.Fns {
			if fv.IsClosure {
				return ip.callFunc(fv, args, pos, calledDirectly)
			}
			_, _, err := ip.matchArgs(fv.Params, args)
			if err == "" {
				return ip.callFunc(fv, args, pos, calledDirectly)
			}
			if firstErr == "" {
				firstErr = err
			}
		}
		ip.fail(pos, "%s", firstErr)
	case *NativeV:
		return f.Fn(&CallCtx{ip: ip, Args: args, Pos: pos, Env: env})
	case *TypeV:
		return ip.construct(f, args, pos, env)
	case *SymV:
		n := &SymV{Parts: append([]SymPart(nil), f.Parts...)}
		last := &n.Parts[len(n.Parts)-1]
		last.Args = args
		last.Called = true
		return n
	case *ArrayV:
		if len(f.E) == 1 {
			if tv, ok := f.E[0].(*TypeV); ok {
				ref := &TypeRef{Kind: tyArray, Elem: &TypeRef{Name: tv.Name}}
				if tv.Ref != nil {
					ref.Elem = tv.Ref
				}
				return ip.constructArray(ref, args, pos)
			}
		}
	case *DictV:
		if len(f.Keys) == 1 {
			if tk, ok := f.Keys[0].(*TypeV); ok {
				d := newDict()
				d.KeyT = &TypeRef{Name: tk.Name}
				if tv, ok := f.Vals[0].(*TypeV); ok {
					d.ValT = &TypeRef{Name: tv.Name}
				}
				return d
			}
		}
	case *StructV:
		if ms := ip.findMethods(f.T, "callAsFunction"); ms != nil {
			return ip.callValue(ip.bindMethods(ms, f, f.T, false), args, pos, env)
		}
	case *ObjectV:
		if ms := ip.findMethods(f.T, "callAsFunction"); ms != nil {
			return ip.callValue(ip.bindMethods(ms, f, f.T, false), args, pos, env)
		}
	case *ModBinder:
		return &ModV{Base: f.Base, Name: f.Name, Args: args, Pos: pos}
	case *KeyPathV:
		if len(args) == 1 {
			return ip.applyKeyPath(f, args[0].V, pos)
		}
	case nil:
		ip.fail(pos, "value of optional type must be unwrapped (it is nil)")
	}
	ip.fail(pos, "cannot call value of non-function type '%s'", typeName(callee))
	return nil
}

func isGenericName(decl *FuncDecl, name string) bool {
	if decl == nil {
		return false
	}
	for _, generic := range decl.Generics {
		if generic == name {
			return true
		}
	}
	return false
}

func (ip *Interp) bindGenericTypes(env *Env, f *FuncV, expectedType *TypeRef) {
	if expectedType == nil || f.ResultT == nil || !isGenericName(f.Decl, f.ResultT.Name) {
		return
	}
	typeValue := &TypeV{Name: expectedType.Name, Ref: expectedType}
	if expectedType.Kind == tyNamed {
		typeValue.Name = ip.resolveAlias(expectedType.Name)
		typeValue.T = ip.lookupTypeInfo(typeValue.Name)
		typeValue.Ref = nil
	}
	env.vars[f.ResultT.Name] = &Slot{V: typeValue, Let: true}
}

func labelsFromParams(params []*Param, args []ArgV) []ArgV {
	for _, a := range args {
		if a.Label != "" {
			return args
		}
	}
	labeled := make([]ArgV, len(args))
	for i, a := range args {
		if i < len(params) && !params[i].Variadic {
			a.Label = params[i].Label
		}
		labeled[i] = a
	}
	return labeled
}

func (ip *Interp) callFunc(f *FuncV, args []ArgV, pos Pos, calledDirectly bool) Value {
	penv := newEnv(f.Env)
	if f.Self != nil {
		penv.self = f.Self
		penv.selfT = f.SelfT
		_, isObj := f.Self.(*ObjectV)
		penv.mutableSelf = f.Mutating || isObj
		penv.staticT = f.SelfT
	} else if f.Static {
		penv.staticT = f.SelfT
	}
	expectedType := ip.expectedType
	ip.expectedType = nil
	if f.Decl != nil && len(f.Decl.Generics) > 0 {
		ip.bindGenericTypes(penv, f, expectedType)
	}
	if f.IsClosure {
		ip.bindClosureParams(penv, f, args)
	} else {
		// A function passed as a value (onDelete(perform: deleteBooks)) is called
		// like a closure, so Swift drops its argument labels.
		if !calledDirectly {
			args = labelsFromParams(f.Params, args)
		}
		vals, set, err := ip.matchArgs(f.Params, args)
		if err != "" {
			ip.fail(pos, "%s", err)
		}
		ip.bindParams(penv, f.Params, vals, set, f.Env)
	}
	env := newEnv(penv)
	fc := &FnCtx{}
	ip.depth++
	defer func() { ip.depth-- }()
	if ip.depth > 600 {
		ip.fail(pos, "stack overflow (infinite recursion?)")
	}
	res := ip.execBody(f.Body, env, fc)
	if f.ResultT != nil && !isGenericName(f.Decl, f.ResultT.Name) {
		res = ip.coerce(res, f.ResultT, nil)
	}
	if f.Self != nil && f.Mutating && penv.self != f.Self {
		ip.lastSelf = penv.self
	}
	return res
}

func (ip *Interp) bindClosureParams(env *Env, f *FuncV, args []ArgV) {
	if !f.HasSig {
		for i, a := range args {
			env.vars[fmt.Sprintf("$%d", i)] = &Slot{V: a.V, Let: true}
		}
		// A closure given one tuple can also read its parts as $0, $1, ...
		// (zip(a, b).map { "\($0)\($1)" }).
		if t, ok := argAt(args, 0).(*TupleV); ok && len(args) == 1 && f.MaxDollar >= 1 {
			for i, e := range t.E {
				env.vars[fmt.Sprintf("$%d", i)] = &Slot{V: e, Let: true}
			}
		}
		return
	}
	params := f.Params
	if len(params) > 1 && len(args) == 1 {
		if t, ok := args[0].V.(*TupleV); ok && len(t.E) == len(params) {
			args = make([]ArgV, len(t.E))
			for i, e := range t.E {
				args[i] = ArgV{V: e}
			}
		}
	}
	for i, p := range params {
		var v Value
		if i < len(args) {
			v = args[i].V
		}
		if p.Name == "_" {
			continue
		}
		if strings.HasPrefix(p.Name, "$") {
			env.vars[p.Name] = &Slot{V: v, Let: true}
			if b, ok := v.(*BindingV); ok {
				bb := b
				env.vars[p.Name[1:]] = &Slot{Get: bb.Get, Set: bb.Set}
			}
			continue
		}
		v = ip.coerce(v, p.Type, nil)
		env.vars[p.Name] = &Slot{V: copyValue(v), Let: true}
	}
}

func (ip *Interp) bindMethods(ms []*FuncDecl, self Value, ti *TypeInfo, static bool) Value {
	mk := func(d *FuncDecl) *FuncV {
		return &FuncV{Name: d.Name, Params: d.Params, Body: d.Body, Env: ip.globals, Self: self, SelfT: ti, Decl: d,
			ResultT: d.Result, HasSig: true, Static: static, Mutating: d.Mods["mutating"], Builder: hasAttr(d.Attrs, "ViewBuilder")}
	}
	if len(ms) == 1 {
		return mk(ms[0])
	}
	o := &OverloadV{}
	for _, d := range ms {
		o.Fns = append(o.Fns, mk(d))
	}
	return o
}

func (ip *Interp) callSuper(name string, args []ArgV, env *Env, pos Pos) Value {
	var self Value
	var owner *TypeInfo
	for e := env; e != nil; e = e.parent {
		if e.self != nil {
			self = e.self
			owner = e.selfT
			break
		}
	}
	if owner != nil && owner.Super == nil && name == "init" && owner.Decl != nil && len(owner.Decl.Inherits) > 0 && owner.Decl.Inherits[0] == "NSObject" {
		return Void
	}
	if owner == nil || owner.Super == nil {
		ip.fail(pos, "'super' members cannot be referenced in a root class")
	}
	sup := owner.Super
	if name == "init" {
		for _, d := range sup.Inits {
			vals, set, err := ip.matchArgs(d.Params, args)
			if err == "" {
				ip.runInit(sup, d, self, vals, set, pos)
				return Void
			}
		}
		if len(sup.Inits) == 0 && len(args) == 0 {
			return Void
		}
		ip.fail(pos, "no matching 'super.init' for these arguments")
	}
	if ms := ip.findMethods(sup, name); ms != nil {
		return ip.callValue(ip.bindMethods(ms, self, sup, false), args, pos, env)
	}
	ip.fail(pos, "value of type '%s' has no member '%s'", sup.Name, name)
	return nil
}

func (ip *Interp) selfInit(args []ArgV, env *Env, pos Pos) Value {
	var e *Env
	for e = env; e != nil; e = e.parent {
		if e.self != nil || e.inInit {
			break
		}
	}
	if e == nil {
		ip.fail(pos, "'self.init' is only allowed inside an initializer")
	}
	if bi, ok := e.self.(*builtinInitSelf); ok {
		v := ip.construct(&TypeV{Name: bi.Name}, args, pos, env)
		e.self = v
		return Void
	}
	ti := e.selfT
	nv := ip.instantiate(ti, args, pos)
	switch s := e.self.(type) {
	case *StructV:
		if n, ok := nv.(*StructV); ok {
			for k, v := range n.F {
				s.F[k] = v
			}
		}
	case *ObjectV:
		if n, ok := nv.(*ObjectV); ok {
			for k, v := range n.F {
				s.F[k] = v
			}
		}
	default:
		e.self = nv
	}
	return Void
}

type builtinInitSelf struct{ Name string }

func (ip *Interp) construct(t *TypeV, args []ArgV, pos Pos, env *Env) Value {
	if t.T != nil {
		return ip.instantiate(t.T, args, pos)
	}
	if t.Ref != nil && t.Ref.Kind == tyArray {
		return ip.constructArray(t.Ref, args, pos)
	}
	if et, ok := ip.ext[t.Name]; ok {
		for _, d := range et.Inits {
			vals, set, err := ip.matchArgs(d.Params, args)
			if err != "" {
				continue
			}
			placeholder := &builtinInitSelf{Name: t.Name}
			envI := newEnv(ip.globals)
			envI.self = placeholder
			envI.selfT = et
			envI.inInit = true
			envI.mutableSelf = true
			ip.bindParams(envI, d.Params, vals, set, ip.globals)
			fc := &FnCtx{isInit: true}
			ip.execBody(d.Body, newEnv(envI), fc)
			if fc.returnedNil {
				return nil
			}
			if envI.self == placeholder {
				ip.fail(pos, "'self.init' isn't called on all paths before returning from initializer")
			}
			return envI.self
		}
	}
	if v, ok := ip.constructSwiftDataType(t, args, pos); ok {
		return v
	}
	if v, ok := ip.constructMediaType(t, args, pos); ok {
		return v
	}
	return ip.builtinConstruct(t.Name, args, pos, env)
}

func (ip *Interp) callMethod(recv Value, recvRef *Ref, name string, args []ArgV, pos Pos, env *Env) Value {
	switch r := recv.(type) {
	case *TypeV:
		if r.T == nil {
			if isModuleName(r.Name) {
				return ip.callValue(ip.evalIdent(&Ident{Pos: pos, Name: name}, ip.globals), args, pos, env)
			}
			if et, ok := ip.ext[r.Name]; ok {
				if ms, ok := et.StaticMethods[name]; ok {
					f := ip.bindMethods(ms, nil, et, true)
					return ip.callValue(f, args, pos, env)
				}
			}
			if name == "init" {
				return ip.construct(r, args, pos, env)
			}
			return ip.builtinStaticCall(r.Name, name, args, pos, env)
		}
		if name == "init" {
			return ip.construct(r, args, pos, env)
		}
		fn := ip.typeMember(r, name, pos)
		return ip.callValue(fn, args, pos, env)
	case *StructV, *ObjectV, *EnumV:
		fn := ip.getMember(recv, name, pos, env)
		mut := false
		switch f := fn.(type) {
		case *FuncV:
			mut = f.Mutating
		case *OverloadV:
			mut = f.Fns[0].Mutating
		}
		if mut && recvRef != nil && recvRef.RO != "" {
			ip.fail(pos, "cannot use mutating member on immutable value: %s", recvRef.RO)
		}
		ip.lastSelf = nil
		res := ip.callValue(fn, args, pos, env)
		if mut && ip.lastSelf != nil && recvRef != nil {
			ns := ip.lastSelf
			ip.lastSelf = nil
			recvRef.Set(ns)
		}
		return res
	case *BindingV, *ProjV, *SymV, *ViewV, *ModV, *GeoV, *TupleV, *SuperV, *KeyPathV, *FuncV, *NativeV:
		if c, ok := recv.(*ColorV); ok {
			_ = c
		}
		if _, isFn := recv.(*FuncV); isFn && name == "callAsFunction" {
			return ip.callValue(recv, args, pos, env)
		}
		fn := ip.getMember(recv, name, pos, env)
		return ip.callValue(fn, args, pos, env)
	case nil:
		if name == "map" || name == "flatMap" {
			return nil
		}
		ip.fail(pos, "Fatal error: Unexpectedly found nil while calling '%s' (value of optional type must be unwrapped)", name)
	}
	if v, ok := ip.builtinMethod(recv, recvRef, name, args, pos, env); ok {
		return v
	}
	// Optionals hold their value directly, so `.map` on a plain value that has
	// no map of its own is Optional.map.
	if (name == "map" || name == "flatMap") && len(args) == 1 {
		return ip.callValue(args[0].V, []ArgV{{V: recv}}, pos, env)
	}
	if v, ok := ip.builtinProperty(recv, name, pos); ok {
		switch v.(type) {
		case *NativeV, *FuncV, *OverloadV:
			return ip.callValue(v, args, pos, env)
		}
	}
	if fn := ip.extMethodFor(recv, name); fn != nil {
		return ip.callValue(fn, args, pos, env)
	}
	if isViewValue(recv) {
		fn := ip.getMember(recv, name, pos, env)
		return ip.callValue(fn, args, pos, env)
	}
	ip.fail(pos, "value of type '%s' has no member '%s'", typeName(recv), name)
	return nil
}

func builtinTypeKey(v Value) []string {
	switch x := v.(type) {
	case int:
		return []string{"Int", "BinaryInteger", "Numeric", "Comparable"}
	case float64:
		return []string{"Double", "CGFloat", "BinaryFloatingPoint", "FloatingPoint", "Numeric", "Comparable"}
	case string:
		return []string{"String", "StringProtocol", "Comparable"}
	case Character:
		return []string{"Character"}
	case bool:
		return []string{"Bool"}
	case *ArrayV:
		return []string{"Array", "Collection", "Sequence", "RandomAccessCollection"}
	case *DictV:
		return []string{"Dictionary", "Collection", "Sequence"}
	case *SetV:
		return []string{"Set", "Collection", "Sequence"}
	case *ColorV:
		return []string{"Color", "View", "ShapeStyle"}
	case *DateV:
		return []string{"Date"}
	case *FontV:
		return []string{"Font"}
	case *ViewV, *ModV:
		return []string{"View"}
	case *RangeV:
		return []string{"Range", "ClosedRange"}
	case *NativeObj:
		return []string{x.Kind}
	case *UUIDV:
		return []string{"UUID"}
	case *URLV:
		return []string{"URL"}
	case *DataV:
		return []string{"Data"}
	case *UIImageV:
		return []string{"UIImage"}
	}
	return nil
}

func (ip *Interp) extMethodFor(recv Value, name string) Value {
	for _, k := range builtinTypeKey(recv) {
		if et, ok := ip.ext[k]; ok {
			if ms, ok := et.Methods[name]; ok {
				return ip.bindMethods(ms, recv, et, false)
			}
		}
	}
	return nil
}

func (ip *Interp) extComputedFor(recv Value, name string) (Value, bool) {
	for _, k := range builtinTypeKey(recv) {
		if et, ok := ip.ext[k]; ok {
			if vd, ok := et.Computed[name]; ok {
				return ip.runGetter(vd.Get, ip.globals, recv, et, vd.Type), true
			}
		}
	}
	return nil, false
}

func (ip *Interp) viewExtMethod(name string) []*FuncDecl {
	if et, ok := ip.ext["View"]; ok {
		if ms, ok := et.Methods[name]; ok {
			return ms
		}
	}
	return nil
}

// ---------- members ----------

func (ip *Interp) typeMemberMaybe(t *TypeV, name string, pos Pos) Value {
	ti := t.T
	if ti.Kind == "enum" {
		if c, ok := ti.CaseMap[name]; ok {
			if len(c.Params) == 0 {
				return &EnumV{T: ti, Case: name}
			}
			cc := c
			return &NativeV{Name: name, Fn: func(cx *CallCtx) Value { return ip.makeEnumCase(ti, cc, cx.Args, cx.Pos) }}
		}
		if _, declared := ip.staticField(ti, "allCases"); name == "allCases" && !declared {
			a := &ArrayV{Elem: &TypeRef{Name: ti.Name}}
			for _, c := range ti.Cases {
				if len(c.Params) == 0 {
					a.E = append(a.E, &EnumV{T: ti, Case: c.Name})
				}
			}
			return a
		}
	}
	if s, ok := ip.staticField(ti, name); ok {
		return s.get()
	}
	if ms := ip.findStaticMethods(ti, name); ms != nil {
		return ip.bindMethods(ms, nil, ti, true)
	}
	if name == "init" {
		tt := t
		return &NativeV{Name: "init", Fn: func(cx *CallCtx) Value { return ip.construct(tt, cx.Args, cx.Pos, cx.Env) }}
	}
	if name == "self" || name == "Type" {
		return t
	}
	if nt, ok := ip.types[ti.Name+"."+name]; ok {
		return &TypeV{Name: nt.Name, T: nt}
	}
	return nil
}

// `Swift.min(...)` and `Foundation.Date()` name the module explicitly.
func isModuleName(name string) bool {
	switch name {
	case "Swift", "Foundation", "SwiftUI", "SwiftData", "UIKit", "Observation", "PhotosUI", "MapKit", "Combine", "CoreLocation":
		return true
	}
	return false
}

func (ip *Interp) typeMember(t *TypeV, name string, pos Pos) Value {
	if t.T == nil && name == "self" {
		return t
	}
	if t.T == nil && isModuleName(t.Name) {
		return ip.evalIdent(&Ident{Pos: pos, Name: name}, ip.globals)
	}
	if t.T == nil && name == "init" {
		builtin := t
		return &NativeV{Name: "init", Fn: func(cx *CallCtx) Value { return ip.construct(builtin, cx.Args, cx.Pos, cx.Env) }}
	}
	if t.T == nil {
		if et, ok := ip.ext[t.Name]; ok {
			if s, ok := ip.staticField(et, name); ok {
				return s.get()
			}
			if ms, ok := et.StaticMethods[name]; ok {
				return ip.bindMethods(ms, nil, et, true)
			}
		}
		return ip.builtinStatic(t.Name, name, pos)
	}
	if v := ip.typeMemberMaybe(t, name, pos); v != nil {
		return v
	}
	ip.fail(pos, "type '%s' has no member '%s'", t.Name, name)
	return nil
}

func (ip *Interp) getMember(base Value, name string, pos Pos, env *Env) Value {
	// `[Item].self` and `[String: Item].self` are types written as literals.
	if name == "self" {
		if t := typeRefFromValue(base); t != nil && t.Kind != tyNamed {
			return &TypeV{Name: t.String(), Ref: t}
		}
	}
	switch b := base.(type) {
	case nil:
		ip.fail(pos, "Fatal error: Unexpectedly found nil while accessing '%s' (value of optional type must be unwrapped)", name)
	case *TypeV:
		return ip.typeMember(b, name, pos)
	case *StructV, *ObjectV:
		ti := typeInfoOf(b)
		if v, ok := ip.getField(b, ti, name, pos); ok {
			return v
		}
		if ms := ip.findMethods(ti, name); ms != nil {
			return ip.bindMethods(ms, b, ti, false)
		}
		if ti.IsView() {
			if ms := ip.viewExtMethod(name); ms != nil {
				return ip.bindMethods(ms, b, ti, false)
			}
			if knownModifiers[name] {
				return &ModBinder{Base: b, Name: name}
			}
		}
		if ip.isConformingShapeOrStyle(ti) {
			if knownModifiers[name] {
				return &ModBinder{Base: b, Name: name}
			}
		}
		if name == "hashValue" {
			return len(hashKey(b))
		}
		if name == "self" {
			return b
		}
		if obj, isObject := b.(*ObjectV); isObject && ti.IsModel {
			if v, ok := ip.modelObjectMember(obj, name); ok {
				return v
			}
		}
		ip.fail(pos, "value of type '%s' has no member '%s'", ti.Name, name)
	case *EnumV:
		if name == "rawValue" {
			if b.T.RawType == "" {
				ip.fail(pos, "value of type '%s' has no member 'rawValue' (declare it as 'enum %s: String' or ': Int')", b.T.Name, b.T.Name)
			}
			return b.T.RawVals[b.Case]
		}
		if vd := ip.findComputed(b.T, name); vd != nil {
			return ip.runGetter(vd.Get, ip.globals, b, b.T, vd.Type)
		}
		if ms := ip.findMethods(b.T, name); ms != nil {
			return ip.bindMethods(ms, b, b.T, false)
		}
		if name == "hashValue" {
			return len(b.Case)
		}
		if name == "self" {
			return b
		}
		if name == "id" && b.T.conformsTo("Identifiable") {
			return b
		}
		if name == "localizedDescription" {
			return describe(b)
		}
		ip.fail(pos, "value of type '%s' has no member '%s'", b.T.Name, name)
	case *SymV:
		n := &SymV{Parts: append(append([]SymPart(nil), b.Parts...), SymPart{Name: name})}
		return n
	case *BindingV:
		switch name {
		case "wrappedValue":
			return b.Get()
		case "projectedValue":
			return b
		}
		bb := b
		return &BindingV{Get: func() Value {
			return ip.getMember(bb.Get(), name, pos, env)
		}, Set: func(v Value) {
			base := bb.Get()
			if ti := typeInfoOf(base); ti != nil {
				if _, isObj := base.(*ObjectV); isObj {
					ip.setField(base, ti, name, v, pos)
					return
				}
				nb := copyValue(base)
				ip.setField(nb, ti, name, v, pos)
				bb.Set(nb)
				return
			}
			if t, ok := base.(*TupleV); ok {
				nt := copyValue(t).(*TupleV)
				if i := tupleIndex(nt, name); i >= 0 {
					nt.E[i] = v
				}
				bb.Set(nt)
				return
			}
			ip.fail(pos, "cannot assign to '%s' through a binding", name)
		}, Name: name}
	case *ProjV:
		pb := b
		return &BindingV{Get: func() Value {
			return ip.getMember(pb.Get(), name, pos, env)
		}, Set: func(v Value) {
			obj := pb.Get()
			if ti := typeInfoOf(obj); ti != nil {
				ip.setField(obj, ti, name, v, pos)
			}
		}, Name: name}
	case *TupleV:
		i := tupleIndex(b, name)
		if i < 0 {
			ip.fail(pos, "value of tuple type has no member '%s'", name)
		}
		return b.E[i]
	case *SuperV:
		if ms := ip.findMethods(b.Owner.Super, name); ms != nil {
			return ip.bindMethods(ms, b.Obj, b.Owner.Super, false)
		}
		ip.fail(pos, "value of type '%s' has no member '%s'", b.Owner.Super.Name, name)
	case *KeyPathV:
		return ip.fail2(pos, "key path has no member '%s'", name)
	}
	if v, ok := ip.builtinProperty(base, name, pos); ok {
		return v
	}
	if v, ok := ip.extComputedFor(base, name); ok {
		return v
	}
	if fn := ip.extMethodFor(base, name); fn != nil {
		return fn
	}
	if isViewValue(base) {
		if ms := ip.viewExtMethod(name); ms != nil {
			return ip.bindMethods(ms, base, ip.ext["View"], false)
		}
		if knownModifiers[name] {
			return &ModBinder{Base: base, Name: name}
		}
		ip.fail(pos, "value of type '%s' has no member '%s' (unknown or unsupported modifier)", typeName(base), name)
	}
	if hasBuiltinMethod(base, name) {
		bv := base
		return &NativeV{Name: name, Fn: func(c *CallCtx) Value {
			v, ok := ip.builtinMethod(bv, &Ref{Get: func() Value { return bv }, Set: func(Value) {}}, name, c.Args, c.Pos, c.Env)
			if !ok {
				ip.fail(c.Pos, "value of type '%s' has no member '%s'", typeName(bv), name)
			}
			return v
		}}
	}
	ip.fail(pos, "value of type '%s' has no member '%s'", typeName(base), name)
	return nil
}

func (ip *Interp) fail2(pos Pos, f string, a ...interface{}) Value {
	ip.fail(pos, f, a...)
	return nil
}

func (ip *Interp) isConformingShapeOrStyle(ti *TypeInfo) bool {
	return ti.conformsTo("Shape") || ti.conformsTo("ShapeStyle")
}

func (ip *Interp) applyKeyPath(kp *KeyPathV, v Value, pos Pos) Value {
	cur := v
	for _, p := range kp.Path {
		if p == "self" {
			continue
		}
		cur = ip.getMember(cur, p, pos, ip.globals)
	}
	return cur
}

// ---------- operators ----------

func (ip *Interp) evalBinary(x *BinaryExpr, env *Env) Value {
	switch x.Op {
	case "&&":
		if !ip.evalBool(x.L, env) {
			return false
		}
		return ip.evalBool(x.R, env)
	case "||":
		if ip.evalBool(x.L, env) {
			return true
		}
		return ip.evalBool(x.R, env)
	case "??":
		l := ip.eval(x.L, env)
		if l != nil {
			return l
		}
		return ip.eval(x.R, env)
	case "...", "..<":
		lo := ip.eval(x.L, env)
		hi := ip.eval(x.R, env)
		if c, ok := compareValues(lo, hi); ok && c > 0 {
			ip.fail(x.Pos, "Fatal error: Range requires lowerBound <= upperBound")
		}
		if _, ok := lo.(float64); ok {
			if i, ok := hi.(int); ok {
				hi = float64(i)
			}
		}
		if _, ok := hi.(float64); ok {
			if i, ok := lo.(int); ok {
				lo = float64(i)
			}
		}
		return &RangeV{Lo: lo, Hi: hi, Closed: x.Op == "..."}
	}
	l := ip.eval(x.L, env)
	r := ip.eval(x.R, env)
	if ls, ok := l.(*SymV); ok {
		l = ip.contextSym(ls, r, x.Pos)
	}
	if rs, ok := r.(*SymV); ok {
		r = ip.contextSym(rs, l, x.Pos)
	}
	// Swift lets an integer literal stand in for a Double, so `2 * 1.5` is valid.
	if li, ok := l.(int); ok {
		if _, rf := r.(float64); rf {
			l = float64(li)
		}
	}
	if ri, ok := r.(int); ok {
		if _, lf := l.(float64); lf {
			r = float64(ri)
		}
	}
	return ip.binop(x.Op, l, r, x.Pos)
}

func (ip *Interp) contextSym(s *SymV, other Value, pos Pos) Value {
	switch o := other.(type) {
	case float64:
		if v := ip.resolveSymAs("Double", s); v != nil {
			return v
		}
	case int:
		if v := ip.resolveSymAs("Int", s); v != nil {
			return v
		}
		if v := ip.resolveSymAs("Double", s); v != nil {
			return v
		}
	case *ColorV:
		if v := ip.resolveSymAs("Color", s); v != nil {
			return v
		}
	case *DateV:
		if v := ip.resolveSymAs("Date", s); v != nil {
			return v
		}
	case *SymV:
		return s
	default:
		if ti := typeInfoOf(o); ti != nil {
			return ip.symToType(ti, s, pos)
		}
	}
	return s
}

func (ip *Interp) userOperator(op string, l, r Value, pos Pos) (Value, bool) {
	var ti *TypeInfo
	if t := typeInfoOf(l); t != nil {
		ti = t
	} else if t := typeInfoOf(r); t != nil {
		ti = t
	}
	if ti == nil {
		return nil, false
	}
	if ms := ip.findStaticMethods(ti, op); ms != nil {
		f := ip.bindMethods(ms, nil, ti, true)
		return ip.callValue(f, []ArgV{{V: l}, {V: r}}, pos, ip.globals), true
	}
	return nil, false
}

func (ip *Interp) equals(l, r Value, pos Pos) bool {
	if v, ok := ip.userOperator("==", l, r, pos); ok {
		return v.(bool)
	}
	if s, ok := r.(*SymV); ok {
		if ti := typeInfoOf(l); ti != nil {
			r = ip.symToType(ti, s, pos)
		} else if c, ok := l.(*ColorV); ok {
			_ = c
			r = ip.resolveSymAs("Color", s)
		} else if l != nil {
			if rv := ip.resolveSymLoose(s); rv != nil {
				r = rv
			}
		}
	}
	if s, ok := l.(*SymV); ok {
		if ti := typeInfoOf(r); ti != nil {
			l = ip.symToType(ti, s, pos)
		}
	}
	return valueEquals(l, r)
}

func (ip *Interp) binop(op string, l, r Value, pos Pos) Value {
	switch op {
	case "==":
		return ip.equals(l, r, pos)
	case "!=":
		if v, ok := ip.userOperator("!=", l, r, pos); ok {
			return v
		}
		return !ip.equals(l, r, pos)
	case "===":
		return l == r
	case "!==":
		return l != r
	case "<", ">", "<=", ">=":
		if v, ok := ip.userOperator(op, l, r, pos); ok {
			return v
		}
		if op != "<" {
			if _, ok := ip.userOperator("<", l, r, pos); ok {
				lt := func(a, b Value) bool { v, _ := ip.userOperator("<", a, b, pos); return v.(bool) }
				switch op {
				case ">":
					return lt(r, l)
				case "<=":
					return !lt(r, l)
				case ">=":
					return !lt(l, r)
				}
			}
		}
		c, ok := compareValues(l, r)
		if !ok {
			ip.fail(pos, "binary operator '%s' cannot be applied to operands of type '%s' and '%s'", op, typeName(l), typeName(r))
		}
		switch op {
		case "<":
			return c < 0
		case ">":
			return c > 0
		case "<=":
			return c <= 0
		}
		return c >= 0
	case "~=":
		if rg, ok := l.(*RangeV); ok {
			return ip.rangeContains(rg, r)
		}
		return valueEquals(l, r)
	}
	if v, ok := ip.userOperator(op, l, r, pos); ok {
		return v
	}
	switch a := l.(type) {
	case int:
		if b, ok := r.(int); ok {
			switch op {
			case "+", "&+":
				return a + b
			case "-", "&-":
				return a - b
			case "*", "&*":
				return a * b
			case "/":
				if b == 0 {
					ip.fail(pos, "Fatal error: Division by zero")
				}
				return a / b
			case "%":
				if b == 0 {
					ip.fail(pos, "Fatal error: Division by zero in remainder operation")
				}
				return a % b
			case "&":
				return a & b
			case "|":
				return a | b
			case "^":
				return a ^ b
			case "<<":
				return a << uint(b)
			case ">>":
				return a >> uint(b)
			}
		}
	case float64:
		if b, ok := r.(float64); ok {
			switch op {
			case "+":
				return a + b
			case "-":
				return a - b
			case "*":
				return a * b
			case "/":
				return a / b
			case "%":
				ip.fail(pos, "'%%' is unavailable: For floating point numbers use truncatingRemainder instead")
			}
		}
	case string:
		switch b := r.(type) {
		case string:
			if op == "+" {
				return a + b
			}
		case Character:
			if op == "+" {
				return a + string(b)
			}
		}
	case Character:
		switch b := r.(type) {
		case string:
			if op == "+" {
				return string(a) + b
			}
		case Character:
			if op == "+" {
				return string(a) + string(b)
			}
		}
	case *ArrayV:
		if b, ok := r.(*ArrayV); ok && op == "+" {
			n := &ArrayV{E: make([]Value, 0, len(a.E)+len(b.E)), Elem: a.Elem}
			for _, e := range a.E {
				n.E = append(n.E, copyValue(e))
			}
			for _, e := range b.E {
				n.E = append(n.E, copyValue(e))
			}
			return n
		}
	case *DateV:
		if f, ok := toFloat(r); ok {
			switch op {
			case "+":
				return &DateV{T: a.T.Add(secs(f))}
			case "-":
				return &DateV{T: a.T.Add(-secs(f))}
			}
		}
		if b, ok := r.(*DateV); ok && op == "-" {
			return a.T.Sub(b.T).Seconds()
		}
	case *DeadlineV:
		if f, ok := toFloat(r); ok {
			switch op {
			case "+":
				return &DeadlineV{T: a.T.Add(secs(f))}
			}
		}
	case *SymV:
		if a.Name() == "now" {
			if f, ok := toFloat(r); ok && op == "+" {
				return &DeadlineV{T: nowTime().Add(secs(f))}
			}
			if s, ok := r.(*SymV); ok && op == "+" {
				return &DeadlineV{T: nowTime().Add(secs(ip.durationSeconds(s)))}
			}
		}
	case *ViewV:
		if (a.Kind == "Text" || a.Kind == "TextConcat") && op == "+" && isTextValue(r) {
			return concatText(l, r)
		}
	case *ModV:
		if op == "+" && isTextValue(l) && isTextValue(r) {
			return concatText(l, r)
		}
	case *SizeV:
		if f, ok := toFloat(r); ok {
			switch op {
			case "*":
				return &SizeV{a.W * f, a.H * f}
			case "/":
				return &SizeV{a.W / f, a.H / f}
			}
		}
	}
	if math.IsNaN(0) {
		return nil
	}
	ip.fail(pos, "binary operator '%s' cannot be applied to operands of type '%s' and '%s'", op, typeName(l), typeName(r))
	return nil
}

func (ip *Interp) evalAssign(x *AssignExpr, env *Env) {
	if id, ok := x.L.(*Ident); ok && id.Name == "_" {
		ip.eval(x.R, env)
		return
	}
	if t, ok := x.L.(*TupleExpr); ok && x.Op == "=" {
		rv := ip.eval(x.R, env)
		tv, ok := rv.(*TupleV)
		if !ok || len(tv.E) != len(t.Elems) {
			ip.fail(x.Pos, "cannot assign value of type '%s' to a tuple", typeName(rv))
		}
		refs := make([]*Ref, len(t.Elems))
		for i, el := range t.Elems {
			refs[i] = ip.ref(el, env)
		}
		for i, r := range refs {
			r.Set(tv.E[i])
		}
		return
	}
	r := ip.ref(x.L, env)
	if r.RO != "" {
		kind := "value"
		if r.ROKind == "property" {
			kind = "property"
		}
		if r.ROKind == "subscript" {
			ip.fail(x.Pos, "cannot assign through subscript: %s", r.RO)
		}
		ip.fail(x.Pos, "cannot assign to %s: %s", kind, r.RO)
	}
	if x.Op == "=" {
		v := ip.eval(x.R, env)
		if i, ok := v.(int); ok {
			if _, isF := r.Get().(float64); isF {
				v = float64(i)
			}
		}
		if s, ok := v.(*SymV); ok {
			old := r.Get()
			if ti := typeInfoOf(old); ti != nil {
				v = ip.symToType(ti, s, x.Pos)
			} else if _, isColor := old.(*ColorV); isColor {
				if c := ip.resolveSymAs("Color", s); c != nil {
					v = c
				}
			}
		}
		r.Set(v)
		return
	}
	old := r.Get()
	rv := ip.eval(x.R, env)
	op := x.Op[:len(x.Op)-1]
	if _, isF := old.(float64); isF {
		if i, ok := rv.(int); ok {
			rv = float64(i)
		}
	}
	if a, ok := old.(*ArrayV); ok && op == "+" {
		if b, ok := rv.(*ArrayV); ok {
			for _, e := range b.E {
				a.E = append(a.E, copyValue(ip.coerce(e, a.Elem, nil)))
			}
			r.Set(a)
			return
		}
	}
	r.Set(ip.binop(op, old, rv, x.Pos))
}
