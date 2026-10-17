package switaculous

import (
	"strings"
	"unicode"
)

var viewLabels = map[string]string{
	"Text":                   "_ verbatim style format tableName bundle comment",
	"Image":                  "_ systemName bundle label decorative uiImage variableValue",
	"Label":                  "_ systemImage image title icon",
	"Button":                 "_ action label role systemImage image intent",
	"Toggle":                 "_ isOn label systemImage image sources",
	"TextField":              "_ text prompt axis value format label formatter onEditingChanged onCommit",
	"SecureField":            "_ text prompt label onCommit",
	"TextEditor":             "text selection",
	"Slider":                 "value in step label minimumValueLabel maximumValueLabel onEditingChanged",
	"Stepper":                "_ value in step label onIncrement onDecrement onEditingChanged format",
	"Picker":                 "_ selection content label sources currentValueLabel systemImage image",
	"DatePicker":             "_ selection in displayedComponents label",
	"ColorPicker":            "_ selection supportsOpacity label",
	"ProgressView":           "_ value total label currentValueLabel timerInterval countsDown",
	"Gauge":                  "value in label currentValueLabel minimumValueLabel maximumValueLabel",
	"Link":                   "_ destination label",
	"Menu":                   "_ content label primaryAction systemImage image",
	"ShareLink":              "_ item items subject message preview label",
	"VStack":                 "alignment spacing content",
	"HStack":                 "alignment spacing content",
	"ZStack":                 "alignment content",
	"LazyVStack":             "alignment spacing pinnedViews content",
	"LazyHStack":             "alignment spacing pinnedViews content",
	"LazyVGrid":              "columns alignment spacing pinnedViews content",
	"LazyHGrid":              "rows alignment spacing pinnedViews content",
	"Grid":                   "alignment horizontalSpacing verticalSpacing content",
	"GridRow":                "alignment content",
	"ScrollView":             "_ showsIndicators content",
	"ScrollViewReader":       "content",
	"List":                   "_ selection id rowContent content children editActions",
	"ForEach":                "_ id content editActions",
	"Section":                "_ header footer content isExpanded",
	"Form":                   "content",
	"Group":                  "content",
	"GlassEffectContainer":   "spacing content",
	"NavigationStack":        "path root content",
	"NavigationView":         "content",
	"NavigationSplitView":    "sidebar content detail columnVisibility preferredCompactColumn",
	"NavigationLink":         "_ destination label value isActive tag selection",
	"TabView":                "selection content",
	"Tab":                    "_ systemImage image value role content label",
	"GroupBox":               "_ label content",
	"DisclosureGroup":        "_ isExpanded content label",
	"LabeledContent":         "_ value format content label",
	"ContentUnavailableView": "_ systemImage image description label actions",
	"Spacer":                 "minLength",
	"Divider":                "",
	"EmptyView":              "",
	"Rectangle":              "",
	"Circle":                 "",
	"Ellipse":                "",
	"Capsule":                "style",
	"RoundedRectangle":       "cornerRadius cornerSize style",
	"UnevenRoundedRectangle": "topLeadingRadius bottomLeadingRadius bottomTrailingRadius topTrailingRadius cornerRadii style",
	"LinearGradient":         "colors gradient stops startPoint endPoint",
	"RadialGradient":         "colors gradient stops center startRadius endRadius",
	"AngularGradient":        "colors gradient stops center startAngle endAngle angle",
	"EllipticalGradient":     "colors gradient stops center startRadiusFraction endRadiusFraction",
	"GeometryReader":         "content",
	"AsyncImage":             "url scale content placeholder transaction",
	"ViewThatFits":           "in content",
	"TimelineView":           "_ content",
	"Canvas":                 "opaque colorMode rendersAsynchronously renderer symbols",
	"Map":                    "*",
	"AnyView":                "_ erasing",
	"WindowGroup":            "_ id for content",
	"ControlGroup":           "_ content label",
	"EditButton":             "",
	"ToolbarItem":            "id placement content",
	"ToolbarItemGroup":       "placement content",
	"Path":                   "_",
	"VideoPlayer":            "*",
	"PhotosPicker":           "*",
	"Chart":                  "*",
	"SpriteView":             "*",
	"Settings":               "content",
	"PasteButton":            "*",
	"RenameButton":           "",
	"TextFieldLink":          "*",
	"HelpLink":               "*",
	"MultiDatePicker":        "*",
	"Layout":                 "*",
}

