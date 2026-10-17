package switaculous

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Store struct {
	state         map[string]*BoxV
	appStorage    map[string]*BoxV
	nav           map[string]*NavState
	geo           map[string]*GeoV
	changeVals    map[string]Value
	prevAppear    map[string]bool
	prevDisappear map[string]Value
	prevTasks     map[string]bool
	dark          bool
	device        string
	landscape     bool
	scrollTo      string
}

func newStore() *Store {
	return &Store{
		state:         map[string]*BoxV{},
		appStorage:    map[string]*BoxV{},
		nav:           map[string]*NavState{},
		geo:           map[string]*GeoV{},
		changeVals:    map[string]Value{},
		prevAppear:    map[string]bool{},
		prevDisappear: map[string]Value{},
		prevTasks:     map[string]bool{},
		device:        "iphone16",
	}
}

type Device struct {
	Name      string  `json:"name"`
	W         float64 `json:"w"`
	H         float64 `json:"h"`
	Top       float64 `json:"top"`
	Bottom    float64 `json:"bottom"`
	Island    bool    `json:"island"`
	Radius    float64 `json:"radius"`
	Kind      string  `json:"kind"`
	Landscape bool    `json:"landscape"`
}

var devices = map[string]Device{
	"iphone16":       {Name: "iPhone 16", W: 393, H: 852, Top: 59, Bottom: 34, Island: true, Radius: 55, Kind: "iphone"},
	"iphone16pro":    {Name: "iPhone 16 Pro", W: 402, H: 874, Top: 62, Bottom: 34, Island: true, Radius: 56, Kind: "iphone"},
	"iphone16promax": {Name: "iPhone 16 Pro Max", W: 440, H: 956, Top: 62, Bottom: 34, Island: true, Radius: 60, Kind: "iphone"},
	"iphonese":       {Name: "iPhone SE", W: 375, H: 667, Top: 20, Bottom: 0, Island: false, Radius: 0, Kind: "iphone"},
	"ipadmini":       {Name: "iPad mini", W: 744, H: 1133, Top: 24, Bottom: 20, Radius: 18, Kind: "ipad"},
	"ipad11":         {Name: "iPad 11\"", W: 820, H: 1180, Top: 24, Bottom: 20, Radius: 18, Kind: "ipad"},
	"ipadpro13":      {Name: "iPad Pro 13\"", W: 1032, H: 1376, Top: 24, Bottom: 20, Radius: 18, Kind: "ipad"},
}

// The device as currently shown: an iPad turned sideways swaps width and height.
func (st *Store) currentDevice() Device {
	d, ok := devices[st.device]
	if !ok {
		d = devices["iphone16"]
	}
	if st.landscape && d.Kind == "ipad" {
		d.W, d.H = d.H, d.W
		d.Landscape = true
	}
	return d
}

func (ip *Interp) device() Device {
	if ip.store != nil {
		return ip.store.currentDevice()
	}
	return devices["iphone16"]
}

func (ip *Interp) isPad() bool      { return ip.device().Kind == "ipad" }
func (ip *Interp) screenW() float64 { return ip.device().W }
func (ip *Interp) screenH() float64 { return ip.device().H }

type ErrInfo struct {
	Msg     string `json:"msg"`
	File    string `json:"file"`
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Col     int    `json:"col"`
	Snippet string `json:"snippet"`
	Kind    string `json:"kind"`
}

type ConsoleLine struct {
	T    string `json:"t"`
	Kind string `json:"k"`
	Msg  string `json:"m"`
	Path string `json:"path,omitempty"`
	Line int    `json:"line,omitempty"`
	Col  int    `json:"col,omitempty"`
}

type PreviewRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	File string `json:"file"`
}

type ticker struct {
	interval float64
	fn       Value
	stop     chan struct{}
}

