//
//  IMSession.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import Foundation
import JobsIMCore
import JobsIMStorage
import JobsIMTransport

@MainActor
final class IMSession {

    static let shared = IMSession()
    let store: IMLocalStore
    var api: IMAPIClient?
    var baseURL: URL?
    var profile = ""
    var deployment = ""
    var restoreEpoch = "1"
    var user: IMJSON = .null
    var demo = false
    var token: String?
    var credentialPair: IMCredentialPair?
    lazy var authentication = IMAuthCoordinator(session: self)
    var lastError: String?
    var changed: (() -> Void)?
    var historyStatus: [String: Int] = [:]
    var activeConversation: String?
    var formalUser: IMJSON = .null
    var formalToken: String?
    private var authoritative: IMAuthoritativeSync?
    private var forwardViews: [String: IMJSON] = [:]
    private var syncObservers: [UUID: @MainActor () -> Void] = [:]
    private var selfStateBusy = Set<String>()
    private var locallyObserved = Set<String>()

    func observeSync(_ callback: @escaping @MainActor () -> Void) -> UUID {
        let id = UUID()
        syncObservers[id] = callback
        return id
    }

    func removeSyncObserver(_ id: UUID) {
        syncObservers.removeValue(forKey: id)
    }

    private func notifySync() {
        for callback in Array(syncObservers.values) {
            callback()
        }
    }

    func cachedMessages(_ conversation: String) -> [IMJSON] {
        return visibleMessages(conversation, namespace: namespace)
    }

    func synchronizeAuthoritatively() async {
        let ns = namespace
        guard !demo, currentCredentialPair != nil, token != nil else {
            return
        }
        if authoritative?.namespace != ns {
            authoritative?.cancel()
            forwardViews.removeAll()
            authoritative = IMAuthoritativeSync(
                store: store, namespace: ns,
                request: { [weak self] path, method, body in
                    guard let self else { throw SessionError.noInstance }
                    return try await self.action(path, method: method, body: body)
                },
                isCurrent: { [weak self] in
                    return self?.namespace == ns && self?.token != nil && self?.demo == false
                },
                didRestrict: { [weak self] in
                    self?.forwardViews.removeAll()
                    if let holders = try? self?.store.syncNamespaces(namespace: ns) {
                        IMMediaCache.purgeForward(namespaceKeys: holders)
                    }
                    IMNotifications.shared.clearAll(namespace: ns)
                    self?.notifySync()
                }, visibleConversation: { [weak self] in
                    return self?.activeConversation
                })
        }
        do {
            try await authoritative?.run()
            guard namespace == ns, token != nil else { return }
            let ids = try store.syncGlobalIDs(namespace: ns)
            if personalStatesEnabled,
                let preference = try store.list("self:preferences", namespace: ns).first {
                applyFontPreference(preference)
            }
            let holders = try store.syncNamespaces(namespace: ns)
            for id in ids {
                forwardViews.removeValue(forKey: ns.key + ":" + id)
                IMMediaCache.purge(messageID: id, namespaceKeys: holders)
            }
            for conversation in try store.list("conversations", namespace: ns) {
                let conversationID = conversation["id"].text
                let messages = try store.list("messages:" + conversationID, namespace: ns)
                for value in messages
                where value["contentUnavailable"] == .bool(true) || value["deleted"] == .bool(true) {
                    forwardViews.removeValue(forKey: ns.key + ":" + value["id"].text)
                    IMMediaCache.purge(messageID: value["id"].text, namespaceKeys: holders)
                }
                if activeConversation != conversationID {
                    if let muted = personalValue("conversation_view", conversation: conversationID)?["mutedUntil"],
                        let until = IMSyncProtocol.date(muted.text), until > Date() {
                        continue
                    }
                    let unread = Set(try store.list("unread:" + conversationID, namespace: ns).map { $0["id"].text })
                    for message in messages where unread.contains(message["id"].text)
                        && message["deleted"] != .bool(true) && message["contentUnavailable"] != .bool(true)
                    {
                        IMNotifications.shared.deliver(message, namespace: ns)
                    }
                }
            }
            if !ids.isEmpty {
                IMNotifications.shared.clearAll(namespaceKeys: holders)
            }
            lastError = nil
            notifySync()
        } catch {
            if namespace == ns {
                forwardViews.removeAll()
                lastError = error.localizedDescription
            }
        }
    }

    func stopAuthoritativeSync() {
        authoritative?.cancel()
        authoritative = nil
        forwardViews.removeAll()
        locallyObserved.removeAll()
    }

    func retainedConversations() -> [IMJSON] {
        guard !demo, currentCredentialPair != nil,
            let state = try? store.syncStateIfPresent(namespace: namespace),
            state["scopeId"].text == deployment, state["restoreEpoch"].text == restoreEpoch
        else { return [] }
        return (try? store.retainedConversations(namespace: namespace)) ?? []
    }

    func retainedMessages(_ conversation: String) -> [IMJSON] {
        guard !demo, currentCredentialPair != nil else { return [] }
        return (try? store.retainedMessages(conversation: conversation, namespace: namespace)) ?? []
    }

    func acceptForwardView(_ value: IMJSON, namespace expected: IMNamespace, generation: String) throws -> IMJSON {
        guard namespace == expected, token != nil,
            try store.forwardProjectionGeneration(messageID: value["id"].text, conversation: value["conversationId"].text, namespace: expected) == generation,
            let stored = try store.list("messages:" + value["conversationId"].text, namespace: expected).first(where: { $0["id"] == value["id"] }),
            stored["deleted"] != .bool(true) || value["deleted"] == .bool(true),
            (UInt64(stored["version"].text) ?? 0) <= (UInt64(value["version"].text) ?? 0)
        else { throw SessionError.noInstance }
        forwardViews[expected.key + ":" + value["id"].text] = value
        return value
    }

