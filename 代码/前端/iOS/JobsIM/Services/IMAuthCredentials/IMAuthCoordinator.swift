//
//  IMAuthCoordinator.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import Foundation
import JobsIMCore
import JobsIMTransport

@MainActor
final class IMAuthCoordinator {

    private unowned let session: IMSession
    private var refreshTasks: [String: Task<IMCredentialPair, Error>] = [:]
    private var initialProofs: [String: IMJSON] = [:]
    private let requestOverride: ((String, [String: IMJSON], String?) async throws -> IMJSON)?
    private let secureWrite: (String?, String) throws -> Void
    var loggingOut = false

    init(
        session: IMSession,
        request: ((String, [String: IMJSON], String?) async throws -> IMJSON)? = nil,
        secureWrite: @escaping (String?, String) throws -> Void = { value, key in
            try IMKeychain.write(value, key: key)
        }
    ) {
        self.session = session
        requestOverride = request
        self.secureWrite = secureWrite
    }

    private func request(_ path: String, body: [String: IMJSON] = [:], token: String? = nil) async throws
        -> IMJSON
    {
        if let requestOverride {
            return try await requestOverride(path, body, token)
        }
        guard let api = session.api else { throw IMSession.SessionError.noInstance }
        return try await api.request(path, method: "POST", body: body, token: token)
    }

    private var metadataNamespace: IMNamespace {
        return IMNamespace(
            profile: session.profile, account: "__auth_state__", demo: false,
            restoreEpoch: session.restoreEpoch, deploymentID: session.deployment, sourceContextID: "personal")
    }

    private var protocolNamespace: IMNamespace {
        return IMNamespace(
            profile: session.profile, account: "__auth_protocol__", demo: false,
            restoreEpoch: "protocol", deploymentID: session.deployment, sourceContextID: "personal")
    }

    private func read(_ id: String, namespace: IMNamespace? = nil) throws -> IMJSON? {
        return try session.store.list("auth-state", namespace: namespace ?? metadataNamespace)
            .first { $0["id"].text == id }
    }

    private func write(_ values: [IMJSON], namespace: IMNamespace? = nil) throws {
        try session.store.save(values, collection: "auth-state", namespace: namespace ?? metadataNamespace)
    }

    private func requestRecord(_ requestID: String) throws -> IMJSON? {
        return try read("request:" + requestID)
    }

    private func pointerID(_ username: String) -> String {
        return "initial:" + username.lowercased()
    }

    func pendingInitial(_ username: String) throws -> IMJSON? {
        guard let pointer = try read(pointerID(username)),
            !["completed", "failed", "recovered"].contains(pointer["state"].text),
            let record = try requestRecord(pointer["requestId"].text)
        else { return nil }
        return record
    }

    private func publicBinding(_ pair: IMCredentialPair, user: IMJSON, state: String = "active") -> IMJSON {
        return .object([
            "id": .string("binding:" + pair.userID), "state": .string(state),
            "profile": .string(pair.profile), "deploymentId": .string(pair.deploymentID),
            "userId": .string(pair.userID), "username": .string(pair.username), "user": user,
            "deviceId": .string(pair.deviceID), "sourceContextId": .string(pair.sourceContextID),
            "restoreEpoch": .string(pair.restoreEpoch), "familyId": .string(pair.familyID),
            "refreshGeneration": .string(pair.refreshGeneration),
            "credentialRequestId": .string(pair.credentialRequestID),
        ])
    }

    private func boundNamespace(_ record: IMJSON) -> IMNamespace {
        return IMNamespace(
            profile: record["profile"].text, account: record["userId"].text, demo: false,
            restoreEpoch: record["restoreEpoch"].text, deploymentID: record["deploymentId"].text,
            sourceContextID: record["sourceContextId"].text, deviceID: record["deviceId"].text)
    }