type Runtime struct {
	mu           sync.Mutex
	dir          string
	store        *Store
	ip           *Interp
	gen          int
	files        map[string]string
	sources      map[string]string
	lastEdited   string
	pinned       string
	clients      map[chan []byte]bool
	cmu          sync.Mutex
	lastPayload  []byte
	errs         []ErrInfo
	console      []ConsoleLine
	warnings     map[string]bool
	warnList     []string
	anim         bool
	handlers     map[string]func(interface{})
	tickers      map[string]*ticker
	assets       map[string]string
	colorAssets  map[string]assetColor
	current      string
	previews     []PreviewRef
	renderCount  int
	previewCache map[string]Value
	lastResult   *RenderResult

	persistentContexts map[string]*ModelContextV
	userDefaultsSaved  string
	userDefaultsLoaded bool
	imageBlobs         map[string]imageBlob
	previousImageBlobs map[string]imageBlob
}

func NewRuntime(dir string) *Runtime {
	return &Runtime{
		dir:      dir,
		store:    newStore(),
		files:    map[string]string{},
		sources:  map[string]string{},
		clients:  map[chan []byte]bool{},
		warnings: map[string]bool{},
		tickers:  map[string]*ticker{},
		assets:   map[string]string{},
		pinned:   "auto",

		persistentContexts: map[string]*ModelContextV{},
	}
}

func (rt *Runtime) logf(kind, f string, a ...interface{}) {
	rt.appendConsole(ConsoleLine{Kind: kind, Msg: fmt.Sprintf(f, a...)})
}

// Error lines carry where the error is so the console can show the location
// and open the file at that line when clicked.
func (rt *Runtime) logError(prefix string, e ErrInfo) {
	line := ConsoleLine{Kind: "error", Msg: prefix + e.Msg, Path: e.Path, Line: e.Line, Col: e.Col}
	if e.File != "" && e.Line > 0 {
		line.Msg = fmt.Sprintf("%s%s:%d:%d: %s", prefix, e.File, e.Line, e.Col, e.Msg)
	}
	rt.appendConsole(line)
}

func (rt *Runtime) appendConsole(line ConsoleLine) {
	line.T = time.Now().Format("15:04:05")
	msg := line.Msg
	rt.console = append(rt.console, line)
	if len(rt.console) > 300 {
		rt.console = rt.console[len(rt.console)-300:]
	}
	if line.Kind == "print" {
		fmt.Fprintln(consoleOutput, msg)
	}
}

// ---------- file scanning ----------

func skipDir(name string) bool {
	return strings.HasPrefix(name, ".") || name == "build" || name == "DerivedData" || name == "node_modules" || name == "Pods" || name == "switaculous-web"
}

type assetColor struct {
	Light string `json:"light"`
	Dark  string `json:"dark"`
}

// Reads a Contents.json from an Assets.xcassets colour set. Components are
// written either as fractions ("0.345") or hex bytes ("0x58").
func readColorSet(folder string) (assetColor, bool) {
	content, err := os.ReadFile(filepath.Join(folder, "Contents.json"))
	if err != nil {
		return assetColor{}, false
	}
	var contents struct {
		Colors []struct {
			Appearances []struct {
				Appearance string `json:"appearance"`
				Value      string `json:"value"`
			} `json:"appearances"`
			Color struct {
				Components map[string]string `json:"components"`
			} `json:"color"`
		} `json:"colors"`
	}
	if json.Unmarshal(content, &contents) != nil {
		return assetColor{}, false
	}
	component := func(components map[string]string, key string, fallback float64) float64 {
		text, ok := components[key]
		if !ok {
			return fallback
		}
		if strings.HasPrefix(text, "0x") || strings.HasPrefix(text, "0X") {
			n, err := strconv.ParseInt(text[2:], 16, 64)
			if err != nil {
				return fallback
			}
			return float64(n) / 255
		}
		f, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return fallback
		}
		if f > 1 {
			return f / 255
		}
		return f
	}
	var result assetColor
	found := false
	for _, entry := range contents.Colors {
		if len(entry.Color.Components) == 0 {
			continue
		}
		c := entry.Color.Components
		css := colorCSS(&ColorV{R: component(c, "red", 0), G: component(c, "green", 0), B: component(c, "blue", 0), A: component(c, "alpha", 1), Op: 1})
		isDark := false
		for _, appearance := range entry.Appearances {
			isDark = isDark || (appearance.Appearance == "luminosity" && appearance.Value == "dark")
		}
		if isDark {
			result.Dark = css
		} else {
			result.Light = css
		}
		found = true
	}
	if result.Dark == "" {
		result.Dark = result.Light
	}
	if result.Light == "" {
		result.Light = result.Dark
	}
	return result, found
}

