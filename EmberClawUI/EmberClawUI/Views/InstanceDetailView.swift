import SwiftUI

struct InstanceDetailView: View {
    let instance: Instance

    var body: some View {
        VStack(spacing: 16) {
            HStack {
                Text(instance.name)
                    .font(.system(size: 18, weight: .bold, design: .rounded))
                StatusBadge(status: instance.status)
                Spacer()
            }

            VStack(spacing: 12) {
                DetailRow(label: "Ready", value: instance.ready)
                DetailRow(label: "Restarts", value: "\(instance.restarts)")
                DetailRow(label: "Age", value: instance.age)

                if let provider = instance.provider {
                    DetailRow(label: "Provider", value: provider)
                }
                if let model = instance.model {
                    DetailRow(label: "Model", value: model)
                }
            }
        }
        .padding(20)
        .glassBackground()
    }
}

struct DetailRow: View {
    let label: String
    let value: String

    var body: some View {
        HStack {
            Text(label)
                .font(.system(size: 13, weight: .medium, design: .rounded))
                .foregroundColor(.secondary)
            Spacer()
            Text(value)
                .font(.system(size: 13, weight: .semibold, design: .rounded))
                .foregroundColor(.primary)
        }
    }
}
