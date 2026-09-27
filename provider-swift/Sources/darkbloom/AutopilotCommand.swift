import ArgumentParser
import Foundation
import ProviderCore

struct Autopilot: AsyncParsableCommand {
    static let configuration = CommandConfiguration(
        commandName: "autopilot", abstract: "Manage experimental demand-based model residency.",
        discussion: "Off by default. Select supported models during start; missing models are downloaded with your selection. Autopilot manages only those cached builds and preserves files on disk.",
        subcommands: [Status.self, Enable.self, Disable.self, Pause.self, Resume.self, Models.self, Pin.self, Unpin.self],
        defaultSubcommand: Status.self)

    struct Status: AsyncParsableCommand {
        static let configuration = CommandConfiguration(abstract: "Show configured consent and live Autopilot state.")
        @OptionGroup var configOptions: ConfigOptions
        @Flag(help: "Emit JSON.") var json = false
        mutating func run() async throws {
            let runtime = try loadRuntimeSnapshot(configOptions: configOptions)
            let settings = runtime.config.backend.modelAutopilot
            let live = DaemonStateFile.read()
            let fresh = live.map { daemonProcessAlive(pid: $0.pid) && Date().timeIntervalSince1970 - $0.writtenAt < 30 } ?? false
            if json {
                try printJSON(AutopilotStatusOutput(configured: settings, live: fresh ? live?.autopilot : nil,
                    phase: fresh ? live?.autopilotPhase : nil, operation: fresh ? live?.autopilotOperation : nil))
                return
            }
            print("Autopilot — Experimental: \(settings.hasConsent ? "enabled" : "off")")
            print("  Live state: \(fresh ? (live?.autopilotPhase ?? "unknown") : "daemon not reporting")")
            if fresh && live?.autopilot?.revision != settings.revision { print("  Configuration change waiting to apply") }
            print("  Selected: \(settings.selectedModels.joined(separator: ", "))")
            print("  Pinned: \(settings.pinnedModels.isEmpty ? "none" : settings.pinnedModels.joined(separator: ", "))")
            if fresh, let state = live?.autopilot {
                print("  Loaded in memory: \(state.residentModels.map(\.modelId).joined(separator: ", "))")
                if let command = state.activeCommandId { print("  Transition: \(command)") }
                if let result = state.lastCommandStatus { print("  Last transition: \(result.rawValue)") }
            }
            if fresh, let operation = live?.autopilotOperation {
                print("  Reason: \(operation.reason.replacingOccurrences(of: "_", with: " "))")
                if let target = operation.target { print("  Target: \(target)") }
                if !operation.releasing.isEmpty { print("  Release: \(operation.releasing.joined(separator: ", "))") }
                if let elapsed = operation.elapsedMs { print("  Last transition: \(String(format: "%.1f", Double(elapsed)/1000)) s") }
            }
            print("  Pause/resume: darkbloom autopilot pause | resume")
            print("  Change selection: darkbloom autopilot models")
        }
    }

