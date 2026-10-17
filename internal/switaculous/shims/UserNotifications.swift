// Preview stand-in: requests are kept in memory and never delivered, and
// permission is always granted.

enum UNAuthorizationOptions: Hashable {
    case badge, sound, alert, carPlay, criticalAlert, providesAppNotificationSettings, provisional, timeSensitive
}

enum UNAuthorizationStatus: Hashable {
    case notDetermined, denied, authorized, provisional, ephemeral
}

enum UNNotificationPresentationOptions: Hashable {
    case badge, sound, banner, list
}

enum UNNotificationActionOptions: Hashable {
    case authenticationRequired, destructive, foreground
}

enum UNNotificationCategoryOptions: Hashable {
    case customDismissAction, allowInCarPlay, hiddenPreviewsShowTitle, hiddenPreviewsShowSubtitle
}

enum UNNotificationInterruptionLevel: Hashable {
    case passive, active, timeSensitive, critical
}

struct UNNotificationSoundName: Hashable {
    var rawValue: String
    init(_ rawValue: String) { self.rawValue = rawValue }
    init(rawValue: String) { self.rawValue = rawValue }
}

struct UNNotificationSound: Hashable {
    var name: UNNotificationSoundName?
    init(named name: UNNotificationSoundName) { self.name = name }
    init() { self.name = nil }
    static let `default` = UNNotificationSound()
    static let defaultCritical = UNNotificationSound()
}

struct UNNotificationActionIcon: Hashable {
    var systemImageName: String?
    var templateImageName: String?
    init(systemImageName: String) { self.systemImageName = systemImageName }
    init(templateImageName: String) { self.templateImageName = templateImageName }
}

class UNNotificationAction {
    var identifier: String
    var title: String
    var options: [UNNotificationActionOptions]
    var icon: UNNotificationActionIcon?

    init(identifier: String, title: String, options: [UNNotificationActionOptions] = [], icon: UNNotificationActionIcon? = nil) {
        self.identifier = identifier
        self.title = title
        self.options = options
        self.icon = icon
    }
}

final class UNTextInputNotificationAction: UNNotificationAction {
    var textInputButtonTitle: String = ""
    var textInputPlaceholder: String = ""
}

final class UNNotificationCategory {
    var identifier: String
    var actions: [UNNotificationAction]
    var intentIdentifiers: [String]
    var options: [UNNotificationCategoryOptions]

    init(identifier: String, actions: [UNNotificationAction], intentIdentifiers: [String], options: [UNNotificationCategoryOptions] = []) {
        self.identifier = identifier
        self.actions = actions
        self.intentIdentifiers = intentIdentifiers
        self.options = options
    }
}

class UNNotificationContent {
    var title: String = ""
    var subtitle: String = ""
    var body: String = ""
    var badge: Int? = nil
    var sound: UNNotificationSound? = nil
    var categoryIdentifier: String = ""
    var threadIdentifier: String = ""
    var userInfo: [String: Any] = [:]
    var interruptionLevel: UNNotificationInterruptionLevel = .active
    var relevanceScore: Double = 0
    var targetContentIdentifier: String? = nil

    init() {}
}

final class UNMutableNotificationContent: UNNotificationContent {
}

class UNNotificationTrigger {
    var repeats: Bool = false
}

final class UNTimeIntervalNotificationTrigger: UNNotificationTrigger {
    var timeInterval: TimeInterval

    init(timeInterval: TimeInterval, repeats: Bool) {
        self.timeInterval = timeInterval
        super.init()
        self.repeats = repeats
    }

    func nextTriggerDate() -> Date? { Date().addingTimeInterval(timeInterval) }
}

final class UNCalendarNotificationTrigger: UNNotificationTrigger {
    var dateComponents: DateComponents

    init(dateMatching dateComponents: DateComponents, repeats: Bool) {
        self.dateComponents = dateComponents
        super.init()
        self.repeats = repeats
    }

    func nextTriggerDate() -> Date? { Calendar.current.nextDate(after: Date(), matching: dateComponents, matchingPolicy: .nextTime) }
}

final class UNNotificationRequest {
    var identifier: String
    var content: UNNotificationContent
    var trigger: UNNotificationTrigger?

    init(identifier: String, content: UNNotificationContent, trigger: UNNotificationTrigger?) {
        self.identifier = identifier
        self.content = content
        self.trigger = trigger
    }
}

final class UNNotification {
    var request: UNNotificationRequest
    var date: Date

    init(request: UNNotificationRequest, date: Date = Date()) {
        self.request = request
        self.date = date
    }
}

final class UNNotificationResponse {
    static let defaultActionIdentifier = "com.apple.UNNotificationDefaultActionIdentifier"
    static let dismissActionIdentifier = "com.apple.UNNotificationDismissActionIdentifier"

    var notification: UNNotification
    var actionIdentifier: String

    init(notification: UNNotification, actionIdentifier: String) {
        self.notification = notification
        self.actionIdentifier = actionIdentifier
    }
}

final class UNNotificationSettings {
    var authorizationStatus: UNAuthorizationStatus = .authorized
    var alertSetting: Bool = true
    var soundSetting: Bool = true
    var badgeSetting: Bool = true
}

protocol UNUserNotificationCenterDelegate {}

final class UNUserNotificationCenter {
    private static let sharedCenter = UNUserNotificationCenter()

    var delegate: UNUserNotificationCenterDelegate?
    private(set) var pendingRequests: [UNNotificationRequest] = []
    private(set) var categories: [UNNotificationCategory] = []

    static func current() -> UNUserNotificationCenter { sharedCenter }

    func requestAuthorization(options: [UNAuthorizationOptions] = []) async throws -> Bool { true }

    func requestAuthorization(options: [UNAuthorizationOptions] = [], completionHandler: @escaping (Bool, Error?) -> Void) {
        completionHandler(true, nil)
    }

    func notificationSettings() async -> UNNotificationSettings { UNNotificationSettings() }

    func setNotificationCategories(_ categories: [UNNotificationCategory]) {
        self.categories = categories
    }

    func notificationCategories() async -> [UNNotificationCategory] { categories }

    func add(_ request: UNNotificationRequest) async throws {
        pendingRequests.removeAll { $0.identifier == request.identifier }
        pendingRequests.append(request)
    }

    func add(_ request: UNNotificationRequest, withCompletionHandler handler: ((Error?) -> Void)? = nil) {
        pendingRequests.removeAll { $0.identifier == request.identifier }
        pendingRequests.append(request)
        handler?(nil)
    }

    func pendingNotificationRequests() async -> [UNNotificationRequest] { pendingRequests }

    func deliveredNotifications() async -> [UNNotification] { [] }

    func removePendingNotificationRequests(withIdentifiers identifiers: [String]) {
        pendingRequests.removeAll { identifiers.contains($0.identifier) }
    }

    func removeAllPendingNotificationRequests() {
        pendingRequests = []
    }

    func removeDeliveredNotifications(withIdentifiers identifiers: [String]) {}

    func removeAllDeliveredNotifications() {}

    func setBadgeCount(_ count: Int) async throws {}
}
