import SwiftUI

struct StatusBadge: View {
    let status: InstanceStatus
    var compact: Bool = false

    @State private var isPulsing = false

    var body: some View {
        HStack(spacing: 4) {
            Image(systemName: status.iconName)
                .font(.system(size: compact ? 10 : 12, weight: .semibold))
                .foregroundColor(status.color)

            if !compact {
                Text(status.rawValue)
                    .font(.system(size: 11, design: .rounded))
                    .fontWeight(.medium)
                    .foregroundColor(status.color)
            }
        }
        .padding(.horizontal, compact ? 6 : 8)
        .padding(.vertical, 3)
        .background(
            Capsule()
                .fill(status.color.opacity(0.15))
        )
        .opacity(status == .pending && isPulsing ? 0.6 : 1.0)
        .onAppear {
            if status == .pending {
                withAnimation(.easeInOut(duration: 1.0).repeatForever(autoreverses: true)) {
                    isPulsing = true
                }
            }
        }
    }
}
