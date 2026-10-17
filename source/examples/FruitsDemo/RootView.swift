import SwiftUI

struct RootView: View {
    @State private var selection = 0
    var body: some View {
        TabView(selection: $selection) {
            HomeView()
                .tabItem { Label("Home", systemImage: "house") }
                .tag(0)
            FormDemo()
                .tabItem { Label("Settings", systemImage: "gear") }
                .tag(1)
            ShapesDemo()
                .tabItem { Label("Shapes", systemImage: "star") }
                .tag(2)
        }
    }
}

struct Fruit: Identifiable, Hashable {
    let id = UUID()
    let name: String
    let emoji: String
    var calories: Int
}

struct HomeView: View {
    @EnvironmentObject var settings: AppSettings
    @State private var fruits = [
        Fruit(name: "Mango", emoji: "🥭", calories: 99),
        Fruit(name: "Banana", emoji: "🍌", calories: 105),
        Fruit(name: "Cherry", emoji: "🍒", calories: 50)
    ]
    @State private var showSheet = false
    @State private var showAlert = false

    var body: some View {
        NavigationStack {
            List {
                Section {
                    ForEach(fruits) { fruit in
                        NavigationLink {
                            FruitDetail(fruit: fruit)
                        } label: {
                            HStack {
                                Text(fruit.emoji)
                                Text(fruit.name)
                                Spacer()
                                Text("\(fruit.calories) kcal")
                                    .foregroundStyle(.secondary)
                            }
                        }
                    }
                    .onDelete { offsets in
                        fruits.remove(atOffsets: offsets)
                    }
                } header: {
                    Text("Hello, \(settings.username)")
                } footer: {
                    Text("Swipe left to delete a fruit.")
                }
                Section("Actions") {
                    Button("Show sheet") { showSheet = true }
                    Button("Show alert") { showAlert = true }
                    Button("Delete all", role: .destructive) { fruits.removeAll() }
                }
            }
            .navigationTitle("Fruits")
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        fruits.append(Fruit(name: "Kiwi", emoji: "🥝", calories: 42))
                    } label: {
                        Image(systemName: "plus")
                    }
                }
            }
            .sheet(isPresented: $showSheet) {
                SheetView()
            }
            .alert("Are you sure?", isPresented: $showAlert) {
                Button("Cancel", role: .cancel) { }
                Button("Delete", role: .destructive) { fruits.removeAll() }
            } message: {
                Text("This will remove every fruit.")
            }
        }
    }
}

struct FruitDetail: View {
    let fruit: Fruit
    @State private var liked = false
    var body: some View {
        VStack(spacing: 16) {
            Text(fruit.emoji).font(.system(size: 96))
            Text(fruit.name).font(.largeTitle).bold()
            Button {
                liked.toggle()
            } label: {
                Label(liked ? "Liked" : "Like", systemImage: liked ? "heart.fill" : "heart")
            }
            .buttonStyle(.borderedProminent)
            .tint(liked ? .pink : .blue)
        }
        .navigationTitle(fruit.name)
        .navigationBarTitleDisplayMode(.inline)
    }
}

struct SheetView: View {
    @Environment(\.dismiss) private var dismiss
    var body: some View {
        NavigationStack {
            VStack(spacing: 12) {
                Image(systemName: "checkmark.circle.fill")
                    .font(.system(size: 64))
                    .foregroundStyle(.green)
                Text("This is a sheet")
                    .font(.title2)
            }
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done") { dismiss() }
                }
            }
            .navigationTitle("Sheet")
            .navigationBarTitleDisplayMode(.inline)
        }
    }
}
