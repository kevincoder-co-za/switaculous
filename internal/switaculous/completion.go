package switaculous

import (
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

type completionItem struct {
	Text    string `json:"text"`
	Insert  string `json:"insert"`
	Detail  string `json:"detail"`
	Doc     string `json:"doc"`
	Example string `json:"example,omitempty"`
	Kind    string `json:"kind"`
}

type completionRequest struct {
	Text string `json:"text"`
	Line int    `json:"line"`
	Ch   int    `json:"ch"`
}

type completionResponse struct {
	Context string           `json:"context"`
	Prefix  string           `json:"prefix"`
	Items   []completionItem `json:"items"`
}

// What the code before the cursor evaluates to, as far as text analysis can tell.
type typeGuess struct {
	name   string
	ref    *TypeRef
	info   *TypeInfo
	isType bool
	isView bool
}

type completer struct {
	ip    *Interp
	text  string
	lines []string
	line  int
	ch    int
	seen  map[string]bool
	items []completionItem
}

var swiftKeywords = strings.Fields(`var let func struct class enum protocol extension import return if else guard switch case default
	for in while repeat break continue true false nil self Self init private static some mutating throws try await async defer
	where is as inout typealias associatedtype fileprivate public internal override final lazy weak unowned`)

var builtinTypeMembers = map[string]string{
	"String": `count isEmpty uppercased() lowercased() capitalized contains(_:) hasPrefix(_:) hasSuffix(_:) trimmingCharacters(in:)
		replacingOccurrences(of:with:) split(separator:) components(separatedBy:) prefix(_:) suffix(_:) dropFirst(_:) dropLast(_:)
		first last reversed() localizedStandardContains(_:) localizedCaseInsensitiveContains(_:) firstIndex(of:) range(of:) isEmpty
		utf8 unicodeScalars description appending(_:) padding(toLength:withPad:startingAt:) formatted() starts(with:)`,
	"Int":    `description isMultiple(of:) formatted() signum() magnitude isZero quotientAndRemainder(dividingBy:) advanced(by:) clamped(to:)`,
	"Double": `rounded() rounded(_:) formatted() formatted(_:) isNaN isInfinite magnitude squareRoot() truncatingRemainder(dividingBy:) description isZero`,
	"Bool":   `toggle() description`,
	"Array": `count isEmpty first last append(_:) append(contentsOf:) insert(_:at:) remove(at:) removeAll() removeAll(where:) removeFirst() removeLast()
		removeLast(_:) contains(_:) contains(where:) firstIndex(of:) firstIndex(where:) lastIndex(of:) map(_:) compactMap(_:) flatMap(_:) filter(_:)
		reduce(_:_:) sorted() sorted(by:) sort() sort(by:) reversed() reverse() enumerated() indices shuffled() shuffle() randomElement()
		prefix(_:) suffix(_:) dropFirst(_:) dropLast(_:) joined(separator:) min() max() min(by:) max(by:) allSatisfy(_:) forEach(_:)
		swapAt(_:_:) move(fromOffsets:toOffset:) remove(atOffsets:) split(separator:) chunked`,
	"Dictionary": `count isEmpty keys values updateValue(_:forKey:) removeValue(forKey:) contains(where:) map(_:) filter(_:) sorted(by:) merge(_:uniquingKeysWith:) mapValues(_:)`,
	"Set":        `count isEmpty insert(_:) remove(_:) contains(_:) union(_:) intersection(_:) subtracting(_:) sorted() map(_:) filter(_:) isSubset(of:) formUnion(_:)`,
	"Date": `timeIntervalSince1970 timeIntervalSinceReferenceDate timeIntervalSince(_:) addingTimeInterval(_:) formatted() formatted(date:time:) formatted(_:)
		description distance(to:) advanced(by:)`,
	"UUID":              `uuidString description`,
	"URL":               `absoluteString lastPathComponent pathExtension path host appendingPathComponent(_:)`,
	"Color":             `opacity(_:) gradient description`,
	"Font":              `bold() italic() weight(_:) monospaced() monospacedDigit() smallCaps() leading(_:) width(_:)`,
	"Text":              `font(_:) bold() italic() foregroundStyle(_:) foregroundColor(_:) underline() strikethrough() fontWeight(_:) kerning(_:) lineLimit(_:) multilineTextAlignment(_:)`,
	"ModelContext":      `insert(_:) delete(_:) save() fetch(_:) fetchCount(_:) hasChanges autosaveEnabled container delete(model:where:) rollback()`,
	"ModelContainer":    `mainContext deleteAllData()`,
	"Binding":           `wrappedValue projectedValue`,
	"PhotosPickerItem":  `loadTransferable(type:) itemIdentifier supportedContentTypes`,
	"UIImage":           `size jpegData(compressionQuality:) pngData() scale`,
	"Data":              `count isEmpty base64EncodedString()`,
	"Calendar":          `component(_:from:) date(byAdding:value:to:) dateComponents(_:from:) startOfDay(for:) isDateInToday(_:) isDate(_:inSameDayAs:) date(from:)`,
	"DateFormatter":     `dateFormat dateStyle timeStyle string(from:) date(from:) locale`,
	"JSONDecoder":       `decode(_:from:) dateDecodingStrategy keyDecodingStrategy`,
	"JSONEncoder":       `encode(_:) outputFormatting dateEncodingStrategy keyEncodingStrategy`,
	"UserDefaults":      `set(_:forKey:) string(forKey:) integer(forKey:) double(forKey:) bool(forKey:) data(forKey:) object(forKey:) array(forKey:) removeObject(forKey:)`,
	"Bundle":            `url(forResource:withExtension:) path(forResource:ofType:) decode(_:)`,
	"GeometryProxy":     `size frame(in:) safeAreaInsets`,
	"Task":              `cancel() isCancelled value`,
	"Timer":             `invalidate() fireDate isValid`,
	"Range":             `lowerBound upperBound contains(_:) count isEmpty map(_:) reversed()`,
	"DismissAction":     `callAsFunction()`,
	"NavigationPath":    `append(_:) removeLast() removeLast(_:) count isEmpty`,
	"FetchDescriptor":   `predicate sortBy fetchLimit fetchOffset`,
	"DragGesture.Value": `translation location startLocation predictedEndTranslation velocity`,
}

var builtinStaticMembers = map[string]string{
	"Color": `red orange yellow green mint teal cyan blue indigo purple pink brown gray black white clear primary secondary accentColor
		init(red:green:blue:opacity:) init(_:) init(hue:saturation:brightness:) init(uiColor:)`,
	"Font":                    `largeTitle title title2 title3 headline subheadline body callout footnote caption caption2 system(size:weight:design:) system(_:design:weight:) custom(_:size:)`,
	"Date":                    `now distantPast distantFuture init() init(timeIntervalSince1970:) init(timeIntervalSinceNow:)`,
	"UUID":                    `init() init(uuidString:)`,
	"Int":                     `random(in:) max min init(_:)`,
	"Double":                  `random(in:) pi infinity init(_:)`,
	"String":                  `init(_:) init(format:_:) init(repeating:count:) init(describing:) init(data:encoding:) init(decoding:as:)`,
	"Array":                   `init() init(repeating:count:)`,
	"Calendar":                `current autoupdatingCurrent`,
	"UserDefaults":            `standard`,
	"Bundle":                  `main`,
	"URL":                     `init(string:) init(fileURLWithPath:)`,
	"Task":                    `init(priority:operation:) sleep(for:) sleep(nanoseconds:) detached(priority:operation:)`,
	"Timer":                   `scheduledTimer(withTimeInterval:repeats:block:) publish(every:on:in:)`,
	"Animation":               `default linear easeIn easeOut easeInOut spring bouncy smooth snappy linear(duration:) easeInOut(duration:) spring(duration:bounce:)`,
	"AnyTransition":           `opacity scale slide move(edge:) push(from:) asymmetric(insertion:removal:) identity`,
	"CGSize":                  `zero init(width:height:)`,
	"CGPoint":                 `zero init(x:y:)`,
	"Angle":                   `degrees(_:) radians(_:) zero`,
	"UnitPoint":               `center top bottom leading trailing topLeading topTrailing bottomLeading bottomTrailing`,
	"Alignment":               `center top bottom leading trailing topLeading topTrailing bottomLeading bottomTrailing`,
	"Edge":                    `top bottom leading trailing`,
	"JSONDecoder":             `init()`,
	"JSONEncoder":             `init()`,
	"DateFormatter":           `init()`,
	"DispatchQueue":           `main global()`,
	"ModelContainer":          `init(for:configurations:)`,
	"FetchDescriptor":         `init(predicate:sortBy:)`,
	"SortDescriptor":          `init(_:order:)`,
	"NavigationPath":          `init()`,
	"UIImagePickerController": `init() isSourceTypeAvailable(_:)`,
	"PhotosPickerItem":        ``,
	"Image":                   `init(systemName:) init(_:) init(uiImage:)`,
	"Text":                    `init(_:) init(_:format:) init(_:style:)`,
	"Locale":                  `current`,
	"TimeZone":                `current`,
	"Haptics":                 ``,
	"Character":               `init(_:)`,
	"Data":                    `init() init(contentsOf:) init(_:)`,
}

// Which implicit members (.something) make sense for a call or argument label.
var implicitMembersByContext = map[string][]string{
	"font":                          strings.Fields(builtinStaticMembers["Font"]),
	"fontWeight":                    {"ultraLight", "thin", "light", "regular", "medium", "semibold", "bold", "heavy", "black"},
	"fontDesign":                    {"default", "rounded", "serif", "monospaced"},
	"foregroundStyle":               append(strings.Fields(builtinStaticMembers["Color"]), "tint", "ultraThinMaterial", "thinMaterial", "regularMaterial", "thickMaterial", "linearGradient(colors:startPoint:endPoint:)"),
	"foregroundColor":               strings.Fields(builtinStaticMembers["Color"]),
	"background":                    append(strings.Fields(builtinStaticMembers["Color"]), "ultraThinMaterial", "thinMaterial", "regularMaterial", "thickMaterial", "ultraThickMaterial", "bar"),
	"fill":                          append(strings.Fields(builtinStaticMembers["Color"]), "linearGradient(colors:startPoint:endPoint:)", "tint"),
	"stroke":                        strings.Fields(builtinStaticMembers["Color"]),
	"tint":                          strings.Fields(builtinStaticMembers["Color"]),
	"color":                         strings.Fields(builtinStaticMembers["Color"]),
	"colors":                        strings.Fields(builtinStaticMembers["Color"]),
	"listRowBackground":             strings.Fields(builtinStaticMembers["Color"]),
	"alignment":                     {"center", "top", "bottom", "leading", "trailing", "topLeading", "topTrailing", "bottomLeading", "bottomTrailing", "firstTextBaseline", "lastTextBaseline"},
	"padding":                       {"all", "horizontal", "vertical", "top", "bottom", "leading", "trailing"},
	"edges":                         {"all", "horizontal", "vertical", "top", "bottom", "leading", "trailing"},
	"edge":                          {"top", "bottom", "leading", "trailing"},
	"ignoresSafeArea":               {"all", "container", "keyboard"},
	"buttonStyle":                   {"automatic", "bordered", "borderedProminent", "borderless", "plain", "glass", "glassProminent"},
	"buttonBorderShape":             {"capsule", "roundedRectangle", "roundedRectangle(radius:)", "circle", "automatic"},
	"textFieldStyle":                {"roundedBorder", "plain", "automatic"},
	"pickerStyle":                   {"menu", "segmented", "wheel", "inline", "navigationLink", "automatic", "palette"},
	"listStyle":                     {"plain", "inset", "grouped", "insetGrouped", "sidebar", "automatic"},
	"toggleStyle":                   {"switch", "button", "automatic"},
	"labelStyle":                    {"iconOnly", "titleOnly", "titleAndIcon", "automatic"},
	"datePickerStyle":               {"compact", "graphical", "wheel", "automatic"},
	"progressViewStyle":             {"linear", "circular", "automatic"},
	"tabViewStyle":                  {"page", "page(indexDisplayMode: .never)", "automatic", "sidebarAdaptable"},
	"controlSize":                   {"mini", "small", "regular", "large", "extraLarge"},
	"navigationBarTitleDisplayMode": {"large", "inline", "automatic"},
	"animation":                     strings.Fields(builtinStaticMembers["Animation"]),
	"withAnimation":                 strings.Fields(builtinStaticMembers["Animation"]),
	"transition":                    strings.Fields(builtinStaticMembers["AnyTransition"]),
	"insertion":                     strings.Fields(builtinStaticMembers["AnyTransition"]),
	"removal":                       strings.Fields(builtinStaticMembers["AnyTransition"]),
	"clipShape":                     {"rect", "rect(cornerRadius:)", "circle", "capsule", "ellipse", "buttonBorder", "containerRelative"},
	"contentShape":                  {"rect", "circle", "capsule"},
	"presentationDetents":           {"medium", "large", "height(_:)", "fraction(_:)"},
	"presentationDragIndicator":     {"visible", "hidden", "automatic"},
	"role":                          {"destructive", "cancel"},
	"placement":                     {"automatic", "topBarLeading", "topBarTrailing", "navigationBarLeading", "navigationBarTrailing", "principal", "bottomBar", "confirmationAction", "cancellationAction", "primaryAction", "keyboard"},
	"displayedComponents":           {"date", "hourAndMinute"},
	"keyboardType":                  {"default", "numberPad", "decimalPad", "emailAddress", "URL", "phonePad", "numbersAndPunctuation", "webSearch"},
	"textInputAutocapitalization":   {"never", "words", "sentences", "characters"},
	"submitLabel":                   {"done", "go", "send", "search", "next", "return", "continue", "join"},
	"matching":                      {"images", "videos", "any(of:)", "not(_:)", "livePhotos", "screenshots"},
	"sourceType":                    {"camera", "photoLibrary", "savedPhotosAlbum"},
	"order":                         {"forward", "reverse"},
	"startPoint":                    strings.Fields(builtinStaticMembers["UnitPoint"]),
	"endPoint":                      strings.Fields(builtinStaticMembers["UnitPoint"]),
	"center":                        strings.Fields(builtinStaticMembers["UnitPoint"]),
	"anchor":                        strings.Fields(builtinStaticMembers["UnitPoint"]),
	"contentMode":                   {"fit", "fill"},
	"imageScale":                    {"small", "medium", "large"},
	"symbolRenderingMode":           {"monochrome", "hierarchical", "palette", "multicolor"},
	"textCase":                      {"uppercase", "lowercase"},
	"multilineTextAlignment":        {"leading", "center", "trailing"},
	"truncationMode":                {"head", "middle", "tail"},
	"scrollIndicators":              {"hidden", "visible", "automatic", "never"},
	"scrollContentBackground":       {"hidden", "visible", "automatic"},
	"listRowSeparator":              {"hidden", "visible", "automatic"},
	"preferredColorScheme":          {"light", "dark"},
	"visibility":                    {"visible", "hidden", "automatic"},
	"titleVisibility":               {"visible", "hidden", "automatic"},
	"dateDecodingStrategy":          {"iso8601", "secondsSince1970", "millisecondsSince1970", "formatted(_:)", "deferredToDate"},
	"dateEncodingStrategy":          {"iso8601", "secondsSince1970", "millisecondsSince1970", "formatted(_:)"},
	"keyDecodingStrategy":           {"convertFromSnakeCase", "useDefaultKeys"},
	"keyEncodingStrategy":           {"convertToSnakeCase", "useDefaultKeys"},
	"outputFormatting":              {"prettyPrinted", "sortedKeys", "withoutEscapingSlashes"},
	"format":                        {"number", "percent", "currency(code:)", "dateTime", "number.precision(.fractionLength(2))"},
	"date":                          {"numeric", "abbreviated", "long", "complete", "omitted"},
	"time":                          {"shortened", "standard", "complete", "omitted"},
	"style":                         {"date", "time", "relative", "timer", "offset"},
	"axis":                          {"vertical", "horizontal"},
	"sort":                          {},
	"deleteRule":                    {"cascade", "nullify", "deny", "noAction"},
	"detents":                       {"medium", "large", "height(_:)", "fraction(_:)"},
	"priority":                      {"userInitiated", "background", "high", "low", "medium", "utility"},
	"spacing":                       {},
	"weight":                        {"ultraLight", "thin", "light", "regular", "medium", "semibold", "bold", "heavy", "black"},
	"design":                        {"default", "rounded", "serif", "monospaced"},
	"minimumDuration":               {},
}

var implicitMembersFallback = strings.Fields(`red orange yellow green blue purple pink gray black white clear primary secondary
	largeTitle title title2 title3 headline subheadline body callout footnote caption caption2 leading trailing center top bottom
	horizontal vertical all infinity bold semibold medium regular rounded borderedProminent bordered plain roundedBorder segmented
	insetGrouped inline large degrees(_:) easeInOut spring opacity scale slide constant(_:) system(size:) ultraThinMaterial automatic
	rect(cornerRadius:) circle capsule now zero`)

func (rt *Runtime) handleComplete(w http.ResponseWriter, r *http.Request) {
	var request completionRequest
	json.NewDecoder(r.Body).Decode(&request)
	rt.mu.Lock()
	ip := rt.ip
	rt.mu.Unlock()
	c := &completer{ip: ip, text: request.Text, lines: strings.Split(request.Text, "\n"), line: request.Line, ch: request.Ch, seen: map[string]bool{}}
	response := c.complete()
	writeJSON(w, response)
}

func (c *completer) complete() completionResponse {
	if c.line < 0 || c.line >= len(c.lines) {
		return completionResponse{}
	}
	before := c.lines[c.line]
	if c.ch < len(before) {
		before = before[:c.ch]
	}
	prefixStart := len(before)
	for prefixStart > 0 && isIdentifierByte(before[prefixStart-1]) {
		prefixStart--
	}
	prefix := before[prefixStart:]
	head := before[:prefixStart]
	if insideStringLiteral(head) {
		return completionResponse{Context: "string", Prefix: prefix}
	}
	if strings.HasSuffix(head, ".") {
		receiver := receiverExpression(head[:len(head)-1])
		if receiver == "" && strings.TrimSpace(head) == "." {
			if chain, ok := c.chainAbove(); ok {
				if chain == "" {
					c.membersOf(typeGuess{name: "View", isView: true})
				} else {
					c.membersOf(c.resolveChain(chain))
				}
				return c.finish("member", prefix)
			}
		}
		if receiver == "" {
			c.implicitMembers(head[:len(head)-1])
			return c.finish("implicit", prefix)
		}
		c.membersOf(c.resolveChain(receiver))
		return c.finish("member", prefix)
	}
	c.identifiers()
	return c.finish("identifier", prefix)
}

func (c *completer) finish(context, prefix string) completionResponse {
	lowerPrefix := strings.ToLower(prefix)
	var matching []completionItem
	for _, item := range c.items {
		if strings.HasPrefix(strings.ToLower(item.Text), lowerPrefix) {
			matching = append(matching, item)
		}
	}
	sort.SliceStable(matching, func(i, j int) bool {
		a, b := matching[i], matching[j]
		if a.Kind != b.Kind && kindRank(a.Kind) != kindRank(b.Kind) {
			return kindRank(a.Kind) < kindRank(b.Kind)
		}
		return a.Text < b.Text
	})
	if len(matching) > 80 {
		matching = matching[:80]
	}
	return completionResponse{Context: context, Prefix: prefix, Items: matching}
}

func kindRank(kind string) int {
	switch kind {
	case "field", "method", "case", "local", "param":
		return 0
	case "type", "view":
		return 1
	case "modifier", "static":
		return 2
	case "keyword":
		return 4
	}
	return 3
}

func isIdentifierByte(b byte) bool {
	return b == '_' || b == '$' || unicode.IsLetter(rune(b)) || unicode.IsDigit(rune(b))
}

func insideStringLiteral(head string) bool {
	inString := false
	for i := 0; i < len(head); i++ {
		switch head[i] {
		case '\\':
			i++
		case '"':
			inString = !inString
		}
	}
	return inString
}

// A line starting with "." usually carries on the expression above it, as in
// a SwiftUI modifier chain. Returns that expression, or "" when it ends with
// the closing brace of a view such as VStack { … }. Not ok when the line above
// leaves an argument or value to come, so ".red" there is an implicit member.
func (c *completer) chainAbove() (string, bool) {
	var parts []string
	for i := c.line - 1; i >= 0; i-- {
		text := strings.TrimSpace(c.lines[i])
		if j := strings.Index(text, " //"); j >= 0 && strings.Count(text[:j], `"`)%2 == 0 {
			text = strings.TrimSpace(text[:j])
		}
		if text == "" || strings.HasPrefix(text, "//") {
			continue
		}
		if len(parts) == 0 && expectsMore(text) {
			return "", false
		}
		parts = append([]string{text}, parts...)
		if !strings.HasPrefix(text, ".") {
			break
		}
	}
	if len(parts) == 0 {
		return "", false
	}
	if strings.HasPrefix(parts[0], "}") || strings.HasPrefix(parts[0], ")") {
		return "", true
	}
	receiver := receiverExpression(strings.Join(parts, ""))
	if strings.HasPrefix(receiver, ".") {
		return "", true
	}
	return receiver, true
}

func expectsMore(text string) bool {
	if strings.ContainsAny(text[len(text)-1:], ",([{=:?") {
		return true
	}
	for _, word := range []string{"in", "return", "else", "case"} {
		if text == word || strings.HasSuffix(text, " "+word) {
			return true
		}
	}
	return false
}

// The expression that ends right before a dot: `item.name`, `items[0]`, `Text("hi").bold()`.
func receiverExpression(head string) string {
	end := len(head)
	i := end
	depth := 0
	for i > 0 {
		b := head[i-1]
		switch {
		case b == ')' || b == ']':
			depth++
		case b == '(' || b == '[':
			if depth == 0 {
				return head[i:end]
			}
			depth--
		case depth > 0:
		case isIdentifierByte(b) || b == '.' || b == '?' || b == '!':
		case b == ' ' || b == '\t':
			if depth == 0 {
				return head[i:end]
			}
		default:
			return head[i:end]
		}
		i--
	}
	return head[i:end]
}

func (c *completer) add(kind, text, insert, detail, doc string) {
	if text == "" || c.seen[text+"|"+detail] {
		return
	}
	c.seen[text+"|"+detail] = true
	if insert == "" {
		insert = text
	}
	c.items = append(c.items, completionItem{Text: text, Insert: insert, Detail: detail, Doc: doc, Kind: kind})
}

// Adds a documented API, carrying its usage example along.
func (c *completer) addDoc(kind, text, insert string, doc apiDoc) {
	count := len(c.items)
	c.add(kind, text, insert, doc.Signature, doc.Summary)
	if len(c.items) > count {
		c.items[count].Example = doc.Example
	}
}

// The cursor marker; the editor puts the caret there after inserting.
const cursorMarker = "\x00"

// "append(contentsOf:)" becomes "append(contentsOf: |)" with the caret at |.
func insertFromSignatureName(name string) string {
	open := strings.Index(name, "(")
	if open < 0 {
		return name
	}
	var parts []string
	for _, label := range strings.Split(strings.TrimSuffix(name[open+1:], ")"), ":") {
		if label == "" {
			continue
		}
		if label == "_" {
			parts = append(parts, "")
		} else {
			parts = append(parts, label+": ")
		}
	}
	return name[:open] + "(" + withCursorInFirstPart(parts) + ")"
}

func withCursorInFirstPart(parts []string) string {
	if len(parts) == 0 {
		return cursorMarker
	}
	parts[0] += cursorMarker
	return strings.Join(parts, ", ")
}

func (c *completer) addNames(kind string, names []string, detail string) {
	for _, name := range names {
		c.add(kind, name, insertFromSignatureName(name), detail, "")
	}
}

func (c *completer) addModifiers() {
	for name, docs := range modifierDocs {
		for _, doc := range docs {
			c.addDoc("modifier", name, insertFromDocSignature(doc.Signature), doc)
		}
	}
	for name := range knownModifiers {
		if _, documented := modifierDocs[name]; !documented {
			c.add("modifier", name, name+"("+cursorMarker+")", "", "")
		}
	}
	if c.ip != nil {
		if viewExtension, ok := c.ip.ext["View"]; ok {
			for name, methods := range viewExtension.Methods {
				c.add("modifier", name, insertFromParams(name, methods[0].Params), signatureOf(name, methods[0]), "Your View extension")
			}
		}
	}
}

// "padding(_ edges: Edge.Set = .all, _ length: CGFloat? = nil)" → "padding(|)";
// parameters without defaults keep their labels, closures get braces.
func insertFromDocSignature(signature string) string {
	open := strings.Index(signature, "(")
	if open < 0 {
		return signature
	}
	var parts []string
	for _, param := range splitTopLevel(signature[open+1 : len(signature)-1]) {
		if strings.Contains(param, "=") {
			continue
		}
		label := strings.Fields(strings.SplitN(param, ":", 2)[0])[0]
		switch {
		case strings.Contains(param, ") ->") && label == "_":
			parts = append(parts, emptyClosure)
		case strings.Contains(param, ") ->"):
			parts = append(parts, label+": "+emptyClosure)
		case label == "_":
			parts = append(parts, "")
		default:
			parts = append(parts, label+": ")
		}
	}
	return callInsert(signature[:open], parts)
}

const emptyClosure = "{ }"

// Builds "name(a: |, b: )" with the caret in the first argument. A closure in
// the last place becomes a trailing closure: "sheet(isPresented: |) { }".
func callInsert(name string, parts []string) string {
	trailing := ""
	if n := len(parts); n > 0 && strings.HasSuffix(parts[n-1], emptyClosure) {
		parts = parts[:n-1]
		trailing = " " + emptyClosure
	}
	if len(parts) == 0 && trailing != "" {
		return name + " { " + cursorMarker + " }"
	}
	if len(parts) > 0 && strings.HasSuffix(parts[0], emptyClosure) {
		parts[0] = strings.TrimSuffix(parts[0], "}") + cursorMarker + " }"
		return name + "(" + strings.Join(parts, ", ") + ")" + trailing
	}
	return name + "(" + withCursorInFirstPart(parts) + ")" + trailing
}

func splitTopLevel(text string) []string {
	var parts []string
	depth := 0
	start := 0
	for i, r := range text {
		switch {
		case r == '(' || r == '<' || r == '[':
			depth++
		case r == '>' && i > 0 && text[i-1] == '-':
			// The arrow of a closure type, not the end of a generic.
		case r == ')' || r == '>' || r == ']':
			depth--
		case r == ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(text[start:i]))
				start = i + 1
			}
		}
	}
	if rest := strings.TrimSpace(text[start:]); rest != "" {
		parts = append(parts, rest)
	}
	return parts
}

