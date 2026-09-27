import Foundation

extension ProviderLoop {
    var autopilotSettings: ModelAutopilotSettings {
        autopilotSettingsOverride ?? loopConfig.config.backend.modelAutopilot
    }

    var autopilotConsented: Bool {
        autopilotSettings.hasConsent && !loopConfig.config.coordinator.privateOnly
    }

    var autopilotManagesResidency: Bool {
        modelAutopilotEnabled || (autopilotConsented && autopilotSettings.paused)
    }

    var autopilotOperationView: DaemonState.AutopilotOperation? {
        let last = autopilotLastCommandId.flatMap { autopilotHistory[$0]?.0 }
        guard let command = autopilotCommand ?? last else { return nil }
        return .init(reason: command.reason ?? "coordinator placement", target: command.loadModelId,
            releasing: command.unloadModelIds, elapsedMs: autopilotCommand == nil ? autopilotLastElapsedMs : nil)
    }

    var autopilotPhase: String {
        if autopilotCommand != nil { return modelAutopilotEnabled ? "transitioning" : "recovering" }
        if !autopilotConsented { return "off" }
        if autopilotSettings.paused { return "paused" }
        return modelAutopilotEnabled ? "active" : "waiting"
    }

    func autopilotAllowsModel(_ id: String) -> Bool {
        !autopilotSettings.enabled || autopilotSettings.allows(id)
    }

    /// Only the daemon started with this config path reads live policy changes.
    /// Tests and embedded loops with no path never read the user's config.
    func refreshAutopilotSettings() {
        guard let path = loopConfig.configPath,
              let config = try? ConfigManager.load(from: path) else { return }
        let settings = config.backend.modelAutopilot
        guard settings != autopilotSettings else { return }
        let previouslyManaged = autopilotManagesResidency
        autopilotSettingsOverride = settings
        autopilotControl = nil
        autopilotGeneration &+= 1
        // Disabling / pausing does not cancel an accepted mutation. Its owner
        // finishes and publishes actual capacity before ordinary changes resume.
        if previouslyManaged != autopilotManagesResidency { startIdleMonitor() }
        publishModelAutopilotSnapshot()
    }

    func handleAutopilotControl(_ control: ModelAutopilotControl) async {
        refreshAutopilotSettings()
        let now = Int64(Date().timeIntervalSince1970 * 1_000)
        guard autopilotConsented, control.revision == autopilotSettings.revision,
              !control.sessionId.isEmpty, control.sessionId.utf8.count <= 128,
              control.expiresAtMs <= now + 200_000 else { return }
        let previouslyManaged = autopilotManagesResidency
        if control.enabled && control.expiresAtMs > now && !autopilotSettings.paused {
            autopilotControl = control
        } else {
            autopilotControl = nil
        }
        autopilotGeneration &+= 1
        if previouslyManaged != autopilotManagesResidency { startIdleMonitor() }
        await updateAggregateCapacity()
        await coordinatorClient?.sendEventHeartbeat()
        writeDaemonState()
    }
}
