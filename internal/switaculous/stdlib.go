package switaculous

import (
	"crypto/rand"
	"fmt"
	"math"
	mrand "math/rand"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type NativeObj struct {
	Kind  string
	Props map[string]Value
}

func newObj(kind string) *NativeObj { return &NativeObj{Kind: kind, Props: map[string]Value{}} }

func secs(f float64) time.Duration { return time.Duration(f * float64(time.Second)) }
func nowTime() time.Time           { return time.Now() }

func newUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return strings.ToUpper(fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]))
}

func graphemes(s string) []string {
	var out []string
	var cur []rune
	prevZWJ := false
	riCount := 0
	for _, r := range s {
		join := false
		if len(cur) > 0 {
			switch {
			case unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || unicode.Is(unicode.Mc, r):
				join = true
			case r == 0x200D || prevZWJ:
				join = true
			case r == 0xFE0F || r == 0xFE0E:
				join = true
			case r >= 0x1F3FB && r <= 0x1F3FF:
				join = true
			case r >= 0xE0020 && r <= 0xE007F:
				join = true
			case r >= 0x1F1E6 && r <= 0x1F1FF && riCount%2 == 1:
				join = true
			case r == '\n' && len(cur) == 1 && cur[0] == '\r':
				join = true
			}
		}
		if !join && len(cur) > 0 {
			out = append(out, string(cur))
			cur = nil
		}
		cur = append(cur, r)
		prevZWJ = r == 0x200D
		if r >= 0x1F1E6 && r <= 0x1F1FF {
			riCount++
		} else {
			riCount = 0
		}
	}
	if len(cur) > 0 {
		out = append(out, string(cur))
	}
	return out
}

func strCount(s string) int { return len(graphemes(s)) }

func charsArray(s string) *ArrayV {
	a := &ArrayV{}
	for _, g := range graphemes(s) {
		a.E = append(a.E, Character(g))
	}
	return a
}

func strOf(v Value) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case Character:
		return string(x), true
	}
	return "", false
}

func argNamed(args []ArgV, label string) (Value, bool) {
	for _, a := range args {
		if a.Label == label {
			return a.V, true
		}
	}
	return nil, false
}

func argAt(args []ArgV, i int) Value {
	if i < len(args) {
		return args[i].V
	}
	return nil
}

func (ip *Interp) needArgs(c []ArgV, n int, pos Pos, name string) {
	if len(c) < n {
		ip.fail(pos, "missing argument in call to '%s'", name)
	}
}

func (ip *Interp) call1(f Value, pos Pos, args ...Value) Value {
	av := make([]ArgV, len(args))
	for i, a := range args {
		av[i] = ArgV{V: a}
	}
	return ip.callValue(f, av, pos, ip.globals)
}

func (ip *Interp) truthy(v Value, pos Pos) bool {
	b, ok := v.(bool)
	if !ok {
		ip.fail(pos, "closure must return a Bool")
	}
	return b
}

// ---------- formatting ----------

func cFormat(spec string, args []Value) string {
	var sb strings.Builder
	ai := 0
	next := func() Value {
		if ai < len(args) {
			v := args[ai]
			ai++
			return v
		}
		return nil
	}
	for i := 0; i < len(spec); i++ {
		c := spec[i]
		if c != '%' {
			sb.WriteByte(c)
			continue
		}
		j := i + 1
		for j < len(spec) && strings.IndexByte("-+ #0123456789.*", spec[j]) >= 0 {
			j++
		}
		for j < len(spec) && strings.IndexByte("lhqLzjt", spec[j]) >= 0 {
			j++
		}
		if j >= len(spec) {
			sb.WriteString(spec[i:])
			break
		}
		flags := strings.Map(func(r rune) rune {
			if strings.ContainsRune("lhqLzjt", r) {
				return -1
			}
			return r
		}, spec[i+1:j])
		verb := spec[j]
		i = j
		switch verb {
		case '%':
			sb.WriteByte('%')
		case 'd', 'i', 'u':
			v := next()
			n, _ := toInt(v)
			sb.WriteString(fmt.Sprintf("%"+flags+"d", n))
		case 'f', 'F', 'e', 'E', 'g', 'G':
			v := next()
			f, _ := toFloat(v)
			sb.WriteString(fmt.Sprintf("%"+flags+string(verb), f))
		case 'x', 'X', 'o':
			v := next()
			n, _ := toInt(v)
			sb.WriteString(fmt.Sprintf("%"+flags+string(verb), n))
		case '@', 's':
			sb.WriteString(fmt.Sprintf("%"+flags+"s", describe(next())))
		case 'c':
			v := next()
			sb.WriteString(describe(v))
		default:
			sb.WriteByte('%')
			sb.WriteByte(verb)
		}
	}
	return sb.String()
}

func groupDigits(s string) string {
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	intPart, frac := s, ""
	if i := strings.IndexByte(s, '.'); i >= 0 {
		intPart, frac = s[:i], s[i:]
	}
	var b strings.Builder
	for i, c := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	r := b.String() + frac
	if neg {
		r = "-" + r
	}
	return r
}

func formatNumberDefault(v Value) string {
	switch x := v.(type) {
	case int:
		return groupDigits(strconv.Itoa(x))
	case float64:
		s := strconv.FormatFloat(x, 'f', -1, 64)
		if i := strings.IndexByte(s, '.'); i >= 0 && len(s)-i-1 > 6 {
			s = strconv.FormatFloat(x, 'f', 6, 64)
			s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
		}
		return groupDigits(s)
	}
	return describe(v)
}

func fixedFrac(f float64, n int) string {
	return groupDigits(strconv.FormatFloat(f, 'f', n, 64))
}

func (ip *Interp) formatStyle(v Value, style Value, pos Pos) string {
	s, ok := style.(*SymV)
	if !ok {
		if no, ok := style.(*NativeObj); ok && no.Kind == "FormatStyle" {
			s = no.Props["sym"].(*SymV)
		} else {
			return formatNumberDefault(v)
		}
	}
	if d, ok := v.(*DateV); ok {
		return ip.formatDateSym(d.T, s)
	}
	f, _ := toFloat(v)
	frac := -1
	for _, p := range s.Parts[1:] {
		if p.Name == "precision" && len(p.Args) > 0 {
			if ps, ok := p.Args[0].V.(*SymV); ok && len(ps.Args()) > 0 {
				n, _ := toInt(ps.Args()[0].V)
				if r, ok := ps.Args()[0].V.(*RangeV); ok {
					n, _ = toInt(r.Hi)
				}
				frac = n
			}
		}
	}
	switch s.Name() {
	case "currency":
		code := "USD"
		if len(s.Args()) > 0 {
			code = describe(s.Args()[0].V)
		}
		sym := map[string]string{"USD": "$", "EUR": "€", "GBP": "£", "ZAR": "R", "JPY": "¥", "INR": "₹", "CAD": "CA$", "AUD": "A$", "CNY": "CN¥"}[code]
		if sym == "" {
			sym = code + " "
		}
		n := 2
		if code == "JPY" {
			n = 0
		}
		if frac >= 0 {
			n = frac
		}
		if f < 0 {
			return "-" + sym + fixedFrac(-f, n)
		}
		return sym + fixedFrac(f, n)
	case "percent":
		if _, isInt := v.(int); isInt {
			f = f / 100
		}
		if frac >= 0 {
			return fixedFrac(f*100, frac) + "%"
		}
		return formatNumberDefault(math.Round(f*100*1e6)/1e6) + "%"
	case "number":
		if frac >= 0 {
			return fixedFrac(f, frac)
		}
		return formatNumberDefault(v)
	case "dateTime":
		return describe(v)
	}
	return formatNumberDefault(v)
}

var monthNames = []string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}

func formatDateStyle(t time.Time, ds, ts string) string {
	var parts []string
	switch ds {
	case "numeric", "short":
		parts = append(parts, fmt.Sprintf("%d/%d/%04d", int(t.Month()), t.Day(), t.Year()))
	case "abbreviated", "medium":
		parts = append(parts, fmt.Sprintf("%s %d, %d", monthNames[t.Month()-1][:3], t.Day(), t.Year()))
	case "long":
		parts = append(parts, fmt.Sprintf("%s %d, %d", monthNames[t.Month()-1], t.Day(), t.Year()))
	case "complete", "full":
		parts = append(parts, fmt.Sprintf("%s, %s %d, %d", t.Weekday().String(), monthNames[t.Month()-1], t.Day(), t.Year()))
	}
	switch ts {
	case "shortened", "short":
		parts = append(parts, t.Format("3:04 PM"))
	case "standard", "medium", "long", "complete", "full":
		parts = append(parts, t.Format("3:04:05 PM"))
	}
	if len(parts) == 2 {
		if ds == "numeric" || ds == "short" {
			return parts[0] + ", " + parts[1]
		}
		return parts[0] + " at " + parts[1]
	}
	return strings.Join(parts, "")
}

func (ip *Interp) formatDateSym(t time.Time, s *SymV) string {
	if s.Name() == "dateTime" && len(s.Parts) > 1 {
		return formatDateComponents(t, s.Parts[1:])
	}
	if s.Name() == "FormatStyle" {
		dateStyle, timeStyle := "", ""
		if v, ok := argNamed(s.Args(), "date"); ok {
			dateStyle = symName(v)
		}
		if v, ok := argNamed(s.Args(), "time"); ok {
			timeStyle = symName(v)
		}
		if dateStyle == "" && timeStyle == "" {
			dateStyle, timeStyle = "numeric", "shortened"
		}
		return formatDateStyle(t, dateStyle, timeStyle)
	}
	return formatDateStyle(t, "numeric", "shortened")
}

// The parts of .dateTime.day().month().year() appear in the order an en-US
// iPhone uses ("Oct 17, 2026"), whatever order the code lists them in.
func formatDateComponents(t time.Time, parts []SymPart) string {
	has := map[string]bool{}
	for _, p := range parts {
		has[p.Name] = true
	}
	month := monthNames[t.Month()-1][:3]
	var date string
	switch {
	case has["month"] && has["day"] && has["year"]:
		date = fmt.Sprintf("%s %d, %d", month, t.Day(), t.Year())
	case has["month"] && has["day"]:
		date = fmt.Sprintf("%s %d", month, t.Day())
	case has["month"] && has["year"]:
		date = fmt.Sprintf("%s %d", month, t.Year())
	case has["day"] && has["year"]:
		date = fmt.Sprintf("%d, %d", t.Day(), t.Year())
	case has["month"]:
		date = month
	case has["day"]:
		date = strconv.Itoa(t.Day())
	case has["year"]:
		date = strconv.Itoa(t.Year())
	}
	if has["weekday"] {
		weekday := t.Weekday().String()[:3]
		if date == "" {
			date = weekday
		} else {
			date = weekday + ", " + date
		}
	}
	var clock string
	switch {
	case has["hour"] && has["minute"] && has["second"]:
		clock = t.Format("3:04:05 PM")
	case has["hour"] && has["minute"]:
		clock = t.Format("3:04 PM")
	case has["hour"]:
		clock = t.Format("3 PM")
	case has["minute"]:
		clock = strconv.Itoa(t.Minute())
	}
	switch {
	case date != "" && clock != "":
		return date + " at " + clock
	case clock != "":
		return clock
	}
	return date
}