func signatureOf(name string, decl *FuncDecl) string {
	var params []string
	for _, p := range decl.Params {
		label := p.Label
		if label == "" {
			label = "_"
		}
		text := label
		if p.Name != "" && p.Name != label {
			text += " " + p.Name
		}
		text += ": " + p.Type.String()
		params = append(params, text)
	}
	signature := name + "(" + strings.Join(params, ", ") + ")"
	if decl.Result != nil {
		signature += " -> " + decl.Result.String()
	}
	return signature
}

func insertFromParams(name string, params []*Param) string {
	var parts []string
	for _, p := range params {
		if p.Default != nil {
			continue
		}
		switch {
		case p.Type != nil && p.Type.Kind == tyFunc && p.Label == "":
			parts = append(parts, emptyClosure)
		case p.Type != nil && p.Type.Kind == tyFunc:
			parts = append(parts, p.Label+": "+emptyClosure)
		case p.Label == "":
			parts = append(parts, "")
		default:
			parts = append(parts, p.Label+": ")
		}
	}
	return callInsert(name, parts)
}

// ---------- identifier context ----------

func (c *completer) identifiers() {
	for _, name := range c.localNames() {
		c.add("local", name, name, "", "")
	}
	if ti := c.enclosingType(); ti != nil {
		c.addTypeMembers(ti, false)
	}
	for name, docs := range viewDocs {
		for _, doc := range docs {
			c.addDoc("view", name, insertFromDocSignature(doc.Signature), doc)
		}
	}
	for name := range viewLabels {
		if _, documented := viewDocs[name]; !documented && !unsupportedViews[name] {
			c.add("view", name, name+"("+cursorMarker+")", "", "")
		}
	}
	if c.ip != nil {
		for name, ti := range c.ip.types {
			if !strings.Contains(name, ".") {
				c.add("type", name, name, ti.Kind, "")
			}
		}
		for name, slot := range c.ip.globals.vars {
			switch fn := slot.V.(type) {
			case *FuncV:
				c.add("function", name, insertFromParams(name, fn.Params), signatureOf(name, fn.Decl), "")
			default:
				c.add("global", name, name, "", "")
			}
		}
	}
	for _, name := range strings.Fields(`print(_:) min(_:_:) max(_:_:) abs(_:) withAnimation(_:_:) stride(from:to:by:) stride(from:through:by:) zip(_:_:)
		Date UUID Color Font Int Double String Bool Array Dictionary Set Date() UUID() Task JSONDecoder JSONEncoder UserDefaults Bundle
		Calendar DateFormatter Timer DispatchQueue NavigationPath Binding State Query ModelContainer FetchDescriptor SortDescriptor
		fatalError(_:) precondition(_:_:) dump(_:) type(of:) sqrt(_:) pow(_:_:) round(_:) floor(_:) ceil(_:)`) {
		c.add("global", name, insertFromSignatureName(name), "", "")
	}
	for _, keyword := range swiftKeywords {
		c.add("keyword", keyword, keyword, "", "")
	}
	for _, wrapper := range []string{"@State", "@Binding", "@Environment", "@Query", "@Bindable", "@Observable", "@Model", "@AppStorage", "@FocusState", "@StateObject", "@ObservedObject", "@EnvironmentObject", "@Published", "@ViewBuilder", "@main", "@Previewable"} {
		c.add("keyword", wrapper, wrapper, "property wrapper", "")
	}
}

