//
//  IMNamespace.swift
//  JobsIMCore
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import CryptoKit
import Foundation

public struct IMNamespace: Hashable, Sendable {

    public let profile: String
    public let account: String
    public let demo: Bool
    public let restoreEpoch: String
    public let deploymentID: String
    public let sourceContextID: String
    public let deviceID: String

    public init(
        profile: String, account: String, demo: Bool, restoreEpoch: String = "1",
        deploymentID: String = "", sourceContextID: String = "", deviceID: String = ""
    ) {
        self.profile = profile
        self.account = account
        self.demo = demo
        self.restoreEpoch = restoreEpoch
        self.deploymentID = deploymentID
        self.sourceContextID = sourceContextID
        self.deviceID = deviceID
    }

    public var key: String {
        var components = [profile, account, demo ? "demo" : "formal", restoreEpoch]
        if !demo, !deploymentID.isEmpty || !sourceContextID.isEmpty || !deviceID.isEmpty {
            components += [deploymentID, sourceContextID, deviceID]
        }
        let raw = components.joined(separator: "\u{0}")
        return SHA256.hash(data: Data(raw.utf8)).map {
            String(format: "%02x", $0)
        }.joined()
    }
}