var datePatternFields = []struct{ unicode, goLayout string }{
	{"yyyy", "2006"}, {"yy", "06"}, {"y", "2006"}, {"MMMM", "January"}, {"MMM", "Jan"}, {"MM", "01"}, {"M", "1"},
	{"dd", "02"}, {"d", "2"}, {"EEEE", "Monday"}, {"EEE", "Mon"}, {"HH", "15"}, {"H", "15"}, {"hh", "03"}, {"h", "3"},
	{"mm", "04"}, {"ss", "05"}, {"SSS", "000"}, {"a", "PM"}, {"zzz", "MST"}, {"ZZZZZ", "-07:00"}, {"Z", "-0700"},
}

// Converts a Unicode date pattern like "y-MM-dd" into Go's reference-time layout.
func goLayoutFromDatePattern(pattern string) string {
	var out strings.Builder
	i := 0
	for i < len(pattern) {
		if pattern[i] == '\'' {
			j := strings.IndexByte(pattern[i+1:], '\'')
			if j < 0 {
				out.WriteString(pattern[i+1:])
				break
			}
			out.WriteString(pattern[i+1 : i+1+j])
			i += j + 2
			continue
		}
		matched := false
		for _, field := range datePatternFields {
			if strings.HasPrefix(pattern[i:], field.unicode) {
				out.WriteString(field.goLayout)
				i += len(field.unicode)
				matched = true
				break
			}
		}
		if !matched {
			out.WriteByte(pattern[i])
			i++
		}
	}
	return out.String()
}

func formatDatePattern(t time.Time, pattern string) string {
	return t.Format(goLayoutFromDatePattern(pattern))
}

func parseDatePattern(text, pattern string) (time.Time, bool) {
	parsed, err := time.ParseInLocation(goLayoutFromDatePattern(pattern), text, time.Local)
	return parsed, err == nil
}

func relativeTime(t time.Time) string {
	d := time.Until(t)
	past := d < 0
	if past {
		d = -d
	}
	var s string
	switch {
	case d < time.Minute:
		s = fmt.Sprintf("%d sec", int(d.Seconds()))
	case d < time.Hour:
		s = fmt.Sprintf("%d min", int(d.Minutes()))
	case d < 24*time.Hour:
		s = fmt.Sprintf("%d hr", int(d.Hours()))
	default:
		s = fmt.Sprintf("%d days", int(d.Hours()/24))
	}
	if past {
		return s + " ago"
	}
	return "in " + s
}

func (ip *Interp) durationSeconds(s *SymV) float64 {
	if len(s.Args()) == 0 {
		return 0
	}
	f, _ := toFloat(s.Args()[0].V)
	switch s.Name() {
	case "milliseconds":
		return f / 1000
	case "microseconds":
		return f / 1e6
	case "nanoseconds":
		return f / 1e9
	case "minutes":
		return f * 60
	}
	return f
}

// ---------- globals ----------

func (ip *Interp) builtinGlobal(name string) Value {
	nf := func(fn func(c *CallCtx) Value) Value { return &NativeV{Name: name, Fn: fn} }
	math1 := func(f func(float64) float64) Value {
		return nf(func(c *CallCtx) Value {
			ip.needArgs(c.Args, 1, c.Pos, name)
			x, ok := toFloat(c.Args[0].V)
			if !ok {
				ip.fail(c.Pos, "cannot convert value of type '%s' to expected argument type 'Double'", typeName(c.Args[0].V))
			}
			return f(x)
		})
	}
	switch name {
	case "print", "debugPrint", "dump", "NSLog":
		return nf(func(c *CallCtx) Value {
			sep, term := " ", "\n"
			var parts []string
			for _, a := range c.Args {
				switch a.Label {
				case "separator":
					sep = describe(a.V)
				case "terminator":
					term = describe(a.V)
				default:
					if name == "debugPrint" {
						parts = append(parts, describeIn(a.V, true))
					} else {
						parts = append(parts, ip.describeV(a.V))
					}
				}
			}
			ip.printFn(strings.TrimSuffix(strings.Join(parts, sep)+term, "\n"))
			return Void
		})
	case "min", "max":
		return nf(func(c *CallCtx) Value {
			if len(c.Args) < 2 {
				ip.fail(c.Pos, "missing argument for parameter #2 in call")
			}
			best := c.Args[0].V
			for _, a := range c.Args[1:] {
				cmp, ok := compareValues(a.V, best)
				if !ok {
					ip.fail(c.Pos, "cannot compare '%s' and '%s'", typeName(a.V), typeName(best))
				}
				if (name == "min" && cmp < 0) || (name == "max" && cmp > 0) {
					best = a.V
				}
			}
			isF := false
			for _, a := range c.Args {
				if _, ok := a.V.(float64); ok {
					isF = true
				}
			}
			if isF {
				f, _ := toFloat(best)
				return f
			}
			return best
		})
	case "abs":
		return nf(func(c *CallCtx) Value {
			switch x := argAt(c.Args, 0).(type) {
			case int:
				if x < 0 {
					return -x
				}
				return x
			case float64:
				return math.Abs(x)
			}
			ip.fail(c.Pos, "abs requires a number")
			return nil
		})
	case "sqrt":
		return math1(math.Sqrt)
	case "floor":
		return math1(math.Floor)
	case "ceil":
		return math1(math.Ceil)
	case "round":
		return math1(math.Round)
	case "exp":
		return math1(math.Exp)
	case "log":
		return math1(math.Log)
	case "log10":
		return math1(math.Log10)
	case "log2":
		return math1(math.Log2)
	case "sin":
		return math1(math.Sin)
	case "cos":
		return math1(math.Cos)
	case "tan":
		return math1(math.Tan)
	case "asin":
		return math1(math.Asin)
	case "acos":
		return math1(math.Acos)
	case "atan":
		return math1(math.Atan)
	case "pow":
		return nf(func(c *CallCtx) Value {
			ip.needArgs(c.Args, 2, c.Pos, name)
			a, _ := toFloat(c.Args[0].V)
			b, _ := toFloat(c.Args[1].V)
			return math.Pow(a, b)
		})
	case "atan2":
		return nf(func(c *CallCtx) Value {
			a, _ := toFloat(argAt(c.Args, 0))
			b, _ := toFloat(argAt(c.Args, 1))
			return math.Atan2(a, b)
		})
	case "hypot":
		return nf(func(c *CallCtx) Value {
			a, _ := toFloat(argAt(c.Args, 0))
			b, _ := toFloat(argAt(c.Args, 1))
			return math.Hypot(a, b)
		})
	case "zip":
		return nf(func(c *CallCtx) Value {
			a := ip.iterate(argAt(c.Args, 0), c.Pos)
			b := ip.iterate(argAt(c.Args, 1), c.Pos)
			out := &ArrayV{}
			for i := 0; i < len(a) && i < len(b); i++ {
				out.E = append(out.E, &TupleV{E: []Value{a[i], b[i]}})
			}
			return out
		})
	case "stride":
		return nf(func(c *CallCtx) Value {
			s := &StrideV{Int: true}
			for _, a := range c.Args {
				f, _ := toFloat(a.V)
				if _, isF := a.V.(float64); isF {
					s.Int = false
				}
				switch a.Label {
				case "from":
					s.From = f
				case "to":
					s.To = f
				case "through":
					s.To = f
					s.Through = true
				case "by":
					s.By = f
				}
			}
			return s
		})
	case "repeatElement":
		return nf(func(c *CallCtx) Value {
			n, _ := toInt(argAt(c.Args, 1))
			a := &ArrayV{}
			for i := 0; i < n; i++ {
				a.E = append(a.E, copyValue(argAt(c.Args, 0)))
			}
			return a
		})
	case "type":
		return nf(func(c *CallCtx) Value {
			v := argAt(c.Args, 0)
			if ti := typeInfoOf(v); ti != nil {
				return &TypeV{Name: ti.Name, T: ti}
			}
			return &TypeV{Name: typeName(v)}
		})
	case "fatalError", "preconditionFailure", "assertionFailure":
		return nf(func(c *CallCtx) Value {
			msg := ""
			if len(c.Args) > 0 {
				msg = describe(c.Args[0].V)
			}
			ip.fail(c.Pos, "Fatal error: %s", msg)
			return nil
		})
	case "precondition", "assert":
		return nf(func(c *CallCtx) Value {
			if b, ok := argAt(c.Args, 0).(bool); ok && !b {
				msg := ""
				if len(c.Args) > 1 {
					msg = describe(c.Args[1].V)
				}
				ip.fail(c.Pos, "%s failed: %s", name, msg)
			}
			return Void
		})
	case "withAnimation", "withTransaction":
		return nf(func(c *CallCtx) Value {
			for _, a := range c.Args {
				if f, ok := a.V.(*FuncV); ok {
					if ip.render != nil || ip.rt != nil {
						ip.markAnimated()
					}
					return ip.callValue(f, nil, c.Pos, c.Env)
				}
			}
			return Void
		})
	case "swap":
		return nf(func(c *CallCtx) Value {
			a, ok1 := argAt(c.Args, 0).(*RefV)
			b, ok2 := argAt(c.Args, 1).(*RefV)
			if !ok1 || !ok2 {
				ip.fail(c.Pos, "swap requires inout arguments (&a, &b)")
			}
			va, vb := a.R.Get(), b.R.Get()
			a.R.Set(vb)
			b.R.Set(va)
			return Void
		})
	case "UIApplication":
		return nil
	}
	return nil
}

// ---------- constructors ----------

func (ip *Interp) constructArray(t *TypeRef, args []ArgV, pos Pos) Value {
	a := &ArrayV{Elem: t.Elem}
	if len(args) == 0 {
		return a
	}
	if v, ok := argNamed(args, "repeating"); ok {
		n, _ := toInt(argAt(args, 1))
		v = ip.coerce(v, t.Elem, nil)
		for i := 0; i < n; i++ {
			a.E = append(a.E, copyValue(v))
		}
		return a
	}
	for _, e := range ip.iterate(args[0].V, pos) {
		a.E = append(a.E, copyValue(ip.coerce(e, t.Elem, nil)))
	}
	return a
}

// Plain data types from frameworks the preview only imitates (MapKit, CoreLocation)
// are kept as bags of their labelled arguments, so code that builds them still runs.
var plainDataTypes = map[string]bool{
	"CLLocationCoordinate2D": true, "MKCoordinateRegion": true, "MKCoordinateSpan": true, "CLLocation": true,
	"MKMapItem": true, "MKPlacemark": true, "MapCameraPosition": true, "CLLocationDistance": true, "MKMapRect": true,
	"MKMapPoint": true, "CLGeocoder": true, "MapCameraBounds": true, "MapStyle": true,
}

func newPlainDataValue(name string, args []ArgV) *NativeObj {
	value := newObj(name)
	for i, a := range args {
		label := a.Label
		if label == "" {
			label = strconv.Itoa(i)
		}
		value.Props[label] = a.V
	}
	return value
}

