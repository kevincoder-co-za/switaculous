package switaculous

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

type RNode struct {
	T  string                 `json:"t"`
	ID string                 `json:"id,omitempty"`
	P  map[string]interface{} `json:"p,omitempty"`
	C  []*RNode               `json:"c,omitempty"`
	M  []RMod                 `json:"m,omitempty"`
}

type RMod struct {
	N string                 `json:"n"`
	A map[string]interface{} `json:"a,omitempty"`
}

func (n *RNode) set(k string, v interface{}) {
	if n.P == nil {
		n.P = map[string]interface{}{}
	}
	n.P[k] = v
}

type NavEntry struct {
	ID       int
	LinkPath string
	Dest     Value
	Value    Value
	IsValue  bool
}

type NavState struct {
	Entries []*NavEntry
	Next    int
}

type destReg struct {
	typ string
	fn  Value
}

type presentedDest struct {
	b    *BindingV
	fn   Value
	item bool
}

type NavCtx struct {
	key       string
	state     *NavState
	pathB     *BindingV
	links     map[string]Value
	dests     []destReg
	presented []presentedDest
}

type PageCtx struct {
	title    string
	subtitle string
	titleSet bool
	mode     string
	lead     []*RNode
	trail    []*RNode
	prin     []*RNode
	bottom   []*RNode
	search   map[string]interface{}
	hideBar  bool
	hideBack bool
	barBg    interface{}
}

type SheetCtx struct {
	detents []interface{}
	bg      interface{}
}

type focusInfo struct {
	b      *BindingV
	equals Value
	has    bool
}

type REnv struct {
	buttonStyle  Value
	styles       map[string]string
	labelsHidden bool
	envObjects   map[string]Value
	envVals      map[string]Value
	nav          *NavCtx
	page         *PageCtx
	sheet        *SheetCtx
	hidden       bool
	inList       bool
	inForm       bool
	inMenu       bool
	inToolbar    bool
	inAlert      bool
	disabled     bool
	submit       Value
	focus        *focusInfo
	ctx          string
	splitSidebar bool
}

func (e *REnv) clone() *REnv { n := *e; return &n }

func (e *REnv) withStyle(k, v string) *REnv {
	n := e.clone()
	n.styles = map[string]string{}
	for a, b := range e.styles {
		n.styles[a] = b
	}
	n.styles[k] = v
	return n
}

func (e *REnv) withEnvVal(k string, v Value) *REnv {
	n := e.clone()
	n.envVals = map[string]Value{}
	for a, b := range e.envVals {
		n.envVals[a] = b
	}
	n.envVals[k] = v
	return n
}

type changeReg struct {
	key     string
	val     Value
	fn      Value
	initial bool
}

type recvReg struct {
	interval float64
	fn       Value
}

type Render struct {
	ip              *Interp
	store           *Store
	handlers        map[string]func(interface{})
	overlays        []*RNode
	appear          map[string]Value
	appearOrder     []string
	disappear       map[string]Value
	tasks           map[string]Value
	changes         []changeReg
	receivers       map[string]*recvReg
	touched         map[string]bool
	keep            []string
	tagVals         map[string]Value
	preferredScheme string
	hideTabBar      bool
	geoKeys         map[string]bool
	navTouched      map[string]bool
	imageBlobs      map[string]imageBlob
}

func (r *Render) on(id string, f func(interface{})) string {
	r.handlers[id] = f
	return id
}

func posKey(p Pos) string { return fmt.Sprintf("%d_%d", p.Line, p.Col) }

func (r *Render) fail(pos Pos, f string, a ...interface{}) { r.ip.fail(pos, f, a...) }

// ---------- helpers ----------

func (v *ViewV) arg(label string) (Value, bool) {
	for _, a := range v.Args {
		if a.Label == label && !(label == "" && a.Trailing) {
			return a.V, true
		}
	}
	return nil, false
}

func (v *ViewV) closure(label string) Value {
	for _, a := range v.Args {
		if a.Label == label {
			return a.V
		}
	}
	if label == "content" {
		for _, a := range v.Args {
			if a.Trailing && a.Label == "" {
				return a.V
			}
		}
		for i := len(v.Args) - 1; i >= 0; i-- {
			if _, ok := v.Args[i].V.(*FuncV); ok && v.Args[i].Label == "" {
				return v.Args[i].V
			}
		}
	}
	return nil
}

func (v *ViewV) firstString() (string, bool) {
	for _, a := range v.Args {
		if a.Label == "" && !a.Trailing {
			if s, ok := a.V.(string); ok {
				return s, true
			}
		}
	}
	return "", false
}

func (r *Render) callBuilder(f Value, args ...Value) Value {
	switch x := f.(type) {
	case nil:
		return nil
	case *FuncV, *NativeV, *OverloadV:
		av := make([]ArgV, len(args))
		for i, a := range args {
			av[i] = ArgV{V: a}
		}
		return r.ip.callValue(x, av, Pos{}, nil)
	}
	return f
}

func symName(v Value) string {
	switch x := v.(type) {
	case *SymV:
		return x.Name()
	case string:
		return x
	case *EnumV:
		return x.Case
	}
	return ""
}

func plainText(nodes []*RNode) string {
	var sb strings.Builder
	for _, n := range nodes {
		switch n.T {
		case "text":
			sb.WriteString(fmt.Sprint(n.P["s"]))
		default:
			if n.T == "image" {
				continue
			}
			sb.WriteString(plainText(n.C))
		}
	}
	return sb.String()
}

func firstSysImage(nodes []*RNode) string {
	for _, n := range nodes {
		if n.T == "image" {
			if s, ok := n.P["sys"].(string); ok {
				return s
			}
		}
		if s := firstSysImage(n.C); s != "" {
			return s
		}
	}
	return ""
}

func (r *Render) textNode(s string, path string) *RNode {
	return &RNode{T: "text", ID: path, P: map[string]interface{}{"s": s}}
}

// A Label placed in a toolbar shows only its icon unless a labelStyle says otherwise.
func labelStyle(env *REnv) string {
	style := env.styles["label"]
	if style == "" && env.inToolbar && !env.inMenu {
		return "iconOnly"
	}
	return style
}

func (r *Render) labelFrom(v *ViewV, path string, env *REnv, closureLabel string) []*RNode {
	if s, ok := v.firstString(); ok {
		n := r.textNode(s, path+"/l")
		icon := map[string]interface{}{}
		if sys, ok := v.arg("systemImage"); ok {
			icon["sys"] = describe(sys)
		} else if img, ok := v.arg("image"); ok {
			icon["asset"] = describe(img)
		} else {
			return []*RNode{n}
		}
		return []*RNode{{T: "label", ID: path + "/lb", P: map[string]interface{}{"style": labelStyle(env)}, C: []*RNode{
			{T: "lbicon", C: []*RNode{{T: "image", P: icon}}}, {T: "lbtitle", C: []*RNode{n}}}}}
	}
	for _, a := range v.Args {
		if a.Label == "" && !a.Trailing {
			if isTextValue(a.V) {
				return r.resolve(a.V, path+"/l", env)
			}
		}
	}
	if f := v.closure(closureLabel); f != nil {
		return r.resolve(r.callBuilder(f), path+"/l", env)
	}
	return nil
}

// ---------- entry ----------

func (r *Render) resolve(v Value, path string, env *REnv) []*RNode {
	switch x := v.(type) {
	case nil, *VoidV:
		return nil
	case *ViewV:
		return r.builtin(x, path, env)
	case *ModV:
		return r.modifier(x, path, env)
	case *ColorV:
		return []*RNode{{T: "color", ID: path, P: map[string]interface{}{"c": r.paint(x)}}}
	case *StructV:
		if x.T.conformsTo("UIViewControllerRepresentable") || x.T.conformsTo("UIViewRepresentable") {
			return r.representable(x, path, env)
		}
		if x.T.IsView() {
			return r.userView(x, path, env)
		}
		if x.T.conformsTo("Shape") {
			return []*RNode{{T: "shape", ID: path, P: map[string]interface{}{"k": "rect"}}}
		}
	case *SymV:
		if c := r.ip.colorFromSym(x); c != nil {
			return r.resolve(c, path, env)
		}
	}
	r.fail(Pos{}, "type '%s' cannot conform to 'View' (only views can appear in a view builder) at %s", typeName(v), path)
	return nil
}

func stateInitHash(v Value) string {
	switch x := v.(type) {
	case *ObjectV:
		return "obj:" + x.T.Name
	case nil:
		return "nil"
	}
	return typeName(v) + ":" + hashKey(v)
}

func (r *Render) prepareStruct(sv *StructV, p string, env *REnv) *StructV {
	ti := sv.T
	inst := &StructV{T: ti, F: make(map[string]Value, len(sv.F))}
	for k, v := range sv.F {
		inst.F[k] = v
	}
	for _, f := range ti.allFields() {
		switch f.Wrapper {
		case "State", "StateObject", "FocusState", "GestureState", "SceneStorage":
			key := p + "|" + f.Name
			cur, _ := inst.F[f.Name].(*BoxV)
			var initV Value
			if cur != nil {
				initV = cur.V
			}
			h := stateInitHash(initV)
			box := r.store.state[key]
			if box == nil || (box.Gen != r.ip.gen && box.InitHash != h) || (box.V != nil && initV != nil && typeName(box.V) != typeName(initV)) {
				box = &BoxV{V: copyValue(initV), Kind: f.Wrapper}
				r.store.state[key] = box
			}
			box.Gen = r.ip.gen
			box.InitHash = h
			inst.F[f.Name] = box
			r.touched[key] = true
		case "Environment":
			inst.F[f.Name] = r.envValue(f, env, p)
		case "Query":
			inst.F[f.Name] = r.queryResults(f, inst.F[f.Name], env)
		case "EnvironmentObject":
			if f.Type != nil {
				inst.F[f.Name] = env.envObjects[f.Type.Name]
			}
		}
	}
	return inst
}

// `Image(.logoBadge)` names an image set through Xcode's generated symbol;
// `Image("LogoBadge")` names it directly.
func (r *Render) imageAssetName(v Value) string {
	symbol, ok := v.(*SymV)
	if !ok || r.ip.rt == nil {
		return describe(v)
	}
	for asset := range r.ip.rt.assets {
		if assetSymbolName(asset) == symbol.Name() {
			return asset
		}
	}
	return symbol.Name()
}

func (r *Render) userView(sv *StructV, path string, env *REnv) []*RNode {
	ti := sv.T
	p := path + ":" + ti.Name
	inst := r.prepareStruct(sv, p, env)
	vd := r.ip.findComputed(ti, "body")
	if vd == nil {
		r.fail(Pos{}, "type '%s' does not conform to protocol 'View' (add 'var body: some View { ... }')", ti.Name)
	}
	body := r.ip.runGetter(vd.Get, r.ip.globals, inst, ti, nil)
	return r.resolve(body, p, env)
}

func (r *Render) envValue(f *FieldInfo, env *REnv, path string) Value {
	if len(f.WrapArgs) == 0 {
		return nil
	}
	switch k := f.WrapArgs[0].X.(type) {
	case *KeyPathExpr:
		name := ""
		if len(k.Path) > 0 {
			name = k.Path[0]
		}
		if v, ok := env.envVals[name]; ok {
			return v
		}
		switch name {
		case "dismiss":
			return &NativeV{Name: "dismiss", Fn: func(c *CallCtx) Value { return Void }}
		case "presentationMode":
			pm := newObj("PresentationMode")
			inner := newObj("PresentationModeValue")
			d, _ := env.envVals["dismiss"]
			if d == nil {
				d = &NativeV{Name: "dismiss", Fn: func(c *CallCtx) Value { return Void }}
			}
			inner.Props["dismiss"] = d
			inner.Props["isPresented"] = env.envVals["dismiss"] != nil
			pm.Props["wrappedValue"] = inner
			return pm
		case "colorScheme":
			if r.store.dark {
				return sym("dark")
			}
			return sym("light")
		case "horizontalSizeClass":
			return sym("compact")
		case "verticalSizeClass":
			return sym("regular")
		case "isEnabled":
			return !env.disabled
		case "scenePhase":
			return sym("active")
		case "openURL":
			return &NativeV{Name: "openURL", Fn: func(c *CallCtx) Value {
				r.ip.printFn("openURL: " + describe(argAt(c.Args, 0)))
				return Void
			}}
		case "isPresented":
			return env.envVals["dismiss"] != nil
		case "isSearching":
			return false
		case "editMode":
			return nil
		case "locale", "calendar", "timeZone":
			return newObj(name)
		case "dynamicTypeSize":
			return sym("large")
		case "displayScale":
			return 3.0
		case "modelContext":
			r.fail(f.Decl.Pos, "No model container found for @Environment(\\.modelContext). Add .modelContainer(for: YourModel.self) to the WindowGroup in your App, or to the view in #Preview")
		case "managedObjectContext":
			r.fail(f.Decl.Pos, "Core Data is not supported by the preview; use SwiftData (@Model and @Query) instead")
		}
		return nil
	default:
		tv := r.ip.eval(f.WrapArgs[0].X, r.ip.globals)
		if t, ok := tv.(*TypeV); ok {
			if v, ok := env.envObjects[t.Name]; ok {
				return v
			}
			r.fail(f.Decl.Pos, "No Observable object of type %s found. Did you forget .environment(%s) on an ancestor view?", t.Name, strings.ToLower(t.Name[:1])+t.Name[1:])
		}
	}
	return nil
}

// ---------- builtin views ----------

