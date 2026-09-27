import ArgumentParser
import Foundation
import Testing
@testable import darkbloom
import ProviderCore

@Suite("ModelAutopilot CLI")
struct AutopilotCommandTests {
    @Test func enrollmentPersistsWithoutChangingLegacyModelOrIdlePolicy() throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: directory) }
        let path = directory.appendingPathComponent("provider.toml")
        var config = ProviderConfig(provider: ProviderSettings(name: "autopilot-test"))
        config.backend.enabledModels = ["a", "b"]
        config.backend.idleTimeoutMins = 17
        try ConfigManager.save(config, to: path)
        try setModelAutopilot(enabled: true, dwell: 600, pins: ["b", "b"], configPath: path.path)
        let enabled = try ConfigManager.load(from: path)
        #expect(enabled.backend.modelAutopilot.enabled)
        #expect(enabled.backend.modelAutopilot.minDwellSeconds == 600)
        #expect(enabled.backend.modelAutopilot.pinnedModels == ["b"])
        #expect(enabled.backend.enabledModels == ["a", "b"])
        #expect(enabled.backend.idleTimeoutMins == 17)
        try setModelAutopilot(enabled: false, configPath: path.path)
        let disabled = try ConfigManager.load(from: path)
        #expect(!disabled.backend.modelAutopilot.enabled)
        #expect(disabled.backend.modelAutopilot.pinnedModels == ["b"])
    }
}

extension AutopilotCommandTests {
    @Test func startupConsentRequiresExplicitYes() {
        #expect(!Start.autopilotAnswer(nil))
        #expect(!Start.autopilotAnswer(""))
        #expect(!Start.autopilotAnswer("n"))
        #expect(!Start.autopilotAnswer("maybe"))
        #expect(Start.autopilotAnswer(" Y "))
        #expect(Start.autopilotAnswer("yes"))
    }
    @Test func emptySelectionDoesNotEnrollOrRewriteConfig() throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        try FileManager.default.createDirectory(at:directory,withIntermediateDirectories:true)
        defer { try? FileManager.default.removeItem(at:directory) }
        let path = directory.appendingPathComponent("provider.toml")
        let config = ProviderConfig(provider:ProviderSettings(name:"enrollment"))
        try ConfigManager.save(config,to:path)
        let before = try Data(contentsOf:path)
        #expect(throws: (any Error).self) { try saveAutopilotEnrollment(enabled:true,models:[],configPath:path.path) }
        #expect(try Data(contentsOf:path) == before)
        try saveAutopilotEnrollment(enabled:true,models:["chosen"],configPath:path.path)
        let enrolled = try ConfigManager.load(from:path)
        #expect(enrolled.backend.modelAutopilot.hasConsent)
        #expect(enrolled.backend.modelAutopilot.allows("chosen"))
        #expect(!enrolled.backend.modelAutopilot.allows("other-downloaded-model"))
        try changeAutopilotPolicy(configPath:path.path) { $0.paused = true }
        let paused = try ConfigManager.load(from:path)
        #expect(paused.backend.modelAutopilot.paused)
        #expect(paused.backend.modelAutopilot.revision != enrolled.backend.modelAutopilot.revision)
    }
}

extension AutopilotCommandTests {
    @Test func savedEnrollmentCannotSilentlyExpandThroughAll() throws {
        var config = ProviderConfig(provider:ProviderSettings(name:"choice"))
        config.backend.modelAutopilot = .init(enabled:true,consentRecorded:true,selectedModels:["chosen"],revision:"selection")
        var start = try Start.parse(["--all"])
        #expect(throws:(any Error).self) { try start.resolveAutopilotChoice(config) }
        start.autopilot = false
        #expect(try start.resolveAutopilotChoice(config) == false)
        start.all = false; start.autopilot = nil; start.local = true
        #expect(try start.resolveAutopilotChoice(config) == false)
        start.autopilot = true
        #expect(throws:(any Error).self) { try start.resolveAutopilotChoice(config) }
    }

    @Test func statusFreshnessFollowsConfiguredHeartbeatCadence() {
        let state = DaemonState(pid: 1, version: "test", writtenAt: 1_000, startedAt: 900)
        #expect(Autopilot.Status.snapshotIsFresh(state, heartbeatIntervalSecs: 60, now: 1_030.5))
        #expect(!Autopilot.Status.snapshotIsFresh(state, heartbeatIntervalSecs: 60, now: 1_121))
        #expect(Autopilot.Status.snapshotIsFresh(state, heartbeatIntervalSecs: 200, now: 1_100.5))
        #expect(!Autopilot.Status.snapshotIsFresh(state, heartbeatIntervalSecs: 200, now: 1_401))
        #expect(!Autopilot.Status.snapshotIsFresh(state, heartbeatIntervalSecs: 5, now: 1_011))
    }
}