func (rt *Runtime) scan() (map[string]string, bool) {
	sig := map[string]string{}
	assets := map[string]string{}
	colorAssets := map[string]assetColor{}
	filepath.Walk(rt.dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if p != rt.dir && skipDir(info.Name()) {
				return filepath.SkipDir
			}
			if strings.HasSuffix(info.Name(), ".colorset") {
				if color, ok := readColorSet(p); ok {
					colorAssets[strings.TrimSuffix(info.Name(), ".colorset")] = color
				}
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(p))
		switch ext {
		case ".swift":
			sig[p] = fmt.Sprintf("%d:%d", info.ModTime().UnixNano(), info.Size())
		case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".pdf":
			base := strings.TrimSuffix(info.Name(), filepath.Ext(info.Name()))
			dir := filepath.Base(filepath.Dir(p))
			if strings.HasSuffix(dir, ".imageset") {
				name := strings.TrimSuffix(dir, ".imageset")
				if old, ok := assets[name]; !ok || strings.Contains(p, "@3x") || (strings.Contains(p, "@2x") && !strings.Contains(old, "@3x")) {
					assets[name] = p
				}
			} else {
				base = strings.TrimSuffix(strings.TrimSuffix(base, "@2x"), "@3x")
				if _, ok := assets[base]; !ok {
					assets[base] = p
				}
			}
		}
		return nil
	})
	rt.assets = assets
	rt.colorAssets = colorAssets
	changed := len(sig) != len(rt.files)
	for k, v := range sig {
		if rt.files[k] != v {
			changed = true
			if rt.files[k] != "" || len(rt.files) > 0 {
				rt.followFileLocked(k)
			}
		}
	}
	return sig, changed
}

func (rt *Runtime) Watch() {
	for {
		rt.mu.Lock()
		sig, changed := rt.scan()
		if changed {
			rt.files = sig
			rt.reloadLocked()
		}
		rt.mu.Unlock()
		time.Sleep(250 * time.Millisecond)
	}
}

func snippet(src string, line, col int) string {
	lines := strings.Split(src, "\n")
	if line < 1 || line > len(lines) {
		return ""
	}
	var sb strings.Builder
	for i := line - 2; i <= line; i++ {
		if i >= 1 && i <= len(lines) {
			sb.WriteString(fmt.Sprintf("%4d | %s\n", i, strings.ReplaceAll(lines[i-1], "\t", "    ")))
		}
	}
	if col > 0 {
		sb.WriteString(strings.Repeat(" ", 7+col-1) + "^")
	}
	return sb.String()
}

func (rt *Runtime) errInfo(pos Pos, msg, kind string) ErrInfo {
	e := ErrInfo{Msg: msg, File: filepath.Base(pos.File), Path: rt.rel(pos.File), Line: pos.Line, Col: pos.Col, Kind: kind}
	if src, ok := rt.sources[pos.File]; ok {
		e.Snippet = snippet(src, pos.Line, pos.Col)
	}
	return e
}

// Xcode projects carry unit-test targets and package manifests next to the app
// code; they aren't part of the UI and would not compile in the preview.
func isNonAppSource(path, src string) bool {
	if filepath.Base(path) == "Package.swift" {
		return true
	}
	return strings.Contains(src, "import XCTest") || strings.Contains(src, "import Testing")
}

