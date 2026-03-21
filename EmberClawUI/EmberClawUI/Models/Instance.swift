import SwiftUI

enum InstanceStatus: String, CaseIterable {
    case running = "Running"
    case error = "Error"
    case pending = "Pending"
    case unknown = "Unknown"

    var color: Color {
        switch self {
        case .running: return Color(red: 0.29, green: 0.87, blue: 0.50)  // #4ADE80
        case .error: return Color(red: 0.97, green: 0.44, blue: 0.44)    // #F87171
        case .pending: return Color(red: 0.98, green: 0.75, blue: 0.14)  // #FBBF24
        case .unknown: return .gray
        }
    }

    var iconName: String {
        switch self {
        case .running: return "checkmark.circle.fill"
        case .error: return "exclamationmark.triangle.fill"
        case .pending: return "clock.fill"
        case .unknown: return "questionmark.circle"
        }
    }

    init(from string: String) {
        let lowered = string.lowercased().trimmingCharacters(in: .whitespaces)
        switch lowered {
        case "running": self = .running
        case "error", "crashloopbackoff", "imagepullbackoff": self = .error
        case "pending", "containercreating": self = .pending
        default: self = .unknown
        }
    }
}

struct Instance: Identifiable, Hashable {
    let id: UUID
    var name: String
    var status: InstanceStatus
    var ready: String
    var restarts: Int
    var age: String
    var provider: String?
    var model: String?

    init(
        id: UUID = UUID(),
        name: String,
        status: InstanceStatus,
        ready: String = "0/0",
        restarts: Int = 0,
        age: String = "",
        provider: String? = nil,
        model: String? = nil
    ) {
        self.id = id
        self.name = name
        self.status = status
        self.ready = ready
        self.restarts = restarts
        self.age = age
        self.provider = provider
        self.model = model
    }

    static func == (lhs: Instance, rhs: Instance) -> Bool {
        lhs.name == rhs.name
    }

    func hash(into hasher: inout Hasher) {
        hasher.combine(name)
    }
}

struct ChatMessage: Identifiable {
    let id: UUID
    let role: MessageRole
    let content: String
    let timestamp: Date

    init(id: UUID = UUID(), role: MessageRole, content: String, timestamp: Date = Date()) {
        self.id = id
        self.role = role
        self.content = content
        self.timestamp = timestamp
    }
}

enum MessageRole {
    case user
    case assistant
}
