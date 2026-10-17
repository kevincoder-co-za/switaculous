package switaculous

import "time"

// The preview's Calendar is always Gregorian with weeks starting on Sunday
// (firstWeekday 1), matching Foundation's en_US behaviour.
const previewFirstWeekday = 1

var calendarMethodNames = map[string]bool{
	"component": true, "dateComponents": true, "date": true, "isDateInToday": true, "isDateInYesterday": true,
	"isDateInTomorrow": true, "isDateInWeekend": true, "isDate": true, "startOfDay": true, "range": true,
	"dateInterval": true, "nextDate": true, "compare": true,
}

func calendarProperty(name string) (Value, bool) {
	switch name {
	case "firstWeekday":
		return previewFirstWeekday, true
	case "weekdaySymbols", "standaloneWeekdaySymbols":
		return stringArray(weekdaySymbols(0)), true
	case "shortWeekdaySymbols", "shortStandaloneWeekdaySymbols":
		return stringArray(weekdaySymbols(3)), true
	case "veryShortWeekdaySymbols", "veryShortStandaloneWeekdaySymbols":
		return stringArray(weekdaySymbols(1)), true
	case "monthSymbols", "standaloneMonthSymbols":
		return stringArray(monthSymbols(0)), true
	case "shortMonthSymbols", "shortStandaloneMonthSymbols":
		return stringArray(monthSymbols(3)), true
	case "veryShortMonthSymbols", "veryShortStandaloneMonthSymbols":
		return stringArray(monthSymbols(1)), true
	case "identifier":
		return sym("gregorian"), true
	case "timeZone":
		return newObj("TimeZone"), true
	case "locale":
		return newObj("Locale"), true
	case "minimumDaysInFirstWeek":
		return 1, true
	}
	return nil, false
}

func weekdaySymbols(width int) []string {
	names := make([]string, 7)
	for i := 0; i < 7; i++ {
		full := time.Weekday(i).String()
		if width > 0 && width < len(full) {
			full = full[:width]
		}
		names[i] = full
	}
	return names
}

func monthSymbols(width int) []string {
	names := make([]string, 12)
	for i := 0; i < 12; i++ {
		full := time.Month(i + 1).String()
		if width > 0 && width < len(full) {
			full = full[:width]
		}
		names[i] = full
	}
	return names
}

func stringArray(items []string) *ArrayV {
	out := &ArrayV{Elem: &TypeRef{Name: "String"}}
	for _, item := range items {
		out.E = append(out.E, item)
	}
	return out
}

func (ip *Interp) dateArg(args []ArgV, label string, index int) (time.Time, bool) {
	var v Value
	if label != "" {
		v, _ = argNamed(args, label)
	}
	if v == nil && index >= 0 && index < len(args) {
		v = args[index].V
	}
	if s, ok := v.(*SymV); ok {
		v = ip.resolveSymAs("Date", s)
	}
	d, ok := v.(*DateV)
	if !ok {
		return time.Time{}, false
	}
	return d.T, true
}

func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

func startOfUnit(t time.Time, unit string) (time.Time, time.Time, bool) {
	y, m, d := t.Date()
	loc := t.Location()
	switch unit {
	case "day":
		start := time.Date(y, m, d, 0, 0, 0, 0, loc)
		return start, start.AddDate(0, 0, 1), true
	case "weekOfYear", "weekOfMonth":
		dayStart := time.Date(y, m, d, 0, 0, 0, 0, loc)
		offset := (int(dayStart.Weekday()) - (previewFirstWeekday - 1) + 7) % 7
		start := dayStart.AddDate(0, 0, -offset)
		return start, start.AddDate(0, 0, 7), true
	case "month":
		start := time.Date(y, m, 1, 0, 0, 0, 0, loc)
		return start, start.AddDate(0, 1, 0), true
	case "year":
		start := time.Date(y, 1, 1, 0, 0, 0, 0, loc)
		return start, start.AddDate(1, 0, 0), true
	case "hour":
		start := time.Date(y, m, d, t.Hour(), 0, 0, 0, loc)
		return start, start.Add(time.Hour), true
	case "minute":
		start := time.Date(y, m, d, t.Hour(), t.Minute(), 0, 0, loc)
		return start, start.Add(time.Minute), true
	}
	return time.Time{}, time.Time{}, false
}

