import SwiftUI

struct ContentView: View {
    @StateObject private var cliService = CLIService()
    @State private var selectedInstance: Instance?
    @State private var refreshTimer: Timer?

    var body: some View {
        NavigationSplitView {
            SidebarView(
                cliService: cliService,
                selectedInstance: $selectedInstance
            )
        } detail: {
            if let instance = selectedInstance {
                ChatView(instance: instance, cliService: cliService)
            } else {
                emptyState
            }
        }
        .navigationSplitViewStyle(.balanced)
        .onAppear {
            startRefreshTimer()
        }
        .onDisappear {
            refreshTimer?.invalidate()
        }
    }

    private var emptyState: some View {
        VStack(spacing: 20) {
            Image(nsImage: NSImage(named: "AppIcon") ?? NSImage())
                .resizable()
                .frame(width: 80, height: 80)
                .clipShape(RoundedRectangle(cornerRadius: 16, style: .continuous))
                .shadow(color: .black.opacity(0.1), radius: 10)

            Text("EmberClaw")
                .font(.system(size: 28, weight: .bold, design: .rounded))
                .foregroundColor(.primary)

            Text("Select an instance or deploy a new one")
                .font(.system(size: 16, design: .rounded))
                .foregroundColor(.secondary)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    private func startRefreshTimer() {
        refreshTimer = Timer.scheduledTimer(withTimeInterval: 10, repeats: true) { _ in
            Task {
                await cliService.refreshInstances()
            }
        }
    }
}

// MARK: - Color Extensions

extension Color {
    static let emberPurple = Color(red: 0.506, green: 0.549, blue: 0.973)   // #818CF8
    static let emberPink = Color(red: 0.957, green: 0.447, blue: 0.714)      // #F472B6
    static let emberGreen = Color(red: 0.290, green: 0.871, blue: 0.502)     // #4ADE80
    static let emberAmber = Color(red: 0.984, green: 0.749, blue: 0.141)     // #FBBF24
    static let emberRed = Color(red: 0.973, green: 0.443, blue: 0.443)       // #F87171
}