    struct Enable: AsyncParsableCommand {
        static let configuration = CommandConfiguration(abstract: "Choose models and start experimental Autopilot.")
        @OptionGroup var configOptions: ConfigOptions
        mutating func run() async throws {
            var start = try Start.parse(["--autopilot"]); start.configOptions = configOptions
            try await start.run()
        }
    }
    struct Models: AsyncParsableCommand {
        static let configuration = CommandConfiguration(abstract: "Choose the Autopilot model set; safely restart after downloads finish.")
        @OptionGroup var configOptions: ConfigOptions
        mutating func run() async throws {
            var start = try Start.parse(["--autopilot"]); start.configOptions = configOptions
            try await start.run()
        }
    }
    struct Disable: AsyncParsableCommand {
        static let configuration = CommandConfiguration(abstract: "Disable new Autopilot changes and restore the saved idle policy.")
        @OptionGroup var configOptions: ConfigOptions
        mutating func run() async throws {
            try setModelAutopilot(enabled: false, configPath: configOptions.config)
            print("Autopilot disabled. The running provider applies this at its next policy refresh; an accepted transition finishes safely.")
        }
    }
    struct Pause: AsyncParsableCommand {
        static let configuration = CommandConfiguration(abstract: "Pause new automatic residency changes.")
        @OptionGroup var configOptions: ConfigOptions
        mutating func run() async throws { try changeAutopilotPolicy(configPath: configOptions.config) { $0.paused = true }; print("Autopilot pause requested.") }
    }
    struct Resume: AsyncParsableCommand {
        static let configuration = CommandConfiguration(abstract: "Resume an existing Autopilot enrollment.")
        @OptionGroup var configOptions: ConfigOptions
        mutating func run() async throws { try changeAutopilotPolicy(configPath: configOptions.config) { $0.paused = false }; print("Autopilot resume requested.") }
    }
    struct Pin: AsyncParsableCommand {
        static let configuration = CommandConfiguration(abstract: "Protect selected models from automatic unloading.")
        @OptionGroup var configOptions: ConfigOptions
        @Argument var models: [String]
        mutating func run() async throws {
            try changeAutopilotPolicy(configPath: configOptions.config) { settings in
                guard Set(models).isSubset(of: Set(settings.selectedModels)) else { throw ValidationError("Pins must be selected Autopilot models.") }
                settings.pinnedModels = Array(Set(settings.pinnedModels + models)).sorted()
            }
            print("Pins updated.")
        }
    }
    struct Unpin: AsyncParsableCommand {
        static let configuration = CommandConfiguration(abstract: "Remove automatic-unload protection from selected models.")
        @OptionGroup var configOptions: ConfigOptions
        @Argument var models: [String]
        mutating func run() async throws {
            try changeAutopilotPolicy(configPath: configOptions.config) { $0.pinnedModels.removeAll { models.contains($0) } }
            print("Pins updated.")
        }
    }
}

private struct AutopilotStatusOutput: Encodable {
    let configured: ModelAutopilotSettings
    let live: ModelAutopilotSnapshot?
    let phase: String?
    let operation: DaemonState.AutopilotOperation?
}

func changeAutopilotPolicy(configPath: String?, change: (inout ModelAutopilotSettings) throws -> Void) throws {
    try withMutableConfig(configPath: configPath) { path, config in
        guard config.backend.modelAutopilot.hasConsent else { throw ValidationError("Enable Autopilot and select models first: darkbloom autopilot enable") }
        try change(&config.backend.modelAutopilot)
        config.backend.modelAutopilot.revision = UUID().uuidString
        try ConfigManager.save(config, to: path)
    }
}

@discardableResult
func setModelAutopilot(enabled: Bool, dwell: UInt64? = nil, pins: [String]? = nil,
                       configPath: String?) throws -> URL {
    if let dwell, !(60...86_400).contains(dwell) { throw ValidationError("min-dwell-seconds must be between 60 and 86400") }
    return try withMutableConfig(configPath: configPath) { path, config in
        var settings = config.backend.modelAutopilot
        if enabled && settings.selectedModels.isEmpty { settings.selectedModels = config.backend.enabledModels }
        guard !enabled || !settings.selectedModels.isEmpty else { throw ValidationError("Select models with darkbloom autopilot enable first.") }
        if let pins {
            guard Set(pins).isSubset(of: Set(settings.selectedModels)) else { throw ValidationError("Pins must be selected models.") }
            settings.pinnedModels = Array(Set(pins)).sorted()
        }
        settings.enabled = enabled; settings.consentRecorded = true; settings.paused = false
        settings.revision = UUID().uuidString
        if let dwell { settings.minDwellSeconds = dwell }
        config.backend.modelAutopilot = settings
        try ConfigManager.save(config, to: path)
        return path
    }
}
