package switaculous

import (
	"crypto/sha1"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ncruces/go-sqlite3"
	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

const swiftDataFolder = ".swiftdata"
const storedDateLayout = time.RFC3339Nano

type modelInfo struct {
	context      *ModelContextV
	persistentID int64
}

type ModelContainerV struct {
	context *ModelContextV
}

type ModelContextV struct {
	container         *ModelContainerV
	storePath         string
	database          *sql.DB
	ip                *Interp
	objectsByType     map[string][]*ObjectV
	objectIndex       map[string]map[int64]*ObjectV
	nextIDByType      map[string]int64
	savedRowHashes    map[*ObjectV]string
	pendingDeletes    map[string][]int64
	relationSnapshots map[*ObjectV]map[string][]*ObjectV
	loadedTypes       map[string]bool
	checkedTables     map[string]bool
	fieldCache        map[*TypeInfo][]storedField
	autosaveEnabled   bool
	lastSaveError     string
}

type storedField struct {
	field        *FieldInfo
	kind         string
	targetType   string
	toMany       bool
	deleteRule   string
	unique       bool
	originalName string
	inverseType  string
	inverseField string
}

type relationshipPair struct {
	ownerType, ownerField, targetType, targetField string
	targetToMany                                   bool
	ownerToMany                                    bool
}

func warmUpSQLite() {
	sqlite3.Initialize()
}

func newModelContext(ip *Interp, storePath string) *ModelContextV {
	ctx := &ModelContextV{
		storePath:         storePath,
		ip:                ip,
		objectsByType:     map[string][]*ObjectV{},
		objectIndex:       map[string]map[int64]*ObjectV{},
		nextIDByType:      map[string]int64{},
		savedRowHashes:    map[*ObjectV]string{},
		pendingDeletes:    map[string][]int64{},
		relationSnapshots: map[*ObjectV]map[string][]*ObjectV{},
		loadedTypes:       map[string]bool{},
		checkedTables:     map[string]bool{},
		fieldCache:        map[*TypeInfo][]storedField{},
		autosaveEnabled:   true,
	}
	ctx.container = &ModelContainerV{context: ctx}
	return ctx
}

func (ip *Interp) openModelContainer(inMemory bool, storeName string, memoryKey string, pos Pos) *ModelContainerV {
	if inMemory || ip.rt == nil {
		if ip.memoryContexts == nil {
			ip.memoryContexts = map[string]*ModelContextV{}
		}
		ctx := ip.memoryContexts[memoryKey]
		if ctx == nil {
			ctx = newModelContext(ip, "")
			ip.memoryContexts[memoryKey] = ctx
		}
		return ctx.container
	}
	rt := ip.rt
	if storeName == "" {
		storeName = "default"
	}
	storePath := filepath.Join(rt.dir, swiftDataFolder, storeName+".store")
	ctx := rt.persistentContexts[storePath]
	if ctx == nil {
		ctx = newModelContext(ip, storePath)
		if err := ctx.openDatabase(); err != nil {
			ip.fail(pos, "Could not open the SwiftData store at %s: %v", storePath, err)
		}
		rt.persistentContexts[storePath] = ctx
	}
	ctx.attachToInterp(ip, pos)
	return ctx.container
}

func (ctx *ModelContextV) openDatabase() error {
	if err := os.MkdirAll(filepath.Dir(ctx.storePath), 0755); err != nil {
		return err
	}
	database, err := sql.Open("sqlite3", ctx.storePath)
	if err != nil {
		return err
	}
	database.SetMaxOpenConns(1)
	if err := database.Ping(); err != nil {
		database.Close()
		return err
	}
	ctx.database = database
	return nil
}

func (ctx *ModelContextV) closeDatabase() {
	if ctx.database != nil {
		ctx.database.Close()
		ctx.database = nil
	}
}

// After a hot reload the persistent store keeps its objects, so views that are
// holding on to a model (a selection, a navigation path) keep working. The
// objects are moved over to the newly compiled class definitions here.
func (ctx *ModelContextV) attachToInterp(ip *Interp, pos Pos) {
	if ctx.ip != ip {
		ctx.ip = ip
		ctx.fieldCache = map[*TypeInfo][]storedField{}
		ctx.checkedTables = map[string]bool{}
		for typeName, objects := range ctx.objectsByType {
			newType := ip.types[typeName]
			if newType == nil || !newType.IsModel {
				delete(ctx.objectsByType, typeName)
				delete(ctx.objectIndex, typeName)
				delete(ctx.loadedTypes, typeName)
				continue
			}
			for _, obj := range objects {
				obj.T = newType
				for _, f := range newType.allFields() {
					if _, exists := obj.F[f.Name]; !exists {
						obj.F[f.Name] = ip.defaultFieldValue(newType, f)
					}
				}
			}
		}
	}
	var newTypes []*TypeInfo
	for _, ti := range ip.modelTypes() {
		if !ctx.loadedTypes[ti.Name] {
			newTypes = append(newTypes, ti)
		}
	}
	if len(newTypes) == 0 {
		return
	}
	if err := ctx.loadTypes(newTypes); err != nil {
		ip.fail(pos, "Could not read the SwiftData store: %v", err)
	}
}

func (ip *Interp) modelTypes() []*TypeInfo {
	var models []*TypeInfo
	for name, ti := range ip.types {
		if ti.IsModel && name == ti.Name {
			models = append(models, ti)
		}
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Name < models[j].Name })
	return models
}

func (ip *Interp) defaultFieldValue(ti *TypeInfo, f *FieldInfo) Value {
	if f.Init == nil {
		return nil
	}
	env := newEnv(ip.globals)
	env.staticT = ti
	return copyValue(ip.evalTyped(f.Init, env, f.Type))
}

// ---------- field descriptions ----------

func attributeNamed(decl *VarDecl, name string) *Attr {
	if decl == nil {
		return nil
	}
	for _, a := range decl.Attrs {
		if a.Name == name {
			return a
		}
	}
	return nil
}

func (ctx *ModelContextV) storedFields(ti *TypeInfo) []storedField {
	if cached, ok := ctx.fieldCache[ti]; ok {
		return cached
	}
	ip := ctx.ip
	var fields []storedField
	for _, f := range ti.allFields() {
		if f.Wrapper != "" || attributeNamed(f.Decl, "Transient") != nil || attributeNamed(f.Decl, "ObservationIgnored") != nil {
			continue
		}
		sf := storedField{field: f, deleteRule: "nullify"}
		sf.targetType, sf.toMany = ip.relationshipTarget(f)
		if sf.targetType != "" && !sf.toMany {
			sf.kind = "model"
		} else {
			sf.kind = ip.storageKind(ti, f)
		}
		if attr := attributeNamed(f.Decl, "Attribute"); attr != nil {
			for _, a := range attr.Args {
				v := ip.eval(a.X, ip.globals)
				switch {
				case a.Label == "originalName":
					sf.originalName = describe(v)
				case symName(v) == "unique":
					sf.unique = true
				}
			}
		}
		if attr := attributeNamed(f.Decl, "Relationship"); attr != nil {
			for _, a := range attr.Args {
				v := ip.eval(a.X, ip.globals)
				switch a.Label {
				case "deleteRule":
					sf.deleteRule = symName(v)
				case "inverse":
					if kp, ok := v.(*KeyPathV); ok && len(kp.Path) > 0 {
						sf.inverseType = kp.Root
						sf.inverseField = kp.Path[0]
					}
				case "originalName":
					sf.originalName = describe(v)
				}
			}
		}
		fields = append(fields, sf)
	}
	ctx.fieldCache[ti] = fields
	return fields
}