    private func completedInitial(_ record: IMJSON) throws -> [IMJSON] {
        let requestID = record["requestId"].text
        var completed: [IMJSON] = [
            record.setting("state", .string("completed")),
            .object([
                "id": .string(pointerID(record["username"].text)),
                "requestId": .string(requestID), "state": .string("completed"),
            ]),
        ]
        let family = record["body"]["previousFamilyId"].text
        if !family.isEmpty {
            for kind in ["refresh:", "logout:"] {
                if let previous = try read(kind + family) {
                    completed.append(
                        previous.setting("state", .string("completed"))
                            .setting("resolvedByRequestId", .string(requestID)))
                }
            }
        }
        return completed
    }

    private func retirementChain(_ original: IMJSON) throws -> [String] {
        var result: [String] = []
        var visiting: Set<String> = []
        func append(_ id: String) throws {
            guard !result.contains(id) else { return }
            guard (8...128).contains(id.utf8.count), visiting.insert(id).inserted,
                let record = try requestRecord(id)
            else {
                throw IMAPIError(
                    status: 409, code: "LOCAL_AUTH_CHAIN_INCOMPLETE",
                    message: IMStrings.text("authUnknownExplanation"))
            }
            for previous in record["body"]["previousRequestIds"].list {
                try append(previous.text)
            }
            let parent = record["body"]["previousRequestId"].text
            if !parent.isEmpty { try append(parent) }
            visiting.remove(id)
            guard result.count < 64 else {
                throw IMAPIError(
                    status: 409, code: "LOCAL_AUTH_CHAIN_LIMIT",
                    message: IMStrings.text("authUnknownExplanation"))
            }
            result.append(id)
        }
        try append(original["requestId"].text)
        return result
    }

    private func install(
        _ pair: IMCredentialPair, user: IMJSON, metadata: [IMJSON],
        namespace: IMNamespace, initial: Bool
    ) throws {
        guard pair.userID == user["id"].text, pair.username == user["username"].text,
            pair.profile == session.profile, pair.deploymentID == session.deployment,
            pair.restoreEpoch == session.restoreEpoch, namespace == metadataNamespace
        else { throw reauthenticationRequired() }
        let binding = try read("binding:" + pair.userID, namespace: namespace)
        var previous: [IMNamespace] = []
        if initial {
            previous.append(
                IMNamespace(
                    profile: pair.profile, account: pair.userID, demo: false,
                    restoreEpoch: pair.restoreEpoch))
            if let current = session.currentCredentialPair, current.userID == pair.userID {
                previous.append(current.namespace)
            }
            if let binding { previous.append(boundNamespace(binding)) }
        }
        // 整对先进入同一个Keychain条目；命名空间交接与旧队列冻结原子完成后才向业务发布。
        guard try !session.accountOperationBlocked(pair) else {
            throw reauthenticationRequired()
        }
        try secureWrite(pair.serialized(), pair.profile + ":" + pair.userID)
        let installing = metadata.filter { $0["id"].text.hasPrefix(initial ? "request:" : "refresh:") }
            .map { $0.setting("state", .string("installing")) }
        try write(installing, namespace: namespace)
        try session.store.commitAuthenticationBinding(
            previousNamespaces: previous,
            newNamespace: pair.namespace,
            metadata: metadata + [
                publicBinding(pair, user: user),
                .object([
                    "id": .string("active-account"), "userId": .string(pair.userID),
                    "state": .string("active"),
                ]),
            ],
            metadataNamespace: namespace)
        session.credentialPair = pair
        session.user = user
        session.token = pair.accessToken
        UserDefaults.standard.set(false, forKey: "loggedOut:" + pair.profile + ":" + pair.userID)
        if initial {
            UserDefaults.standard.set(false, forKey: "accountSelectionRequired:" + pair.profile)
        }
        UserDefaults.standard.set(try JSONEncoder.make().encode(user), forKey: "user:" + pair.profile)
    }

    func savedAccountBindings() throws -> [IMJSON] {
        return try session.store.list("auth-state", namespace: metadataNamespace).filter {
            $0["id"].text.hasPrefix("binding:") && $0["state"].text == "active"
                && $0["profile"].text == session.profile
                && $0["deploymentId"].text == session.deployment
        }
    }