var unsupportedViews = map[string]bool{
	"VideoPlayer": true, "Chart": true, "SpriteView": true,
	"Canvas": true, "PasteButton": true, "TextFieldLink": true, "HelpLink": true, "MultiDatePicker": true,
}

func isViewType(name string) bool {
	_, ok := viewLabels[name]
	return ok
}

var localizedTypes = map[string]bool{
	"Text": true, "Button": true, "Label": true, "Toggle": true, "NavigationLink": true, "Section": true, "Link": true,
	"Menu": true, "TextField": true, "SecureField": true, "Picker": true, "Stepper": true, "DatePicker": true,
	"ColorPicker": true, "ProgressView": true, "LabeledContent": true, "DisclosureGroup": true, "GroupBox": true,
	"ContentUnavailableView": true, "Tab": true, "ShareLink": true, "ControlGroup": true,
}

var builtinTypeNames = map[string]bool{}

func init() {
	for k := range viewLabels {
		builtinTypeNames[k] = true
	}
	for _, n := range strings.Fields(`Int Int8 Int16 Int32 Int64 UInt UInt8 UInt16 UInt32 UInt64 Double Float Float32 Float64 CGFloat
	TimeInterval String Substring Character Bool Array Dictionary Set ContiguousArray ArraySlice Optional Void Never Any AnyObject
	AnyHashable UUID Date URL Color UIColor Font UIFont Animation AnyTransition Edge Alignment HorizontalAlignment VerticalAlignment
	Axis ButtonRole ContentMode UnitPoint Angle CGSize CGPoint CGRect EdgeInsets Gradient GridItem Timer DispatchQueue DispatchTime
	Task Calendar DateFormatter NumberFormatter ISO8601DateFormatter RelativeDateTimeFormatter DateComponentsFormatter DateComponents
	Locale TimeZone Scanner State Binding StateObject ObservedObject Alert ActionSheet ToolbarItemPlacement UIScreen UIApplication
	MainActor LocalizedStringKey AttributedString LocalizedStringResource Text.Case StrokeStyle CharacterSet NSNumber
	UIImpactFeedbackGenerator UINotificationFeedbackGenerator ShapeStyle View Shape Material TextAlignment Visibility
	ScenePhase ColorScheme ListStyle ButtonStyle PresentationDetent JSONDecoder JSONEncoder URLSession FileManager ImageRenderer UserDefaults
	NotificationCenter Transaction ScrollViewProxy GeometryProxy Image.Scale Font.Weight Font.Design RoundedCornerStyle
	ToolbarPlacement Comparable Equatable Hashable Identifiable Error CaseIterable Codable ObservableObject EnvironmentValues
	SymbolRenderingMode KeyboardType UIKeyboardType TextInputAutocapitalization SubmitLabel DynamicTypeSize ControlSize Prominence
	ScrollDismissesKeyboardMode BlendMode ContentSizeCategory Namespace Published Environment EnvironmentObject FocusState AppStorage
	SceneStorage Bindable Observable LinearGradient RadialGradient AngularGradient Duration Swift Foundation SwiftUI Result
	NSString PresentationMode EditMode HapticFeedback SensoryFeedback Selection TabViewStyle PageTabViewStyle NavigationPath IndexSet DragGesture TapGesture LongPressGesture MagnificationGesture MagnifyGesture RotationGesture RotateGesture SpatialTapGesture ModelContainer
	ModelContext ModelConfiguration Schema FetchDescriptor SortDescriptor SortOrder Query Predicate PersistentIdentifier
	PersistentModel Data UIImage NSImage PhotosPickerItem PhotosPickerSelectionBehavior PHPickerFilter PHPickerConfiguration
	PHPickerViewController PHPickerResult NSItemProvider UIImagePickerController UIViewControllerRepresentable
	UIViewRepresentable NSObject UINavigationControllerDelegate UIImagePickerControllerDelegate PHPickerViewControllerDelegate
	UIViewController UIView Transferable Bundle UTF8 Unicode CLLocationCoordinate2D MKCoordinateRegion MKCoordinateSpan CLLocation MKMapItem MKPlacemark MapCameraPosition MKMapRect MKMapPoint CLGeocoder MapCameraBounds MapStyle Marker Annotation MapPolyline MapCircle UserAnnotation JSONEncoder JSONDecoder DecodingError EncodingError CodingKey PropertyListEncoder PropertyListDecoder`) {
		builtinTypeNames[n] = true
	}
}