func (ip *Interp) builtinConstruct(name string, args []ArgV, pos Pos, env *Env) Value {
	if plainDataTypes[name] {
		return newPlainDataValue(name, args)
	}
	if v, ok := ip.constructCodableType(name, args, pos); ok {
		return v
	}
	if isViewType(name) {
		return ip.makeView(name, args, pos, env)
	}
	if sn, ok := styleTypeNames[name]; ok {
		return sym(sn)
	}
	switch name {
	case "DragGesture", "TapGesture", "LongPressGesture", "MagnificationGesture", "MagnifyGesture", "RotationGesture", "RotateGesture", "SpatialTapGesture":
		g := newObj("Gesture")
		g.Props["kind"] = name
		return g
	case "ModelContainer":
		ip.fail(pos, "SwiftData is not supported by the preview")
	}
	a0 := argAt(args, 0)
	switch name {
	case "Int", "Int64", "Int32", "UInt", "Int16", "Int8", "UInt8", "UInt64", "UInt32":
		if len(args) == 0 {
			return 0
		}
		switch x := a0.(type) {
		case int:
			return x
		case float64:
			if math.IsNaN(x) || math.IsInf(x, 0) {
				ip.fail(pos, "Fatal error: Double value cannot be converted to Int because it is either infinite or NaN")
			}
			return int(x)
		case string:
			if r, ok := argNamed(args, "radix"); ok {
				rr, _ := toInt(r)
				n, err := strconv.ParseInt(x, rr, 64)
				if err != nil {
					return nil
				}
				return int(n)
			}
			n, err := strconv.Atoi(x)
			if err != nil {
				return nil
			}
			return n
		case Character:
			n, err := strconv.Atoi(string(x))
			if err != nil {
				return nil
			}
			return n
		case bool:
			if x {
				return 1
			}
			return 0
		}
		ip.fail(pos, "no exact matches in call to initializer 'Int(_:)' with argument of type '%s'", typeName(a0))
	case "Double", "Float", "CGFloat", "Float64", "Float32", "TimeInterval":
		if len(args) == 0 {
			return 0.0
		}
		switch x := a0.(type) {
		case int:
			return float64(x)
		case float64:
			return x
		case string:
			f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
			if err != nil {
				return nil
			}
			return f
		}
		ip.fail(pos, "no exact matches in call to initializer '%s(_:)' with argument of type '%s'", name, typeName(a0))
	case "String", "Substring", "LocalizedStringKey", "AttributedString", "LocalizedStringResource":
		if len(args) == 0 {
			return ""
		}
		if v, ok := argNamed(args, "repeating"); ok {
			n, _ := toInt(argAt(args, 1))
			s, _ := strOf(v)
			return strings.Repeat(s, max(0, n))
		}
		if v, ok := argNamed(args, "format"); ok {
			var rest []Value
			for _, a := range args[1:] {
				if arr, ok := a.V.(*ArrayV); ok && a.Label == "arguments" {
					rest = append(rest, arr.E...)
				} else {
					rest = append(rest, a.V)
				}
			}
			return cFormat(describe(v), rest)
		}
		if v, ok := argNamed(args, "describing"); ok {
			if v != nil && args[0].X != nil && env != nil && ip.isDeclaredOptional(args[0].X, env) {
				return "Optional(" + describeIn(v, true) + ")"
			}
			return ip.describeV(v)
		}
		if v, ok := argNamed(args, "localized"); ok {
			return describe(v)
		}
		if r, ok := argNamed(args, "radix"); ok {
			n, _ := toInt(a0)
			rr, _ := toInt(r)
			s := strconv.FormatInt(int64(n), rr)
			if u, ok := argNamed(args, "uppercase"); ok && u == true {
				s = strings.ToUpper(s)
			}
			return s
		}
		switch x := a0.(type) {
		case *ArrayV:
			var sb strings.Builder
			for _, e := range x.E {
				sb.WriteString(describe(e))
			}
			return sb.String()
		}
		return ip.describeV(a0)
	case "Character":
		s, _ := strOf(a0)
		return Character(s)
	case "Bool":
		switch x := a0.(type) {
		case string:
			if x == "true" {
				return true
			}
			if x == "false" {
				return false
			}
			return nil
		case bool:
			return x
		}
		return false
	case "Array", "ContiguousArray", "ArraySlice":
		return ip.constructArray(&TypeRef{Kind: tyArray}, args, pos)
	case "Set":
		s := newSet()
		if len(args) > 0 {
			for _, e := range ip.iterate(a0, pos) {
				s.Add(copyValue(e))
			}
		}
		return s
	case "Dictionary":
		d := newDict()
		if v, ok := argNamed(args, "uniqueKeysWithValues"); ok {
			for _, e := range ip.iterate(v, pos) {
				t := e.(*TupleV)
				d.Set(t.E[0], copyValue(t.E[1]))
			}
		}
		if v, ok := argNamed(args, "grouping"); ok {
			by, hasBy := argNamed(args, "by")
			if !hasBy {
				by = args[len(args)-1].V
			}
			for _, e := range ip.iterate(v, pos) {
				k := ip.call1(by, pos, e)
				cur, ok := d.Get(k)
				if !ok {
					cur = &ArrayV{}
					d.Set(k, cur)
				}
				cur.(*ArrayV).E = append(cur.(*ArrayV).E, copyValue(e))
			}
		}
		return d
	case "UUID":
		if v, ok := argNamed(args, "uuidString"); ok {
			return &UUIDV{S: describe(v)}
		}
		return &UUIDV{S: newUUID()}
	case "Date":
		if v, ok := argNamed(args, "timeIntervalSince1970"); ok {
			f, _ := toFloat(v)
			return &DateV{T: time.Unix(0, int64(f*1e9))}
		}
		if v, ok := argNamed(args, "timeIntervalSinceNow"); ok {
			f, _ := toFloat(v)
			return &DateV{T: time.Now().Add(secs(f))}
		}
		if v, ok := argNamed(args, "timeInterval"); ok {
			f, _ := toFloat(v)
			since, _ := argNamed(args, "since")
			if d, ok := since.(*DateV); ok {
				return &DateV{T: d.T.Add(secs(f))}
			}
		}
		return &DateV{T: time.Now()}
	case "URL":
		if v, ok := argNamed(args, "string"); ok {
			s := describe(v)
			if s == "" || strings.ContainsAny(s, " ") {
				return nil
			}
			return &URLV{S: s}
		}
		return &URLV{S: describe(a0)}
	case "CGSize":
		w, _ := toFloat(mustArg(args, "width"))
		h, _ := toFloat(mustArg(args, "height"))
		return &SizeV{w, h}
	case "CGPoint":
		x, _ := toFloat(mustArg(args, "x"))
		y, _ := toFloat(mustArg(args, "y"))
		return &PointV{x, y}
	case "CGRect":
		x, _ := toFloat(mustArg(args, "x"))
		y, _ := toFloat(mustArg(args, "y"))
		w, _ := toFloat(mustArg(args, "width"))
		h, _ := toFloat(mustArg(args, "height"))
		return &RectV{x, y, w, h}
	case "DateFormatter", "NumberFormatter", "ISO8601DateFormatter", "RelativeDateTimeFormatter", "DateComponentsFormatter":
		o := newObj(name)
		return o
	case "Scanner":
		o := newObj("Scanner")
		o.Props["string"] = describe(a0)
		return o
	case "DateComponents":
		o := newObj("DateComponents")
		for _, a := range args {
			o.Props[a.Label] = a.V
		}
		return o
	case "State":
		v, _ := argNamed(args, "initialValue")
		if v == nil {
			v, _ = argNamed(args, "wrappedValue")
		}
		return &BoxV{V: copyValue(v), Kind: "State"}
	case "StateObject", "ObservedObject":
		v, _ := argNamed(args, "wrappedValue")
		return &BoxV{V: v, Kind: name}
	case "Binding":
		if g, ok := argNamed(args, "get"); ok {
			s, _ := argNamed(args, "set")
			return &BindingV{Get: func() Value { return ip.call1(g, pos) }, Set: func(v Value) { ip.call1(s, pos, v) }}
		}
		if b, ok := a0.(*BindingV); ok {
			return b
		}
	case "Task":
		for _, a := range args {
			if f, ok := a.V.(*FuncV); ok {
				return ip.spawnTask(f, pos)
			}
		}
		return Void
	case "Angle":
		if v, ok := argNamed(args, "degrees"); ok {
			f, _ := toFloat(v)
			return &SymV{Parts: []SymPart{{Name: "degrees", Args: []ArgV{{V: f}}, Called: true}}}
		}
		if v, ok := argNamed(args, "radians"); ok {
			f, _ := toFloat(v)
			return &SymV{Parts: []SymPart{{Name: "radians", Args: []ArgV{{V: f}}, Called: true}}}
		}
	case "Color", "UIColor":
		return ip.builtinColor(args, pos)
	case "NavigationPath":
		return &ArrayV{}
	case "Font":
		if len(args) > 0 {
			if f, ok := args[0].V.(*FontV); ok {
				return f
			}
		}
		return &FontV{Style: "body"}
	case "NSNumber":
		return a0
	case "Timer", "DispatchQueue", "Calendar", "Locale", "TimeZone":
		return newObj(name)
	case "Optional":
		return argAt(args, 0)
	case "Gradient":
		return &ViewV{Kind: "Gradient", Args: args, Pos: pos}
	case "GridItem", "EdgeInsets", "UnitPoint", "Alert", "ToolbarItem", "ToolbarItemGroup", "Animation", "ToolbarItemPlacement",
		"Text.Case", "EdgeInsetsAlias", "StrokeStyle", "LocalizedError", "ActionSheet", "Transaction":
		return &ViewV{Kind: name, Args: args, Pos: pos}
	case "URLSession", "NotificationCenter":
		ip.fail(pos, "'%s' is not supported by the SwiftUI preview", name)
	}
	ip.fail(pos, "'%s' cannot be constructed in the preview", name)
	return nil
}

func mustArg(args []ArgV, label string) Value {
	v, _ := argNamed(args, label)
	return v
}

// ---------- static members on builtin types ----------

func (ip *Interp) builtinStatic(typeName, name string, pos Pos) Value {
	if v, ok := ip.mediaStatic(typeName, name); ok {
		return v
	}
	if v, ok := ip.codableStatic(typeName, name); ok {
		return v
	}
	switch typeName {
	case "Int", "Int64", "Int32", "UInt":
		switch name {
		case "max":
			return math.MaxInt64
		case "min":
			return math.MinInt64
		case "zero":
			return 0
		}
	case "Double", "CGFloat", "Float", "TimeInterval":
		switch name {
		case "pi":
			return math.Pi
		case "infinity":
			return math.Inf(1)
		case "greatestFiniteMagnitude":
			return math.MaxFloat64
		case "leastNormalMagnitude", "leastNonzeroMagnitude":
			return 2.2250738585072014e-308
		case "nan":
			return math.NaN()
		case "zero":
			return 0.0
		case "ulpOfOne":
			return 2.220446049250313e-16
		}
	case "Date":
		switch name {
		case "now":
			return &DateV{T: time.Now()}
		case "distantPast":
			return &DateV{T: time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)}
		case "distantFuture":
			return &DateV{T: time.Date(4001, 1, 1, 0, 0, 0, 0, time.UTC)}
		}
	case "Color", "UIColor":
		if c := ip.namedColor(name); c != nil {
			return c
		}
	case "Font":
		if f := fontFromSym(sym(name)); f != nil {
			return f
		}
	case "UIScreen":
		if name == "main" {
			o := newObj("UIScreen")
			o.Props["bounds"] = &RectV{0, 0, ip.screenW(), ip.screenH()}
			o.Props["scale"] = 3.0
			return o
		}
	case "Calendar":
		if name == "current" || name == "autoupdatingCurrent" {
			return newObj("Calendar")
		}
	case "DispatchQueue":
		return newObj("DispatchQueue")
	case "Locale", "TimeZone":
		return newObj(typeName)
	case "DispatchTime":
		if name == "now" {
			return &NativeV{Name: "now", Fn: func(c *CallCtx) Value { return &DeadlineV{T: time.Now()} }}
		}
	case "Task":
		if name == "sleep" {
			return &NativeV{Name: "sleep", Fn: func(c *CallCtx) Value { return ip.builtinStaticCall("Task", "sleep", c.Args, c.Pos, c.Env) }}
		}
		if name == "isCancelled" {
			return false
		}
	case "UUID":
	case "String", "Character", "Bool", "Array", "Dictionary", "Set":
		ip.fail(pos, "type '%s' has no member '%s'", typeName, name)
	}
	if typeName == "Double" || typeName == "Int" || typeName == "CGFloat" || typeName == "Bool" || typeName == "Float" {
		tn := typeName
		return &NativeV{Name: name, Fn: func(c *CallCtx) Value { return ip.builtinStaticCall(tn, name, c.Args, c.Pos, c.Env) }}
	}
	return sym(name)
}