    func suspendForAccountSwitch() throws {
        for task in refreshTasks.values {
            task.cancel()
        }
        refreshTasks.removeAll()
        initialProofs.removeAll()
        guard !session.profile.isEmpty, !session.deployment.isEmpty else {
            return
        }
        try write([
            .object(["id": .string("active-account"), "state": .string("suspended")]),
            .object(["id": .string("active-auth-intent"), "state": .string("superseded-local-selection")])
        ])
    }

    func selectSavedAccount(userID: String) throws {
        guard !loggingOut,
            let binding = try savedAccountBindings().first(where: { $0["userId"].text == userID })
        else {
            throw reauthenticationRequired()
        }
        try write([.object([
            "id": .string("active-account"), "userId": .string(userID), "state": .string("active")
        ])])
        session.user = binding["user"]
        try restore(allowAccountSelection: true)
        guard session.currentCredentialPair?.userID == userID else {
            throw reauthenticationRequired()
        }
    }

    func updateCurrentUser(_ value: IMJSON) throws {
        guard let pair = session.currentCredentialPair,
            value["id"].text == pair.userID, !value["username"].text.isEmpty
        else {
            throw reauthenticationRequired()
        }
        let updated = try pair.replacingUsername(value["username"].text)
        try secureWrite(updated.serialized(), updated.profile + ":" + updated.userID)
        try write([publicBinding(updated, user: value)])
        session.credentialPair = updated
        session.token = updated.accessToken
        session.user = value
        UserDefaults.standard.set(try JSONEncoder.make().encode(value), forKey: "user:" + pair.profile)
    }

    func restore(allowAccountSelection: Bool = false) throws {
        guard !session.profile.isEmpty, !session.deployment.isEmpty else { return }
        if !allowAccountSelection,
            UserDefaults.standard.bool(forKey: "accountSelectionRequired:" + session.profile) {
            return
        }
        let namespace = metadataNamespace
        let records = try session.store.list("auth-state", namespace: namespace)
        if let pending = records.last(where: { record in
            record["id"].text.hasPrefix("request:")
                && ["securing", "installing", "unknown"].contains(record["state"].text)
                && !record["resultUser"]["id"].text.isEmpty
                && records.contains {
                    $0["id"].text == pointerID(record["username"].text)
                        && $0["requestId"] == record["requestId"] && $0["state"].text != "completed"
                }
                && records.contains {
                    $0["id"].text == "active-auth-intent" && $0["requestId"] == record["requestId"]
                }
        }),
            let secret = try IMKeychain.readValidated(
                session.profile + ":" + pending["resultUser"]["id"].text),
            let pair = try? IMCredentialPair.decode(secret),
            pair.credentialRequestID == pending["requestId"].text,
            pair.profile == session.profile, pair.deploymentID == session.deployment,
            pair.restoreEpoch == session.restoreEpoch
        {
            try install(
                pair, user: pending["resultUser"],
                metadata: try completedInitial(pending), namespace: namespace, initial: true)
            return
        }
        let candidates = records.filter {
            $0["id"].text.hasPrefix("binding:") && $0["state"].text == "active"
        }
        let active = records.first { $0["id"].text == "active-account" }
        let selected = candidates.first { $0["userId"] == (active?["userId"] ?? session.user["id"]) }
        guard active?["state"].text != "logged_out", var binding = selected,
            let secret = try IMKeychain.readValidated(session.profile + ":" + binding["userId"].text),
            let pair = try? IMCredentialPair.decode(secret), pair.profile == session.profile,
            pair.deploymentID == session.deployment, pair.restoreEpoch == session.restoreEpoch
        else { return }
        if let logout = try read("logout:" + pair.familyID), logout["state"].text != "completed" {
            throw reauthenticationRequired()
        }
        if let pending = try pendingInitial(pair.username), pending["state"].text != "completed" {
            guard pending["state"].text == "installing", pending["requestId"].text == pair.credentialRequestID
            else {
                throw reauthenticationRequired()
            }
            try install(
                pair, user: pending["resultUser"], metadata: try completedInitial(pending),
                namespace: namespace,
                initial: true)
            return
        }
        if let refresh = try read("refresh:" + pair.familyID), refresh["state"].text != "completed" {
            guard refresh["state"].text == "installing",
                refresh["requestId"].text == pair.credentialRequestID,
                let old = UInt64(refresh["refreshGeneration"].text), old < UInt64.max,
                UInt64(pair.refreshGeneration) == old + 1
            else { throw reauthenticationRequired() }
            binding = publicBinding(pair, user: binding["user"])
            try write([refresh.setting("state", .string("completed")), binding])
        }
        guard binding["userId"].text == pair.userID, binding["deviceId"].text == pair.deviceID,
            binding["sourceContextId"].text == pair.sourceContextID,
            binding["familyId"].text == pair.familyID,
            binding["refreshGeneration"].text == pair.refreshGeneration,
            binding["credentialRequestId"].text == pair.credentialRequestID
        else { throw reauthenticationRequired() }
        guard try !session.accountOperationBlocked(pair) else {
            throw reauthenticationRequired()
        }
        session.credentialPair = pair
        session.user = binding["user"]
        session.token = pair.accessToken
    }