var knownModifiers = map[string]bool{}
var noopModifiers = map[string]bool{}

func init() {
	for _, n := range strings.Fields(`padding frame background overlay foregroundColor foregroundStyle tint accentColor font fontWeight bold
	italic fontDesign fontWidth monospaced monospacedDigit underline strikethrough kerning tracking textCase lineLimit lineSpacing
	multilineTextAlignment truncationMode minimumScaleFactor allowsTightening cornerRadius clipShape clipped mask shadow opacity
	scaleEffect rotationEffect rotation3DEffect offset position blur brightness contrast saturation grayscale hueRotation colorInvert
	colorMultiply blendMode border zIndex layoutPriority fixedSize aspectRatio scaledToFit scaledToFill ignoresSafeArea
	edgesIgnoringSafeArea hidden disabled allowsHitTesting onTapGesture onLongPressGesture animation transition listRowBackground
	listRowInsets listRowSeparator listRowSeparatorTint listSectionSeparator scrollContentBackground imageScale symbolRenderingMode
	symbolVariant resizable renderingMode interpolation antialiased safeAreaInset badge buttonStyle textFieldStyle pickerStyle
	toggleStyle labelStyle listStyle datePickerStyle progressViewStyle menuStyle tabViewStyle gaugeStyle groupBoxStyle
	indexViewStyle controlSize labelsHidden environmentObject environment preferredColorScheme navigationTitle navigationSubtitle navigationBarTitle
	navigationBarTitleDisplayMode toolbar navigationBarItems navigationBarBackButtonHidden navigationBarHidden searchable
	navigationDestination toolbarBackground toolbarColorScheme toolbarTitleDisplayMode tabItem tag sheet fullScreenCover
	popover alert confirmationDialog actionSheet onAppear onDisappear task onChange onReceive refreshable onSubmit onDelete
	onMove swipeActions contextMenu modifier id fill stroke strokeBorder trim inset focused gesture simultaneousGesture
	highPriorityGesture presentationDetents presentationDragIndicator presentationCornerRadius presentationBackground
	interactiveDismissDisabled statusBarHidden statusBar containerRelativeFrame scrollIndicators scrollTargetBehavior
	scrollTargetLayout textSelection contentShape listRowSpacing listSectionSpacing headerProminence buttonBorderShape
	dynamicTypeSize sensoryFeedback contentTransition matchedGeometryEffect drawingGroup compositingGroup redacted
	privacySensitive defaultScrollAnchor persistentSystemOverlays coordinateSpace onHover help focusable keyboardType
	textInputAutocapitalization autocapitalization autocorrectionDisabled disableAutocorrection textContentType submitLabel
	scrollDismissesKeyboard accessibilityLabel accessibilityHint accessibilityValue accessibilityIdentifier accessibilityHidden
	accessibilityElement accessibilityAddTraits accessibilityRemoveTraits accessibilityAction accessibilitySortPriority
	accessibilityAdjustableAction accessibilityShowsLargeContentViewer equatable navigationSplitViewStyle navigationViewStyle
	contentMargins scrollClipDisabled scrollPosition scrollBounceBehavior scrollDisabled presentationCompactAdaptation
	presentationBackgroundInteraction toolbarRole navigationBarTitleTextColor tableStyle onOpenURL handlesExternalEvents
	symbolEffect phaseAnimator keyframeAnimator geometryGroup visualEffect containerShape fontWeightCompat environmentValues
	safeAreaPadding backgroundStyle foregroundStyleCompat listItemTint lineHeight baselineOffset onKeyPress defaultFocus
	searchSuggestions searchScopes onDrag onDrop draggable dropDestination fileImporter fileExporter photosPicker
	scenePadding typesettingLanguage speechAnnouncementsQueued labelReservedIconWidth formStyle tabItemCompat
	monospacedCompat inspector sheetCompat toolbarVisibility toolbarBackgroundVisibility navigationBarBackButtonTitle
	onGeometryChange onScrollGeometryChange scrollTransition tabBarMinimizeBehavior glassEffect backgroundExtensionEffect modelContainer modelContext defersSystemGestures`) {
		knownModifiers[n] = true
	}
	for _, n := range strings.Fields(`accessibilityLabel accessibilityHint accessibilityValue accessibilityIdentifier accessibilityHidden
	accessibilityElement accessibilityAddTraits accessibilityRemoveTraits accessibilityAction accessibilitySortPriority
	accessibilityAdjustableAction accessibilityShowsLargeContentViewer keyboardType textInputAutocapitalization autocapitalization
	autocorrectionDisabled disableAutocorrection textContentType submitLabel scrollDismissesKeyboard equatable drawingGroup
	compositingGroup privacySensitive defaultScrollAnchor persistentSystemOverlays coordinateSpace onHover help focusable
	dynamicTypeSize sensoryFeedback contentTransition matchedGeometryEffect scrollTargetBehavior scrollTargetLayout
	textSelection contentShape navigationSplitViewStyle navigationViewStyle scrollClipDisabled scrollPosition scrollBounceBehavior
	presentationCompactAdaptation presentationBackgroundInteraction toolbarRole tableStyle onOpenURL handlesExternalEvents
	symbolEffect geometryGroup containerShape interpolation antialiased indexViewStyle headerProminence buttonBorderShape
	presentationDragIndicator interactiveDismissDisabled presentationCornerRadius refreshable onKeyPress defaultFocus searchSuggestions searchScopes onDrag onDrop draggable dropDestination
	fileImporter fileExporter typesettingLanguage speechAnnouncementsQueued labelReservedIconWidth formStyle toolbarColorScheme
	toolbarTitleDisplayMode statusBar statusBarHidden onScrollGeometryChange scrollTransition onGeometryChange
	tabBarMinimizeBehavior backgroundExtensionEffect navigationBarBackButtonTitle defersSystemGestures glassEffect listItemTint menuStyle gaugeStyle
	groupBoxStyle redacted visualEffect phaseAnimator keyframeAnimator inspector`) {
		noopModifiers[n] = true
	}
}

