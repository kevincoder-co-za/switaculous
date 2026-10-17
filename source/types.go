package main

import (
	"fmt"
	"sort"
	"strings"
)

type FieldInfo struct {
	Name     string
	Decl     *VarDecl
	Wrapper  string
	WrapArgs []Arg
	Type     *TypeRef
	Let      bool
	Init     Expr
}

type TypeInfo struct {
	Name          string
	Kind          string
	Decl          *TypeDecl
	Fields        []*FieldInfo
	FieldMap      map[string]*FieldInfo
	Computed      map[string]*VarDecl
	Methods       map[string][]*FuncDecl
	StaticMethods map[string][]*FuncDecl
	StaticFields  map[string]*VarDecl
	StaticVals    map[string]*Slot
	Inits         []*FuncDecl
	HasBodyInit   bool
	Cases         []*EnumCase
	CaseMap       map[string]*EnumCase
	RawVals       map[string]Value
	RawType       string
	Super         *TypeInfo
	Conforms      map[string]bool
	Subscripts    []*FuncDecl
	Aliases       map[string]*TypeRef
	Outer         string
	IsModel       bool
}

func newTypeInfo(name, kind string) *TypeInfo {
	return &TypeInfo{
		Name: name, Kind: kind,
		FieldMap:      map[string]*FieldInfo{},
		Computed:      map[string]*VarDecl{},
		Methods:       map[string][]*FuncDecl{},
		StaticMethods: map[string][]*FuncDecl{},
		StaticFields:  map[string]*VarDecl{},
		StaticVals:    map[string]*Slot{},
		CaseMap:       map[string]*EnumCase{},
		RawVals:       map[string]Value{},
		Conforms:      map[string]bool{},
		Aliases:       map[string]*TypeRef{},
	}
}

func (t *TypeInfo) IsView() bool {
	for c := t; c != nil; c = c.Super {
		if c.Conforms["View"] || c.Conforms["UIViewControllerRepresentable"] || c.Conforms["UIViewRepresentable"] {
			return true
		}
	}
	return false
}

func (t *TypeInfo) conformsTo(name string) bool {
	for c := t; c != nil; c = c.Super {
		if c.Name == name || c.Conforms[name] {
			return true
		}
	}
	return false
}

func (t *TypeInfo) field(name string) *FieldInfo {
	for c := t; c != nil; c = c.Super {
		if f, ok := c.FieldMap[name]; ok {
			return f
		}
	}
	return nil
}

func (t *TypeInfo) allFields() []*FieldInfo {
	var chain []*TypeInfo
	for c := t; c != nil; c = c.Super {
		chain = append([]*TypeInfo{c}, chain...)
	}
	var out []*FieldInfo
	for _, c := range chain {
		out = append(out, c.Fields...)
	}
	return out
}

var knownWrappers = map[string]bool{
	"State": true, "Binding": true, "ObservedObject": true, "StateObject": true, "EnvironmentObject": true,
	"Environment": true, "Published": true, "FocusState": true, "AppStorage": true, "SceneStorage": true,
	"Namespace": true, "GestureState": true, "Bindable": true, "ScaledMetric": true, "Query": true,
	"FocusedValue": true, "AccessibilityFocusState": true, "UIApplicationDelegateAdaptor": true,
}

// ---------- Program loading ----------

type PreviewInfo struct {
	Name string
	File string
	Body []Stmt
	Kind string // preview, provider, app
	T    *TypeInfo
	Pos  Pos
}

type pendingExt struct {
	d *TypeDecl
}