var declarationPattern = regexp.MustCompile(`\b(?:var|let|func|case)\s+([A-Za-z_]\w*)`)
var parameterPattern = regexp.MustCompile(`([A-Za-z_]\w*)\s*:\s*[A-Z\[(]`)
var closureParamPattern = regexp.MustCompile(`\{\s*(?:\(?\s*([A-Za-z_]\w*)(?:\s*,\s*([A-Za-z_]\w*))?\s*\)?)\s+in\b`)
var forInPattern = regexp.MustCompile(`\bfor\s+([A-Za-z_]\w*)\s+in\b`)

func (c *completer) localNames() []string {
	textAbove := strings.Join(c.lines[:c.line+1], "\n")
	seen := map[string]bool{}
	var names []string
	collect := func(matches [][]string) {
		for _, m := range matches {
			for _, name := range m[1:] {
				if name != "" && !seen[name] {
					seen[name] = true
					names = append(names, name)
				}
			}
		}
	}
	collect(declarationPattern.FindAllStringSubmatch(textAbove, -1))
	collect(parameterPattern.FindAllStringSubmatch(textAbove, -1))
	collect(closureParamPattern.FindAllStringSubmatch(textAbove, -1))
	collect(forInPattern.FindAllStringSubmatch(textAbove, -1))
	return names
}