func (r *Render) builtin(v *ViewV, path string, env *REnv) []*RNode {
	ip := r.ip
	switch v.Kind {
	case "Tuple":
		var out []*RNode
		for i, k := range v.Kids {
			key := strconv.Itoa(i)
			if i < len(v.Keys) {
				key = v.Keys[i]
			}
			out = append(out, r.resolve(k, path+"/"+key, env)...)
		}
		return out
	case "Group", "WindowGroup", "ControlGroupContent", "ScrollViewReaderContent", "Settings", "GlassEffectContainer":
		return r.resolve(r.callBuilder(v.closure("content")), path+"/c", env)
	case "Prerendered":
		if n, ok := v.Extra["nodes"].([]*RNode); ok {
			return n
		}
		return nil
	case "EmptyView":
		return nil
	case "Text":
		return []*RNode{r.textOf(v, path)}
	case "TextConcat":
		var segs []*RNode
		for i, k := range v.Kids {
			segs = append(segs, r.resolve(k, path+"/"+strconv.Itoa(i), env)...)
		}
		return []*RNode{{T: "textcat", ID: path, C: segs}}
	case "Image":
		n := &RNode{T: "image", ID: path, P: map[string]interface{}{}}
		if s, ok := v.arg("systemName"); ok {
			n.P["sys"] = describe(s)
		} else if s, ok := v.arg("decorative"); ok {
			n.P["asset"] = describe(s)
		} else if img, ok := v.arg("uiImage"); ok {
			r.uiImageNode(n, img)
		} else if u, ok := v.Extra["url"]; ok {
			n.P["url"] = describe(u)
		} else if s, ok := v.arg(""); ok {
			n.P["asset"] = r.imageAssetName(s)
		}
		return []*RNode{n}
	case "Label":
		var icon, title []*RNode
		if s, ok := v.firstString(); ok {
			title = []*RNode{r.textNode(s, path+"/t")}
		} else if f := v.closure("title"); f != nil {
			title = r.resolve(r.callBuilder(f), path+"/t", env)
		} else if f := v.closure("content"); f != nil {
			title = r.resolve(r.callBuilder(f), path+"/t", env)
		}
		if s, ok := v.arg("systemImage"); ok {
			icon = []*RNode{{T: "image", ID: path + "/i", P: map[string]interface{}{"sys": describe(s)}}}
		} else if s, ok := v.arg("image"); ok {
			icon = []*RNode{{T: "image", ID: path + "/i", P: map[string]interface{}{"asset": describe(s)}}}
		} else if f := v.closure("icon"); f != nil {
			icon = r.resolve(r.callBuilder(f), path+"/i", env)
		}
		n := &RNode{T: "label", ID: path, P: map[string]interface{}{"style": labelStyle(env)}}
		n.C = append(n.C, &RNode{T: "lbicon", C: icon}, &RNode{T: "lbtitle", C: title})
		return []*RNode{n}
	case "VStack", "HStack", "ZStack", "LazyVStack", "LazyHStack":
		t := strings.ToLower(strings.TrimPrefix(v.Kind, "Lazy"))
		n := &RNode{T: t, ID: path, P: map[string]interface{}{}}
		if al, ok := v.arg("alignment"); ok {
			n.P["al"] = symName(al)
		}
		if sp, ok := v.arg("spacing"); ok && sp != nil {
			f, _ := toFloat(sp)
			n.P["sp"] = f
		}
		n.C = r.resolve(r.callBuilder(v.closure("content")), path+"/c", env)
		return []*RNode{n}
	case "Layout":
		n := &RNode{T: "hstack", ID: path, P: map[string]interface{}{"wrap": true, "al": "top"}}
		if layout, ok := v.Extra["layout"].(*StructV); ok {
			if sp, ok := layout.F["spacing"]; ok && sp != nil {
				f, _ := toFloat(sp)
				n.P["sp"] = f
			}
		}
		n.C = r.resolve(r.callBuilder(v.closure("content")), path+"/c", env)
		return []*RNode{n}
	case "Spacer":
		n := &RNode{T: "spacer", ID: path}
		if m, ok := v.arg("minLength"); ok {
			f, _ := toFloat(m)
			n.set("min", f)
		}
		return []*RNode{n}
	case "Divider":
		return []*RNode{{T: "divider", ID: path}}
	case "ScrollView":
		ax := "v"
		if a, ok := v.arg(""); ok {
			switch x := a.(type) {
			case *SymV:
				if x.Name() == "horizontal" {
					ax = "h"
				}
			case *ArrayV:
				ax = "vh"
				if len(x.E) == 1 && symName(x.E[0]) == "horizontal" {
					ax = "h"
				}
			}
		}
		n := &RNode{T: "scroll", ID: path, P: map[string]interface{}{"ax": ax}}
		if s, ok := v.arg("showsIndicators"); ok && s == false {
			n.P["noind"] = true
		}
		n.C = r.resolve(r.callBuilder(v.closure("content")), path+"/c", env)
		return []*RNode{n}
	case "ScrollViewReader":
		proxy := newObj("ScrollViewProxy")
		proxy.Props["scrollTo"] = &NativeV{Name: "scrollTo", Fn: func(c *CallCtx) Value {
			r.store.scrollTo = hashKey(argAt(c.Args, 0))
			return Void
		}}
		return r.resolve(r.callBuilder(v.closure("content"), proxy), path+"/c", env)
	case "List", "Form":
		return []*RNode{r.list(v, path, env)}
	case "Section":
		return []*RNode{r.section(v, path, env)}
	case "ForEach":
		return r.forEach(v, path, env)
	case "Button":
		return []*RNode{r.button(v, path, env)}
	case "PhotosPicker":
		return []*RNode{r.photosPicker(v, path, env)}
	case "Toggle":
		return []*RNode{r.toggle(v, path, env)}
	case "TextField", "SecureField", "TextEditor":
		return []*RNode{r.textField(v, path, env)}
	case "Slider":
		return []*RNode{r.slider(v, path, env)}
	case "Stepper":
		return []*RNode{r.stepper(v, path, env)}
	case "Picker":
		return []*RNode{r.picker(v, path, env)}
	case "DatePicker":
		return []*RNode{r.datePicker(v, path, env)}
	case "ColorPicker":
		b, _ := v.arg("selection")
		bv, _ := b.(*BindingV)
		n := &RNode{T: "colorpicker", ID: path, P: map[string]interface{}{}}
		n.C = r.labelFrom(v, path, env, "label")
		if bv != nil {
			if c, ok := bv.Get().(*ColorV); ok {
				n.P["v"] = r.colorHex(c)
			}
			n.P["id"] = r.on(path+"#color", func(x interface{}) {
				s, _ := x.(string)
				var rr, gg, bb int
				fmt.Sscanf(strings.TrimPrefix(s, "#"), "%02x%02x%02x", &rr, &gg, &bb)
				bv.Set(&ColorV{R: float64(rr) / 255, G: float64(gg) / 255, B: float64(bb) / 255, A: 1, Op: 1})
			})
		}
		return []*RNode{n}
	case "ProgressView", "Gauge":
		n := &RNode{T: "progress", ID: path, P: map[string]interface{}{}}
		if val, ok := v.arg("value"); ok && val != nil {
			f, _ := toFloat(val)
			total := 1.0
			if t, ok := v.arg("total"); ok {
				total, _ = toFloat(t)
			}
			if rg, ok := v.arg("in"); ok {
				if rv, ok := rg.(*RangeV); ok {
					lo, _ := toFloat(rv.Lo)
					hi, _ := toFloat(rv.Hi)
					f = f - lo
					total = hi - lo
				}
			}
			if total == 0 {
				total = 1
			}
			n.P["v"] = math.Max(0, math.Min(1, f/total))
		}
		if v.Kind == "Gauge" {
			n.P["gauge"] = true
		}
		n.C = r.labelFrom(v, path, env, "label")
		if f := v.closure("currentValueLabel"); f != nil {
			n.P["cur"] = r.resolve(r.callBuilder(f), path+"/cur", env)
		}
		if len(n.C) == 0 {
			if f := v.closure("content"); f != nil {
				n.C = r.resolve(r.callBuilder(f), path+"/l", env)
			}
		}
		return []*RNode{n}
	case "Link":
		n := &RNode{T: "link", ID: path, P: map[string]interface{}{}}
		if d, ok := v.arg("destination"); ok {
			n.P["url"] = describe(d)
		}
		n.C = r.labelFrom(v, path, env, "label")
		return []*RNode{n}
	case "ShareLink":
		n := &RNode{T: "button", ID: path, P: map[string]interface{}{"style": r.buttonStyleName(env), "ctx": env.ctx}}
		lbl := r.labelFrom(v, path, env, "label")
		if len(lbl) == 0 {
			// `ShareLink(item: url) { Label(...) }` passes the label as a trailing closure.
			if f := v.closure("content"); f != nil {
				lbl = r.resolve(r.callBuilder(f), path+"/l", env)
			}
		}
		if len(lbl) == 0 {
			lbl = []*RNode{{T: "image", P: map[string]interface{}{"sys": "square.and.arrow.up"}}}
		}
		n.C = lbl
		n.P["id"] = r.on(path+"#share", func(interface{}) { ip.printFn("ShareLink tapped (sharing is not available in the preview)") })
		return []*RNode{n}
	case "Menu":
		n := &RNode{T: "menu", ID: path, P: map[string]interface{}{}}
		lbl := r.labelFrom(v, path, env, "label")
		n.C = lbl
		e := env.clone()
		e.inMenu = true
		e.ctx = "menu"
		n.P["items"] = r.resolve(r.callBuilder(v.closure("content")), path+"/items", e)
		n.P["style"] = r.buttonStyleName(env)
		n.P["mid"] = path + "#menu"
		return []*RNode{n}
	case "NavigationStack", "NavigationView":
		return []*RNode{r.navStack(v, path, env)}
	case "NavigationSplitView":
		sb := v.closure("sidebar")
		if sb == nil {
			sb = v.closure("content")
		}
		nv := &ViewV{Kind: "NavigationStack", Args: []ArgV{{Label: "content", V: sb}}, Pos: v.Pos}
		if !r.ip.isPad() {
			return []*RNode{r.navStack(nv, path, env)}
		}
		// On iPad the sidebar is its own column and pushed pages appear in the detail column.
		sidebarEnv := env.clone()
		sidebarEnv.splitSidebar = true
		sidebar := r.navStack(nv, path, sidebarEnv)
		detailView := &ViewV{Kind: "NavigationStack", Args: []ArgV{{Label: "content", V: v.closure("detail")}}, Pos: v.Pos}
		if v.closure("detail") == nil {
			detailView = &ViewV{Kind: "Text", Args: []ArgV{{V: ""}}, Pos: v.Pos}
		}
		detail := r.resolve(detailView, path+"/detail", env)
		split := &RNode{T: "split", ID: path + ":split", P: map[string]interface{}{"portrait": !r.ip.device().Landscape}, C: append([]*RNode{sidebar}, detail...)}
		return []*RNode{split}
	case "NavigationLink":
		return []*RNode{r.navLink(v, path, env)}
	case "TabView":
		return []*RNode{r.tabView(v, path, env)}
	case "Tab":
		return r.resolve(r.callBuilder(v.closure("content")), path+"/c", env)
	case "GroupBox":
		n := &RNode{T: "groupbox", ID: path, P: map[string]interface{}{}}
		lbl := r.labelFrom(v, path, env, "label")
		if len(lbl) > 0 {
			n.P["label"] = lbl
		}
		n.C = r.resolve(r.callBuilder(v.closure("content")), path+"/c", env)
		return []*RNode{n}
	case "DisclosureGroup":
		var open bool
		var set func(bool)
		if b, ok := v.arg("isExpanded"); ok {
			bv := b.(*BindingV)
			open = bv.Get() == true
			set = func(x bool) { bv.Set(x) }
		} else {
			key := path + "|expanded"
			box := r.store.state[key]
			if box == nil {
				box = &BoxV{V: false}
				r.store.state[key] = box
			}
			r.touched[key] = true
			open = box.V == true
			set = func(x bool) { box.V = x }
		}
		n := &RNode{T: "disclosure", ID: path, P: map[string]interface{}{"open": open}}
		n.P["label"] = r.labelFrom(v, path, env, "label")
		cur := open
		n.P["id"] = r.on(path+"#disc", func(interface{}) { set(!cur) })
		if open {
			n.C = r.resolve(r.callBuilder(v.closure("content")), path+"/c", env)
		}
		return []*RNode{n}
	case "LabeledContent":
		n := &RNode{T: "labeled", ID: path}
		lbl := r.labelFrom(v, path, env, "label")
		var val []*RNode
		if x, ok := v.arg("value"); ok {
			s := describe(x)
			if f, ok := v.arg("format"); ok {
				s = ip.formatStyle(x, f, v.Pos)
			}
			val = []*RNode{r.textNode(s, path+"/v")}
		} else if f := v.closure("content"); f != nil {
			val = r.resolve(r.callBuilder(f), path+"/v", env)
		}
		n.C = []*RNode{{T: "lbtitle", C: lbl}, {T: "lbvalue", C: val}}
		return []*RNode{n}
	case "ContentUnavailableView":
		n := &RNode{T: "unavailable", ID: path, P: map[string]interface{}{}}
		if s, ok := v.firstString(); ok {
			n.P["title"] = s
		}
		if s, ok := v.arg("systemImage"); ok {
			n.P["sys"] = describe(s)
		}
		if f := v.closure("description"); f != nil {
			n.P["desc"] = r.resolve(r.callBuilder(f), path+"/d", env)
		} else if d, ok := v.arg("description"); ok {
			n.P["desc"] = r.resolve(d, path+"/d", env)
		}
		if f := v.closure("label"); f != nil {
			n.P["lbl"] = r.resolve(r.callBuilder(f), path+"/l", env)
		}
		if f := v.closure("actions"); f != nil {
			n.C = r.resolve(r.callBuilder(f), path+"/a", env)
		}
		return []*RNode{n}
	case "ControlGroup":
		n := &RNode{T: "hstack", ID: path, P: map[string]interface{}{"sp": 8.0}}
		n.C = r.resolve(r.callBuilder(v.closure("content")), path+"/c", env)
		return []*RNode{n}
	case "Rectangle", "Circle", "Ellipse", "Capsule", "RoundedRectangle", "UnevenRoundedRectangle":
		return []*RNode{{T: "shape", ID: path, P: r.shapeSpec(v)}}
	case "LinearGradient", "RadialGradient", "AngularGradient", "EllipticalGradient", "ColorGradient":
		return []*RNode{{T: "gradient", ID: path, P: map[string]interface{}{"g": r.paint(v)}}}
	case "GeometryReader":
		g := r.store.geo[path]
		if g == nil {
			g = &GeoV{W: ip.screenW(), H: ip.screenH() - 59 - 34}
		}
		r.geoKeys[path] = true
		val := r.callBuilder(v.closure("content"), &GeoV{W: g.W, H: g.H, X: g.X, Y: g.Y})
		return []*RNode{{T: "geo", ID: path, P: map[string]interface{}{"gid": path}, C: r.resolve(val, path+"/g", env)}}
	case "AsyncImage":
		u, _ := v.arg("url")
		url := ""
		if u != nil {
			url = describe(u)
		}
		if f := v.closure("content"); f != nil && url != "" {
			img := &ViewV{Kind: "Image", Extra: map[string]Value{"url": url, "async": true}}
			return r.resolve(r.callBuilder(f, img), path+"/c", env)
		}
		if url == "" {
			if f := v.closure("placeholder"); f != nil {
				return r.resolve(r.callBuilder(f), path+"/ph", env)
			}
		}
		return []*RNode{{T: "image", ID: path, P: map[string]interface{}{"url": url, "async": true}}}
	case "ViewThatFits":
		vals := r.flattenValues(r.callBuilder(v.closure("content")))
		if len(vals) > 0 {
			return r.resolve(vals[0], path+"/0", env)
		}
		return nil
	case "TimelineView":
		every := 1.0
		if s, ok := v.arg(""); ok {
			if sv, ok := s.(*SymV); ok {
				switch sv.Name() {
				case "everyMinute":
					every = 60
				case "animation":
					every = 0.05
				case "periodic":
					for _, a := range sv.Args() {
						if a.Label == "by" {
							every, _ = toFloat(a.V)
						}
					}
				}
			}
		}
		r.receivers[path+"#timeline"] = &recvReg{interval: every}
		ctx := newObj("TimelineContext")
		ctx.Props["date"] = &DateV{T: time.Now()}
		return r.resolve(r.callBuilder(v.closure("content"), ctx), path+"/c", env)
	case "Grid":
		n := &RNode{T: "grid", ID: path, P: map[string]interface{}{}}
		if hs, ok := v.arg("horizontalSpacing"); ok {
			n.P["hs"], _ = toFloat(hs)
		}
		if vs, ok := v.arg("verticalSpacing"); ok {
			n.P["vs"], _ = toFloat(vs)
		}
		if al, ok := v.arg("alignment"); ok {
			n.P["al"] = symName(al)
		}
		n.C = r.resolve(r.callBuilder(v.closure("content")), path+"/c", env)
		return []*RNode{n}
	case "GridRow":
		n := &RNode{T: "gridrow", ID: path}
		n.C = r.resolve(r.callBuilder(v.closure("content")), path+"/c", env)
		return []*RNode{n}
	case "LazyVGrid", "LazyHGrid":
		n := &RNode{T: "vgrid", ID: path, P: map[string]interface{}{}}
		key := "columns"
		if v.Kind == "LazyHGrid" {
			n.T = "hgrid"
			key = "rows"
		}
		cols, _ := v.arg(key)
		var specs []interface{}
		if a, ok := cols.(*ArrayV); ok {
			for _, it := range a.E {
				specs = append(specs, r.gridItem(it))
			}
		}
		n.P["cols"] = specs
		if sp, ok := v.arg("spacing"); ok && sp != nil {
			n.P["sp"], _ = toFloat(sp)
		}
		if al, ok := v.arg("alignment"); ok {
			n.P["al"] = symName(al)
		}
		n.C = r.resolve(r.callBuilder(v.closure("content")), path+"/c", env)
		return []*RNode{n}
	case "EditButton":
		return []*RNode{{T: "button", ID: path, P: map[string]interface{}{"style": "borderless", "ctx": env.ctx}, C: []*RNode{r.textNode("Edit", path+"/t")}}}
	case "Placeholder":
		return []*RNode{{T: "placeholder", ID: path, P: map[string]interface{}{"text": describe(v.Extra["name"])}}}
	case "Chart":
		return r.chartView(v, path)
	case "Map":
		return []*RNode{{T: "placeholder", ID: path, P: map[string]interface{}{"text": "Map", "map": true}}}
	case "Path":
		return []*RNode{{T: "shape", ID: path, P: map[string]interface{}{"k": "rect"}}}
	case "Gradient", "GridItem", "EdgeInsets", "UnitPoint", "Alert", "ToolbarItem", "ToolbarItemGroup", "Animation", "StrokeStyle", "ActionSheet":
		r.fail(v.Pos, "'%s' is not a View and can't be placed here", v.Kind)
	}
	r.fail(v.Pos, "'%s' is not supported by the preview yet", v.Kind)
	return nil
}