func (rt *Runtime) reloadLocked() {
	start := time.Now()
	files := map[string][]Stmt{}
	rt.errs = nil
	rt.sources = map[string]string{}
	names := make([]string, 0, len(rt.files))
	for f := range rt.files {
		names = append(names, f)
	}
	sort.Strings(names)
	for _, f := range names {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		src := string(b)
		if isNonAppSource(f, src) {
			continue
		}
		rt.sources[f] = src
		stmts, err := ParseFile(f, src)
		if err != nil {
			if se, ok := err.(*SwiftError); ok {
				rt.errs = append(rt.errs, rt.errInfo(se.Pos, se.Msg, "compile"))
			} else {
				rt.errs = append(rt.errs, ErrInfo{Msg: err.Error(), File: filepath.Base(f), Kind: "compile"})
			}
			continue
		}
		files[f] = stmts
	}
	// Frameworks the preview can't run (AlarmKit, StoreKit, ...) are stood in
	// for by embedded Swift shims, loaded only when the project imports them.
	for _, fw := range importedFrameworks(rt.sources) {
		src, ok := shimSource(fw)
		if !ok {
			continue
		}
		path := shimPath(fw)
		rt.sources[path] = src
		stmts, err := ParseFile(path, src)
		if err != nil {
			if se, ok := err.(*SwiftError); ok {
				rt.errs = append(rt.errs, rt.errInfo(se.Pos, se.Msg, "compile"))
			} else {
				rt.errs = append(rt.errs, ErrInfo{Msg: err.Error(), File: path, Kind: "compile"})
			}
			continue
		}
		files[path] = stmts
	}
	if len(rt.errs) > 0 {
		rt.logError("Build failed: ", rt.errs[0])
		rt.broadcastLocked(nil)
		return
	}
	ip := NewInterp(rt.store)
	ip.rt = rt
	ip.gen = rt.gen + 1
	ip.printFn = func(s string) { rt.logf("print", "%s", s) }
	ip.warnFn = func(p Pos, s string) {
		k := p.String() + s
		if !rt.warnings[k] {
			rt.warnings[k] = true
			rt.warnList = append(rt.warnList, fmt.Sprintf("%s: %s", p, s))
			rt.logf("warn", "%s: %s", p, s)
		}
	}
	if err := ip.Load(files); err != nil {
		se, _ := err.(*SwiftError)
		if se != nil {
			rt.errs = append(rt.errs, rt.errInfo(se.Pos, se.Msg, "compile"))
		} else {
			rt.errs = append(rt.errs, ErrInfo{Msg: err.Error(), Kind: "compile"})
		}
		rt.logError("Build failed: ", rt.errs[0])
		rt.broadcastLocked(nil)
		return
	}
	rt.gen++
	rt.ip = ip
	if !rt.userDefaultsLoaded {
		rt.userDefaultsLoaded = true
		rt.loadUserDefaultsLocked()
	}
	rt.warnings = map[string]bool{}
	rt.warnList = nil
	for k, t := range rt.tickers {
		close(t.stop)
		delete(rt.tickers, k)
	}
	rt.store.prevAppear = map[string]bool{}
	rt.store.prevTasks = map[string]bool{}
	rt.store.prevDisappear = map[string]Value{}
	rt.logf("info", "Reloaded %d file(s) in %dms", len(files), time.Since(start).Milliseconds())
	rt.renderLocked()
}

// ---------- previews ----------

func (rt *Runtime) previewList(ip *Interp) []PreviewRef {
	var out []PreviewRef
	if ip.mainApp != nil {
		out = append(out, PreviewRef{ID: "app", Name: ip.mainApp.Name + " (App)", File: ""})
	}
	counts := map[string]int{}
	for _, p := range ip.previews {
		base := filepath.Base(p.File)
		counts[base]++
		name := p.Name
		if name == "" {
			name = strings.TrimSuffix(base, ".swift")
			if counts[base] > 1 {
				name += fmt.Sprintf(" %d", counts[base])
			}
		}
		id := fmt.Sprintf("%s:%s:%d", p.Kind, base, counts[base])
		out = append(out, PreviewRef{ID: id, Name: name, File: p.File})
	}
	return out
}