func (ip *Interp) Load(files map[string][]Stmt) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if re, ok := r.(*RuntimeError); ok {
				err = &SwiftError{re.Pos, re.Msg}
				return
			}
			if se, ok := r.(*SwiftError); ok {
				err = se
				return
			}
			panic(r)
		}
	}()
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var exts []*TypeDecl
	var typeDecls []*TypeDecl
	for _, fn := range names {
		for _, s := range files[fn] {
			switch d := s.(type) {
			case *TypeDecl:
				if d.Kind == "extension" {
					exts = append(exts, d)
				} else {
					typeDecls = append(typeDecls, ip.registerType(d, "")...)
				}
			case *FuncDecl:
				fv := &FuncV{Name: d.Name, Params: d.Params, Body: d.Body, Env: ip.globals, Decl: d, ResultT: d.Result, HasSig: true}
				if old, ok := ip.globals.vars[d.Name]; ok {
					if of, ok2 := old.V.(*OverloadV); ok2 {
						of.Fns = append(of.Fns, fv)
					} else if f0, ok2 := old.V.(*FuncV); ok2 {
						old.V = &OverloadV{Fns: []*FuncV{f0, fv}}
					}
				} else {
					ip.globals.vars[d.Name] = &Slot{V: fv, Let: true}
				}
			case *VarDecl:
				ip.defineGlobalVar(d)
			case *BlockStmt:
				for _, b := range d.Body {
					if vd, ok := b.(*VarDecl); ok {
						ip.defineGlobalVar(vd)
					}
				}
			case *PreviewDecl:
				name := d.Name
				ip.previews = append(ip.previews, &PreviewInfo{Name: name, File: fn, Body: d.Body, Kind: "preview", Pos: d.Pos})
			case *TypeAliasDecl:
				ip.aliases[d.Name] = d.T
			case *ExprStmt:
				return &SwiftError{d.X.P(), "expressions are not allowed at the top level"}
			case nil:
			default:
				return &SwiftError{Pos{File: fn, Line: 1, Col: 1}, fmt.Sprintf("statements are not allowed at the top level (%T)", s)}
			}
		}
	}
	for _, d := range typeDecls {
		ti := ip.types[ip.fullName(d)]
		ip.addMembers(ti, d.Members)
		for _, inh := range d.Inherits {
			ti.Conforms[inh] = true
		}
	}
	for _, d := range exts {
		if ti, ok := ip.types[d.Name]; ok && ti.Kind != "protocol" {
			ip.addMembers(ti, d.Members)
			for _, inh := range d.Inherits {
				ti.Conforms[inh] = true
			}
		} else {
			if pt, ok := ip.types[d.Name]; ok && pt.Kind == "protocol" {
				for _, inh := range d.Inherits {
					pt.Conforms[inh] = true
				}
			} else if !builtinTypeNames[d.Name] && !ip.isKnownProtocol(d.Name) {
				return &SwiftError{d.Pos, fmt.Sprintf("cannot find type '%s' in scope", d.Name)}
			}
			et, ok := ip.ext[d.Name]
			if !ok {
				et = newTypeInfo(d.Name, "extension")
				ip.ext[d.Name] = et
			}
			ip.addMembers(et, d.Members)
		}
	}
	for _, d := range typeDecls {
		ti := ip.types[ip.fullName(d)]
		if ti.Kind == "class" && len(d.Inherits) > 0 {
			if sup, ok := ip.types[d.Inherits[0]]; ok && sup.Kind == "class" {
				ti.Super = sup
			}
		}
		if ti.Kind == "enum" {
			raw := ""
			if len(d.Inherits) > 0 {
				switch d.Inherits[0] {
				case "String", "Int", "Double", "Character", "Float":
					raw = d.Inherits[0]
				}
			}
			ti.RawType = raw
			next := 0
			for _, c := range ti.Cases {
				if c.Raw != nil {
					v := ip.eval(c.Raw, ip.globals)
					ti.RawVals[c.Name] = v
					if iv, ok := v.(int); ok {
						next = iv + 1
					}
				} else if raw == "String" {
					ti.RawVals[c.Name] = c.Name
				} else if raw == "Int" {
					ti.RawVals[c.Name] = next
					next++
				}
			}
		}
		for _, a := range d.Attrs {
			if a.Name == "main" {
				ip.mainApp = ti
			}
			if a.Name == "Model" {
				if ti.Kind != "class" {
					return &SwiftError{a.Pos, "'@Model' can only be applied to a class (write '@Model final class " + ti.Name + "')"}
				}
				ti.IsModel = true
				for _, protocol := range []string{"PersistentModel", "Identifiable", "Hashable", "Observable"} {
					ti.Conforms[protocol] = true
				}
			}
		}
		if ti.Conforms["PreviewProvider"] {
			ip.previews = append(ip.previews, &PreviewInfo{Name: ti.Name, File: d.Pos.File, Kind: "provider", T: ti, Pos: d.Pos})
		}
	}
	return nil
}