func (r *Render) gridItem(v Value) interface{} {
	m := map[string]interface{}{"k": "flexible"}
	var args []ArgV
	switch x := v.(type) {
	case *ViewV:
		args = x.Args
	case *SymV:
		args = x.Args()
	}
	for _, a := range args {
		switch a.Label {
		case "", "size":
			if s, ok := a.V.(*SymV); ok {
				m["k"] = s.Name()
				for _, sa := range s.Args() {
					f, _ := toFloat(sa.V)
					if math.IsInf(f, 1) {
						continue
					}
					switch sa.Label {
					case "minimum":
						m["min"] = f
					case "maximum":
						m["max"] = f
					case "":
						m["size"] = f
					}
				}
			}
		case "spacing":
			m["sp"], _ = toFloat(a.V)
		case "alignment":
			m["al"] = symName(a.V)
		}
	}
	return m
}

func (r *Render) textOf(v *ViewV, path string) *RNode {
	ip := r.ip
	n := &RNode{T: "text", ID: path, P: map[string]interface{}{}}
	if len(v.Args) == 0 {
		n.P["s"] = ""
		return n
	}
	a0 := v.Args[0]
	md := v.Extra["md"] == true
	s := ""
	switch a0.Label {
	case "verbatim":
		s = describe(a0.V)
		md = false
	default:
		switch x := a0.V.(type) {
		case string:
			s = x
		case Character:
			s = string(x)
		case *DateV:
			if st, ok := v.arg("style"); ok {
				switch symName(st) {
				case "date":
					s = formatDateStyle(x.T, "long", "")
				case "time":
					s = formatDateStyle(x.T, "", "shortened")
				case "relative", "offset":
					s = relativeTime(x.T)
				case "timer":
					d := time.Since(x.T)
					if d < 0 {
						d = -d
					}
					s = fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
					r.receivers[path+"#timer"] = &recvReg{interval: 1}
				}
			} else if f, ok := v.arg("format"); ok {
				s = ip.formatStyle(x, f, v.Pos)
			} else {
				s = formatDateStyle(x.T, "numeric", "shortened")
			}
		default:
			if f, ok := v.arg("format"); ok {
				s = ip.formatStyle(x, f, v.Pos)
			} else {
				s = describe(x)
			}
		}
	}
	n.P["s"] = s
	if md && strings.ContainsAny(s, "*_`~[") {
		n.P["md"] = true
	}
	return n
}

func (r *Render) shapeSpec(v Value) map[string]interface{} {
	m := map[string]interface{}{"k": "rect"}
	switch x := v.(type) {
	case *ViewV:
		switch x.Kind {
		case "Rectangle":
			m["k"] = "rect"
		case "Circle":
			m["k"] = "circle"
		case "Ellipse":
			m["k"] = "ellipse"
		case "Capsule":
			m["k"] = "capsule"
		case "RoundedRectangle":
			m["k"] = "rrect"
			if cr, ok := x.arg("cornerRadius"); ok {
				m["r"], _ = toFloat(cr)
			}
			if cs, ok := x.arg("cornerSize"); ok {
				if sz, ok := cs.(*SizeV); ok {
					m["r"] = sz.W
				}
			}
		case "UnevenRoundedRectangle":
			m["k"] = "uneven"
			for _, l := range []string{"topLeadingRadius", "bottomLeadingRadius", "bottomTrailingRadius", "topTrailingRadius"} {
				if a, ok := x.arg(l); ok {
					m[l], _ = toFloat(a)
				}
			}
		}
	case *SymV:
		switch x.Name() {
		case "circle":
			m["k"] = "circle"
		case "capsule":
			m["k"] = "capsule"
		case "ellipse":
			m["k"] = "ellipse"
		case "rect", "rectangle":
			m["k"] = "rect"
			for _, a := range x.Args() {
				if a.Label == "cornerRadius" {
					m["k"] = "rrect"
					m["r"], _ = toFloat(a.V)
				}
			}
		case "roundedRectangle":
			m["k"] = "rrect"
			for _, a := range x.Args() {
				if a.Label == "cornerRadius" {
					m["r"], _ = toFloat(a.V)
				}
			}
		}
	case *ModV:
		return r.shapeSpec(x.Base)
	}
	return m
}

// ---------- paint ----------

func (r *Render) colorHex(c *ColorV) string {
	if c.Name != "" {
		if hex, ok := lightHex[c.Name]; ok {
			return hex
		}
		return "#888888"
	}
	cl := func(f float64) int { return int(math.Max(0, math.Min(255, math.Round(f*255)))) }
	return fmt.Sprintf("#%02x%02x%02x", cl(c.R), cl(c.G), cl(c.B))
}

var lightHex = map[string]string{"red": "#ff3b30", "orange": "#ff9500", "yellow": "#ffcc00", "green": "#34c759", "mint": "#00c7be", "teal": "#30b0c7",
	"cyan": "#32ade6", "blue": "#007aff", "indigo": "#5856d6", "purple": "#af52de", "pink": "#ff2d55", "brown": "#a2845e", "gray": "#8e8e93",
	"black": "#000000", "white": "#ffffff"}

func colorCSS(c *ColorV) string {
	op := c.Op
	if op == 0 && c.Name == "" {
		op = 1
	}
	if c.Name != "" {
		base := "var(--c-" + c.Name + ")"
		if op != 1 {
			return fmt.Sprintf("color-mix(in srgb, %s %.1f%%, transparent)", base, math.Max(0, math.Min(1, op))*100)
		}
		return base
	}
	cl := func(f float64) int { return int(math.Max(0, math.Min(255, math.Round(f*255)))) }
	return fmt.Sprintf("rgba(%d,%d,%d,%.3f)", cl(c.R), cl(c.G), cl(c.B), c.A*op)
}

var materials = map[string]bool{"ultraThinMaterial": true, "thinMaterial": true, "regularMaterial": true, "thickMaterial": true, "ultraThickMaterial": true, "bar": true, "material": true}

func unitPoint(v Value) []float64 {
	switch x := v.(type) {
	case *SymV:
		switch x.Name() {
		case "top":
			return []float64{0.5, 0}
		case "bottom":
			return []float64{0.5, 1}
		case "leading":
			return []float64{0, 0.5}
		case "trailing":
			return []float64{1, 0.5}
		case "topLeading":
			return []float64{0, 0}
		case "topTrailing":
			return []float64{1, 0}
		case "bottomLeading":
			return []float64{0, 1}
		case "bottomTrailing":
			return []float64{1, 1}
		case "center":
			return []float64{0.5, 0.5}
		case "init":
			var px, py float64
			for _, a := range x.Args() {
				if a.Label == "x" {
					px, _ = toFloat(a.V)
				} else if a.Label == "y" {
					py, _ = toFloat(a.V)
				}
			}
			return []float64{px, py}
		}
	case *ViewV:
		if x.Kind == "UnitPoint" {
			px, _ := x.arg("x")
			py, _ := x.arg("y")
			fx, _ := toFloat(px)
			fy, _ := toFloat(py)
			return []float64{fx, fy}
		}
	}
	return []float64{0.5, 0.5}
}

func (r *Render) gradientStops(args []ArgV) [][]interface{} {
	var stops [][]interface{}
	addColors := func(arr Value) {
		a, ok := arr.(*ArrayV)
		if !ok {
			return
		}
		for i, c := range a.E {
			loc := 0.0
			if len(a.E) > 1 {
				loc = float64(i) / float64(len(a.E)-1)
			}
			stops = append(stops, []interface{}{r.paintColor(c), loc})
		}
	}
	addStops := func(arr Value) {
		a, ok := arr.(*ArrayV)
		if !ok {
			return
		}
		for _, s := range a.E {
			var cargs []ArgV
			switch x := s.(type) {
			case *SymV:
				cargs = x.Args()
			case *ViewV:
				cargs = x.Args
			}
			var col Value
			loc := 0.0
			for _, ca := range cargs {
				if ca.Label == "color" {
					col = ca.V
				} else if ca.Label == "location" {
					loc, _ = toFloat(ca.V)
				}
			}
			stops = append(stops, []interface{}{r.paintColor(col), loc})
		}
	}
	for _, a := range args {
		switch a.Label {
		case "colors":
			addColors(a.V)
		case "stops":
			addStops(a.V)
		case "gradient", "":
			if g, ok := a.V.(*ViewV); ok && g.Kind == "Gradient" {
				if c, ok := g.arg("colors"); ok {
					addColors(c)
				}
				if s, ok := g.arg("stops"); ok {
					addStops(s)
				}
			}
			if s, ok := a.V.(*SymV); ok && s.Name() == "init" {
				for _, ga := range s.Args() {
					if ga.Label == "colors" {
						addColors(ga.V)
					}
					if ga.Label == "stops" {
						addStops(ga.V)
					}
				}
			}
		}
	}
	return stops
}

func (r *Render) paintColor(v Value) string {
	switch x := v.(type) {
	case *ColorV:
		return colorCSS(x)
	case *SymV:
		if c, ok := r.ip.colorFromSym(x).(*ColorV); ok {
			return colorCSS(c)
		}
	}
	return "transparent"
}

func (r *Render) gradFromArgs(kind string, args []ArgV) map[string]interface{} {
	g := map[string]interface{}{"g": kind, "stops": r.gradientStops(args)}
	for _, a := range args {
		switch a.Label {
		case "startPoint":
			g["sp"] = unitPoint(a.V)
		case "endPoint":
			g["ep"] = unitPoint(a.V)
		case "center":
			g["c"] = unitPoint(a.V)
		case "startRadius":
			g["sr"], _ = toFloat(a.V)
		case "endRadius":
			g["er"], _ = toFloat(a.V)
		case "startAngle", "angle":
			g["sa"] = r.angleDeg(a.V)
		case "endAngle":
			g["ea"] = r.angleDeg(a.V)
		}
	}
	return g
}

func (r *Render) angleDeg(v Value) float64 {
	switch x := v.(type) {
	case *SymV:
		if len(x.Args()) > 0 {
			f, _ := toFloat(x.Args()[0].V)
			if x.Name() == "radians" {
				return f * 180 / math.Pi
			}
			return f
		}
		if x.Name() == "zero" {
			return 0
		}
	case int, float64:
		f, _ := toFloat(x)
		return f
	}
	return 0
}

func (r *Render) paint(v Value) interface{} {
	switch x := v.(type) {
	case nil:
		return nil
	case *ColorV:
		return colorCSS(x)
	case *ViewV:
		switch x.Kind {
		case "LinearGradient":
			return r.gradFromArgs("linear", x.Args)
		case "RadialGradient", "EllipticalGradient":
			return r.gradFromArgs("radial", x.Args)
		case "AngularGradient":
			return r.gradFromArgs("angular", x.Args)
		case "ColorGradient":
			c := x.Args[0].V.(*ColorV)
			return map[string]interface{}{"g": "linear", "sp": []float64{0.5, 0}, "ep": []float64{0.5, 1},
				"stops": [][]interface{}{{fmt.Sprintf("color-mix(in srgb, %s 80%%, white)", colorCSS(c)), 0}, {colorCSS(c), 1}}}
		}
	case *SymV:
		name := x.Name()
		if materials[name] {
			return map[string]interface{}{"mat": name}
		}
		switch name {
		case "linearGradient":
			return r.gradFromArgs("linear", x.Args())
		case "radialGradient":
			return r.gradFromArgs("radial", x.Args())
		case "angularGradient", "conicGradient":
			return r.gradFromArgs("angular", x.Args())
		case "tint":
			return "var(--tint)"
		case "background":
			return "var(--c-systemBackground)"
		case "foreground":
			return "var(--c-primary)"
		case "selection":
			return "var(--tint)"
		}
		if c := r.ip.colorFromSym(x); c != nil {
			if cv, ok := c.(*ColorV); ok {
				return colorCSS(cv)
			}
			return r.paint(c)
		}
		if len(x.Parts) > 1 && x.Parts[1].Name == "opacity" {
			return r.paint(sym(name))
		}
	case *StructV:
		return nil
	}
	return nil
}

func isPaintValue(v Value) bool {
	switch x := v.(type) {
	case *ColorV:
		return true
	case *SymV:
		return true
	case *ViewV:
		switch x.Kind {
		case "LinearGradient", "RadialGradient", "AngularGradient", "EllipticalGradient", "ColorGradient":
			return true
		}
	}
	return false
}

// ---------- lists ----------

func (r *Render) list(v *ViewV, path string, env *REnv) *RNode {
	style := env.styles["list"]
	if style == "" || style == "automatic" {
		style = "insetGrouped"
	}
	if v.Kind == "Form" {
		if style == "plain" {
			style = "insetGrouped"
		}
	}
	e := env.clone()
	e.inList = true
	e.inForm = v.Kind == "Form"
	e.ctx = "list"
	var kids []*RNode
	if data, ok := v.arg(""); ok {
		fe := &ViewV{Kind: "ForEach", Pos: v.Pos, Args: []ArgV{{V: data}}}
		if id, ok := v.arg("id"); ok {
			fe.Args = append(fe.Args, ArgV{Label: "id", V: id})
		}
		fe.Args = append(fe.Args, ArgV{Label: "content", V: v.closure("content")})
		if rc := v.closure("rowContent"); rc != nil {
			fe.Args[len(fe.Args)-1].V = rc
		}
		kids = r.forEach(fe, path+"/c", e)
	} else {
		kids = r.resolve(r.callBuilder(v.closure("content")), path+"/c", e)
	}
	n := &RNode{T: "list", ID: path, P: map[string]interface{}{"style": style}}
	var cur *RNode
	for _, k := range kids {
		if k.T == "section" {
			cur = nil
			n.C = append(n.C, k)
			continue
		}
		if cur == nil {
			cur = &RNode{T: "section", ID: k.ID + "/sec"}
			n.C = append(n.C, cur)
		}
		cur.C = append(cur.C, k)
	}
	return n
}

func (r *Render) section(v *ViewV, path string, env *REnv) *RNode {
	n := &RNode{T: "section", ID: path, P: map[string]interface{}{}}
	if s, ok := v.firstString(); ok {
		n.P["h"] = []*RNode{r.textNode(s, path+"/h")}
	} else if h, ok := v.arg("header"); ok {
		n.P["h"] = r.resolve(r.callBuilder(h), path+"/h", env)
	}
	if f, ok := v.arg("footer"); ok {
		n.P["f"] = r.resolve(r.callBuilder(f), path+"/f", env)
	}
	if !env.inList {
		n.P["bare"] = true
	}
	n.C = r.resolve(r.callBuilder(v.closure("content")), path+"/c", env)
	return n
}