var typeDeclarationPattern = regexp.MustCompile(`(?m)^\s*(?:@\w+\s+)*(?:final\s+)?(?:private\s+|fileprivate\s+|public\s+)?(struct|class|enum|extension|actor)\s+([A-Za-z_]\w*)`)

func (c *completer) enclosingType() *TypeInfo {
	if c.ip == nil {
		return nil
	}
	textAbove := strings.Join(c.lines[:c.line], "\n")
	matches := typeDeclarationPattern.FindAllStringSubmatchIndex(textAbove, -1)
	for i := len(matches) - 1; i >= 0; i-- {
		m := matches[i]
		name := textAbove[m[4]:m[5]]
		if braceDepth(textAbove[m[0]:]) > 0 {
			if ti, ok := c.ip.types[name]; ok {
				return ti
			}
			if ti, ok := c.ip.ext[name]; ok {
				return ti
			}
		}
	}
	return nil
}

func braceDepth(text string) int {
	depth := 0
	inString := false
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '\\':
			i++
		case '"':
			inString = !inString
		case '{':
			if !inString {
				depth++
			}
		case '}':
			if !inString {
				depth--
			}
		}
	}
	return depth
}

// ---------- implicit members ----------

func (c *completer) implicitMembers(head string) {
	context, label := callContext(head)
	added := false
	for _, key := range []string{label, context} {
		if members, ok := implicitMembersByContext[key]; ok && key != "" {
			c.addNames("static", members, "")
			added = len(members) > 0 || added
		}
	}
	if c.ip != nil {
		c.addEnumCasesForParameter(context, label)
	}
	if !added {
		c.addNames("static", implicitMembersFallback, "")
	}
}

