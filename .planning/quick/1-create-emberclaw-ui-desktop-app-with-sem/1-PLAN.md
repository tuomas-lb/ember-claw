---
phase: quick-emberclaw-ui
plan: 01
type: execute
wave: 1
depends_on: []
files_modified:
  - EmberClawUI/EmberClawUI.xcodeproj/project.pbxproj
  - EmberClawUI/EmberClawUI/EmberClawUIApp.swift
  - EmberClawUI/EmberClawUI/ContentView.swift
  - EmberClawUI/EmberClawUI/Views/SidebarView.swift
  - EmberClawUI/EmberClawUI/Views/ChatView.swift
  - EmberClawUI/EmberClawUI/Views/DeployWizardView.swift
  - EmberClawUI/EmberClawUI/Views/StatusBadge.swift
  - EmberClawUI/EmberClawUI/Models/Instance.swift
  - EmberClawUI/EmberClawUI/Services/CLIService.swift
  - EmberClawUI/EmberClawUI/Assets.xcassets/AppIcon.appiconset/Contents.json
  - EmberClawUI/EmberClawUI/Assets.xcassets/AccentColor.colorset/Contents.json
autonomous: true
requirements: []
must_haves:
  truths:
    - "App window has semi-transparent glassmorphism background with vibrancy"
    - "Sidebar lists running PicoClaw instances with status badges"
    - "Clicking an instance opens a chat interface in the main area"
    - "Deploy wizard creates new instances via eclaw deploy"
    - "Chat sends and receives messages via eclaw CLI process"
  artifacts:
    - path: "EmberClawUI/EmberClawUI.xcodeproj/project.pbxproj"
      provides: "Xcode project configuration"
    - path: "EmberClawUI/EmberClawUI/Services/CLIService.swift"
      provides: "eclaw CLI bridge via Process()"
    - path: "EmberClawUI/EmberClawUI/Views/SidebarView.swift"
      provides: "Instance list sidebar"
    - path: "EmberClawUI/EmberClawUI/Views/ChatView.swift"
      provides: "Chat interface"
  key_links:
    - from: "CLIService.swift"
      to: "bin/eclaw"
      via: "Process() shell-out"
      pattern: "Process\\(\\)"
    - from: "SidebarView.swift"
      to: "CLIService.swift"
      via: "listInstances() call"
      pattern: "listInstances"
---

<objective>
Create the EmberClawUI macOS desktop app -- a SwiftUI application with glassmorphism design that provides a graphical interface for managing PicoClaw instances via the eclaw CLI binary.

Purpose: Give users a beautiful, approachable desktop UI for deploying, monitoring, and chatting with PicoClaw AI instances instead of using the terminal.
Output: Complete Xcode project at EmberClawUI/ ready to build and run.
</objective>

<execution_context>
@/Users/tuomas/.claude/get-shit-done/workflows/execute-plan.md
@/Users/tuomas/.claude/get-shit-done/templates/summary.md
</execution_context>

<context>
@.planning/STATE.md

The eclaw CLI binary lives at bin/eclaw (relative to repo root). The app will resolve this path or use PATH lookup.

CLI output formats:
- `eclaw list` outputs a box-drawing table:
```
┌───────────┬─────────┬───────┬──────────┬───────┐
│   NAME    │ STATUS  │ READY │ RESTARTS │  AGE  │
├───────────┼─────────┼───────┼──────────┼───────┤
│ watcher-1 │ Running │ 1/1   │ 0        │ 5h31m │
└───────────┴─────────┴───────┴──────────┴───────┘
```

- `eclaw status <name>` outputs key-value lines:
```
Name:            watcher-1
Deployment:      picoclaw-watcher-1
Ready:           1/1
Pod Phase:       Running
Provider:
Model:
Age:             5h31m
```