    func preflight(_ descriptor: IMJSON, authenticating: Bool = false) async throws {
        let enabled = descriptor["features"]["auth_refresh"] == .bool(true)
        let seen = try read("protocol", namespace: protocolNamespace)?["authRefresh"] == .bool(true)
        guard enabled || !seen else {
            throw IMAPIError(
                status: 409, code: "AUTH_REFRESH_UNSUPPORTED", message: IMStrings.text("connectionNeeded"))
        }
        if enabled && !seen {
            try write(
                [.object(["id": .string("protocol"), "authRefresh": .bool(true)])],
                namespace: protocolNamespace)
        }
        guard enabled, !authenticating, session.token != nil else { return }
        guard !loggingOut else { throw reauthenticationRequired() }
        if session.currentCredentialPair == nil { try restore() }
        guard let pair = session.currentCredentialPair else {
            session.invalidateAuthentication(namespace: session.namespace, token: session.token)
            throw reauthenticationRequired()
        }
        guard try !session.accountOperationBlocked(pair) else {
            throw reauthenticationRequired()
        }
        if let task = refreshTasks[pair.namespace.key] {
            _ = try await task.value
        } else if let pending = try read("refresh:" + pair.familyID), pending["state"].text != "completed" {
            session.invalidateAuthentication(namespace: pair.namespace, token: session.token)
            throw reauthenticationRequired()
        }
        if let pending = try pendingInitial(pair.username), pending["state"].text != "completed" {
            session.invalidateAuthentication(
                namespace: pair.namespace, token: session.token,
                preserveCredentialPair: !pending["resultUser"]["id"].text.isEmpty)
            throw reauthenticationRequired()
        }
    }

    func refresh(rejectedToken: String) async throws -> String {
        guard let pair = session.currentCredentialPair else {
            throw reauthenticationRequired()
        }
        guard try !session.accountOperationBlocked(pair) else {
            throw reauthenticationRequired()
        }
        if pair.accessToken != rejectedToken { return pair.accessToken }
        let key = pair.namespace.key
        if let active = refreshTasks[key] { return try await active.value.accessToken }
        if let prior = try read("refresh:" + pair.familyID), prior["state"].text != "completed" {
            throw reauthenticationRequired()
        }
        return try await beginRefresh(pair, rejectedToken: rejectedToken)
    }