func (ip *Interp) makeView(name string, args []ArgV, pos Pos, env *Env) Value {
	if unsupportedViews[name] {
		ip.warn(pos, "'%s' is not supported by the preview and is shown as a placeholder", name)
		return &ViewV{Kind: "Placeholder", Args: args, Pos: pos, Extra: map[string]Value{"name": name}}
	}
	allowed := viewLabels[name]
	if allowed != "*" {
		set := map[string]bool{}
		for _, l := range strings.Fields(allowed) {
			set[l] = true
		}
		for _, a := range args {
			l := a.Label
			if l == "" {
				if a.Trailing || set["_"] || set["content"] {
					continue
				}
				if len(set) == 0 {
					ip.fail(pos, "argument passed to call that takes no arguments")
				}
				ip.fail(pos, "missing argument label in call to '%s'", name)
			}
			if !set[l] {
				ip.fail(pos, "extra argument '%s' in call to '%s'", l, name)
			}
		}
	}
	for i := range args {
		args[i].V = constBinding(args[i].V)
	}
	v := &ViewV{Kind: name, Args: args, Pos: pos, Extra: map[string]Value{}}
	switch name {
	case "Text":
		if len(args) == 0 {
			ip.fail(pos, "missing argument for parameter #1 in call to 'Text'")
		}
		a := args[0]
		if a.Label == "" {
			if _, isLit := a.X.(*StrLit); isLit {
				v.Extra["md"] = true
			}
			switch a.V.(type) {
			case string, Character, *DateV:
			default:
				if _, ok := argNamed(args, "format"); !ok {
					if _, ok := argNamed(args, "style"); !ok {
						ip.fail(pos, "no exact matches in call to initializer: Text needs a String, but got '%s' (use Text(\"\\(value)\"))", typeName(a.V))
					}
				}
			}
		}
	case "AnyView":
		if len(args) > 0 {
			return args[0].V
		}
	case "Image":
		if len(args) == 0 {
			ip.fail(pos, "missing argument for parameter #1 in call to 'Image'")
		}
	case "NavigationLink", "Button", "Toggle", "Picker", "Stepper", "Slider", "TextField", "SecureField":
		if name == "Toggle" {
			if _, ok := argNamed(args, "isOn"); !ok {
				ip.fail(pos, "missing argument for parameter 'isOn' in call")
			}
		}
		if name == "TextField" || name == "SecureField" {
			_, t := argNamed(args, "text")
			_, vv := argNamed(args, "value")
			if !t && !vv {
				ip.fail(pos, "missing argument for parameter 'text' in call")
			}
		}
		if name == "Slider" {
			if _, ok := argNamed(args, "value"); !ok {
				ip.fail(pos, "missing argument for parameter 'value' in call")
			}
		}
		for i := range args {
			args[i].V = constBinding(args[i].V)
		}
		for _, l := range []string{"isOn", "text", "selection", "value"} {
			if bv, ok := argNamed(args, l); ok {
				if l == "value" && name == "NavigationLink" {
					continue
				}
				if l == "value" && name == "TextField" {
					if _, ok := bv.(*BindingV); !ok {
						ip.fail(pos, "cannot convert value of type '%s' to expected argument type 'Binding' (use $)", typeName(bv))
					}
					continue
				}
				if _, ok := bv.(*BindingV); !ok {
					ip.fail(pos, "cannot convert value of type '%s' to expected argument type 'Binding<%s>' — pass a binding with $ (e.g. %s: $myValue)", typeName(bv), typeName(bv), l)
				}
			}
		}
	}
	return v
}

