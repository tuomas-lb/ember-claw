import Foundation

enum CLIError: LocalizedError {
    case binaryNotFound
    case executionFailed(String)
    case parseError(String)

    var errorDescription: String? {
        switch self {
        case .binaryNotFound:
            return "eclaw binary not found. Ensure it is built and available in bin/ or on PATH."
        case .executionFailed(let message):
            return "CLI command failed: \(message)"
        case .parseError(let message):
            return "Failed to parse CLI output: \(message)"
        }
    }
}

@MainActor
class CLIService: ObservableObject {
    @Published var instances: [Instance] = []
    @Published var isLoading = false
    @Published var errorMessage: String?

    private let eclawPath: String?

    init() {
        eclawPath = CLIService.findEclawBinary()
    }

    private static func findEclawBinary() -> String? {
        // First check relative to the app bundle (dev from repo)
        let bundlePath = Bundle.main.bundlePath
        let repoRelativePath = (bundlePath as NSString)
            .deletingLastPathComponent
            .appending("/bin/eclaw")

        if FileManager.default.isExecutableFile(atPath: repoRelativePath) {
            return repoRelativePath
        }

        // Try the repo root bin/ directory directly
        let devPath = ((((bundlePath as NSString)
            .deletingLastPathComponent as NSString)
            .deletingLastPathComponent as NSString)
            .deletingLastPathComponent as NSString)
            .appendingPathComponent("bin/eclaw")

        if FileManager.default.isExecutableFile(atPath: devPath) {
            return devPath
        }

        // Fall back to which
        let whichProcess = Process()
        whichProcess.executableURL = URL(fileURLWithPath: "/usr/bin/which")
        whichProcess.arguments = ["eclaw"]
        let whichPipe = Pipe()
        whichProcess.standardOutput = whichPipe
        whichProcess.standardError = Pipe()

        do {
            try whichProcess.run()
            whichProcess.waitUntilExit()
            let data = whichPipe.fileHandleForReading.readDataToEndOfFile()
            let path = String(data: data, encoding: .utf8)?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
            if !path.isEmpty && FileManager.default.isExecutableFile(atPath: path) {
                return path
            }
        } catch {
            // Fall through
        }

        return nil
    }

    func listInstances() async throws -> [Instance] {
        let output = try await runCLI(arguments: ["list"])
        return parseListOutput(output)
    }

    func getStatus(name: String) async throws -> Instance {
        let output = try await runCLI(arguments: ["status", name])
        return parseStatusOutput(output, fallbackName: name)
    }

    func deployInstance(name: String, provider: String, model: String, apiKey: String) async throws {
        _ = try await runCLI(arguments: [
            "deploy", name,
            "--provider", provider,
            "--model", model,
            "--api-key", apiKey
        ])
    }

    func deleteInstance(name: String) async throws {
        _ = try await runCLI(arguments: ["delete", name])
    }

    func restartInstance(name: String) async throws {
        _ = try await runCLI(arguments: ["restart", name])
    }

    func sendMessage(to instance: String, message: String) async throws -> String {
        let output = try await runCLI(arguments: ["chat", instance, "-m", message])
        return output.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    func refreshInstances() async {
        isLoading = true
        errorMessage = nil
        do {
            instances = try await listInstances()
        } catch {
            errorMessage = error.localizedDescription
        }
        isLoading = false
    }

    // MARK: - Private

    private func runCLI(arguments: [String]) async throws -> String {
        guard let path = eclawPath else {
            throw CLIError.binaryNotFound
        }

        return try await withCheckedThrowingContinuation { continuation in
            DispatchQueue.global(qos: .userInitiated).async {
                let process = Process()
                process.executableURL = URL(fileURLWithPath: path)
                process.arguments = arguments

                let stdoutPipe = Pipe()
                let stderrPipe = Pipe()
                process.standardOutput = stdoutPipe
                process.standardError = stderrPipe

                do {
                    try process.run()
                    process.waitUntilExit()

                    let stdoutData = stdoutPipe.fileHandleForReading.readDataToEndOfFile()
                    let stderrData = stderrPipe.fileHandleForReading.readDataToEndOfFile()
                    let stdout = String(data: stdoutData, encoding: .utf8) ?? ""
                    let stderr = String(data: stderrData, encoding: .utf8) ?? ""

                    if process.terminationStatus != 0 {
                        let errorMsg = stderr.isEmpty ? stdout : stderr
                        continuation.resume(throwing: CLIError.executionFailed(errorMsg.trimmingCharacters(in: .whitespacesAndNewlines)))
                    } else {
                        continuation.resume(returning: stdout)
                    }
                } catch {
                    continuation.resume(throwing: CLIError.executionFailed(error.localizedDescription))
                }
            }
        }
    }

    private func parseListOutput(_ output: String) -> [Instance] {
        let lines = output.components(separatedBy: "\n")
        let borderChars: Set<Character> = ["─", "┌", "┐", "└", "┘", "├", "┤", "┬", "┴", "┼"]

        var instances: [Instance] = []

        for line in lines {
            let trimmed = line.trimmingCharacters(in: .whitespaces)
            guard !trimmed.isEmpty else { continue }

            // Skip border rows
            if trimmed.contains(where: { borderChars.contains($0) }) {
                continue
            }

            // Skip header row
            if trimmed.uppercased().contains("NAME") && trimmed.uppercased().contains("STATUS") {
                continue
            }

            // Parse data row: split by pipe character
            let columns = trimmed
                .split(separator: "\u{2502}")  // Box-drawing vertical bar
                .map { $0.trimmingCharacters(in: .whitespaces) }
                .filter { !$0.isEmpty }

            guard columns.count >= 5 else { continue }

            let name = columns[0]
            let status = InstanceStatus(from: columns[1])
            let ready = columns[2]
            let restarts = Int(columns[3]) ?? 0
            let age = columns[4]

            instances.append(Instance(
                name: name,
                status: status,
                ready: ready,
                restarts: restarts,
                age: age
            ))
        }

        return instances
    }

    private func parseStatusOutput(_ output: String, fallbackName: String) -> Instance {
        var name = fallbackName
        var status: InstanceStatus = .unknown
        var ready = "0/0"
        var restarts = 0
        var age = ""
        var provider: String?
        var model: String?

        let lines = output.components(separatedBy: "\n")
        for line in lines {
            let parts = line.split(separator: ":", maxSplits: 1).map {
                $0.trimmingCharacters(in: .whitespaces)
            }
            guard parts.count == 2 else { continue }

            let key = parts[0].lowercased()
            let value = parts[1]

            switch key {
            case "name": name = value
            case "pod phase": status = InstanceStatus(from: value)
            case "ready": ready = value
            case "restarts": restarts = Int(value) ?? 0
            case "age": age = value
            case "provider": if !value.isEmpty { provider = value }
            case "model": if !value.isEmpty { model = value }
            default: break
            }
        }

        return Instance(
            name: name,
            status: status,
            ready: ready,
            restarts: restarts,
            age: age,
            provider: provider,
            model: model
        )
    }
}