    func removeForwardView(_ id: String, namespace expected: IMNamespace) {
        forwardViews.removeValue(forKey: expected.key + ":" + id)
    }

    private func visibleMessages(_ id: String, namespace expected: IMNamespace) -> [IMJSON] {
        return ((try? store.list("messages:" + id, namespace: expected)) ?? []).map { value in
            guard value["deleted"] != .bool(true), let view = forwardViews[expected.key + ":" + value["id"].text],
                view["version"] == value["version"], view["forwardProjectionGeneration"] == value["forwardProjectionGeneration"]
            else { return value }
            return view
        }
    }

    var namespace: IMNamespace {
        if !demo, let pair = currentCredentialPair {
            return pair.namespace
        }
        return IMNamespace(
            profile: profile, account: user["id"].text, demo: demo, restoreEpoch: demo ? "demo" : restoreEpoch
        )
    }

    var currentCredentialPair: IMCredentialPair? {
        guard let pair = credentialPair, pair.profile == profile, pair.deploymentID == deployment,
            pair.userID == user["id"].text, pair.restoreEpoch == restoreEpoch,
            pair.accessToken == token, !demo
        else { return nil }
        return pair
    }

    private init() {
        #if DEBUG
            if ProcessInfo.processInfo.environment["JOBS_IM_SYNC_INJECTED_TESTS"] == "1" {
                do {
                    store = try IMLocalStore(path: ":memory:")
                } catch {
                    fatalError("Injected test store could not open")
                }
                // 测试host不能恢复真实账户、读取原Keychain或接触原消息库。
                return
            }
        #endif
        let directory = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
            .appendingPathComponent("JobsIM", isDirectory: true)
        do {
            try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
            store = try IMLocalStore(path: directory.appendingPathComponent("history.sqlite").path)
        } catch {
            fatalError("Local store could not open: \(error.localizedDescription)")
        }
        if let address = UserDefaults.standard.string(forKey: "instanceURL"), let url = URL(string: address) {
            baseURL = url
            api = IMAPIClient(
                baseURL: Self.apiURL(url),
                certificate: IMTrustStore.shared.certificate(for: url.absoluteString))
            profile = UserDefaults.standard.string(forKey: "instanceProfile") ?? UUID().uuidString
            deployment = UserDefaults.standard.string(forKey: "instanceDeployment") ?? ""
            restoreEpoch = UserDefaults.standard.string(forKey: "epoch:" + profile) ?? "1"
            if let data = UserDefaults.standard.data(forKey: "user:" + profile),
                let value = try? JSONDecoder.make().decode(IMJSON.self, from: data)
            {
                user = value
                if !UserDefaults.standard.bool(forKey: "loggedOut:" + profile + ":" + user["id"].text) {
                    let stored = IMKeychain.read(profile + ":" + user["id"].text)
                    if stored?.first != "{" { token = stored }
                }
            }
            do {
                try authentication.restore()
            } catch {
                token = nil
                credentialPair = nil
                lastError = IMStrings.text("connectionNeeded")
            }
        }
    }

    init(store: IMLocalStore) {
        self.store = store
    }

    static func apiURL(_ url: URL) -> URL {
        if url.path.trimmingCharacters(in: CharacterSet(charactersIn: "/")).hasSuffix("api") {
            return url.appendingPathComponent("", isDirectory: true)
        }
        return url.appendingPathComponent("api", isDirectory: true)
    }

    func connect(_ address: String) async throws -> IMJSON {
        guard let url = URL(string: address), url.user == nil, url.password == nil, url.query == nil,
            url.fragment == nil
        else {
            throw SessionError.invalidURL
        }
        var permitted = url.scheme == "https"
        #if DEBUG
            permitted =
                permitted
                || (url.scheme == "http" && ["localhost", "127.0.0.1", "::1"].contains(url.host ?? ""))
        #endif
        guard permitted else {
            throw SessionError.invalidURL
        }
        let client = IMAPIClient(
            baseURL: Self.apiURL(url), certificate: IMTrustStore.shared.certificate(for: url.absoluteString))
        let descriptor = try await client.request("bootstrap")
        let id = descriptor.firstText("deploymentId", "deployment_id", "id")
        guard !id.isEmpty else {
            throw SessionError.invalidInstance
        }
        return descriptor.setting("verifiedURL", .string(url.absoluteString)).setting(
            "trustFingerprint",
            .string(IMTrustStore.shared.certificate(for: url.absoluteString)?.fingerprint ?? "system"))
    }

    func useInstance(_ descriptor: IMJSON) {
        stopAuthoritativeSync()
        guard !demo else {
            return
        }
        let address = descriptor["verifiedURL"].text
        let id = descriptor.firstText("deploymentId", "deployment_id", "id")
        let fingerprint = descriptor["trustFingerprint"].text
        let switched =
            address != baseURL?.absoluteString || id != deployment
            || fingerprint != (UserDefaults.standard.string(forKey: "instanceTrustFingerprint") ?? "system")
        if switched {
            IMNotifications.shared.clearAll(namespace: namespace)
            token = nil
            user = .null
            credentialPair = nil
            profile = UUID().uuidString
        }
        baseURL = URL(string: address)
        deployment = id
        let epoch = descriptor["restoreEpoch"].text
        if !epoch.isEmpty {
            restoreEpoch = epoch
        }
        UserDefaults.standard.set(restoreEpoch, forKey: "epoch:" + profile)
        api = baseURL.map {
            IMAPIClient(
                baseURL: Self.apiURL($0), certificate: IMTrustStore.shared.certificate(for: $0.absoluteString)
            )
        }
        UserDefaults.standard.set(address, forKey: "instanceURL")
        UserDefaults.standard.set(profile, forKey: "instanceProfile")
        UserDefaults.standard.set(id, forKey: "instanceDeployment")
        UserDefaults.standard.set(fingerprint, forKey: "instanceTrustFingerprint")
        if switched {
            changed?()
        }
    }