func isTextValue(v Value) bool {
	switch x := v.(type) {
	case *ViewV:
		return x.Kind == "Text" || x.Kind == "TextConcat"
	case *ModV:
		return isTextValue(x.Base)
	}
	return false
}

func concatText(l, r Value) Value {
	var kids []Value
	if lv, ok := l.(*ViewV); ok && lv.Kind == "TextConcat" {
		kids = append(kids, lv.Kids...)
	} else {
		kids = append(kids, l)
	}
	if rv, ok := r.(*ViewV); ok && rv.Kind == "TextConcat" {
		kids = append(kids, rv.Kids...)
	} else {
		kids = append(kids, r)
	}
	return &ViewV{Kind: "TextConcat", Kids: kids}
}

// ---------- colors ----------

var namedColorAlias = map[string]string{
	"systemRed": "red", "systemOrange": "orange", "systemYellow": "yellow", "systemGreen": "green", "systemMint": "mint",
	"systemTeal": "teal", "systemCyan": "cyan", "systemBlue": "blue", "systemIndigo": "indigo", "systemPurple": "purple",
	"systemPink": "pink", "systemBrown": "brown", "systemGray": "gray", "label": "primary", "secondaryLabel": "secondary",
	"tertiaryLabel": "tertiary", "quaternaryLabel": "quaternary", "accentColor": "accentColor", "tintColor": "accentColor",
	"lightGray": "lightGray", "darkGray": "darkGray", "darkText": "black", "lightText": "white", "link": "blue",
}

var namedColors = map[string]bool{}

func init() {
	for _, n := range strings.Fields(`red orange yellow green mint teal cyan blue indigo purple pink brown gray black white clear primary
	secondary tertiary quaternary accentColor systemBackground secondarySystemBackground tertiarySystemBackground
	systemGroupedBackground secondarySystemGroupedBackground tertiarySystemGroupedBackground systemGray2 systemGray3 systemGray4
	systemGray5 systemGray6 separator opaqueSeparator systemFill secondarySystemFill tertiarySystemFill quaternarySystemFill
	lightGray darkGray placeholderText`) {
		namedColors[n] = true
	}
}

func (ip *Interp) namedColor(name string) *ColorV {
	if a, ok := namedColorAlias[name]; ok {
		name = a
	}
	if namedColors[name] {
		return &ColorV{Name: name, Op: 1}
	}
	if asset := ip.assetColorForSymbol(name); asset != "" {
		return &ColorV{Name: assetColorName(asset), Op: 1}
	}
	return nil
}

// Xcode generates `Color.brandGreen` for a "BrandGreen" color set: the symbol
// is the asset name with a lowercase first letter and punctuation removed.
func (ip *Interp) assetColorForSymbol(symbol string) string {
	if ip.rt == nil {
		return ""
	}
	for asset := range ip.rt.colorAssets {
		if assetSymbolName(asset) == symbol {
			return asset
		}
	}
	return ""
}

func assetSymbolName(asset string) string {
	var out strings.Builder
	upperNext := false
	for i, r := range asset {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			upperNext = true
			continue
		}
		switch {
		case i == 0:
			out.WriteRune(unicode.ToLower(r))
		case upperNext:
			out.WriteRune(unicode.ToUpper(r))
		default:
			out.WriteRune(r)
		}
		upperNext = false
	}
	return out.String()
}

