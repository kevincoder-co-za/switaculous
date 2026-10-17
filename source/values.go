package main

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Value interface{}

type VoidV struct{}

var Void = &VoidV{}

type Character string

type ArrayV struct {
	E    []Value
	Elem *TypeRef
}

type DictV struct {
	Keys []Value
	Vals []Value
	idx  map[string]int
	KeyT *TypeRef
	ValT *TypeRef
}

type SetV struct {
	Keys []Value
	idx  map[string]int
	Elem *TypeRef
}

type TupleV struct {
	E []Value
	L []string
}

type RangeV struct {
	Lo, Hi     Value
	Closed     bool
	NoLo, NoHi bool
}

type StrideV struct {
	From, To, By float64
	Through      bool
	Int          bool
}

type StructV struct {
	T *TypeInfo
	F map[string]Value
}

type ObjectV struct {
	T     *TypeInfo
	F     map[string]Value
	ID    int
	Model *modelInfo
}

type EnumV struct {
	T      *TypeInfo
	Case   string
	Assoc  []Value
	Labels []string
}

type FuncV struct {
	Name      string
	Params    []*Param
	Body      []Stmt
	Env       *Env
	Self      Value
	SelfT     *TypeInfo
	Decl      *FuncDecl
	IsClosure bool
	HasSig    bool
	ResultT   *TypeRef
	Builder   bool
	Static    bool
	Mutating  bool
	MaxDollar int
}

type CallCtx struct {
	ip   *Interp
	Args []ArgV
	Pos  Pos
	Env  *Env
}

type NativeV struct {
	Name string
	Fn   func(c *CallCtx) Value
}

type TypeV struct {
	Name string
	T    *TypeInfo
	Ref  *TypeRef
}

type SymPart struct {
	Name   string
	Args   []ArgV
	Called bool
}

type SymV struct {
	Parts []SymPart
}

func (s *SymV) Name() string { return s.Parts[0].Name }
func (s *SymV) Args() []ArgV { return s.Parts[0].Args }
func sym(name string) *SymV  { return &SymV{Parts: []SymPart{{Name: name}}} }

type BindingV struct {
	Get  func() Value
	Set  func(Value)
	Name string
}

type BoxV struct {
	V        Value
	Kind     string
	Key      string
	InitHash string
	Gen      int
}

type ProjV struct {
	Get func() Value
	Set func(Value)
}

type RefV struct {
	R *Ref
}

type Ref struct {
	Get    func() Value
	Set    func(Value)
	RO     string // reason this location is immutable
	ROKind string // "value" or "property"
	InInit bool
}

type OverloadV struct{ Fns []*FuncV }

type SuperV struct {
	Obj   Value
	Owner *TypeInfo
}

type DeadlineV struct{ T time.Time }

type TimerV struct {
	stop    chan struct{}
	stopped bool
}

type PublisherV struct {
	Interval float64
}

type KeyPathV struct {
	Root string
	Path []string
}

type ArgV struct {
	Label    string
	V        Value
	X        Expr
	Trailing bool
}

type ViewV struct {
	Kind  string
	Args  []ArgV
	Pos   Pos
	Extra map[string]Value
	Kids  []Value
	Keys  []string
}

var posInf = math.Inf(1)

type ModV struct {
	Base Value
	Name string
	Args []ArgV
	Pos  Pos
}

type ModBinder struct {
	Base Value
	Name string
}

type ColorV struct {
	R, G, B, A float64
	Name       string
	Op         float64
}

type FontV struct {
	Name   string
	Style  string
	Size   float64
	Weight string
	Design string
	Italic bool
	Mono   bool
}

type UUIDV struct{ S string }
type DateV struct{ T time.Time }
type URLV struct{ S string }

type GeoV struct {
	W, H float64
	X, Y float64
}

type SizeV struct{ W, H float64 }
type PointV struct{ X, Y float64 }
type RectV struct{ X, Y, W, H float64 }

type lazyFieldV struct{}

type OptNilSignal struct{}

type ThrowSignal struct {
	V   Value
	Pos Pos
}

