// Preview stand-in: alarms are kept in memory and never ring.

protocol AlarmMetadata {}

extension Locale {
    enum Weekday: String, CaseIterable, Hashable {
        case sunday, monday, tuesday, wednesday, thursday, friday, saturday
    }
}

struct AlarmButton: Hashable {
    var text: LocalizedStringResource
    var textColor: Color
    var systemImageName: String

    init(text: LocalizedStringResource, textColor: Color, systemImageName: String) {
        self.text = text
        self.textColor = textColor
        self.systemImageName = systemImageName
    }
}

struct AlarmPresentation {
    struct Alert {
        enum SecondaryButtonBehavior {
            case countdown
            case custom
        }

        var title: LocalizedStringResource
        var stopButton: AlarmButton
        var secondaryButton: AlarmButton?
        var secondaryButtonBehavior: SecondaryButtonBehavior?

        init(title: LocalizedStringResource, stopButton: AlarmButton = AlarmButton(text: "Stop", textColor: .white, systemImageName: "stop.fill"), secondaryButton: AlarmButton? = nil, secondaryButtonBehavior: SecondaryButtonBehavior? = nil) {
            self.title = title
            self.stopButton = stopButton
            self.secondaryButton = secondaryButton
            self.secondaryButtonBehavior = secondaryButtonBehavior
        }
    }

    struct Countdown {
        var title: LocalizedStringResource
        var pauseButton: AlarmButton?
    }

    struct Paused {
        var title: LocalizedStringResource
        var resumeButton: AlarmButton
    }

    var alert: Alert
    var countdown: Countdown?
    var paused: Paused?

    init(alert: Alert, countdown: Countdown? = nil, paused: Paused? = nil) {
        self.alert = alert
        self.countdown = countdown
        self.paused = paused
    }
}

struct AlarmAttributes<Metadata> {
    var presentation: AlarmPresentation
    var metadata: Metadata?
    var tintColor: Color

    init(presentation: AlarmPresentation, metadata: Metadata? = nil, tintColor: Color) {
        self.presentation = presentation
        self.metadata = metadata
        self.tintColor = tintColor
    }
}

enum AlertConfiguration {
    enum AlertSound: Hashable {
        case `default`
        case named(String)
    }
}

struct Alarm: Identifiable, Hashable {
    enum State: Hashable {
        case scheduled
        case countdown
        case paused
        case alerting
    }

    enum Schedule: Hashable {
        struct Relative: Hashable {
            struct Time: Hashable {
                var hour: Int
                var minute: Int

                init(hour: Int, minute: Int) {
                    self.hour = hour
                    self.minute = minute
                }
            }

            enum Recurrence: Hashable {
                case never
                case weekly([Locale.Weekday])
            }

            var time: Time
            var repeats: Recurrence

            init(time: Time, repeats: Recurrence = .never) {
                self.time = time
                self.repeats = repeats
            }
        }

        case fixed(Date)
        case relative(Relative)
    }

    struct CountdownDuration: Hashable {
        var preAlert: TimeInterval?
        var postAlert: TimeInterval?

        init(preAlert: TimeInterval? = nil, postAlert: TimeInterval? = nil) {
            self.preAlert = preAlert
            self.postAlert = postAlert
        }
    }

    var id: UUID
    var state: State
    var schedule: Schedule?
    var countdownDuration: CountdownDuration?

    static func == (lhs: Alarm, rhs: Alarm) -> Bool { lhs.id == rhs.id }
    func hash(into hasher: inout Hasher) { hasher.combine(id) }
}

@Observable
final class AlarmManager {
    enum AuthorizationState: Hashable {
        case notDetermined
        case authorized
        case denied
    }

    struct AlarmConfiguration<Metadata> {
        var schedule: Alarm.Schedule?
        var countdownDuration: Alarm.CountdownDuration?
        var attributes: AlarmAttributes<Metadata>
        var sound: AlertConfiguration.AlertSound

        static func alarm(schedule: Alarm.Schedule, attributes: AlarmAttributes<Metadata>, stopIntent: Any? = nil, secondaryIntent: Any? = nil, sound: AlertConfiguration.AlertSound = .default) -> AlarmConfiguration<Metadata> {
            AlarmConfiguration(schedule: schedule, countdownDuration: nil, attributes: attributes, sound: sound)
        }

        static func timer(duration: TimeInterval, attributes: AlarmAttributes<Metadata>, stopIntent: Any? = nil, secondaryIntent: Any? = nil, sound: AlertConfiguration.AlertSound = .default) -> AlarmConfiguration<Metadata> {
            AlarmConfiguration(schedule: nil, countdownDuration: Alarm.CountdownDuration(preAlert: duration), attributes: attributes, sound: sound)
        }
    }

    static let shared = AlarmManager()

    // The preview behaves like a device that already granted alarm access.
    private(set) var authorizationState: AuthorizationState = .authorized
    private(set) var alarms: [Alarm] = []

    // Real AlarmKit exposes these as async sequences; here they are finite and
    // yield the current value once, so `for await` loops run a single pass.
    var alarmUpdates: [[Alarm]] { [alarms] }
    var authorizationUpdates: [AuthorizationState] { [authorizationState] }

    func requestAuthorization() async throws -> AuthorizationState {
        authorizationState = .authorized
        return authorizationState
    }

    func schedule<Metadata>(id: UUID, configuration: AlarmConfiguration<Metadata>) async throws -> Alarm {
        let alarm = Alarm(id: id, state: .scheduled, schedule: configuration.schedule, countdownDuration: configuration.countdownDuration)
        alarms.removeAll { $0.id == id }
        alarms.append(alarm)
        return alarm
    }

    func cancel(id: UUID) throws {
        alarms.removeAll { $0.id == id }
    }

    func stop(id: UUID) throws {
        alarms.removeAll { $0.id == id }
    }

    func pause(id: UUID) throws {
        if let index = alarms.firstIndex(where: { $0.id == id }) {
            alarms[index].state = .paused
        }
    }

    func resume(id: UUID) throws {
        if let index = alarms.firstIndex(where: { $0.id == id }) {
            alarms[index].state = .countdown
        }
    }

    func countdown(id: UUID) throws {
        if let index = alarms.firstIndex(where: { $0.id == id }) {
            alarms[index].state = .countdown
        }
    }
}
