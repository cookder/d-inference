import ArgumentParser
import Foundation
import ProviderCore
#if canImport(Darwin)
import Darwin
#endif

extension Start {
    static func autopilotAnswer(_ input: String?) -> Bool {
        ["y", "yes"].contains(input?.trimmingCharacters(in: .whitespacesAndNewlines).lowercased() ?? "")
    }

    func resolveAutopilotChoice(_ config: ProviderConfig) throws -> Bool {
        if local || config.coordinator.privateOnly {
            if autopilot == true { throw ValidationError("Autopilot requires a network provider.") }
            return false
        }
        if all && (autopilot ?? config.backend.modelAutopilot.hasConsent) {
            throw ValidationError("Autopilot requires explicit model selection; use the picker or repeat --model.")
        }
        if let autopilot {
            return autopilot
        }
        let settings = config.backend.modelAutopilot
        if settings.consentRecorded { return settings.hasConsent }
        guard isatty(STDIN_FILENO) != 0, !foreground, !local, !config.coordinator.privateOnly,
              model.isEmpty, !all else { return false }
        print("Autopilot — Experimental")
        print("Darkbloom can load and unload your selected models from memory based on network demand.")
        print("Downloaded files stay on disk. You can pause or disable Autopilot at any time.")
        print("Enable Autopilot? [y/N]: ", terminator: "")
        return Self.autopilotAnswer(readLine())
    }
}

@discardableResult
func saveAutopilotEnrollment(enabled: Bool, models: [String], configPath: String?) throws -> URL {
    guard !enabled || !models.isEmpty else { throw ValidationError("Select at least one model for Autopilot.") }
    return try withMutableConfig(configPath: configPath) { path, config in
        var settings = config.backend.modelAutopilot
        settings.enabled = enabled
        settings.consentRecorded = true
        settings.paused = false
        settings.selectedModels = enabled ? Array(Set(models)).sorted() : settings.selectedModels
        settings.pinnedModels = settings.pinnedModels.filter { settings.selectedModels.contains($0) }
        settings.revision = UUID().uuidString
        config.backend.modelAutopilot = settings
        if enabled { config.backend.enabledModels = settings.selectedModels }
        try ConfigManager.save(config, to: path)
        return path
    }
}

extension Start {
    func verifyAutopilotSelection(_ ids: [String], snapshot: RuntimeSnapshot,
                                  coordinatorURL: String, runtimeCapabilities: Set<ProviderRuntimeCapability>) async throws {
        guard !ids.isEmpty else { throw ValidationError("Select at least one model for Autopilot.") }
        if !model.isEmpty && Set(ids) != Set(model) { throw ValidationError("Every requested Autopilot model must be downloaded and supported by this Mac.") }
        let client = ModelCatalogClient(coordinatorURL: coordinatorURL)
        let catalog = try await client.fetchCatalogSnapshot(typeFilter: "text", includeAliases: true)
        let byID = Dictionary(catalog.models.map { ($0.id,$0) }, uniquingKeysWith: { first,_ in first })
        let local = snapshot.hardware.map { ModelScanner.scanAllModels(hardwareInfo: $0) } ?? []
        let byLocalID = Dictionary(local.map { ($0.id,$0) }, uniquingKeysWith: { first,_ in first })
        let verifier = ModelDownloader(catalogClient: client,runtimeCapabilities: runtimeCapabilities)
        print("  Verifying selected builds before enabling Autopilot...")
        for id in ids {
            guard let entry = byID[id], entry.active != false,
                  (entry.minRamGb ?? 0) <= Int(snapshot.hardware?.memoryGb ?? 0),
                  ModelRuntimeRequirements.isEligible(modelID:id,catalogRequirements:entry.requiredProviderCapabilities,available:runtimeCapabilities),
                  let localModel = byLocalID[id],
                  Self.modelFitsBudget(sizeGb:localModel.estimatedMemoryGb,memoryGb:Double(snapshot.hardware?.memoryGb ?? 0)) else {
                throw ValidationError("Selected model is not eligible on this Mac: \(id)")
            }
            try await verifier.verifySelectedModel(entry)
        }
    }
}