func (r *Render) forEachItems(data Value, idKP Value, pos Pos) ([]Value, []Value) {
	ip := r.ip
	var items []Value
	switch d := data.(type) {
	case *BindingV:
		base := d.Get()
		arr, ok := base.(*ArrayV)
		if !ok {
			r.fail(pos, "ForEach($binding) requires a binding to an Array")
		}
		for i := range arr.E {
			idx := i
			bb := d
			items = append(items, &BindingV{Get: func() Value {
				a := bb.Get().(*ArrayV)
				if idx < len(a.E) {
					return a.E[idx]
				}
				return nil
			}, Set: func(v Value) {
				a := bb.Get().(*ArrayV)
				if idx < len(a.E) {
					a.E[idx] = copyValue(ip.coerce(v, a.Elem, nil))
				}
				bb.Set(a)
			}})
		}
	case *ArrayV:
		items = append(items, d.E...)
	case *RangeV:
		items = ip.iterate(d, pos)
		if idKP == nil {
			ids := make([]Value, len(items))
			copy(ids, items)
			return items, ids
		}
	case *SetV:
		r.fail(pos, "generic struct 'ForEach' requires that 'Set' conform to 'RandomAccessCollection' (wrap it: Array(mySet))")
	case *DictV:
		r.fail(pos, "generic struct 'ForEach' requires that 'Dictionary' conform to 'RandomAccessCollection' (use Array(dict.keys) or dict.sorted(by:))")
	case string:
		r.fail(pos, "generic struct 'ForEach' requires that 'String' conform to 'RandomAccessCollection' (use Array(text))")
	case nil:
		r.fail(pos, "ForEach data is nil")
	default:
		items = ip.iterate(d, pos)
	}
	ids := make([]Value, len(items))
	for i, it := range items {
		val := it
		if b, ok := it.(*BindingV); ok {
			val = b.Get()
		}
		if kp, ok := idKP.(*KeyPathV); ok {
			ids[i] = ip.applyKeyPath(kp, val, pos)
			continue
		}
		ti := typeInfoOf(val)
		if ti != nil && ti.conformsTo("Identifiable") {
			ids[i] = ip.getMember(val, "id", pos, nil)
			continue
		}
		if ti != nil {
			if f := ti.field("id"); f != nil {
				r.fail(pos, "Referencing initializer 'init(_:content:)' on 'ForEach' requires that '%s' conform to 'Identifiable' (add ': Identifiable' to the type)", ti.Name)
			}
		}
		r.fail(pos, "Referencing initializer 'init(_:content:)' on 'ForEach' requires that '%s' conform to 'Identifiable' (use ForEach(items, id: \\.self))", typeName(val))
	}
	return items, ids
}

func (r *Render) forEach(v *ViewV, path string, env *REnv) []*RNode {
	data, _ := v.arg("")
	idKP, _ := v.arg("id")
	content := v.closure("content")
	items, ids := r.forEachItems(data, idKP, v.Pos)
	var out []*RNode
	seen := map[string]int{}
	for i, item := range items {
		k := hashKey(ids[i])
		if c := seen[k]; c > 0 {
			k = fmt.Sprintf("%s~%d", k, c)
		}
		seen[hashKey(ids[i])]++
		r.tagVals[hashKey(ids[i])] = ids[i]
		val := r.callBuilder(content, item)
		nodes := r.resolve(val, path+"/k"+k, env)
		for _, n := range nodes {
			n.set("_i", i)
			n.set("_id", hashKey(ids[i]))
		}
		out = append(out, nodes...)
	}
	return out
}

// ---------- controls ----------

func (r *Render) buttonStyleName(env *REnv) string {
	if env.buttonStyle == nil {
		return "automatic"
	}
	if s, ok := env.buttonStyle.(*SymV); ok {
		return s.Name()
	}
	return "custom"
}

func (r *Render) button(v *ViewV, path string, env *REnv) *RNode {
	ip := r.ip
	var lbl []*RNode
	if s, ok := v.firstString(); ok {
		t := r.textNode(s, path+"/l")
		if sys, ok := v.arg("systemImage"); ok {
			lbl = []*RNode{{T: "label", ID: path + "/lb", P: map[string]interface{}{"style": labelStyle(env)}, C: []*RNode{
				{T: "lbicon", C: []*RNode{{T: "image", P: map[string]interface{}{"sys": describe(sys)}}}}, {T: "lbtitle", C: []*RNode{t}}}}}
		} else {
			lbl = []*RNode{t}
		}
	} else if f := v.closure("label"); f != nil {
		lbl = r.resolve(r.callBuilder(f), path+"/l", env)
	} else {
		for _, a := range v.Args {
			if a.Label == "" && a.Trailing {
				if _, hasAction := v.arg("action"); hasAction {
					lbl = r.resolve(r.callBuilder(a.V), path+"/l", env)
				}
			}
		}
	}
	action, ok := v.arg("action")
	if !ok {
		for _, a := range v.Args {
			if a.Label == "" && a.Trailing {
				action = a.V
				break
			}
		}
	}
	role := ""
	if rl, ok := v.arg("role"); ok {
		role = symName(rl)
	}
	id := path + "#tap"
	act := action
	if !env.disabled {
		r.on(id, func(interface{}) {
			if act != nil {
				ip.callValue(act, nil, v.Pos, nil)
			}
		})
	}
	if sv, ok := env.buttonStyle.(*StructV); ok {
		cfg := newObj("ButtonStyleConfiguration")
		cfg.Props["label"] = &ViewV{Kind: "Prerendered", Extra: map[string]Value{"nodes": lbl}}
		cfg.Props["isPressed"] = false
		if role != "" {
			cfg.Props["role"] = sym(role)
		} else {
			cfg.Props["role"] = nil
		}
		cfg.Props["trigger"] = &NativeV{Name: "trigger", Fn: func(c *CallCtx) Value {
			if act != nil {
				ip.callValue(act, nil, v.Pos, nil)
			}
			return Void
		}}
		inst := r.prepareStruct(sv, path+":style", env)
		body := ip.callMethod(inst, nil, "makeBody", []ArgV{{Label: "configuration", V: cfg}}, v.Pos, nil)
		e := env.clone()
		e.buttonStyle = nil
		nodes := r.resolve(body, path+"/style", e)
		n := &RNode{T: "tap", ID: path, P: map[string]interface{}{"id": id}, C: nodes}
		if sv.T.conformsTo("PrimitiveButtonStyle") {
			n.P["id"] = nil
		}
		return n
	}
	n := &RNode{T: "button", ID: path, P: map[string]interface{}{"style": r.buttonStyleName(env), "ctx": env.ctx}, C: lbl}
	if cs := env.styles["controlSize"]; cs != "" && cs != "regular" {
		n.P["cs"] = cs
	}
	if !env.disabled {
		n.P["id"] = id
	} else {
		n.P["dis"] = true
	}
	if role != "" {
		n.P["role"] = role
	}
	return n
}

func (r *Render) toggle(v *ViewV, path string, env *REnv) *RNode {
	b, _ := v.arg("isOn")
	bv := b.(*BindingV)
	on := bv.Get() == true
	n := &RNode{T: "toggle", ID: path, P: map[string]interface{}{"on": on, "style": env.styles["toggle"], "ctx": env.ctx}}
	lbl := r.labelFrom(v, path, env, "label")
	if len(lbl) == 0 {
		for _, a := range v.Args {
			if a.Trailing && a.Label == "" {
				lbl = r.resolve(r.callBuilder(a.V), path+"/l", env)
			}
		}
	}
	if !env.labelsHidden {
		n.C = lbl
	}
	if !env.disabled {
		n.P["id"] = r.on(path+"#toggle", func(interface{}) { bv.Set(!(bv.Get() == true)) })
	} else {
		n.P["dis"] = true
	}
	return n
}

func (r *Render) textField(v *ViewV, path string, env *REnv) *RNode {
	ip := r.ip
	n := &RNode{T: "textfield", ID: path, P: map[string]interface{}{"style": env.styles["textField"], "ctx": env.ctx}}
	if v.Kind == "SecureField" {
		n.P["sec"] = true
	}
	if v.Kind == "TextEditor" {
		n.P["editor"] = true
		n.P["multi"] = true
	}
	if s, ok := v.firstString(); ok {
		n.P["ph"] = s
	}
	if p, ok := v.arg("prompt"); ok {
		if pv, ok := p.(*ViewV); ok && pv.Kind == "Text" && len(pv.Args) > 0 {
			n.P["ph"] = describe(pv.Args[0].V)
		}
	}
	if ax, ok := v.arg("axis"); ok && symName(ax) == "vertical" {
		n.P["multi"] = true
	}
	if tb, ok := v.arg("text"); ok {
		bv := tb.(*BindingV)
		n.P["v"] = describe(bv.Get())
		if !env.disabled {
			n.P["id"] = r.on(path+"#text", func(x interface{}) {
				s, _ := x.(string)
				bv.Set(s)
			})
		}
	} else if vb, ok := v.arg("value"); ok {
		bv := vb.(*BindingV)
		cur := bv.Get()
		if f, ok := v.arg("format"); ok {
			n.P["v"] = strings.ReplaceAll(ip.formatStyle(cur, f, v.Pos), ",", "")
			if _, isCur := f.(*SymV); isCur && f.(*SymV).Name() == "currency" {
				n.P["v"] = strings.ReplaceAll(ip.formatStyle(cur, f, v.Pos), ",", "")
			}
		} else {
			n.P["v"] = describe(cur)
		}
		n.P["num"] = true
		n.P["id"] = r.on(path+"#num", func(x interface{}) {
			s, _ := x.(string)
			s = strings.Trim(strings.ReplaceAll(s, ",", ""), " $€£R%")
			if _, isInt := bv.Get().(int); isInt {
				if i, err := strconv.Atoi(s); err == nil {
					bv.Set(i)
				}
				return
			}
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				bv.Set(f)
			}
		})
	}
	if env.submit != nil {
		sf := env.submit
		n.P["sub"] = r.on(path+"#submit", func(interface{}) { ip.callValue(sf, nil, v.Pos, nil) })
	}
	if env.focus != nil {
		fi := env.focus
		cur := fi.b.Get()
		focused := false
		if fi.has {
			focused = valueEquals(cur, fi.equals) || (func() bool {
				if s, ok := fi.equals.(*SymV); ok {
					if e, ok := cur.(*EnumV); ok {
						return e.Case == s.Name()
					}
				}
				return false
			})()
		} else {
			focused = cur == true
		}
		n.P["foc"] = focused
		n.P["fid"] = r.on(path+"#focus", func(x interface{}) {
			f, _ := x.(bool)
			if fi.has {
				if f {
					val := fi.equals
					if s, ok := val.(*SymV); ok {
						if e, ok := fi.b.Get().(*EnumV); ok {
							val = ip.symToType(e.T, s, Pos{})
						}
					}
					fi.b.Set(val)
				} else if focusedNow := fi.b.Get(); focusedNow != nil && valueEquals(focusedNow, fi.equals) {
					fi.b.Set(nil)
				}
			} else {
				fi.b.Set(f)
			}
		})
	}
	if env.disabled {
		n.P["dis"] = true
	}
	return n
}

func (r *Render) slider(v *ViewV, path string, env *REnv) *RNode {
	b, _ := v.arg("value")
	bv := b.(*BindingV)
	cur := bv.Get()
	if _, isInt := cur.(int); isInt {
		r.fail(v.Pos, "Slider requires a Double binding (got Int). Declare it like @State private var value = 0.0")
	}
	f, _ := toFloat(cur)
	lo, hi := 0.0, 1.0
	if rg, ok := v.arg("in"); ok {
		if rv, ok := rg.(*RangeV); ok {
			lo, _ = toFloat(rv.Lo)
			hi, _ = toFloat(rv.Hi)
		}
	}
	n := &RNode{T: "slider", ID: path, P: map[string]interface{}{"v": f, "min": lo, "max": hi}}
	if st, ok := v.arg("step"); ok {
		n.P["step"], _ = toFloat(st)
	}
	if fm := v.closure("minimumValueLabel"); fm != nil {
		n.P["minL"] = r.resolve(r.callBuilder(fm), path+"/min", env)
	}
	if fm := v.closure("maximumValueLabel"); fm != nil {
		n.P["maxL"] = r.resolve(r.callBuilder(fm), path+"/max", env)
	}
	if !env.disabled {
		n.P["id"] = r.on(path+"#slide", func(x interface{}) {
			switch t := x.(type) {
			case float64:
				bv.Set(t)
			case string:
				if fv, err := strconv.ParseFloat(t, 64); err == nil {
					bv.Set(fv)
				}
			}
		})
	} else {
		n.P["dis"] = true
	}
	return n
}

func (r *Render) stepper(v *ViewV, path string, env *REnv) *RNode {
	ip := r.ip
	n := &RNode{T: "stepper", ID: path, P: map[string]interface{}{"ctx": env.ctx}}
	lbl := r.labelFrom(v, path, env, "label")
	if len(lbl) == 0 {
		for _, a := range v.Args {
			if a.Trailing && a.Label == "" {
				lbl = r.resolve(r.callBuilder(a.V), path+"/l", env)
			}
		}
	}
	if !env.labelsHidden {
		n.C = lbl
	}
	if b, ok := v.arg("value"); ok {
		bv := b.(*BindingV)
		var step Value = 1
		if s, ok := v.arg("step"); ok {
			step = s
		}
		var rg *RangeV
		if x, ok := v.arg("in"); ok {
			rg, _ = x.(*RangeV)
		}
		cur := bv.Get()
		canInc, canDec := true, true
		if rg != nil {
			if c, ok := compareValues(cur, rg.Hi); ok && c >= 0 {
				canInc = false
			}
			if c, ok := compareValues(cur, rg.Lo); ok && c <= 0 {
				canDec = false
			}
		}
		clamp := func(x Value) Value {
			if rg == nil {
				return x
			}
			if c, _ := compareValues(x, rg.Hi); c > 0 {
				return rg.Hi
			}
			if c, _ := compareValues(x, rg.Lo); c < 0 {
				return rg.Lo
			}
			return x
		}
		if canInc && !env.disabled {
			n.P["inc"] = r.on(path+"#inc", func(interface{}) {
				nv := ip.binop("+", bv.Get(), step, v.Pos)
				if _, isF := bv.Get().(float64); isF {
					nv, _ = toFloatV(nv)
				}
				bv.Set(clamp(nv))
			})
		}
		if canDec && !env.disabled {
			n.P["dec"] = r.on(path+"#dec", func(interface{}) {
				nv := ip.binop("-", bv.Get(), step, v.Pos)
				bv.Set(clamp(nv))
			})
		}
	} else {
		if f, ok := v.arg("onIncrement"); ok && f != nil {
			n.P["inc"] = r.on(path+"#inc", func(interface{}) { ip.callValue(f, nil, v.Pos, nil) })
		}
		if f, ok := v.arg("onDecrement"); ok && f != nil {
			n.P["dec"] = r.on(path+"#dec", func(interface{}) { ip.callValue(f, nil, v.Pos, nil) })
		}
	}
	return n
}

func toFloatV(v Value) (Value, bool) {
	f, ok := toFloat(v)
	return f, ok
}

func (r *Render) picker(v *ViewV, path string, env *REnv) *RNode {
	b, _ := v.arg("selection")
	bv := b.(*BindingV)
	n := &RNode{T: "picker", ID: path, P: map[string]interface{}{"ctx": env.ctx}}
	style := env.styles["picker"]
	if style == "" || style == "automatic" {
		style = "menu"
	}
	n.P["style"] = style
	lbl := r.labelFrom(v, path, env, "label")
	if !env.labelsHidden {
		n.P["label"] = lbl
	}
	e := env.clone()
	e.ctx = "picker"
	opts := r.resolve(r.callBuilder(v.closure("content")), path+"/o", e)
	sel := hashKey(bv.Get())
	var list []interface{}
	for _, o := range opts {
		k, ok := o.P["tag"].(string)
		if !ok {
			k, ok = o.P["_id"].(string)
		}
		if !ok {
			continue
		}
		list = append(list, map[string]interface{}{"k": k, "n": o, "t": plainText([]*RNode{o}), "sys": firstSysImage([]*RNode{o})})
	}
	n.P["opts"] = list
	n.P["sel"] = sel
	if !env.disabled {
		n.P["id"] = r.on(path+"#pick", func(x interface{}) {
			k, _ := x.(string)
			if val, ok := r.tagVals[k]; ok {
				if s, ok := val.(*SymV); ok {
					if ti := typeInfoOf(bv.Get()); ti != nil {
						val = r.ip.symToType(ti, s, Pos{})
					}
				}
				bv.Set(val)
			}
		})
	}
	return n
}