func dateComponentValue(t time.Time, unit string) (int, bool) {
	switch unit {
	case "era":
		return 1, true
	case "year":
		return t.Year(), true
	case "month":
		return int(t.Month()), true
	case "day":
		return t.Day(), true
	case "hour":
		return t.Hour(), true
	case "minute":
		return t.Minute(), true
	case "second":
		return t.Second(), true
	case "nanosecond":
		return t.Nanosecond(), true
	case "weekday":
		return int(t.Weekday()) + 1, true
	case "weekdayOrdinal":
		return (t.Day()-1)/7 + 1, true
	case "weekOfYear":
		_, week := t.ISOWeek()
		return week, true
	case "weekOfMonth":
		monthStart, _, _ := startOfUnit(t, "month")
		weekStart, _, _ := startOfUnit(monthStart, "weekOfYear")
		return int(t.Sub(weekStart).Hours()/24)/7 + 1, true
	case "quarter":
		return (int(t.Month())-1)/3 + 1, true
	}
	return 0, false
}

func addToDate(t time.Time, unit string, n int) time.Time {
	switch unit {
	case "day":
		return t.AddDate(0, 0, n)
	case "weekOfYear", "weekOfMonth", "weekday":
		return t.AddDate(0, 0, 7*n)
	case "month":
		return t.AddDate(0, n, 0)
	case "year":
		return t.AddDate(n, 0, 0)
	case "hour":
		return t.Add(time.Duration(n) * time.Hour)
	case "minute":
		return t.Add(time.Duration(n) * time.Minute)
	case "second":
		return t.Add(time.Duration(n) * time.Second)
	}
	return t
}

func componentInt(o *NativeObj, name string, fallback int) int {
	if v, ok := o.Props[name]; ok && v != nil {
		if n, ok := toInt(v); ok {
			return n
		}
	}
	return fallback
}

// Builds a Date from DateComponents the way Calendar.date(from:) does:
// missing fields default to the start of the epoch-era day, so components
// holding only hour/minute give a time on 1 January 2001 like Foundation.
func dateFromComponents(o *NativeObj) time.Time {
	year := componentInt(o, "year", 2001)
	month := componentInt(o, "month", 1)
	day := componentInt(o, "day", 1)
	hour := componentInt(o, "hour", 0)
	minute := componentInt(o, "minute", 0)
	second := componentInt(o, "second", 0)
	return time.Date(year, time.Month(month), day, hour, minute, second, 0, time.Local)
}

func componentsOf(t time.Time, units []Value) *NativeObj {
	o := newObj("DateComponents")
	for _, u := range units {
		s, ok := u.(*SymV)
		if !ok {
			continue
		}
		if v, ok := dateComponentValue(t, s.Name()); ok {
			o.Props[s.Name()] = v
		}
	}
	return o
}

// Walks forward one minute at a time until a date matches the components,
// which is plenty for the hour/minute/weekday matching reminder apps use.
func nextMatchingDate(after time.Time, match *NativeObj) Value {
	candidate := after.Truncate(time.Minute).Add(time.Minute)
	limit := after.AddDate(1, 0, 0)
	for candidate.Before(limit) {
		if componentsMatch(candidate, match) {
			return &DateV{T: candidate}
		}
		candidate = candidate.Add(time.Minute)
	}
	return nil
}

func componentsMatch(t time.Time, match *NativeObj) bool {
	for name, v := range match.Props {
		if v == nil {
			continue
		}
		want, ok := toInt(v)
		if !ok {
			continue
		}
		have, known := dateComponentValue(t, name)
		if known && have != want {
			return false
		}
	}
	return true
}