func (rt *Runtime) choosePreview(ip *Interp, refs []PreviewRef) string {
	if rt.pinned != "auto" {
		for _, r := range refs {
			if r.ID == rt.pinned {
				return r.ID
			}
		}
	}
	if rt.lastEdited != "" {
		for _, r := range refs {
			if r.File == rt.lastEdited {
				return r.ID
			}
		}
	}
	if rt.current != "" {
		for _, r := range refs {
			if r.ID == rt.current {
				return r.ID
			}
		}
	}
	for _, r := range refs {
		if r.ID == "app" {
			return r.ID
		}
	}
	if len(refs) > 0 {
		return refs[0].ID
	}
	return ""
}

func (rt *Runtime) rootValue(ip *Interp, id string, r *Render, env *REnv) Value {
	if id == "app" && ip.mainApp != nil {
		// SwiftUI creates the App struct once for the whole run, so its init and
		// the default values of its @State properties must not run again on
		// every render (a store created there may start Tasks, for example).
		ck := fmt.Sprintf("%d|app", rt.gen)
		inst, ok := rt.previewCache[ck]
		if !ok {
			inst = ip.instantiate(ip.mainApp, nil, Pos{})
			rt.previewCache = map[string]Value{ck: inst}
		}
		if sv, ok := inst.(*StructV); ok {
			inst = r.prepareStruct(sv, ":"+ip.mainApp.Name, env)
		}
		return ip.getMember(inst, "body", Pos{}, nil)
	}
	refs := rt.previewList(ip)
	for i, r := range refs {
		if r.ID != id {
			continue
		}
		pi := i
		if ip.mainApp != nil {
			pi--
		}
		p := ip.previews[pi]
		ck := fmt.Sprintf("%d|%s", rt.gen, id)
		if v, ok := rt.previewCache[ck]; ok {
			return v
		}
		var v Value
		if p.Kind == "provider" {
			v = ip.typeMember(&TypeV{Name: p.T.Name, T: p.T}, "previews", p.Pos)
		} else {
			v = ip.execBody(p.Body, newEnv(ip.globals), &FnCtx{})
		}
		rt.previewCache = map[string]Value{ck: v}
		return v
	}
	if ti, ok := ip.types["ContentView"]; ok && ti.IsView() {
		return ip.instantiate(ti, nil, Pos{})
	}
	return nil
}

// ---------- rendering ----------

func (rt *Runtime) catch(f func()) (failed bool) {
	defer func() {
		if r := recover(); r != nil {
			failed = true
			switch e := r.(type) {
			case *RuntimeError:
				info := rt.errInfo(e.Pos, e.Msg, "runtime")
				rt.errs = append(rt.errs, info)
				rt.logError("", info)
			case *ThrowSignal:
				info := rt.errInfo(e.Pos, "Uncaught error thrown: "+describe(e.V), "runtime")
				rt.errs = append(rt.errs, info)
				rt.logError("", info)
			case *OptNilSignal:
				rt.errs = append(rt.errs, ErrInfo{Msg: "optional chaining on nil", Kind: "runtime"})
			case *abortSignal:
				failed = false
			case *SwiftError:
				info := rt.errInfo(e.Pos, e.Msg, "runtime")
				rt.errs = append(rt.errs, info)
				rt.logError("", info)
			default:
				rt.errs = append(rt.errs, ErrInfo{Msg: fmt.Sprintf("internal error: %v", r), Kind: "runtime"})
				fmt.Fprintf(os.Stderr, "internal error: %v\n", r)
				printStack()
			}
		}
	}()
	f()
	return false
}