    func authenticate(
        mode: String, username: String, password: String, recoveryCode: String = "", requestID: String,
        otp: String = "", explicitReauthentication: Bool = false
    ) async throws -> IMJSON {
        guard let api, !demo else {
            throw SessionError.noInstance
        }
        let descriptor = try await verifyRestoreEpoch()
        try await authentication.preflight(descriptor, authenticating: true)
        if mode != "recover", descriptor["features"]["auth_refresh"] == .bool(true) {
            return try await authentication.authenticate(
                mode: mode, username: username, password: password,
                otp: otp, proposedRequestID: requestID, explicitNew: explicitReauthentication,
                descriptor: descriptor)
        }
        var body: [String: IMJSON] = [
            "username": .string(username), "password": .string(password), "requestId": .string(requestID),
            "deviceName": .string("JobsIM iOS"),
        ]
        if !recoveryCode.isEmpty {
            body["code"] = .string(recoveryCode)
        }
        if !otp.isEmpty { body["otp"] = .string(otp) }
        let result = try await api.request("auth/" + mode, method: "POST", body: body)
        if mode == "recover", descriptor["features"]["auth_refresh"] == .bool(true) {
            try authentication.recovered(username)
        }
        let secret = result["token"].text
        if !secret.isEmpty, !result["user"]["id"].text.isEmpty {
            let value = result["user"]
            let context = try await api.request("auth/context", token: secret)
            let epoch = context["restoreEpoch"].text
            if !epoch.isEmpty {
                restoreEpoch = epoch
                UserDefaults.standard.set(epoch, forKey: "epoch:" + profile)
            }
            do {
                try IMKeychain.write(secret, key: profile + ":" + value["id"].text)
            } catch {
                _ = try? await api.request("auth/logout", method: "POST", token: secret)
                throw error
            }
            UserDefaults.standard.set(false, forKey: "loggedOut:" + profile + ":" + value["id"].text)
            token = secret
            user = value
            UserDefaults.standard.set(try JSONEncoder.make().encode(value), forKey: "user:" + profile)
            changed?()
        }
        return result
    }

    func enterDemo() {
        stopAuthoritativeSync()
        IMNotifications.shared.clearAll(namespace: namespace)
        formalUser = user
        formalToken = token
        demo = true
        user = .object([
            "id": .string("demo-jobs"), "username": .string("jobs"), "displayName": .string("Jobs"),
        ])
        token = nil
        seedDemo()
        changed?()
    }

    func exitDemo() {
        demo = false
        user = formalUser
        token = formalToken
        changed?()
    }

    func logout() async {
        if demo {
            exitDemo()
            return
        }
        if let pair = currentCredentialPair {
            IMSyncCoordinator.shared.stop()
            do { try await authentication.logout(pair) } catch { lastError = error.localizedDescription }
            return
        }
        if let api, let token {
            if (try? await verifyRestoreEpoch()) != nil {
                _ = try? await api.request("auth/logout", method: "POST", token: token)
            }
        }
        IMNotifications.shared.clearAll(namespace: namespace)
        UserDefaults.standard.set(true, forKey: "loggedOut:" + profile + ":" + user["id"].text)
        try? IMKeychain.write(nil, key: profile + ":" + user["id"].text)
        token = nil
        credentialPair = nil
        user = .null
        changed?()
    }

    func invalidateAuthentication(
        namespace expected: IMNamespace, token expectedToken: String?,
        preserveCredentialPair: Bool = false
    ) {
        guard namespace == expected, let expectedToken, token == expectedToken, !demo else { return }
        stopAuthoritativeSync()
        IMNotifications.shared.clearAll(namespace: expected)
        UserDefaults.standard.set(true, forKey: "loggedOut:" + profile + ":" + user["id"].text)
        if !preserveCredentialPair {
            try? IMKeychain.write(nil, key: profile + ":" + user["id"].text)
        }
        token = nil
        credentialPair = nil
        user = .null
        changed?()
    }

    func list(_ path: String, collection: String? = nil) async -> [IMJSON] {
        let key = collection ?? path
        let ns = namespace
        let requestToken = token
        if demo {
            return (try? store.list(key, namespace: ns)) ?? []
        }
        if key == "conversations" || key == "contacts" || key.hasPrefix("messages:") {
            await synchronizeAuthoritatively()
            guard namespace == ns else { return [] }
            return (try? store.list(key, namespace: ns)) ?? []
        }
        let syncGuard = try? store.syncStateIfPresent(namespace: ns)?["guardGeneration"]
        do {
            _ = try await verifyRestoreEpoch()
            guard namespace == ns else { return [] }
            guard api != nil, token != nil else {
                throw SessionError.noInstance
            }
            let result = try await action(path, method: "GET")
            guard namespace == ns,
                (try? store.syncStateIfPresent(namespace: ns)?["guardGeneration"]) == syncGuard
            else {
                return []
            }
            guard case .array(let items) = result["items"] else {
                throw SessionError.invalidInstance
            }
            if key.hasPrefix("messages:") {
                try store.save(items, collection: key, namespace: ns)
            } else {
                try store.replace(items, collection: key, namespace: ns)
            }
            lastError = nil
            return items
        } catch {
            if (error as? IMAPIError)?.status == 401 {
                invalidateAuthentication(namespace: ns, token: requestToken)
            }
            lastError = error.localizedDescription
            guard namespace == ns else { return [] }
            return (try? store.list(key, namespace: ns)) ?? []
        }
    }

