// Preview stand-in: intents are declared but the system never invokes them.

protocol IntentResult {}

struct IntentResultContainer: IntentResult {
    var value: Any?
    var dialog: IntentDialog?
}

struct IntentDialog: Hashable {
    var full: String
    var supporting: String?

    init(_ full: String) {
        self.full = full
    }

    init(full: String, supporting: String? = nil) {
        self.full = full
        self.supporting = supporting
    }
}

protocol AppIntent {}
protocol LiveActivityIntent: AppIntent {}
protocol AudioPlaybackIntent: AppIntent {}
protocol SystemIntent: AppIntent {}
protocol OpenIntent: AppIntent {}
protocol AppEntity {}
protocol AppEnum {}
protocol AppShortcutsProvider {}

extension AppIntent {
    static var isDiscoverable: Bool { true }
    static var openAppWhenRun: Bool { false }
    static var description: String? { nil }

    static func result() -> IntentResultContainer {
        IntentResultContainer(value: nil, dialog: nil)
    }

    static func result(dialog: IntentDialog) -> IntentResultContainer {
        IntentResultContainer(value: nil, dialog: dialog)
    }

    static func result<Value>(value: Value) -> IntentResultContainer {
        IntentResultContainer(value: value, dialog: nil)
    }
}

struct IntentDescription: Hashable {
    var descriptionText: String
    var categoryName: String?

    init(_ descriptionText: String, categoryName: String? = nil) {
        self.descriptionText = descriptionText
        self.categoryName = categoryName
    }
}
