import SwiftUI

struct SidebarView: View {
    @ObservedObject var cliService: CLIService
    @Binding var selectedInstance: Instance?
    @State private var showDeployWizard = false

    var body: some View {
        VStack(spacing: 0) {
            // Header
            HStack(spacing: 10) {
                Image(nsImage: NSImage(named: "AppIcon") ?? NSImage())
                    .resizable()
                    .frame(width: 32, height: 32)
                    .clipShape(RoundedRectangle(cornerRadius: 8, style: .continuous))

                Text("EmberClaw")
                    .font(.system(size: 20, weight: .bold, design: .rounded))
                    .foregroundColor(.primary)

                Spacer()

                Button(action: {
                    Task { await cliService.refreshInstances() }
                }) {
                    Image(systemName: "arrow.clockwise")
                        .font(.system(size: 14))
                        .foregroundColor(.secondary)
                }
                .buttonStyle(.plain)
                .help("Refresh instances")
            }
            .padding(.horizontal, 16)
            .padding(.vertical, 12)

            Divider()
                .padding(.horizontal, 16)

            // Instance List
            if cliService.isLoading && cliService.instances.isEmpty {
                Spacer()
                ProgressView()
                    .scaleEffect(0.8)
                Text("Loading instances...")
                    .font(.system(size: 13, design: .rounded))
                    .foregroundColor(.secondary)
                    .padding(.top, 8)
                Spacer()
            } else if cliService.instances.isEmpty {
                Spacer()
                VStack(spacing: 12) {
                    Image(systemName: "cube.transparent")
                        .font(.system(size: 36))
                        .foregroundColor(.secondary.opacity(0.5))

                    Text("No instances")
                        .font(.system(size: 14, weight: .medium, design: .rounded))
                        .foregroundColor(.secondary)

                    Text("Deploy your first PicoClaw instance")
                        .font(.system(size: 12, design: .rounded))
                        .foregroundColor(.secondary.opacity(0.7))
                        .multilineTextAlignment(.center)
                }
                .padding()
                Spacer()
            } else {
                ScrollView {
                    LazyVStack(spacing: 4) {
                        ForEach(cliService.instances) { instance in
                            InstanceRow(
                                instance: instance,
                                isSelected: selectedInstance == instance,
                                cliService: cliService
                            )
                            .onTapGesture {
                                withAnimation(.easeInOut(duration: 0.2)) {
                                    selectedInstance = instance
                                }
                            }
                            .contextMenu {
                                Button("Restart") {
                                    Task {
                                        try? await cliService.restartInstance(name: instance.name)
                                        await cliService.refreshInstances()
                                    }
                                }
                                Divider()
                                Button("Delete", role: .destructive) {
                                    Task {
                                        try? await cliService.deleteInstance(name: instance.name)
                                        if selectedInstance == instance {
                                            selectedInstance = nil
                                        }
                                        await cliService.refreshInstances()
                                    }
                                }
                            }
                        }
                    }
                    .padding(.horizontal, 8)
                    .padding(.vertical, 8)
                }
            }

            if let error = cliService.errorMessage {
                HStack(spacing: 6) {
                    Image(systemName: "exclamationmark.triangle.fill")
                        .font(.system(size: 11))
                        .foregroundColor(.emberRed)
                    Text(error)
                        .font(.system(size: 11, design: .rounded))
                        .foregroundColor(.secondary)
                        .lineLimit(2)
                }
                .padding(.horizontal, 12)
                .padding(.vertical, 8)
                .glassBackground(cornerRadius: 8)
                .padding(.horizontal, 12)
                .padding(.bottom, 4)
            }

            Divider()
                .padding(.horizontal, 16)

            // Deploy Button
            Button(action: { showDeployWizard = true }) {
                HStack {
                    Image(systemName: "plus.circle.fill")
                        .font(.system(size: 16))
                    Text("Deploy Instance")
                        .font(.system(size: 14, weight: .medium, design: .rounded))
                }
                .frame(maxWidth: .infinity)
                .padding(.vertical, 10)
                .background(
                    LinearGradient(
                        colors: [.emberPurple, .emberPink],
                        startPoint: .leading,
                        endPoint: .trailing
                    )
                    .opacity(0.8)
                )
                .foregroundColor(.white)
                .clipShape(RoundedRectangle(cornerRadius: 10, style: .continuous))
            }
            .buttonStyle(.plain)
            .padding(.horizontal, 12)
            .padding(.vertical, 10)
        }
        .frame(minWidth: 250, maxWidth: 280)
        .sheet(isPresented: $showDeployWizard) {
            DeployWizardView(cliService: cliService, isPresented: $showDeployWizard)
        }
        .task {
            await cliService.refreshInstances()
        }
    }
}

struct InstanceRow: View {
    let instance: Instance
    let isSelected: Bool
    let cliService: CLIService

    var body: some View {
        HStack(spacing: 10) {
            StatusBadge(status: instance.status, compact: true)

            VStack(alignment: .leading, spacing: 2) {
                Text(instance.name)
                    .font(.system(size: 13, weight: .medium, design: .rounded))
                    .foregroundColor(.primary)

                Text(instance.age)
                    .font(.system(size: 11, design: .rounded))
                    .foregroundColor(.secondary)
            }

            Spacer()

            Text(instance.ready)
                .font(.system(size: 11, weight: .medium, design: .monospaced))
                .foregroundColor(.secondary)
        }
        .padding(.horizontal, 12)
        .padding(.vertical, 8)
        .background(
            RoundedRectangle(cornerRadius: 10, style: .continuous)
                .fill(isSelected ? Color.accentColor.opacity(0.15) : Color.clear)
        )
        .contentShape(Rectangle())
    }
}