type RuntimeError struct {
	Pos  Pos
	Msg  string
	Kind string
}

func (e *RuntimeError) Error() string { return e.Pos.String() + ": " + e.Msg }

type abortSignal struct{}

// ---------- helpers ----------

func newDict() *DictV { return &DictV{idx: map[string]int{}} }
func newSet() *SetV   { return &SetV{idx: map[string]int{}} }

func (d *DictV) Get(k Value) (Value, bool) {
	if d.idx == nil {
		d.reindex()
	}
	i, ok := d.idx[hashKey(k)]
	if !ok {
		return nil, false
	}
	return d.Vals[i], true
}

func (d *DictV) reindex() {
	d.idx = map[string]int{}
	for i, k := range d.Keys {
		d.idx[hashKey(k)] = i
	}
}

func (d *DictV) Set(k, v Value) {
	if d.idx == nil {
		d.reindex()
	}
	h := hashKey(k)
	if i, ok := d.idx[h]; ok {
		d.Vals[i] = v
		return
	}
	d.idx[h] = len(d.Keys)
	d.Keys = append(d.Keys, k)
	d.Vals = append(d.Vals, v)
}

func (d *DictV) Remove(k Value) (Value, bool) {
	if d.idx == nil {
		d.reindex()
	}
	h := hashKey(k)
	i, ok := d.idx[h]
	if !ok {
		return nil, false
	}
	v := d.Vals[i]
	d.Keys = append(d.Keys[:i:i], d.Keys[i+1:]...)
	d.Vals = append(d.Vals[:i:i], d.Vals[i+1:]...)
	d.reindex()
	return v, true
}

func (s *SetV) Has(k Value) bool {
	if s.idx == nil {
		s.reindex()
	}
	_, ok := s.idx[hashKey(k)]
	return ok
}
func (s *SetV) reindex() {
	s.idx = map[string]int{}
	for i, k := range s.Keys {
		s.idx[hashKey(k)] = i
	}
}
func (s *SetV) Add(k Value) bool {
	if s.Has(k) {
		return false
	}
	s.idx[hashKey(k)] = len(s.Keys)
	s.Keys = append(s.Keys, k)
	return true
}
func (s *SetV) Remove(k Value) bool {
	if !s.Has(k) {
		return false
	}
	i := s.idx[hashKey(k)]
	s.Keys = append(s.Keys[:i:i], s.Keys[i+1:]...)
	s.reindex()
	return true
}

func copyValue(v Value) Value {
	switch x := v.(type) {
	case *ArrayV:
		n := &ArrayV{E: make([]Value, len(x.E)), Elem: x.Elem}
		for i, e := range x.E {
			n.E[i] = copyValue(e)
		}
		return n
	case *DictV:
		n := &DictV{Keys: make([]Value, len(x.Keys)), Vals: make([]Value, len(x.Vals)), KeyT: x.KeyT, ValT: x.ValT}
		for i := range x.Keys {
			n.Keys[i] = copyValue(x.Keys[i])
			n.Vals[i] = copyValue(x.Vals[i])
		}
		n.reindex()
		return n
	case *SetV:
		n := &SetV{Keys: make([]Value, len(x.Keys)), Elem: x.Elem}
		for i := range x.Keys {
			n.Keys[i] = copyValue(x.Keys[i])
		}
		n.reindex()
		return n
	case *StructV:
		n := &StructV{T: x.T, F: make(map[string]Value, len(x.F))}
		for k, e := range x.F {
			n.F[k] = copyValue(e)
		}
		return n
	case *TupleV:
		n := &TupleV{E: make([]Value, len(x.E)), L: x.L}
		for i, e := range x.E {
			n.E[i] = copyValue(e)
		}
		return n
	case *EnumV:
		if len(x.Assoc) == 0 {
			return x
		}
		n := &EnumV{T: x.T, Case: x.Case, Assoc: make([]Value, len(x.Assoc)), Labels: x.Labels}
		for i, e := range x.Assoc {
			n.Assoc[i] = copyValue(e)
		}
		return n
	}
	return v
}

