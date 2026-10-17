import SwiftUI
import Observation

extension Color {
    init(hex: String) {
        let hex = hex.trimmingCharacters(in: CharacterSet.alphanumerics.inverted)
        var int: UInt64 = 0
        Scanner(string: hex).scanHexInt64(&int)
        let r = Double((int >> 16) & 0xFF) / 255
        let g = Double((int >> 8) & 0xFF) / 255
        let b = Double(int & 0xFF) / 255
        self.init(.sRGB, red: r, green: g, blue: b, opacity: 1)
    }
    static let brand = Color(hex: "#FF6B35")
}

struct CardModifier: ViewModifier {
    func body(content: Content) -> some View {
        content
            .padding()
            .background(.background)
            .clipShape(RoundedRectangle(cornerRadius: 16))
            .shadow(color: .black.opacity(0.1), radius: 8, y: 4)
    }
}

extension View {
    func card() -> some View { modifier(CardModifier()) }
}

struct PillButtonStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.headline)
            .foregroundStyle(.white)
            .padding(.horizontal, 24)
            .padding(.vertical, 12)
            .background(Color.brand, in: Capsule())
            .scaleEffect(configuration.isPressed ? 0.95 : 1)
    }
}

@Observable
class TodoStore {
    var todos: [Todo] = [
        Todo(title: "Buy milk"),
        Todo(title: "Learn SwiftUI", done: true),
        Todo(title: "Walk the dog")
    ]
    var remaining: Int { todos.filter { !$0.done }.count }
}

struct Todo: Identifiable, Hashable {
    var id = UUID()
    var title: String
    var done = false
}

struct TodoApp: View {
    @State private var store = TodoStore()
    @State private var newTitle = ""
    @State private var search = ""
    @State private var path: [Todo] = []
    @FocusState private var focused: Bool
    @AppStorage("launches") private var launches = 0

    var filtered: [Todo] {
        search.isEmpty ? store.todos : store.todos.filter { $0.title.localizedCaseInsensitiveContains(search) }
    }

    var body: some View {
        NavigationStack(path: $path) {
            List {
                Section {
                    HStack {
                        TextField("New todo", text: $newTitle)
                            .focused($focused)
                            .onSubmit(add)
                        Button("Add", action: add)
                            .disabled(newTitle.isEmpty)
                    }
                }
                Section("\(store.remaining) remaining") {
                    ForEach($store.todos) { $todo in
                        HStack {
                            Image(systemName: todo.done ? "checkmark.circle.fill" : "circle")
                                .foregroundStyle(todo.done ? .green : .secondary)
                                .onTapGesture { todo.done.toggle() }
                            NavigationLink(value: todo) {
                                Text(todo.title)
                                    .strikethrough(todo.done)
                            }
                        }
                        .swipeActions {
                            Button(role: .destructive) {
                                store.todos.removeAll { $0.id == todo.id }
                            } label: {
                                Label("Delete", systemImage: "trash")
                            }
                        }
                    }
                }
            }
            .searchable(text: $search)
            .navigationTitle("Todos")
            .navigationDestination(for: Todo.self) { todo in
                TodoDetail(todo: todo)
            }
            .onAppear { launches += 1 }
            .overlay {
                if store.todos.isEmpty {
                    ContentUnavailableView("No Todos", systemImage: "checklist", description: Text("Add one above."))
                }
            }
        }
    }

    func add() {
        guard !newTitle.isEmpty else { return }
        store.todos.append(Todo(title: newTitle))
        newTitle = ""
    }
}

struct TodoDetail: View {
    let todo: Todo
    @State private var seconds = 0
    @State private var showConfirm = false
    let timer = Timer.publish(every: 1, on: .main, in: .common).autoconnect()

    var body: some View {
        VStack(spacing: 20) {
            Text(todo.title)
                .font(.title)
                .card()
            Text("Open for \(seconds)s")
                .monospacedDigit()
            Button("Celebrate") { showConfirm = true }
                .buttonStyle(PillButtonStyle())
            LazyVGrid(columns: [GridItem(.adaptive(minimum: 60))], spacing: 12) {
                ForEach(0..<8) { i in
                    RoundedRectangle(cornerRadius: 10)
                        .fill(Color.brand.opacity(Double(i + 2) / 10))
                        .frame(height: 60)
                        .overlay(Text("\(i)").foregroundStyle(.white))
                }
            }
            .padding(.horizontal)
            Spacer()
        }
        .padding(.top)
        .onReceive(timer) { _ in seconds += 1 }
        .confirmationDialog("Celebrate how?", isPresented: $showConfirm, titleVisibility: .visible) {
            Button("Confetti") { }
            Button("Fireworks") { }
        }
        .navigationTitle("Detail")
    }
}

struct ProfileView: View {
    @State private var isLoading = true
    @State private var name = ""
    var body: some View {
        Group {
            if isLoading {
                ProgressView("Loading…")
            } else {
                VStack {
                    Image(systemName: "person.crop.circle.fill")
                        .resizable()
                        .scaledToFit()
                        .frame(width: 100, height: 100)
                        .foregroundStyle(.gray)
                    Text(name).font(.title2.bold())
                }
            }
        }
        .task {
            try? await Task.sleep(nanoseconds: 300_000_000)
            name = "Taylor"
            isLoading = false
        }
    }
}

struct GeoDemo: View {
    var body: some View {
        GeometryReader { geo in
            VStack {
                Text("Width: \(Int(geo.size.width))")
                Rectangle()
                    .fill(.blue)
                    .frame(width: geo.size.width / 2, height: 40)
            }
            .frame(maxWidth: .infinity)
        }
    }
}

#Preview("Todos") { TodoApp() }
#Preview("Profile") { ProfileView() }
#Preview("Geo") { GeoDemo() }

struct TodoApp_Previews: PreviewProvider {
    static var previews: some View {
        TodoApp()
    }
}
