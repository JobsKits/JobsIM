//
//  IMAccountUIService.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import Foundation
import JobsIMCore
import JobsIMTransport

extension IMSession {
    var savedAccountChoices: [IMJSON] {
        let bindings = (try? authentication.savedAccountBindings()) ?? []
        return bindings.compactMap { binding in
            let userID = binding["userId"].text
            guard !userID.isEmpty else {
                return nil
            }
            let value = !demo && user["id"].text == userID ? user : binding["user"]
            return value.setting("userId", .string(userID))
                .setting("id", .string(profile + ":" + userID))
        }.sorted {
            $0.firstText("displayName", "username") < $1.firstText("displayName", "username")
        }
    }

    func beginAccountSwitch() {
        guard !authentication.loggingOut, !profileMutationInProgress else {
            lastError = IMStrings.text("accountOperationBusy")
            changed?()
            return
        }
        do {
            try authentication.suspendForAccountSwitch()
        } catch {
            lastError = error.localizedDescription
            changed?()
            return
        }
        UserDefaults.standard.set(true, forKey: "accountSelectionRequired:" + profile)
        accountSelectionID = UUID()
        IMSyncCoordinator.shared.stop()
        stopAuthoritativeSync()
        IMNotifications.shared.clearAll(namespace: namespace)
        activeConversation = nil
        token = nil
        credentialPair = nil
        user = .null
        formalUser = .null
        formalToken = nil
        demo = false
        changed?()
    }

    func activateSavedAccount(id: String) async throws {
        guard !authentication.loggingOut, !profileMutationInProgress,
            let choice = savedAccountChoices.first(where: {
                $0["id"].text == id
            })
        else {
            throw SessionError.noInstance
        }
        beginAccountSwitch()
        let selection = accountSelectionID
        let expectedProfile = profile
        guard token == nil, !demo,
            UserDefaults.standard.bool(forKey: "accountSelectionRequired:" + profile)
        else {
            throw SessionError.noInstance
        }
        do {
            try authentication.selectSavedAccount(userID: choice["userId"].text)
            let expected = namespace
            let value = try await action("me", method: "GET")
            guard accountSelectionID == selection, profile == expectedProfile,
                namespace == expected, value["id"].text == choice["userId"].text
            else {
                throw SessionError.noInstance
            }
            try authentication.updateCurrentUser(value)
            UserDefaults.standard.set(false, forKey: "accountSelectionRequired:" + profile)
            lastError = nil
            changed?()
        } catch {
            if accountSelectionID == selection, profile == expectedProfile {
                token = nil
                credentialPair = nil
                user = .null
                try? authentication.suspendForAccountSwitch()
                UserDefaults.standard.set(true, forKey: "accountSelectionRequired:" + profile)
                lastError = error.localizedDescription
                changed?()
            }
            throw error
        }
    }

    func fetchUserProfile(_ userID: String) async throws -> IMJSON {
        if userID == user["id"].text {
            if demo {
                return user
            }
            let expected = namespace
            let family = currentCredentialPair?.familyID
            let value = try await action("me", method: "GET")
            guard namespace == expected, currentCredentialPair?.familyID == family,
                value["id"].text == userID
            else {
                throw SessionError.noInstance
            }
            if currentCredentialPair != nil {
                try authentication.updateCurrentUser(value)
                changed?()
            }
            return value
        }
        if demo {
            if let person = try store.list("contacts", namespace: namespace).first(where: {
                $0["id"].text == userID
            }) {
                return person
            }
            return .object([
                "id": .string(userID), "username": .string(userID), "displayName": .string(userID),
            ])
        }
        guard UUID(uuidString: userID) != nil else {
            throw SessionError.invalidInstance
        }
        return try await action("users/" + userID + "/profile", method: "GET")
    }

    func contactSnapshot() async -> [IMJSON] {
        return await contactsSnapshot()
    }

    func contactRelationship(userID: String) async throws -> IMJSON {
        if demo {
            let person = try store.list("contacts", namespace: namespace).first {
                $0["id"].text == userID
            }
            return .object([
                "peerId": .string(userID), "friend": .bool(person != nil),
                "blockedByMe": person?["blockedByMe"] ?? .bool(false),
            ])
        }
        guard UUID(uuidString: userID) != nil else {
            throw SessionError.invalidInstance
        }
        return try await action("contacts/" + userID + "/relationship", method: "GET")
    }

    func directConversation(user person: IMJSON) async throws -> IMJSON {
        let ns = namespace
        let userID = person["id"].text
        guard !userID.isEmpty else {
            throw SessionError.invalidInstance
        }
        if demo {
            let id = userID == user["id"].text ? "notes" : userID
            if let existing = try store.list("conversations", namespace: ns).first(where: {
                $0["id"].text == id
            }) {
                return existing.setting("peer", person)
            }
            let value = IMJSON.object([
                "id": .string(id), "kind": .string(userID == user["id"].text ? "self" : "direct"),
                "title": .string(person.firstText("displayName", "username")), "peer": person,
                "members": .array([user, person]),
            ])
            try store.save([value], collection: "conversations", namespace: ns)
            return value
        }
        guard UUID(uuidString: userID) != nil else {
            throw SessionError.invalidInstance
        }
        if let existing = try store.list("conversations", namespace: ns).first(where: {
            $0["kind"].text == "direct"
                && ($0["peer"]["id"].text == userID || $0["peerId"].text == userID
                    || $0["members"].list.contains {
                        $0["id"].text == userID || $0["userId"].text == userID
                    })
        }) {
            return existing.setting("peer", person)
        }
        let key = "direct:" + userID
        let pending = try store.list("conversation-open-outbox", namespace: ns).first {
            $0["id"].text == key
        }
        let operation =
            pending
            ?? .object([
                "id": .string(key), "requestId": .string(UUID().uuidString),
                "kind": .string(userID == user["id"].text ? "self" : "direct"),
                "memberIds": .array([.string(userID)]),
            ])
        try store.save([operation], collection: "conversation-open-outbox", namespace: ns)
        let value = try await action("conversations", body: operation.fields.filter {
            $0.key != "id"
        })
        guard namespace == ns, !value["id"].text.isEmpty else {
            throw SessionError.noInstance
        }
        try store.remove(key, collection: "conversation-open-outbox", namespace: ns)
        await synchronizeAuthoritatively()
        guard namespace == ns else {
            throw SessionError.noInstance
        }
        return value.setting("peer", person)
    }
}
