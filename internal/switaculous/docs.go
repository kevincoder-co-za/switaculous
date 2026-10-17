package switaculous

import "strings"

type apiDoc struct {
	Signature string `json:"sig"`
	Summary   string `json:"doc"`
}

// Signatures are written the way Xcode's completion shows them; the part after
// " — " is the one-line description.
var modifierDocs = parseDocLines(`
padding(_ edges: Edge.Set = .all, _ length: CGFloat? = nil) — Adds space around the view; padding() uses the system amount
padding(_ length: CGFloat) — Adds the same space on all four sides
frame(width: CGFloat? = nil, height: CGFloat? = nil, alignment: Alignment = .center) — Fixes the view's size
frame(maxWidth: CGFloat? = nil, maxHeight: CGFloat? = nil, alignment: Alignment = .center) — Lets the view grow up to a size; .infinity fills the parent
font(_ font: Font?) — Sets the text font, for example .title or .system(size: 20)
fontWeight(_ weight: Font.Weight?) — Sets the weight, for example .bold or .semibold
fontDesign(_ design: Font.Design?) — Uses .rounded, .serif or .monospaced lettering
bold(_ isActive: Bool = true) — Makes text bold
italic(_ isActive: Bool = true) — Makes text italic
underline(_ isActive: Bool = true, color: Color? = nil) — Underlines text
strikethrough(_ isActive: Bool = true, color: Color? = nil) — Draws a line through text
foregroundStyle(_ style: ShapeStyle) — Colours text and symbols, for example .secondary or .red
foregroundColor(_ color: Color?) — Older name for foregroundStyle
background(_ style: ShapeStyle, in shape: Shape? = nil) — Paints a colour or gradient behind the view
background(alignment: Alignment = .center, content: () -> View) — Places another view behind this one
overlay(alignment: Alignment = .center, content: () -> View) — Places another view on top of this one
tint(_ tint: Color?) — Sets the accent colour used by buttons and controls
opacity(_ opacity: Double) — Makes the view translucent (0 hidden, 1 fully visible)
cornerRadius(_ radius: CGFloat) — Rounds the corners (older than clipShape)
clipShape(_ shape: Shape) — Clips the view to a shape, for example .rect(cornerRadius: 12) or Circle()
clipped() — Cuts off anything drawn outside the view's frame
shadow(color: Color = .black.opacity(0.33), radius: CGFloat, x: CGFloat = 0, y: CGFloat = 0) — Adds a drop shadow
border(_ style: ShapeStyle, width: CGFloat = 1) — Draws a border around the view's frame
offset(x: CGFloat = 0, y: CGFloat = 0) — Moves the view without affecting layout
position(x: CGFloat, y: CGFloat) — Places the view's centre at a point in its parent
rotationEffect(_ angle: Angle, anchor: UnitPoint = .center) — Rotates the view
scaleEffect(_ scale: CGFloat, anchor: UnitPoint = .center) — Scales the view
blur(radius: CGFloat) — Blurs the view
fixedSize(horizontal: Bool = true, vertical: Bool = true) — Stops the view from being squashed below its ideal size
layoutPriority(_ value: Double) — Gives the view priority when space is short
aspectRatio(_ aspectRatio: CGFloat? = nil, contentMode: ContentMode) — Keeps a width-to-height ratio
scaledToFit() — Scales to fit inside the available space
scaledToFill() — Scales to fill the available space, cropping if needed
resizable() — Lets an Image be resized
imageScale(_ scale: Image.Scale) — Makes a symbol image .small, .medium or .large
symbolRenderingMode(_ mode: SymbolRenderingMode?) — Chooses .monochrome, .hierarchical, .palette or .multicolor symbols
lineLimit(_ number: Int?) — Limits text to a number of lines
multilineTextAlignment(_ alignment: TextAlignment) — Aligns wrapped text .leading, .center or .trailing
minimumScaleFactor(_ factor: CGFloat) — Lets text shrink to fit
truncationMode(_ mode: Text.TruncationMode) — Where to cut long text: .head, .middle or .tail
textCase(_ textCase: Text.Case?) — Shows text as .uppercase or .lowercase
kerning(_ kerning: CGFloat) — Adjusts the space between letters
navigationTitle(_ title: String) — Sets the title shown in the navigation bar
navigationBarTitleDisplayMode(_ displayMode: NavigationBarItem.TitleDisplayMode) — .large, .inline or .automatic title
navigationBarBackButtonHidden(_ hidesBackButton: Bool = true) — Hides the back button
navigationDestination(for data: Data.Type, destination: (Data) -> View) — The view to push for NavigationLink(value:) of this type
navigationDestination(isPresented: Binding<Bool>, destination: () -> View) — Pushes a view when the binding is true
toolbar(content: () -> ToolbarContent) — Adds items to the navigation bar; use ToolbarItem(placement:)
toolbarBackground(_ style: ShapeStyle, for bars: ToolbarPlacement...) — Colours the navigation or tab bar
toolbarColorScheme(_ colorScheme: ColorScheme?, for bars: ToolbarPlacement...) — Light or dark bar appearance
searchable(text: Binding<String>, prompt: String? = nil) — Adds a search field to a navigation view
refreshable(action: () async -> Void) — Adds pull to refresh to a List or ScrollView
tabItem(_ label: () -> View) — The label of this view's tab in a TabView
tag(_ tag: Hashable) — Identifies this view for a Picker or TabView selection
badge(_ count: Int) — Shows a number on a tab item or list row
listStyle(_ style: ListStyle) — .plain, .inset, .grouped, .insetGrouped or .sidebar
listRowBackground(_ view: View?) — Background of this list row
listRowSeparator(_ visibility: Visibility, edges: VerticalEdge.Set = .all) — Hides or shows the row's separator
listRowInsets(_ insets: EdgeInsets?) — Changes the padding inside the row
swipeActions(edge: HorizontalEdge = .trailing, allowsFullSwipe: Bool = true, content: () -> View) — Buttons revealed by swiping the row
onDelete(perform action: ((IndexSet) -> Void)?) — Lets rows of a ForEach be deleted by swiping
onMove(perform action: ((IndexSet, Int) -> Void)?) — Lets rows be reordered in edit mode
scrollContentBackground(_ visibility: Visibility) — .hidden removes the default List or Form background
scrollIndicators(_ visibility: ScrollIndicatorVisibility) — Shows or hides scroll bars
scrollDismissesKeyboard(_ mode: ScrollDismissesKeyboardMode) — Dismisses the keyboard while scrolling
scrollTargetBehavior(_ behavior: ScrollTargetBehavior) — .paging or .viewAligned snapping
contentMargins(_ edges: Edge.Set = .all, _ length: CGFloat?) — Insets scrollable content
safeAreaInset(edge: VerticalEdge, content: () -> View) — Pins a view to an edge and shrinks the safe area
ignoresSafeArea(_ regions: SafeAreaRegions = .all, edges: Edge.Set = .all) — Extends the view under the status bar or home indicator
buttonStyle(_ style: ButtonStyle) — .bordered, .borderedProminent, .borderless, .plain or your own
buttonBorderShape(_ shape: ButtonBorderShape) — .capsule, .roundedRectangle or .circle for bordered buttons
controlSize(_ controlSize: ControlSize) — .mini, .small, .regular, .large or .extraLarge
labelStyle(_ style: LabelStyle) — .iconOnly, .titleOnly or .titleAndIcon
textFieldStyle(_ style: TextFieldStyle) — .roundedBorder or .plain
pickerStyle(_ style: PickerStyle) — .menu, .segmented, .wheel, .inline or .navigationLink
toggleStyle(_ style: ToggleStyle) — .switch or .button
datePickerStyle(_ style: DatePickerStyle) — .compact, .graphical or .wheel
progressViewStyle(_ style: ProgressViewStyle) — .linear or .circular
gaugeStyle(_ style: GaugeStyle) — .linearCapacity, .accessoryCircular and others
tabViewStyle(_ style: TabViewStyle) — .page for swipeable pages
disabled(_ disabled: Bool) — Greys out the view and ignores taps
hidden() — Hides the view but keeps its space
keyboardType(_ type: UIKeyboardType) — .numberPad, .decimalPad, .emailAddress and others
textContentType(_ textContentType: UITextContentType?) — Hints the keyboard, for example .emailAddress
textInputAutocapitalization(_ autocapitalization: TextInputAutocapitalization?) — .never, .words or .sentences
autocorrectionDisabled(_ disable: Bool = true) — Turns off autocorrect
submitLabel(_ submitLabel: SubmitLabel) — The keyboard's return key: .done, .search, .go, .send
onSubmit(action: () -> Void) — Runs when the user presses return in a text field
focused(_ binding: FocusState<Bool>.Binding) — Ties the keyboard focus to a @FocusState
onTapGesture(count: Int = 1, perform action: () -> Void) — Runs an action when tapped
onLongPressGesture(minimumDuration: Double = 0.5, perform action: () -> Void) — Runs an action after a long press
gesture(_ gesture: Gesture) — Attaches a DragGesture, MagnifyGesture or other gesture
simultaneousGesture(_ gesture: Gesture) — A gesture that runs alongside the view's own
onAppear(perform action: (() -> Void)? = nil) — Runs when the view is shown
onDisappear(perform action: (() -> Void)? = nil) — Runs when the view is removed
task(priority: TaskPriority = .userInitiated, _ action: () async -> Void) — Runs async work when the view appears and cancels it when it disappears
onChange(of value: Equatable, initial: Bool = false, _ action: (oldValue, newValue) -> Void) — Runs when a value changes
onReceive(_ publisher: Publisher, perform action: (Output) -> Void) — Runs when a timer or publisher fires
animation(_ animation: Animation?, value: Equatable) — Animates changes whenever the value changes
transition(_ t: AnyTransition) — How the view appears and disappears, for example .slide or .scale
contentTransition(_ transition: ContentTransition) — Animates text changes with .numericText()
matchedGeometryEffect(id: Hashable, in namespace: Namespace.ID) — Animates a view between two places
sheet(isPresented: Binding<Bool>, onDismiss: (() -> Void)? = nil, content: () -> View) — Presents a sheet when the binding is true
sheet(item: Binding<Item?>, content: (Item) -> View) — Presents a sheet for a non-nil item
fullScreenCover(isPresented: Binding<Bool>, content: () -> View) — Presents a full-screen view
popover(isPresented: Binding<Bool>, content: () -> View) — Presents a popover (a sheet on iPhone)
alert(_ title: String, isPresented: Binding<Bool>, actions: () -> View, message: () -> View) — Shows an alert with buttons
confirmationDialog(_ title: String, isPresented: Binding<Bool>, titleVisibility: Visibility = .automatic, actions: () -> View) — Shows an action sheet from the bottom
presentationDetents(_ detents: Set<PresentationDetent>) — Sheet heights: [.medium, .large, .height(300)]
presentationDragIndicator(_ visibility: Visibility) — Shows the grab handle on a sheet
interactiveDismissDisabled(_ isDisabled: Bool = true) — Stops a sheet from being swiped away
contextMenu(menuItems: () -> View) — Menu shown on a long press
environment(_ object: Observable) — Shares an @Observable object with child views
environment(_ keyPath: WritableKeyPath<EnvironmentValues, V>, _ value: V) — Sets an environment value, for example \.colorScheme
environmentObject(_ object: ObservableObject) — Shares an ObservableObject with child views
modelContainer(for modelType: PersistentModel.Type, inMemory: Bool = false) — Gives the view tree a SwiftData store
modelContainer(_ container: ModelContainer) — Uses an existing SwiftData container
preferredColorScheme(_ colorScheme: ColorScheme?) — Forces .light or .dark mode
accentColor(_ accentColor: Color?) — Older name for tint
id(_ id: Hashable) — Gives the view an identity; changing it recreates the view
redacted(reason: RedactionReasons) — Shows grey placeholders with .placeholder
help(_ text: String) — A tooltip (not shown on iPhone)
accessibilityLabel(_ label: String) — Text read by VoiceOver
accessibilityHidden(_ hidden: Bool) — Hides the view from VoiceOver
photosPicker(isPresented: Binding<Bool>, selection: Binding<PhotosPickerItem?>, matching: PHPickerFilter? = nil) — Presents the photo library picker
labelsHidden() — Hides the label of a Toggle, Picker or DatePicker
containerRelativeFrame(_ axes: Axis.Set, alignment: Alignment = .center) — Sizes the view relative to its container
glassEffect(_ glass: Glass = .regular, in shape: Shape = Capsule()) — Liquid Glass background (iOS 26)
mask(alignment: Alignment = .center, _ mask: () -> View) — Shows the view only where the mask is opaque
zIndex(_ value: Double) — Which overlapping view is on top
`)

