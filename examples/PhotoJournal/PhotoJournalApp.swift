import SwiftUI
import SwiftData

@main
struct PhotoJournalApp: App {
    var body: some Scene {
        WindowGroup {
            JournalView()
        }
        .modelContainer(for: Entry.self)
    }
}
