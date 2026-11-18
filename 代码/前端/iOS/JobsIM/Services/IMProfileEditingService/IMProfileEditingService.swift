//
//  IMProfileEditingService.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import Foundation
import JobsIMCore
import JobsIMTransport

extension IMSession {
    func updateOwnProfile(username: String,
                          displayName: String,
                          signature: String? = nil) async throws -> IMJSON {
        guard !profileMutationInProgress, !authentication.loggingOut else {
            throw IMAPIError(
                status: 409, code: "ACCOUNT_OPERATION_BUSY", message: IMStrings.text("accountOperationBusy"))
        }
        let name = username.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        let display = displayName.trimmingCharacters(in: .whitespacesAndNewlines)
        let bio = signature ?? user["signature"].text
        guard name.range(of: "^[a-z0-9_]{3,32}$", options: .regularExpression) != nil,
            !display.isEmpty, display.utf8.count <= 200, bio.unicodeScalars.count <= 200
        else {
            throw SessionError.invalidInstance
        }
        let ns = namespace
        if demo {
            user = user.setting("username", .string(name))
                .setting("displayName", .string(display)).setting("signature", .string(bio))
            try store.save([user], collection: "demo-profile", namespace: ns)
            changed?()
            return user
        }
        guard let pair = currentCredentialPair else {
            throw SessionError.noInstance
        }
        profileMutationInProgress = true
        defer {
            profileMutationInProgress = false
        }
        let collection = "profile-editing-outbox"
        let pending = try store.list(collection, namespace: ns).first {
            $0["id"].text == "own-profile"
        }
        if let pending, pending["username"].text != name || pending["displayName"].text != display
            || pending["signature"].text != bio {
            throw IMAPIError(
                status: 409, code: "PROFILE_ORIGINAL_PENDING", message: IMStrings.text("accountOperationBusy")
            )
        }
        var intent =
            pending
            ?? .object([
                "id": .string("own-profile"), "requestId": .string(UUID().uuidString),
                "username": .string(name), "expectedUsername": .string(user["username"].text),
                "displayName": .string(display), "signature": .string(bio),
                "familyId": .string(pair.familyID),
                "state": .string("prepared"),
            ])
        guard intent["familyId"].text == pair.familyID else {
            throw SessionError.noInstance
        }
        try store.save([intent], collection: collection, namespace: ns)
        func current() throws {
            guard namespace == ns, currentCredentialPair?.familyID == pair.familyID else {
                throw SessionError.noInstance
            }
        }
        if intent["expectedUsername"].text != name, intent["state"].text != "renamed" {
            do {
                let result = try await action(
                    "me/username", method: "PATCH",
                    body: [
                        "username": intent["username"], "expectedUsername": intent["expectedUsername"],
                        "requestId": intent["requestId"],
                    ])
                try current()
                guard result["requestId"] == intent["requestId"], result["user"]["id"].text == pair.userID
                else {
                    throw SessionError.invalidInstance
                }
                try authentication.updateCurrentUser(result["user"])
                if result["current"] != .bool(true) {
                    try store.remove("own-profile", collection: collection, namespace: ns)
                    changed?()
                    return user
                }
                intent = intent.setting("state", .string("renamed"))
                try store.save([intent], collection: collection, namespace: ns)
            } catch let error as IMAPIError {
                if namespace == ns,
                    error.status == 400
                        || ["USERNAME_TAKEN", "USERNAME_RESERVED", "USERNAME_CHANGED"].contains(error.code)
                {
                    try store.save(
                        [
                            intent.setting("id", intent["requestId"]).setting(
                                "failureCode", .string(error.code))
                        ], collection: "profile-editing-history", namespace: ns)
                    try store.remove("own-profile", collection: collection, namespace: ns)
                }
                throw error
            }
        }
        try current()
        if user["displayName"].text != display || user["signature"].text != bio {
            let value = try await action("me", method: "PATCH",
                                         body: ["displayName": intent["displayName"],
                                                "signature": intent["signature"]])
            try current()
            guard value["id"].text == pair.userID else {
                throw SessionError.invalidInstance
            }
            try authentication.updateCurrentUser(value)
        }
        let verified = try await action("me", method: "GET")
        try current()
        guard verified["id"].text == pair.userID else {
            throw SessionError.invalidInstance
        }
        try authentication.updateCurrentUser(verified)
        try store.remove("own-profile", collection: collection, namespace: ns)
        changed?()
        return user
    }
}