func (ip *Interp) colorFromSym(s *SymV) Value {
	name := s.Name()
	var base Value
	if et, ok := ip.ext["Color"]; ok {
		if sl, ok := ip.staticField(et, name); ok {
			base = sl.get()
		}
	}
	if base == nil {
		if et, ok := ip.ext["ShapeStyle"]; ok {
			if sl, ok := ip.staticField(et, name); ok {
				base = sl.get()
			}
		}
	}
	if base == nil {
		if c := ip.namedColor(name); c != nil {
			base = c
		} else if name == "init" && s.Parts[0].Called {
			base = ip.builtinColor(s.Parts[0].Args, Pos{})
		} else {
			return nil
		}
	}
	if len(s.Parts) > 1 {
		return ip.applySymRest(base, s, Pos{})
	}
	return base
}

// Asset colours become CSS variables named --c-asset-<name> in the preview.
func assetColorName(name string) string {
	var out strings.Builder
	out.WriteString("asset-")
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			out.WriteRune(r)
		} else {
			out.WriteByte('-')
		}
	}
	return out.String()
}

func (ip *Interp) builtinColor(args []ArgV, pos Pos) Value {
	c := &ColorV{A: 1, Op: 1}
	if len(args) == 0 {
		return &ColorV{Name: "clear", Op: 1}
	}
	if s, ok := args[0].V.(string); ok && args[0].Label == "" {
		if ip.rt != nil {
			if _, found := ip.rt.colorAssets[s]; found {
				return &ColorV{Name: assetColorName(s), Op: 1}
			}
		}
		ip.warn(pos, "color '%s' was not found in an Assets.xcassets folder; shown as gray", s)
		return &ColorV{Name: "gray", Op: 1}
	}
	if s, ok := args[0].V.(*SymV); ok && args[0].Label == "" {
		if s.Name() == "sRGB" || s.Name() == "displayP3" || s.Name() == "sRGBLinear" {
			args = args[1:]
		} else if nc := ip.namedColor(s.Name()); nc != nil {
			return nc
		}
	}
	if cv, ok := args[0].V.(*ColorV); ok {
		return cv
	}
	if v, ok := argNamed(args, "uiColor"); ok {
		if cv, ok := v.(*ColorV); ok {
			return cv
		}
		if s, ok := v.(*SymV); ok {
			if nc := ip.namedColor(s.Name()); nc != nil {
				return nc
			}
		}
	}
	get := func(l string) (float64, bool) {
		v, ok := argNamed(args, l)
		if !ok {
			return 0, false
		}
		f, _ := toFloat(v)
		return f, true
	}
	if r, ok := get("red"); ok {
		g, _ := get("green")
		b, _ := get("blue")
		c.R, c.G, c.B = r, g, b
	} else if w, ok := get("white"); ok {
		c.R, c.G, c.B = w, w, w
	} else if h, ok := get("hue"); ok {
		s, _ := get("saturation")
		br, _ := get("brightness")
		c.R, c.G, c.B = hsbToRGB(h, s, br)
	}
	if o, ok := get("opacity"); ok {
		c.A = o
	} else if o, ok := get("alpha"); ok {
		c.A = o
	}
	return c
}

func hsbToRGB(h, s, v float64) (float64, float64, float64) {
	h = h - float64(int(h))
	i := int(h * 6)
	f := h*6 - float64(i)
	p := v * (1 - s)
	q := v * (1 - f*s)
	t := v * (1 - (1-f)*s)
	switch i % 6 {
	case 0:
		return v, t, p
	case 1:
		return q, v, p
	case 2:
		return p, v, t
	case 3:
		return p, q, v
	case 4:
		return t, p, v
	}
	return v, p, q
}

