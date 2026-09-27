import ArgumentParser
import Foundation
import ProviderCore

struct Autopilot: AsyncParsableCommand {
    static let configuration = CommandConfiguration(
        commandName: "autopilot", abstract: "Manage experimental demand-based model residency.",
        discussion: "Off by default. Select supported models during start; missing models are downloaded with your selection. Autopilot manages only those cached builds and preserves files on disk.",
        subcommands: [Status.self, Enable.self, Disable.self, Pause.self, Resume.self, Models.self, Pin.self, Unpin.self],
        defaultSubcommand: Status.self)

}
