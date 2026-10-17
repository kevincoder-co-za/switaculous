import SwiftUI

enum Flavor: String, CaseIterable, Identifiable {
    case chocolate, vanilla, strawberry
    var id: Self { self }
}

struct FormDemo: View {
    @EnvironmentObject var settings: AppSettings
    @State private var volume = 0.5
    @State private var count = 3
    @State private var flavor: Flavor = .vanilla
    @State private var mode = 0
    @State private var date = Date()
    @State private var bio = ""

    var body: some View {
        NavigationStack {
            Form {
                Section("Profile") {
                    TextField("Username", text: $settings.username)
                    Toggle("Notifications", isOn: $settings.notifications)
                    Picker("Flavor", selection: $flavor) {
                        ForEach(Flavor.allCases) { f in
                            Text(f.rawValue.capitalized).tag(f)
                        }
                    }
                }
                Section {
                    Picker("Mode", selection: $mode) {
                        Text("Day").tag(0)
                        Text("Week").tag(1)
                        Text("Month").tag(2)
                    }
                    .pickerStyle(.segmented)
                    Stepper("Count: \(count)", value: $count, in: 0...10)
                    Slider(value: $volume)
                    DatePicker("Date", selection: $date, displayedComponents: .date)
                    LabeledContent("Volume", value: "\(Int(volume * 100))%")
                }
                Section("About") {
                    Label("Version 1.0", systemImage: "info.circle")
                    Link("Swift.org", destination: URL(string: "https://www.swift.org")!)
                }
            }
            .navigationTitle("Settings")
        }
    }
}
