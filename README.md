# SwiftUI Tutor — live SwiftUI previews on Windows and Linux

SwiftUI Tutor is a small playground for SwiftUI that runs in your browser: a file list on the
left, a Swift code editor in the middle and a live iPhone preview on the right. Type in the
editor and the preview updates as you go (hot reload), keeping your `@State` values where it
can. You can also open the same folder in VS Code; changes you save there show up in the
editor and the preview straight away. It's a single program with no dependencies, so there's
nothing else to install.

> **What this is, and what it isn't.** SwiftUI Tutor is a learning tool for people on Windows
> or Linux who want to learn SwiftUI before investing in a Mac. It is **not** a replacement for
> Xcode or the real SwiftUI framework, it can't build apps for a phone, and the preview is an
> approximation of how things look on a device. When you're ready to ship, use Xcode.

## Quick start (Windows)

1. Unzip the download anywhere, for example `C:\SwiftUITutor`.
2. Double-click `swiftui-tutor.exe`.
   - It creates a `MyApp` folder next to the program with a starter app, and opens SwiftUI Tutor in your browser.
   - If Windows SmartScreen warns about an unknown app, click **More info → Run anyway**. The program isn't code-signed.
3. Click `ContentView.swift` in the file list, change some code, and watch the phone update.

To open a different folder, click the folder name in the top-left corner and type or paste its
path (a new, empty folder gets a starter app). You can also start the program with a folder:

```
swiftui-tutor.exe C:\path\to\your\project
```

You can also drag a folder onto `swiftui-tutor.exe`.

Try the included examples:

```
swiftui-tutor.exe examples\FruitsDemo
swiftui-tutor.exe examples\Todos
swiftui-tutor.exe examples\Gallery
swiftui-tutor.exe examples\Recipes
swiftui-tutor.exe examples\PhotoJournal
```

## The editor

