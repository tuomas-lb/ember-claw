import SwiftUI

struct GlassBackground: ViewModifier {
    var cornerRadius: CGFloat = 16

    func body(content: Content) -> some View {
        content
            .background(
                ZStack {
                    VisualEffectView(material: .popover, blendingMode: .withinWindow)
                    Color.white.opacity(0.15)
                }
                .clipShape(RoundedRectangle(cornerRadius: cornerRadius, style: .continuous))
            )
            .overlay(
                RoundedRectangle(cornerRadius: cornerRadius, style: .continuous)
                    .stroke(Color.white.opacity(0.3), lineWidth: 1)
            )
            .shadow(color: .black.opacity(0.1), radius: 10)
    }
}

extension View {
    func glassBackground(cornerRadius: CGFloat = 16) -> some View {
        modifier(GlassBackground(cornerRadius: cornerRadius))
    }
}