    func conversationSnapshot() async -> [IMJSON] {
        let ns = namespace
        if !demo {
            await synchronizeAuthoritatively()
        }
        guard namespace == ns else { return [] }
        let values = (try? store.list("conversations", namespace: ns)) ?? []
        var profiles: [String: IMJSON] = [:]
        for value in values where value["kind"].text == "direct" {
            let members = await list("conversations/" + value["id"].text + "/members")
            profiles[value["id"].text] = members.first { $0["id"] != user["id"] }
        }
        for value in values {
            await IMForwardService(session: self).refreshCached(value["id"].text)
            guard namespace == ns else { return [] }
        }
        var serverCounts: [String: IMJSON] = [:]
        if personalStatesEnabled {
            for conversation in values {
                let id = conversation["id"].text
                do {
                    let read = try await action("conversations/" + id + "/read", method: "GET")
                    guard namespace == ns else {
                        return []
                    }
                    guard read["scopeId"].text == ns.deploymentID, read["userId"].text == ns.account,
                        read["sourceContextId"] == .string("personal"),
                        read["revisionEpoch"].text == ns.restoreEpoch,
                        read["conversationId"] == .string(id),
                        read["countStatus"] == .string("exact_current_source")
                    else {
                        continue
                    }
                    _ = try IMSyncProtocol.counter(read["unreadCount"])
                    _ = try IMSyncProtocol.counter(read["countSourceWatermark"])
                    serverCounts[id] = read["unreadCount"]
                } catch {
                    break
                }
            }
        }
        guard namespace == ns else {
            return []
        }
        return values.map { value in
            let id = value["id"].text
            let unread = (try? store.list("unread:" + id, namespace: namespace))?.count ?? 0
            let last = (try? store.list("messages:" + id, namespace: namespace))?.last
            var item = value.setting("unreadCount", .number(Decimal(unread)))
            if personalStatesEnabled {
                item = item.setting("unreadCount", serverCounts[id] ?? .string(unread > 0 ? "~" + String(unread) : ""))
                    .setting("countStatus", .string(serverCounts[id] == nil ? "local_estimate" : "exact_current_source"))
                if let view = personalValue("conversation_view", conversation: id) {
                    item = item.setting("personalView", view)
                    if view["manualUnread"] == .bool(true), serverCounts[id] == .string("0") || item["unreadCount"].text.isEmpty {
                        item = item.setting("unreadCount", .string("•"))
                    }
                }
            }
            if let peer = profiles[id] {
                item = item.setting("avatarURL", peer["avatarURL"]).setting(
                    "avatarThumbnailURL", peer["avatarThumbnailURL"])
                item = item.setting("title", .string(peer.firstText("displayName", "username")))
            }
            if let last {
                item = item.setting(
                    "lastMessage", last["deleted"] == .bool(true) ? .string("") : last["text"])
            }
            return item
        }
    }

    func markLocalRead(_ conversation: String) {
        if let state = try? store.syncStateIfPresent(namespace: namespace), state["schemaVersion"] == .string("2") {
            return
        }
        try? store.replace([], collection: "unread:" + conversation, namespace: namespace)
        IMNotifications.shared.clear(conversation: conversation, namespace: namespace)
    }

    var personalStatesEnabled: Bool {
        guard !demo, currentCredentialPair != nil,
            let state = try? store.syncStateIfPresent(namespace: namespace)
        else {
            return false
        }
        return state["schemaVersion"] == .string("2") && state["restriction"].text.isEmpty
    }

    func personalValue(_ kind: String, conversation: String) -> IMJSON? {
        return (try? store.list("self:" + kind, namespace: namespace))?.first {
            $0["conversationId"].text == conversation
        }
    }

    private func applyFontPreference(_ value: IMJSON) {
        let sizes = ["small": -2, "standard": 0, "large": 4]
        guard let size = sizes[value["fontPreset"].text] else {
            return
        }
        if UserDefaults.standard.integer(forKey: "fontScale") != size {
            UserDefaults.standard.set(size, forKey: "fontScale")
            changed?()
        }
    }

    func setFontPreference(_ preset: String) async throws {
        guard ["small", "standard", "large"].contains(preset), personalStatesEnabled else {
            throw SessionError.noInstance
        }
        let ns = namespace
        if let pending = try store.list("self-state-operations", namespace: ns).first(where: {
            $0["path"] == .string("me/preferences") && $0["state"] == .string("pending")
                && $0["familyId"].text == currentCredentialPair?.familyID
        }) {
            _ = try await writePersonalState(path: "me/preferences", method: "PATCH", conversation: "",
                                             body: pending["body"].fields)
        }
        guard namespace == ns else {
            throw SessionError.noInstance
        }
        let current = try await action("me/preferences", method: "GET")
        guard namespace == ns else {
            throw SessionError.noInstance
        }
        let ref: IMJSON = .object(["id": .string(ns.account), "subjectUserId": .string(ns.account), "kind": .string("preferences")])
        try IMSyncProtocol.personalBody(current, reference: ref)
        guard current["scopeId"].text == ns.deploymentID, current["revisionEpoch"].text == ns.restoreEpoch else {
            try IMSyncProtocol.refuse("SYNC_CONTEXT_CHANGED")
        }
        _ = try await writePersonalState(path: "me/preferences", method: "PATCH",
                                         conversation: "", body: ["revisionEpoch": .string(ns.restoreEpoch),
                                                                  "expectedVersion": current["version"], "fontPreset": .string(preset)])
        await synchronizeAuthoritatively()
    }