func (ip *Interp) isKnownProtocol(name string) bool {
	if t, ok := ip.types[name]; ok && t.Kind == "protocol" {
		return true
	}
	switch name {
	case "View", "Identifiable", "Hashable", "Equatable", "Comparable", "Codable", "ObservableObject",
		"ShapeStyle", "Shape", "ButtonStyle", "ViewModifier", "Error", "CustomStringConvertible", "Collection",
		"Sequence", "PrimitiveButtonStyle", "LabelStyle", "ToggleStyle", "TextFieldStyle", "App", "Scene",
		"Sendable", "CaseIterable", "RawRepresentable", "BinaryInteger", "FloatingPoint", "Numeric", "StringProtocol",
		"RandomAccessCollection", "MutableCollection", "LocalizedError", "Decodable", "Encodable", "GroupBoxStyle",
		"ProgressViewStyle", "PickerStyle", "ListStyle", "EnvironmentKey", "PreferenceKey", "Animatable", "BinaryFloatingPoint",
		"AnyObject", "Any":
		return true
	}
	return false
}

func (ip *Interp) fullName(d *TypeDecl) string {
	if n, ok := ip.declNames[d]; ok {
		return n
	}
	return d.Name
}

func (ip *Interp) registerType(d *TypeDecl, outer string) []*TypeDecl {
	name := d.Name
	full := name
	if outer != "" {
		full = outer + "." + name
	}
	if old, ok := ip.types[full]; ok && old.Decl == d {
		return nil
	}
	if old, ok := ip.types[full]; ok && old.Decl != nil {
		panic(&SwiftError{d.Pos, fmt.Sprintf("invalid redeclaration of '%s'", name)})
	}
	ti := newTypeInfo(name, d.Kind)
	ti.Decl = d
	ti.Outer = outer
	ip.types[full] = ti
	if outer != "" {
		if _, exists := ip.types[name]; !exists {
			ip.types[name] = ti
		}
	}
	ip.declNames[d] = full
	out := []*TypeDecl{d}
	for _, m := range d.Members {
		if nd, ok := m.(*TypeDecl); ok && nd.Kind != "extension" {
			out = append(out, ip.registerType(nd, full)...)
		}
	}
	return out
}

func (ip *Interp) defineGlobalVar(d *VarDecl) {
	name := d.Name
	if d.Computed {
		vd := d
		ip.globals.vars[name] = &Slot{Get: func() Value {
			return ip.runGetter(vd.Get, ip.globals, nil, nil, vd.Type)
		}}
		return
	}
	slot := &Slot{Let: d.Let, T: d.Type}
	done := false
	vd := d
	slot.Get = func() Value {
		if !done {
			done = true
			if vd.Init != nil {
				slot.V = copyValue(ip.evalTyped(vd.Init, ip.globals, vd.Type))
			}
		}
		return slot.V
	}
	slot.Set = func(v Value) {
		done = true
		slot.V = copyValue(ip.coerce(v, vd.Type, nil))
	}
	ip.globals.vars[name] = slot
}

func (ip *Interp) addMembers(ti *TypeInfo, members []Stmt) {
	for _, m := range members {
		switch d := m.(type) {
		case *VarDecl:
			ip.addVar(ti, d)
		case *BlockStmt:
			for _, b := range d.Body {
				if vd, ok := b.(*VarDecl); ok {
					ip.addVar(ti, vd)
				}
			}
		case *FuncDecl:
			if d.IsInit {
				ti.Inits = append(ti.Inits, d)
				if ti.Decl != nil && containsStmt(ti.Decl.Members, d) {
					ti.HasBodyInit = true
				}
			} else if d.IsSubscript {
				ti.Subscripts = append(ti.Subscripts, d)
			} else if d.Mods["static"] || d.Mods["class"] {
				ti.StaticMethods[d.Name] = append(ti.StaticMethods[d.Name], d)
			} else {
				ti.Methods[d.Name] = append(ti.Methods[d.Name], d)
			}
		case *EnumCaseDecl:
			for _, c := range d.Cases {
				ti.Cases = append(ti.Cases, c)
				ti.CaseMap[c.Name] = c
			}
		case *TypeAliasDecl:
			ti.Aliases[d.Name] = d.T
			ip.aliases[d.Name] = d.T
		}
	}
}

