package switaculous

import (
	"strings"
)

// Swift Charts in the preview: the chart's content closure runs once per data
// element, every mark it produces is collected with its plotted values, and
// the browser draws the result as an SVG.

var chartMarkKinds = map[string]string{
	"BarMark": "bar", "LineMark": "line", "PointMark": "point", "AreaMark": "area", "RuleMark": "rule", "RectangleMark": "rect",
}

type chartMark struct {
	kind   string
	values map[string]interface{}
	series string
	color  string
	smooth bool
	width  float64
	dash   bool
}

func (m chartMark) toJSON() map[string]interface{} {
	out := map[string]interface{}{"k": m.kind}
	for key, value := range m.values {
		out[key] = value
	}
	if m.series != "" {
		out["series"] = m.series
	}
	if m.color != "" {
		out["color"] = m.color
	}
	if m.smooth {
		out["smooth"] = true
	}
	if m.width > 0 {
		out["width"] = m.width
	}
	if m.dash {
		out["dash"] = true
	}
	return out
}

// A plotted value is `.value("Label", v)`; the label becomes the axis title
// and v is a number, Date or category string.
func (r *Render) plottableValue(v Value) (interface{}, string, string) {
	if s, ok := v.(*SymV); ok && len(s.Parts) == 1 && s.Parts[0].Name == "value" && s.Parts[0].Called {
		args := s.Parts[0].Args
		label := ""
		if len(args) > 0 {
			label = describe(args[0].V)
		}
		var raw Value
		if len(args) > 1 {
			raw = args[1].V
		}
		value, kind := plotCoordinate(raw)
		return value, kind, label
	}
	value, kind := plotCoordinate(v)
	return value, kind, ""
}

func plotCoordinate(v Value) (interface{}, string) {
	switch x := v.(type) {
	case nil:
		return nil, ""
	case int:
		return float64(x), "number"
	case float64:
		return x, "number"
	case *DateV:
		return float64(x.T.UnixMilli()) / 1000, "date"
	case string:
		return x, "category"
	case *EnumV:
		return describe(x), "category"
	}
	if f, ok := toFloat(v); ok {
		return f, "number"
	}
	return describe(v), "category"
}

func (r *Render) collectChartMarks(v Value, chart *chartSpec, inherited chartMark) {
	switch x := v.(type) {
	case nil, *VoidV:
		return
	case *ModV:
		mark := inherited
		switch x.Name {
		case "foregroundStyle":
			if by, ok := argNamed(x.Args, "by"); ok {
				value, _, label := r.plottableValue(by)
				mark.series = describe(value)
				if label != "" {
					chart.seriesLabel = label
				}
			} else if len(x.Args) > 0 {
				mark.color = r.paintColor(x.Args[0].V)
			}
		case "interpolationMethod":
			name := symName(argAt(x.Args, 0))
			mark.smooth = name == "catmullRom" || name == "cardinal" || name == "monotone"
		case "lineStyle":
			mark.dash = true
		}
		r.collectChartMarks(x.Base, chart, mark)
		return
	case *ViewV:
		if kind, isMark := chartMarkKinds[x.Kind]; isMark {
			mark := inherited
			mark.kind = kind
			mark.values = map[string]interface{}{}
			for _, a := range x.Args {
				value, valueKind, label := r.plottableValue(a.V)
				switch a.Label {
				case "x", "xStart", "xEnd":
					mark.values[a.Label] = value
					chart.noteAxis("x", valueKind, label)
				case "y", "yStart", "yEnd":
					mark.values[a.Label] = value
					chart.noteAxis("y", valueKind, label)
				case "width":
					mark.width = markSize(a.V)
				}
			}
			chart.marks = append(chart.marks, mark)
			return
		}
		for _, item := range r.flattenValues(x) {
			if item == v {
				return
			}
			r.collectChartMarks(item, chart, inherited)
		}
	}
}

func markSize(v Value) float64 {
	if s, ok := v.(*SymV); ok && len(s.Parts) == 1 && s.Parts[0].Called && len(s.Parts[0].Args) > 0 {
		f, _ := toFloat(s.Parts[0].Args[0].V)
		return f
	}
	f, _ := toFloat(v)
	return f
}