func (rt *Runtime) renderLocked() {
	ip := rt.ip
	if ip == nil {
		rt.broadcastLocked(nil)
		return
	}
	rt.errs = nil
	refs := rt.previewList(ip)
	rt.previews = refs
	cur := rt.choosePreview(ip, refs)
	rt.current = cur
	rt.syncModelContextsLocked()
	var res *RenderResult
	for pass := 0; pass < 8; pass++ {
		r := newRender(ip)
		ip.render = r
		var nodes []*RNode
		failed := rt.catch(func() {
			env := &REnv{styles: map[string]string{}, envObjects: map[string]Value{}, envVals: map[string]Value{}}
			root := rt.rootValue(ip, cur, r, env)
			if root == nil {
				nodes = []*RNode{{T: "empty", P: map[string]interface{}{"msg": "No preview found. Add a #Preview { ContentView() } or an @main App."}}}
				return
			}
			nodes = r.resolve(root, "", env)
		})
		ip.render = nil
		rt.handlers = r.handlers
		rt.keepImageBlobsLocked(r.imageBlobs)
		if failed {
			break
		}
		res = &RenderResult{Root: &RNode{T: "root", C: nodes}, Overlays: r.overlays, PreferredScheme: r.preferredScheme, HideTabBar: r.hideTabBar}
		r.gc()
		rt.updateTickers(r)
		ran := false
		failed2 := rt.catch(func() { ran = rt.postRender(r) })
		if failed2 || !ran {
			break
		}
	}
	rt.autosaveModelContextsLocked()
	rt.saveUserDefaultsLocked()
	if res != nil {
		rt.lastResult = res
	}
	rt.broadcastLocked(res)
}

func (rt *Runtime) postRender(r *Render) bool {
	st := rt.store
	ip := rt.ip
	var run []func()
	newAppear := map[string]bool{}
	for _, k := range r.appearOrder {
		newAppear[k] = true
		if !st.prevAppear[k] {
			f := r.appear[k]
			run = append(run, func() { ip.callValue(f, nil, Pos{}, nil) })
		}
	}
	for k, fn := range st.prevDisappear {
		if _, still := r.disappear[k]; !still {
			f := fn
			run = append(run, func() { ip.callValue(f, nil, Pos{}, nil) })
		}
	}
	st.prevAppear = newAppear
	st.prevDisappear = r.disappear
	newTasks := map[string]bool{}
	for k, fn := range r.tasks {
		newTasks[k] = true
		if !st.prevTasks[k] {
			f := fn
			run = append(run, func() { ip.spawnTask(f, Pos{}) })
		}
	}
	st.prevTasks = newTasks
	seen := map[string]bool{}
	for _, c := range r.changes {
		seen[c.key] = true
		old, ok := st.changeVals[c.key]
		st.changeVals[c.key] = c.val
		if c.fn == nil {
			continue
		}
		fire := false
		var oldV Value = old
		if ok && !ip.equals(old, c.val, Pos{}) {
			fire = true
		} else if !ok && c.initial {
			fire = true
			oldV = c.val
		}
		if fire {
			f := c.fn
			nv := c.val
			ov := oldV
			run = append(run, func() {
				if fv, ok := f.(*FuncV); ok {
					if fv.HasSig && len(fv.Params) == 0 {
						ip.callValue(f, nil, Pos{}, nil)
						return
					}
					if fv.HasSig && len(fv.Params) >= 2 {
						ip.callValue(f, []ArgV{{V: ov}, {V: nv}}, Pos{}, nil)
						return
					}
					if !fv.HasSig {
						ip.callValue(f, []ArgV{{V: ov}, {V: nv}}, Pos{}, nil)
						return
					}
				}
				ip.callValue(f, []ArgV{{V: nv}}, Pos{}, nil)
			})
		}
	}
	for k := range st.changeVals {
		if !seen[k] {
			delete(st.changeVals, k)
		}
	}
	for _, f := range run {
		f()
	}
	return len(run) > 0
}