func fontFromSym(s *SymV) *FontV {
	f := &FontV{}
	name := s.Name()
	switch name {
	case "largeTitle", "title", "title2", "title3", "headline", "subheadline", "body", "callout", "footnote", "caption", "caption2", "extraLargeTitle", "extraLargeTitle2":
		f.Style = name
	case "system":
		args := s.Parts[0].Args
		f.Style = "body"
		for _, a := range args {
			switch a.Label {
			case "size":
				f.Size, _ = toFloat(a.V)
				f.Style = ""
			case "weight":
				if w, ok := a.V.(*SymV); ok {
					f.Weight = w.Name()
				}
			case "design":
				if d, ok := a.V.(*SymV); ok {
					f.Design = d.Name()
				}
			case "":
				if st, ok := a.V.(*SymV); ok {
					f.Style = st.Name()
				}
			}
		}
	case "custom":
		args := s.Parts[0].Args
		f.Style = "body"
		for _, a := range args {
			switch a.Label {
			case "size", "fixedSize":
				f.Size, _ = toFloat(a.V)
				f.Style = ""
			case "relativeTo":
			case "":
				if n, ok := a.V.(string); ok {
					f.Name = n
				}
			}
		}
	default:
		return nil
	}
	for _, p := range s.Parts[1:] {
		switch p.Name {
		case "bold":
			f.Weight = "bold"
		case "italic":
			f.Italic = true
		case "weight":
			if len(p.Args) > 0 {
				if w, ok := p.Args[0].V.(*SymV); ok {
					f.Weight = w.Name()
				}
			}
		case "monospaced", "monospacedDigit":
			f.Mono = true
		}
	}
	return f
}

func (ip *Interp) resolveSymAs(typ string, s *SymV) Value {
	switch typ {
	case "Color", "UIColor":
		return ip.colorFromSym(s)
	case "Font":
		if f := fontFromSym(s); f != nil {
			return f
		}
		if et, ok := ip.ext["Font"]; ok {
			if sl, ok := ip.staticField(et, s.Name()); ok {
				return sl.get()
			}
		}
	case "Double", "CGFloat", "Float", "TimeInterval":
		switch s.Name() {
		case "pi":
			return 3.141592653589793
		case "infinity":
			return posInf
		case "zero":
			return 0.0
		}
	case "Date":
		if s.Name() == "now" {
			return ip.builtinStatic("Date", "now", Pos{})
		}
	case "Int":
		switch s.Name() {
		case "zero":
			return 0
		case "max":
			return ip.builtinStatic("Int", "max", Pos{})
		}
	}
	return nil
}

func (ip *Interp) resolveSymLoose(s *SymV) Value {
	switch s.Name() {
	case "infinity":
		return posInf
	case "pi":
		return 3.141592653589793
	case "zero":
		return 0
	}
	return nil
}

func constBinding(v Value) Value {
	if s, ok := v.(*SymV); ok && s.Name() == "constant" && len(s.Parts) == 1 && len(s.Args()) > 0 {
		val := copyValue(s.Args()[0].V)
		return &BindingV{Get: func() Value { return val }, Set: func(Value) {}}
	}
	return v
}

var styleTypeNames = map[string]string{
	"PlainListStyle": "plain", "GroupedListStyle": "grouped", "InsetGroupedListStyle": "insetGrouped", "InsetListStyle": "inset",
	"SidebarListStyle": "sidebar", "DefaultListStyle": "automatic", "PlainButtonStyle": "plain", "BorderlessButtonStyle": "borderless",
	"BorderedButtonStyle": "bordered", "BorderedProminentButtonStyle": "borderedProminent", "DefaultButtonStyle": "automatic",
	"RoundedBorderTextFieldStyle": "roundedBorder", "PlainTextFieldStyle": "plain", "DefaultTextFieldStyle": "automatic",
	"SegmentedPickerStyle": "segmented", "MenuPickerStyle": "menu", "WheelPickerStyle": "wheel", "InlinePickerStyle": "inline",
	"DefaultPickerStyle": "automatic", "NavigationLinkPickerStyle": "navigationLink", "SwitchToggleStyle": "switch",
	"DefaultToggleStyle": "automatic", "ButtonToggleStyle": "button", "PageTabViewStyle": "page", "DefaultTabViewStyle": "automatic",
	"CircularProgressViewStyle": "circular", "LinearProgressViewStyle": "linear", "StackNavigationViewStyle": "stack",
	"DoubleColumnNavigationViewStyle": "columns", "CompactDatePickerStyle": "compact", "GraphicalDatePickerStyle": "graphical",
	"WheelDatePickerStyle": "wheel", "IconOnlyLabelStyle": "iconOnly", "TitleOnlyLabelStyle": "titleOnly", "TitleAndIconLabelStyle": "titleAndIcon",
	"DefaultLabelStyle": "automatic", "BorderlessButtonMenuStyle": "borderless", "DefaultMenuStyle": "automatic",
}

func init() {
	for k := range styleTypeNames {
		builtinTypeNames[k] = true
	}
}