func (r *Render) datePicker(v *ViewV, path string, env *REnv) *RNode {
	b, _ := v.arg("selection")
	bv := b.(*BindingV)
	n := &RNode{T: "datepicker", ID: path, P: map[string]interface{}{"ctx": env.ctx}}
	comps := "both"
	if c, ok := v.arg("displayedComponents"); ok {
		switch x := c.(type) {
		case *SymV:
			comps = x.Name()
		case *ArrayV:
			if len(x.E) == 1 {
				comps = symName(x.E[0])
			}
		}
	}
	n.P["comps"] = comps
	if d, ok := bv.Get().(*DateV); ok {
		n.P["v"] = d.T.Format("2006-01-02T15:04")
		n.P["disp"] = map[string]string{
			"date":          formatDateStyle(d.T, "abbreviated", ""),
			"hourAndMinute": formatDateStyle(d.T, "", "shortened"),
		}
	}
	if !env.labelsHidden {
		n.C = r.labelFrom(v, path, env, "label")
	}
	n.P["id"] = r.on(path+"#date", func(x interface{}) {
		s, _ := x.(string)
		old, _ := bv.Get().(*DateV)
		loc := time.Local
		var t time.Time
		var err error
		switch {
		case len(s) == 5:
			t, err = time.ParseInLocation("15:04", s, loc)
			if err == nil && old != nil {
				y, m, d := old.T.Date()
				t = time.Date(y, m, d, t.Hour(), t.Minute(), 0, 0, loc)
			}
		case len(s) == 10:
			t, err = time.ParseInLocation("2006-01-02", s, loc)
			if err == nil && old != nil {
				t = time.Date(t.Year(), t.Month(), t.Day(), old.T.Hour(), old.T.Minute(), 0, 0, loc)
			}
		default:
			t, err = time.ParseInLocation("2006-01-02T15:04", s, loc)
		}
		if err == nil {
			bv.Set(&DateV{T: t})
		}
	})
	return n
}

// ---------- navigation ----------

func (r *Render) navStack(v *ViewV, path string, env *REnv) *RNode {
	ip := r.ip
	key := path + ":nav"
	st := r.store.nav[key]
	if st == nil {
		st = &NavState{}
		r.store.nav[key] = st
	}
	r.navTouched[key] = true
	nav := &NavCtx{key: key, state: st, links: map[string]Value{}}
	if b, ok := v.arg("path"); ok {
		nav.pathB, _ = b.(*BindingV)
	}
	var entries []*NavEntry
	if nav.pathB != nil {
		if arr, ok := nav.pathB.Get().(*ArrayV); ok {
			for i, e := range arr.E {
				entries = append(entries, &NavEntry{ID: i, Value: e, IsValue: true})
			}
		}
	} else {
		entries = st.Entries
	}
	content := v.closure("content")
	if content == nil {
		content = v.closure("root")
	}
	type pageRec struct {
		pc    *PageCtx
		nodes []*RNode
		back  string
	}
	var pages []pageRec
	pc0 := &PageCtx{}
	e0 := env.clone()
	e0.nav, e0.page, e0.inList, e0.inForm, e0.ctx = nav, pc0, false, false, ""
	e0.hidden = env.hidden || (len(entries) > 0 && !env.splitSidebar)
	nodes0 := r.resolve(r.callBuilder(content), key+"/root", e0)
	pages = append(pages, pageRec{pc0, nodes0, ""})
	popTo := func(k int) func() {
		return func() {
			if nav.pathB != nil {
				if arr, ok := nav.pathB.Get().(*ArrayV); ok && k <= len(arr.E) {
					nav.pathB.Set(&ArrayV{E: append([]Value(nil), arr.E[:k]...), Elem: arr.Elem})
				}
				return
			}
			if k <= len(st.Entries) {
				st.Entries = st.Entries[:k]
			}
		}
	}
	for k, ent := range entries {
		var dest Value
		if ent.IsValue {
			tn := typeName(ent.Value)
			for i := len(nav.dests) - 1; i >= 0; i-- {
				if nav.dests[i].typ == tn || (nav.dests[i].typ == "Double" && tn == "Int") {
					dest = r.callBuilder(nav.dests[i].fn, ent.Value)
					break
				}
			}
			if dest == nil {
				dest = &ViewV{Kind: "Text", Args: []ArgV{{V: "⚠️ No navigationDestination(for: " + tn + ".self) found"}}, Extra: map[string]Value{}}
			}
		} else {
			if d, ok := nav.links[ent.LinkPath]; ok {
				dest = d
			} else {
				dest = ent.Dest
			}
			dest = r.callBuilder(dest)
		}
		pc := &PageCtx{}
		e := env.clone()
		e.nav, e.page, e.inList, e.inForm, e.ctx = nav, pc, false, false, ""
		e.hidden = env.hidden || k < len(entries)-1
		pop := popTo(k)
		e = e.withEnvVal("dismiss", &NativeV{Name: "dismiss", Fn: func(c *CallCtx) Value { pop(); return Void }})
		nodes := r.resolve(dest, key+"/p"+strconv.Itoa(ent.ID), e)
		backID := r.on(key+"#back"+strconv.Itoa(k), func(interface{}) { pop() })
		pages = append(pages, pageRec{pc, nodes, backID})
	}
	for i, pd := range nav.presented {
		var val Value
		if pd.item {
			val = pd.b.Get()
			if val == nil {
				continue
			}
		} else if pd.b.Get() != true {
			continue
		}
		pc := &PageCtx{}
		e := env.clone()
		e.nav, e.page, e.inList, e.inForm, e.ctx = nav, pc, false, false, ""
		b := pd.b
		item := pd.item
		dismiss := func() {
			if item {
				b.Set(nil)
			} else {
				b.Set(false)
			}
		}
		e = e.withEnvVal("dismiss", &NativeV{Name: "dismiss", Fn: func(c *CallCtx) Value { dismiss(); return Void }})
		var dest Value
		if item {
			dest = r.callBuilder(pd.fn, val)
		} else {
			dest = r.callBuilder(pd.fn)
		}
		nodes := r.resolve(dest, key+"/pres"+strconv.Itoa(i), e)
		backID := r.on(key+"#presback"+strconv.Itoa(i), func(interface{}) { dismiss() })
		pages = append(pages, pageRec{pc, nodes, backID})
	}
	_ = ip
	n := &RNode{T: "nav", ID: key, P: map[string]interface{}{"depth": len(pages) - 1}}
	for i, p := range pages {
		pn := &RNode{T: "page", ID: fmt.Sprintf("%s/pg%d", key, i), P: map[string]interface{}{}, C: p.nodes}
		pn.P["title"] = p.pc.title
		if p.pc.subtitle != "" {
			pn.P["subtitle"] = p.pc.subtitle
		}
		mode := p.pc.mode
		if mode == "" || mode == "automatic" {
			mode = "large"
			if i > 0 {
				prevMode := pages[i-1].pc.mode
				if prevMode == "inline" {
					mode = "inline"
				}
			}
		}
		pn.P["mode"] = mode
		if len(p.pc.lead) > 0 {
			pn.P["lead"] = p.pc.lead
		}
		if len(p.pc.trail) > 0 {
			pn.P["trail"] = p.pc.trail
		}
		if len(p.pc.prin) > 0 {
			pn.P["prin"] = p.pc.prin
		}
		if len(p.pc.bottom) > 0 {
			pn.P["bottom"] = p.pc.bottom
		}
		if p.pc.search != nil {
			pn.P["search"] = p.pc.search
		}
		if p.pc.hideBar {
			pn.P["hideBar"] = true
		}
		if p.pc.barBg != nil {
			pn.P["barBg"] = p.pc.barBg
		}
		if i > 0 && !p.pc.hideBack {
			pn.P["back"] = p.back
			bt := pages[i-1].pc.title
			if bt == "" || len(bt) > 14 {
				bt = "Back"
			}
			pn.P["backT"] = bt
		}
		n.C = append(n.C, pn)
	}
	return n
}

func (r *Render) navLink(v *ViewV, path string, env *REnv) *RNode {
	nav := env.nav
	id := path + "#link"
	var lbl []*RNode
	if s, ok := v.firstString(); ok {
		lbl = []*RNode{r.textNode(s, path+"/l")}
	} else if f := v.closure("label"); f != nil {
		lbl = r.resolve(r.callBuilder(f), path+"/l", env)
	} else {
		for _, a := range v.Args {
			if a.Trailing && a.Label == "" {
				if _, hasDest := v.arg("destination"); hasDest {
					lbl = r.resolve(r.callBuilder(a.V), path+"/l", env)
				} else if _, hasVal := v.arg("value"); hasVal {
					lbl = r.resolve(r.callBuilder(a.V), path+"/l", env)
				}
			}
		}
	}
	n := &RNode{T: "navlink", ID: path, P: map[string]interface{}{"ctx": env.ctx}, C: lbl}
	if nav == nil {
		n.P["dis"] = true
		return n
	}
	if val, ok := v.arg("value"); ok {
		vv := val
		n.P["id"] = r.on(id, func(interface{}) {
			if nav.pathB != nil {
				arr, _ := nav.pathB.Get().(*ArrayV)
				if arr == nil {
					arr = &ArrayV{}
				}
				na := &ArrayV{E: append(append([]Value(nil), arr.E...), copyValue(vv)), Elem: arr.Elem}
				nav.pathB.Set(na)
				return
			}
			nav.state.Next++
			nav.state.Entries = append(nav.state.Entries, &NavEntry{ID: nav.state.Next, Value: copyValue(vv), IsValue: true})
		})
		return n
	}
	var dest Value
	if d, ok := v.arg("destination"); ok {
		dest = d
	} else if f := v.closure("destination"); f != nil {
		dest = f
	} else {
		for _, a := range v.Args {
			if a.Trailing && a.Label == "" {
				dest = a.V
				break
			}
		}
	}
	if b, ok := v.arg("isActive"); ok {
		if bv, ok := b.(*BindingV); ok {
			nav.presented = append(nav.presented, presentedDest{b: bv, fn: dest})
			n.P["id"] = r.on(id, func(interface{}) { bv.Set(true) })
			return n
		}
	}
	nav.links[id] = dest
	d := dest
	n.P["id"] = r.on(id, func(interface{}) {
		nav.state.Next++
		nav.state.Entries = append(nav.state.Entries, &NavEntry{ID: nav.state.Next, LinkPath: id, Dest: d})
	})
	return n
}

// ---------- tabs ----------

func (r *Render) flattenValues(v Value) []Value {
	switch x := v.(type) {
	case nil, *VoidV:
		return nil
	case *ViewV:
		switch x.Kind {
		case "Tuple":
			var out []Value
			for _, k := range x.Kids {
				out = append(out, r.flattenValues(k)...)
			}
			return out
		case "Group":
			return r.flattenValues(r.callBuilder(x.closure("content")))
		case "ForEach":
			data, _ := x.arg("")
			idKP, _ := x.arg("id")
			items, _ := r.forEachItems(data, idKP, x.Pos)
			var out []Value
			for _, it := range items {
				out = append(out, r.flattenValues(r.callBuilder(x.closure("content"), it))...)
			}
			return out
		}
	}
	return []Value{v}
}

type tabInfo struct {
	val    Value
	tag    Value
	hasTag bool
	label  Value
	badge  Value
	title  string
	sys    string
}

func (r *Render) tabView(v *ViewV, path string, env *REnv) *RNode {
	kids := r.flattenValues(r.callBuilder(v.closure("content")))
	var tabs []tabInfo
	for _, k := range kids {
		ti := tabInfo{val: k}
		cur := k
		for {
			m, ok := cur.(*ModV)
			if !ok {
				break
			}
			switch m.Name {
			case "tag":
				if !ti.hasTag && len(m.Args) > 0 {
					ti.tag = m.Args[0].V
					ti.hasTag = true
				}
			case "tabItem":
				if ti.label == nil && len(m.Args) > 0 {
					ti.label = m.Args[0].V
				}
			case "badge":
				if len(m.Args) > 0 {
					ti.badge = m.Args[0].V
				}
			}
			cur = m.Base
		}
		if tv, ok := cur.(*ViewV); ok && tv.Kind == "Tab" {
			if s, ok := tv.firstString(); ok {
				ti.title = s
			}
			if s, ok := tv.arg("systemImage"); ok {
				ti.sys = describe(s)
			}
			if val, ok := tv.arg("value"); ok {
				ti.tag = val
				ti.hasTag = true
			}
			if label, ok := tv.arg("label"); ok && ti.label == nil {
				ti.label = label
			}
		}
		tabs = append(tabs, ti)
	}
	var selB *BindingV
	if b, ok := v.arg("selection"); ok {
		selB, _ = b.(*BindingV)
	}
	if selB == nil {
		key := path + "|tabsel"
		box := r.store.state[key]
		if box == nil {
			var init Value = 0
			if len(tabs) > 0 && tabs[0].hasTag {
				init = tabs[0].tag
			}
			box = &BoxV{V: init}
			r.store.state[key] = box
		}
		r.touched[key] = true
		bx := box
		selB = &BindingV{Get: func() Value { return bx.V }, Set: func(x Value) { bx.V = x }}
	}
	sel := selB.Get()
	selIdx := -1
	for i, t := range tabs {
		if t.hasTag && r.ip.equals(t.tag, sel, Pos{}) {
			selIdx = i
			break
		}
	}
	if selIdx < 0 {
		if i, ok := sel.(int); ok && i >= 0 && i < len(tabs) && !tabs[i].hasTag {
			selIdx = i
		} else {
			selIdx = 0
		}
	}
	style := env.styles["tabView"]
	n := &RNode{T: "tabview", ID: path, P: map[string]interface{}{"style": style, "sel": selIdx}}
	var items []interface{}
	for i, t := range tabs {
		var lbl []*RNode
		if t.label != nil {
			e := env.clone()
			e.ctx = "tabitem"
			lbl = r.resolve(r.callBuilder(t.label), path+"/ti"+strconv.Itoa(i), e)
		}
		item := map[string]interface{}{"t": plainText(lbl), "sys": firstSysImage(lbl), "sel": i == selIdx}
		if t.title != "" {
			item["t"] = t.title
		}
		if t.sys != "" {
			item["sys"] = t.sys
		}
		if t.badge != nil {
			item["badge"] = describe(t.badge)
		}
		tag := t.tag
		hasTag := t.hasTag
		idx := i
		item["id"] = r.on(path+"#tab"+strconv.Itoa(i), func(interface{}) {
			if hasTag {
				selB.Set(tag)
			} else {
				selB.Set(idx)
			}
		})
		items = append(items, item)
	}
	n.P["items"] = items
	if style == "page" {
		for i, t := range tabs {
			e := env.clone()
			e.hidden = env.hidden || i != selIdx
			pg := &RNode{T: "tabpage", ID: path + "/" + strconv.Itoa(i), C: r.resolve(t.val, path+"/"+strconv.Itoa(i), e)}
			n.C = append(n.C, pg)
		}
	} else {
		for i := range tabs {
			if i != selIdx {
				r.keep = append(r.keep, path+"/"+strconv.Itoa(i))
			}
		}
		if selIdx < len(tabs) {
			n.C = r.resolve(tabs[selIdx].val, path+"/"+strconv.Itoa(selIdx), env)
		}
	}
	return n
}

// ---------- modifiers ----------

var styleKeys = map[string]string{
	"textFieldStyle": "textField", "pickerStyle": "picker", "toggleStyle": "toggle", "labelStyle": "label",
	"listStyle": "list", "datePickerStyle": "datePicker", "progressViewStyle": "progress", "tabViewStyle": "tabView",
	"controlSize": "controlSize", "menuStyle": "menu",
}