type chartSpec struct {
	marks         []chartMark
	xKind         string
	yKind         string
	xLabel        string
	yLabel        string
	seriesLabel   string
	xDomain       []interface{}
	yDomain       []interface{}
	yIncludesZero bool
	hideXAxis     bool
	hideYAxis     bool
	hideLegend    bool
}

func (c *chartSpec) noteAxis(axis, kind, label string) {
	if axis == "x" {
		if c.xKind == "" {
			c.xKind = kind
		}
		if c.xLabel == "" {
			c.xLabel = label
		}
		return
	}
	if c.yKind == "" {
		c.yKind = kind
	}
	if c.yLabel == "" {
		c.yLabel = label
	}
}

func (c *chartSpec) toJSON() map[string]interface{} {
	marks := make([]interface{}, 0, len(c.marks))
	for _, m := range c.marks {
		marks = append(marks, m.toJSON())
	}
	out := map[string]interface{}{
		"marks": marks, "xKind": c.xKind, "yKind": c.yKind, "xLabel": c.xLabel, "yLabel": c.yLabel,
		"includesZero": c.yIncludesZero, "hideX": c.hideXAxis, "hideY": c.hideYAxis, "hideLegend": c.hideLegend,
	}
	if c.seriesLabel != "" {
		out["seriesLabel"] = c.seriesLabel
	}
	if c.xDomain != nil {
		out["xDomain"] = c.xDomain
	}
	if c.yDomain != nil {
		out["yDomain"] = c.yDomain
	}
	return out
}

func (r *Render) chartView(v *ViewV, path string) []*RNode {
	chart := &chartSpec{yIncludesZero: true}
	content := v.closure("content")
	if data, ok := v.arg(""); ok && content != nil {
		for _, element := range r.ip.iterate(data, v.Pos) {
			r.collectChartMarks(r.callBuilder(content, element), chart, chartMark{})
		}
	} else if content != nil {
		r.collectChartMarks(r.callBuilder(content), chart, chartMark{})
	}
	return []*RNode{{T: "chart", ID: path, P: chart.toJSON()}}
}

// Scale domains: `.automatic(includesZero: false)`, a ClosedRange, or an array
// of category names.
func (r *Render) chartDomain(v Value) ([]interface{}, bool, bool) {
	switch x := v.(type) {
	case *SymV:
		if x.Name() == "automatic" {
			includesZero := true
			for _, part := range x.Parts {
				if part.Name == "automatic" {
					if flag, ok := argNamed(part.Args, "includesZero"); ok {
						includesZero = flag == true
					}
				}
			}
			return nil, includesZero, true
		}
	case *RangeV:
		lo, _ := plotCoordinate(x.Lo)
		hi, _ := plotCoordinate(x.Hi)
		return []interface{}{lo, hi}, true, true
	case *ArrayV:
		out := make([]interface{}, 0, len(x.E))
		for _, e := range x.E {
			value, _ := plotCoordinate(e)
			out = append(out, value)
		}
		return out, true, true
	}
	return nil, true, false
}

func (r *Render) applyChartModifier(nodes []*RNode, name string, args []ArgV) {
	for _, n := range nodes {
		if n.T != "chart" {
			continue
		}
		switch name {
		case "chartXScale", "chartYScale":
			domainValue, ok := argNamed(args, "domain")
			if !ok {
				continue
			}
			domain, includesZero, known := r.chartDomain(domainValue)
			if !known {
				continue
			}
			if name == "chartXScale" {
				if domain != nil {
					n.P["xDomain"] = domain
				}
			} else {
				if domain != nil {
					n.P["yDomain"] = domain
				}
				n.P["includesZero"] = includesZero
			}
		case "chartXAxis":
			n.P["hideX"] = symName(argAt(args, 0)) == "hidden"
		case "chartYAxis":
			n.P["hideY"] = symName(argAt(args, 0)) == "hidden"
		case "chartLegend":
			n.P["hideLegend"] = symName(argAt(args, 0)) == "hidden"
		}
	}
}

func isChartModifier(name string) bool {
	return strings.HasPrefix(name, "chart")
}
