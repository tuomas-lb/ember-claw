import SwiftUI

struct ContentView: View {
    @StateObject private var cliService = CLIService()
    @State private var selectedInstance: Instance?

    var body: some View {
        Text("EmberClaw UI")
            .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
}
