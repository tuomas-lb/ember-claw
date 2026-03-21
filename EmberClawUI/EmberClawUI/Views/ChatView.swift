import SwiftUI

struct ChatView: View {
    let instance: Instance
    @ObservedObject var cliService: CLIService
    @State private var messages: [ChatMessage] = []
    @State private var inputText = ""
    @State private var isWaitingForResponse = false
    @State private var showDeleteConfirmation = false

    var body: some View {
        VStack(spacing: 0) {
            // Top bar
            topBar

            Divider()

            // Messages
            messageList

            Divider()

            // Input bar
            inputBar
        }
        .background(Color.clear)
    }

    // MARK: - Top Bar

    private var topBar: some View {
        HStack(spacing: 12) {
            VStack(alignment: .leading, spacing: 2) {
                Text(instance.name)
                    .font(.system(size: 18, weight: .semibold, design: .rounded))

                if let provider = instance.provider, let model = instance.model {
                    Text("\(provider) / \(model)")
                        .font(.system(size: 12, design: .rounded))
                        .foregroundColor(.secondary)
                }
            }

            StatusBadge(status: instance.status)

            Spacer()

            HStack(spacing: 8) {
                Button(action: {
                    Task {
                        try? await cliService.restartInstance(name: instance.name)
                        await cliService.refreshInstances()
                    }
                }) {
                    Image(systemName: "arrow.counterclockwise")
                        .font(.system(size: 14))
                        .foregroundColor(.secondary)
                }
                .buttonStyle(.plain)
                .help("Restart instance")

                Button(action: { showDeleteConfirmation = true }) {
                    Image(systemName: "trash")
                        .font(.system(size: 14))
                        .foregroundColor(.emberRed)
                }
                .buttonStyle(.plain)
                .help("Delete instance")
            }
        }
        .padding(.horizontal, 20)
        .padding(.vertical, 12)
        .alert("Delete Instance", isPresented: $showDeleteConfirmation) {
            Button("Cancel", role: .cancel) {}
            Button("Delete", role: .destructive) {
                Task {
                    try? await cliService.deleteInstance(name: instance.name)
                    await cliService.refreshInstances()
                }
            }
        } message: {
            Text("Are you sure you want to delete \"\(instance.name)\"? This action cannot be undone.")
        }
    }

    // MARK: - Message List

    private var messageList: some View {
        ScrollViewReader { proxy in
            ScrollView {
                LazyVStack(spacing: 12) {
                    if messages.isEmpty {
                        emptyChat
                    }

                    ForEach(messages) { message in
                        MessageBubble(message: message)
                            .id(message.id)
                    }

                    if isWaitingForResponse {
                        loadingBubble
                    }
                }
                .padding(20)
            }
            .onChange(of: messages.count) { _, _ in
                if let lastMessage = messages.last {
                    withAnimation(.easeOut(duration: 0.3)) {
                        proxy.scrollTo(lastMessage.id, anchor: .bottom)
                    }
                }
            }
        }
    }

    private var emptyChat: some View {
        VStack(spacing: 16) {
            Spacer()
                .frame(height: 60)

            Image(systemName: "bubble.left.and.bubble.right")
                .font(.system(size: 48))
                .foregroundColor(.secondary.opacity(0.3))

            Text("Start a conversation")
                .font(.system(size: 16, weight: .medium, design: .rounded))
                .foregroundColor(.secondary)

            Text("Send a message to chat with this PicoClaw instance")
                .font(.system(size: 13, design: .rounded))
                .foregroundColor(.secondary.opacity(0.7))
                .multilineTextAlignment(.center)
        }
        .frame(maxWidth: .infinity)
    }

    private var loadingBubble: some View {
        HStack(alignment: .top, spacing: 8) {
            Image(systemName: "sparkles")
                .font(.system(size: 14))
                .foregroundColor(.emberPurple)
                .frame(width: 24, height: 24)

            TypingIndicator()
                .padding(.horizontal, 16)
                .padding(.vertical, 10)
                .background(Color.white.opacity(0.1))
                .clipShape(BubbleShape(isUser: false))

            Spacer()
        }
    }

    // MARK: - Input Bar

    private var inputBar: some View {
        HStack(spacing: 10) {
            TextField("Type a message...", text: $inputText)
                .textFieldStyle(.plain)
                .font(.system(size: 14, design: .rounded))
                .padding(.horizontal, 16)
                .padding(.vertical, 10)
                .glassBackground(cornerRadius: 20)
                .onSubmit {
                    sendMessage()
                }

            Button(action: sendMessage) {
                Image(systemName: "paperplane.fill")
                    .font(.system(size: 14))
                    .foregroundColor(.white)
                    .frame(width: 36, height: 36)
                    .background(
                        LinearGradient(
                            colors: inputText.isEmpty ? [.gray] : [.emberPurple, .emberPink],
                            startPoint: .topLeading,
                            endPoint: .bottomTrailing
                        )
                    )
                    .clipShape(Circle())
            }
            .buttonStyle(.plain)
            .disabled(inputText.isEmpty || isWaitingForResponse)
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 12)
    }

    // MARK: - Actions