func (ip *Interp) relationshipTarget(f *FieldInfo) (string, bool) {
	t := f.Type
	if t == nil {
		t = arrayTypeFromInitializer(f.Init)
	}
	if t == nil {
		return "", false
	}
	if t.Kind == tyArray && t.Elem != nil {
		if target := ip.types[ip.resolveAlias(t.Elem.Name)]; target != nil && target.IsModel {
			return target.Name, true
		}
		return "", false
	}
	if t.Kind == tyNamed {
		if target := ip.types[ip.resolveAlias(t.Name)]; target != nil && target.IsModel {
			return target.Name, false
		}
	}
	return "", false
}

func arrayTypeFromInitializer(e Expr) *TypeRef {
	call, ok := e.(*CallExpr)
	if !ok || len(call.Args) != 0 {
		return nil
	}
	lit, ok := call.Fn.(*ArrayLit)
	if !ok || len(lit.Elems) != 1 {
		return nil
	}
	if id, ok := lit.Elems[0].(*Ident); ok {
		return &TypeRef{Kind: tyArray, Elem: &TypeRef{Name: id.Name}}
	}
	return nil
}

func (ip *Interp) storageKind(ti *TypeInfo, f *FieldInfo) string {
	if f.Type != nil {
		if f.Type.Kind != tyNamed {
			return "json"
		}
		switch ip.resolveAlias(f.Type.Name) {
		case "Int", "Int8", "Int16", "Int32", "Int64", "UInt", "UInt8", "UInt16", "UInt32", "UInt64":
			return "int"
		case "Double", "Float", "CGFloat", "TimeInterval", "Float32", "Float64":
			return "double"
		case "Bool":
			return "bool"
		case "String":
			return "string"
		case "Date":
			return "date"
		case "Data":
			return "data"
		case "UUID":
			return "uuid"
		case "URL":
			return "url"
		}
		return "json"
	}
	switch ip.defaultFieldValue(ti, f).(type) {
	case int:
		return "int"
	case float64:
		return "double"
	case bool:
		return "bool"
	case string:
		return "string"
	case *DateV:
		return "date"
	case *DataV:
		return "data"
	case *UUIDV:
		return "uuid"
	case *URLV:
		return "url"
	}
	return "json"
}

func sqlColumnType(kind string) string {
	switch kind {
	case "int", "bool", "model":
		return "INTEGER"
	case "double":
		return "REAL"
	case "data":
		return "BLOB"
	}
	return "TEXT"
}

func quoteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// ---------- loading ----------

func (ctx *ModelContextV) ensureTable(ti *TypeInfo) error {
	if ctx.database == nil || ctx.checkedTables[ti.Name] {
		return nil
	}
	table := quoteIdentifier(ti.Name)
	if _, err := ctx.database.Exec("CREATE TABLE IF NOT EXISTS " + table + " (persistent_id INTEGER PRIMARY KEY)"); err != nil {
		return err
	}
	existing := map[string]bool{}
	rows, err := ctx.database.Query("SELECT name FROM pragma_table_info(?)", ti.Name)
	if err != nil {
		return err
	}
	for rows.Next() {
		var name string
		rows.Scan(&name)
		existing[name] = true
	}
	rows.Close()
	for _, sf := range ctx.storedFields(ti) {
		name := sf.field.Name
		if existing[name] {
			continue
		}
		var statement string
		if sf.originalName != "" && existing[sf.originalName] {
			statement = fmt.Sprintf("ALTER TABLE %s RENAME COLUMN %s TO %s", table, quoteIdentifier(sf.originalName), quoteIdentifier(name))
		} else {
			statement = fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, quoteIdentifier(name), sqlColumnType(sf.kind))
		}
		if _, err := ctx.database.Exec(statement); err != nil {
			return err
		}
		existing[name] = true
	}
	ctx.checkedTables[ti.Name] = true
	return nil
}

type loadedRow struct {
	obj     *ObjectV
	columns map[string]interface{}
}

func (ctx *ModelContextV) loadTypes(types []*TypeInfo) error {
	var loaded []loadedRow
	for _, ti := range types {
		ctx.loadedTypes[ti.Name] = true
		if ctx.objectIndex[ti.Name] == nil {
			ctx.objectIndex[ti.Name] = map[int64]*ObjectV{}
		}
		if ctx.database == nil {
			continue
		}
		if err := ctx.ensureTable(ti); err != nil {
			return err
		}
		rows, err := ctx.database.Query("SELECT * FROM " + quoteIdentifier(ti.Name) + " ORDER BY persistent_id")
		if err != nil {
			return err
		}
		columnNames, _ := rows.Columns()
		for rows.Next() {
			raw := make([]interface{}, len(columnNames))
			pointers := make([]interface{}, len(columnNames))
			for i := range raw {
				pointers[i] = &raw[i]
			}
			if err := rows.Scan(pointers...); err != nil {
				rows.Close()
				return err
			}
			columns := map[string]interface{}{}
			for i, name := range columnNames {
				columns[name] = raw[i]
			}
			id, _ := columns["persistent_id"].(int64)
			ctx.ip.objID++
			obj := &ObjectV{T: ti, F: map[string]Value{}, ID: ctx.ip.objID, Model: &modelInfo{context: ctx, persistentID: id}}
			ctx.objectsByType[ti.Name] = append(ctx.objectsByType[ti.Name], obj)
			ctx.objectIndex[ti.Name][id] = obj
			if id > ctx.nextIDByType[ti.Name] {
				ctx.nextIDByType[ti.Name] = id
			}
			loaded = append(loaded, loadedRow{obj: obj, columns: columns})
		}
		rows.Close()
	}
	for _, row := range loaded {
		ctx.fillLoadedObject(row)
	}
	for _, row := range loaded {
		ctx.savedRowHashes[row.obj] = ctx.rowHash(row.obj)
		ctx.relationSnapshots[row.obj] = ctx.currentRelations(row.obj)
	}
	return nil
}