func (rt *Runtime) updateTickers(r *Render) {
	for k, reg := range r.receivers {
		if t, ok := rt.tickers[k]; ok && t.interval == reg.interval {
			t.fn = reg.fn
			continue
		} else if ok {
			close(t.stop)
		}
		t := &ticker{interval: reg.interval, fn: reg.fn, stop: make(chan struct{})}
		rt.tickers[k] = t
		go rt.runTicker(t, rt.gen)
	}
	for k, t := range rt.tickers {
		if _, ok := r.receivers[k]; !ok {
			close(t.stop)
			delete(rt.tickers, k)
		}
	}
}

func (rt *Runtime) runTicker(t *ticker, gen int) {
	iv := t.interval
	if iv < 0.016 {
		iv = 0.016
	}
	tk := time.NewTicker(time.Duration(iv * float64(time.Second)))
	defer tk.Stop()
	for {
		select {
		case <-t.stop:
			return
		case <-tk.C:
			rt.mu.Lock()
			if rt.gen != gen {
				rt.mu.Unlock()
				return
			}
			if t.fn != nil {
				f := t.fn
				rt.catch(func() { rt.ip.callValue(f, []ArgV{{V: &DateV{T: time.Now()}}}, Pos{}, nil) })
			}
			rt.renderLocked()
			rt.mu.Unlock()
		}
	}
}

// ---------- async ----------

func (ip *Interp) markAnimated() {
	if ip.rt != nil {
		ip.rt.anim = true
	}
}

func (ip *Interp) spawnTask(f Value, pos Pos) Value {
	rt := ip.rt
	h := newObj("Task")
	cancelled := false
	h.Props["cancel"] = &NativeV{Name: "cancel", Fn: func(c *CallCtx) Value { cancelled = true; return Void }}
	h.Props["isCancelled"] = false
	if rt == nil {
		ip.callValue(f, nil, pos, nil)
		return h
	}
	gen := rt.gen
	if fv, ok := f.(*FuncV); ok && len(fv.Body) > 0 && os.Getenv("SWIT_DEBUG_TASKS") != "" {
		if es, ok := fv.Body[0].(*ExprStmt); ok {
			fmt.Fprintf(os.Stderr, "spawnTask from %s\n", es.X.P())
		} else {
			fmt.Fprintf(os.Stderr, "spawnTask from %T\n", fv.Body[0])
		}
	}
	go func() {
		rt.mu.Lock()
		defer rt.mu.Unlock()
		if rt.gen != gen || rt.ip != ip || cancelled {
			return
		}
		ip.inTask++
		rt.catch(func() { ip.callValue(f, nil, pos, nil) })
		ip.inTask--
		if rt.gen == gen {
			rt.renderLocked()
		}
	}()
	return h
}

func (ip *Interp) sleep(d float64, pos Pos) {
	rt := ip.rt
	if rt == nil {
		return
	}
	if ip.inTask == 0 {
		ip.fail(pos, "'async' call in a function that does not support concurrency — wrap it in Task { ... } or use .task { ... }")
	}
	gen := rt.gen
	rt.renderLocked()
	saved := ip.inTask
	ip.inTask = 0
	rt.mu.Unlock()
	time.Sleep(time.Duration(d * float64(time.Second)))
	rt.mu.Lock()
	ip.inTask = saved
	if rt.gen != gen || rt.ip != ip {
		panic(&abortSignal{})
	}
}

func (ip *Interp) runLater(d float64, f Value, pos Pos) {
	rt := ip.rt
	if rt == nil {
		return
	}
	gen := rt.gen
	go func() {
		if d > 0 {
			time.Sleep(time.Duration(d * float64(time.Second)))
		}
		rt.mu.Lock()
		defer rt.mu.Unlock()
		if rt.gen != gen || rt.ip != ip {
			return
		}
		rt.catch(func() { ip.callValue(f, nil, pos, nil) })
		rt.renderLocked()
	}()
}

func (t *TimerV) Stop() {
	if !t.stopped {
		t.stopped = true
		close(t.stop)
	}
}