func (r *Render) modifier(m *ModV, path string, env *REnv) []*RNode {
	ip := r.ip
	a := m.Args
	name := m.Name
	a0 := argAt(a, 0)
	switch name {
	case "buttonStyle":
		e := env.clone()
		if s, ok := a0.(*SymV); ok && s.Name() == "automatic" {
			e.buttonStyle = nil
		} else {
			e.buttonStyle = a0
		}
		return r.resolve(m.Base, path, e)
	case "textFieldStyle", "pickerStyle", "toggleStyle", "labelStyle", "listStyle", "datePickerStyle", "progressViewStyle", "tabViewStyle", "controlSize", "menuStyle":
		sn := symName(a0)
		if tv, ok := a0.(*ViewV); ok {
			sn = strings.TrimSuffix(strings.ToLower(tv.Kind[:1])+tv.Kind[1:], "TabViewStyle")
		}
		if name == "tabViewStyle" && (sn == "page" || strings.HasPrefix(sn, "page")) {
			sn = "page"
		}
		return r.resolve(m.Base, path, env.withStyle(styleKeys[name], sn))
	case "labelsHidden":
		e := env.clone()
		e.labelsHidden = true
		return r.resolve(m.Base, path, e)
	case "modelContainer":
		return r.resolve(m.Base, path, env.withEnvVal("modelContext", r.modelContainerFromModifier(m, path)))
	case "modelContext":
		ctx, ok := a0.(*ModelContextV)
		if !ok {
			r.fail(m.Pos, ".modelContext(_:) needs a ModelContext")
		}
		return r.resolve(m.Base, path, env.withEnvVal("modelContext", ctx))
	case "environmentObject":
		e := env.clone()
		e.envObjects = map[string]Value{}
		for k, v := range env.envObjects {
			e.envObjects[k] = v
		}
		if ti := typeInfoOf(a0); ti != nil {
			for c := ti; c != nil; c = c.Super {
				e.envObjects[c.Name] = a0
			}
		}
		return r.resolve(m.Base, path, e)
	case "environment":
		if len(a) == 2 {
			if kp, ok := a0.(*KeyPathV); ok && len(kp.Path) > 0 {
				e := env.withEnvVal(kp.Path[0], a[1].V)
				nodes := r.resolve(m.Base, path, e)
				if kp.Path[0] == "colorScheme" {
					for _, n := range nodes {
						n.M = append(n.M, RMod{N: "scheme", A: map[string]interface{}{"v": symName(a[1].V)}})
					}
				}
				return nodes
			}
		}
		e := env.clone()
		e.envObjects = map[string]Value{}
		for k, v := range env.envObjects {
			e.envObjects[k] = v
		}
		if ti := typeInfoOf(a0); ti != nil {
			e.envObjects[ti.Name] = a0
		}
		return r.resolve(m.Base, path, e)
	case "disabled":
		b := a0 == true
		e := env.clone()
		e.disabled = env.disabled || b
		nodes := r.resolve(m.Base, path, e)
		if b {
			for _, n := range nodes {
				n.M = append(n.M, RMod{N: "disabled"})
			}
		}
		return nodes
	case "onSubmit":
		e := env.clone()
		for _, x := range a {
			if f, ok := x.V.(*FuncV); ok {
				e.submit = f
			}
		}
		return r.resolve(m.Base, path, e)
	case "focused":
		e := env.clone()
		if b, ok := a0.(*BindingV); ok {
			e.focus = &focusInfo{b: b}
			if eq, ok := argNamed(a, "equals"); ok {
				e.focus.equals = eq
				e.focus.has = true
			}
		}
		return r.resolve(m.Base, path, e)
	case "preferredColorScheme":
		if s := symName(a0); s != "" {
			r.preferredScheme = s
		}
		return r.resolve(m.Base, path, env)
	case "id":
		k := hashKey(a0)
		nodes := r.resolve(m.Base, path+"#"+k, env)
		for _, n := range nodes {
			n.set("sid", k)
		}
		return nodes
	case "modifier":
		vm := a0
		if sv, ok := vm.(*StructV); ok {
			inst := r.prepareStruct(sv, path+":mod", env)
			body := ip.callMethod(inst, nil, "body", []ArgV{{Label: "content", V: m.Base}}, m.Pos, nil)
			return r.resolve(body, path+":mod", env)
		}
		return r.resolve(m.Base, path, env)
	case "tag":
		nodes := r.resolve(m.Base, path, env)
		k := hashKey(a0)
		r.tagVals[k] = a0
		for _, n := range nodes {
			n.set("tag", k)
		}
		return nodes
	case "tabItem":
		return r.resolve(m.Base, path, env)
	case "navigationTitle", "navigationBarTitle":
		nodes := r.resolve(m.Base, path, env)
		if env.page != nil {
			t := ""
			switch x := a0.(type) {
			case string:
				t = x
			case *BindingV:
				t = describe(x.Get())
			default:
				if isTextValue(x) {
					t = plainText(r.resolve(x, path+"/title", env))
				}
			}
			env.page.title = t
			if dm, ok := argNamed(a, "displayMode"); ok {
				env.page.mode = symName(dm)
			}
		}
		return nodes
	case "chartXScale", "chartYScale", "chartXAxis", "chartYAxis", "chartLegend":
		nodes := r.resolve(m.Base, path, env)
		r.applyChartModifier(nodes, name, a)
		return nodes
	case "navigationSubtitle":
		nodes := r.resolve(m.Base, path, env)
		if env.page != nil {
			switch x := a0.(type) {
			case string:
				env.page.subtitle = x
			default:
				if isTextValue(x) {
					env.page.subtitle = plainText(r.resolve(x, path+"/subtitle", env))
				}
			}
		}
		return nodes
	case "navigationBarTitleDisplayMode", "toolbarTitleDisplayMode":
		nodes := r.resolve(m.Base, path, env)
		if env.page != nil {
			env.page.mode = symName(a0)
			if env.page.mode == "inlineLarge" {
				env.page.mode = "large"
			}
		}
		return nodes
	case "navigationBarBackButtonHidden":
		nodes := r.resolve(m.Base, path, env)
		if env.page != nil && (len(a) == 0 || a0 == true) {
			env.page.hideBack = true
		}
		return nodes
	case "navigationBarHidden":
		nodes := r.resolve(m.Base, path, env)
		if env.page != nil && a0 == true {
			env.page.hideBar = true
		}
		return nodes
	case "toolbar", "toolbarVisibility":
		nodes := r.resolve(m.Base, path, env)
		if s, ok := a0.(*SymV); ok {
			hidden := s.Name() == "hidden"
			target := ""
			if f, ok := argNamed(a, "for"); ok {
				target = symName(f)
				if arr, ok := f.(*ArrayV); ok && len(arr.E) > 0 {
					target = symName(arr.E[0])
				}
			}
			if hidden {
				if target == "tabBar" {
					r.hideTabBar = true
				} else if env.page != nil {
					env.page.hideBar = true
				}
			}
			return nodes
		}
		r.toolbar(m, path, env)
		return nodes
	case "navigationBarItems":
		nodes := r.resolve(m.Base, path, env)
		if env.page != nil {
			e := env.clone()
			e.page = nil
			e.ctx = "toolbar"
			if l, ok := argNamed(a, "leading"); ok {
				env.page.lead = append(env.page.lead, r.resolve(l, path+"/nbl", e)...)
			}
			if t, ok := argNamed(a, "trailing"); ok {
				env.page.trail = append(env.page.trail, r.resolve(t, path+"/nbt", e)...)
			}
		}
		return nodes
	case "toolbarBackground":
		nodes := r.resolve(m.Base, path, env)
		if env.page != nil && isPaintValue(a0) {
			if s, ok := a0.(*SymV); !ok || (s.Name() != "visible" && s.Name() != "hidden" && s.Name() != "automatic") {
				env.page.barBg = r.paint(a0)
			}
		}
		return nodes
	case "searchable":
		nodes := r.resolve(m.Base, path, env)
		if b, ok := argNamed(a, "text"); ok && env.page != nil {
			bv := b.(*BindingV)
			ph := "Search"
			if p, ok := argNamed(a, "prompt"); ok {
				if s, ok := p.(string); ok {
					ph = s
				} else if isTextValue(p) {
					ph = plainText(r.resolve(p, path+"/sp", env))
				}
			}
			env.page.search = map[string]interface{}{"v": describe(bv.Get()), "ph": ph,
				"id": r.on(path+"#search", func(x interface{}) { s, _ := x.(string); bv.Set(s) })}
		}
		return nodes
	case "navigationDestination":
		if env.nav != nil {
			if f, ok := argNamed(a, "for"); ok {
				tn := ""
				if tv, ok := f.(*TypeV); ok {
					tn = tv.Name
				}
				var fn Value
				for _, x := range a {
					if x.Label == "destination" || (x.Trailing && x.Label == "") {
						fn = x.V
					}
				}
				env.nav.dests = append(env.nav.dests, destReg{typ: tn, fn: fn})
			} else if b, ok := argNamed(a, "isPresented"); ok {
				var fn Value
				for _, x := range a {
					if x.Label == "destination" || (x.Trailing && x.Label == "") {
						fn = x.V
					}
				}
				env.nav.presented = append(env.nav.presented, presentedDest{b: b.(*BindingV), fn: fn})
			} else if b, ok := argNamed(a, "item"); ok {
				var fn Value
				for _, x := range a {
					if x.Label == "destination" || (x.Trailing && x.Label == "") {
						fn = x.V
					}
				}
				env.nav.presented = append(env.nav.presented, presentedDest{b: b.(*BindingV), fn: fn, item: true})
			}
		}
		return r.resolve(m.Base, path, env)
	case "sheet", "fullScreenCover", "popover":
		nodes := r.resolve(m.Base, path, env)
		r.presentSheet(m, path, env)
		return nodes
	case "alert", "confirmationDialog", "actionSheet":
		nodes := r.resolve(m.Base, path, env)
		r.presentAlert(m, path, env)
		return nodes
	case "photosPicker":
		nodes := r.resolve(m.Base, path, env)
		r.presentPhotosPicker(m, path, env)
		return nodes
	case "presentationDetents":
		if env.sheet != nil {
			if arr, ok := a0.(*ArrayV); ok {
				var dets []interface{}
				for _, d := range arr.E {
					if s, ok := d.(*SymV); ok {
						switch s.Name() {
						case "medium", "large":
							dets = append(dets, s.Name())
						case "height":
							f, _ := toFloat(argAt(s.Args(), 0))
							dets = append(dets, f)
						case "fraction":
							f, _ := toFloat(argAt(s.Args(), 0))
							dets = append(dets, map[string]interface{}{"frac": f})
						}
					}
				}
				env.sheet.detents = dets
			} else if st, ok := a0.(*SetV); ok {
				var dets []interface{}
				for _, d := range st.Keys {
					dets = append(dets, symName(d))
				}
				env.sheet.detents = dets
			}
		}
		return r.resolve(m.Base, path, env)
	case "presentationBackground":
		if env.sheet != nil {
			env.sheet.bg = r.paint(a0)
		}
		return r.resolve(m.Base, path, env)
	case "onAppear", "onDisappear", "task":
		var fn Value
		for _, x := range a {
			if _, ok := x.V.(*FuncV); ok {
				fn = x.V
			}
		}
		key := path + "#" + name + "@" + posKey(m.Pos)
		if name == "task" {
			if id, ok := argNamed(a, "id"); ok {
				key += "=" + hashKey(id)
			}
		}
		if fn != nil && !env.hidden {
			switch name {
			case "onAppear":
				r.appear[key] = fn
				r.appearOrder = append(r.appearOrder, key)
			case "task":
				r.tasks[key] = fn
			case "onDisappear":
				r.disappear[key] = fn
			}
		}
		return r.resolve(m.Base, path, env)
	case "onChange":
		var of Value
		if v, ok := argNamed(a, "of"); ok {
			of = v
		} else {
			of = a0
		}
		var fn Value
		for _, x := range a {
			if x.Trailing || x.Label == "perform" || x.Label == "action" {
				fn = x.V
			}
		}
		initial := false
		if v, ok := argNamed(a, "initial"); ok {
			initial = v == true
		}
		if !env.hidden {
			r.changes = append(r.changes, changeReg{key: path + "#change@" + posKey(m.Pos), val: copyValue(of), fn: fn, initial: initial})
		}
		return r.resolve(m.Base, path, env)
	case "onReceive":
		if p, ok := a0.(*PublisherV); ok && !env.hidden {
			var fn Value
			for _, x := range a {
				if x.Trailing || x.Label == "perform" {
					fn = x.V
				}
			}
			r.receivers[path+"#recv@"+posKey(m.Pos)] = &recvReg{interval: p.Interval, fn: fn}
		}
		return r.resolve(m.Base, path, env)
	case "onDelete", "onMove":
		nodes := r.resolve(m.Base, path, env)
		var fn Value
		for _, x := range a {
			if x.Label == "perform" || x.Trailing || x.Label == "" {
				fn = x.V
			}
		}
		if fn == nil || name == "onMove" {
			return nodes
		}
		for _, n := range nodes {
			i, ok := n.P["_i"].(int)
			if !ok {
				continue
			}
			idx := i
			f := fn
			n.set("del", r.on(n.ID+"#del", func(interface{}) {
				set := newSet()
				set.Add(idx)
				ip.callValue(f, []ArgV{{V: set}}, m.Pos, nil)
			}))
		}
		return nodes
	case "swipeActions", "contextMenu":
		nodes := r.resolve(m.Base, path, env)
		var fn Value
		for _, x := range a {
			if x.Label == "content" || x.Label == "menuItems" || (x.Trailing && x.Label == "") {
				fn = x.V
			}
		}
		if fn == nil {
			return nodes
		}
		e := env.clone()
		e.ctx = "menu"
		e.inMenu = true
		key := "swipe"
		if name == "contextMenu" {
			key = "ctxm"
		}
		items := r.resolve(r.callBuilder(fn), path+"/"+key, e)
		for _, n := range nodes {
			n.set(key, items)
			if name == "swipeActions" {
				if ed, ok := argNamed(a, "edge"); ok && symName(ed) == "leading" {
					n.set("swipeL", true)
				}
			}
		}
		return nodes
	case "fill", "stroke", "strokeBorder", "trim", "inset", "foregroundColor", "foregroundStyle":
		nodes := r.resolve(m.Base, path, env)
		if len(nodes) == 1 && nodes[0].T == "shape" && len(nodes[0].M) == 0 {
			sh := nodes[0]
			switch name {
			case "fill", "foregroundColor", "foregroundStyle":
				if len(a) == 0 {
					sh.P["fill"] = "currentColor"
				} else {
					sh.P["fill"] = r.paint(a0)
				}
				if name != "fill" {
					sh.M = append(sh.M, RMod{N: "fg", A: map[string]interface{}{"c": r.paint(a0)}})
				}
			case "stroke", "strokeBorder":
				var c Value
				lw := 1.0
				for _, x := range a {
					switch x.Label {
					case "":
						if f, ok := x.V.(float64); ok {
							lw = f
						} else if i, ok := x.V.(int); ok {
							lw = float64(i)
						} else if sv, ok := x.V.(*ViewV); ok && sv.Kind == "StrokeStyle" {
							for _, sa := range sv.Args {
								switch sa.Label {
								case "lineWidth":
									lw, _ = toFloat(sa.V)
								case "lineCap":
									sh.P["cap"] = symName(sa.V)
								case "dash":
									if arr, ok := sa.V.(*ArrayV); ok {
										var ds []float64
										for _, d := range arr.E {
											f, _ := toFloat(d)
											ds = append(ds, f)
										}
										sh.P["dash"] = ds
									}
								}
							}
						} else {
							c = x.V
						}
					case "lineWidth":
						lw, _ = toFloat(x.V)
					case "style":
						if sv, ok := x.V.(*ViewV); ok {
							for _, sa := range sv.Args {
								switch sa.Label {
								case "lineWidth":
									lw, _ = toFloat(sa.V)
								case "lineCap":
									sh.P["cap"] = symName(sa.V)
								case "dash":
									if arr, ok := sa.V.(*ArrayV); ok {
										var ds []float64
										for _, d := range arr.E {
											f, _ := toFloat(d)
											ds = append(ds, f)
										}
										sh.P["dash"] = ds
									}
								}
							}
						}
					}
				}
				if c != nil {
					sh.P["stroke"] = r.paint(c)
				} else {
					sh.P["stroke"] = "currentColor"
				}
				sh.P["lw"] = lw
				if name == "strokeBorder" {
					sh.P["inner"] = true
				}
			case "trim":
				from, to := 0.0, 1.0
				if f, ok := argNamed(a, "from"); ok {
					from, _ = toFloat(f)
				}
				if t, ok := argNamed(a, "to"); ok {
					to, _ = toFloat(t)
				}
				sh.P["trim"] = []float64{from, to}
			case "inset":
				f, _ := toFloat(argNamed2(a, "by"))
				sh.P["inset"] = f
			}
			return nodes
		}
		if name == "fill" || name == "stroke" || name == "strokeBorder" || name == "trim" || name == "inset" {
			if len(nodes) == 1 && nodes[0].T == "shape" {
				return nodes
			}
			r.fail(m.Pos, "value of type 'some View' has no member '%s' (only shapes like Circle() support .%s)", name, name)
		}
		mod := RMod{N: "fg", A: map[string]interface{}{"c": r.paint(a0)}}
		for _, n := range nodes {
			n.M = append(n.M, mod)
		}
		return nodes
	case "resizable", "renderingMode", "symbolRenderingMode", "symbolVariant":
		nodes := r.resolve(m.Base, path, env)
		for _, n := range nodes {
			if n.T == "image" && len(n.M) == 0 {
				switch name {
				case "resizable":
					n.set("rz", true)
				case "symbolRenderingMode":
					n.set("srm", symName(a0))
				case "symbolVariant":
					n.set("var", symName(a0))
				}
			} else if name == "resizable" {
				r.fail(m.Pos, "'resizable()' is only available on Image")
			}
		}
		return nodes
	}
	if noopModifiers[name] {
		return r.resolve(m.Base, path, env)
	}
	if !knownModifiers[name] {
		r.fail(m.Pos, "value of type 'some View' has no member '%s'", name)
	}
	nodes := r.resolve(m.Base, path, env)
	mod, ok := r.visualMod(m, path, env, nodes)
	if !ok {
		return nodes
	}
	for _, n := range nodes {
		n.M = append(n.M, mod)
	}
	return nodes
}