- `eclaw deploy <name> --provider <p> --model <m> --api-key <k>` deploys a new instance
- `eclaw chat <name>` runs interactive chat (stdin/stdout piped)
- `eclaw chat <name> -m "message"` single-shot message
- `eclaw delete <name>` deletes an instance
- `eclaw restart <name>` restarts an instance
- `eclaw logs <name>` streams logs
- `eclaw models --provider <p> --api-key <k>` lists available models

Providers: openai, gemini, anthropic, groq, deepseek, openrouter

Brand logo exists at: assets/brand/emberclaw-logo.png
</context>

<tasks>

<task type="auto">
  <name>Task 1: Create Xcode project structure and CLI service layer</name>
  <files>
    EmberClawUI/EmberClawUI.xcodeproj/project.pbxproj
    EmberClawUI/EmberClawUI/EmberClawUIApp.swift
    EmberClawUI/EmberClawUI/Info.plist
    EmberClawUI/EmberClawUI/Models/Instance.swift
    EmberClawUI/EmberClawUI/Services/CLIService.swift
    EmberClawUI/EmberClawUI/Assets.xcassets/Contents.json
    EmberClawUI/EmberClawUI/Assets.xcassets/AppIcon.appiconset/Contents.json
    EmberClawUI/EmberClawUI/Assets.xcassets/AccentColor.colorset/Contents.json
    EmberClawUI/EmberClawUI/EmberClawUI.entitlements
  </files>
  <action>
    Create a new SwiftUI macOS app project at EmberClawUI/ in the repo root. Do NOT use `xcodebuild` or Xcode CLI to scaffold -- create the project files directly.

    **EmberClawUIApp.swift:**
    - macOS app entry point with @main
    - Set .windowStyle(.hiddenTitleBar) for clean glassmorphism look
    - Set minimum window size ~900x600
    - Use NSWindow delegate or .onAppear to apply NSVisualEffectView with .behindWindow material and .fullSizeContentView style mask for true transparency
    - Register a WindowGroup with ContentView

    **Models/Instance.swift:**
    - `struct Instance: Identifiable, Hashable` with fields: id (UUID), name (String), status (enum: running, error, pending, unknown), ready (String like "1/1"), restarts (Int), age (String), provider (String?), model (String?)
    - `enum InstanceStatus: String, CaseIterable` with computed color property: running=green, error=red, pending=orange, unknown=gray
    - Status icon computed property: running=checkmark.circle.fill, error=exclamationmark.triangle.fill, pending=clock.fill, unknown=questionmark.circle

    **Services/CLIService.swift:**
    - `class CLIService: ObservableObject`
    - Store eclaw binary path. First check `Bundle.main.bundlePath`/../../../bin/eclaw (for dev from repo), then fall back to finding "eclaw" on PATH via `/usr/bin/which eclaw`
    - `func listInstances() async throws -> [Instance]`: Run `eclaw list` via Process(), capture stdout, parse the box-drawing table. Split by newlines, skip header/border rows (contain ─┌┐└┘├┤┬┴┼), extract data rows by splitting on │ and trimming. Map columns: NAME, STATUS, READY, RESTARTS, AGE.
    - `func getStatus(name: String) async throws -> Instance`: Run `eclaw status <name>`, parse key-value lines to populate provider/model fields.
    - `func deployInstance(name: String, provider: String, model: String, apiKey: String) async throws`: Run `eclaw deploy <name> --provider <provider> --model <model> --api-key <apiKey>`.
    - `func deleteInstance(name: String) async throws`: Run `eclaw delete <name>`.
    - `func restartInstance(name: String) async throws`: Run `eclaw restart <name>`.
    - `func sendMessage(to instance: String, message: String) async throws -> String`: Run `eclaw chat <instance> -m "<message>"`, capture stdout as response.
    - Private helper `func runCLI(arguments: [String]) async throws -> String` that creates a Process, sets executableURL to eclaw path, pipes stdout and stderr, runs, and returns trimmed stdout. Throw descriptive errors on non-zero exit.
    - All Process calls should run on a background thread (Task.detached or similar) to avoid blocking UI.

    **Assets.xcassets:** Create proper structure with Contents.json files. Copy the emberclaw-logo.png from assets/brand/ into the asset catalog as AppIcon (or as an image set for sidebar use).

    **Entitlements:** Add com.apple.security.app-sandbox = NO (or disable sandbox) so the app can shell out to eclaw binary. Also add com.apple.security.network.client = YES.

    **project.pbxproj:** Create a valid Xcode project file targeting macOS 14.0+, Swift 5.9+, with all the source files referenced. Use a proper PBX structure. The build settings should include MACOSX_DEPLOYMENT_TARGET = 14.0, SWIFT_VERSION = 5.0, COMBINE_HIDPI_IMAGES = YES. Disable sandbox in build settings (ENABLE_APP_SANDBOX = NO).
  </action>
  <verify>
    <automated>cd /Users/tuomas/Projects/ember-claw && xcodebuild -project EmberClawUI/EmberClawUI.xcodeproj -scheme EmberClawUI -destination 'platform=macOS' build 2>&1 | tail -5</automated>
  </verify>
  <done>Xcode project builds without errors. CLIService can parse eclaw list output. Instance model correctly represents all states.</done>