// For `.font(.` returns ("font", ""); for `Color(red: 1, green: .` returns ("Color", "green").
func callContext(head string) (string, string) {
	depth := 0
	i := len(head)
	for i > 0 {
		b := head[i-1]
		switch b {
		case ')', ']':
			depth++
		case '(', '[':
			if depth == 0 {
				name := identifierBefore(head[:i-1])
				label := ""
				if j := strings.LastIndexAny(head[i:], ",("); true {
					segment := head[i+j+1:]
					if colon := strings.Index(segment, ":"); colon >= 0 && !strings.Contains(segment[colon:], "(") {
						label = strings.TrimSpace(segment[:colon])
					}
				}
				return name, label
			}
			depth--
		case ':':
			if depth == 0 {
				return identifierBefore(strings.TrimSpace(head[:i-1])), identifierBefore(head[:i-1])
			}
		case '=':
			if depth == 0 {
				return "", identifierBefore(strings.TrimSpace(head[:i-1]))
			}
		}
		i--
	}
	return "", ""
}

func identifierBefore(text string) string {
	text = strings.TrimRight(text, " \t")
	end := len(text)
	start := end
	for start > 0 && isIdentifierByte(text[start-1]) {
		start--
	}
	return text[start:end]
}

func (c *completer) addEnumCasesForParameter(context, label string) {
	ti := c.ip.types[context]
	if ti == nil {
		for _, candidate := range c.ip.types {
			if candidate.Kind == "enum" && candidate.Name == strings.Title(label) {
				ti = candidate
			}
		}
	}
	if ti == nil {
		return
	}
	for _, f := range ti.allFields() {
		if f.Name == label && f.Type != nil {
			if enumInfo := c.ip.types[f.Type.Name]; enumInfo != nil && enumInfo.Kind == "enum" {
				c.addTypeMembers(enumInfo, true)
			}
		}
	}
	for _, init := range ti.Inits {
		for _, p := range init.Params {
			if p.Label == label && p.Type != nil {
				if enumInfo := c.ip.types[p.Type.Name]; enumInfo != nil && enumInfo.Kind == "enum" {
					c.addTypeMembers(enumInfo, true)
				}
			}
		}
	}
	if ti.Kind == "enum" && label == "" {
		c.addTypeMembers(ti, true)
	}
}