func argNamed2(a []ArgV, l string) Value {
	v, _ := argNamed(a, l)
	if v == nil && len(a) > 0 {
		return a[0].V
	}
	return v
}

func (r *Render) toolbar(m *ModV, path string, env *REnv) {
	pc := env.page
	if pc == nil {
		return
	}
	var fn Value
	for _, x := range m.Args {
		if x.Label == "content" || (x.Trailing && x.Label == "") {
			fn = x.V
		}
	}
	if fn == nil {
		return
	}
	items := r.flattenValues(r.callBuilder(fn))
	for i, item := range items {
		placement := "automatic"
		val := item
		if vv, ok := item.(*ViewV); ok && (vv.Kind == "ToolbarItem" || vv.Kind == "ToolbarItemGroup") {
			if p, ok := vv.arg("placement"); ok {
				placement = symName(p)
			}
			val = r.callBuilder(vv.closure("content"))
		}
		e := env.clone()
		e.page = nil
		e.inToolbar = true
		e.ctx = "toolbar"
		nodes := r.resolve(val, path+"/tb"+strconv.Itoa(i), e)
		switch placement {
		case "navigationBarLeading", "topBarLeading", "cancellationAction", "navigation":
			pc.lead = append(pc.lead, nodes...)
		case "principal":
			pc.prin = append(pc.prin, nodes...)
		case "bottomBar", "bottomOrnament", "status":
			pc.bottom = append(pc.bottom, nodes...)
		case "keyboard":
		default:
			pc.trail = append(pc.trail, nodes...)
		}
	}
}

func (r *Render) presentEnv(env *REnv) *REnv {
	e := env.clone()
	e.nav, e.page, e.inList, e.inForm, e.hidden, e.inMenu, e.inToolbar, e.ctx = nil, nil, false, false, false, false, false, ""
	e.disabled = false
	return e
}

func (r *Render) presentSheet(m *ModV, path string, env *REnv) {
	ip := r.ip
	a := m.Args
	shown := false
	var item Value
	var dismiss func()
	if b, ok := argNamed(a, "isPresented"); ok {
		bv := b.(*BindingV)
		shown = bv.Get() == true
		dismiss = func() { bv.Set(false) }
	} else if b, ok := argNamed(a, "item"); ok {
		bv := b.(*BindingV)
		item = bv.Get()
		shown = item != nil
		dismiss = func() { bv.Set(nil) }
	}
	if !shown || env.hidden {
		return
	}
	var content Value
	for _, x := range a {
		if x.Label == "content" || (x.Trailing && x.Label == "") {
			content = x.V
		}
	}
	onDismiss, _ := argNamed(a, "onDismiss")
	full := dismiss
	if onDismiss != nil {
		full = func() {
			dismiss()
			ip.callValue(onDismiss, nil, m.Pos, nil)
		}
	}
	sc := &SheetCtx{}
	e := r.presentEnv(env)
	e.sheet = sc
	e = e.withEnvVal("dismiss", &NativeV{Name: "dismiss", Fn: func(c *CallCtx) Value { full(); return Void }})
	key := path + "#sheet@" + posKey(m.Pos)
	var val Value
	if item != nil {
		val = r.callBuilder(content, item)
	} else {
		val = r.callBuilder(content)
	}
	disID := r.on(key+"#dismiss", func(interface{}) { full() })
	nodes := r.resolve(val, key, e)
	n := &RNode{T: "sheet", ID: key, P: map[string]interface{}{"full": m.Name == "fullScreenCover", "dis": disID}, C: nodes}
	if len(sc.detents) > 0 {
		n.P["det"] = sc.detents
	}
	if sc.bg != nil {
		n.P["bg"] = sc.bg
	}
	r.overlays = append(r.overlays, n)
}

func (r *Render) presentAlert(m *ModV, path string, env *REnv) {
	ip := r.ip
	a := m.Args
	var bv *BindingV
	var data Value
	usingItem := false
	if b, ok := argNamed(a, "isPresented"); ok {
		bv = b.(*BindingV)
		if bv.Get() != true {
			return
		}
	} else if b, ok := argNamed(a, "item"); ok {
		bv = b.(*BindingV)
		data = bv.Get()
		usingItem = true
		if data == nil {
			return
		}
	} else {
		return
	}
	if env.hidden {
		return
	}
	if d, ok := argNamed(a, "presenting"); ok {
		data = d
	}
	dismiss := func() {
		if usingItem {
			bv.Set(nil)
		} else {
			bv.Set(false)
		}
	}
	key := path + "#alert@" + posKey(m.Pos)
	title := ""
	for _, x := range a {
		if x.Label == "" && !x.Trailing {
			if s, ok := x.V.(string); ok {
				title = s
			} else if isTextValue(x.V) {
				title = plainText(r.resolve(x.V, key+"/t", env))
			}
		}
		if x.Label == "title" {
			title = describe(x.V)
		}
	}
	callWith := func(f Value) Value {
		if f == nil {
			return nil
		}
		if data != nil {
			if fv, ok := f.(*FuncV); ok && (len(fv.Params) > 0 || !fv.HasSig) {
				return r.callBuilder(f, data)
			}
		}
		return r.callBuilder(f)
	}
	var actions, message Value
	for _, x := range a {
		switch x.Label {
		case "actions":
			actions = x.V
		case "message":
			message = x.V
		case "content":
			actions = x.V
		case "":
			if x.Trailing {
				if actions == nil {
					actions = x.V
				}
			}
		}
	}
	style := "alert"
	if m.Name != "alert" {
		style = "sheet"
	}
	var btns []interface{}
	msg := ""
	av := callWith(actions)
	if lv, ok := av.(*ViewV); ok && (lv.Kind == "Alert" || lv.Kind == "ActionSheet") {
		for _, la := range lv.Args {
			switch la.Label {
			case "title":
				title = plainText(r.resolve(la.V, key+"/t", env))
			case "message":
				msg = plainText(r.resolve(la.V, key+"/m", env))
			case "dismissButton", "primaryButton", "secondaryButton":
				btns = append(btns, r.legacyAlertButton(la.V, key+"/"+la.Label, dismiss))
			case "buttons":
				if arr, ok := la.V.(*ArrayV); ok {
					for i, b := range arr.E {
						btns = append(btns, r.legacyAlertButton(b, key+"/b"+strconv.Itoa(i), dismiss))
					}
				}
			}
		}
		if len(btns) == 0 {
			btns = append(btns, map[string]interface{}{"t": "OK", "id": r.on(key+"#ok", func(interface{}) { dismiss() })})
		}
	} else {
		e := r.presentEnv(env)
		e.inAlert = true
		e.ctx = "alert"
		nodes := r.resolve(av, key+"/a", e)
		var walk func(ns []*RNode)
		walk = func(ns []*RNode) {
			for _, n := range ns {
				if n.T == "button" || n.T == "tap" {
					orig, _ := n.P["id"].(string)
					role, _ := n.P["role"].(string)
					id := r.on(n.ID+"#alertbtn", func(x interface{}) {
						if h, ok := r.handlers[orig]; ok {
							h(x)
						}
						dismiss()
					})
					btns = append(btns, map[string]interface{}{"t": plainText(n.C), "role": role, "id": id})
				} else if n.T == "textfield" {
					btns = append(btns, map[string]interface{}{"tf": n})
				} else {
					walk(n.C)
				}
			}
		}
		walk(nodes)
		hasCancel := false
		for _, b := range btns {
			if bm, ok := b.(map[string]interface{}); ok && bm["role"] == "cancel" {
				hasCancel = true
			}
		}
		if len(btns) == 0 {
			btns = append(btns, map[string]interface{}{"t": "OK", "id": r.on(key+"#ok", func(interface{}) { dismiss() })})
		} else if style == "sheet" && !hasCancel {
			btns = append(btns, map[string]interface{}{"t": "Cancel", "role": "cancel", "id": r.on(key+"#cancel", func(interface{}) { dismiss() })})
		}
		if message != nil {
			msg = plainText(r.resolve(callWith(message), key+"/m", e))
		}
	}
	if style == "sheet" {
		if tv, ok := argNamed(a, "titleVisibility"); !ok || symName(tv) != "visible" {
			if m.Name == "confirmationDialog" {
				title = ""
			}
		}
	}
	_ = ip
	r.overlays = append(r.overlays, &RNode{T: "alert", ID: key, P: map[string]interface{}{"title": title, "msg": msg, "btns": btns, "style": style,
		"dis": r.on(key+"#dismiss", func(interface{}) { dismiss() })}})
}

func (r *Render) legacyAlertButton(v Value, key string, dismiss func()) interface{} {
	s, ok := v.(*SymV)
	if !ok {
		return map[string]interface{}{"t": "OK", "id": r.on(key, func(interface{}) { dismiss() })}
	}
	role := ""
	switch s.Name() {
	case "cancel":
		role = "cancel"
	case "destructive":
		role = "destructive"
	}
	label := "OK"
	if role == "cancel" {
		label = "Cancel"
	}
	var action Value
	for _, a := range s.Args() {
		if a.Label == "" && isTextValue(a.V) {
			label = plainText(r.resolve(a.V, key+"/l", &REnv{}))
		}
		if a.Label == "action" || a.Trailing {
			action = a.V
		}
	}
	return map[string]interface{}{"t": label, "role": role, "id": r.on(key, func(interface{}) {
		if action != nil {
			r.ip.callValue(action, nil, Pos{}, nil)
		}
		dismiss()
	})}
}

// ---------- visual modifiers ----------

func edgeSet(v Value) map[string]bool {
	out := map[string]bool{}
	add := func(n string) {
		switch n {
		case "all":
			out["t"], out["l"], out["b"], out["r"] = true, true, true, true
		case "horizontal":
			out["l"], out["r"] = true, true
		case "vertical":
			out["t"], out["b"] = true, true
		case "top":
			out["t"] = true
		case "bottom":
			out["b"] = true
		case "leading":
			out["l"] = true
		case "trailing":
			out["r"] = true
		}
	}
	switch x := v.(type) {
	case *SymV:
		add(x.Name())
	case *ArrayV:
		for _, e := range x.E {
			add(symName(e))
		}
	case *SetV:
		for _, e := range x.Keys {
			add(symName(e))
		}
	}
	return out
}

func numOrInf(v Value) interface{} {
	switch x := v.(type) {
	case *SymV:
		if x.Name() == "infinity" {
			return "inf"
		}
	case float64:
		if math.IsInf(x, 1) {
			return "inf"
		}
		return x
	case int:
		return float64(x)
	}
	return nil
}

func (r *Render) viewArgNodes(v Value, path string, env *REnv) []*RNode {
	if f, ok := v.(*FuncV); ok {
		v = r.callBuilder(f)
	}
	return r.resolve(v, path, env)
}

