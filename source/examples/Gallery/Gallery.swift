import SwiftUI

struct Movie: Identifiable {
    let id = UUID()
    let title: String
    let year: Int
    let rating: Double
    let color: Color
}

let sampleMovies = [
    Movie(title: "Inception", year: 2010, rating: 8.8, color: .indigo),
    Movie(title: "Up", year: 2009, rating: 8.3, color: .orange),
    Movie(title: "Arrival", year: 2016, rating: 7.9, color: .teal),
]

struct RatingView: View {
    @Binding var rating: Int
    var maxRating = 5
    var body: some View {
        HStack(spacing: 4) {
            ForEach(1...maxRating, id: \.self) { n in
                Image(systemName: n <= rating ? "star.fill" : "star")
                    .foregroundStyle(n <= rating ? .yellow : .gray)
                    .onTapGesture { rating = n }
            }
        }
        .font(.title2)
    }
}

struct MoviesView: View {
    @State private var selected: Movie?
    @State private var rating = 3
    @State private var showDetails = false
    @State private var sort = "Title"

    var body: some View {
        ZStack {
            LinearGradient(colors: [.purple.opacity(0.3), .blue.opacity(0.2)], startPoint: .top, endPoint: .bottom)
                .ignoresSafeArea()
            VStack(alignment: .leading, spacing: 16) {
                HStack {
                    Text("Movies").font(.largeTitle.bold())
                    Spacer()
                    Menu {
                        Button("Title") { sort = "Title" }
                        Button("Year") { sort = "Year" }
                        Divider()
                        Button("Reset", role: .destructive) { sort = "Title" }
                    } label: {
                        Label("Sort", systemImage: "arrow.up.arrow.down")
                    }
                }
                Text("Sorted by ") + Text(sort).bold().foregroundColor(.purple)
                ScrollView(.horizontal, showsIndicators: false) {
                    HStack(spacing: 12) {
                        ForEach(sampleMovies) { movie in
                            VStack(alignment: .leading) {
                                RoundedRectangle(cornerRadius: 12)
                                    .fill(movie.color.gradient)
                                    .frame(width: 140, height: 180)
                                    .overlay(alignment: .bottomLeading) {
                                        Text(String(movie.year))
                                            .font(.caption.bold())
                                            .padding(6)
                                            .background(.ultraThinMaterial, in: Capsule())
                                            .padding(8)
                                    }
                                Text(movie.title).font(.headline)
                                Text("⭐️ \(movie.rating, specifier: "%.1f")")
                                    .font(.subheadline)
                                    .foregroundStyle(.secondary)
                            }
                            .onTapGesture { selected = movie }
                            .contextMenu {
                                Button("Share", systemImage: "square.and.arrow.up") { }
                            }
                        }
                    }
                }
                RatingView(rating: $rating)
                Text("Your rating: \(rating)/5")
                Button(showDetails ? "Hide details" : "Show details") {
                    withAnimation(.spring) { showDetails.toggle() }
                }
                if showDetails {
                    GroupBox("Details") {
                        Text("Some extra details appear with a transition.")
                    }
                    .transition(.opacity)
                }
                Spacer()
            }
            .padding()
        }
        .sheet(item: $selected) { movie in
            VStack(spacing: 12) {
                Text(movie.title).font(.title.bold())
                Text("Released in \(movie.year)")
                Gauge(value: movie.rating, in: 0...10) {
                    Text("Rating")
                }
                .padding(.horizontal, 40)
            }
            .presentationDetents([.medium])
        }
    }
}

struct OnboardingView: View {
    @State private var page = 0
    let pages = [("sparkles", "Welcome", "Learn SwiftUI on any computer."),
                 ("bolt.fill", "Fast", "Edits show up instantly."),
                 ("checkmark.seal.fill", "Compatible", "Paste into Xcode when ready.")]
    var body: some View {
        VStack {
            TabView(selection: $page) {
                ForEach(0..<pages.count, id: \.self) { i in
                    VStack(spacing: 16) {
                        Image(systemName: pages[i].0)
                            .font(.system(size: 72))
                            .foregroundStyle(.tint)
                        Text(pages[i].1).font(.title.bold())
                        Text(pages[i].2).foregroundStyle(.secondary)
                    }
                    .tag(i)
                }
            }
            .tabViewStyle(.page)
            Button(page == pages.count - 1 ? "Get Started" : "Next") {
                withAnimation { page = min(page + 1, pages.count - 1) }
            }
            .buttonStyle(.borderedProminent)
            .controlSize(.large)
            .padding(.bottom)
        }
    }
}

struct ControlsView: View {
    @State private var notes = "Hello"
    @State private var password = ""
    @State private var expanded = true
    @State private var color = Color.blue
    @State private var progress = 0.3
    @State private var level = 2
    var body: some View {
        Form {
            DisclosureGroup("Advanced", isExpanded: $expanded) {
                Toggle("Wi-Fi", isOn: .constant(true)).tint(.purple)
                SecureField("Password", text: $password)
            }
            Section("Notes") {
                TextEditor(text: $notes).frame(height: 80)
            }
            Section {
                ColorPicker("Accent", selection: $color)
                ProgressView(value: progress)
                Stepper("Level \(level)", onIncrement: { level += 1 }, onDecrement: { level -= 1 })
                Grid(alignment: .leading) {
                    GridRow { Text("Name"); Text("Kevin").bold() }
                    GridRow { Text("City"); Text("Cape Town").bold() }
                }
            }
        }
        .onChange(of: level) { oldValue, newValue in
            print("level changed from \(oldValue) to \(newValue)")
            progress = Double(newValue) / 10
        }
    }
}

#Preview("Movies") { MoviesView() }
#Preview("Onboarding") { OnboardingView() }
#Preview("Controls") { ControlsView() }