    func observeVisibleMessages(_ ids: [String], conversation: String, membershipGeneration: String) async {
        guard personalStatesEnabled, activeConversation == conversation, !ids.isEmpty,
            !selfStateBusy.contains("observe:" + conversation)
        else {
            return
        }
        let ns = namespace
        selfStateBusy.insert("observe:" + conversation)
        defer {
            selfStateBusy.remove("observe:" + conversation)
        }
        do {
            let read = try await action("conversations/" + conversation + "/read", method: "GET")
            guard namespace == ns, activeConversation == conversation,
                read["userId"].text == ns.account, read["scopeId"].text == ns.deploymentID,
                read["sourceContextId"] == .string("personal"),
                read["revisionEpoch"].text == ns.restoreEpoch,
                read["conversationId"].text == conversation, read["writable"] == .bool(true),
                read["countStatus"] == .string("exact_current_source")
            else {
                try IMSyncProtocol.refuse("SYNC_CONTEXT_CHANGED")
            }
            _ = try IMSyncProtocol.counter(read["unreadCount"])
            _ = try IMSyncProtocol.counter(read["countSourceWatermark"])
            _ = try IMSyncProtocol.counter(read["membershipGeneration"])
            try IMPersonalStateService.requireObservedMembership(read, original: membershipGeneration)
            let observationKey = ns.key + ":" + read["membershipGeneration"].text + ":"
            let unique = Array(Set(ids)).sorted().filter { !locallyObserved.contains(observationKey + $0) }
            if unique.isEmpty {
                return
            }
            guard unique.count <= 200 else {
                try IMSyncProtocol.refuse()
            }
            for id in unique {
                _ = try IMSyncProtocol.id(.string(id))
            }
            let result = try await writePersonalState(path: "conversations/" + conversation + "/read", method: "PUT",
                                             conversation: conversation,
                                             body: ["revisionEpoch": .string(ns.restoreEpoch),
                                                    "membershipGeneration": read["membershipGeneration"],
                                                    "observedMessageIds": .array(unique.map(IMJSON.string))])
            if result["current"] == .bool(true), result["result"]["membershipGeneration"] == read["membershipGeneration"] {
                for id in result["result"]["observedMessageIds"].list {
                    locallyObserved.insert(observationKey + id.text)
                }
            }
        } catch {
            if namespace == ns {
                lastError = error.localizedDescription
            }
        }
    }

    func retryPersonalState(_ path: String) async throws {
        guard personalStatesEnabled,
            let original = try store.list("self-state-operations", namespace: namespace).first(where: {
                $0["path"] == .string(path) && $0["state"] == .string("pending")
                    && $0["familyId"].text == currentCredentialPair?.familyID
            })
        else {
            return
        }
        _ = try await writePersonalState(path: path, method: original["method"].text,
                                         conversation: original["conversationId"].text,
                                         body: original["body"].fields, retryOriginal: true)
        await synchronizeAuthoritatively()
    }

    func writePersonalState(path: String, method: String, conversation: String,
                            body fields: [String: IMJSON], retryOriginal: Bool = false) async throws -> IMJSON {
        let ns = namespace
        guard personalStatesEnabled, let family = currentCredentialPair?.familyID,
            let guardState = try store.syncStateIfPresent(namespace: ns),
            !selfStateBusy.contains(path)
        else {
            throw SessionError.noInstance
        }
        selfStateBusy.insert(path)
        defer {
            selfStateBusy.remove(path)
        }
        let existing = try store.list("self-state-operations", namespace: ns).first {
            $0["path"] == .string(path) && $0["state"] == .string("pending")
                && $0["familyId"] == .string(family)
        }
        let fresh = existing == nil
        var original = existing ?? .null
        if fresh {
            guard fields["requestId"] == nil else {
                try IMSyncProtocol.refuse("SELF_ORIGINAL_OPERATION_MISSING")
            }
            var body = fields
            let id = UUID().uuidString.lowercased()
            body["requestId"] = .string(id)
            original = .object(["id": .string(id), "path": .string(path), "method": .string(method),
                                "conversationId": .string(conversation), "familyId": .string(family),
                                "body": .object(body), "state": .string("pending")])
            try store.save([original], collection: "self-state-operations", namespace: ns)
        } else {
            // New input must not silently become a replay of a different pending operation.
            guard original["body"] == .object(fields) else {
                try IMSyncProtocol.refuse("SELF_OPERATION_PENDING")
            }
        }
        guard original["path"] == .string(path), original["method"] == .string(method),
            original["conversationId"] == .string(conversation),
            original["body"]["revisionEpoch"].text == ns.restoreEpoch
        else {
            try IMSyncProtocol.refuse("SYNC_CONTEXT_CHANGED")
        }
        func current() throws {
            guard !Task.isCancelled, namespace == ns, currentCredentialPair?.familyID == family,
                personalStatesEnabled, try store.syncStateIfPresent(namespace: ns) == guardState
            else {
                try IMSyncProtocol.refuse("SYNC_CONTEXT_CHANGED")
            }
        }
        let draft = path.hasSuffix("/draft")
        let resultBody = IMPersonalStateService.resultQuery(original)
        let missing = path == "me/preferences" ? "PREFERENCE_OPERATION_NOT_FOUND"
            : draft ? "DRAFT_OPERATION_NOT_FOUND" : path.hasSuffix("/read")
            ? "READ_OPERATION_NOT_FOUND" : "VIEW_OPERATION_NOT_FOUND"
        do {
            try current()
            let result: IMJSON
            do {
                result = try await action(path + "-results", method: "POST", body: resultBody)
            } catch let error as IMAPIError where error.status == 404 && error.code == missing
                && (fresh || retryOriginal) {
                try current()
                result = try await action(path, method: method, body: original["body"].fields)
            }
            try current()
            try IMPersonalStateService.validateResult(result, original: original, namespace: ns)
            if result["status"] != .string("prepared") {
                try store.completeSelfStateOperation(namespace: ns, expectedGuard: guardState,
                                                     original: original, result: result)
            }
            return result
        } catch {
            let permanent = ["PREFERENCE_VERSION_CONFLICT", "VIEW_VERSION_CONFLICT", "MANUAL_UNREAD_CHANGED",
                             "QUALIFICATION_CHANGED", "RESTORE_EPOCH_CHANGED", "DRAFT_ADMISSION_CHANGED",
                             "DRAFT_VERSION_CONFLICT", "IDEMPOTENCY_CONFLICT"]
            if let failure = error as? IMAPIError, permanent.contains(failure.code) {
                try current()
                try store.transitionSelfStateOperation(namespace: ns, expectedGuard: guardState,
                                                       original: original, state: "rejected", reason: failure.code)
            }
            throw error
        }
    }