func (ip *Interp) calendarMethod(name string, args []ArgV, pos Pos) (Value, bool) {
	switch name {
	case "component":
		unit, _ := argAt(args, 0).(*SymV)
		t, ok := ip.dateArg(args, "from", -1)
		if !ok || unit == nil {
			return 0, true
		}
		v, _ := dateComponentValue(t, unit.Name())
		return v, true
	case "dateComponents":
		t, ok := ip.dateArg(args, "from", -1)
		if !ok {
			return newObj("DateComponents"), true
		}
		var units []Value
		switch set := argAt(args, 0).(type) {
		case *SetV:
			units = set.Keys
		case *ArrayV:
			units = set.E
		}
		return componentsOf(t, units), true
	case "date":
		if comps, ok := argNamed(args, "from"); ok {
			if o, ok := comps.(*NativeObj); ok {
				return &DateV{T: dateFromComponents(o)}, true
			}
			return nil, true
		}
		if hourV, ok := argNamed(args, "bySettingHour"); ok {
			t, ok := ip.dateArg(args, "of", -1)
			if !ok {
				return nil, true
			}
			hour, _ := toInt(hourV)
			minuteV, _ := argNamed(args, "minute")
			minute, _ := toInt(minuteV)
			secondV, _ := argNamed(args, "second")
			second, _ := toInt(secondV)
			y, m, d := t.Date()
			return &DateV{T: time.Date(y, m, d, hour, minute, second, 0, t.Location())}, true
		}
		if comps, ok := argNamed(args, "byAdding"); ok {
			t, ok := ip.dateArg(args, "to", -1)
			if !ok {
				return nil, true
			}
			if unit, ok := comps.(*SymV); ok {
				val, _ := argNamed(args, "value")
				n, _ := toInt(val)
				return &DateV{T: addToDate(t, unit.Name(), n)}, true
			}
			if o, ok := comps.(*NativeObj); ok {
				for unit, v := range o.Props {
					if n, ok := toInt(v); ok {
						t = addToDate(t, unit, n)
					}
				}
				return &DateV{T: t}, true
			}
			return nil, true
		}
		return nil, true
	case "isDateInToday", "isDateInYesterday", "isDateInTomorrow":
		t, ok := ip.dateArg(args, "", 0)
		if !ok {
			return false, true
		}
		reference := startOfDay(time.Now())
		if name == "isDateInYesterday" {
			reference = reference.AddDate(0, 0, -1)
		} else if name == "isDateInTomorrow" {
			reference = reference.AddDate(0, 0, 1)
		}
		return startOfDay(t).Equal(reference), true
	case "isDateInWeekend":
		t, ok := ip.dateArg(args, "", 0)
		if !ok {
			return false, true
		}
		return t.Weekday() == time.Saturday || t.Weekday() == time.Sunday, true
	case "isDate":
		t1, ok1 := ip.dateArg(args, "", 0)
		if other, ok := argNamed(args, "inSameDayAs"); ok {
			t2, ok2 := other.(*DateV)
			return ok1 && ok2 && startOfDay(t1).Equal(startOfDay(t2.T)), true
		}
		other, _ := argNamed(args, "equalTo")
		t2, ok2 := other.(*DateV)
		granularity, _ := argNamed(args, "toGranularity")
		unit, _ := granularity.(*SymV)
		if !ok1 || !ok2 || unit == nil {
			return false, true
		}
		s1, _, known := startOfUnit(t1, unit.Name())
		s2, _, _ := startOfUnit(t2.T, unit.Name())
		return known && s1.Equal(s2), true
	case "compare":
		t1, ok1 := ip.dateArg(args, "", 0)
		t2, ok2 := ip.dateArg(args, "to", -1)
		granularity, _ := argNamed(args, "toGranularity")
		unit, _ := granularity.(*SymV)
		if !ok1 || !ok2 || unit == nil {
			return sym("orderedSame"), true
		}
		s1, _, _ := startOfUnit(t1, unit.Name())
		s2, _, _ := startOfUnit(t2, unit.Name())
		switch {
		case s1.Before(s2):
			return sym("orderedAscending"), true
		case s1.After(s2):
			return sym("orderedDescending"), true
		}
		return sym("orderedSame"), true
	case "startOfDay":
		t, ok := ip.dateArg(args, "for", 0)
		if !ok {
			return nil, true
		}
		return &DateV{T: startOfDay(t)}, true
	case "range":
		smaller, _ := argAt(args, 0).(*SymV)
		larger, _ := argNamed(args, "in")
		largerUnit, _ := larger.(*SymV)
		t, ok := ip.dateArg(args, "for", -1)
		if !ok || smaller == nil || largerUnit == nil {
			return nil, true
		}
		start, end, known := startOfUnit(t, largerUnit.Name())
		if !known {
			return nil, true
		}
		count := 0
		for cursor := start; cursor.Before(end); cursor = addToDate(cursor, smaller.Name(), 1) {
			count++
		}
		lo := 1
		if smaller.Name() == "hour" || smaller.Name() == "minute" || smaller.Name() == "second" {
			lo = 0
		}
		return &RangeV{Lo: lo, Hi: lo + count}, true
	case "dateInterval":
		unit, _ := argAt(args, 0).(*SymV)
		t, ok := ip.dateArg(args, "for", -1)
		if !ok || unit == nil {
			return nil, true
		}
		start, end, known := startOfUnit(t, unit.Name())
		if !known {
			return nil, true
		}
		interval := newObj("DateInterval")
		interval.Props["start"] = &DateV{T: start}
		interval.Props["end"] = &DateV{T: end}
		interval.Props["duration"] = end.Sub(start).Seconds()
		return interval, true
	case "nextDate":
		t, ok := ip.dateArg(args, "after", -1)
		match, _ := argNamed(args, "matching")
		comps, isComps := match.(*NativeObj)
		if !ok || !isComps {
			return nil, true
		}
		return nextMatchingDate(t, comps), true
	}
	return nil, false
}