    private func beginRefresh(_ pair: IMCredentialPair, rejectedToken: String) async throws -> String {
        let key = pair.namespace.key
        if let active = refreshTasks[key] { return try await active.value.accessToken }
        let meta = metadataNamespace
        guard session.currentCredentialPair == pair else {
            throw reauthenticationRequired()
        }
        let requestID = UUID().uuidString
        let marker: IMJSON = .object([
            "id": .string("refresh:" + pair.familyID), "state": .string("inflight"),
            "requestId": .string(requestID), "familyId": .string(pair.familyID),
            "refreshGeneration": .string(pair.refreshGeneration), "userId": .string(pair.userID),
            "username": .string(pair.username), "deviceId": .string(pair.deviceID),
        ])
        try write([marker], namespace: meta)
        let task = Task<IMCredentialPair, Error> { [unowned self] in
            do {
                let result = try await self.request(
                    "auth/refresh",
                    body: [
                        "refreshToken": .string(pair.refreshToken), "requestId": .string(requestID),
                        "familyId": .string(pair.familyID),
                        "refreshGeneration": .string(pair.refreshGeneration),
                    ])
                guard self.session.namespace == pair.namespace, self.session.token == rejectedToken else {
                    throw self.reauthenticationRequired()
                }
                let next = try IMCredentialPair(
                    response: result, profile: pair.profile,
                    deploymentID: pair.deploymentID, requestID: requestID)
                try next.validateRotation(from: pair)
                try self.install(
                    next, user: result["user"], metadata: [marker.setting("state", .string("completed"))],
                    namespace: meta, initial: false)
                return next
            } catch {
                let localHandoffPending =
                    (try? self.read(marker["id"].text, namespace: meta))?["state"].text == "installing"
                if !localHandoffPending {
                    let code = (error as? IMAPIError)?.code ?? "AUTH_STORAGE_UNAVAILABLE"
                    let known = [
                        "REFRESH_REUSE_DETECTED", "REFRESH_INVALID", "REFRESH_EXPIRED",
                        "REFRESH_REAUTH_REQUIRED",
                    ].contains(code)
                    try? self.write(
                        [
                            marker.setting("state", .string(known ? "reauth_required" : "unknown"))
                                .setting("errorCode", .string(code))
                        ], namespace: meta)
                }
                self.session.invalidateAuthentication(
                    namespace: pair.namespace, token: rejectedToken,
                    preserveCredentialPair: localHandoffPending)
                throw self.reauthenticationRequired()
            }
        }
        refreshTasks[key] = task
        defer { refreshTasks.removeValue(forKey: key) }
        return try await task.value.accessToken
    }

    private static func definitelyRejectedInitial(_ error: Error) -> Bool {
        guard let response = error as? IMAPIError else {
            return false
        }
        if response.status == 403 {
            return ["MFA_REQUIRED", "MFA_INVALID", "MFA_REPLAY", "MFA_NOT_ENROLLED"]
                .contains(response.code)
        }
        if response.status == 401 {
            return response.code == "INVALID_CREDENTIALS"
        }
        if response.status == 400 {
            return ["INVALID_USERNAME", "INVALID_PASSWORD", "RESERVED_USERNAME"].contains(response.code)
        }
        return response.status == 409 && ["USERNAME_TAKEN", "USERNAME_RESERVED"].contains(response.code)
    }