    func contactsSnapshot() async -> [IMJSON] {
        let ns = namespace
        if demo {
            return (try? store.list("contacts", namespace: ns)) ?? []
        }
        await synchronizeAuthoritatively()
        guard namespace == ns else { return [] }
        let rows = (try? store.list("contacts", namespace: ns)) ?? []
        // 当前公开资料只用于显示；同步friend集合仍由权威投影决定。
        guard let response = try? await action("contacts", method: "GET"), namespace == ns else { return rows }
        let profiles = Dictionary(response["items"].list.map { ($0["id"].text, $0) }, uniquingKeysWith: { first, _ in first })
        return rows.map { profiles[$0["id"].text] ?? $0 }
    }

    func action(_ path: String, method: String = "POST", body: [String: IMJSON] = [:]) async throws -> IMJSON
    {
        guard !demo else {
            return try demoAction(path, method: method, body: body)
        }
        let selfStatePath = path == "me/preferences" || path == "me/preferences-results"
            || path.hasPrefix("conversations/") && ["/read", "/read-results", "/view", "/view-results", "/draft", "/draft-results"].contains(where: { path.hasSuffix($0) })
        return try await authorized(requiredFeature: selfStatePath ? "sync_self_state" : nil,
                                    additionalFeature: path.hasSuffix("/draft") || path.hasSuffix("/draft-results") ? "cloud_drafts" : nil) { api, token in
            return try await api.request(path, method: method, body: body, token: token)
        }
    }

    func authorized<T>(requiredFeature: String? = nil, additionalFeature: String? = nil,
                       _ operation: @MainActor (IMAPIClient, String) async throws -> T) async throws -> T {
        let ns = namespace
        let descriptor = try await verifyRestoreEpoch()
        if [requiredFeature, additionalFeature].compactMap({ $0 }).contains(where: { descriptor["features"][$0] != .bool(true) }) {
            if namespace == ns {
                try? store.syncRestrict(namespace: ns, reason: "SYNC_CAPABILITY_REQUIRED")
            }
            try IMSyncProtocol.refuse("SYNC_CAPABILITY_REQUIRED")
        }
        try await authentication.preflight(descriptor)
        guard namespace == ns, let api, let requestToken = token else { throw SessionError.noInstance }
        func invoke(_ accessToken: String) async throws -> T {
            let value = try await operation(api, accessToken)
            guard namespace == ns else { throw SessionError.noInstance }
            return value
        }
        do {
            return try await invoke(requestToken)
        } catch {
            if let failure = error as? IMAPIError, failure.status == 401 {
                if failure.code == "ACCESS_EXPIRED", descriptor["features"]["auth_refresh"] == .bool(true) {
                    let next = try await authentication.refresh(rejectedToken: requestToken)
                    guard namespace == ns else { throw SessionError.noInstance }
                    do { return try await invoke(next) } catch {
                        if (error as? IMAPIError)?.status == 401 {
                            invalidateAuthentication(namespace: ns, token: next)
                        }
                        throw error
                    }
                }
                if let current = currentCredentialPair, current.accessToken != requestToken, namespace == ns {
                    do { return try await invoke(current.accessToken) } catch {
                        if (error as? IMAPIError)?.status == 401 {
                            invalidateAuthentication(namespace: ns, token: current.accessToken)
                        }
                        throw error
                    }
                }
                invalidateAuthentication(namespace: ns, token: requestToken)
            }
            throw error
        }
    }

    func pendingAuthentication(_ username: String) -> IMJSON? {
        return try? authentication.pendingInitial(username)
    }

    func send(
        _ text: String, conversation: String, type: String = "text", reply: String? = nil,
        attachment: String? = nil, admission: String? = nil, clientID: String = UUID().uuidString,
        mentionedUserIDs: [String] = [], mentionAll: Bool = false,
        didEnqueue: (() -> Void)? = nil
    ) async throws {
        let ns = namespace
        let snapshot = try admissionSnapshot(conversation, supplied: admission)
        let id = clientID
        var body: [String: IMJSON] = [
            "id": .string(id), "clientMsgId": .string(id), "conversationId": .string(conversation),
            "senderId": user["id"], "type": .string(type), "text": .string(text),
            "createdAt": .string(ISO8601DateFormatter.make().string(from: Date())),
            "status": .string("queued"),
            "restoreEpoch": .string(restoreEpoch),
        ]
        if !demo { body["sendAdmission"] = .string(snapshot) }
        if !mentionedUserIDs.isEmpty {
            body["mentionedUserIds"] = .array(mentionedUserIDs.map(IMJSON.string))
        }
        if mentionAll { body["mentionAll"] = .bool(true) }
        if let reply {
            body["replyToId"] = .string(reply)
        }
        if let attachment {
            body["attachmentId"] = .string(attachment)
        }
        try store.enqueue(.object(body), conversation: conversation, namespace: ns)
        didEnqueue?()
        if demo {
            try store.acknowledge(
                clientID: id, message: .object(body).setting("status", .string("demo")),
                conversation: conversation, namespace: ns)
            return
        }
        do {
            try await requireSendAdmissionProtocol(requireRole: mentionAll)
            guard namespace == ns else { throw SessionError.noInstance }
            let result = try await action(
                "conversations/\(conversation)/messages", body: Self.wireFields(body))
            guard result["clientMsgId"].text == id, result["conversationId"].text == conversation else {
                throw SessionError.invalidInstance
            }
            try await reconcileOriginalMessage(id)
        } catch {
            if Self.permanentSendFailure(error) {
                try store.freezeOutbox(
                    clientID: id, conversation: conversation, namespace: ns,
                    reason: "admission_or_mention_changed")
            }
            lastError = error.localizedDescription
            throw error
        }
    }

