import Foundation
@testable import ProviderCore

func autopilotTestLoop(enabled: Bool) async throws -> ProviderLoop {
    let loop = try ProviderLoop(config: ProviderLoopConfig(
        coordinatorURL: "ws://127.0.0.1:0/unused",
        hardware: HardwareInfo(machineModel: "Mac16,5", chipName: "Apple M4 Max", chipFamily: .m4, chipTier: .max,
            memoryGb: 128, memoryAvailableGb: 124, cpuCores: CpuCores(total: 16, performance: 12, efficiency: 4),
            gpuCores: 40, memoryBandwidthGbs: 546),
        models: [], config: ProviderConfig(provider: ProviderSettings(name: "autopilot-test"), backend: BackendSettings(modelAutopilot: .init(enabled: enabled, consentRecorded:true, selectedModels:["target","uncached","other"], revision:"test")))),
        purgeLegacyFiles: false, attestationSigner: nil)
    if enabled { await loop.activateAutopilotForTesting() }
    return loop
}

extension ProviderLoop {
    func activateAutopilotForTesting() {
        autopilotControl = .init(sessionId:"session", revision:"test", enabled:true,
            expiresAtMs:Int64(Date().timeIntervalSince1970*1000)+120000)
        publishModelAutopilotSnapshot()
    }
}