    func authenticate(
        mode: String, username: String, password: String, otp: String,
        proposedRequestID: String, explicitNew: Bool, descriptor: IMJSON = .null
    ) async throws -> IMJSON {
        guard session.api != nil || requestOverride != nil else { throw IMSession.SessionError.noInstance }
        let namespace = metadataNamespace
        let old = try pendingInitial(username)
        var record: IMJSON
        if let old, !explicitNew {
            guard old["purpose"].text == mode || (mode == "login" && old["purpose"].text == "reauthenticate")
            else {
                throw IMAPIError(
                    status: 409, code: "ORIGINAL_AUTHENTICATION_REQUIRED",
                    message: IMStrings.text("authUnknownExplanation"))
            }
            record = old
        } else {
            var fields: [String: IMJSON] = [
                "username": .string(username), "requestId": .string(proposedRequestID),
                "deviceName": .string("JobsIM iOS"),
            ]
            var purpose = mode
            if let old, explicitNew {
                guard proposedRequestID != old["requestId"].text,
                    try requestRecord(proposedRequestID) == nil
                else {
                    throw IMAPIError(
                        status: 409, code: "AUTHENTICATION_ID_REUSE",
                        message: IMStrings.text("authUnknownExplanation"))
                }
                guard descriptor["features"]["auth_retirement_chain"] == .bool(true) else {
                    throw IMAPIError(
                        status: 409, code: "AUTH_RETIREMENT_CHAIN_UNSUPPORTED",
                        message: IMStrings.text("connectionNeeded"))
                }
                guard
                    old["purpose"].text != "register" || old["state"].text == "delivery_expired"
                        || !old["resultUser"]["id"].text.isEmpty
                else {
                    throw IMAPIError(
                        status: 409, code: "ORIGINAL_REGISTRATION_REQUIRED",
                        message: IMStrings.text("connectionNeeded"))
                }
                purpose = "reauthenticate"
                fields["previousRequestId"] = old["requestId"]
                fields["previousRequestIds"] = .array(try retirementChain(old).map(IMJSON.string))
                if !old["resultFamilyId"].text.isEmpty { fields["previousFamilyId"] = old["resultFamilyId"] }
            } else if mode == "login" {
                let unresolved = try session.store.list("auth-state", namespace: namespace).filter {
                    ($0["id"].text.hasPrefix("refresh:") || $0["id"].text.hasPrefix("logout:"))
                        && $0["username"].text == username.lowercased()
                        && $0["state"].text != "completed"
                }
                let binding = try session.store.list("auth-state", namespace: namespace).first {
                    $0["id"].text.hasPrefix("binding:") && $0["username"].text == username.lowercased()
                }
                let unknown = unresolved.first { $0["familyId"] == binding?["familyId"] } ?? unresolved.last
                if let unknown {
                    purpose = "reauthenticate"
                    fields["previousFamilyId"] = unknown["familyId"]
                }
            }
            guard !explicitNew || purpose == "reauthenticate" else {
                throw IMAPIError(
                    status: 409, code: "PREVIOUS_AUTHENTICATION_REQUIRED",
                    message: IMStrings.text("connectionNeeded"))
            }
            record = .object([
                "id": .string("request:" + proposedRequestID), "requestId": .string(proposedRequestID),
                "purpose": .string(purpose), "username": .string(username.lowercased()),
                "body": .object(fields),
                "otpRequired": .bool(!otp.isEmpty), "state": .string("pending"),
            ])
            try write(
                [
                    record,
                    .object([
                        "id": .string(pointerID(username)), "requestId": .string(proposedRequestID),
                        "state": .string("pending"),
                    ]),
                ], namespace: namespace)
        }
        let requestID = record["requestId"].text
        let proof: IMJSON = .object([
            "password": .string(password),
            "otp": .string(record["otpRequired"] == .bool(true) ? otp : ""),
        ])
        if let originalProof = initialProofs[requestID], originalProof != proof {
            throw IMAPIError(
                status: 409, code: "ORIGINAL_AUTHENTICATION_PROOF_CHANGED",
                message: IMStrings.text("authUnknownExplanation"))
        }
        initialProofs[requestID] = proof
        try write(
            [
                .object([
                    "id": .string("active-auth-intent"), "requestId": .string(requestID),
                    "username": .string(username.lowercased()),
                ])
            ], namespace: namespace)
        var body = record["body"].fields
        body["password"] = .string(password)
        if record["otpRequired"] == .bool(true) { body["otp"] = .string(otp) }
        do {
            let result = try await request("auth/" + record["purpose"].text, body: body)
            guard self.metadataNamespace == namespace,
                try read(pointerID(username), namespace: namespace)?["requestId"].text == requestID,
                try read("active-auth-intent", namespace: namespace)?["requestId"].text == requestID
            else {
                throw IMAPIError(
                    status: 409, code: "PREVIOUS_AUTHENTICATION_SUPERSEDED",
                    message: IMStrings.text("connectionNeeded"))
            }
            let pair = try IMCredentialPair(
                response: result, profile: session.profile,
                deploymentID: session.deployment, requestID: requestID)
            guard pair.restoreEpoch == session.restoreEpoch else { throw reauthenticationRequired() }
            record = record.setting("state", .string("securing")).setting("resultUser", result["user"])
                .setting("resultFamilyId", .string(pair.familyID))
            try write([record], namespace: namespace)
            try install(
                pair, user: result["user"],
                metadata: try completedInitial(record),
                namespace: namespace, initial: true)
            initialProofs.removeValue(forKey: requestID)
            for previous in record["body"]["previousRequestIds"].list {
                initialProofs.removeValue(forKey: previous.text)
            }
            session.changed?()
            return result
        } catch {
            if (try? read(record["id"].text, namespace: namespace))?["state"].text == "installing" {
                session.invalidateAuthentication(
                    namespace: session.namespace, token: session.token,
                    preserveCredentialPair: true)
                throw IMAPIError(
                    status: 503, code: "LOCAL_AUTH_HANDOFF_PENDING",
                    message: IMStrings.text("connectionNeeded"))
            }
            let code = (error as? IMAPIError)?.code ?? "AUTH_STORAGE_UNAVAILABLE"
            if !record["resultUser"]["id"].text.isEmpty {
                session.invalidateAuthentication(
                    namespace: session.namespace, token: session.token,
                    preserveCredentialPair: true)
            }
            // A rejected successor does not resolve its earlier unknown authentication.
            let rejected = (old == nil || explicitNew)
                && record["resultUser"]["id"].text.isEmpty
                && Self.definitelyRejectedInitial(error)
            let state = code == "DELIVERY_EXPIRED" ? "delivery_expired" : rejected ? "failed" : "unknown"
            if rejected { initialProofs.removeValue(forKey: requestID) }
            let current =
                (try? read(pointerID(username), namespace: namespace))?["requestId"].text == requestID
            var changes = [
                record.setting("state", .string(current ? state : "superseded"))
                    .setting("errorCode", .string(code))
            ]
            if current {
                let pointer = rejected ? old : nil
                changes.append(
                    .object([
                        "id": .string(pointerID(username)),
                        "requestId": pointer?["requestId"] ?? .string(requestID),
                        "state": pointer?["state"] ?? .string(state),
                    ]))
                if let pointer {
                    changes.append(.object([
                        "id": .string("active-auth-intent"), "requestId": pointer["requestId"],
                        "username": .string(username.lowercased()),
                    ]))
                }
            }
            try? write(changes, namespace: namespace)
            throw error
        }
    }