    func admissionSnapshot(_ conversation: String, supplied: String? = nil) throws -> String {
        if demo { return "" }
        let metadata = try store.list("conversations", namespace: namespace).first {
            $0["id"].text == conversation
        }
        if metadata?["readOnly"] == .bool(true) {
            throw IMAPIError(status: 403, code: "CONVERSATION_READ_ONLY", message: IMStrings.text("readOnly"))
        }
        let value = supplied.map(IMJSON.string) ?? metadata?["sendAdmission"] ?? .null
        guard case .string(let snapshot) = value, !snapshot.isEmpty else {
            throw IMAPIError(
                status: 409, code: "SEND_ADMISSION_UNAVAILABLE",
                message: IMStrings.text("sendAdmissionPaused"))
        }
        return snapshot
    }

    func requireSendAdmissionProtocol(requireRole: Bool = false) async throws {
        let descriptor = try await verifyRestoreEpoch()
        guard descriptor["features"]["send_admission"] == .bool(true) else {
            throw IMAPIError(
                status: 409, code: "SEND_ADMISSION_UNSUPPORTED",
                message: IMStrings.text("sendAdmissionPaused"))
        }
        if requireRole && descriptor["features"]["role_admission"] != .bool(true) {
            throw IMAPIError(
                status: 409, code: "ROLE_ADMISSION_UNSUPPORTED",
                message: IMStrings.text("sendAdmissionPaused"))
        }
    }

    func reconcileOriginalMessage(_ clientID: String) async throws {
        let ns = namespace
        guard !demo, let original = try store.originalOutbox(clientID: clientID, namespace: ns) else {
            throw SessionError.noInstance
        }
        _ = try await verifyRestoreEpoch()
        guard namespace == ns else { throw SessionError.noInstance }
        do {
            let conversation = original["conversationId"].text
            let result = try await action(
                "conversations/" + conversation + "/message-results", body: Self.wireFields(original.fields))
            guard namespace == ns, result["conversationId"].text == conversation,
                result["senderId"] == user["id"]
            else {
                throw SessionError.invalidInstance
            }
            try store.acknowledgeOriginal(original, message: result, namespace: ns)
        } catch {
            if (error as? IMAPIError)?.code == "SEND_ADMISSION_CHANGED" {
                try store.freezeOutbox(
                    clientID: clientID, conversation: original["conversationId"].text, namespace: ns,
                    reason: "admission_changed")
            }
            throw error
        }
    }

    static func wireFields(_ fields: [String: IMJSON]) -> [String: IMJSON] {
        if fields["type"]?.text == "forward" {
            let referenceOnly: Set<String> = [
                "clientMsgId", "type", "forwardOfId", "forwardSourceVersion",
                "forwardAdmission", "restoreEpoch", "sendAdmission",
            ]
            return fields.filter { referenceOnly.contains($0.key) }
        }
        let allowed: Set<String> = [
            "clientMsgId", "type", "text", "replyToId", "attachmentId", "forwardOfId", "restoreEpoch",
            "sendAdmission", "mentionedUserIds", "mentionAll",
            "forwardSourceVersion", "forwardAdmission",
        ]
        return fields.filter {
            allowed.contains($0.key)
        }
    }

    private static func permanentSendFailure(_ error: Error) -> Bool {
        guard let code = (error as? IMAPIError)?.code else { return false }
        return [
            "SEND_ADMISSION_CHANGED", "QUALIFICATION_CHANGED", "MENTION_FORBIDDEN",
            "MENTION_ALL_FORBIDDEN", "MENTION_MEMBER_REQUIRED", "INVALID_MENTIONS",
            "ROLE_ADMISSION_UNSUPPORTED",
        ].contains(code)
    }

    private func demoAction(_ path: String, method: String, body: [String: IMJSON]) throws -> IMJSON {
        let parts = path.split(separator: "/").map(String.init)
        if parts.first == "messages", parts.count >= 2 {
            for conversation in try store.list("conversations", namespace: namespace) {
                let collection = "messages:" + conversation["id"].text
                if var value = try store.list(collection, namespace: namespace).first(where: {
                    $0["id"].text == parts[1]
                }) {
                    if method == "PATCH" {
                        value = value.setting("text", body["text"] ?? .string(""))
                    } else if method == "DELETE" && parts.count == 2 {
                        value = value.setting("text", .string("")).setting("deleted", .bool(true))
                    } else if parts.last == "favorite" || parts.last == "pin" {
                        value = value.setting(
                            parts.last == "pin" ? "pinned" : "favorite", .bool(method != "DELETE"))
                        if parts.last == "favorite" {
                            try store.save([value], collection: "saved", namespace: namespace)
                        }
                    } else if parts.last == "reactions" {
                        value = value.setting(
                            "reactions",
                            .array(
                                value["reactions"].list + [.object(["emoji": body["emoji"] ?? .string("")])]))
                    }
                    try store.save([value], collection: collection, namespace: namespace)
                    return value
                }
            }
        }
        return .object(body)
    }

