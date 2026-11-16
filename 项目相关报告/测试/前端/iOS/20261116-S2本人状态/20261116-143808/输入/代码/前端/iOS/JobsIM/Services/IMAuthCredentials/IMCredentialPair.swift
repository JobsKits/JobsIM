//
//  IMCredentialPair.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import Foundation
import JobsIMCore
import JobsIMTransport

struct IMCredentialPair: Codable, Equatable, Sendable {

    let profile: String
    let deploymentID: String
    let userID: String
    let username: String
    let deviceID: String
    let restoreEpoch: String
    let sourceContextID: String
    let accessToken: String
    let refreshToken: String
    let familyID: String
    let refreshGeneration: String
    let accessExpiresAt: String
    let idleExpiresAt: String
    let absoluteExpiresAt: String
    let credentialRequestID: String

    init(response: IMJSON, profile: String, deploymentID: String, requestID: String) throws {
        self.profile = profile
        self.deploymentID = deploymentID
        userID = response["user"]["id"].text
        username = response["user"]["username"].text
        deviceID = response["deviceId"].text
        restoreEpoch = response["restoreEpoch"].text
        sourceContextID = response["sourceContextId"].text
        accessToken = response["token"].text
        refreshToken = response["refreshToken"].text
        familyID = response["familyId"].text
        refreshGeneration = response["refreshGeneration"].text
        accessExpiresAt = response["accessExpiresAt"].text
        idleExpiresAt = response["idleExpiresAt"].text
        absoluteExpiresAt = response["absoluteExpiresAt"].text
        credentialRequestID = requestID
        try validate()
    }

    func validate() throws {
        guard !profile.isEmpty, UUID(uuidString: deploymentID) != nil, UUID(uuidString: userID) != nil,
            UUID(uuidString: deviceID) != nil, UUID(uuidString: familyID) != nil,
            !username.isEmpty, (8...128).contains(credentialRequestID.utf8.count),
            sourceContextID == "personal", !accessToken.isEmpty,
            !refreshToken.isEmpty, accessToken != refreshToken,
            Self.counter(restoreEpoch) != nil, Self.counter(refreshGeneration) != nil,
            let access = Self.date(accessExpiresAt), let idle = Self.date(idleExpiresAt),
            let absolute = Self.date(absoluteExpiresAt), access <= idle, idle <= absolute
        else {
            throw IMAPIError(
                status: 0, code: "INVALID_AUTH_PAIR", message: IMStrings.text("connectionNeeded"))
        }
    }

    func validateRotation(from original: IMCredentialPair) throws {
        try validate()
        guard profile == original.profile, deploymentID == original.deploymentID,
            userID == original.userID, username == original.username, deviceID == original.deviceID,
            restoreEpoch == original.restoreEpoch, sourceContextID == original.sourceContextID,
            familyID == original.familyID, let before = Self.counter(original.refreshGeneration),
            before < UInt64.max, Self.counter(refreshGeneration) == before + 1,
            Self.date(absoluteExpiresAt) == Self.date(original.absoluteExpiresAt),
            accessToken != original.accessToken, refreshToken != original.refreshToken
        else {
            throw IMAPIError(
                status: 0, code: "INVALID_REFRESH_PAIR", message: IMStrings.text("connectionNeeded"))
        }
    }

    func serialized() throws -> String {
        return String(decoding: try JSONEncoder.make().encode(self), as: UTF8.self)
    }

    static func decode(_ serialized: String) throws -> IMCredentialPair {
        do {
            let pair = try JSONDecoder.make().decode(IMCredentialPair.self, from: Data(serialized.utf8))
            try pair.validate()
            return pair
        } catch {
            throw IMAPIError(
                status: 0, code: "INVALID_AUTH_PAIR", message: IMStrings.text("connectionNeeded"))
        }
    }

    var namespace: IMNamespace {
        return IMNamespace(
            profile: profile, account: userID, demo: false, restoreEpoch: restoreEpoch,
            deploymentID: deploymentID, sourceContextID: sourceContextID, deviceID: deviceID)
    }

    private static func counter(_ value: String) -> UInt64? {
        guard let number = UInt64(value), number > 0, String(number) == value else {
            return nil
        }
        return number
    }

    private static func date(_ value: String) -> Date? {
        let fractional = ISO8601DateFormatter.make(options: [.withInternetDateTime, .withFractionalSeconds])
        return fractional.date(from: value) ?? ISO8601DateFormatter.make().date(from: value)
    }
}
