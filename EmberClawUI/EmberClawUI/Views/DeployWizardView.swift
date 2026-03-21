import SwiftUI

struct DeployWizardView: View {
    @ObservedObject var cliService: CLIService
    @Binding var isPresented: Bool

    @State private var instanceName = ""
    @State private var provider = "openai"
    @State private var model = ""
    @State private var apiKey = ""
    @State private var isDeploying = false
    @State private var errorMessage: String?

    private let providers = ["openai", "gemini", "anthropic", "groq", "deepseek", "openrouter"]

    private var isFormValid: Bool {
        let namePattern = /^[a-zA-Z0-9][a-zA-Z0-9\-]*$/
        return !instanceName.isEmpty
            && instanceName.wholeMatch(of: namePattern) != nil
            && !model.isEmpty
            && !apiKey.isEmpty
    }

    var body: some View {
        VStack(spacing: 0) {
            // Header
            HStack {
                Text("Deploy New Instance")
                    .font(.system(size: 18, weight: .bold, design: .rounded))

                Spacer()

                Button(action: { isPresented = false }) {
                    Image(systemName: "xmark.circle.fill")
                        .font(.system(size: 20))
                        .foregroundColor(.secondary)
                }
                .buttonStyle(.plain)
            }
            .padding(20)

            Divider()

            // Form
            ScrollView {
                VStack(spacing: 20) {
                    // Instance Name
                    FormField(label: "Instance Name", hint: "Alphanumeric and hyphens only") {
                        TextField("my-agent", text: $instanceName)
                            .textFieldStyle(.plain)
                            .font(.system(size: 14, design: .rounded))
                            .padding(10)
                            .glassBackground(cornerRadius: 8)
                    }

                    // Provider
                    FormField(label: "Provider", hint: "AI provider service") {
                        Picker("", selection: $provider) {
                            ForEach(providers, id: \.self) { p in
                                Text(p).tag(p)
                            }
                        }
                        .pickerStyle(.menu)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .padding(6)
                        .glassBackground(cornerRadius: 8)
                    }

                    // Model
                    FormField(label: "Model", hint: "e.g., gpt-4, gemini-pro, claude-3-opus") {
                        TextField("gpt-4", text: $model)
                            .textFieldStyle(.plain)
                            .font(.system(size: 14, design: .rounded))
                            .padding(10)
                            .glassBackground(cornerRadius: 8)
                    }

                    // API Key
                    FormField(label: "API Key", hint: "Your provider API key") {
                        SecureField("sk-...", text: $apiKey)
                            .textFieldStyle(.plain)
                            .font(.system(size: 14, design: .rounded))
                            .padding(10)
                            .glassBackground(cornerRadius: 8)
                    }

                    if let error = errorMessage {
                        HStack(spacing: 8) {
                            Image(systemName: "exclamationmark.triangle.fill")
                                .foregroundColor(.emberRed)
                            Text(error)
                                .font(.system(size: 13, design: .rounded))
                                .foregroundColor(.emberRed)
                        }
                        .padding(12)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .glassBackground(cornerRadius: 8)
                    }
                }
                .padding(20)
            }

            Divider()

            // Actions
            HStack(spacing: 12) {
                Button("Cancel") {
                    isPresented = false
                }
                .buttonStyle(.plain)
                .font(.system(size: 14, weight: .medium, design: .rounded))
                .foregroundColor(.secondary)
                .padding(.horizontal, 20)
                .padding(.vertical, 10)

                Spacer()

                Button(action: deploy) {
                    HStack(spacing: 6) {
                        if isDeploying {
                            ProgressView()
                                .controlSize(.small)
                        } else {
                            Image(systemName: "rocket.fill")
                        }
                        Text(isDeploying ? "Deploying..." : "Deploy")
                    }
                    .font(.system(size: 14, weight: .semibold, design: .rounded))
                    .foregroundColor(.white)
                    .padding(.horizontal, 24)
                    .padding(.vertical, 10)
                    .background(
                        LinearGradient(
                            colors: isFormValid && !isDeploying
                                ? [.emberPurple, .emberPink]
                                : [.gray.opacity(0.5)],
                            startPoint: .leading,
                            endPoint: .trailing
                        )
                    )
                    .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
                }
                .buttonStyle(.plain)
                .disabled(!isFormValid || isDeploying)
            }
            .padding(20)
        }
        .frame(width: 460, height: 520)
        .background(
            VisualEffectView(material: .hudWindow, blendingMode: .behindWindow)
        )
    }

    private func deploy() {
        isDeploying = true
        errorMessage = nil

        Task {
            do {
                try await cliService.deployInstance(
                    name: instanceName,
                    provider: provider,
                    model: model,
                    apiKey: apiKey
                )
                await cliService.refreshInstances()
                isPresented = false
            } catch {
                errorMessage = error.localizedDescription
            }
            isDeploying = false
        }
    }
}

struct FormField<Content: View>: View {
    let label: String
    let hint: String
    @ViewBuilder let content: Content

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(label)
                .font(.system(size: 13, weight: .semibold, design: .rounded))
                .foregroundColor(.primary)

            content

            Text(hint)
                .font(.system(size: 11, design: .rounded))
                .foregroundColor(.secondary.opacity(0.7))
        }
    }
}