func (ctx *ModelContextV) fillLoadedObject(row loadedRow) {
	ip := ctx.ip
	ti := row.obj.T
	stored := map[string]storedField{}
	for _, sf := range ctx.storedFields(ti) {
		stored[sf.field.Name] = sf
	}
	for _, f := range ti.allFields() {
		sf, isStored := stored[f.Name]
		raw, hasColumn := row.columns[f.Name]
		if !isStored || !hasColumn || raw == nil {
			if isStored && hasColumn && f.Type != nil && f.Type.Opt {
				row.obj.F[f.Name] = nil
				continue
			}
			row.obj.F[f.Name] = ip.defaultFieldValue(ti, f)
			continue
		}
		row.obj.F[f.Name] = ctx.valueFromColumn(raw, sf)
	}
}

func (ctx *ModelContextV) valueFromColumn(raw interface{}, sf storedField) Value {
	if text, ok := raw.(string); ok && sf.kind != "string" && sf.kind != "date" && sf.kind != "uuid" && sf.kind != "url" {
		return decodeStoredJSONText(ctx.ip, ctx, text)
	}
	switch sf.kind {
	case "int":
		switch n := raw.(type) {
		case int64:
			return int(n)
		case float64:
			return int(n)
		}
	case "double":
		switch n := raw.(type) {
		case int64:
			return float64(n)
		case float64:
			return n
		}
	case "bool":
		if n, ok := raw.(int64); ok {
			return n != 0
		}
	case "string":
		switch s := raw.(type) {
		case string:
			return s
		case []byte:
			return string(s)
		}
	case "date":
		if s, ok := raw.(string); ok {
			if t, err := time.Parse(storedDateLayout, s); err == nil {
				return &DateV{T: t.Local()}
			}
		}
		if t, ok := raw.(time.Time); ok {
			return &DateV{T: t.Local()}
		}
	case "data":
		if b, ok := raw.([]byte); ok {
			return &DataV{Bytes: append([]byte(nil), b...)}
		}
	case "uuid":
		if s, ok := raw.(string); ok {
			return &UUIDV{S: s}
		}
	case "url":
		if s, ok := raw.(string); ok {
			return &URLV{S: s}
		}
	case "model":
		if id, ok := raw.(int64); ok {
			return ctx.objectIndex[sf.targetType][id]
		}
	}
	return nil
}

func (ctx *ModelContextV) columnValue(v Value, sf storedField) interface{} {
	if v == nil {
		return nil
	}
	switch sf.kind {
	case "int":
		if i, ok := v.(int); ok {
			return int64(i)
		}
	case "double":
		if f, ok := toFloat(v); ok {
			return f
		}
	case "bool":
		if b, ok := v.(bool); ok {
			if b {
				return int64(1)
			}
			return int64(0)
		}
	case "string":
		if s, ok := v.(string); ok {
			return s
		}
	case "date":
		if d, ok := v.(*DateV); ok {
			return d.T.UTC().Format(storedDateLayout)
		}
	case "data":
		if d, ok := v.(*DataV); ok {
			return d.Bytes
		}
	case "uuid":
		if u, ok := v.(*UUIDV); ok {
			return u.S
		}
	case "url":
		if u, ok := v.(*URLV); ok {
			return u.S
		}
	case "model":
		if obj, ok := v.(*ObjectV); ok {
			if obj.Model != nil && obj.Model.context == ctx {
				return obj.Model.persistentID
			}
			return nil
		}
	}
	encoded, _ := json.Marshal(encodeStoredJSON(ctx, v))
	return string(encoded)
}

// ---------- self-describing JSON for structs, enums, arrays and other values ----------

type storedFloat float64

func (f storedFloat) MarshalJSON() ([]byte, error) {
	value := float64(f)
	if math.IsInf(value, 0) || math.IsNaN(value) {
		return []byte("null"), nil
	}
	text := strconv.FormatFloat(value, 'g', -1, 64)
	if !strings.ContainsAny(text, ".eE") {
		text += ".0"
	}
	return []byte(text), nil
}

func encodeStoredJSON(ctx *ModelContextV, v Value) interface{} {
	encodeList := func(values []Value) []interface{} {
		list := make([]interface{}, len(values))
		for i, e := range values {
			list[i] = encodeStoredJSON(ctx, e)
		}
		return list
	}
	encodeFields := func(fields map[string]Value) map[string]interface{} {
		out := map[string]interface{}{}
		for name, value := range fields {
			switch value.(type) {
			case *BoxV, *BindingV, *FuncV, *NativeV:
				continue
			}
			out[name] = encodeStoredJSON(ctx, value)
		}
		return out
	}
	switch x := v.(type) {
	case nil:
		return nil
	case bool, string, int:
		return x
	case float64:
		return storedFloat(x)
	case Character:
		return map[string]interface{}{"$character": string(x)}
	case *DateV:
		return map[string]interface{}{"$date": x.T.UTC().Format(storedDateLayout)}
	case *UUIDV:
		return map[string]interface{}{"$uuid": x.S}
	case *URLV:
		return map[string]interface{}{"$url": x.S}
	case *DataV:
		return map[string]interface{}{"$data": base64.StdEncoding.EncodeToString(x.Bytes)}
	case *ArrayV:
		return encodeList(x.E)
	case *SetV:
		return map[string]interface{}{"$set": encodeList(x.Keys)}
	case *DictV:
		pairs := make([]interface{}, len(x.Keys))
		for i := range x.Keys {
			pairs[i] = []interface{}{encodeStoredJSON(ctx, x.Keys[i]), encodeStoredJSON(ctx, x.Vals[i])}
		}
		return map[string]interface{}{"$dictionary": pairs}
	case *TupleV:
		return map[string]interface{}{"$tuple": encodeList(x.E), "labels": x.L}
	case *StructV:
		return map[string]interface{}{"$struct": x.T.Name, "fields": encodeFields(x.F)}
	case *EnumV:
		encoded := map[string]interface{}{"$enum": x.T.Name, "case": x.Case}
		if len(x.Assoc) > 0 {
			encoded["values"] = encodeList(x.Assoc)
			encoded["labels"] = x.Labels
		}
		return encoded
	case *ObjectV:
		if ctx != nil && x.Model != nil && x.Model.context == ctx {
			return map[string]interface{}{"$model": x.T.Name, "id": x.Model.persistentID}
		}
		return map[string]interface{}{"$object": x.T.Name, "fields": encodeFields(x.F)}
	}
	return nil
}

func decodeStoredJSONText(ip *Interp, ctx *ModelContextV, text string) Value {
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	var parsed interface{}
	if err := decoder.Decode(&parsed); err != nil {
		return nil
	}
	return decodeStoredJSON(ip, ctx, parsed)
}

