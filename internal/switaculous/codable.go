package switaculous

import (
	"regexp"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Foundation counts dates from 1 January 2001 unless a strategy says otherwise.
var foundationReferenceDate = time.Date(2001, 1, 1, 0, 0, 0, 0, time.UTC)

const userDefaultsFile = "UserDefaults.json"

type jsonCoder struct {
	ip           *Interp
	dateStrategy Value
	keyStrategy  string
	pos          Pos
}

type orderedJSONObject struct {
	keys   []string
	values map[string]interface{}
}

func (o *orderedJSONObject) set(key string, value interface{}) {
	if _, exists := o.values[key]; !exists {
		o.keys = append(o.keys, key)
	}
	o.values[key] = value
}

func (o *orderedJSONObject) MarshalJSON() ([]byte, error) {
	var out bytes.Buffer
	out.WriteByte('{')
	for i, key := range o.keys {
		if i > 0 {
			out.WriteByte(',')
		}
		encodedKey, _ := json.Marshal(key)
		out.Write(encodedKey)
		out.WriteByte(':')
		encodedValue, err := json.Marshal(o.values[key])
		if err != nil {
			return nil, err
		}
		out.Write(encodedValue)
	}
	out.WriteByte('}')
	return out.Bytes(), nil
}

func newCodingError(kind, message string) *NativeObj {
	err := newObj(kind)
	err.Props["localizedDescription"] = message
	err.Props["description"] = message
	return err
}

func throwCodingError(kind, message string, pos Pos) {
	panic(&ThrowSignal{V: newCodingError(kind, message), Pos: pos})
}

// Foundation's localizedDescription for decoding errors is a generic sentence;
// the detailed reason only shows when the error itself is printed.
func throwDecodingError(detail string, pos Pos) {
	err := newCodingError("DecodingError", detail)
	if strings.HasPrefix(detail, "keyNotFound") || strings.HasPrefix(detail, "valueNotFound") {
		err.Props["localizedDescription"] = "The data couldn’t be read because it is missing."
	} else {
		err.Props["localizedDescription"] = "The data couldn’t be read because it isn’t in the correct format."
	}
	panic(&ThrowSignal{V: err, Pos: pos})
}

func (ip *Interp) newJSONCoder(coder *NativeObj, pos Pos) *jsonCoder {
	c := &jsonCoder{ip: ip, pos: pos}
	c.dateStrategy = coder.Props["dateEncodingStrategy"]
	if c.dateStrategy == nil {
		c.dateStrategy = coder.Props["dateDecodingStrategy"]
	}
	for _, name := range []string{"keyEncodingStrategy", "keyDecodingStrategy"} {
		if strategy := symName(coder.Props[name]); strategy != "" {
			c.keyStrategy = strategy
		}
	}
	return c
}

// ---------- the Swift-visible objects ----------

func (ip *Interp) constructCodableType(name string, args []ArgV, pos Pos) (Value, bool) {
	switch name {
	case "JSONEncoder", "JSONDecoder", "PropertyListEncoder", "PropertyListDecoder":
		coder := newObj(name)
		coder.Props["outputFormatting"] = &ArrayV{}
		return coder, true
	case "FileManager":
		return newObj("FileManager"), true
	case "ImageRenderer":
		renderer := newObj("ImageRenderer")
		renderer.Props["content"], _ = argNamed(args, "content")
		renderer.Props["scale"] = 3.0
		renderer.Props["isOpaque"] = false
		renderer.Props["proposedSize"] = sym("unspecified")
		renderer.Props["uiImage"] = nil
		renderer.Props["cgImage"] = nil
		return renderer, true
	}
	return nil, false
}

func (ip *Interp) codableStatic(typeName, name string) (Value, bool) {
	switch typeName + "." + name {
	case "UserDefaults.standard":
		return newObj("UserDefaults"), true
	case "Bundle.main":
		return newObj("Bundle"), true
	case "FileManager.default":
		return newObj("FileManager"), true
	}
	return nil, false
}

func (ip *Interp) codableNativeObjMethod(o *NativeObj, name string, args []ArgV, pos Pos) (Value, bool) {
	switch o.Kind {
	case "JSONEncoder", "PropertyListEncoder":
		if name == "encode" {
			return ip.newJSONCoder(o, pos).encodeToData(argAt(args, 0), o), true
		}
	case "JSONDecoder", "PropertyListDecoder":
		if name == "decode" {
			data, _ := mustArg(args, "from").(*DataV)
			if data == nil {
				throwDecodingError("dataCorrupted: decode(_:from:) needs Data.", pos)
			}
			return ip.newJSONCoder(o, pos).decodeFromData(typeRefFromValue(argAt(args, 0)), data), true
		}
	case "UserDefaults":
		return ip.userDefaultsMethod(name, args, pos)
	case "Bundle":
		return ip.bundleMethod(name, args, pos)
	case "FileManager":
		return ip.fileManagerMethod(name, args, pos)
	case "ImageRenderer":
		if name == "render" {
			ip.warn(pos, "ImageRenderer.render does nothing in the preview; the drawing callback is never called")
			return Void, true
		}
	}
	return nil, false
}

func typeRefFromValue(v Value) *TypeRef {
	switch x := v.(type) {
	case *TypeV:
		if x.Ref != nil {
			return x.Ref
		}
		return &TypeRef{Name: x.Name}
	case *ArrayV:
		if len(x.E) == 1 {
			if elem := typeRefFromValue(x.E[0]); elem != nil {
				return &TypeRef{Kind: tyArray, Elem: elem}
			}
		}
	case *DictV:
		if len(x.Keys) == 1 {
			key, value := typeRefFromValue(x.Keys[0]), typeRefFromValue(x.Vals[0])
			if key != nil && value != nil {
				return &TypeRef{Kind: tyDict, Key: key, Elem: value}
			}
		}
	}
	return nil
}

// ---------- encoding ----------

func (c *jsonCoder) encodeToData(v Value, coder *NativeObj) Value {
	encoded := c.encodeValue(v, "")
	var out []byte
	var err error
	formatting, _ := coder.Props["outputFormatting"].(*ArrayV)
	pretty := false
	if formatting != nil {
		for _, option := range formatting.E {
			pretty = pretty || symName(option) == "prettyPrinted"
		}
	} else if symName(coder.Props["outputFormatting"]) == "prettyPrinted" {
		pretty = true
	}
	if pretty {
		out, err = json.MarshalIndent(encoded, "", "  ")
	} else {
		out, err = json.Marshal(encoded)
	}
	if err != nil {
		throwCodingError("EncodingError", err.Error(), c.pos)
	}
	return &DataV{Bytes: out}
}

func (c *jsonCoder) encodeValue(v Value, fieldName string) interface{} {
	switch x := v.(type) {
	case nil:
		return nil
	case bool, string, int:
		return x
	case Character:
		return string(x)
	case float64:
		if math.IsInf(x, 0) || math.IsNaN(x) {
			throwCodingError("EncodingError", "Unable to encode Double."+describe(x)+" directly in JSON.", c.pos)
		}
		return storedFloat(x)
	case *DateV:
		return c.encodeDate(x)
	case *UUIDV:
		return x.S
	case *URLV:
		return x.S
	case *DataV:
		return base64.StdEncoding.EncodeToString(x.Bytes)
	case *ArrayV:
		list := make([]interface{}, len(x.E))
		for i, e := range x.E {
			list[i] = c.encodeValue(e, fieldName)
		}
		return list
	case *SetV:
		list := make([]interface{}, len(x.Keys))
		for i, e := range x.Keys {
			list[i] = c.encodeValue(e, fieldName)
		}
		return list
	case *DictV:
		object := &orderedJSONObject{values: map[string]interface{}{}}
		for i, key := range x.Keys {
			object.set(describe(key), c.encodeValue(x.Vals[i], fieldName))
		}
		return object
	case *EnumV:
		if x.T.RawType != "" {
			return c.encodeValue(x.T.RawVals[x.Case], fieldName)
		}
		if len(x.Assoc) == 0 {
			return x.Case
		}
		c.ip.fail(c.pos, "cannot encode '%s.%s': enums with associated values need a custom encode(to:)", x.T.Name, x.Case)
	case *StructV:
		return c.encodeFields(x.T, x.F)
	case *ObjectV:
		return c.encodeFields(x.T, x.F)
	case *BoxV:
		return c.encodeValue(x.V, fieldName)
	}
	c.ip.fail(c.pos, "value of type '%s' cannot be encoded as JSON (it does not conform to Encodable)", typeName(v))
	return nil
}

func (c *jsonCoder) encodeFields(ti *TypeInfo, fields map[string]Value) interface{} {
	if c.ip.findMethods(ti, "encode") != nil {
		c.ip.fail(c.pos, "'%s' has a custom encode(to:), which the preview can't run; remove it to use the synthesized encoding", ti.Name)
	}
	object := &orderedJSONObject{values: map[string]interface{}{}}
	keys := c.ip.codingKeys(ti)
	for _, f := range ti.allFields() {
		if f.Wrapper != "" && f.Wrapper != "Published" {
			continue
		}
		key, included := c.jsonKey(ti, keys, f.Name)
		if !included {
			continue
		}
		value := fields[f.Name]
		if box, ok := value.(*BoxV); ok {
			value = box.V
		}
		if value == nil {
			continue
		}
		object.set(key, c.encodeValue(value, f.Name))
	}
	return object
}

func (c *jsonCoder) encodeDate(d *DateV) interface{} {
	switch strategy := c.dateStrategy.(type) {
	case *SymV:
		switch strategy.Name() {
		case "iso8601":
			return d.T.UTC().Format("2006-01-02T15:04:05Z")
		case "secondsSince1970":
			return storedFloat(float64(d.T.UnixNano()) / 1e9)
		case "millisecondsSince1970":
			return storedFloat(float64(d.T.UnixNano()) / 1e6)
		case "formatted":
			if formatter, ok := argAt(strategy.Args(), 0).(*NativeObj); ok {
				return describe(c.ip.nativeObjMethod(formatter, "string", []ArgV{{Label: "from", V: d}}, c.pos, nil))
			}
		}
	}
	return storedFloat(d.T.Sub(foundationReferenceDate).Seconds())
}

// A nested `enum CodingKeys: String, CodingKey` renames or drops properties.
func (ip *Interp) codingKeys(ti *TypeInfo) *TypeInfo {
	for _, candidate := range []string{ti.Name + ".CodingKeys", "CodingKeys"} {
		if keys, ok := ip.types[candidate]; ok && keys.Kind == "enum" && keys.Outer == ti.Name {
			return keys
		}
	}
	return nil
}

func (c *jsonCoder) jsonKey(ti *TypeInfo, keys *TypeInfo, fieldName string) (string, bool) {
	if keys != nil {
		caseInfo, listed := keys.CaseMap[fieldName]
		if !listed {
			return "", false
		}
		if raw, ok := keys.RawVals[caseInfo.Name].(string); ok {
			return raw, true
		}
		return fieldName, true
	}
	if c.keyStrategy == "convertToSnakeCase" || c.keyStrategy == "convertFromSnakeCase" {
		return snakeCase(fieldName), true
	}
	return fieldName, true
}

func snakeCase(name string) string {
	var out strings.Builder
	runes := []rune(name)
	for i, r := range runes {
		if unicode.IsUpper(r) {
			previousIsLower := i > 0 && unicode.IsLower(runes[i-1])
			nextIsLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if i > 0 && (previousIsLower || nextIsLower) {
				out.WriteByte('_')
			}
			out.WriteRune(unicode.ToLower(r))
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}

// ---------- decoding ----------

func (c *jsonCoder) decodeFromData(t *TypeRef, data *DataV) Value {
	if t == nil {
		c.ip.fail(c.pos, "decode(_:from:) needs a type, for example [Astronaut].self")
	}
	decoder := json.NewDecoder(bytes.NewReader(data.Bytes))
	decoder.UseNumber()
	var parsed interface{}
	if err := decoder.Decode(&parsed); err != nil {
		throwDecodingError("dataCorrupted: The given data was not valid JSON.", c.pos)
	}
	return c.decodeValue(parsed, t, "")
}

func (c *jsonCoder) decodeValue(j interface{}, t *TypeRef, path string) Value {
	ip := c.ip
	if j == nil {
		if t.Opt {
			return nil
		}
		throwDecodingError(fmt.Sprintf("valueNotFound: expected %s at '%s' but found null instead.", t.String(), keyPathOrRoot(path)), c.pos)
	}
	switch t.Kind {
	case tyArray:
		list, ok := j.([]interface{})
		if !ok {
			c.typeMismatch(t, j, path)
		}
		array := &ArrayV{Elem: t.Elem}
		for i, e := range list {
			array.E = append(array.E, c.decodeValue(e, t.Elem, path+"["+strconv.Itoa(i)+"]"))
		}
		return array
	case tyDict:
		object, ok := j.(map[string]interface{})
		if !ok {
			c.typeMismatch(t, j, path)
		}
		dict := newDict()
		dict.KeyT, dict.ValT = t.Key, t.Elem
		keys := make([]string, 0, len(object))
		for key := range object {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			var dictKey Value = key
			if t.Key != nil && ip.resolveAlias(t.Key.Name) == "Int" {
				dictKey, _ = strconv.Atoi(key)
			}
			dict.Set(dictKey, c.decodeValue(object[key], t.Elem, path+"."+key))
		}
		return dict
	case tyNamed:
		return c.decodeNamed(j, t, path)
	}
	c.typeMismatch(t, j, path)
	return nil
}

func (c *jsonCoder) decodeNamed(j interface{}, t *TypeRef, path string) Value {
	ip := c.ip
	name := ip.resolveAlias(t.Name)
	if alias, ok := ip.aliases[t.Name]; ok && alias.Kind != tyNamed {
		return c.decodeValue(j, alias, path)
	}
	number, isNumber := j.(json.Number)
	text, isText := j.(string)
	switch name {
	case "Int", "Int8", "Int16", "Int32", "Int64", "UInt", "UInt8", "UInt16", "UInt32", "UInt64":
		if isNumber {
			if n, err := number.Int64(); err == nil {
				return int(n)
			}
		}
	case "Double", "Float", "CGFloat", "TimeInterval", "Float32", "Float64":
		if isNumber {
			f, _ := number.Float64()
			return f
		}
	case "Bool":
		if b, ok := j.(bool); ok {
			return b
		}
	case "String":
		if isText {
			return text
		}
	case "Character":
		if isText && len(text) > 0 {
			return Character(text)
		}
	case "UUID":
		if isText {
			return &UUIDV{S: text}
		}
	case "URL":
		if isText {
			return &URLV{S: text}
		}
	case "Data":
		if isText {
			if decoded, err := base64.StdEncoding.DecodeString(text); err == nil {
				return &DataV{Bytes: decoded}
			}
		}
	case "Date":
		if date := c.decodeDate(j); date != nil {
			return date
		}
	case "Array":
		if len(t.Args) == 1 {
			return c.decodeValue(j, &TypeRef{Kind: tyArray, Elem: t.Args[0]}, path)
		}
	case "Dictionary":
		if len(t.Args) == 2 {
			return c.decodeValue(j, &TypeRef{Kind: tyDict, Key: t.Args[0], Elem: t.Args[1]}, path)
		}
	case "Set":
		if len(t.Args) == 1 {
			array := c.decodeValue(j, &TypeRef{Kind: tyArray, Elem: t.Args[0]}, path).(*ArrayV)
			set := newSet()
			set.Elem = t.Args[0]
			for _, e := range array.E {
				set.Add(e)
			}
			return set
		}
	default:
		if ti := ip.lookupTypeInfo(name); ti != nil {
			return c.decodeUserType(j, ti, path)
		}
		ip.fail(c.pos, "cannot decode a value of type '%s' (it does not conform to Decodable)", name)
	}
	c.typeMismatch(t, j, path)
	return nil
}

func (c *jsonCoder) decodeUserType(j interface{}, ti *TypeInfo, path string) Value {
	ip := c.ip
	if ti.Kind == "enum" {
		if ti.RawType == "" {
			ip.fail(c.pos, "cannot decode '%s': enums with associated values need a custom init(from:)", ti.Name)
		}
		raw := c.decodeValue(j, &TypeRef{Name: ti.RawType}, path)
		for _, enumCase := range ti.Cases {
			if valueEquals(ti.RawVals[enumCase.Name], raw) {
				return &EnumV{T: ti, Case: enumCase.Name}
			}
		}
		throwDecodingError(fmt.Sprintf("dataCorrupted: Cannot initialize %s from invalid %s value %s at '%s'.", ti.Name, ti.RawType, describe(raw), keyPathOrRoot(path)), c.pos)
	}
	if ti.Kind != "struct" && ti.Kind != "class" {
		ip.fail(c.pos, "cannot decode a value of type '%s'", ti.Name)
	}
	for _, init := range ti.Inits {
		if len(init.Params) == 1 && init.Params[0].Label == "from" {
			ip.fail(c.pos, "'%s' has a custom init(from:), which the preview can't run; remove it to use the synthesized decoding", ti.Name)
		}
	}
	object, ok := j.(map[string]interface{})
	if !ok {
		c.typeMismatch(&TypeRef{Name: ti.Name}, j, path)
	}
	instance := ip.newInstance(ti)
	fields := fieldsOf(instance)
	keys := ip.codingKeys(ti)
	for _, f := range ti.allFields() {
		if f.Wrapper != "" && f.Wrapper != "Published" {
			continue
		}
		key, included := c.jsonKey(ti, keys, f.Name)
		if !included {
			continue
		}
		raw, present := object[key]
		fieldType := f.Type
		if fieldType == nil {
			fieldType = inferTypeRef(fields[f.Name])
		}
		if !present {
			if fieldType != nil && fieldType.Opt {
				fields[f.Name] = nil
				continue
			}
			throwDecodingError(fmt.Sprintf("keyNotFound: No value associated with key '%s' in %s at '%s'.", key, ti.Name, keyPathOrRoot(path)), c.pos)
		}
		if fieldType == nil {
			ip.fail(c.pos, "cannot decode '%s.%s' without a declared type (write 'var %s: SomeType = ...')", ti.Name, f.Name, f.Name)
		}
		value := c.decodeValue(raw, fieldType, path+"."+key)
		if box, isBox := fields[f.Name].(*BoxV); isBox {
			box.V = value
		} else {
			fields[f.Name] = value
		}
	}
	return instance
}

func inferTypeRef(v Value) *TypeRef {
	switch x := v.(type) {
	case int:
		return &TypeRef{Name: "Int"}
	case float64:
		return &TypeRef{Name: "Double"}
	case bool:
		return &TypeRef{Name: "Bool"}
	case string:
		return &TypeRef{Name: "String"}
	case *UUIDV:
		return &TypeRef{Name: "UUID"}
	case *DateV:
		return &TypeRef{Name: "Date"}
	case *StructV:
		return &TypeRef{Name: x.T.Name}
	case *ObjectV:
		return &TypeRef{Name: x.T.Name}
	case *EnumV:
		return &TypeRef{Name: x.T.Name}
	case *BoxV:
		return inferTypeRef(x.V)
	}
	return nil
}

func (c *jsonCoder) decodeDate(j interface{}) Value {
	number, isNumber := j.(json.Number)
	text, isText := j.(string)
	if strategy, ok := c.dateStrategy.(*SymV); ok {
		switch strategy.Name() {
		case "iso8601":
			if isText {
				for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05Z", "2006-01-02T15:04:05"} {
					if t, err := time.Parse(layout, text); err == nil {
						return &DateV{T: t.Local()}
					}
				}
			}
			return nil
		case "secondsSince1970":
			if isNumber {
				f, _ := number.Float64()
				return &DateV{T: time.Unix(0, int64(f*1e9))}
			}
			return nil
		case "millisecondsSince1970":
			if isNumber {
				f, _ := number.Float64()
				return &DateV{T: time.Unix(0, int64(f*1e6))}
			}
			return nil
		case "formatted":
			if formatter, ok := argAt(strategy.Args(), 0).(*NativeObj); ok && isText {
				if date, ok := c.ip.nativeObjMethod(formatter, "date", []ArgV{{Label: "from", V: text}}, c.pos, nil).(*DateV); ok {
					return date
				}
			}
			return nil
		}
	}
	if isNumber {
		f, _ := number.Float64()
		return &DateV{T: foundationReferenceDate.Add(time.Duration(f * float64(time.Second))).Local()}
	}
	return nil
}

func (c *jsonCoder) typeMismatch(t *TypeRef, j interface{}, path string) {
	found := "a value"
	switch j.(type) {
	case string:
		found = "a string"
	case json.Number:
		found = "a number"
	case bool:
		found = "a boolean"
	case []interface{}:
		found = "an array"
	case map[string]interface{}:
		found = "a dictionary"
	}
	throwDecodingError(fmt.Sprintf("typeMismatch: expected to decode %s at '%s' but found %s instead.", t.String(), keyPathOrRoot(path), found), c.pos)
}

func keyPathOrRoot(path string) string {
	if path == "" {
		return "(root)"
	}
	return strings.TrimPrefix(path, ".")
}

// ---------- UserDefaults ----------

func (ip *Interp) userDefaultsBox(key string) *BoxV {
	if ip.store == nil {
		return &BoxV{Kind: "AppStorage", Key: key}
	}
	box := ip.store.appStorage[key]
	if box == nil {
		box = &BoxV{Kind: "AppStorage", Key: key}
		ip.store.appStorage[key] = box
	}
	return box
}

func (ip *Interp) userDefaultsMethod(name string, args []ArgV, pos Pos) (Value, bool) {
	key := describe(mustArg(args, "forKey"))
	switch name {
	case "set", "setValue":
		ip.userDefaultsBox(key).V = copyValue(argAt(args, 0))
		return Void, true
	case "removeObject":
		if ip.store != nil {
			delete(ip.store.appStorage, key)
		}
		return Void, true
	case "register":
		if defaults, ok := mustArg(args, "defaults").(*DictV); ok {
			for i, dictKey := range defaults.Keys {
				box := ip.userDefaultsBox(describe(dictKey))
				if box.V == nil {
					box.V = copyValue(defaults.Vals[i])
				}
			}
		}
		return Void, true
	case "synchronize":
		return true, true
	case "object", "value", "string", "data", "array", "dictionary", "stringArray", "url":
		return ip.userDefaultsBox(key).V, true
	case "integer":
		n, _ := toInt(ip.userDefaultsBox(key).V)
		return n, true
	case "double", "float":
		f, _ := toFloat(ip.userDefaultsBox(key).V)
		return f, true
	case "bool":
		return ip.userDefaultsBox(key).V == true, true
	}
	return nil, false
}

func (rt *Runtime) userDefaultsPath() string {
	return filepath.Join(rt.dir, swiftDataFolder, userDefaultsFile)
}

func (rt *Runtime) loadUserDefaultsLocked() {
	content, err := os.ReadFile(rt.userDefaultsPath())
	if err != nil || rt.ip == nil {
		return
	}
	var stored map[string]json.RawMessage
	if json.Unmarshal(content, &stored) != nil {
		return
	}
	for key, raw := range stored {
		rt.store.appStorage[key] = &BoxV{Kind: "AppStorage", Key: key, V: decodeStoredJSONText(rt.ip, nil, string(raw))}
	}
	rt.userDefaultsSaved = string(content)
}

func (rt *Runtime) saveUserDefaultsLocked() {
	if rt.ip == nil {
		return
	}
	stored := map[string]interface{}{}
	for key, box := range rt.store.appStorage {
		if box.V != nil {
			stored[key] = encodeStoredJSON(nil, box.V)
		}
	}
	content, err := json.MarshalIndent(stored, "", "  ")
	if err != nil || string(content) == rt.userDefaultsSaved {
		return
	}
	if len(stored) == 0 && rt.userDefaultsSaved == "" {
		return
	}
	os.MkdirAll(filepath.Dir(rt.userDefaultsPath()), 0755)
	if os.WriteFile(rt.userDefaultsPath(), content, 0644) == nil {
		rt.userDefaultsSaved = string(content)
	}
}

// ---------- Bundle and file reading ----------

func (ip *Interp) projectResourcePath(fileName string) string {
	if ip.rt == nil || fileName == "" {
		return ""
	}
	found := ""
	filepath.Walk(ip.rt.dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || found != "" {
			return nil
		}
		if info.IsDir() {
			if p != ip.rt.dir && skipDir(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if info.Name() == fileName {
			found = p
		}
		return nil
	})
	return found
}

// Bundle.main in the preview answers for the project folder: the version
// comes from Info.plist when there is one, with Xcode's defaults otherwise.
func (ip *Interp) bundleProperty(name string) (Value, bool) {
	switch name {
	case "infoDictionary":
		info := newDict()
		info.Set("CFBundleShortVersionString", "1.0")
		info.Set("CFBundleVersion", "1")
		info.Set("CFBundleName", ip.projectName())
		info.Set("CFBundleDisplayName", ip.projectName())
		for key, value := range ip.infoPlistStrings() {
			info.Set(key, value)
		}
		return info, true
	case "bundleIdentifier":
		if id, ok := ip.infoPlistStrings()["CFBundleIdentifier"]; ok {
			return id, true
		}
		return "com.example." + strings.ToLower(ip.projectName()), true
	case "bundlePath", "resourcePath":
		if ip.rt != nil {
			return ip.rt.dir, true
		}
		return "", true
	case "bundleURL", "resourceURL":
		if ip.rt != nil {
			return &URLV{S: ip.rt.dir}, true
		}
		return nil, true
	}
	return nil, false
}

func (ip *Interp) projectName() string {
	if ip.rt == nil {
		return "Preview"
	}
	return filepath.Base(ip.rt.dir)
}

var plistStringEntry = regexp.MustCompile(`(?s)<key>([^<]+)</key>\s*<string>([^<]*)</string>`)

func (ip *Interp) infoPlistStrings() map[string]string {
	values := map[string]string{}
	path := ip.projectResourcePath("Info.plist")
	if path == "" {
		return values
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return values
	}
	for _, match := range plistStringEntry.FindAllStringSubmatch(string(data), -1) {
		values[match[1]] = match[2]
	}
	return values
}

func (ip *Interp) bundleMethod(name string, args []ArgV, pos Pos) (Value, bool) {
	switch name {
	case "url", "path":
		resource := describe(mustArg(args, "forResource"))
		extension := ""
		if v, ok := argNamed(args, "withExtension"); ok && v != nil {
			extension = describe(v)
		} else if v, ok := argNamed(args, "ofType"); ok && v != nil {
			extension = describe(v)
		}
		fileName := resource
		if extension != "" {
			fileName += "." + strings.TrimPrefix(extension, ".")
		}
		path := ip.projectResourcePath(fileName)
		if path == "" {
			return nil, true
		}
		if name == "path" {
			return path, true
		}
		return &URLV{S: path}, true
	case "decode":
		return nil, false
	}
	return nil, false
}

func (ip *Interp) readFileValue(source Value, pos Pos) []byte {
	path := ""
	switch x := source.(type) {
	case *URLV:
		path = strings.TrimPrefix(x.S, "file://")
	case string:
		path = x
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		panic(&ThrowSignal{V: newCodingError("URLError", "The preview can't download from the internet: "+path), Pos: pos})
	}
	if path != "" && !filepath.IsAbs(path) {
		path = ip.projectResourcePath(path)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		panic(&ThrowSignal{V: newCodingError("CocoaError", "The file couldn't be opened because there is no such file: "+describe(source)), Pos: pos})
	}
	return content
}