func containsStmt(list []Stmt, s Stmt) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func (ip *Interp) addVar(ti *TypeInfo, d *VarDecl) {
	if d.Mods["static"] || d.Mods["class"] {
		ti.StaticFields[d.Name] = d
		return
	}
	if d.Computed {
		ti.Computed[d.Name] = d
		return
	}
	if ti.Kind == "protocol" {
		return
	}
	fi := &FieldInfo{Name: d.Name, Decl: d, Type: d.Type, Let: d.Let, Init: d.Init}
	for _, a := range d.Attrs {
		if knownWrappers[a.Name] {
			fi.Wrapper = a.Name
			fi.WrapArgs = a.Args
		}
	}
	if d.Name == "" {
		return
	}
	if _, dup := ti.FieldMap[d.Name]; dup {
		panic(&SwiftError{d.Pos, fmt.Sprintf("invalid redeclaration of '%s'", d.Name)})
	}
	ti.Fields = append(ti.Fields, fi)
	ti.FieldMap[d.Name] = fi
}

// ---------- method lookup ----------

func (ip *Interp) findMethods(ti *TypeInfo, name string) []*FuncDecl {
	for c := ti; c != nil; c = c.Super {
		if ms, ok := c.Methods[name]; ok {
			return ms
		}
	}
	for c := ti; c != nil; c = c.Super {
		for proto := range c.Conforms {
			if et, ok := ip.ext[proto]; ok {
				if ms, ok := et.Methods[name]; ok {
					return ms
				}
			}
			if pt, ok := ip.types[proto]; ok && pt.Kind == "protocol" {
				for pp := range pt.Conforms {
					if et, ok := ip.ext[pp]; ok {
						if ms, ok := et.Methods[name]; ok {
							return ms
						}
					}
				}
			}
		}
	}
	return nil
}

func (ip *Interp) findComputed(ti *TypeInfo, name string) *VarDecl {
	for c := ti; c != nil; c = c.Super {
		if vd, ok := c.Computed[name]; ok {
			return vd
		}
	}
	for c := ti; c != nil; c = c.Super {
		for proto := range c.Conforms {
			if et, ok := ip.ext[proto]; ok {
				if vd, ok := et.Computed[name]; ok {
					return vd
				}
			}
		}
	}
	return nil
}

func (ip *Interp) findStaticMethods(ti *TypeInfo, name string) []*FuncDecl {
	for c := ti; c != nil; c = c.Super {
		if ms, ok := c.StaticMethods[name]; ok {
			return ms
		}
	}
	for proto := range ti.Conforms {
		if et, ok := ip.ext[proto]; ok {
			if ms, ok := et.StaticMethods[name]; ok {
				return ms
			}
		}
	}
	return nil
}

func (ip *Interp) staticField(ti *TypeInfo, name string) (*Slot, bool) {
	for c := ti; c != nil; c = c.Super {
		if s, ok := c.StaticVals[name]; ok {
			return s, true
		}
		if vd, ok := c.StaticFields[name]; ok {
			owner := c
			if vd.Computed {
				d := vd
				s := &Slot{Get: func() Value {
					env := newEnv(ip.globals)
					env.staticT = owner
					return ip.runGetter(d.Get, env, nil, nil, d.Type)
				}}
				return s, true
			}
			env := newEnv(ip.globals)
			env.staticT = owner
			s := &Slot{Let: vd.Let, T: vd.Type}
			owner.StaticVals[name] = s
			if vd.Init != nil {
				s.V = copyValue(ip.evalTyped(vd.Init, env, vd.Type))
			}
			return s, true
		}
	}
	return nil, false
}

// ---------- instantiation ----------

func (ip *Interp) newInstance(ti *TypeInfo) Value {
	fields := map[string]Value{}
	for _, f := range ti.allFields() {
		var v Value
		if f.Init != nil && f.Decl != nil && f.Decl.Mods["lazy"] {
			v = &lazyFieldV{}
		} else if f.Init != nil {
			env := newEnv(ip.globals)
			env.staticT = ti
			v = copyValue(ip.evalTyped(f.Init, env, f.Type))
		}
		switch f.Wrapper {
		case "State", "StateObject", "FocusState", "GestureState", "SceneStorage":
			v = &BoxV{V: v, Kind: f.Wrapper}
		case "AppStorage":
			key := ""
			if len(f.WrapArgs) > 0 {
				key = describe(ip.eval(f.WrapArgs[0].X, ip.globals))
			}
			if ip.store != nil {
				if b, ok := ip.store.appStorage[key]; ok {
					v = b
				} else {
					b := &BoxV{V: v, Kind: "AppStorage", Key: key}
					ip.store.appStorage[key] = b
					v = b
				}
			} else {
				v = &BoxV{V: v, Kind: "AppStorage"}
			}
		case "Namespace":
			v = sym("namespace")
		}
		fields[f.Name] = v
	}
	if ti.Kind == "class" {
		ip.objID++
		return &ObjectV{T: ti, F: fields, ID: ip.objID}
	}
	return &StructV{T: ti, F: fields}
}

