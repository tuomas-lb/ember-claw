import SwiftUI

@main
struct EmberClawUIApp: App {
    var body: some Scene {
        WindowGroup {
            ContentView()
                .frame(minWidth: 900, minHeight: 600)
                .background(VisualEffectView(material: .hudWindow, blendingMode: .behindWindow))
                .onAppear {
                    applyWindowAppearance()
                }
        }
        .windowStyle(.hiddenTitleBar)
    }

    private func applyWindowAppearance() {
        DispatchQueue.main.async {
            if let window = NSApplication.shared.windows.first {
                window.isOpaque = false
                window.backgroundColor = .clear
                window.titlebarAppearsTransparent = true
                window.styleMask.insert(.fullSizeContentView)

                let visualEffect = NSVisualEffectView()
                visualEffect.material = .hudWindow
                visualEffect.blendingMode = .behindWindow
                visualEffect.state = .active
                visualEffect.autoresizingMask = [.width, .height]
                visualEffect.frame = window.contentView?.bounds ?? .zero

                if let contentView = window.contentView {
                    contentView.addSubview(visualEffect, positioned: .below, relativeTo: contentView.subviews.first)
                }
            }
        }
    }
}