    @discardableResult
    func verifyRestoreEpoch() async throws -> IMJSON {
        #if DEBUG
            if ProcessInfo.processInfo.environment["JOBS_IM_SYNC_INJECTED_TESTS"] == "1" {
                throw IMAPIError(status: 0, code: "TEST_TRANSPORT_DISABLED", message: "Injected synchronization tests")
            }
        #endif
        guard !demo, let api else { return .null }
        let ns = namespace
        let descriptor = try await api.request("bootstrap")
        guard namespace == ns else { throw SessionError.noInstance }
        guard !deployment.isEmpty, descriptor["deploymentId"].text == deployment else {
            invalidateAuthentication(namespace: ns, token: token)
            throw IMAPIError(
                status: 409, code: "DEPLOYMENT_CHANGED", message: IMStrings.text("connectionNeeded"))
        }
        let epoch = descriptor["restoreEpoch"].text
        guard !epoch.isEmpty, UInt64(epoch) != nil else {
            throw SessionError.invalidInstance
        }
        if epoch != restoreEpoch {
            stopAuthoritativeSync()
            let oldEpoch = restoreEpoch
            restoreEpoch = epoch
            UserDefaults.standard.set(epoch, forKey: "epoch:" + profile)
            UserDefaults.standard.set(oldEpoch, forKey: "archivedEpoch:" + profile + ":" + user["id"].text)
            UserDefaults.standard.set(true, forKey: "loggedOut:" + profile + ":" + user["id"].text)
            try? IMKeychain.write(nil, key: profile + ":" + user["id"].text)
            token = nil
            credentialPair = nil
            lastError = "RESTORE_EPOCH_CHANGED"
            changed?()
            throw IMAPIError(
                status: 409, code: "RESTORE_EPOCH_CHANGED", message: IMStrings.text("connectionNeeded"))
        }
        return descriptor
    }

    func messageHistory(_ id: String) async -> [IMJSON] {
        let ns = namespace
        if demo {
            return (try? store.list("messages:" + id, namespace: ns)) ?? []
        }
        await synchronizeAuthoritatively()
        guard namespace == ns else {
            return []
        }
        await IMForwardService(session: self).refreshCached(id)
        guard namespace == ns else { return [] }
        historyStatus[ns.key + ":" + id] = lastError == nil ? 0 : -1
        return visibleMessages(id, namespace: ns)
    }

    func flushPending() async {
        guard !demo else {
            return
        }
        let ns = namespace
        for item in (try? store.allOutbox(namespace: ns)) ?? [] {
            guard namespace == ns else {
                return
            }
            do {
                try await reconcileOriginalMessage(item["clientMsgId"].text)
            } catch {
                lastError = error.localizedDescription
            }
        }
    }

    func seedDemo() {
        guard ((try? store.list("conversations", namespace: namespace)) ?? []).isEmpty else {
            return
        }
        let samples: [(String, String, String, String)] = [
            ("studio", "设计工作室", "周一见，一起把这个想法做出来。", "group"), ("lin", "林间", "那就沿着河边慢慢走吧。", "direct"),
            ("notes", "给自己的消息", "让重要的想法，有处安放。", "self"), ("team", "JobsIM 共建小组", "原生体验，从一次好好聊天开始。", "group"),
        ]
        let conversations = samples.enumerated().map { index, item in
            IMJSON.object([
                "id": .string(item.0), "title": .string(item.1), "lastMessage": .string(item.2),
                "kind": .string(item.3), "unreadCount": .number(index == 0 ? 3 : 0),
                "updatedAt": .string("2026-11-14T08:30:00Z"),
            ])
        }
        try? store.save(conversations, collection: "conversations", namespace: namespace)
        let messages: [IMJSON] = [
            .object([
                "id": .string("m1"), "senderId": .string("lin"), "text": .string("有时候，把事情说清楚，\n就已经往前走了一大步。"),
                "type": .string("text"), "createdAt": .string("2026-11-14T08:20:00Z"),
            ]),
            .object([
                "id": .string("m2"), "senderId": .string("demo-jobs"), "text": .string("是的。让消息回到人与人之间。"),
                "type": .string("text"), "status": .string("demo"),
                "createdAt": .string("2026-11-14T08:21:00Z"),
            ]),
            .object([
                "id": .string("m3"), "senderId": .string("lin"), "text": .string("那就沿着河边慢慢走吧。"),
                "type": .string("text"), "createdAt": .string("2026-11-14T08:22:00Z"),
            ]),
        ]
        for item in samples {
            try? store.save(messages, collection: "messages:" + item.0, namespace: namespace)
        }
        try? store.save(
            [
                .object(["id": .string("lin"), "displayName": .string("林间"), "username": .string("lin")]),
                .object(["id": .string("chen"), "displayName": .string("陈羽"), "username": .string("chen")]),
            ], collection: "contacts", namespace: namespace)
    }

    enum SessionError: LocalizedError {
        case invalidURL, invalidInstance, noInstance

        var errorDescription: String? {
            switch self {
            /// 仅允许可信协议与不含秘密的固定入口。
            case .invalidURL:
                return IMStrings.text("invalidURL")
            /// 不把网页可达当作实例验真。
            case .invalidInstance:
                return IMStrings.text("invalidInstance")
            /// 首连与认证不能用样例成功替代。
            case .noInstance:
                return IMStrings.text("connectionNeeded")
            }
        }
    }
}