func isWrapperExcluded(w string) bool {
	switch w {
	case "Environment", "EnvironmentObject", "FocusState", "Namespace", "AppStorage", "SceneStorage", "GestureState", "AccessibilityFocusState", "FocusedValue", "Query":
		return true
	}
	return false
}

func (ip *Interp) memberwiseParams(ti *TypeInfo) []*Param {
	var ps []*Param
	for _, f := range ti.Fields {
		if isWrapperExcluded(f.Wrapper) {
			continue
		}
		if f.Let && f.Init != nil {
			continue
		}
		p := &Param{Label: f.Name, Name: f.Name, Type: f.Type}
		if f.Init != nil {
			p.Default = f.Init
		} else if f.Type != nil && f.Type.Opt && !f.Let {
			p.Default = &NilLit{}
		}
		if f.Type != nil && f.Type.Kind == tyFunc {
			p.Attrs = append(p.Attrs, &Attr{Name: "escaping"})
		}
		ps = append(ps, p)
	}
	return ps
}

func (ip *Interp) instantiate(ti *TypeInfo, args []ArgV, pos Pos) Value {
	switch ti.Kind {
	case "protocol":
		ip.fail(pos, "'any %s' cannot be constructed because it has no accessible initializers", ti.Name)
	case "enum":
		if len(args) == 1 && args[0].Label == "rawValue" && len(ti.Inits) == 0 {
			for _, c := range ti.Cases {
				if valueEquals(ti.RawVals[c.Name], args[0].V) {
					return &EnumV{T: ti, Case: c.Name}
				}
			}
			return nil
		}
	}
	var inits []*FuncDecl
	for c := ti; c != nil; c = c.Super {
		inits = append(inits, c.Inits...)
		if len(c.Inits) > 0 {
			break
		}
	}
	var firstErr string
	for _, d := range inits {
		vals, set, errMsg := ip.matchArgs(d.Params, args)
		if errMsg != "" {
			if firstErr == "" {
				firstErr = errMsg
			}
			continue
		}
		var inst Value
		if ti.Kind == "enum" {
			inst = &EnumV{T: ti}
		} else {
			inst = ip.newInstance(ti)
		}
		return ip.runInit(ti, d, inst, vals, set, pos)
	}
	if ti.Kind == "struct" && !ti.HasBodyInit {
		params := ip.memberwiseParams(ti)
		vals, set, errMsg := ip.matchArgs(params, args)
		if errMsg == "" {
			inst := ip.newInstance(ti).(*StructV)
			for i, p := range params {
				var v Value
				if set[i] {
					v = ip.coerce(vals[i], p.Type, nil)
				} else {
					continue
				}
				f := ti.FieldMap[p.Name]
				switch f.Wrapper {
				case "State", "StateObject":
					inst.F[p.Name] = &BoxV{V: copyValue(v), Kind: f.Wrapper}
				case "Binding":
					v = constBinding(v)
					if _, ok := v.(*BindingV); !ok {
						ip.fail(pos, "cannot convert value of type '%s' to expected argument type 'Binding<%s>' (use $%s)", typeName(v), typeName(v), p.Name)
					}
					inst.F[p.Name] = v
				default:
					inst.F[p.Name] = copyValue(v)
				}
			}
			return inst
		}
		if firstErr == "" {
			firstErr = errMsg
		}
	}
	if ti.Kind == "class" && len(inits) == 0 {
		if len(args) == 0 {
			return ip.newInstance(ti)
		}
		ip.fail(pos, "argument passed to call that takes no arguments")
	}
	if len(inits) == 0 && ti.Kind == "enum" {
		ip.fail(pos, "'%s' cannot be constructed because it has no accessible initializers", ti.Name)
	}
	if firstErr == "" {
		firstErr = "no matching initializer for '" + ti.Name + "'"
	}
	ip.fail(pos, "%s", firstErr)
	return nil
}