func decodeStoredJSON(ip *Interp, ctx *ModelContextV, j interface{}) Value {
	decodeList := func(raw interface{}) []Value {
		list, _ := raw.([]interface{})
		values := make([]Value, len(list))
		for i, e := range list {
			values[i] = decodeStoredJSON(ip, ctx, e)
		}
		return values
	}
	decodeInto := func(fields map[string]Value, raw interface{}) {
		stored, _ := raw.(map[string]interface{})
		for name, value := range stored {
			fields[name] = decodeStoredJSON(ip, ctx, value)
		}
	}
	switch x := j.(type) {
	case nil:
		return nil
	case bool, string:
		return x
	case json.Number:
		text := x.String()
		if strings.ContainsAny(text, ".eE") {
			f, _ := x.Float64()
			return f
		}
		n, _ := x.Int64()
		return int(n)
	case []interface{}:
		return &ArrayV{E: decodeList(x)}
	case map[string]interface{}:
		if s, ok := x["$character"].(string); ok {
			return Character(s)
		}
		if s, ok := x["$date"].(string); ok {
			t, _ := time.Parse(storedDateLayout, s)
			return &DateV{T: t.Local()}
		}
		if s, ok := x["$uuid"].(string); ok {
			return &UUIDV{S: s}
		}
		if s, ok := x["$url"].(string); ok {
			return &URLV{S: s}
		}
		if s, ok := x["$data"].(string); ok {
			b, _ := base64.StdEncoding.DecodeString(s)
			return &DataV{Bytes: b}
		}
		if raw, ok := x["$set"]; ok {
			set := newSet()
			for _, v := range decodeList(raw) {
				set.Add(v)
			}
			return set
		}
		if raw, ok := x["$dictionary"]; ok {
			dict := newDict()
			pairs, _ := raw.([]interface{})
			for _, pair := range pairs {
				kv, _ := pair.([]interface{})
				if len(kv) == 2 {
					dict.Set(decodeStoredJSON(ip, ctx, kv[0]), decodeStoredJSON(ip, ctx, kv[1]))
				}
			}
			return dict
		}
		if raw, ok := x["$tuple"]; ok {
			tuple := &TupleV{E: decodeList(raw)}
			labels, _ := x["labels"].([]interface{})
			for _, l := range labels {
				label, _ := l.(string)
				tuple.L = append(tuple.L, label)
			}
			return tuple
		}
		if name, ok := x["$struct"].(string); ok {
			ti := ip.types[name]
			if ti == nil || ti.Kind != "struct" {
				return nil
			}
			inst, _ := ip.newInstance(ti).(*StructV)
			decodeInto(inst.F, x["fields"])
			return inst
		}
		if name, ok := x["$enum"].(string); ok {
			ti := ip.types[name]
			caseName, _ := x["case"].(string)
			if ti == nil || ti.CaseMap[caseName] == nil {
				return nil
			}
			value := &EnumV{T: ti, Case: caseName}
			if values := decodeList(x["values"]); len(values) > 0 {
				value.Assoc = values
			}
			labels, _ := x["labels"].([]interface{})
			for _, l := range labels {
				label, _ := l.(string)
				value.Labels = append(value.Labels, label)
			}
			return value
		}
		if name, ok := x["$model"].(string); ok {
			if ctx == nil {
				return nil
			}
			id, _ := x["id"].(json.Number).Int64()
			return ctx.objectIndex[name][id]
		}
		if name, ok := x["$object"].(string); ok {
			ti := ip.types[name]
			if ti == nil || ti.Kind != "class" {
				return nil
			}
			obj, _ := ip.newInstance(ti).(*ObjectV)
			decodeInto(obj.F, x["fields"])
			return obj
		}
	}
	return nil
}

// ---------- inserting, deleting and relationships ----------

func (ctx *ModelContextV) liveObjects(typeName string) []*ObjectV {
	return ctx.objectsByType[typeName]
}

func (ctx *ModelContextV) insertModel(v Value, pos Pos) {
	obj, ok := v.(*ObjectV)
	if !ok || !obj.T.IsModel {
		ctx.ip.fail(pos, "insert(_:) needs an instance of a @Model class, but got a value of type '%s'", typeName(v))
	}
	if obj.Model != nil && obj.Model.context == ctx {
		return
	}
	typeName := obj.T.Name
	if !ctx.loadedTypes[typeName] {
		if err := ctx.loadTypes([]*TypeInfo{obj.T}); err != nil {
			ctx.ip.fail(pos, "Could not read the SwiftData store: %v", err)
		}
	}
	// @Attribute(.unique) turns an insert of a duplicate into an update of the existing object.
	for _, sf := range ctx.storedFields(obj.T) {
		if !sf.unique {
			continue
		}
		for _, existing := range ctx.liveObjects(typeName) {
			if valueEquals(existing.F[sf.field.Name], obj.F[sf.field.Name]) {
				for name, value := range obj.F {
					existing.F[name] = value
				}
				return
			}
		}
	}
	ctx.nextIDByType[typeName]++
	id := ctx.nextIDByType[typeName]
	obj.Model = &modelInfo{context: ctx, persistentID: id}
	ctx.objectsByType[typeName] = append(ctx.objectsByType[typeName], obj)
	if ctx.objectIndex[typeName] == nil {
		ctx.objectIndex[typeName] = map[int64]*ObjectV{}
	}
	ctx.objectIndex[typeName][id] = obj
}

func (ctx *ModelContextV) deleteModel(v Value, pos Pos) {
	obj, ok := v.(*ObjectV)
	if !ok || !obj.T.IsModel {
		ctx.ip.fail(pos, "delete(_:) needs an instance of a @Model class, but got a value of type '%s'", typeName(v))
	}
	if obj.Model == nil || obj.Model.context != ctx {
		return
	}
	typeName := obj.T.Name
	objects := ctx.objectsByType[typeName]
	for i, candidate := range objects {
		if candidate == obj {
			ctx.objectsByType[typeName] = append(objects[:i:i], objects[i+1:]...)
			break
		}
	}
	delete(ctx.objectIndex[typeName], obj.Model.persistentID)
	if _, saved := ctx.savedRowHashes[obj]; saved {
		ctx.pendingDeletes[typeName] = append(ctx.pendingDeletes[typeName], obj.Model.persistentID)
	}
	delete(ctx.savedRowHashes, obj)
	delete(ctx.relationSnapshots, obj)
	obj.Model = nil

	for _, sf := range ctx.storedFields(obj.T) {
		if sf.deleteRule == "cascade" {
			for _, related := range modelObjectsIn(obj.F[sf.field.Name]) {
				ctx.deleteModel(related, pos)
			}
		}
	}
	ctx.removeReferencesTo(obj)
}

func (ctx *ModelContextV) removeReferencesTo(deleted *ObjectV) {
	for _, objects := range ctx.objectsByType {
		for _, obj := range objects {
			for name, value := range obj.F {
				switch x := value.(type) {
				case *ObjectV:
					if x == deleted {
						obj.F[name] = nil
					}
				case *ArrayV:
					if containsObject(x.E, deleted) {
						kept := &ArrayV{Elem: x.Elem}
						for _, e := range x.E {
							if e != Value(deleted) {
								kept.E = append(kept.E, e)
							}
						}
						obj.F[name] = kept
					}
				}
			}
			if snapshot := ctx.relationSnapshots[obj]; snapshot != nil {
				for field, targets := range snapshot {
					snapshot[field] = withoutObject(targets, deleted)
				}
			}
		}
	}
}

