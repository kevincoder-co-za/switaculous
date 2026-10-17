import SwiftUI

struct ContentView: View {
    @State private var count = 0
    @State private var name = ""

    var body: some View {
        NavigationStack {
            VStack(spacing: 20) {
                Image(systemName: "hand.wave")
                    .font(.system(size: 60))
                    .foregroundStyle(.orange)

                Text("Hello, \(name.isEmpty ? "SwiftUI" : name)!")
                    .font(.title)
                    .bold()

                TextField("Your name", text: $name)
                    .textFieldStyle(.roundedBorder)

                Text("Tapped \(count) times")
                    .foregroundStyle(.secondary)

                Button("Tap me") {
                    count += 1
                }
                .buttonStyle(.borderedProminent)
            }
            .padding()
            .navigationTitle("Welcome")
        }
    }
}

#Preview {
    ContentView()
}