| Part | What it does |
|---|---|
| File list (left) | Every `.swift` file in the project, including subfolders. Click a file to open it. Hover over a file for **rename** (✎) and **delete** (🗑). **+** creates a new SwiftUI view file; type `Views/ProfileView` to put it in a subfolder. A red badge marks files with errors. |
| Code editor (middle) | Swift syntax highlighting, line numbers, bracket matching and auto-closing, and code completion (see below). Your work saves automatically about half a second after you stop typing; **Ctrl+S** saves straight away. **Ctrl+/** comments lines out, **Ctrl+F** searches. A line with an error turns red, with the message shown underneath it. |
| Assets tab (left) | The images and colour sets in your project. **+ Image** and **+ Colour** add new ones to `Assets.xcassets` in the format Xcode uses. Click an asset to insert `Image("name")` or `Color("name")` at the cursor. |
| Icons tab (left) | A searchable catalogue of the system icons the preview can draw. Click one to insert `Image(systemName: "name")`, or just the name when the cursor is inside a string. |
| Open in VS Code | Opens the project folder in VS Code at the file and line you're on. VS Code needs to be installed with its `code` command on the PATH (the installer's default). |
| Show folder | Opens the project folder in File Explorer. |
| Panel borders | Drag the borders between the panels to resize them. |

**Code completion.** Suggestions appear as you type and with **Ctrl+Space**. They know the
code: after `book.` you get the properties and methods of your `Book` type, after `Text("Hi").`
the view modifiers with their signatures, inside `.font(.` the fonts, inside `Image(systemName: "`
the icon names, and inside `Image("` your asset names. Choosing a method or view inserts it with
its argument labels and puts the cursor on the first one. Hover over a view or modifier name for a
short description of what it does; **Ctrl+Shift+Space** shows it for the word under the cursor.

Edits you make in VS Code (or any other editor) are picked up automatically. If you have unsaved
changes to the same file in the browser, your browser copy wins until you save it.

**Tip:** install the *Swift* extension (by the Swift Server Work Group) in VS Code for syntax highlighting there.

## Using the preview

| Control | What it does |
|---|---|
| Preview picker | **Auto** shows the `#Preview` in the file you're editing. You can also pin a specific `#Preview`, `PreviewProvider`, or your `@main App`. |
| Device | Four iPhone sizes and three iPads. iPads get a **Rotate** button; in landscape `NavigationSplitView` shows the sidebar and detail side by side, in portrait the sidebar slides in from a toolbar button, and `TabView` puts its tabs at the top like iPadOS. |
| Pop out preview | Opens the phone in its own window, so the editor and the preview can be on different screens. Both windows show the same app state. Click again to bring it back. |
| Share | Shows a QR code. Scan it with an iPhone or iPad on the same Wi-Fi and the app opens in that device's browser, scaled to the screen, following every change you make. The phone can only see and tap; it can't read or change your files. Windows may ask to allow `swiftui-tutor` through the firewall the first time. Start the program with `-local-only` to turn sharing off. |
| iOS 26 / iOS 18 style | Glass look (floating tab bar, glass buttons) or the classic look |
| Dark | Toggles dark mode. `@Environment(\.colorScheme)` follows it. |
| Reset state | Throws away all `@State` and starts the app fresh. Saved SwiftData stays. |
| Erase data | Only shown when the project has `@Model` classes. Deletes everything the app saved with SwiftData, like deleting the app from a phone. |
| Console | `print()` output, warnings and errors |

The preview is interactive. You can tap buttons, type in text fields, flip toggles, push
navigation links, open sheets, alerts and menus, swipe list rows to delete, and switch tabs.

When you make a mistake, a red box under the phone shows the error and the line it's on;
click it to jump to that line. Build errors leave the last working preview on screen, greyed
out.

## What's supported

**Swift language:** structs, classes and inheritance, enums (raw values, associated values,
`CaseIterable`), protocols with extensions, extensions on your own and built-in types,
generics, closures and trailing closures, `if`/`guard let`, `switch` with patterns and
`where`, `for`/`while`/`repeat`, labeled loops, optionals (`?.`, `??`, `!`, `if let x`),
`throws`/`try`/`do-catch`, `inout`, operator overloading, `didSet`/`willSet`, computed and
static properties, `mutating`, tuples, ranges, `stride`, and string interpolation with
`specifier:` and `format:`.

**Standard library:** String, Character, Int, Double, Bool, Array, Dictionary, Set, Optional,
plus the common methods (`map`, `filter`, `reduce`, `sorted`, `firstIndex`, `split`,
`replacingOccurrences`, `formatted()`, and so on). Also `UUID`, `Date`, `Calendar`,
`DateFormatter`, `Timer`, `DispatchQueue`, `Task { }` and `Task.sleep`, `async`/`await`,
`String(format:)`, `lazy var`, generics with inferred return types, and `if`/`switch`
expressions.

**Codable and saving:** `JSONEncoder`/`JSONDecoder` with the synthesized `Codable`
conformance, `CodingKeys`, `keyEncodingStrategy`, `dateDecodingStrategy` (`.iso8601`,
`.secondsSince1970`, `.formatted(formatter)`) and `outputFormatting`. `UserDefaults.standard`
and `@AppStorage` share one store that is saved in the project's `.swiftdata` folder, so
values survive a restart. `Bundle.main.url(forResource:withExtension:)` and
`Data(contentsOf:)` read files from your project folder, so the usual
`Bundle.main.decode("astronauts.json")` extension works; just put the JSON file in the
project folder. Custom `init(from:)` / `encode(to:)` are not supported.

**Views:** Text (including Markdown and concatenation), Image (system symbols and your own
images), Label, VStack/HStack/ZStack, Lazy stacks, LazyVGrid/LazyHGrid, Grid, Spacer,
Divider, ScrollView, ScrollViewReader, List, Form, Section, ForEach (including `$bindings`),
Group, NavigationStack, NavigationView, NavigationSplitView, NavigationLink (destination- and
value-based), `navigationDestination`, TabView (normal and `.page`), `Tab`, Button (all
built-in styles plus your own `ButtonStyle`), Toggle, TextField, SecureField, TextEditor,
Slider, Stepper, Picker (menu, segmented, inline, wheel), DatePicker, ColorPicker,
ProgressView, Gauge, Link, Menu, ShareLink, GroupBox, DisclosureGroup, LabeledContent,
ContentUnavailableView, GeometryReader, AsyncImage, ViewThatFits, TimelineView, the shapes
(Rectangle, RoundedRectangle, Circle, Capsule, Ellipse, UnevenRoundedRectangle) with
`fill`/`stroke`/`trim`, gradients, materials, and Color.

**State and data flow:** `@State`, `@Binding`, `@StateObject`, `@ObservedObject`,
`@EnvironmentObject`, `@Published`, `@Observable` with `@Bindable`, `@Environment` (dismiss,
colorScheme and others), `@FocusState`, `@AppStorage`, `@Previewable`, `.constant()` bindings,
and `Binding(get:set:)`.

**Modifiers:** about 250 are recognised. Most are drawn; accessibility, keyboard and similar
ones are accepted and have no visual effect. They include layout (padding, frame, offset,
position, aspectRatio, fixedSize, ignoresSafeArea, safeAreaInset), styling, `.sheet`,
`.fullScreenCover`, `.alert`, `.confirmationDialog`, `.toolbar`, `.searchable`,
`.swipeActions`, `.contextMenu`, `.onAppear`, `.task`, `.onChange`, `.onReceive`,
`.onTapGesture`, `withAnimation` and `.transition`, custom `ViewModifier`s and `View`
extensions.

## SwiftData

`@Model`, `@Query`, `ModelContainer`, `ModelContext` and `#Predicate` work like on a phone, and
the data is saved in a real SQLite database. Xcode's SwiftData project template runs unchanged.

- **Where the data lives:** in `.swiftdata/default.store` inside your project folder (or
  `<name>.store` for `ModelConfiguration("name")`). It's a normal SQLite file with one table per
  `@Model` class and one column per property, so you can open it with a tool like
  *DB Browser for SQLite*. Structs, enums and arrays are stored as JSON text; `Data` (photos) as
  BLOBs.
- **Saving:** changes are saved automatically after every tap, like SwiftData's autosave. `try
  modelContext.save()` also works, and `isAutosaveEnabled: false` turns autosave off.
- **Previews:** `.modelContainer(for: Item.self, inMemory: true)` and
  `ModelConfiguration(isStoredInMemoryOnly: true)` keep the data in memory, so each hot reload
  starts the preview with fresh sample data, as in Xcode. Pick your `App` in the preview picker
  to use the saved data.
- **Hot reload:** adding a property to a `@Model` adds a column to the table (the property's
  default value fills existing rows). `@Attribute(originalName:)` renames a column.
- **Supported:** `@Model`, `@Query(filter:sort:order:)`, `@Query(sort: [SortDescriptor])`,
  `Query(...)` built in `init` (`_items = Query(...)`), `@Environment(\.modelContext)`,
  `.modelContainer(for:inMemory:isAutosaveEnabled:)`, `.modelContainer(container)`,
  `ModelContainer(for:configurations:)`, `Schema`, `ModelConfiguration`, `container.mainContext`,
  `insert`, `delete`, `delete(model:where:)`, `save`, `fetch`, `fetchCount`, `FetchDescriptor`
  (`predicate`, `sortBy`, `fetchLimit`, `fetchOffset`), `SortDescriptor`, `#Predicate`,
  `@Attribute(.unique)`, `@Attribute(.externalStorage)`, `@Transient`,
  `@Relationship(deleteRule: .cascade, inverse:)` with both ends kept in sync, `@Bindable` model
  editing, `hasChanges` and `persistentModelID`.
- **Not supported:** CloudKit sync, undo, `rollback()`, versioned migration plans
  (`VersionedSchema`) and history tracking.

## Photos and camera

The photo and camera APIs work with your computer instead of a phone's photo library and camera:

- **`PhotosPicker`** (and `.photosPicker(isPresented:selection:)`) opens your browser's file
  picker. Single and multiple selection, `matching:` filters and `maxSelectionCount:` work.
  `loadTransferable(type: Data.self)` and `loadTransferable(type: Image.self)` return the picked
  photo.
- **Camera:** the usual `UIViewControllerRepresentable` wrapper around `UIImagePickerController`
  with `sourceType = .camera` shows an iPhone-style camera screen using your webcam (the browser
  asks for permission the first time). The shutter, Retake and Use Photo buttons work, and the
  photo arrives in your delegate's `imagePickerController(_:didFinishPickingMediaWithInfo:)`.
  Without a webcam you can choose a photo file instead. `sourceType = .photoLibrary` and the
  older `PHPickerViewController` wrapper use the file picker.
- **Images:** `UIImage(data:)`, `Image(uiImage:)`, `jpegData(compressionQuality:)`,
  `pngData()` and `size` work, so photos can be stored in SwiftData as `Data`. Large photos are
  scaled to at most 2048 pixels on the long side when picked.

The `PhotoJournal` example puts all of this together.

## Using it with Claude Code (MCP)

SwiftUI Tutor can act as an MCP server, so Claude Code (or any assistant that speaks MCP) can
work on the open project: read and write the Swift files, see build and runtime errors with
line numbers, look at an outline of the rendered screen, tap buttons and type into fields to
check its work, switch previews and devices, and look up which views and modifiers the
preview supports. Everything it does shows up live in your editor and preview.

Register it once (use your own path):

```
claude mcp add swiftui-tutor -- "C:\Users\you\Documents\OtherProjects\SwiftUITutor\swiftui-tutor.exe" -mcp
```

Or add it to a project's `.mcp.json`:

```json
{
  "mcpServers": {
    "swiftui-tutor": {
      "command": "C:\\Users\\you\\Documents\\OtherProjects\\SwiftUITutor\\swiftui-tutor.exe",
      "args": ["-mcp"]
    }
  }
}
```

When SwiftUI Tutor is already open, the MCP server talks to that window. When it isn't, it
starts the preview itself for the folder you pass after `-mcp` (or `MyApp`), so Claude can
still see the app. A typical session: *"Add a settings screen with a dark-mode toggle and
check it works"* — Claude writes the file, reads the errors, taps the toggle, and fixes what
it finds.

Tools: `project_info`, `read_swift_file`, `write_swift_file`, `delete_swift_file`,
`preview_status`, `select_preview`, `set_device`, `tap`, `type_text`, `reset_state`,
`list_assets`, `search_api`.

## Compatibility: what to expect

- **SwiftUI Tutor → Xcode:** if your code is valid SwiftUI, it builds in Xcode unchanged.
  SwiftUI Tutor only reads standard Swift syntax and doesn't add any language of its own.
- **SwiftUI Tutor is not a Swift compiler.** It runs your code directly and doesn't do Swift's
  full type checking. It does catch many common mistakes with compiler-style messages: wrong
  argument labels, wrong argument order, assigning to a `let`, changing a property without
  `@State` ("'self' is immutable"), passing a value where a `$binding` is needed, `ForEach`
  over non-`Identifiable` data, `Text(someInt)`, and unknown modifiers. Some type errors that
  Xcode reports, such as adding an `Int` to a `Double` variable, will still run here. So treat
  Xcode as the final judge, and expect the occasional fix-up when you move code over.
- **Looks:** layout follows SwiftUI's rules (stacks, spacers, frames, alignment, safe areas)
  and the iOS look is closely imitated, but it isn't pixel-identical. The glass effects are a
  frosted-blur approximation, and text uses your system font (Segoe UI on Windows).
- **Icons:** `Image(systemName:)` draws original look-alike icons for about 400 common symbol
  names plus about 300 aliases, and every `.fill` / `.circle` / `.circle.fill` variant of them.
  Unknown names show a placeholder icon, but the name still works in Xcode.
- **Not supported:** Core Data, networking (`URLSession`), UIKit views other than the photo
  and camera pickers, maps (shown as a placeholder), charts, video players, `Canvas` and custom
  `Path` drawing. Use Xcode for these.

## Images and colours from Assets.xcassets

Put `.png`/`.jpg` files anywhere in your project folder (or in an `Assets.xcassets` image
set) and use `Image("name")` without the extension. Colour sets in `Assets.xcassets` work
too: `Color("Brand")` uses the set's light and dark colours, and an `AccentColor` set tints
buttons and toggles like it does on a phone.

## Dropping in an Xcode project

Point SwiftUI Tutor at the folder that holds your `.swift` files (the one next to the
`.xcodeproj`). Unit-test targets (`import XCTest` / `import Testing`) and `Package.swift`
are skipped automatically; everything else is loaded, including subfolders.

## Building from source

The Go source is in the `source` folder. You need Go 1.24 or newer:

```
cd source
go build -o swiftui-tutor.exe .
```

For other platforms, use `GOOS=darwin` or `GOOS=linux`.

## License and disclaimer

MIT licence; see `LICENSE`. The program also includes CodeMirror 5, SQLite and a few Go
libraries under their own open-source licences; see `THIRD_PARTY_NOTICES.md`.

SwiftUI Tutor is an independent, unofficial learning project. It is not affiliated with,
endorsed by, or sponsored by the makers of Swift, SwiftUI, iOS, iPhone or Xcode, and all
product names belong to their respective owners. No proprietary code, fonts or icon artwork
are included; every icon is an original drawing.