func modelObjectsIn(v Value) []*ObjectV {
	switch x := v.(type) {
	case *ObjectV:
		if x.T.IsModel {
			return []*ObjectV{x}
		}
	case *ArrayV:
		var objects []*ObjectV
		for _, e := range x.E {
			if obj, ok := e.(*ObjectV); ok && obj.T.IsModel {
				objects = append(objects, obj)
			}
		}
		return objects
	}
	return nil
}

func containsObject(values []Value, obj *ObjectV) bool {
	for _, v := range values {
		if v == Value(obj) {
			return true
		}
	}
	return false
}

func withoutObject(objects []*ObjectV, removed *ObjectV) []*ObjectV {
	var kept []*ObjectV
	for _, obj := range objects {
		if obj != removed {
			kept = append(kept, obj)
		}
	}
	return kept
}

// Pointing a new object at an object that is already in a context (for example
// stop.trip = trip) inserts the new object into that context, as SwiftData does.
func insertIntoRelatedContext(obj *ObjectV, value Value) {
	for _, related := range modelObjectsIn(value) {
		if related.Model != nil {
			related.Model.context.insertModel(obj, Pos{})
			return
		}
	}
}

// Like SwiftData, a new object that is only reachable through a relationship of
// an inserted object is inserted too.
func (ctx *ModelContextV) insertRelatedObjects() {
	for changed := true; changed; {
		changed = false
		for _, objects := range ctx.objectsByType {
			for _, obj := range objects {
				for _, sf := range ctx.storedFields(obj.T) {
					for _, related := range modelObjectsIn(obj.F[sf.field.Name]) {
						if related.Model == nil {
							ctx.insertModel(related, Pos{})
							changed = true
						}
					}
				}
			}
		}
	}
}

func (ctx *ModelContextV) currentRelations(obj *ObjectV) map[string][]*ObjectV {
	relations := map[string][]*ObjectV{}
	for _, sf := range ctx.storedFields(obj.T) {
		if sf.targetType != "" {
			relations[sf.field.Name] = modelObjectsIn(obj.F[sf.field.Name])
		}
	}
	return relations
}

func (ctx *ModelContextV) relationshipPairs() []relationshipPair {
	ip := ctx.ip
	var pairs []relationshipPair
	paired := map[string]bool{}
	addPair := func(ownerType string, owner storedField, targetField string) {
		target := ip.types[owner.targetType]
		if target == nil {
			return
		}
		var inverse *storedField
		for _, sf := range ctx.storedFields(target) {
			if sf.field.Name == targetField {
				found := sf
				inverse = &found
			}
		}
		if inverse == nil || paired[ownerType+"."+owner.field.Name] || paired[owner.targetType+"."+targetField] {
			return
		}
		paired[ownerType+"."+owner.field.Name] = true
		paired[owner.targetType+"."+targetField] = true
		pairs = append(pairs, relationshipPair{
			ownerType: ownerType, ownerField: owner.field.Name, ownerToMany: owner.toMany,
			targetType: owner.targetType, targetField: targetField, targetToMany: inverse.toMany,
		})
	}
	models := ip.modelTypes()
	for _, ti := range models {
		for _, sf := range ctx.storedFields(ti) {
			if sf.targetType != "" && sf.inverseField != "" {
				addPair(ti.Name, sf, sf.inverseField)
			}
		}
	}
	// Without an explicit inverse, two models that point at each other through
	// exactly one property each are treated as the two ends of one relationship.
	for _, ti := range models {
		for _, sf := range ctx.storedFields(ti) {
			if sf.targetType == "" || sf.targetType == ti.Name || paired[ti.Name+"."+sf.field.Name] {
				continue
			}
			if countRelationships(ctx.storedFields(ti), sf.targetType) != 1 {
				continue
			}
			target := ip.types[sf.targetType]
			if target == nil || countRelationships(ctx.storedFields(target), ti.Name) != 1 {
				continue
			}
			for _, back := range ctx.storedFields(target) {
				if back.targetType == ti.Name {
					addPair(ti.Name, sf, back.field.Name)
				}
			}
		}
	}
	return pairs
}

func countRelationships(fields []storedField, targetType string) int {
	count := 0
	for _, sf := range fields {
		if sf.targetType == targetType {
			count++
		}
	}
	return count
}

// SwiftData keeps both ends of a relationship in sync: adding a sight to
// destination.sights also sets sight.destination. Changes made since the last
// sync are found by comparing with a snapshot and copied to the other end.
func (ctx *ModelContextV) syncRelationships() {
	ctx.insertRelatedObjects()
	pairs := ctx.relationshipPairs()
	for _, pair := range pairs {
		ctx.copyRelationshipChanges(pair.ownerType, pair.ownerField, pair.targetField, pair.targetToMany, pair.ownerToMany)
		ctx.copyRelationshipChanges(pair.targetType, pair.targetField, pair.ownerField, pair.ownerToMany, pair.targetToMany)
	}
	for _, objects := range ctx.objectsByType {
		for _, obj := range objects {
			ctx.relationSnapshots[obj] = ctx.currentRelations(obj)
		}
	}
}

func (ctx *ModelContextV) copyRelationshipChanges(sourceType, sourceField, inverseField string, inverseToMany, sourceToMany bool) {
	for _, source := range ctx.liveObjects(sourceType) {
		current := modelObjectsIn(source.F[sourceField])
		previous := ctx.relationSnapshots[source][sourceField]
		for _, target := range current {
			if !containsPointer(previous, target) {
				ctx.linkInverse(target, inverseField, inverseToMany, source, sourceField, sourceToMany)
			}
		}
		for _, target := range previous {
			if !containsPointer(current, target) {
				ctx.unlinkInverse(target, inverseField, inverseToMany, source)
			}
		}
		if ctx.relationSnapshots[source] == nil {
			ctx.relationSnapshots[source] = map[string][]*ObjectV{}
		}
		ctx.relationSnapshots[source][sourceField] = current
	}
}

func containsPointer(objects []*ObjectV, obj *ObjectV) bool {
	for _, candidate := range objects {
		if candidate == obj {
			return true
		}
	}
	return false
}