func (ip *Interp) builtinStaticCall(typeN, name string, args []ArgV, pos Pos, env *Env) Value {
	if v, ok := ip.mediaStaticCall(typeN, name, args); ok {
		return v
	}
	switch typeN + "." + name {
	case "Int.random", "UInt.random":
		r, _ := argNamed(args, "in")
		rg, ok := r.(*RangeV)
		if !ok {
			ip.fail(pos, "Int.random(in:) requires a range")
		}
		lo, _ := toInt(rg.Lo)
		hi, _ := toInt(rg.Hi)
		if !rg.Closed {
			hi--
		}
		if hi < lo {
			ip.fail(pos, "Fatal error: Range is empty")
		}
		return lo + mrand.Intn(hi-lo+1)
	case "Double.random", "CGFloat.random", "Float.random":
		r, _ := argNamed(args, "in")
		rg, ok := r.(*RangeV)
		if !ok {
			ip.fail(pos, "Double.random(in:) requires a range")
		}
		lo, _ := toFloat(rg.Lo)
		hi, _ := toFloat(rg.Hi)
		return lo + mrand.Float64()*(hi-lo)
	case "Bool.random":
		return mrand.Intn(2) == 1
	case "Font.system", "Font.custom":
		return fontFromSym(&SymV{Parts: []SymPart{{Name: name, Args: args, Called: true}}})
	case "Timer.scheduledTimer":
		return ip.scheduleTimer(args, pos)
	case "Timer.publish":
		iv, _ := argNamed(args, "every")
		f, _ := toFloat(iv)
		return &PublisherV{Interval: f}
	case "Task.sleep":
		d := 0.0
		if v, ok := argNamed(args, "nanoseconds"); ok {
			f, _ := toFloat(v)
			d = f / 1e9
		} else if v, ok := argNamed(args, "for"); ok {
			if s, ok := v.(*SymV); ok {
				d = ip.durationSeconds(s)
			}
		} else if len(args) > 0 {
			f, _ := toFloat(args[0].V)
			d = f / 1e9
		}
		ip.sleep(d, pos)
		return Void
	case "Task.detached":
		for _, a := range args {
			if f, ok := a.V.(*FuncV); ok {
				return ip.spawnTask(f, pos)
			}
		}
		return Void
	case "MainActor.run":
		for _, a := range args {
			if f, ok := a.V.(*FuncV); ok {
				return ip.callValue(f, nil, pos, env)
			}
		}
		return Void
	case "Color.init", "Font.init":
		return ip.construct(&TypeV{Name: typeN}, args, pos, env)
	case "UIImpactFeedbackGenerator.init", "UINotificationFeedbackGenerator.init":
		return newObj("Haptics")
	}
	v := ip.builtinStatic(typeN, name, pos)
	return ip.callValue(v, args, pos, env)
}

// ---------- properties ----------

var builtinMethodNames = map[string]bool{}

func init() {
	for _, n := range strings.Fields(`uppercased lowercased hasPrefix hasSuffix contains split components replacingOccurrences
	trimmingCharacters reversed prefix suffix dropFirst dropLast append insert remove removeAll removeFirst removeLast popLast
	map compactMap flatMap filter reduce sorted sort reverse enumerated joined min max shuffled shuffle randomElement forEach
	allSatisfy firstIndex lastIndex first last swapAt move removeValue updateValue mapValues compactMapValues union intersection
	subtracting symmetricDifference formUnion formIntersection subtract isSubset isSuperset isDisjoint toggle isMultiple rounded
	squareRoot truncatingRemainder formatted addingTimeInterval timeIntervalSince distance advanced starts count isEmpty
	localizedCaseInsensitiveContains localizedStandardContains caseInsensitiveCompare elementsEqual index drop padding
	compare lowercasedFirst capitalized signum clamped prefixWhile string date component isDateInToday startOfDay
	async asyncAfter invalidate autoconnect scanHexInt64 scanInt dateComponents description uuidString write`) {
		builtinMethodNames[n] = true
	}
}

func hasBuiltinMethod(v Value, name string) bool {
	switch v.(type) {
	case int, float64, string, Character, bool, *ArrayV, *DictV, *SetV, *RangeV, *DateV, *StrideV:
		return builtinMethodNames[name]
	}
	return false
}

func (ip *Interp) builtinProperty(v Value, name string, pos Pos) (Value, bool) {
	if result, ok := ip.mediaProperty(v, name); ok {
		return result, true
	}
	switch x := v.(type) {
	case *ModelContextV:
		if result, ok := ip.modelContextProperty(x, name); ok {
			return result, true
		}
		return &NativeV{Name: name, Fn: func(c *CallCtx) Value {
			result, ok := ip.modelContextMethod(x, name, c.Args, c.Pos)
			if !ok {
				ip.fail(c.Pos, "value of type 'ModelContext' has no member '%s'", name)
			}
			return result
		}}, true
	case *ModelContainerV:
		return ip.modelContainerMember(x, name, nil, pos, false)
	case string:
		switch name {
		case "count":
			return strCount(x), true
		case "isEmpty":
			return x == "", true
		case "first":
			g := graphemes(x)
			if len(g) == 0 {
				return nil, true
			}
			return Character(g[0]), true
		case "last":
			g := graphemes(x)
			if len(g) == 0 {
				return nil, true
			}
			return Character(g[len(g)-1]), true
		case "capitalized", "localizedCapitalized":
			return capitalize(x), true
		case "description", "debugDescription":
			return x, true
		case "utf8", "unicodeScalars":
			a := &ArrayV{}
			for _, b := range []byte(x) {
				a.E = append(a.E, int(b))
			}
			return a, true
		case "lines":
			a := &ArrayV{}
			for _, l := range strings.Split(x, "\n") {
				a.E = append(a.E, l)
			}
			return a, true
		case "startIndex":
			return 0, true
		case "endIndex":
			return strCount(x), true
		case "indices":
			return &RangeV{Lo: 0, Hi: strCount(x)}, true
		}
	case Character:
		r, _ := utf8.DecodeRuneInString(string(x))
		switch name {
		case "isLetter":
			return unicode.IsLetter(r), true
		case "isNumber":
			return unicode.IsNumber(r), true
		case "isWholeNumber":
			return unicode.IsDigit(r), true
		case "isWhitespace":
			return unicode.IsSpace(r), true
		case "isUppercase":
			return unicode.IsUpper(r), true
		case "isLowercase":
			return unicode.IsLower(r), true
		case "isPunctuation":
			return unicode.IsPunct(r), true
		case "isLetterOrNumber":
			return unicode.IsLetter(r) || unicode.IsNumber(r), true
		case "isSymbol":
			return unicode.IsSymbol(r), true
		case "isASCII":
			return r < 128, true
		case "isNewline":
			return r == '\n' || r == '\r', true
		case "isHexDigit":
			return strings.ContainsRune("0123456789abcdefABCDEF", r), true
		case "wholeNumberValue":
			if unicode.IsDigit(r) {
				return int(r - '0'), true
			}
			return nil, true
		case "asciiValue":
			if r < 128 {
				return int(r), true
			}
			return nil, true
		case "description":
			return string(x), true
		}
	case int:
		switch name {
		case "description":
			return strconv.Itoa(x), true
		case "magnitude":
			if x < 0 {
				return -x, true
			}
			return x, true
		case "isZero":
			return x == 0, true
		case "nonzeroBitCount":
			n := 0
			for y := uint64(x); y != 0; y &= y - 1 {
				n++
			}
			return n, true
		}
	case float64:
		switch name {
		case "description":
			return formatDouble(x), true
		case "magnitude":
			return math.Abs(x), true
		case "isNaN":
			return math.IsNaN(x), true
		case "isInfinite":
			return math.IsInf(x, 0), true
		case "isFinite":
			return !math.IsInf(x, 0) && !math.IsNaN(x), true
		case "isZero":
			return x == 0, true
		case "sign":
			if x < 0 {
				return sym("minus"), true
			}
			return sym("plus"), true
		}
	case bool:
		if name == "description" {
			return describe(x), true
		}
	case *ArrayV:
		switch name {
		case "count":
			return len(x.E), true
		case "isEmpty":
			return len(x.E) == 0, true
		case "first":
			if len(x.E) == 0 {
				return nil, true
			}
			return x.E[0], true
		case "last":
			if len(x.E) == 0 {
				return nil, true
			}
			return x.E[len(x.E)-1], true
		case "indices":
			return &RangeV{Lo: 0, Hi: len(x.E)}, true
		case "startIndex":
			return 0, true
		case "endIndex":
			return len(x.E), true
		case "description":
			return describe(x), true
		case "lazy":
			return x, true
		case "underestimatedCount":
			return len(x.E), true
		}
	case *DictV:
		switch name {
		case "count":
			return len(x.Keys), true
		case "isEmpty":
			return len(x.Keys) == 0, true
		case "keys":
			return &ArrayV{E: append([]Value(nil), x.Keys...)}, true
		case "values":
			a := &ArrayV{}
			for _, v := range x.Vals {
				a.E = append(a.E, v)
			}
			return a, true
		case "first":
			if len(x.Keys) == 0 {
				return nil, true
			}
			return &TupleV{E: []Value{x.Keys[0], x.Vals[0]}, L: []string{"key", "value"}}, true
		case "description":
			return describe(x), true
		}
	case *SetV:
		switch name {
		case "count":
			return len(x.Keys), true
		case "isEmpty":
			return len(x.Keys) == 0, true
		case "first":
			if len(x.Keys) == 0 {
				return nil, true
			}
			return x.Keys[0], true
		case "description":
			return describe(x), true
		}
	case *RangeV:
		switch name {
		case "lowerBound":
			return x.Lo, true
		case "upperBound":
			return x.Hi, true
		case "count":
			lo, _ := toInt(x.Lo)
			hi, _ := toInt(x.Hi)
			if x.Closed {
				hi++
			}
			return max(0, hi-lo), true
		case "isEmpty":
			c, _ := compareValues(x.Lo, x.Hi)
			return c == 0 && !x.Closed, true
		case "first":
			return x.Lo, true
		case "last":
			if x.Closed {
				return x.Hi, true
			}
			h, _ := toInt(x.Hi)
			return h - 1, true
		}
	case *DateV:
		switch name {
		case "timeIntervalSince1970":
			return float64(x.T.UnixNano()) / 1e9, true
		case "timeIntervalSinceNow":
			return time.Until(x.T).Seconds(), true
		case "description":
			return describe(x), true
		}
	case *UUIDV:
		switch name {
		case "uuidString", "description":
			return x.S, true
		}
	case *URLV:
		switch name {
		case "absoluteString", "description":
			return x.S, true
		case "host":
			s := x.S
			if i := strings.Index(s, "://"); i >= 0 {
				s = s[i+3:]
			}
			if i := strings.IndexByte(s, '/'); i >= 0 {
				s = s[:i]
			}
			return s, true
		case "lastPathComponent":
			parts := strings.Split(strings.TrimRight(x.S, "/"), "/")
			return parts[len(parts)-1], true
		case "path":
			return x.S, true
		}
		if v, ok := ip.urlProperty(x, name); ok {
			return v, true
		}
		if urlMethodNames[name] {
			u := x
			return &NativeV{Name: name, Fn: func(c *CallCtx) Value {
				result, _ := ip.urlMethod(u, name, c.Args, c.Pos)
				return result
			}}, true
		}
	case *ColorV:
		switch name {
		case "gradient":
			return &ViewV{Kind: "ColorGradient", Args: []ArgV{{V: x}}}, true
		case "description":
			return describe(x), true
		}
	case *GeoV:
		switch name {
		case "size":
			return &SizeV{x.W, x.H}, true
		case "safeAreaInsets":
			o := newObj("EdgeInsets")
			o.Props["top"], o.Props["bottom"], o.Props["leading"], o.Props["trailing"] = 0.0, 0.0, 0.0, 0.0
			return o, true
		case "frame":
			g := x
			return &NativeV{Name: "frame", Fn: func(c *CallCtx) Value { return &RectV{g.X, g.Y, g.W, g.H} }}, true
		}
	case *SizeV:
		switch name {
		case "width":
			return x.W, true
		case "height":
			return x.H, true
		}
	case *PointV:
		switch name {
		case "x":
			return x.X, true
		case "y":
			return x.Y, true
		}
	case *RectV:
		switch name {
		case "width":
			return x.W, true
		case "height":
			return x.H, true
		case "minX":
			return x.X, true
		case "minY":
			return x.Y, true
		case "midX":
			return x.X + x.W/2, true
		case "midY":
			return x.Y + x.H/2, true
		case "maxX":
			return x.X + x.W, true
		case "maxY":
			return x.Y + x.H, true
		case "size":
			return &SizeV{x.W, x.H}, true
		case "origin":
			return &PointV{x.X, x.Y}, true
		}
	case *NativeObj:
		if v, ok := x.Props[name]; ok {
			return v, true
		}
		if x.Kind == "Calendar" {
			if v, ok := calendarProperty(name); ok {
				return v, true
			}
		}
		if x.Kind == "Bundle" {
			if v, ok := ip.bundleProperty(name); ok {
				return v, true
			}
		}
		if x.Kind == "FileManager" {
			if v, ok := ip.fileManagerProperty(name); ok {
				return v, true
			}
		}
		switch x.Kind {
		case "DispatchQueue", "Calendar", "Scanner", "DateFormatter", "NumberFormatter", "Haptics", "Timer", "RelativeDateTimeFormatter", "ISO8601DateFormatter", "DateComponentsFormatter", "Gesture",
			"JSONEncoder", "JSONDecoder", "PropertyListEncoder", "PropertyListDecoder", "UserDefaults", "Bundle", "FileManager", "ImageRenderer":
			o := x
			return &NativeV{Name: name, Fn: func(c *CallCtx) Value { return ip.nativeObjMethod(o, name, c.Args, c.Pos, c.Env) }}, true
		case "Locale":
			return localeProperty(name), true
		case "TimeZone":
			switch name {
			case "identifier":
				return "UTC", true
			case "secondsFromGMT":
				return &NativeV{Name: name, Fn: func(c *CallCtx) Value { return 0 }}, true
			}
			return "", true
		}
		return nil, true
	case *TimerV:
		t := x
		if name == "invalidate" {
			return &NativeV{Name: "invalidate", Fn: func(c *CallCtx) Value { t.Stop(); return Void }}, true
		}
		if name == "isValid" {
			return !t.stopped, true
		}
	case *PublisherV:
		p := x
		if name == "autoconnect" || name == "receive" {
			return &NativeV{Name: name, Fn: func(c *CallCtx) Value { return p }}, true
		}
		if name == "upstream" {
			return p, true
		}
	case *DeadlineV:
	case *FontV:
		f := *x
		switch name {
		case "bold":
			return &NativeV{Name: name, Fn: func(c *CallCtx) Value { f.Weight = "bold"; return &f }}, true
		case "italic":
			return &NativeV{Name: name, Fn: func(c *CallCtx) Value { f.Italic = true; return &f }}, true
		case "weight":
			return &NativeV{Name: name, Fn: func(c *CallCtx) Value {
				if s, ok := argAt(c.Args, 0).(*SymV); ok {
					f.Weight = s.Name()
				}
				return &f
			}}, true
		case "monospaced", "monospacedDigit":
			return &NativeV{Name: name, Fn: func(c *CallCtx) Value { f.Mono = true; return &f }}, true
		case "smallCaps", "leading", "width":
			return &NativeV{Name: name, Fn: func(c *CallCtx) Value { return &f }}, true
		}
	case *VoidV:
	}
	return nil, false
}