    func reauthenticationRequired() -> IMAPIError {
        return IMAPIError(
            status: 401, code: "REFRESH_REAUTH_REQUIRED", message: IMStrings.text("connectionNeeded"))
    }

    func recovered(_ username: String) throws {
        guard let pending = try pendingInitial(username) else { return }
        try write([
            pending.setting("state", .string("recovered")),
            .object([
                "id": .string(pointerID(username)), "requestId": pending["requestId"],
                "state": .string("recovered"),
            ]),
        ])
    }

    func logout(_ original: IMCredentialPair) async throws {
        guard session.api != nil || requestOverride != nil else { throw IMSession.SessionError.noInstance }
        let meta = metadataNamespace
        let marker: IMJSON = .object([
            "id": .string("logout:" + original.familyID), "state": .string("pending"),
            "familyId": .string(original.familyID), "userId": .string(original.userID),
            "username": .string(original.username),
        ])
        try write([marker], namespace: meta)
        loggingOut = true
        defer { loggingOut = false }
        var completed = false
        do {
            do {
                _ = try await request("auth/logout", token: original.accessToken)
            } catch let error as IMAPIError where error.code == "ACCESS_EXPIRED" {
                let next = try await refresh(rejectedToken: original.accessToken)
                _ = try await request("auth/logout", token: next)
            }
            completed = true
        } catch {
            session.lastError = IMStrings.text("connectionNeeded")
        }
        let binding = publicBinding(
            session.currentCredentialPair ?? original, user: session.user, state: "logged_out")
        try? write(
            [
                marker.setting("state", .string(completed ? "completed" : "unknown")), binding,
                .object([
                    "id": .string("active-account"), "userId": .string(original.userID),
                    "state": .string("logged_out"),
                ]),
            ], namespace: meta)
        IMNotifications.shared.clearAll(namespace: original.namespace)
        try? IMKeychain.write(nil, key: original.profile + ":" + original.userID)
        UserDefaults.standard.set(true, forKey: "loggedOut:" + original.profile + ":" + original.userID)
        session.token = nil
        session.credentialPair = nil
        session.user = .null
        session.changed?()
    }
}