func (ctx *ModelContextV) linkInverse(target *ObjectV, inverseField string, inverseToMany bool, source *ObjectV, sourceField string, sourceToMany bool) {
	if inverseToMany {
		list, _ := target.F[inverseField].(*ArrayV)
		if list == nil {
			list = &ArrayV{}
		}
		if !containsObject(list.E, source) {
			target.F[inverseField] = &ArrayV{E: append(append([]Value(nil), list.E...), source), Elem: list.Elem}
		}
		return
	}
	previousOwner, _ := target.F[inverseField].(*ObjectV)
	target.F[inverseField] = source
	if previousOwner != nil && previousOwner != source {
		if sourceToMany {
			if list, ok := previousOwner.F[sourceField].(*ArrayV); ok {
				kept := &ArrayV{Elem: list.Elem}
				for _, e := range list.E {
					if e != Value(target) {
						kept.E = append(kept.E, e)
					}
				}
				previousOwner.F[sourceField] = kept
			}
		} else if previousOwner.F[sourceField] == Value(target) {
			previousOwner.F[sourceField] = nil
		}
	}
}

func (ctx *ModelContextV) unlinkInverse(target *ObjectV, inverseField string, inverseToMany bool, source *ObjectV) {
	if inverseToMany {
		if list, ok := target.F[inverseField].(*ArrayV); ok && containsObject(list.E, source) {
			kept := &ArrayV{Elem: list.Elem}
			for _, e := range list.E {
				if e != Value(source) {
					kept.E = append(kept.E, e)
				}
			}
			target.F[inverseField] = kept
		}
		return
	}
	if target.F[inverseField] == Value(source) {
		target.F[inverseField] = nil
	}
}

// ---------- fetching ----------

func (ctx *ModelContextV) fetch(descriptor *NativeObj, pos Pos) []Value {
	ip := ctx.ip
	typeName, _ := descriptor.Props["modelType"].(string)
	if typeName == "" {
		ip.fail(pos, "FetchDescriptor needs a model type, for example FetchDescriptor<Item>()")
	}
	ti := ip.types[typeName]
	if ti == nil || !ti.IsModel {
		ip.fail(pos, "'%s' is not a @Model class, so it can't be fetched", typeName)
	}
	if !ctx.loadedTypes[typeName] {
		if err := ctx.loadTypes([]*TypeInfo{ti}); err != nil {
			ip.fail(pos, "Could not read the SwiftData store: %v", err)
		}
	}
	ctx.syncRelationships()
	var results []Value
	predicate := descriptor.Props["predicate"]
	for _, obj := range ctx.liveObjects(typeName) {
		if predicate != nil && !ip.truthy(ip.callValue(predicate, []ArgV{{V: obj}}, pos, nil), pos) {
			continue
		}
		results = append(results, obj)
	}
	if sorts, ok := descriptor.Props["sortBy"].(*ArrayV); ok && len(sorts.E) > 0 {
		sort.SliceStable(results, func(i, j int) bool {
			return ip.sortsBefore(results[i], results[j], sorts.E, pos)
		})
	}
	if offset, ok := descriptor.Props["fetchOffset"].(int); ok && offset > 0 {
		if offset >= len(results) {
			results = nil
		} else {
			results = results[offset:]
		}
	}
	if limit, ok := descriptor.Props["fetchLimit"].(int); ok && limit >= 0 && limit < len(results) {
		results = results[:limit]
	}
	return results
}

func (ip *Interp) sortsBefore(a, b Value, sortDescriptors []Value, pos Pos) bool {
	for _, d := range sortDescriptors {
		descriptor, ok := d.(*NativeObj)
		if !ok {
			continue
		}
		keyPath, _ := descriptor.Props["keyPath"].(*KeyPathV)
		if keyPath == nil {
			continue
		}
		order := compareSortValues(ip.applyKeyPath(keyPath, a, pos), ip.applyKeyPath(keyPath, b, pos))
		if order == 0 {
			continue
		}
		if symName(descriptor.Props["order"]) == "reverse" {
			return order > 0
		}
		return order < 0
	}
	return false
}

// String sorting follows SwiftData's default .localizedStandard comparison,
// which ignores case, and nil sorts before any value.
func compareSortValues(a, b Value) int {
	if a == nil || b == nil {
		switch {
		case a == nil && b == nil:
			return 0
		case a == nil:
			return -1
		}
		return 1
	}
	if sa, ok := a.(string); ok {
		if sb, ok := b.(string); ok {
			if c := strings.Compare(strings.ToLower(sa), strings.ToLower(sb)); c != 0 {
				return c
			}
			return strings.Compare(sa, sb)
		}
	}
	if ba, ok := a.(bool); ok {
		if bb, ok := b.(bool); ok {
			switch {
			case ba == bb:
				return 0
			case !ba:
				return -1
			}
			return 1
		}
	}
	if c, ok := compareValues(a, b); ok {
		return c
	}
	return strings.Compare(describe(a), describe(b))
}

// ---------- saving ----------

func (ctx *ModelContextV) rowHash(obj *ObjectV) string {
	hasher := sha1.New()
	for _, sf := range ctx.storedFields(obj.T) {
		hasher.Write([]byte(sf.field.Name))
		switch v := ctx.columnValue(obj.F[sf.field.Name], sf).(type) {
		case []byte:
			hasher.Write(v)
		default:
			fmt.Fprintf(hasher, "=%T:%v;", v, v)
		}
	}
	return hex.EncodeToString(hasher.Sum(nil))
}

func (ctx *ModelContextV) hasChanges() bool {
	for _, ids := range ctx.pendingDeletes {
		if len(ids) > 0 {
			return true
		}
	}
	for _, objects := range ctx.objectsByType {
		for _, obj := range objects {
			if saved, ok := ctx.savedRowHashes[obj]; !ok || saved != ctx.rowHash(obj) {
				return true
			}
		}
	}
	return false
}