// ---------- member resolution ----------

func (c *completer) resolveChain(receiver string) typeGuess {
	segments := splitChain(receiver)
	if len(segments) == 0 {
		return typeGuess{}
	}
	current := c.resolveFirst(segments[0])
	for _, segment := range segments[1:] {
		current = c.resolveMember(current, segment)
	}
	return current
}

func splitChain(receiver string) []string {
	var segments []string
	depth := 0
	start := 0
	for i := 0; i < len(receiver); i++ {
		switch receiver[i] {
		case '(', '[':
			depth++
		case ')', ']':
			depth--
		case '.':
			if depth == 0 {
				segments = append(segments, receiver[start:i])
				start = i + 1
			}
		}
	}
	segments = append(segments, receiver[start:])
	return segments
}

func stripCall(segment string) (string, bool) {
	segment = strings.TrimRight(segment, "?!")
	if i := strings.IndexAny(segment, "(["); i >= 0 {
		return segment[:i], true
	}
	return segment, false
}

func (c *completer) guessForTypeName(name string, isType bool) typeGuess {
	guess := typeGuess{name: name, isType: isType}
	if c.ip != nil {
		guess.info = c.ip.lookupTypeInfo(name)
		if guess.info == nil {
			guess.info = c.ip.ext[name]
		}
	}
	if guess.info != nil && guess.info.IsView() {
		guess.isView = true
	}
	if isViewType(name) {
		guess.isView = !isType
	}
	return guess
}

func (c *completer) guessForTypeRef(ref *TypeRef) typeGuess {
	if ref == nil {
		return typeGuess{}
	}
	switch ref.Kind {
	case tyArray:
		return typeGuess{name: "Array", ref: ref}
	case tyDict:
		return typeGuess{name: "Dictionary", ref: ref}
	case tyNamed:
		guess := c.guessForTypeName(ref.Name, false)
		guess.ref = ref
		return guess
	}
	return typeGuess{}
}

func (c *completer) resolveFirst(segment string) typeGuess {
	name, called := stripCall(segment)
	if name == "" {
		if strings.HasPrefix(segment, "\"") {
			return typeGuess{name: "String"}
		}
		if strings.HasPrefix(segment, "[") {
			return typeGuess{name: "Array"}
		}
		return typeGuess{}
	}
	if name == "self" {
		if ti := c.enclosingType(); ti != nil {
			return typeGuess{name: ti.Name, info: ti, isView: ti.IsView()}
		}
		return typeGuess{}
	}
	if unicode.IsUpper(rune(name[0])) {
		if called {
			return c.guessForTypeName(name, false)
		}
		return c.guessForTypeName(name, true)
	}
	if ref := c.declaredType(name); ref != nil {
		return c.guessForTypeRef(ref)
	}
	if ti := c.enclosingType(); ti != nil {
		if f := ti.field(name); f != nil {
			return c.guessForField(ti, f)
		}
		if vd, ok := ti.Computed[name]; ok && vd.Type != nil {
			return c.guessForTypeRef(vd.Type)
		}
		if methods := ti.Methods[name]; methods != nil && methods[0].Result != nil {
			return c.guessForTypeRef(methods[0].Result)
		}
	}
	if c.ip != nil {
		if slot, ok := c.ip.globals.vars[name]; ok && slot.V != nil {
			return c.guessForValue(slot.V)
		}
	}
	return typeGuess{}
}