func (ip *Interp) scheduleTimer(args []ArgV, pos Pos) Value {
	iv, _ := argNamed(args, "withTimeInterval")
	if iv == nil {
		iv, _ = argNamed(args, "timeInterval")
	}
	interval, _ := toFloat(iv)
	repeats := false
	if r, ok := argNamed(args, "repeats"); ok {
		repeats = r == true
	}
	var fn Value
	for _, a := range args {
		if a.Label == "block" || a.Trailing {
			fn = a.V
		}
	}
	t := &TimerV{stop: make(chan struct{})}
	rt := ip.rt
	if rt == nil || fn == nil {
		return t
	}
	if interval < 0.01 {
		interval = 0.01
	}
	gen := rt.gen
	go func() {
		tk := time.NewTicker(time.Duration(interval * float64(time.Second)))
		defer tk.Stop()
		for {
			select {
			case <-t.stop:
				return
			case <-tk.C:
				rt.mu.Lock()
				if rt.gen != gen || rt.ip != ip {
					rt.mu.Unlock()
					return
				}
				rt.catch(func() { ip.callValue(fn, []ArgV{{V: t}}, pos, nil) })
				rt.renderLocked()
				rt.mu.Unlock()
				if !repeats {
					t.Stop()
					return
				}
			}
		}
	}()
	return t
}

// ---------- events ----------

func (rt *Runtime) HandleEvent(id string, v interface{}) {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	h, ok := rt.handlers[id]
	if !ok || rt.ip == nil {
		return
	}
	rt.errs = nil
	if failed := rt.catch(func() { h(v) }); failed {
		errs := rt.errs
		rt.renderLocked()
		rt.errs = append(errs, rt.errs...)
		rt.broadcastLocked(nil)
		return
	}
	rt.renderLocked()
}

// ---------- broadcasting ----------

type Payload struct {
	Type     string        `json:"type"`
	Gen      int           `json:"gen"`
	Tree     *RenderResult `json:"tree,omitempty"`
	Errors   []ErrInfo     `json:"errors"`
	Console  []ConsoleLine `json:"console"`
	Warnings []string      `json:"warnings"`
	Previews []PreviewRef  `json:"previews"`
	Current  string        `json:"current"`
	Pinned   string        `json:"pinned"`
	Anim     bool          `json:"anim"`
	Dark     bool          `json:"dark"`
	Device   Device        `json:"device"`
	DeviceID string        `json:"deviceId"`
	ScrollTo string        `json:"scrollTo,omitempty"`
	Seq      int           `json:"seq"`
	Dir      string        `json:"dir"`
	Files    []FileRef     `json:"files"`
	Focus    string        `json:"focus"`

	UsesSwiftData bool                  `json:"usesSwiftData"`
	AssetColors   map[string]assetColor `json:"assetColors"`
	Images        []ImageAsset          `json:"images"`
}

func (rt *Runtime) broadcastLocked(res *RenderResult) {
	rt.renderCount++
	dev := rt.store.currentDevice()
	p := Payload{Type: "render", Gen: rt.gen, Tree: res, Errors: rt.errs, Console: rt.console, Warnings: rt.warnList,
		Previews: rt.previews, Current: rt.current, Pinned: rt.pinned, Anim: rt.anim, Dark: rt.store.dark,
		Device: dev, DeviceID: rt.store.device, ScrollTo: rt.store.scrollTo, Seq: rt.renderCount, Dir: rt.dir,
		Files: rt.fileRefs(), Focus: rt.rel(rt.lastEdited),
		UsesSwiftData: rt.ip != nil && len(rt.ip.modelTypes()) > 0, AssetColors: rt.colorAssets, Images: rt.imageAssets()}
	rt.anim = false
	rt.store.scrollTo = ""
	b, err := json.Marshal(p)
	if err != nil {
		fmt.Fprintln(os.Stderr, "json error:", err)
		return
	}
	if res == nil && rt.lastPayload != nil && len(rt.errs) == 0 {
		return
	}
	rt.lastPayload = b
	rt.cmu.Lock()
	for c := range rt.clients {
		select {
		case c <- b:
		default:
		}
	}
	rt.cmu.Unlock()
}