func (ctx *ModelContextV) save() error {
	ctx.syncRelationships()
	if ctx.database == nil {
		ctx.pendingDeletes = map[string][]int64{}
		for _, objects := range ctx.objectsByType {
			for _, obj := range objects {
				ctx.savedRowHashes[obj] = ctx.rowHash(obj)
			}
		}
		return nil
	}
	type pendingWrite struct {
		obj  *ObjectV
		hash string
	}
	var writes []pendingWrite
	for _, objects := range ctx.objectsByType {
		for _, obj := range objects {
			hash := ctx.rowHash(obj)
			if ctx.savedRowHashes[obj] != hash {
				writes = append(writes, pendingWrite{obj, hash})
			}
		}
	}
	hasDeletes := false
	for _, ids := range ctx.pendingDeletes {
		hasDeletes = hasDeletes || len(ids) > 0
	}
	if len(writes) == 0 && !hasDeletes {
		return nil
	}
	for _, write := range writes {
		if err := ctx.ensureTable(write.obj.T); err != nil {
			return err
		}
	}
	tx, err := ctx.database.Begin()
	if err != nil {
		return err
	}
	for typeName, ids := range ctx.pendingDeletes {
		for _, id := range ids {
			if _, err := tx.Exec("DELETE FROM "+quoteIdentifier(typeName)+" WHERE persistent_id = ?", id); err != nil {
				tx.Rollback()
				return err
			}
		}
	}
	for _, write := range writes {
		if err := ctx.writeRow(tx, write.obj); err != nil {
			tx.Rollback()
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	ctx.pendingDeletes = map[string][]int64{}
	for _, write := range writes {
		ctx.savedRowHashes[write.obj] = write.hash
	}
	return nil
}

func (ctx *ModelContextV) writeRow(tx *sql.Tx, obj *ObjectV) error {
	fields := ctx.storedFields(obj.T)
	columns := []string{"persistent_id"}
	placeholders := []string{"?"}
	values := []interface{}{obj.Model.persistentID}
	for _, sf := range fields {
		columns = append(columns, quoteIdentifier(sf.field.Name))
		placeholders = append(placeholders, "?")
		values = append(values, ctx.columnValue(obj.F[sf.field.Name], sf))
	}
	statement := fmt.Sprintf("INSERT OR REPLACE INTO %s (%s) VALUES (%s)",
		quoteIdentifier(obj.T.Name), strings.Join(columns, ", "), strings.Join(placeholders, ", "))
	_, err := tx.Exec(statement, values...)
	return err
}

func (ctx *ModelContextV) deleteAllData() {
	for typeName, objects := range ctx.objectsByType {
		for _, obj := range objects {
			obj.Model = nil
			delete(ctx.savedRowHashes, obj)
			delete(ctx.relationSnapshots, obj)
		}
		delete(ctx.objectsByType, typeName)
		ctx.objectIndex[typeName] = map[int64]*ObjectV{}
		if ctx.database != nil {
			ctx.database.Exec("DELETE FROM " + quoteIdentifier(typeName))
		}
	}
}

func (rt *Runtime) syncModelContextsLocked() {
	if rt.ip == nil {
		return
	}
	for _, ctx := range rt.persistentContexts {
		if ctx.ip == rt.ip {
			ctx.syncRelationships()
		}
	}
	for _, ctx := range rt.ip.memoryContexts {
		ctx.syncRelationships()
	}
}

func (rt *Runtime) autosaveModelContextsLocked() {
	for _, ctx := range rt.persistentContexts {
		if ctx.ip != rt.ip || !ctx.autosaveEnabled {
			continue
		}
		if err := ctx.save(); err != nil {
			message := err.Error()
			if message != ctx.lastSaveError {
				rt.logf("error", "SwiftData could not save: %s", message)
			}
			ctx.lastSaveError = message
		} else {
			ctx.lastSaveError = ""
		}
	}
}

func (rt *Runtime) closeModelStoresLocked() {
	for path, ctx := range rt.persistentContexts {
		ctx.closeDatabase()
		delete(rt.persistentContexts, path)
	}
}

func (rt *Runtime) eraseModelDataLocked() {
	rt.closeModelStoresLocked()
	os.RemoveAll(filepath.Join(rt.dir, swiftDataFolder))
	rt.userDefaultsSaved = ""
	rt.userDefaultsLoaded = true
	if rt.ip != nil {
		rt.ip.memoryContexts = nil
	}
}

// ---------- Swift API ----------

func modelTypeName(v Value) string {
	if t, ok := v.(*TypeV); ok {
		return t.Name
	}
	return ""
}

func (ip *Interp) newFetchDescriptor(modelType string, args []ArgV) *NativeObj {
	descriptor := newObj("FetchDescriptor")
	descriptor.Props["modelType"] = modelType
	descriptor.Props["predicate"] = nil
	descriptor.Props["sortBy"] = &ArrayV{}
	descriptor.Props["fetchLimit"] = nil
	descriptor.Props["fetchOffset"] = nil
	descriptor.Props["includePendingChanges"] = true
	if v, ok := argNamed(args, "predicate"); ok {
		descriptor.Props["predicate"] = v
	}
	if v, ok := argNamed(args, "sortBy"); ok {
		if list, ok := v.(*ArrayV); ok {
			descriptor.Props["sortBy"] = list
		}
	}
	return descriptor
}

func (ip *Interp) newSortDescriptor(keyPath Value, order Value) *NativeObj {
	descriptor := newObj("SortDescriptor")
	descriptor.Props["keyPath"] = keyPath
	if order == nil {
		order = sym("forward")
	}
	descriptor.Props["order"] = order
	return descriptor
}

func (ip *Interp) newQuery(args []ArgV, pos Pos) *NativeObj {
	descriptor := ip.newFetchDescriptor("", nil)
	if first := argAt(args, 0); first != nil && args[0].Label == "" {
		if d, ok := first.(*NativeObj); ok && d.Kind == "FetchDescriptor" {
			descriptor = d
		}
	}
	if v, ok := argNamed(args, "filter"); ok {
		descriptor.Props["predicate"] = v
	}
	order, _ := argNamed(args, "order")
	if v, ok := argNamed(args, "sort"); ok {
		switch sortValue := v.(type) {
		case *KeyPathV:
			descriptor.Props["sortBy"] = &ArrayV{E: []Value{ip.newSortDescriptor(sortValue, order)}}
		case *ArrayV:
			descriptor.Props["sortBy"] = sortValue
		case *NativeObj:
			descriptor.Props["sortBy"] = &ArrayV{E: []Value{sortValue}}
		}
	}
	query := newObj("Query")
	query.Props["descriptor"] = descriptor
	return query
}

func (ip *Interp) constructSwiftDataType(t *TypeV, args []ArgV, pos Pos) (Value, bool) {
	switch t.Name {
	case "FetchDescriptor":
		modelType := ""
		if t.Ref != nil && len(t.Ref.Args) > 0 {
			modelType = ip.resolveAlias(t.Ref.Args[0].Name)
		}
		return ip.newFetchDescriptor(modelType, args), true
	case "SortDescriptor":
		order, _ := argNamed(args, "order")
		return ip.newSortDescriptor(argAt(args, 0), order), true
	case "Query":
		return ip.newQuery(args, pos), true
	case "Schema":
		schema := newObj("Schema")
		schema.Props["models"] = argAt(args, 0)
		return schema, true
	case "ModelConfiguration":
		config := newObj("ModelConfiguration")
		config.Props["name"] = "default"
		config.Props["isStoredInMemoryOnly"] = false
		if len(args) > 0 && args[0].Label == "" {
			if name, ok := args[0].V.(string); ok {
				config.Props["name"] = name
			}
		}
		if v, ok := argNamed(args, "isStoredInMemoryOnly"); ok {
			config.Props["isStoredInMemoryOnly"] = v == true
		}
		if v, ok := argNamed(args, "schema"); ok {
			config.Props["schema"] = v
		}
		return config, true
	case "ModelContainer":
		inMemory, storeName := false, "default"
		var configurations []Value
		if v, ok := argNamed(args, "configurations"); ok {
			if list, ok := v.(*ArrayV); ok {
				configurations = list.E
			} else {
				configurations = []Value{v}
			}
			for _, a := range args {
				if a.Label == "" && a.V != v {
					if _, isConfig := a.V.(*NativeObj); isConfig {
						configurations = append(configurations, a.V)
					}
				}
			}
		}
		for _, c := range configurations {
			if config, ok := c.(*NativeObj); ok && config.Kind == "ModelConfiguration" {
				inMemory = inMemory || config.Props["isStoredInMemoryOnly"] == true
				if name, ok := config.Props["name"].(string); ok && name != "" {
					storeName = name
				}
			}
		}
		return ip.openModelContainer(inMemory, storeName, "container@"+pos.String(), pos), true
	case "ModelContext":
		if container, ok := argAt(args, 0).(*ModelContainerV); ok {
			return container.context, true
		}
		ip.fail(pos, "ModelContext(_:) needs a ModelContainer")
	}
	return nil, false
}

func (ip *Interp) modelContextMethod(ctx *ModelContextV, name string, args []ArgV, pos Pos) (Value, bool) {
	switch name {
	case "insert":
		ctx.insertModel(argAt(args, 0), pos)
		return Void, true
	case "delete":
		if t, ok := argNamed(args, "model"); ok {
			typeName := modelTypeName(t)
			descriptor := ip.newFetchDescriptor(typeName, nil)
			if predicate, ok := argNamed(args, "where"); ok {
				descriptor.Props["predicate"] = predicate
			}
			for _, obj := range ctx.fetch(descriptor, pos) {
				ctx.deleteModel(obj, pos)
			}
			return Void, true
		}
		ctx.deleteModel(argAt(args, 0), pos)
		return Void, true
	case "save":
		if err := ctx.save(); err != nil {
			panic(&ThrowSignal{V: "SwiftData save failed: " + err.Error(), Pos: pos})
		}
		return Void, true
	case "fetch":
		descriptor, ok := argAt(args, 0).(*NativeObj)
		if !ok || descriptor.Kind != "FetchDescriptor" {
			ip.fail(pos, "fetch(_:) needs a FetchDescriptor, for example FetchDescriptor<Item>()")
		}
		return &ArrayV{E: ctx.fetch(descriptor, pos)}, true
	case "fetchCount":
		descriptor, ok := argAt(args, 0).(*NativeObj)
		if !ok || descriptor.Kind != "FetchDescriptor" {
			ip.fail(pos, "fetchCount(_:) needs a FetchDescriptor, for example FetchDescriptor<Item>()")
		}
		return len(ctx.fetch(descriptor, pos)), true
	case "model", "registeredModel":
		id := describe(argAt(args, 0))
		for _, objects := range ctx.objectsByType {
			for _, obj := range objects {
				if persistentIdentifier(obj) == id {
					return obj, true
				}
			}
		}
		return nil, true
	case "transaction":
		ip.callValue(argAt(args, 0), nil, pos, nil)
		if err := ctx.save(); err != nil {
			panic(&ThrowSignal{V: "SwiftData save failed: " + err.Error(), Pos: pos})
		}
		return Void, true
	case "rollback":
		ip.warn(pos, "rollback() is not supported by the preview; unsaved changes are kept")
		return Void, true
	case "processPendingChanges":
		ctx.syncRelationships()
		return Void, true
	}
	return nil, false
}

func (ip *Interp) modelContextProperty(ctx *ModelContextV, name string) (Value, bool) {
	switch name {
	case "hasChanges":
		return ctx.hasChanges(), true
	case "autosaveEnabled":
		return ctx.autosaveEnabled, true
	case "container":
		return ctx.container, true
	case "insertedModelsArray", "changedModelsArray", "deletedModelsArray":
		return &ArrayV{}, true
	case "undoManager":
		return nil, true
	}
	return nil, false
}

func (ip *Interp) modelContainerMember(container *ModelContainerV, name string, args []ArgV, pos Pos, isCall bool) (Value, bool) {
	switch name {
	case "mainContext":
		return container.context, true
	case "deleteAllData":
		if isCall {
			container.context.deleteAllData()
			return Void, true
		}
		return &NativeV{Name: name, Fn: func(c *CallCtx) Value {
			container.context.deleteAllData()
			return Void
		}}, true
	case "configurations", "schema":
		return &ArrayV{}, true
	}
	return nil, false
}

func persistentIdentifier(obj *ObjectV) string {
	if obj.Model == nil {
		return fmt.Sprintf("x-swiftdata://%s/t%d", obj.T.Name, obj.ID)
	}
	return fmt.Sprintf("x-swiftdata://%s/p%d", obj.T.Name, obj.Model.persistentID)
}

func (ip *Interp) modelObjectMember(obj *ObjectV, name string) (Value, bool) {
	switch name {
	case "id", "persistentModelID":
		return persistentIdentifier(obj), true
	case "modelContext":
		if obj.Model == nil {
			return nil, true
		}
		return obj.Model.context, true
	case "isDeleted":
		return obj.Model == nil, true
	case "hasChanges":
		if obj.Model == nil {
			return true, true
		}
		saved, ok := obj.Model.context.savedRowHashes[obj]
		return !ok || saved != obj.Model.context.rowHash(obj), true
	}
	return nil, false
}

func (r *Render) queryResults(f *FieldInfo, current Value, env *REnv) Value {
	ip := r.ip
	modelType := ""
	if f.Type != nil && f.Type.Kind == tyArray && f.Type.Elem != nil {
		modelType = ip.resolveAlias(f.Type.Elem.Name)
	}
	if modelType == "" {
		r.fail(f.Decl.Pos, "@Query needs an array type that names the model, for example: @Query var items: [Item]")
	}
	query, _ := current.(*NativeObj)
	if query == nil || query.Kind != "Query" {
		query = ip.newQuery(ip.evalArgs(f.WrapArgs, ip.globals, false), f.Decl.Pos)
	}
	ctx, _ := env.envVals["modelContext"].(*ModelContextV)
	if ctx == nil {
		r.fail(f.Decl.Pos, "@Query needs a model container. Add .modelContainer(for: %s.self) to the WindowGroup in your App, or to the view in #Preview", modelType)
	}
	source, _ := query.Props["descriptor"].(*NativeObj)
	descriptor := newObj("FetchDescriptor")
	for k, v := range source.Props {
		descriptor.Props[k] = v
	}
	descriptor.Props["modelType"] = modelType
	return &ArrayV{E: ctx.fetch(descriptor, f.Decl.Pos)}
}

func (r *Render) modelContainerFromModifier(m *ModV, path string) *ModelContextV {
	ip := r.ip
	first := argAt(m.Args, 0)
	if container, ok := first.(*ModelContainerV); ok {
		return container.context
	}
	inMemory := false
	if v, ok := argNamed(m.Args, "inMemory"); ok {
		inMemory = v == true
	}
	container := ip.openModelContainer(inMemory, "default", fmt.Sprintf("modifier@%s|%s", m.Pos, path), m.Pos)
	if v, ok := argNamed(m.Args, "isAutosaveEnabled"); ok {
		container.context.autosaveEnabled = v != false
	}
	return container.context
}