var viewDocs = parseDocLines(`
Text(_ content: String) — A line or paragraph of text; supports Markdown like **bold**
Image(systemName: String) — A system symbol icon, for example "star.fill"
Image(_ name: String) — An image from your assets
Image(uiImage: UIImage) — A photo loaded from Data
Label(_ title: String, systemImage: String) — An icon next to a title
VStack(alignment: HorizontalAlignment = .center, spacing: CGFloat? = nil, content: () -> View) — Stacks views vertically
HStack(alignment: VerticalAlignment = .center, spacing: CGFloat? = nil, content: () -> View) — Lines views up horizontally
ZStack(alignment: Alignment = .center, content: () -> View) — Layers views on top of each other
LazyVStack(alignment: HorizontalAlignment = .center, spacing: CGFloat? = nil, content: () -> View) — A vertical stack that creates rows as they scroll into view
LazyHStack(alignment: VerticalAlignment = .center, spacing: CGFloat? = nil, content: () -> View) — A horizontal stack that creates items lazily
LazyVGrid(columns: [GridItem], alignment: HorizontalAlignment = .center, spacing: CGFloat? = nil, content: () -> View) — A grid that scrolls vertically
LazyHGrid(rows: [GridItem], spacing: CGFloat? = nil, content: () -> View) — A grid that scrolls horizontally
Grid(alignment: Alignment = .center, horizontalSpacing: CGFloat? = nil, verticalSpacing: CGFloat? = nil, content: () -> View) — A table-like grid of GridRow views
GridRow(content: () -> View) — One row inside a Grid
GridItem(_ size: GridItem.Size = .flexible(), spacing: CGFloat? = nil) — A column definition: .fixed(80), .flexible() or .adaptive(minimum: 100)
Spacer(minLength: CGFloat? = nil) — Flexible empty space that pushes views apart
Divider() — A thin separator line
ScrollView(_ axes: Axis.Set = .vertical, showsIndicators: Bool = true, content: () -> View) — Makes its content scrollable
ScrollViewReader(content: (ScrollViewProxy) -> View) — Lets you call proxy.scrollTo(id)
List(content: () -> View) — A scrolling list with system row styling
List(_ data: [Identifiable], content: (Element) -> View) — A list built from a collection
Form(content: () -> View) — A grouped list for settings and data entry
Section(_ header: String, content: () -> View) — Groups rows in a List or Form
ForEach(_ data: [Identifiable], content: (Element) -> View) — Creates a view for every element
ForEach(_ data: Range<Int>, id: KeyPath, content: (Element) -> View) — Creates views for each value in a range
Group(content: () -> View) — Groups views without affecting layout
NavigationStack(content: () -> View) — A navigation bar with push and pop
NavigationStack(path: Binding<NavigationPath>, content: () -> View) — A navigation stack you can control from code
NavigationSplitView(sidebar: () -> View, detail: () -> View) — Sidebar and detail columns (a stack on iPhone)
NavigationLink(_ title: String, destination: () -> View) — A row that pushes a view when tapped
NavigationLink(value: Hashable, label: () -> View) — Pushes the view from navigationDestination(for:)
TabView(selection: Binding<Hashable>? = nil, content: () -> View) — A tab bar switching between views
Tab(_ title: String, systemImage: String, content: () -> View) — One tab in a TabView (iOS 18)
Button(_ title: String, action: () -> Void) — A tappable button with a text label
Button(_ title: String, systemImage: String, action: () -> Void) — A button with an icon and a title
Button(role: ButtonRole?, action: () -> Void, label: () -> View) — A button with a custom label; role .destructive shows red
Toggle(_ title: String, isOn: Binding<Bool>) — An on/off switch
TextField(_ prompt: String, text: Binding<String>) — A single-line text input
TextField(_ prompt: String, value: Binding<Value>, format: FormatStyle) — A text field for numbers or dates
SecureField(_ prompt: String, text: Binding<String>) — A password field
TextEditor(text: Binding<String>) — A multi-line text input
Slider(value: Binding<Double>, in bounds: ClosedRange<Double> = 0...1, step: Double? = nil) — A draggable slider
Stepper(_ title: String, value: Binding<Int>, in bounds: ClosedRange<Int>, step: Int = 1) — Plus and minus buttons
Picker(_ title: String, selection: Binding<Selection>, content: () -> View) — Choose one value from several
DatePicker(_ title: String, selection: Binding<Date>, displayedComponents: DatePickerComponents = [.date, .hourAndMinute]) — Date and time entry
ColorPicker(_ title: String, selection: Binding<Color>) — A colour swatch that opens the colour picker
ProgressView(value: Double? = nil, total: Double = 1) — A progress bar, or a spinner with no value
Gauge(value: Double, in bounds: ClosedRange<Double> = 0...1, label: () -> View) — A gauge showing a value in a range
Link(_ title: String, destination: URL) — Opens a web link
Menu(_ title: String, content: () -> View) — A button that shows a menu of actions
ShareLink(item: String) — A share button
GroupBox(_ title: String, content: () -> View) — Content in a rounded box with a title
DisclosureGroup(_ title: String, isExpanded: Binding<Bool>, content: () -> View) — Collapsible content
LabeledContent(_ title: String, value: String) — A label on the left and a value on the right
ContentUnavailableView(_ title: String, systemImage: String, description: Text? = nil) — The standard empty-state screen
GeometryReader(content: (GeometryProxy) -> View) — Reads the size you have been given
AsyncImage(url: URL?) — Loads an image from a URL
TimelineView(_ schedule: TimelineSchedule, content: (Context) -> View) — Redraws on a schedule, for clocks
Rectangle() — A rectangle shape
RoundedRectangle(cornerRadius: CGFloat, style: RoundedCornerStyle = .continuous) — A rectangle with rounded corners
Circle() — A circle shape
Capsule() — A pill shape
Ellipse() — An oval shape
Color(red: Double, green: Double, blue: Double, opacity: Double = 1) — A custom colour; also Color.red, Color("Asset")
LinearGradient(colors: [Color], startPoint: UnitPoint, endPoint: UnitPoint) — A gradient between two points
RadialGradient(colors: [Color], center: UnitPoint, startRadius: CGFloat, endRadius: CGFloat) — A circular gradient
AngularGradient(colors: [Color], center: UnitPoint) — A gradient that sweeps around a centre
PhotosPicker(selection: Binding<PhotosPickerItem?>, matching: PHPickerFilter? = nil, label: () -> View) — Opens the photo library
EditButton() — Toggles edit mode in a List
ToolbarItem(placement: ToolbarItemPlacement = .automatic, content: () -> View) — One item in a toolbar
ToolbarItemGroup(placement: ToolbarItemPlacement = .automatic, content: () -> View) — Several toolbar items together
AnyView(_ view: View) — Erases a view's type
EmptyView() — A view that draws nothing
Map(position: Binding<MapCameraPosition>) — A map (shown as a placeholder in the preview)
`)

func parseDocLines(text string) map[string][]apiDoc {
	docs := map[string][]apiDoc{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		signature, summary, _ := strings.Cut(line, " — ")
		name := signature
		if i := strings.Index(signature, "("); i >= 0 {
			name = signature[:i]
		}
		docs[name] = append(docs[name], apiDoc{Signature: signature, Summary: summary})
	}
	return docs
}