func (c *completer) guessForField(ti *TypeInfo, f *FieldInfo) typeGuess {
	if f.Type != nil {
		return c.guessForTypeRef(f.Type)
	}
	if f.Wrapper == "Environment" && len(f.WrapArgs) > 0 {
		if kp, ok := f.WrapArgs[0].X.(*KeyPathExpr); ok && len(kp.Path) > 0 {
			return c.guessForTypeName(environmentValueType(kp.Path[0]), false)
		}
	}
	if c.ip != nil && f.Init != nil {
		value := c.safeDefaultValue(ti, f)
		if value != nil {
			return c.guessForValue(value)
		}
	}
	return typeGuess{}
}

func environmentValueType(key string) string {
	switch key {
	case "modelContext":
		return "ModelContext"
	case "dismiss":
		return "DismissAction"
	case "colorScheme":
		return "ColorScheme"
	}
	return ""
}

func (c *completer) safeDefaultValue(ti *TypeInfo, f *FieldInfo) (value Value) {
	defer func() {
		if recover() != nil {
			value = nil
		}
	}()
	return c.ip.defaultFieldValue(ti, f)
}

func (c *completer) guessForValue(v Value) typeGuess {
	switch x := v.(type) {
	case *ArrayV:
		return typeGuess{name: "Array", ref: &TypeRef{Kind: tyArray, Elem: x.Elem}}
	case *DictV:
		return typeGuess{name: "Dictionary", ref: &TypeRef{Kind: tyDict, Key: x.KeyT, Elem: x.ValT}}
	case *StructV:
		return typeGuess{name: x.T.Name, info: x.T, isView: x.T.IsView()}
	case *ObjectV:
		return typeGuess{name: x.T.Name, info: x.T}
	case *EnumV:
		return typeGuess{name: x.T.Name, info: x.T}
	case *ViewV, *ModV:
		return typeGuess{name: "View", isView: true}
	case *BoxV:
		return c.guessForValue(x.V)
	}
	return c.guessForTypeName(typeName(v), false)
}

var typedDeclarationPattern = regexp.MustCompile(`\b(?:var|let)\s+%s\s*:\s*([^=\n{]+)`)
var constructedDeclarationPattern = regexp.MustCompile(`\b(?:var|let)\s+%s\s*=\s*(?:try\??\s*)?([A-Z]\w*)\s*[(.]`)
var literalDeclarationPattern = regexp.MustCompile(`\b(?:var|let)\s+%s\s*=\s*(".*?"|\d+\.\d+|\d+|true|false|\[|Date\(\)|\.now)`)
var parameterDeclarationPattern = regexp.MustCompile(`[(,]\s*(?:\w+\s+)?%s\s*:\s*([^,)=]+)`)
var forEachPattern = regexp.MustCompile(`(?:ForEach|List)\(\s*(\$?[\w.]+)[^{]*\{\s*(?:\(\s*)?%s\b`)
var forLoopPattern = regexp.MustCompile(`\bfor\s+%s\s+in\s+([\w.]+)`)

func (c *completer) declaredType(name string) *TypeRef {
	textAbove := strings.Join(c.lines[:c.line+1], "\n")
	quoted := regexp.QuoteMeta(name)
	find := func(pattern *regexp.Regexp) string {
		specific := regexp.MustCompile(strings.Replace(pattern.String(), "%s", quoted, 1))
		matches := specific.FindAllStringSubmatch(textAbove, -1)
		if len(matches) == 0 {
			return ""
		}
		return strings.TrimSpace(matches[len(matches)-1][1])
	}
	if typeText := find(typedDeclarationPattern); typeText != "" {
		return parseTypeText(typeText)
	}
	if typeText := find(parameterDeclarationPattern); typeText != "" {
		return parseTypeText(typeText)
	}
	if constructed := find(constructedDeclarationPattern); constructed != "" {
		return &TypeRef{Name: constructed}
	}
	if literal := find(literalDeclarationPattern); literal != "" {
		switch {
		case strings.HasPrefix(literal, "\""):
			return &TypeRef{Name: "String"}
		case literal == "true" || literal == "false":
			return &TypeRef{Name: "Bool"}
		case literal == "[":
			return &TypeRef{Kind: tyArray}
		case literal == "Date()" || literal == ".now":
			return &TypeRef{Name: "Date"}
		case strings.Contains(literal, "."):
			return &TypeRef{Name: "Double"}
		default:
			return &TypeRef{Name: "Int"}
		}
	}
	for _, pattern := range []*regexp.Regexp{forEachPattern, forLoopPattern} {
		if collection := find(pattern); collection != "" {
			collectionType := c.resolveChain(strings.TrimPrefix(collection, "$"))
			if collectionType.ref != nil && collectionType.ref.Kind == tyArray {
				return collectionType.ref.Elem
			}
		}
	}
	return nil
}

func parseTypeText(text string) *TypeRef {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "[") && strings.HasSuffix(text, "]") {
		inner := text[1 : len(text)-1]
		if colon := strings.Index(inner, ":"); colon >= 0 {
			return &TypeRef{Kind: tyDict, Key: parseTypeText(inner[:colon]), Elem: parseTypeText(inner[colon+1:])}
		}
		return &TypeRef{Kind: tyArray, Elem: parseTypeText(inner)}
	}
	optional := strings.HasSuffix(text, "?")
	text = strings.TrimSuffix(text, "?")
	if open := strings.Index(text, "<"); open >= 0 {
		return &TypeRef{Name: text[:open], Args: []*TypeRef{parseTypeText(strings.TrimSuffix(text[open+1:], ">"))}, Opt: optional}
	}
	return &TypeRef{Name: strings.TrimPrefix(text, "some "), Opt: optional}
}