</task>

<task type="auto">
  <name>Task 2: Build all SwiftUI views -- sidebar, chat, deploy wizard, glassmorphism</name>
  <files>
    EmberClawUI/EmberClawUI/ContentView.swift
    EmberClawUI/EmberClawUI/Views/SidebarView.swift
    EmberClawUI/EmberClawUI/Views/ChatView.swift
    EmberClawUI/EmberClawUI/Views/DeployWizardView.swift
    EmberClawUI/EmberClawUI/Views/StatusBadge.swift
    EmberClawUI/EmberClawUI/Views/InstanceDetailView.swift
    EmberClawUI/EmberClawUI/Views/GlassBackground.swift
    EmberClawUI/EmberClawUI/Helpers/VisualEffectView.swift
  </files>
  <action>
    Build the complete UI layer with glassmorphism styling throughout.

    **Helpers/VisualEffectView.swift:**
    - NSViewRepresentable wrapping NSVisualEffectView
    - Material: .hudWindow or .popover for frosted glass effect
    - BlendingMode: .behindWindow
    - State: .active (always vibrant)
    - Parameterize material and blending mode for reuse

    **Views/GlassBackground.swift:**
    - A reusable modifier/view that applies glassmorphism to any container
    - Semi-transparent background (white.opacity(0.15) or similar over VisualEffectView)
    - Subtle border (white.opacity(0.3), 1px)
    - cornerRadius(16) with smooth clipping
    - Soft shadow (.shadow(color: .black.opacity(0.1), radius: 10))

    **ContentView.swift:**
    - NavigationSplitView with sidebar + detail layout
    - Sidebar: SidebarView (width ~250)
    - Detail: Show ChatView when instance selected, otherwise a welcoming empty state with the EmberClaw logo and "Select an instance or deploy a new one" text
    - @StateObject var cliService = CLIService()
    - @State var selectedInstance: Instance?
    - Apply VisualEffectView as window background
    - Timer to refresh instance list every 10 seconds

    **Views/SidebarView.swift:**
    - Header: EmberClaw logo (from assets) + "EmberClaw" text in rounded, friendly font (.rounded design)
    - List of instances with ForEach, each row shows:
      - StatusBadge (small colored dot + icon)
      - Instance name in medium weight
      - Age in caption style, muted color
    - Selection binding to selectedInstance
    - "+" button at bottom to show deploy wizard sheet
    - Pull-to-refresh or manual refresh button
    - Use soft pastel background tints for list rows
    - Swipe actions: delete (red), restart (orange)
    - Context menu on right-click: Status, Restart, Logs, Delete

    **Views/StatusBadge.swift:**
    - Compact badge showing status icon + text
    - Color-coded: Running=soft green (#4ADE80), Error=soft red (#F87171), Pending=soft amber (#FBBF24), Unknown=gray
    - Pill shape with slight background tint matching status color at 15% opacity
    - SF Symbols icons: checkmark.circle.fill, exclamationmark.triangle.fill, clock.fill, questionmark.circle
    - Subtle pulse animation on "Pending" status (opacity oscillation)

    **Views/ChatView.swift:**
    - Top bar: instance name, status badge, action buttons (restart, delete with confirmation)
    - Scrollable message list with ScrollViewReader for auto-scroll to bottom
    - Messages styled as chat bubbles:
      - User messages: right-aligned, soft blue/purple tint (#818CF8 at 20% opacity), rounded corners (topLeading, bottomLeading, bottomTrailing)
      - Bot responses: left-aligned, glass-style background (white.opacity(0.1)), rounded corners (topTrailing, bottomLeading, bottomTrailing)
      - Cute small avatar icons: person.fill for user, sparkles for bot
    - Input bar at bottom: TextField with glass background, rounded corners, send button with paper plane icon
    - Send button: gradient from soft purple to soft pink, circular
    - Loading state: animated dots "..." while waiting for response
    - @State messages array of ChatMessage(id, role, content, timestamp)
    - On send: append user message, call cliService.sendMessage(), append bot response
    - Keyboard shortcut: Cmd+Enter or just Enter to send

    **Views/DeployWizardView.swift:**
    - Presented as .sheet with glass background
    - Step-by-step or single-form layout:
      - Instance name: TextField with validation (non-empty, alphanumeric + hyphens)
      - Provider: Picker with options: openai, gemini, anthropic, groq, deepseek, openrouter
      - Model: TextField (free text, user types model identifier)
      - API Key: SecureField
    - "Deploy" button: gradient matching send button style, disabled until form valid
    - Progress/loading state during deployment
    - Success: dismiss sheet, refresh instance list
    - Error: show alert with error message
    - Cancel button in toolbar

    **Views/InstanceDetailView.swift:**
    - Alternative detail view showing instance status info (provider, model, ready, restarts, age)
    - Displayed when you want full status, accessible from context menu
    - Glass card layout with key-value pairs

    **Color scheme and design tokens:**
    - Use Color extensions for the palette: .emberPurple (#818CF8), .emberPink (#F472B6), .emberGreen (#4ADE80), .emberAmber (#FBBF24), .emberRed (#F87171)
    - Font: .system(.body, design: .rounded) throughout for the cute/friendly feel
    - Spacing: generous padding (16-20pt), comfortable touch targets
    - All interactive elements should have .animation(.easeInOut(duration: 0.2)) for smooth transitions
    - Dark mode aware: glass effects work naturally, text uses .primary/.secondary
  </action>
  <verify>
    <automated>cd /Users/tuomas/Projects/ember-claw && xcodebuild -project EmberClawUI/EmberClawUI.xcodeproj -scheme EmberClawUI -destination 'platform=macOS' build 2>&1 | tail -5</automated>
  </verify>
  <done>All views build and compile. Glassmorphism background renders. Sidebar shows instance list. Chat view has message bubbles and input. Deploy wizard has form with validation. Status badges show colored icons. App has cohesive cute/fresh design with soft colors and rounded elements.</done>
</task>

</tasks>

<verification>
1. `xcodebuild -project EmberClawUI/EmberClawUI.xcodeproj -scheme EmberClawUI build` succeeds
2. App launches with transparent/vibrancy window
3. Sidebar populates with instances from `eclaw list`
4. Selecting an instance shows chat interface
5. Sending a message returns a response
6. Deploy wizard can create a new instance
</verification>

<success_criteria>
- EmberClawUI Xcode project builds and runs on macOS 14+
- Window has glassmorphism/vibrancy effect (semi-transparent background)
- Instance list refreshes and shows real data from eclaw CLI
- Chat interface sends messages and displays responses
- Deploy wizard creates instances via eclaw deploy
- Design is cute, fresh, with soft colors, rounded corners, and smooth animations
</success_criteria>

<output>
After completion, create `.planning/quick/1-create-emberclaw-ui-desktop-app-with-sem/1-SUMMARY.md`
</output>
