// Foundation pieces the preview has no native version of.

final class ProcessInfo {
    static let processInfo = ProcessInfo()

    var arguments: [String] { [] }
    var environment: [String: String] { [:] }
    var processName: String { "Preview" }
    var hostName: String { "preview" }
    var isLowPowerModeEnabled: Bool { false }
    var isiOSAppOnMac: Bool { false }
    var isMacCatalystApp: Bool { false }
    var systemUptime: TimeInterval { 0 }
    var processorCount: Int { 1 }
    var operatingSystemVersionString: String { "Preview" }
    var thermalState: ThermalState { .nominal }

    enum ThermalState: Hashable {
        case nominal, fair, serious, critical
    }
}