func (c *completer) resolveMember(current typeGuess, segment string) typeGuess {
	name, _ := stripCall(segment)
	if current.ref != nil && current.ref.Kind == tyArray {
		switch name {
		case "first", "last", "randomElement", "min", "max", "removeFirst", "removeLast", "popLast":
			return c.guessForTypeRef(current.ref.Elem)
		case "count", "firstIndex", "lastIndex":
			return typeGuess{name: "Int"}
		case "isEmpty", "contains", "allSatisfy":
			return typeGuess{name: "Bool"}
		case "filter", "sorted", "reversed", "shuffled", "prefix", "suffix", "dropFirst", "dropLast":
			return current
		case "joined":
			return typeGuess{name: "String"}
		}
		return typeGuess{name: "Array"}
	}
	if current.info != nil {
		if f := current.info.field(name); f != nil {
			return c.guessForField(current.info, f)
		}
		if vd, ok := current.info.Computed[name]; ok {
			if vd.Type != nil {
				return c.guessForTypeRef(vd.Type)
			}
			return typeGuess{}
		}
		if methods := c.ip.findMethods(current.info, name); methods != nil {
			if methods[0].Result != nil {
				return c.guessForTypeRef(methods[0].Result)
			}
			return typeGuess{}
		}
		if current.info.Kind == "enum" {
			if _, isCase := current.info.CaseMap[name]; isCase {
				return typeGuess{name: current.info.Name, info: current.info}
			}
			if name == "rawValue" {
				return typeGuess{name: current.info.RawType}
			}
		}
		if current.isType && name == "self" {
			return current
		}
	}
	switch current.name {
	case "String":
		switch name {
		case "count":
			return typeGuess{name: "Int"}
		case "isEmpty", "contains", "hasPrefix", "hasSuffix":
			return typeGuess{name: "Bool"}
		case "split", "components":
			return typeGuess{name: "Array", ref: &TypeRef{Kind: tyArray, Elem: &TypeRef{Name: "String"}}}
		}
		return typeGuess{name: "String"}
	case "Date":
		if name == "formatted" {
			return typeGuess{name: "String"}
		}
		return typeGuess{name: "Double"}
	case "ModelContainer":
		if name == "mainContext" {
			return typeGuess{name: "ModelContext"}
		}
	case "Color", "Font", "Text":
		return typeGuess{name: current.name, isView: current.name == "Text"}
	case "Dictionary":
		if current.ref != nil && (name == "keys" || name == "values") {
			elem := current.ref.Key
			if name == "values" {
				elem = current.ref.Elem
			}
			return typeGuess{name: "Array", ref: &TypeRef{Kind: tyArray, Elem: elem}}
		}
	}
	if current.isView || isViewType(current.name) || knownModifiers[name] {
		return typeGuess{name: "View", isView: true}
	}
	return typeGuess{}
}

// ---------- producing members ----------

func (c *completer) membersOf(guess typeGuess) {
	if guess.info != nil {
		c.addTypeMembers(guess.info, guess.isType)
	}
	if guess.isType {
		if statics, ok := builtinStaticMembers[guess.name]; ok {
			c.addNames("static", strings.Fields(statics), guess.name)
		}
		if guess.info == nil && isViewType(guess.name) {
			for _, doc := range viewDocs[guess.name] {
				c.addDoc("static", "init", insertFromDocSignature(strings.Replace(doc.Signature, guess.name, "init", 1)), doc)
			}
		}
		if guess.info == nil && c.ip != nil {
			if ext, ok := c.ip.ext[guess.name]; ok {
				c.addTypeMembers(ext, true)
			}
		}
		return
	}
	if members, ok := builtinTypeMembers[guess.name]; ok {
		c.addNames("method", strings.Fields(members), guess.name)
	}
	if guess.ref != nil && guess.ref.Opt {
		c.addNames("method", []string{"map(_:)", "flatMap(_:)"}, "Optional")
	}
	if c.ip != nil {
		if ext, ok := c.ip.ext[guess.name]; ok {
			c.addTypeMembers(ext, false)
		}
	}
	if guess.isView || guess.name == "View" || guess.name == "" || (guess.info != nil && guess.info.IsView()) {
		c.addModifiers()
	}
	if guess.name == "" {
		c.addNames("static", implicitMembersFallback, "")
	}
}

func (c *completer) addTypeMembers(ti *TypeInfo, static bool) {
	for t := ti; t != nil; t = t.Super {
		if static {
			for name, vd := range t.StaticFields {
				c.add("static", name, name, vd.Type.String(), "")
			}
			for name, methods := range t.StaticMethods {
				c.add("static", name, insertFromParams(name, methods[0].Params), signatureOf(name, methods[0]), "")
			}
			for _, enumCase := range t.Cases {
				c.add("case", enumCase.Name, insertFromParams(enumCase.Name, enumCase.Params), "case", "")
			}
			for _, init := range t.Inits {
				c.add("static", "init", insertFromParams("init", init.Params), signatureOf("init", init), "")
			}
			if len(t.Inits) == 0 && t.Kind == "struct" {
				var params []*Param
				for _, f := range t.Fields {
					if f.Wrapper == "" {
						params = append(params, &Param{Label: f.Name, Name: f.Name, Type: f.Type, Default: f.Init})
					}
				}
				c.add("static", "init", insertFromParams("init", params), signatureOf("init", &FuncDecl{Params: params}), "memberwise initializer")
			}
			for name, nested := range c.nestedTypes(t) {
				c.add("type", name, name, nested.Kind, "")
			}
			continue
		}
		for _, f := range t.Fields {
			detail := ""
			if f.Type != nil {
				detail = f.Type.String()
			}
			c.add("field", f.Name, f.Name, detail, wrapperNote(f))
			if f.Wrapper == "State" || f.Wrapper == "Binding" || f.Wrapper == "Bindable" || f.Wrapper == "AppStorage" || f.Wrapper == "FocusState" {
				c.add("field", "$"+f.Name, "$"+f.Name, "Binding", "")
			}
		}
		for name, vd := range t.Computed {
			detail := ""
			if vd.Type != nil {
				detail = vd.Type.String()
			}
			c.add("field", name, name, detail, "computed")
		}
		for name, methods := range t.Methods {
			c.add("method", name, insertFromParams(name, methods[0].Params), signatureOf(name, methods[0]), "")
		}
		for _, enumCase := range t.Cases {
			c.add("case", enumCase.Name, insertFromParams(enumCase.Name, enumCase.Params), "case", "")
		}
		if t.Kind == "enum" && t.RawType != "" {
			c.add("field", "rawValue", "rawValue", t.RawType, "")
		}
		if t.IsModel {
			c.addNames("field", []string{"modelContext", "persistentModelID", "hasChanges"}, "PersistentModel")
		}
	}
}

func wrapperNote(f *FieldInfo) string {
	if f.Wrapper == "" {
		return ""
	}
	return "@" + f.Wrapper
}

func (c *completer) nestedTypes(ti *TypeInfo) map[string]*TypeInfo {
	nested := map[string]*TypeInfo{}
	if c.ip == nil {
		return nested
	}
	for name, t := range c.ip.types {
		if strings.HasPrefix(name, ti.Name+".") && !strings.Contains(name[len(ti.Name)+1:], ".") {
			nested[name[len(ti.Name)+1:]] = t
		}
	}
	return nested
}

func (rt *Runtime) handleDocs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{"modifiers": modifierDocs, "views": viewDocs, "types": typeDocs})
}