func (r *Render) visualMod(m *ModV, path string, env *REnv, base []*RNode) (RMod, bool) {
	a := m.Args
	a0 := argAt(a, 0)
	A := map[string]interface{}{}
	name := m.Name
	f0 := func() float64 { f, _ := toFloat(a0); return f }
	switch name {
	case "padding", "safeAreaPadding", "contentMargins":
		amt := 16.0
		edges := map[string]bool{"t": true, "l": true, "b": true, "r": true}
		insets := false
		for _, x := range a {
			switch v := x.V.(type) {
			case int, float64:
				amt, _ = toFloat(v)
			case *SymV, *ArrayV, *SetV:
				edges = edgeSet(v)
			case *ViewV:
				if v.Kind == "EdgeInsets" {
					insets = true
					for _, ia := range v.Args {
						f, _ := toFloat(ia.V)
						switch ia.Label {
						case "top":
							A["t"] = f
						case "leading":
							A["l"] = f
						case "bottom":
							A["b"] = f
						case "trailing":
							A["r"] = f
						}
					}
				}
			case *NativeObj:
				insets = true
				for k, pv := range v.Props {
					f, _ := toFloat(pv)
					A[map[string]string{"top": "t", "leading": "l", "bottom": "b", "trailing": "r"}[k]] = f
				}
			}
		}
		if !insets {
			for _, e := range []string{"t", "l", "b", "r"} {
				if edges[e] {
					A[e] = amt
				} else {
					A[e] = 0.0
				}
			}
		}
		return RMod{N: "pad", A: A}, true
	case "frame", "containerRelativeFrame":
		if name == "containerRelativeFrame" {
			A["maxW"] = "inf"
			return RMod{N: "frame", A: A}, true
		}
		for _, x := range a {
			key := map[string]string{"width": "w", "height": "h", "minWidth": "minW", "maxWidth": "maxW", "minHeight": "minH", "maxHeight": "maxH", "idealWidth": "iw", "idealHeight": "ih"}[x.Label]
			if key != "" {
				if v := numOrInf(x.V); v != nil {
					A[key] = v
				}
			}
			if x.Label == "alignment" {
				A["al"] = symName(x.V)
			}
		}
		return RMod{N: "frame", A: A}, true
	case "background", "overlay", "listRowBackground", "safeAreaInset", "mask":
		if name == "safeAreaInset" {
			edge, _ := argNamed(a, "edge")
			A["edge"] = symName(edge)
			for _, x := range a {
				if x.Label == "content" || (x.Trailing && x.Label == "") {
					A["node"] = r.viewArgNodes(x.V, path+"/sai", env)
				}
				if x.Label == "spacing" {
					A["sp"], _ = toFloat(x.V)
				}
			}
			return RMod{N: "sai", A: A}, true
		}
		for _, x := range a {
			switch x.Label {
			case "alignment":
				A["al"] = symName(x.V)
			case "in":
				A["shape"] = r.shapeSpec(x.V)
			case "ignoresSafeAreaEdges":
			case "content":
				A["node"] = r.viewArgNodes(x.V, path+"/"+name, env)
			case "", "style":
				if x.Trailing {
					A["node"] = r.viewArgNodes(x.V, path+"/"+name, env)
				} else if isPaintValue(x.V) && !(isShapeView(x.V)) {
					if p := r.paint(x.V); p != nil {
						A["paint"] = p
					} else {
						A["node"] = r.viewArgNodes(x.V, path+"/"+name, env)
					}
				} else if x.V != nil {
					A["node"] = r.viewArgNodes(x.V, path+"/"+name, env)
				}
			}
		}
		if len(a) == 0 && name == "background" {
			A["paint"] = "var(--c-systemBackground)"
		}
		n := map[string]string{"background": "bg", "overlay": "ov", "listRowBackground": "rowbg", "mask": "mask"}[name]
		return RMod{N: n, A: A}, true
	case "tint", "accentColor":
		if a0 == nil {
			return RMod{}, false
		}
		A["c"] = r.paint(a0)
		return RMod{N: "tint", A: A}, true
	case "font":
		var fv *FontV
		switch x := a0.(type) {
		case *FontV:
			fv = x
		case *SymV:
			if v, ok := r.ip.resolveSymAs("Font", x).(*FontV); ok {
				fv = v
			}
		}
		if fv == nil {
			return RMod{}, false
		}
		A["f"] = map[string]interface{}{"st": fv.Style, "sz": fv.Size, "w": fv.Weight, "d": fv.Design, "i": fv.Italic, "mono": fv.Mono, "fam": fv.Name}
		return RMod{N: "font", A: A}, true
	case "fontWeight":
		A["w"] = symName(a0)
		return RMod{N: "fw", A: A}, true
	case "bold":
		if len(a) > 0 && a0 == false {
			return RMod{}, false
		}
		A["w"] = "bold"
		return RMod{N: "fw", A: A}, true
	case "italic":
		if len(a) > 0 && a0 == false {
			return RMod{}, false
		}
		return RMod{N: "italic"}, true
	case "fontDesign":
		A["d"] = symName(a0)
		return RMod{N: "fdesign", A: A}, true
	case "fontWidth":
		return RMod{}, false
	case "monospaced":
		return RMod{N: "fdesign", A: map[string]interface{}{"d": "monospaced"}}, true
	case "monospacedDigit":
		return RMod{N: "tabnum"}, true
	case "underline", "strikethrough":
		if len(a) > 0 && a0 == false {
			return RMod{}, false
		}
		if c, ok := argNamed(a, "color"); ok {
			A["c"] = r.paint(c)
		}
		return RMod{N: name, A: A}, true
	case "kerning", "tracking":
		A["v"] = f0()
		return RMod{N: "kern", A: A}, true
	case "textCase":
		A["v"] = symName(a0)
		return RMod{N: "textcase", A: A}, true
	case "lineLimit":
		switch x := a0.(type) {
		case int:
			A["n"] = x
		case *RangeV:
			A["n"], _ = toInt(x.Hi)
			A["min"], _ = toInt(x.Lo)
		case nil:
			A["n"] = nil
		}
		return RMod{N: "linelimit", A: A}, true
	case "lineSpacing":
		A["v"] = f0()
		return RMod{N: "linespacing", A: A}, true
	case "multilineTextAlignment":
		A["al"] = symName(a0)
		return RMod{N: "talign", A: A}, true
	case "truncationMode", "minimumScaleFactor", "allowsTightening":
		return RMod{}, false
	case "cornerRadius":
		A["r"] = f0()
		return RMod{N: "radius", A: A}, true
	case "clipShape", "containerShape":
		A["shape"] = r.shapeSpec(a0)
		return RMod{N: "clip", A: A}, true
	case "clipped":
		return RMod{N: "clipped"}, true
	case "shadow":
		A["r"], A["x"], A["y"] = 0.0, 0.0, 0.0
		A["c"] = "rgba(0,0,0,0.33)"
		for _, x := range a {
			switch x.Label {
			case "color":
				A["c"] = r.paint(x.V)
			case "radius":
				A["r"], _ = toFloat(x.V)
			case "x":
				A["x"], _ = toFloat(x.V)
			case "y":
				A["y"], _ = toFloat(x.V)
			}
		}
		return RMod{N: "shadow", A: A}, true
	case "opacity":
		A["v"] = f0()
		return RMod{N: "opacity", A: A}, true
	case "scaleEffect":
		sx, sy := 1.0, 1.0
		for _, x := range a {
			switch x.Label {
			case "":
				switch v := x.V.(type) {
				case *SizeV:
					sx, sy = v.W, v.H
				default:
					f, _ := toFloat(v)
					sx, sy = f, f
				}
			case "x":
				sx, _ = toFloat(x.V)
			case "y":
				sy, _ = toFloat(x.V)
			case "anchor":
				A["anchor"] = unitPoint(x.V)
			}
		}
		A["x"], A["y"] = sx, sy
		return RMod{N: "scale", A: A}, true
	case "rotationEffect":
		A["deg"] = r.angleDeg(a0)
		if an, ok := argNamed(a, "anchor"); ok {
			A["anchor"] = unitPoint(an)
		}
		return RMod{N: "rotate", A: A}, true
	case "rotation3DEffect":
		A["deg"] = r.angleDeg(a0)
		if ax, ok := argNamed(a, "axis"); ok {
			if t, ok := ax.(*TupleV); ok && len(t.E) == 3 {
				x, _ := toFloat(t.E[0])
				y, _ := toFloat(t.E[1])
				z, _ := toFloat(t.E[2])
				A["ax"] = []float64{x, y, z}
			}
		}
		return RMod{N: "rotate3d", A: A}, true
	case "offset", "position":
		x, y := 0.0, 0.0
		for _, arg := range a {
			switch arg.Label {
			case "x":
				x, _ = toFloat(arg.V)
			case "y":
				y, _ = toFloat(arg.V)
			case "":
				switch v := arg.V.(type) {
				case *SizeV:
					x, y = v.W, v.H
				case *PointV:
					x, y = v.X, v.Y
				}
			}
		}
		A["x"], A["y"] = x, y
		if name == "position" {
			return RMod{N: "position", A: A}, true
		}
		return RMod{N: "offset", A: A}, true
	case "blur":
		if rad, ok := argNamed(a, "radius"); ok {
			A["r"], _ = toFloat(rad)
		} else {
			A["r"] = f0()
		}
		return RMod{N: "blur", A: A}, true
	case "brightness", "contrast", "saturation", "grayscale", "hueRotation", "colorInvert", "colorMultiply", "blendMode":
		if name == "hueRotation" {
			A["v"] = r.angleDeg(a0)
		} else if name == "colorMultiply" {
			A["c"] = r.paint(a0)
		} else if name == "blendMode" {
			A["v"] = symName(a0)
		} else {
			A["v"] = f0()
		}
		return RMod{N: name, A: A}, true
	case "border":
		A["c"] = r.paint(a0)
		A["w"] = 1.0
		if w, ok := argNamed(a, "width"); ok {
			A["w"], _ = toFloat(w)
		}
		return RMod{N: "border", A: A}, true
	case "zIndex":
		A["v"] = f0()
		return RMod{N: "zindex", A: A}, true
	case "layoutPriority":
		A["v"] = f0()
		return RMod{N: "priority", A: A}, true
	case "fixedSize":
		h, vv := true, true
		if x, ok := argNamed(a, "horizontal"); ok {
			h = x == true
		}
		if x, ok := argNamed(a, "vertical"); ok {
			vv = x == true
		}
		A["h"], A["v"] = h, vv
		return RMod{N: "fixed", A: A}, true
	case "aspectRatio", "scaledToFit", "scaledToFill":
		mode := "fit"
		if name == "scaledToFill" {
			mode = "fill"
		}
		for _, x := range a {
			switch x.Label {
			case "":
				if f, ok := toFloat(x.V); ok {
					A["r"] = f
				} else if sz, ok := x.V.(*SizeV); ok && sz.H != 0 {
					A["r"] = sz.W / sz.H
				}
			case "contentMode":
				mode = symName(x.V)
			}
		}
		A["mode"] = mode
		if len(base) == 1 && base[0].T == "image" && len(base[0].M) == 0 {
			base[0].set("fit", mode)
			if rr, ok := A["r"]; ok {
				base[0].set("ar", rr)
			}
			return RMod{}, false
		}
		return RMod{N: "aspect", A: A}, true
	case "ignoresSafeArea", "edgesIgnoringSafeArea":
		es := map[string]bool{"t": true, "b": true, "l": true, "r": true}
		for _, x := range a {
			if x.Label == "edges" || (x.Label == "" && name == "edgesIgnoringSafeArea") {
				es = edgeSet(x.V)
			}
		}
		A["t"], A["b"] = es["t"], es["b"]
		return RMod{N: "isa", A: A}, true
	case "hidden":
		return RMod{N: "hidden"}, true
	case "allowsHitTesting", "scrollDisabled":
		A["v"] = a0 == true
		if name == "scrollDisabled" {
			A["v"] = !(a0 == true)
		}
		return RMod{N: "hittest", A: A}, true
	case "onTapGesture":
		var fn Value
		count := 1
		for _, x := range a {
			if _, ok := x.V.(*FuncV); ok {
				fn = x.V
			}
			if x.Label == "count" {
				count, _ = toInt(x.V)
			}
		}
		if fn == nil || env.disabled {
			return RMod{}, false
		}
		f := fn
		ip := r.ip
		A["id"] = r.on(path+"#tapg@"+posKey(m.Pos), func(interface{}) {
			if fv, ok := f.(*FuncV); ok && fv.HasSig && len(fv.Params) == 1 {
				ip.callValue(f, []ArgV{{V: &PointV{}}}, m.Pos, nil)
				return
			}
			ip.callValue(f, nil, m.Pos, nil)
		})
		A["count"] = count
		return RMod{N: "tap", A: A}, true
	case "gesture", "simultaneousGesture", "highPriorityGesture":
		if g, ok := a0.(*NativeObj); ok && g.Kind == "Gesture" {
			if f, ok := g.Props["ended"].(*FuncV); ok && (g.Props["kind"] == "TapGesture" || g.Props["kind"] == "SpatialTapGesture" || g.Props["kind"] == "LongPressGesture") {
				fn := f
				id := r.on(path+"#gesture@"+posKey(m.Pos), func(interface{}) {
					if fn.HasSig && len(fn.Params) == 1 {
						r.ip.callValue(fn, []ArgV{{V: true}}, m.Pos, nil)
						return
					}
					r.ip.callValue(fn, nil, m.Pos, nil)
				})
				if g.Props["kind"] == "LongPressGesture" {
					return RMod{N: "longpress", A: map[string]interface{}{"id": id}}, true
				}
				return RMod{N: "tap", A: map[string]interface{}{"id": id, "count": 1}}, true
			}
		}
		return RMod{}, false
	case "onLongPressGesture":
		var fn Value
		for _, x := range a {
			if x.Label == "perform" || x.Trailing {
				if _, ok := x.V.(*FuncV); ok && fn == nil {
					fn = x.V
				}
			}
		}
		if fn == nil {
			return RMod{}, false
		}
		f := fn
		A["id"] = r.on(path+"#lp@"+posKey(m.Pos), func(interface{}) { r.ip.callValue(f, nil, m.Pos, nil) })
		return RMod{N: "longpress", A: A}, true
	case "animation":
		dur := 0.35
		if s, ok := a0.(*SymV); ok {
			for _, p := range s.Parts {
				for _, sa := range p.Args {
					if sa.Label == "duration" || sa.Label == "response" {
						dur, _ = toFloat(sa.V)
					}
				}
			}
			if s.Name() == "linear" || s.Name() == "easeIn" || s.Name() == "easeOut" || s.Name() == "easeInOut" {
				A["ease"] = s.Name()
			}
		}
		if a0 == nil && len(a) > 0 {
			return RMod{}, false
		}
		A["dur"] = dur
		return RMod{N: "anim", A: A}, true
	case "transition":
		k := symName(a0)
		if s, ok := a0.(*SymV); ok && s.Name() == "move" && len(s.Args()) > 0 {
			k = "move-" + symName(s.Args()[0].V)
		}
		A["k"] = k
		return RMod{N: "transition", A: A}, true
	case "listRowInsets":
		if ev, ok := a0.(*ViewV); ok && ev.Kind == "EdgeInsets" {
			for _, ia := range ev.Args {
				f, _ := toFloat(ia.V)
				A[map[string]string{"top": "t", "leading": "l", "bottom": "b", "trailing": "r"}[ia.Label]] = f
			}
		} else if a0 == nil || len(a) == 0 {
			A["t"], A["l"], A["b"], A["r"] = 0.0, 0.0, 0.0, 0.0
		}
		return RMod{N: "rowinsets", A: A}, true
	case "listRowSeparator", "listSectionSeparator":
		A["hidden"] = symName(a0) == "hidden"
		return RMod{N: "rowsep", A: A}, true
	case "listRowSeparatorTint", "listRowSpacing", "listSectionSpacing":
		return RMod{}, false
	case "scrollContentBackground":
		A["hidden"] = symName(a0) == "hidden"
		return RMod{N: "scrollbg", A: A}, true
	case "scrollIndicators":
		A["hidden"] = symName(a0) == "hidden" || symName(a0) == "never"
		return RMod{N: "scrollind", A: A}, true
	case "imageScale":
		A["s"] = symName(a0)
		return RMod{N: "imgscale", A: A}, true
	case "badge":
		if a0 == nil {
			return RMod{}, false
		}
		if i, ok := a0.(int); ok && i == 0 {
			return RMod{}, false
		}
		A["v"] = describe(a0)
		if isTextValue(a0) {
			A["v"] = plainText(r.resolve(a0, path+"/badge", env))
		}
		return RMod{N: "badge", A: A}, true
	case "foregroundColor", "foregroundStyle":
		A["c"] = r.paint(a0)
		return RMod{N: "fg", A: A}, true
	case "symbolRenderingMode", "symbolVariant", "renderingMode", "imageScaleCompat":
		return RMod{}, false
	case "lineHeight", "baselineOffset":
		return RMod{}, false
	case "backgroundStyle":
		A["paint"] = r.paint(a0)
		return RMod{N: "bg", A: A}, true
	}
	r.ip.warn(m.Pos, "modifier '.%s' is not rendered by the preview", name)
	return RMod{}, false
}

func isShapeView(v Value) bool {
	if x, ok := v.(*ViewV); ok {
		switch x.Kind {
		case "Rectangle", "Circle", "Ellipse", "Capsule", "RoundedRectangle", "UnevenRoundedRectangle":
			return true
		}
	}
	return false
}

// ---------- root ----------

type RenderResult struct {
	Root            *RNode   `json:"root"`
	Overlays        []*RNode `json:"overlays"`
	PreferredScheme string   `json:"scheme,omitempty"`
	HideTabBar      bool     `json:"hideTabBar,omitempty"`
}

func newRender(ip *Interp) *Render {
	return &Render{
		ip: ip, store: ip.store,
		handlers:   map[string]func(interface{}){},
		appear:     map[string]Value{},
		disappear:  map[string]Value{},
		tasks:      map[string]Value{},
		receivers:  map[string]*recvReg{},
		touched:    map[string]bool{},
		tagVals:    map[string]Value{},
		geoKeys:    map[string]bool{},
		navTouched: map[string]bool{},
		imageBlobs: map[string]imageBlob{},
	}
}

func (r *Render) gc() {
	for k := range r.store.state {
		if r.touched[k] {
			continue
		}
		keep := false
		for _, p := range r.keep {
			if strings.HasPrefix(k, p) {
				keep = true
				break
			}
		}
		if !keep {
			delete(r.store.state, k)
		}
	}
	for k := range r.store.nav {
		if r.navTouched[k] {
			continue
		}
		keep := false
		for _, p := range r.keep {
			if strings.HasPrefix(k, p) {
				keep = true
			}
		}
		if !keep {
			delete(r.store.nav, k)
		}
	}
	keys := make([]string, 0)
	for k := range r.store.changeVals {
		keys = append(keys, k)
	}
	sort.Strings(keys)
}