func isNumber(v Value) bool {
	switch v.(type) {
	case int, float64:
		return true
	}
	return false
}

func toFloat(v Value) (float64, bool) {
	switch x := v.(type) {
	case int:
		return float64(x), true
	case float64:
		return x, true
	}
	return 0, false
}

func toInt(v Value) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case float64:
		return int(x), true
	}
	return 0, false
}

func hashKey(v Value) string {
	switch x := v.(type) {
	case nil:
		return "nil"
	case int:
		return "i" + strconv.Itoa(x)
	case float64:
		if x == math.Trunc(x) && math.Abs(x) < 1e15 {
			return "i" + strconv.Itoa(int(x))
		}
		return "f" + strconv.FormatFloat(x, 'g', -1, 64)
	case string:
		return "s" + x
	case Character:
		return "s" + string(x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case *UUIDV:
		return "u" + x.S
	case *EnumV:
		s := "e" + x.Case
		for _, a := range x.Assoc {
			s += "|" + hashKey(a)
		}
		return s
	case *SymV:
		return "e" + x.Name()
	case *StructV:
		keys := make([]string, 0, len(x.F))
		for k := range x.F {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		s := "S" + x.T.Name + "{"
		for _, k := range keys {
			s += k + ":" + hashKey(x.F[k]) + ","
		}
		return s + "}"
	case *ObjectV:
		return fmt.Sprintf("o%d", x.ID)
	case *DataV:
		return "data" + string(x.Bytes)
	case *UIImageV:
		return "img" + x.blobKey() + x.SystemName
	case *PhotosPickerItemV:
		return "photo" + x.Identifier
	case *TupleV:
		s := "("
		for _, e := range x.E {
			s += hashKey(e) + ","
		}
		return s + ")"
	case *ArrayV:
		s := "["
		for _, e := range x.E {
			s += hashKey(e) + ","
		}
		return s + "]"
	case *DateV:
		return "d" + x.T.String()
	case *ColorV:
		return fmt.Sprintf("c%s%v%v%v%v%v", x.Name, x.R, x.G, x.B, x.A, x.Op)
	case *BoxV:
		return hashKey(x.V)
	case *KeyPathV:
		return "k" + x.Root + "." + strings.Join(x.Path, ".")
	case *NativeObj:
		if isValueTypeNativeObj(x) {
			names := make([]string, 0, len(x.Props))
			for name := range x.Props {
				names = append(names, name)
			}
			sort.Strings(names)
			s := "n" + x.Kind + "{"
			for _, name := range names {
				s += name + ":" + hashKey(x.Props[name]) + ","
			}
			return s + "}"
		}
	}
	return fmt.Sprintf("p%p", v)
}

// These Swift types are structs, so two separately created values with the same
// contents are equal (a Picker tagged with SortDescriptor(\Book.title) relies on it).
func isValueTypeNativeObj(o *NativeObj) bool {
	switch o.Kind {
	case "SortDescriptor", "FetchDescriptor", "ModelConfiguration", "PHPickerConfiguration":
		return true
	}
	return plainDataTypes[o.Kind]
}

func valueEquals(a, b Value) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	switch x := a.(type) {
	case *KeyPathV:
		return hashKey(a) == hashKey(b)
	case *NativeObj:
		if isValueTypeNativeObj(x) {
			return hashKey(a) == hashKey(b)
		}
	case int:
		switch y := b.(type) {
		case int:
			return x == y
		case float64:
			return float64(x) == y
		}
		return false
	case float64:
		f, ok := toFloat(b)
		return ok && f == x
	case string:
		switch y := b.(type) {
		case string:
			return x == y
		case Character:
			return x == string(y)
		}
		return false
	case Character:
		switch y := b.(type) {
		case string:
			return string(x) == y
		case Character:
			return x == y
		}
		return false
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case *EnumV:
		switch y := b.(type) {
		case *EnumV:
			if x.Case != y.Case || len(x.Assoc) != len(y.Assoc) {
				return false
			}
			for i := range x.Assoc {
				if !valueEquals(x.Assoc[i], y.Assoc[i]) {
					return false
				}
			}
			return true
		case *SymV:
			return x.Case == y.Name() && symArgsEqual(y, x.Assoc)
		}
		return false
	case *SymV:
		switch y := b.(type) {
		case *EnumV:
			return valueEquals(y, x)
		case *SymV:
			return hashKey(x) == hashKey(y) && symArgsEqual(x, argVals(y.Args()))
		case *ColorV:
			return y.Name == x.Name()
		}
		return false
	case *ArrayV:
		y, ok := b.(*ArrayV)
		if !ok || len(x.E) != len(y.E) {
			return false
		}
		for i := range x.E {
			if !valueEquals(x.E[i], y.E[i]) {
				return false
			}
		}
		return true
	case *SetV:
		y, ok := b.(*SetV)
		if !ok || len(x.Keys) != len(y.Keys) {
			return false
		}
		for _, k := range x.Keys {
			if !y.Has(k) {
				return false
			}
		}
		return true
	case *DictV:
		y, ok := b.(*DictV)
		if !ok || len(x.Keys) != len(y.Keys) {
			return false
		}
		for i, k := range x.Keys {
			v2, ok := y.Get(k)
			if !ok || !valueEquals(x.Vals[i], v2) {
				return false
			}
		}
		return true
	case *TupleV:
		y, ok := b.(*TupleV)
		if !ok || len(x.E) != len(y.E) {
			return false
		}
		for i := range x.E {
			if !valueEquals(x.E[i], y.E[i]) {
				return false
			}
		}
		return true
	case *StructV:
		y, ok := b.(*StructV)
		if !ok || x.T != y.T {
			return false
		}
		for k, v := range x.F {
			if !valueEquals(v, y.F[k]) {
				return false
			}
		}
		return true
	case *ObjectV:
		return a == b
	case *DataV:
		y, ok := b.(*DataV)
		return ok && string(x.Bytes) == string(y.Bytes)
	case *UIImageV:
		y, ok := b.(*UIImageV)
		return ok && x.blobKey() == y.blobKey() && x.SystemName == y.SystemName
	case *PhotosPickerItemV:
		y, ok := b.(*PhotosPickerItemV)
		return ok && x.Identifier == y.Identifier
	case *UUIDV:
		y, ok := b.(*UUIDV)
		return ok && x.S == y.S
	case *DateV:
		y, ok := b.(*DateV)
		return ok && x.T.Equal(y.T)
	case *ColorV:
		switch y := b.(type) {
		case *ColorV:
			return hashKey(x) == hashKey(y)
		case *SymV:
			return x.Name == y.Name()
		}
		return false
	case *URLV:
		y, ok := b.(*URLV)
		return ok && x.S == y.S
	case *BoxV:
		return valueEquals(x.V, b)
	case *RangeV:
		y, ok := b.(*RangeV)
		return ok && valueEquals(x.Lo, y.Lo) && valueEquals(x.Hi, y.Hi) && x.Closed == y.Closed
	}
	return a == b
}

func symArgsEqual(s *SymV, vals []Value) bool {
	args := s.Args()
	if len(args) == 0 {
		return len(vals) == 0 || !s.Parts[0].Called
	}
	if len(args) != len(vals) {
		return false
	}
	for i := range args {
		if !valueEquals(args[i].V, vals[i]) {
			return false
		}
	}
	return true
}

func argVals(a []ArgV) []Value {
	out := make([]Value, len(a))
	for i := range a {
		out[i] = a[i].V
	}
	return out
}

func formatDouble(f float64) string {
	if math.IsInf(f, 1) {
		return "inf"
	}
	if math.IsInf(f, -1) {
		return "-inf"
	}
	if math.IsNaN(f) {
		return "nan"
	}
	if f == math.Trunc(f) && math.Abs(f) < 1e16 {
		return strconv.FormatFloat(f, 'f', 1, 64)
	}
	s := strconv.FormatFloat(f, 'g', -1, 64)
	return s
}

func describe(v Value) string { return describeIn(v, false) }

func describeIn(v Value, nested bool) string {
	switch x := v.(type) {
	case nil:
		return "nil"
	case int:
		return strconv.Itoa(x)
	case float64:
		return formatDouble(x)
	case string:
		if nested {
			return strconv.Quote(x)
		}
		return x
	case Character:
		if nested {
			return strconv.Quote(string(x))
		}
		return string(x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	case *VoidV:
		return "()"
	case *ArrayV:
		parts := make([]string, len(x.E))
		for i, e := range x.E {
			parts[i] = describeIn(e, true)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case *SetV:
		parts := make([]string, len(x.Keys))
		for i, e := range x.Keys {
			parts[i] = describeIn(e, true)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case *DictV:
		if len(x.Keys) == 0 {
			return "[:]"
		}
		parts := make([]string, len(x.Keys))
		for i := range x.Keys {
			parts[i] = describeIn(x.Keys[i], true) + ": " + describeIn(x.Vals[i], true)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case *TupleV:
		parts := make([]string, len(x.E))
		for i, e := range x.E {
			p := describeIn(e, true)
			if i < len(x.L) && x.L[i] != "" {
				p = x.L[i] + ": " + p
			}
			parts[i] = p
		}
		return "(" + strings.Join(parts, ", ") + ")"
	case *RangeV:
		op := "..<"
		if x.Closed {
			op = "..."
		}
		lo, hi := "", ""
		if !x.NoLo {
			lo = describe(x.Lo)
		}
		if !x.NoHi {
			hi = describe(x.Hi)
		}
		return lo + op + hi
	case *StructV:
		parts := []string{}
		for _, f := range x.T.Fields {
			if fv, ok := x.F[f.Name]; ok {
				if b, isBox := fv.(*BoxV); isBox {
					fv = b.V
				}
				parts = append(parts, f.Name+": "+describeIn(fv, true))
			}
		}
		return x.T.Name + "(" + strings.Join(parts, ", ") + ")"
	case *ObjectV:
		return x.T.Name
	case *DataV:
		return fmt.Sprintf("%d bytes", len(x.Bytes))
	case *UIImageV:
		return fmt.Sprintf("<UIImage size {%d, %d}>", x.Width, x.Height)
	case *PhotosPickerItemV:
		return "PhotosPickerItem(itemIdentifier: " + x.Identifier + ")"
	case *EnumV:
		s := x.Case
		if len(x.Assoc) > 0 {
			parts := make([]string, len(x.Assoc))
			for i, a := range x.Assoc {
				parts[i] = describeIn(a, true)
				if i < len(x.Labels) && x.Labels[i] != "" {
					parts[i] = x.Labels[i] + ": " + parts[i]
				}
			}
			s += "(" + strings.Join(parts, ", ") + ")"
		}
		return s
	case *SymV:
		return x.Name()
	case *UUIDV:
		return x.S
	case *DateV:
		return x.T.UTC().Format("2006-01-02 15:04:05 +0000")
	case *URLV:
		return x.S
	case *FuncV, *NativeV:
		return "(Function)"
	case *TypeV:
		return x.Name
	case *ColorV:
		if x.Name != "" {
			return x.Name
		}
		return fmt.Sprintf("#%02X%02X%02X", int(x.R*255), int(x.G*255), int(x.B*255))
	case *BoxV:
		return describeIn(x.V, nested)
	case *BindingV:
		return "Binding(" + describeIn(x.Get(), true) + ")"
	case *ViewV:
		return x.Kind
	case *ModV:
		return "ModifiedContent"
	case *SizeV:
		return fmt.Sprintf("(%s, %s)", formatDouble(x.W), formatDouble(x.H))
	case *PointV:
		return fmt.Sprintf("(%s, %s)", formatDouble(x.X), formatDouble(x.Y))
	case *NativeObj:
		if description, ok := x.Props["description"].(string); ok {
			return description
		}
		return x.Kind
	}
	return fmt.Sprintf("%v", v)
}

func typeName(v Value) string {
	switch x := v.(type) {
	case nil:
		return "Optional"
	case int:
		return "Int"
	case float64:
		return "Double"
	case string:
		return "String"
	case Character:
		return "Character"
	case bool:
		return "Bool"
	case *ArrayV:
		return "Array"
	case *DictV:
		return "Dictionary"
	case *SetV:
		return "Set"
	case *TupleV:
		return "Tuple"
	case *RangeV:
		return "Range"
	case *StructV:
		return x.T.Name
	case *ObjectV:
		return x.T.Name
	case *EnumV:
		return x.T.Name
	case *UUIDV:
		return "UUID"
	case *DateV:
		return "Date"
	case *URLV:
		return "URL"
	case *ColorV:
		return "Color"
	case *FontV:
		return "Font"
	case *ViewV:
		return x.Kind
	case *ModV:
		return "ModifiedContent"
	case *FuncV, *NativeV:
		return "Function"
	case *BindingV:
		return "Binding"
	case *VoidV:
		return "Void"
	case *PublisherV:
		return "Publisher"
	case *TimerV:
		return "Timer"
	case *NativeObj:
		return x.Kind
	case *DataV:
		return "Data"
	case *UIImageV:
		return "UIImage"
	case *PhotosPickerItemV:
		return "PhotosPickerItem"
	case *ModelContextV:
		return "ModelContext"
	case *ModelContainerV:
		return "ModelContainer"
	case *GeoV:
		return "GeometryProxy"
	case *SizeV:
		return "CGSize"
	case *PointV:
		return "CGPoint"
	case *RectV:
		return "CGRect"
	case *KeyPathV:
		return "KeyPath"
	case *TypeV:
		return x.Name + ".Type"
	case *SymV:
		return "." + x.Name()
	case *ProjV:
		return "ObservedObject.Wrapper"
	case *RefV:
		return "inout"
	}
	return fmt.Sprintf("%T", v)
}

func isViewValue(v Value) bool {
	switch x := v.(type) {
	case *ViewV, *ModV, *ColorV:
		return true
	case *StructV:
		return x.T.IsView()
	}
	return false
}

func compareValues(a, b Value) (int, bool) {
	switch x := a.(type) {
	case int:
		switch y := b.(type) {
		case int:
			if x < y {
				return -1, true
			} else if x > y {
				return 1, true
			}
			return 0, true
		case float64:
			return cmpF(float64(x), y), true
		}
	case float64:
		if f, ok := toFloat(b); ok {
			return cmpF(x, f), true
		}
	case string:
		var y string
		switch t := b.(type) {
		case string:
			y = t
		case Character:
			y = string(t)
		default:
			return 0, false
		}
		return strings.Compare(x, y), true
	case Character:
		var y string
		switch t := b.(type) {
		case string:
			y = t
		case Character:
			y = string(t)
		default:
			return 0, false
		}
		return strings.Compare(string(x), y), true
	case *DateV:
		if y, ok := b.(*DateV); ok {
			if x.T.Before(y.T) {
				return -1, true
			} else if x.T.After(y.T) {
				return 1, true
			}
			return 0, true
		}
	case bool:
		return 0, false
	case *TupleV:
		if y, ok := b.(*TupleV); ok {
			for i := 0; i < len(x.E) && i < len(y.E); i++ {
				c, ok := compareValues(x.E[i], y.E[i])
				if !ok {
					return 0, false
				}
				if c != 0 {
					return c, true
				}
			}
			return 0, true
		}
	case *EnumV:
		if y, ok := b.(*EnumV); ok && x.T == y.T {
			ix, iy := 0, 0
			for i, c := range x.T.Cases {
				if c.Name == x.Case {
					ix = i
				}
				if c.Name == y.Case {
					iy = i
				}
			}
			if ix < iy {
				return -1, true
			} else if ix > iy {
				return 1, true
			}
			return 0, true
		}
	}
	return 0, false
}

func cmpF(a, b float64) int {
	if a < b {
		return -1
	} else if a > b {
		return 1
	}
	return 0
}