// The preview behaves like an iPhone set to US English.
func localeProperty(name string) Value {
	switch name {
	case "identifier":
		return "en_US"
	case "currency", "currencyCode":
		code := newObj("Currency")
		code.Props["identifier"] = "USD"
		code.Props["description"] = "USD"
		return code
	case "currencySymbol":
		return "$"
	case "language", "languageCode":
		language := newObj("Language")
		language.Props["languageCode"] = "en"
		language.Props["identifier"] = "en"
		language.Props["description"] = "en"
		return language
	case "region", "regionCode":
		return "US"
	case "decimalSeparator":
		return "."
	case "groupingSeparator":
		return ","
	case "measurementSystem":
		return sym("us")
	case "usesMetricSystem":
		return false
	}
	return nil
}

func capitalize(s string) string {
	var b strings.Builder
	start := true
	for _, r := range s {
		if unicode.IsSpace(r) || r == '-' {
			start = true
			b.WriteRune(r)
			continue
		}
		if start {
			b.WriteRune(unicode.ToUpper(r))
			start = false
		} else {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

func (ip *Interp) nativeObjMethod(o *NativeObj, name string, args []ArgV, pos Pos, env *Env) Value {
	if result, ok := ip.mediaNativeObjMethod(o, name, args, pos); ok {
		return result
	}
	if result, ok := ip.codableNativeObjMethod(o, name, args, pos); ok {
		return result
	}
	switch fn := o.Props[name].(type) {
	case *NativeV, *FuncV:
		return ip.callValue(fn, args, pos, env)
	}
	if extensionMethod := ip.extMethodFor(o, name); extensionMethod != nil {
		return ip.callValue(extensionMethod, args, pos, env)
	}
	switch o.Kind {
	case "DispatchQueue":
		switch name {
		case "async", "sync":
			for _, a := range args {
				if f, ok := a.V.(*FuncV); ok {
					if name == "sync" {
						return ip.callValue(f, nil, pos, env)
					}
					ip.runLater(0, f, pos)
				}
			}
			return Void
		case "asyncAfter":
			d := 0.0
			if v, ok := argNamed(args, "deadline"); ok {
				switch dl := v.(type) {
				case *DeadlineV:
					d = time.Until(dl.T).Seconds()
				case float64:
					d = dl
				}
			}
			for _, a := range args {
				if f, ok := a.V.(*FuncV); ok {
					ip.runLater(d, f, pos)
				}
			}
			return Void
		case "global":
			return o
		}
	case "Calendar":
		if result, ok := ip.calendarMethod(name, args, pos); ok {
			return result
		}
	case "DateFormatter", "ISO8601DateFormatter", "RelativeDateTimeFormatter":
		switch name {
		case "string", "localizedString":
			d, _ := argAt(args, 0).(*DateV)
			if d == nil {
				if v, ok := argNamed(args, "for"); ok {
					d, _ = v.(*DateV)
				}
			}
			if d == nil {
				return ""
			}
			if o.Kind == "RelativeDateTimeFormatter" {
				return relativeTime(d.T)
			}
			if o.Kind == "ISO8601DateFormatter" {
				return d.T.UTC().Format(time.RFC3339)
			}
			if f, ok := o.Props["dateFormat"].(string); ok && f != "" {
				return formatDatePattern(d.T, f)
			}
			ds, ts := "", ""
			if s, ok := o.Props["dateStyle"].(*SymV); ok {
				ds = s.Name()
			}
			if s, ok := o.Props["timeStyle"].(*SymV); ok {
				ts = s.Name()
			}
			return formatDateStyle(d.T, ds, ts)
		case "date":
			text := describe(mustArg(args, "from"))
			if o.Kind == "ISO8601DateFormatter" {
				for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02"} {
					if parsed, err := time.Parse(layout, text); err == nil {
						return &DateV{T: parsed.Local()}
					}
				}
				return nil
			}
			pattern, _ := o.Props["dateFormat"].(string)
			if pattern == "" {
				return nil
			}
			if parsed, ok := parseDatePattern(text, pattern); ok {
				return &DateV{T: parsed}
			}
			return nil
		}
	case "NumberFormatter":
		if name == "string" {
			v := argAt(args, 0)
			f, _ := toFloat(v)
			if mf, ok := o.Props["maximumFractionDigits"].(int); ok {
				return fixedFrac(f, mf)
			}
			if s, ok := o.Props["numberStyle"].(*SymV); ok && s.Name() == "currency" {
				return "$" + fixedFrac(f, 2)
			}
			return formatNumberDefault(v)
		}
	case "Scanner":
		switch name {
		case "scanHexInt64", "scanHexInt32":
			ref, ok := argAt(args, 0).(*RefV)
			s := strings.TrimPrefix(strings.TrimPrefix(describe(o.Props["string"]), "#"), "0x")
			n, err := strconv.ParseUint(s, 16, 64)
			if err != nil {
				return false
			}
			if ok {
				ref.R.Set(int(n))
			}
			return true
		case "scanInt":
			n, err := strconv.Atoi(describe(o.Props["string"]))
			if err != nil {
				return nil
			}
			return n
		}
		if name == "charactersToBeSkipped" {
			return Void
		}
	case "Haptics":
		return Void
	case "Gesture":
		switch name {
		case "onEnded", "onChanged", "updating", "simultaneously", "sequenced", "exclusively", "onTapGesture":
			for _, a := range args {
				if f, ok := a.V.(*FuncV); ok && name == "onEnded" {
					o.Props["ended"] = f
				}
			}
			return o
		}
		return o
	}
	ip.fail(pos, "'%s.%s' is not supported by the preview", o.Kind, name)
	return nil
}

// ---------- methods ----------

func (ip *Interp) mutCheck(ref *Ref, name string, pos Pos) {
	if ref != nil && ref.RO != "" {
		ip.fail(pos, "cannot use mutating member on immutable value: %s", ref.RO)
	}
}

func (ip *Interp) closureArg(args []ArgV, i int, pos Pos, name string) Value {
	if i >= len(args) {
		ip.fail(pos, "missing closure argument in call to '%s'", name)
	}
	return args[i].V
}

func (ip *Interp) builtinMethod(recv Value, ref *Ref, name string, args []ArgV, pos Pos, env *Env) (Value, bool) {
	if result, ok := ip.mediaMethod(recv, name, args, pos); ok {
		return result, true
	}
	switch x := recv.(type) {
	case *ModelContextV:
		return ip.modelContextMethod(x, name, args, pos)
	case *ModelContainerV:
		return ip.modelContainerMember(x, name, args, pos, true)
	case *ArrayV:
		return ip.arrayMethod(x, ref, name, args, pos)
	case string:
		return ip.stringMethod(x, ref, name, args, pos)
	case *URLV:
		return ip.urlMethod(x, name, args, pos)
	case Character:
		switch name {
		case "uppercased":
			return strings.ToUpper(string(x)), true
		case "lowercased":
			return strings.ToLower(string(x)), true
		}
	case int:
		switch name {
		case "isMultiple":
			n, _ := toInt(mustArg(args, "of"))
			if n == 0 {
				return x == 0, true
			}
			return x%n == 0, true
		case "signum":
			switch {
			case x > 0:
				return 1, true
			case x < 0:
				return -1, true
			}
			return 0, true
		case "formatted":
			if len(args) > 0 {
				return ip.formatStyle(x, args[0].V, pos), true
			}
			return formatNumberDefault(x), true
		case "advanced":
			n, _ := toInt(mustArg(args, "by"))
			return x + n, true
		case "distance":
			n, _ := toInt(mustArg(args, "to"))
			return n - x, true
		case "clamped":
			r, _ := mustArg(args, "to").(*RangeV)
			if r != nil {
				lo, _ := toInt(r.Lo)
				hi, _ := toInt(r.Hi)
				return max(lo, min(hi, x)), true
			}
		case "quotientAndRemainder":
			n, _ := toInt(mustArg(args, "dividingBy"))
			return &TupleV{E: []Value{x / n, x % n}, L: []string{"quotient", "remainder"}}, true
		}
	case float64:
		switch name {
		case "rounded":
			if len(args) > 0 {
				if s, ok := args[0].V.(*SymV); ok {
					switch s.Name() {
					case "up":
						return math.Ceil(x), true
					case "down":
						return math.Floor(x), true
					case "towardZero":
						return math.Trunc(x), true
					case "toNearestOrEven":
						return math.RoundToEven(x), true
					}
				}
			}
			return math.Round(x), true
		case "round":
			ip.mutCheck(ref, name, pos)
			ref.Set(math.Round(x))
			return Void, true
		case "squareRoot":
			return math.Sqrt(x), true
		case "truncatingRemainder":
			d, _ := toFloat(mustArg(args, "dividingBy"))
			return math.Mod(x, d), true
		case "remainder":
			d, _ := toFloat(mustArg(args, "dividingBy"))
			return math.Remainder(x, d), true
		case "formatted":
			if len(args) > 0 {
				return ip.formatStyle(x, args[0].V, pos), true
			}
			return formatNumberDefault(x), true
		case "clamped":
			r, _ := mustArg(args, "to").(*RangeV)
			if r != nil {
				lo, _ := toFloat(r.Lo)
				hi, _ := toFloat(r.Hi)
				return math.Max(lo, math.Min(hi, x)), true
			}
		case "isMultiple":
			n, _ := toFloat(mustArg(args, "of"))
			return math.Mod(x, n) == 0, true
		}
	case bool:
		if name == "toggle" {
			ip.mutCheck(ref, name, pos)
			ref.Set(!x)
			return Void, true
		}
	case *DictV:
		return ip.dictMethod(x, ref, name, args, pos)
	case *SetV:
		return ip.setMethod(x, ref, name, args, pos)
	case *RangeV:
		if name == "contains" {
			if len(args) == 1 && args[0].Label == "" {
				if _, isFn := args[0].V.(*FuncV); !isFn {
					return ip.rangeContains(x, args[0].V), true
				}
			}
		}
		if name == "clamped" {
			return x, true
		}
		if name == "reversed" {
			items := ip.iterate(x, pos)
			for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
				items[i], items[j] = items[j], items[i]
			}
			return &ArrayV{E: items}, true
		}
		if name == "randomElement" {
			lo, _ := toInt(x.Lo)
			hi, _ := toInt(x.Hi)
			if x.Closed {
				hi++
			}
			if hi <= lo {
				return nil, true
			}
			return lo + mrand.Intn(hi-lo), true
		}
		return ip.arrayMethod(&ArrayV{E: ip.iterate(x, pos)}, nil, name, args, pos)
	case *StrideV:
		return ip.arrayMethod(&ArrayV{E: ip.iterate(x, pos)}, nil, name, args, pos)
	case *DateV:
		switch name {
		case "formatted":
			if len(args) == 0 {
				return formatDateStyle(x.T, "numeric", "shortened"), true
			}
			ds, ts := "", ""
			if v, ok := argNamed(args, "date"); ok {
				if s, ok := v.(*SymV); ok {
					ds = s.Name()
				}
			}
			if v, ok := argNamed(args, "time"); ok {
				if s, ok := v.(*SymV); ok {
					ts = s.Name()
				}
			}
			if s, ok := args[0].V.(*SymV); ok && args[0].Label == "" {
				return ip.formatStyle(x, s, pos), true
			}
			return formatDateStyle(x.T, ds, ts), true
		case "addingTimeInterval", "advanced":
			f, _ := toFloat(argAt(args, 0))
			return &DateV{T: x.T.Add(secs(f))}, true
		case "timeIntervalSince":
			d, _ := argAt(args, 0).(*DateV)
			if d == nil {
				return 0.0, true
			}
			return x.T.Sub(d.T).Seconds(), true
		case "distance":
			d, _ := argAt(args, 0).(*DateV)
			if d == nil {
				return 0.0, true
			}
			return d.T.Sub(x.T).Seconds(), true
		case "addTimeInterval":
			f, _ := toFloat(argAt(args, 0))
			ref.Set(&DateV{T: x.T.Add(secs(f))})
			return Void, true
		}
	case *NativeObj:
		return ip.nativeObjMethod(x, name, args, pos, env), true
	case *ColorV:
		switch name {
		case "opacity":
			f, _ := toFloat(argAt(args, 0))
			c := *x
			if c.Op == 0 {
				c.Op = 1
			}
			c.Op *= f
			return &c, true
		case "mix":
			return x, true
		}
	}
	return nil, false
}

func (ip *Interp) predicate(f Value, pos Pos) func(Value) bool {
	return func(v Value) bool { return ip.truthy(ip.call1(f, pos, v), pos) }
}

func (ip *Interp) arrayMethod(a *ArrayV, ref *Ref, name string, args []ArgV, pos Pos) (Value, bool) {
	mut := func() {
		if ref == nil {
			ip.fail(pos, "cannot use mutating member '%s' on this value", name)
		}
		ip.mutCheck(ref, name, pos)
	}
	switch name {
	case "append":
		mut()
		if v, ok := argNamed(args, "contentsOf"); ok {
			for _, e := range ip.iterate(v, pos) {
				a.E = append(a.E, copyValue(ip.coerce(e, a.Elem, nil)))
			}
		} else {
			ip.needArgs(args, 1, pos, name)
			a.E = append(a.E, copyValue(ip.coerce(args[0].V, a.Elem, nil)))
		}
		ref.Set(a)
		return Void, true
	case "insert":
		mut()
		at, _ := toInt(mustArg(args, "at"))
		if at < 0 || at > len(a.E) {
			ip.fail(pos, "Fatal error: Array index is out of range")
		}
		var items []Value
		if v, ok := argNamed(args, "contentsOf"); ok {
			items = ip.iterate(v, pos)
		} else {
			items = []Value{copyValue(ip.coerce(args[0].V, a.Elem, nil))}
		}
		ne := append([]Value(nil), a.E[:at]...)
		ne = append(ne, items...)
		a.E = append(ne, a.E[at:]...)
		ref.Set(a)
		return Void, true
	case "remove":
		mut()
		if v, ok := argNamed(args, "atOffsets"); ok {
			idx := map[int]bool{}
			for _, e := range ip.iterate(v, pos) {
				i, _ := toInt(e)
				idx[i] = true
			}
			var ne []Value
			for i, e := range a.E {
				if !idx[i] {
					ne = append(ne, e)
				}
			}
			a.E = ne
			ref.Set(a)
			return Void, true
		}
		at, _ := toInt(mustArg(args, "at"))
		if at < 0 || at >= len(a.E) {
			ip.fail(pos, "Fatal error: Index out of range")
		}
		v := a.E[at]
		a.E = append(a.E[:at:at], a.E[at+1:]...)
		ref.Set(a)
		return v, true
	case "removeAll":
		mut()
		if len(args) > 0 && args[0].Label == "where" || len(args) > 0 && args[0].Trailing {
			p := ip.predicate(args[0].V, pos)
			var ne []Value
			for _, e := range a.E {
				if !p(e) {
					ne = append(ne, e)
				}
			}
			a.E = ne
		} else {
			a.E = nil
		}
		ref.Set(a)
		return Void, true
	case "removeFirst", "removeLast":
		mut()
		n := 1
		if len(args) > 0 {
			n, _ = toInt(args[0].V)
		}
		if len(a.E) < n {
			ip.fail(pos, "Fatal error: Can't remove %s element from an empty collection", map[bool]string{true: "first", false: "last"}[name == "removeFirst"])
		}
		var v Value
		if name == "removeFirst" {
			v = a.E[0]
			a.E = a.E[n:]
		} else {
			v = a.E[len(a.E)-1]
			a.E = a.E[:len(a.E)-n]
		}
		ref.Set(a)
		if len(args) > 0 {
			return Void, true
		}
		return v, true
	case "popLast":
		mut()
		if len(a.E) == 0 {
			return nil, true
		}
		v := a.E[len(a.E)-1]
		a.E = a.E[:len(a.E)-1]
		ref.Set(a)
		return v, true
	case "swapAt":
		mut()
		i, _ := toInt(argAt(args, 0))
		j, _ := toInt(argAt(args, 1))
		a.E[i], a.E[j] = a.E[j], a.E[i]
		ref.Set(a)
		return Void, true
	case "move":
		mut()
		from, _ := argNamed(args, "fromOffsets")
		to, _ := toInt(mustArg(args, "toOffset"))
		idx := map[int]bool{}
		var moving []Value
		for _, e := range ip.iterate(from, pos) {
			i, _ := toInt(e)
			idx[i] = true
		}
		var rest []Value
		adj := to
		for i, e := range a.E {
			if idx[i] {
				moving = append(moving, e)
				if i < to {
					adj--
				}
			} else {
				rest = append(rest, e)
			}
		}
		if adj > len(rest) {
			adj = len(rest)
		}
		ne := append([]Value(nil), rest[:adj]...)
		ne = append(ne, moving...)
		a.E = append(ne, rest[adj:]...)
		ref.Set(a)
		return Void, true
	case "sort":
		mut()
		ip.sortValues(a.E, args, pos)
		ref.Set(a)
		return Void, true
	case "reverse":
		mut()
		for i, j := 0, len(a.E)-1; i < j; i, j = i+1, j-1 {
			a.E[i], a.E[j] = a.E[j], a.E[i]
		}
		ref.Set(a)
		return Void, true
	case "shuffle":
		mut()
		mrand.Shuffle(len(a.E), func(i, j int) { a.E[i], a.E[j] = a.E[j], a.E[i] })
		ref.Set(a)
		return Void, true
	case "contains":
		if len(args) > 0 && (args[0].Label == "where" || args[0].Trailing) {
			p := ip.predicate(args[0].V, pos)
			for _, e := range a.E {
				if p(e) {
					return true, true
				}
			}
			return false, true
		}
		ip.needArgs(args, 1, pos, name)
		for _, e := range a.E {
			if ip.equals(e, args[0].V, pos) {
				return true, true
			}
		}
		return false, true
	case "firstIndex", "lastIndex", "index":
		var p func(Value) bool
		if v, ok := argNamed(args, "of"); ok {
			p = func(e Value) bool { return ip.equals(e, v, pos) }
		} else {
			p = ip.predicate(ip.closureArg(args, 0, pos, name), pos)
		}
		if name == "lastIndex" {
			for i := len(a.E) - 1; i >= 0; i-- {
				if p(a.E[i]) {
					return i, true
				}
			}
			return nil, true
		}
		for i, e := range a.E {
			if p(e) {
				return i, true
			}
		}
		return nil, true
	case "first", "last":
		p := ip.predicate(ip.closureArg(args, 0, pos, name), pos)
		if name == "last" {
			for i := len(a.E) - 1; i >= 0; i-- {
				if p(a.E[i]) {
					return a.E[i], true
				}
			}
			return nil, true
		}
		for _, e := range a.E {
			if p(e) {
				return e, true
			}
		}
		return nil, true
	case "map":
		f := ip.closureArg(args, 0, pos, name)
		out := &ArrayV{E: make([]Value, 0, len(a.E))}
		for _, e := range a.E {
			out.E = append(out.E, ip.mapFn(f, e, pos))
		}
		return out, true
	case "compactMap":
		f := ip.closureArg(args, 0, pos, name)
		out := &ArrayV{}
		for _, e := range a.E {
			if v := ip.mapFn(f, e, pos); v != nil {
				out.E = append(out.E, v)
			}
		}
		return out, true
	case "flatMap":
		f := ip.closureArg(args, 0, pos, name)
		out := &ArrayV{}
		for _, e := range a.E {
			v := ip.mapFn(f, e, pos)
			if arr, ok := v.(*ArrayV); ok {
				out.E = append(out.E, arr.E...)
			} else if v != nil {
				out.E = append(out.E, ip.iterate(v, pos)...)
			}
		}
		return out, true
	case "filter":
		p := ip.predicate(ip.closureArg(args, 0, pos, name), pos)
		out := &ArrayV{Elem: a.Elem}
		for _, e := range a.E {
			if p(e) {
				out.E = append(out.E, e)
			}
		}
		return out, true
	case "forEach":
		f := ip.closureArg(args, 0, pos, name)
		for _, e := range a.E {
			ip.call1(f, pos, e)
		}
		return Void, true
	case "reduce":
		if v, ok := argNamed(args, "into"); ok {
			acc := copyValue(v)
			f := args[len(args)-1].V
			for _, e := range a.E {
				r := &Ref{Get: func() Value { return acc }, Set: func(nv Value) { acc = nv }}
				ip.callValue(f, []ArgV{{V: &RefV{R: r}}, {V: e}}, pos, ip.globals)
			}
			return acc, true
		}
		ip.needArgs(args, 2, pos, name)
		acc := args[0].V
		f := args[1].V
		for _, e := range a.E {
			if _, isF := e.(float64); isF {
				if i, ok := acc.(int); ok {
					acc = float64(i)
				}
			}
			acc = ip.callValue(f, []ArgV{{V: acc}, {V: e}}, pos, ip.globals)
		}
		return acc, true
	case "sorted":
		out := &ArrayV{E: append([]Value(nil), a.E...), Elem: a.Elem}
		ip.sortValues(out.E, args, pos)
		return out, true
	case "reversed":
		out := &ArrayV{E: make([]Value, len(a.E)), Elem: a.Elem}
		for i, e := range a.E {
			out.E[len(a.E)-1-i] = e
		}
		return out, true
	case "enumerated":
		out := &ArrayV{}
		for i, e := range a.E {
			out.E = append(out.E, &TupleV{E: []Value{i, e}, L: []string{"offset", "element"}})
		}
		return out, true
	case "joined":
		sep := ""
		if v, ok := argNamed(args, "separator"); ok {
			sep = describe(v)
		}
		if len(a.E) > 0 {
			if _, isArr := a.E[0].(*ArrayV); isArr {
				out := &ArrayV{}
				for _, e := range a.E {
					out.E = append(out.E, e.(*ArrayV).E...)
				}
				return out, true
			}
		}
		parts := make([]string, len(a.E))
		for i, e := range a.E {
			s, ok := strOf(e)
			if !ok {
				ip.fail(pos, "joined(separator:) requires an array of Strings, found '%s' (use .map { String($0) } first)", typeName(e))
			}
			parts[i] = s
		}
		return strings.Join(parts, sep), true
	case "min", "max":
		if len(a.E) == 0 {
			return nil, true
		}
		less := func(x, y Value) bool {
			if len(args) > 0 {
				return ip.truthy(ip.callValue(args[0].V, []ArgV{{V: x}, {V: y}}, pos, ip.globals), pos)
			}
			c, ok := compareValues(x, y)
			if !ok {
				if v, ok := ip.userOperator("<", x, y, pos); ok {
					return v.(bool)
				}
				ip.fail(pos, "'%s' requires elements to be Comparable", name)
			}
			return c < 0
		}
		best := a.E[0]
		for _, e := range a.E[1:] {
			if name == "min" && less(e, best) || name == "max" && less(best, e) {
				best = e
			}
		}
		return best, true
	case "shuffled":
		out := &ArrayV{E: append([]Value(nil), a.E...), Elem: a.Elem}
		mrand.Shuffle(len(out.E), func(i, j int) { out.E[i], out.E[j] = out.E[j], out.E[i] })
		return out, true
	case "randomElement":
		if len(a.E) == 0 {
			return nil, true
		}
		return a.E[mrand.Intn(len(a.E))], true
	case "prefix", "suffix", "dropFirst", "dropLast":
		n := 1
		if len(args) > 0 {
			if args[0].Label == "while" || args[0].Trailing {
				p := ip.predicate(args[0].V, pos)
				i := 0
				for i < len(a.E) && p(a.E[i]) {
					i++
				}
				return &ArrayV{E: append([]Value(nil), a.E[:i]...)}, true
			}
			n, _ = toInt(args[0].V)
		} else if name == "prefix" || name == "suffix" {
			ip.fail(pos, "missing argument for parameter #1 in call")
		}
		if n > len(a.E) {
			n = len(a.E)
		}
		var e []Value
		switch name {
		case "prefix":
			e = a.E[:n]
		case "suffix":
			e = a.E[len(a.E)-n:]
		case "dropFirst":
			e = a.E[n:]
		case "dropLast":
			e = a.E[:len(a.E)-n]
		}
		return &ArrayV{E: append([]Value(nil), e...), Elem: a.Elem}, true
	case "drop":
		p := ip.predicate(args[0].V, pos)
		i := 0
		for i < len(a.E) && p(a.E[i]) {
			i++
		}
		return &ArrayV{E: append([]Value(nil), a.E[i:]...)}, true
	case "allSatisfy":
		p := ip.predicate(ip.closureArg(args, 0, pos, name), pos)
		for _, e := range a.E {
			if !p(e) {
				return false, true
			}
		}
		return true, true
	case "count":
		p := ip.predicate(ip.closureArg(args, 0, pos, name), pos)
		n := 0
		for _, e := range a.E {
			if p(e) {
				n++
			}
		}
		return n, true
	case "elementsEqual":
		return valueEquals(a, &ArrayV{E: ip.iterate(argAt(args, 0), pos)}), true
	case "starts":
		other := ip.iterate(mustArg(args, "with"), pos)
		if len(other) > len(a.E) {
			return false, true
		}
		for i := range other {
			if !valueEquals(a.E[i], other[i]) {
				return false, true
			}
		}
		return true, true
	case "formatted":
		parts := make([]string, len(a.E))
		for i, e := range a.E {
			parts[i] = describe(e)
		}
		switch len(parts) {
		case 0:
			return "", true
		case 1:
			return parts[0], true
		case 2:
			return parts[0] + " and " + parts[1], true
		}
		return strings.Join(parts[:len(parts)-1], ", ") + ", and " + parts[len(parts)-1], true
	case "split":
		sep, _ := argNamed(args, "separator")
		out := &ArrayV{}
		cur := &ArrayV{}
		for _, e := range a.E {
			if valueEquals(e, sep) {
				if len(cur.E) > 0 {
					out.E = append(out.E, cur)
				}
				cur = &ArrayV{}
			} else {
				cur.E = append(cur.E, e)
			}
		}
		if len(cur.E) > 0 {
			out.E = append(out.E, cur)
		}
		return out, true
	case "uniqued":
	}
	return nil, false
}

func (ip *Interp) mapFn(f Value, e Value, pos Pos) Value {
	if kp, ok := f.(*KeyPathV); ok {
		return ip.applyKeyPath(kp, e, pos)
	}
	return ip.call1(f, pos, e)
}

func (ip *Interp) sortValues(e []Value, args []ArgV, pos Pos) {
	var less func(x, y Value) bool
	if len(args) > 0 {
		f := args[0].V
		if id, ok := f.(*NativeV); ok && (id.Name == ">" || id.Name == "<") {
			_ = id
		}
		if s, ok := args[0].X.(*Ident); ok && (s.Name == "<" || s.Name == ">") {
			op := s.Name
			less = func(x, y Value) bool { return ip.binop(op, x, y, pos).(bool) }
		} else if kp, ok := f.(*KeyPathV); ok {
			less = func(x, y Value) bool {
				c, _ := compareValues(ip.applyKeyPath(kp, x, pos), ip.applyKeyPath(kp, y, pos))
				return c < 0
			}
		} else {
			less = func(x, y Value) bool {
				return ip.truthy(ip.callValue(f, []ArgV{{V: x}, {V: y}}, pos, ip.globals), pos)
			}
		}
	} else {
		less = func(x, y Value) bool {
			c, ok := compareValues(x, y)
			if !ok {
				if v, ok := ip.userOperator("<", x, y, pos); ok {
					return v.(bool)
				}
				ip.fail(pos, "referencing instance method 'sorted()' requires that '%s' conform to 'Comparable'", typeName(x))
			}
			return c < 0
		}
	}
	sort.SliceStable(e, func(i, j int) bool { return less(e[i], e[j]) })
}

func (ip *Interp) stringMethod(s string, ref *Ref, name string, args []ArgV, pos Pos) (Value, bool) {
	arg0s := func() string {
		if len(args) == 0 {
			ip.fail(pos, "missing argument in call to '%s'", name)
		}
		v, ok := strOf(args[0].V)
		if !ok {
			ip.fail(pos, "cannot convert value of type '%s' to expected argument type 'String'", typeName(args[0].V))
		}
		return v
	}
	switch name {
	case "write":
		target, _ := argNamed(args, "to")
		if target == nil {
			target, _ = argNamed(args, "toFile")
		}
		ip.writeSandboxFile(target, []byte(s), pos)
		return Void, true
	case "uppercased":
		return strings.ToUpper(s), true
	case "lowercased":
		return strings.ToLower(s), true
	case "hasPrefix":
		return strings.HasPrefix(s, arg0s()), true
	case "hasSuffix":
		return strings.HasSuffix(s, arg0s()), true
	case "contains":
		if len(args) > 0 && (args[0].Label == "where" || args[0].Trailing) {
			p := ip.predicate(args[0].V, pos)
			for _, g := range graphemes(s) {
				if p(Character(g)) {
					return true, true
				}
			}
			return false, true
		}
		return strings.Contains(s, arg0s()), true
	case "localizedCaseInsensitiveContains", "localizedStandardContains":
		return strings.Contains(strings.ToLower(s), strings.ToLower(arg0s())), true
	case "caseInsensitiveCompare", "compare":
		c := strings.Compare(strings.ToLower(s), strings.ToLower(arg0s()))
		return map[int]Value{-1: sym("orderedAscending"), 0: sym("orderedSame"), 1: sym("orderedDescending")}[c], true
	case "split":
		sepV, ok := argNamed(args, "separator")
		if !ok && len(args) > 0 && args[0].Label == "" {
			sepV = args[0].V
			ok = true
		}
		out := &ArrayV{}
		if !ok {
			if f, ok2 := argNamed(args, "whereSeparator"); ok2 {
				p := ip.predicate(f, pos)
				cur := ""
				for _, g := range graphemes(s) {
					if p(Character(g)) {
						if cur != "" {
							out.E = append(out.E, cur)
						}
						cur = ""
					} else {
						cur += g
					}
				}
				if cur != "" {
					out.E = append(out.E, cur)
				}
				return out, true
			}
		}
		sep, _ := strOf(sepV)
		omit := true
		if v, ok := argNamed(args, "omittingEmptySubsequences"); ok {
			omit = v == true
		}
		for _, p := range strings.Split(s, sep) {
			if p == "" && omit {
				continue
			}
			out.E = append(out.E, p)
		}
		return out, true
	case "components":
		sep, _ := strOf(mustArg(args, "separatedBy"))
		if so, ok := mustArg(args, "separatedBy").(*SymV); ok {
			out := &ArrayV{}
			fs := strings.FieldsFunc(s, func(r rune) bool {
				if so.Name() == "newlines" {
					return r == '\n'
				}
				return unicode.IsSpace(r)
			})
			_ = fs
			var parts []string
			if so.Name() == "newlines" {
				parts = strings.Split(s, "\n")
			} else {
				parts = strings.FieldsFunc(s, unicode.IsSpace)
			}
			for _, p := range parts {
				out.E = append(out.E, p)
			}
			return out, true
		}
		out := &ArrayV{}
		for _, p := range strings.Split(s, sep) {
			out.E = append(out.E, p)
		}
		return out, true
	case "replacingOccurrences":
		of, _ := strOf(mustArg(args, "of"))
		with, _ := strOf(mustArg(args, "with"))
		return strings.ReplaceAll(s, of, with), true
	case "replacing":
		of, _ := strOf(argAt(args, 0))
		with, _ := strOf(mustArg(args, "with"))
		return strings.ReplaceAll(s, of, with), true
	case "trimmingCharacters":
		cs, _ := mustArg(args, "in").(*SymV)
		if cs != nil && cs.Name() == "whitespaces" {
			return strings.Trim(s, " \t"), true
		}
		if cs != nil && cs.Name() == "punctuationCharacters" {
			return strings.TrimFunc(s, unicode.IsPunct), true
		}
		return strings.TrimSpace(s), true
	case "reversed":
		g := graphemes(s)
		for i, j := 0, len(g)-1; i < j; i, j = i+1, j-1 {
			g[i], g[j] = g[j], g[i]
		}
		return strings.Join(g, ""), true
	case "prefix", "suffix", "dropFirst", "dropLast":
		g := graphemes(s)
		n := 1
		if len(args) > 0 {
			if args[0].Label == "while" || args[0].Trailing {
				p := ip.predicate(args[0].V, pos)
				i := 0
				for i < len(g) && p(Character(g[i])) {
					i++
				}
				return strings.Join(g[:i], ""), true
			}
			n, _ = toInt(args[0].V)
		}
		if n > len(g) {
			n = len(g)
		}
		switch name {
		case "prefix":
			return strings.Join(g[:n], ""), true
		case "suffix":
			return strings.Join(g[len(g)-n:], ""), true
		case "dropFirst":
			return strings.Join(g[n:], ""), true
		}
		return strings.Join(g[:len(g)-n], ""), true
	case "append":
		ip.mutCheck(ref, name, pos)
		if v, ok := argNamed(args, "contentsOf"); ok {
			t, _ := strOf(v)
			ref.Set(s + t)
		} else {
			ref.Set(s + arg0s())
		}
		return Void, true
	case "insert":
		ip.mutCheck(ref, name, pos)
		c := arg0s()
		at, _ := toInt(mustArg(args, "at"))
		g := graphemes(s)
		if at > len(g) {
			at = len(g)
		}
		ref.Set(strings.Join(g[:at], "") + c + strings.Join(g[at:], ""))
		return Void, true
	case "removeAll":
		ip.mutCheck(ref, name, pos)
		if len(args) > 0 {
			p := ip.predicate(args[0].V, pos)
			var b strings.Builder
			for _, g := range graphemes(s) {
				if !p(Character(g)) {
					b.WriteString(g)
				}
			}
			ref.Set(b.String())
		} else {
			ref.Set("")
		}
		return Void, true
	case "removeLast", "removeFirst", "popLast":
		ip.mutCheck(ref, name, pos)
		g := graphemes(s)
		if len(g) == 0 {
			if name == "popLast" {
				return nil, true
			}
			ip.fail(pos, "Fatal error: Can't remove from an empty collection")
		}
		if name == "removeFirst" {
			ref.Set(strings.Join(g[1:], ""))
			return Character(g[0]), true
		}
		ref.Set(strings.Join(g[:len(g)-1], ""))
		return Character(g[len(g)-1]), true
	case "padding":
		n, _ := toInt(mustArg(args, "toLength"))
		pad, _ := strOf(mustArg(args, "withPad"))
		g := graphemes(s)
		for len(g) < n {
			g = append(g, pad)
		}
		return strings.Join(g[:n], ""), true
	case "starts":
		t, _ := strOf(mustArg(args, "with"))
		return strings.HasPrefix(s, t), true
	case "distance":
		from, _ := toInt(mustArg(args, "from"))
		to, _ := toInt(mustArg(args, "to"))
		return to - from, true
	case "firstIndex", "lastIndex":
		g := graphemes(s)
		var p func(Value) bool
		if v, ok := argNamed(args, "of"); ok {
			p = func(e Value) bool { return valueEquals(e, v) }
		} else {
			p = ip.predicate(args[0].V, pos)
		}
		for i := range g {
			idx := i
			if name == "lastIndex" {
				idx = len(g) - 1 - i
			}
			if p(Character(g[idx])) {
				return idx, true
			}
		}
		return nil, true
	}
	switch name {
	case "map", "filter", "compactMap", "forEach", "sorted", "enumerated", "allSatisfy", "first", "last", "count",
		"reduce", "shuffled", "randomElement", "flatMap", "min", "max", "joined", "contains", "drop":
		v, ok := ip.arrayMethod(charsArray(s), nil, name, args, pos)
		if ok && name == "filter" {
			var b strings.Builder
			for _, e := range v.(*ArrayV).E {
				b.WriteString(describe(e))
			}
			return b.String(), true
		}
		return v, ok
	}
	return nil, false
}

func (ip *Interp) dictMethod(d *DictV, ref *Ref, name string, args []ArgV, pos Pos) (Value, bool) {
	pairs := func() []Value {
		out := make([]Value, len(d.Keys))
		for i := range d.Keys {
			out[i] = &TupleV{E: []Value{d.Keys[i], d.Vals[i]}, L: []string{"key", "value"}}
		}
		return out
	}
	switch name {
	case "removeValue":
		ip.mutCheck(ref, name, pos)
		v, _ := d.Remove(mustArg(args, "forKey"))
		ref.Set(d)
		return v, true
	case "updateValue":
		ip.mutCheck(ref, name, pos)
		k := mustArg(args, "forKey")
		old, _ := d.Get(k)
		d.Set(k, copyValue(args[0].V))
		ref.Set(d)
		return old, true
	case "removeAll":
		ip.mutCheck(ref, name, pos)
		d.Keys, d.Vals = nil, nil
		d.reindex()
		ref.Set(d)
		return Void, true
	case "mapValues", "compactMapValues":
		f := args[0].V
		out := newDict()
		for i, k := range d.Keys {
			v := ip.call1(f, pos, d.Vals[i])
			if v == nil && name == "compactMapValues" {
				continue
			}
			out.Set(k, v)
		}
		return out, true
	case "filter":
		p := ip.predicate(args[0].V, pos)
		out := newDict()
		for i, k := range d.Keys {
			if p(&TupleV{E: []Value{k, d.Vals[i]}, L: []string{"key", "value"}}) {
				out.Set(k, d.Vals[i])
			}
		}
		return out, true
	case "merge":
		ip.mutCheck(ref, name, pos)
		other, _ := argAt(args, 0).(*DictV)
		if other != nil {
			for i, k := range other.Keys {
				if old, exists := d.Get(k); exists && len(args) > 1 {
					d.Set(k, ip.callValue(args[1].V, []ArgV{{V: old}, {V: other.Vals[i]}}, pos, ip.globals))
				} else {
					d.Set(k, other.Vals[i])
				}
			}
		}
		ref.Set(d)
		return Void, true
	case "index":
		k := mustArg(args, "forKey")
		_, ok := d.Get(k)
		if !ok {
			return nil, true
		}
		return 0, true
	}
	return ip.arrayMethod(&ArrayV{E: pairs()}, nil, name, args, pos)
}

func (ip *Interp) setMethod(s *SetV, ref *Ref, name string, args []ArgV, pos Pos) (Value, bool) {
	other := func() *SetV {
		o := newSet()
		for _, e := range ip.iterate(argAt(args, 0), pos) {
			o.Add(e)
		}
		return o
	}
	switch name {
	case "insert":
		ip.mutCheck(ref, name, pos)
		v := copyValue(ip.coerce(args[0].V, s.Elem, nil))
		ok := s.Add(v)
		ref.Set(s)
		return &TupleV{E: []Value{ok, v}, L: []string{"inserted", "memberAfterInsert"}}, true
	case "remove":
		ip.mutCheck(ref, name, pos)
		v := ip.coerce(args[0].V, s.Elem, nil)
		ok := s.Remove(v)
		ref.Set(s)
		if ok {
			return v, true
		}
		return nil, true
	case "removeAll":
		ip.mutCheck(ref, name, pos)
		s.Keys = nil
		s.reindex()
		ref.Set(s)
		return Void, true
	case "contains":
		if len(args) > 0 && args[0].Label == "" && !args[0].Trailing {
			return s.Has(ip.coerce(args[0].V, s.Elem, nil)), true
		}
	case "union", "formUnion":
		out := s
		if name == "union" {
			out = copyValue(s).(*SetV)
		} else {
			ip.mutCheck(ref, name, pos)
		}
		for _, e := range ip.iterate(argAt(args, 0), pos) {
			out.Add(e)
		}
		if name == "formUnion" {
			ref.Set(out)
			return Void, true
		}
		return out, true
	case "intersection", "formIntersection":
		o := other()
		out := newSet()
		for _, e := range s.Keys {
			if o.Has(e) {
				out.Add(e)
			}
		}
		if name == "formIntersection" {
			ref.Set(out)
			return Void, true
		}
		return out, true
	case "subtracting", "subtract":
		o := other()
		out := newSet()
		for _, e := range s.Keys {
			if !o.Has(e) {
				out.Add(e)
			}
		}
		if name == "subtract" {
			ref.Set(out)
			return Void, true
		}
		return out, true
	case "symmetricDifference":
		o := other()
		out := newSet()
		for _, e := range s.Keys {
			if !o.Has(e) {
				out.Add(e)
			}
		}
		for _, e := range o.Keys {
			if !s.Has(e) {
				out.Add(e)
			}
		}
		return out, true
	case "isSubset", "isSuperset", "isDisjoint":
		o := other()
		switch name {
		case "isSubset":
			for _, e := range s.Keys {
				if !o.Has(e) {
					return false, true
				}
			}
			return true, true
		case "isSuperset":
			for _, e := range o.Keys {
				if !s.Has(e) {
					return false, true
				}
			}
			return true, true
		}
		for _, e := range s.Keys {
			if o.Has(e) {
				return false, true
			}
		}
		return true, true
	}
	return ip.arrayMethod(&ArrayV{E: append([]Value(nil), s.Keys...)}, nil, name, args, pos)
}
