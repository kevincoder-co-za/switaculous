import SwiftUI

@main
struct DemoApp: App {
    @StateObject private var settings = AppSettings()
    var body: some Scene {
        WindowGroup {
            RootView()
                .environmentObject(settings)
        }
    }
}

class AppSettings: ObservableObject {
    @Published var username = "Kevin"
    @Published var notifications = true
}