    private func sendMessage() {
        let text = inputText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty, !isWaitingForResponse else { return }

        let userMessage = ChatMessage(role: .user, content: text)
        messages.append(userMessage)
        inputText = ""
        isWaitingForResponse = true

        Task {
            do {
                let response = try await cliService.sendMessage(to: instance.name, message: text)
                let botMessage = ChatMessage(role: .assistant, content: response)
                messages.append(botMessage)
            } catch {
                let errorMessage = ChatMessage(role: .assistant, content: "Error: \(error.localizedDescription)")
                messages.append(errorMessage)
            }
            isWaitingForResponse = false
        }
    }
}

// MARK: - Message Bubble

struct MessageBubble: View {
    let message: ChatMessage

    private var isUser: Bool { message.role == .user }

    var body: some View {
        HStack(alignment: .top, spacing: 8) {
            if isUser { Spacer(minLength: 60) }

            if !isUser {
                Image(systemName: "sparkles")
                    .font(.system(size: 14))
                    .foregroundColor(.emberPurple)
                    .frame(width: 24, height: 24)
            }

            VStack(alignment: isUser ? .trailing : .leading, spacing: 4) {
                Text(message.content)
                    .font(.system(size: 14, design: .rounded))
                    .foregroundColor(.primary)
                    .textSelection(.enabled)
                    .padding(.horizontal, 16)
                    .padding(.vertical, 10)
                    .background(
                        isUser
                            ? AnyShapeStyle(Color.emberPurple.opacity(0.2))
                            : AnyShapeStyle(Color.white.opacity(0.1))
                    )
                    .clipShape(BubbleShape(isUser: isUser))

                Text(message.timestamp, style: .time)
                    .font(.system(size: 10, design: .rounded))
                    .foregroundColor(.secondary.opacity(0.6))
            }

            if isUser {
                Image(systemName: "person.fill")
                    .font(.system(size: 14))
                    .foregroundColor(.secondary)
                    .frame(width: 24, height: 24)
            }

            if !isUser { Spacer(minLength: 60) }
        }
    }
}

// MARK: - Bubble Shape

struct BubbleShape: Shape {
    let isUser: Bool

    func path(in rect: CGRect) -> Path {
        let radius: CGFloat = 16
        let path = UIBezierPathHelper.bubblePath(rect: rect, radius: radius, isUser: isUser)
        return Path(path)
    }
}

enum UIBezierPathHelper {
    static func bubblePath(rect: CGRect, radius: CGFloat, isUser: Bool) -> CGPath {
        let path = CGMutablePath()
        let topLeft = isUser ? radius : 4
        let topRight = isUser ? 4 : radius
        let bottomLeft = radius
        let bottomRight = radius

        path.addRoundedRect(in: rect, cornerWidth: 0, cornerHeight: 0)

        // Use a simpler approach with NSBezierPath
        let bezierPath = NSBezierPath(roundedRect: rect,
                                       xRadius: 0,
                                       yRadius: 0)

        // Build custom rounded rect
        let cgPath = CGMutablePath()

        // Top-left
        cgPath.move(to: CGPoint(x: rect.minX + topLeft, y: rect.minY))
        // Top edge
        cgPath.addLine(to: CGPoint(x: rect.maxX - topRight, y: rect.minY))
        // Top-right corner
        cgPath.addArc(tangent1End: CGPoint(x: rect.maxX, y: rect.minY),
                      tangent2End: CGPoint(x: rect.maxX, y: rect.minY + topRight),
                      radius: topRight)
        // Right edge
        cgPath.addLine(to: CGPoint(x: rect.maxX, y: rect.maxY - bottomRight))
        // Bottom-right corner
        cgPath.addArc(tangent1End: CGPoint(x: rect.maxX, y: rect.maxY),
                      tangent2End: CGPoint(x: rect.maxX - bottomRight, y: rect.maxY),
                      radius: bottomRight)
        // Bottom edge
        cgPath.addLine(to: CGPoint(x: rect.minX + bottomLeft, y: rect.maxY))
        // Bottom-left corner
        cgPath.addArc(tangent1End: CGPoint(x: rect.minX, y: rect.maxY),
                      tangent2End: CGPoint(x: rect.minX, y: rect.maxY - bottomLeft),
                      radius: bottomLeft)
        // Left edge
        cgPath.addLine(to: CGPoint(x: rect.minX, y: rect.minY + topLeft))
        // Top-left corner
        cgPath.addArc(tangent1End: CGPoint(x: rect.minX, y: rect.minY),
                      tangent2End: CGPoint(x: rect.minX + topLeft, y: rect.minY),
                      radius: topLeft)
        cgPath.closeSubpath()

        return cgPath
    }
}

// MARK: - Typing Indicator

struct TypingIndicator: View {
    @State private var phase = 0.0

    var body: some View {
        HStack(spacing: 4) {
            ForEach(0..<3, id: \.self) { index in
                Circle()
                    .fill(Color.secondary)
                    .frame(width: 6, height: 6)
                    .offset(y: sin(phase + Double(index) * 0.8) * 4)
            }
        }
        .onAppear {
            withAnimation(.linear(duration: 1.2).repeatForever(autoreverses: false)) {
                phase = .pi * 2
            }
        }
    }
}