func (ip *Interp) runInit(ti *TypeInfo, d *FuncDecl, inst Value, vals []Value, set []bool, pos Pos) Value {
	env := newEnv(ip.globals)
	env.self = inst
	env.selfT = ti
	env.mutableSelf = true
	env.inInit = true
	env.staticT = ti
	ip.bindParams(env, d.Params, vals, set, ip.globals)
	fc := &FnCtx{isInit: true}
	ip.execBody(d.Body, env, fc)
	if fc.returnedNil {
		return nil
	}
	if fc.replacedSelf != nil {
		return fc.replacedSelf
	}
	return env.self
}

func (ip *Interp) matchArgs(params []*Param, args []ArgV) ([]Value, []bool, string) {
	vals := make([]Value, len(params))
	set := make([]bool, len(params))
	ai := 0
	for pi, prm := range params {
		if prm.Variadic {
			var items []Value
			for ai < len(args) {
				a := args[ai]
				if (len(items) == 0 && a.Label == prm.Label) || (len(items) > 0 && a.Label == "" && !a.Trailing) {
					items = append(items, a.V)
					ai++
					continue
				}
				break
			}
			vals[pi] = &ArrayV{E: items}
			set[pi] = true
			continue
		}
		if ai < len(args) {
			a := args[ai]
			if a.Trailing && a.Label == "" {
				if isFuncParam(prm) || prm.Default == nil {
					vals[pi] = a.V
					set[pi] = true
					ai++
					continue
				}
				continue
			}
			if a.Label == prm.Label {
				vals[pi] = a.V
				set[pi] = true
				ai++
				continue
			}
		}
		if prm.Default != nil {
			continue
		}
		if ai < len(args) {
			a := args[ai]
			for k := ai + 1; k < len(args); k++ {
				if args[k].Label == prm.Label {
					return nil, nil, fmt.Sprintf("argument '%s' must precede argument '%s'", prm.Label, labelOr(a.Label))
				}
			}
			if a.Trailing {
				return nil, nil, fmt.Sprintf("missing argument for parameter '%s' in call", labelOr(prm.Label))
			}
			return nil, nil, fmt.Sprintf("incorrect argument label in call (have '%s:', expected '%s:')", labelOr(a.Label), labelOr(prm.Label))
		}
		return nil, nil, fmt.Sprintf("missing argument for parameter '%s' in call", labelOr(prm.Label))
	}
	if ai < len(args) {
		a := args[ai]
		if a.Trailing {
			return nil, nil, "extra trailing closure passed in call"
		}
		if a.Label == "" {
			return nil, nil, "extra argument in call"
		}
		return nil, nil, fmt.Sprintf("extra argument '%s' in call", a.Label)
	}
	return vals, set, ""
}

func labelOr(l string) string {
	if l == "" {
		return "_"
	}
	return l
}

func isFuncParam(p *Param) bool {
	if p.Type != nil && p.Type.Kind == tyFunc {
		return true
	}
	for _, a := range p.Attrs {
		if a.Name == "escaping" || a.Name == "ViewBuilder" || a.Name == "autoclosure" {
			return true
		}
	}
	if p.Type != nil && (p.Type.Name == "Content" || strings.HasSuffix(p.Type.Name, "Content") || p.Type.Name == "Label") {
		return true
	}
	return false
}

func (ip *Interp) bindParams(env *Env, params []*Param, vals []Value, set []bool, defEnv *Env) {
	for i, prm := range params {
		var v Value
		if set[i] {
			v = vals[i]
		} else if prm.Default != nil {
			v = ip.evalTyped(prm.Default, defEnv, prm.Type)
		}
		if prm.InOut {
			if rv, ok := v.(*RefV); ok {
				r := rv.R
				env.vars[prm.Name] = &Slot{Get: r.Get, Set: r.Set}
				continue
			}
			ip.fail(Pos{}, "passing value of type '%s' to an inout parameter requires explicit '&'", typeName(v))
		}
		v = ip.coerce(v, prm.Type, nil)
		if fv, ok := v.(*FuncV); ok && hasAttr(prm.Attrs, "ViewBuilder") {
			nf := *fv
			nf.Builder = true
			v = &nf
		}
		env.vars[prm.Name] = &Slot{V: copyValue(v), Let: true, T: prm.Type}
	}
}

func hasAttr(attrs []*Attr, name string) bool {
	for _, a := range attrs {
		if a.Name == name {
			return true
		}
	}
	return false
}
