//
//  IMDemoEnvironmentService.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import Foundation
import JobsIMCore
import JobsIMTransport
import JobsSwiftDSL

@MainActor
final class IMDemoEnvironmentService {

    struct Candidate {

        let profile: String
        let username: String
        let password: String
    }

    private struct Host: Decodable {

        let controllerURL: String
        let caSha256: String
        let caCertificatePEM: String
        let demo: Bool
    }

    static let shared = IMDemoEnvironmentService()
    private(set) var currentCandidate: Candidate?
    private var operation = UUID()
    private var starting = false

    var isEnabled: Bool {
        let session = IMSession.shared
        return session.demo || matches(profile: session.profile,
                                       deployment: session.deployment,
                                       address: session.baseURL?.absoluteString ?? "")
    }

    func matches(profile: String, deployment: String, address: String) -> Bool {
        let defaults = UserDefaults.standard
        return !profile.isEmpty && !deployment.isEmpty
            && defaults.string(forKey: "demoEnvironmentProfile") == profile
            && defaults.string(forKey: "demoEnvironmentDeployment") == deployment
            && defaults.string(forKey: "demoEnvironmentURL") == address
    }

    func setEnabled(_ enabled: Bool) async throws {
        if !enabled {
            disconnect()
            return
        }
        guard !starting else {
            throw unavailable("demoStarting")
        }
        let session = IMSession.shared
        guard !session.profileMutationInProgress, !session.authentication.loggingOut,
              session.token == nil || isEnabled else {
            throw unavailable("accountOperationBusy")
        }
        let host = try bundledHost()
        guard let origin = URL(string: host.controllerURL), host.demo,
              origin.scheme == "https", origin.host != nil,
              origin.user == nil, origin.password == nil, origin.query == nil,
              origin.fragment == nil, ["", "/"].contains(origin.path) else {
            throw IMSession.SessionError.invalidInstance
        }
        let certificate = try IMInstanceCertificate(address: origin.absoluteString,
                                                     certificate: Data(host.caCertificatePEM.utf8),
                                                     verifiedFingerprint: host.caSha256)
        let api = IMAPIClient(baseURL: origin.appendingPathComponent("api/demo/v1/"),
                              certificate: certificate)
        let expectedProfile = session.profile
        let expectedSelection = session.accountSelectionID
        let expectedToken = session.token
        let expectedUserID = session.user["id"].text
        let expectedFamily = session.currentCredentialPair?.familyID
        // 等待Mac启动时可能在另一页登录，迟到的演示结果不得切走新账户。
        let contextIsCurrent = {
            session.profile == expectedProfile
                && session.accountSelectionID == expectedSelection
                && session.token == expectedToken
                && session.user["id"].text == expectedUserID
                && session.currentCredentialPair?.familyID == expectedFamily
        }
        let id = UUID()
        operation = id
        starting = true
        defer {
            if operation == id {
                starting = false
            }
        }
        var result = try await api.request("ensure-start", method: "POST")
        let deadline = ContinuousClock.now.advanced(by: .seconds(120))
        for _ in 0..<120 {
            try Task.checkCancellation()
            guard operation == id, contextIsCurrent(),
                  ContinuousClock.now < deadline else {
                throw CancellationError()
            }
            if result["state"].text == "ready" {
                break
            }
            guard result["state"].text == "starting" else {
                throw unavailable("demoHostUnavailable")
            }
            try await Task.sleep(for: .seconds(1))
            result = try await api.request("status")
        }
        if result["state"].text == "ready", result["accounts"].list.isEmpty {
            result = try await api.request("ensure-start", method: "POST")
        }
        guard result["state"].text == "ready",
              let account = result["accounts"].list.first,
              !account["username"].text.isEmpty, !account["password"].text.isEmpty,
              account["password"].text.utf8.count <= 4096,
              operation == id, contextIsCurrent() else {
            throw unavailable("demoHostUnavailable")
        }
        let connection = result["connection"]
        guard let server = URL(string: connection["serverURL"].text),
              server.scheme == "https", server.host?.lowercased() == origin.host?.lowercased(),
              server.user == nil, server.password == nil, server.query == nil,
              server.fragment == nil, ["", "/"].contains(server.path),
              UUID(uuidString: connection["deploymentId"].text) != nil else {
            throw IMSession.SessionError.invalidInstance
        }
        // 控制面已锁定的专属CA同时验证业务实例，不接受返回任意主机或新信任根。
        let serverCertificate = try IMInstanceCertificate(address: server.absoluteString,
                                                          certificate: certificate.certificateDER,
                                                          verifiedFingerprint: certificate.fingerprint)
        guard connection["caSha256"].text.lowercased().filter({ $0 != ":" && !$0.isWhitespace })
                == certificate.fingerprint else {
            throw IMSession.SessionError.invalidInstance
        }
        let backend = IMAPIClient(baseURL: IMSession.apiURL(server), certificate: serverCertificate)
        let descriptor = try await backend.request("bootstrap")
        guard operation == id, contextIsCurrent(),
              descriptor["deploymentId"].text == connection["deploymentId"].text else {
            throw IMSession.SessionError.invalidInstance
        }
        if !isEnabled {
            savePreviousConnection(session)
        }
        try IMTrustStore.shared.save(serverCertificate)
        if session.demo {
            session.beginAccountSwitch()
        }
        session.useInstance(descriptor.setting("verifiedURL", .string(server.absoluteString))
            .setting("trustFingerprint", .string(serverCertificate.fingerprint)))
        guard session.deployment == connection["deploymentId"].text, session.token == nil else {
            throw IMSession.SessionError.invalidInstance
        }
        UserDefaults.standard.set(session.profile, forKey: "demoEnvironmentProfile")
        UserDefaults.standard.set(session.deployment, forKey: "demoEnvironmentDeployment")
        UserDefaults.standard.set(server.absoluteString, forKey: "demoEnvironmentURL")
        currentCandidate = Candidate(profile: session.profile,
                                     username: account["username"].text,
                                     password: account["password"].text)
        UserDefaults.standard.set(true, forKey: "accountSelectionRequired:" + session.profile)
        session.changed?()
    }

    func disconnect() {
        let session = IMSession.shared
        guard isEnabled || starting else {
            return
        }
        operation = UUID()
        starting = false
        guard !session.profileMutationInProgress, !session.authentication.loggingOut else {
            session.lastError = IMStrings.text("accountOperationBusy")
            return
        }
        let wasDemo = isEnabled
        currentCandidate = nil
        if wasDemo {
            session.beginAccountSwitch()
            guard session.token == nil, !session.demo else {
                return
            }
            restorePreviousConnection(session)
        }
        for key in ["demoEnvironmentProfile", "demoEnvironmentDeployment", "demoEnvironmentURL"] {
            UserDefaults.standard.removeObject(forKey: key)
        }
        session.changed?()
    }

    private func bundledHost() throws -> Host {
        let url = Bundle.main.url(forResource: "demo-host", withExtension: "json", subdirectory: "DemoHost")
            ?? Bundle.main.url(forResource: "demo-host", withExtension: "json")
        guard let url, let data = try? Data(contentsOf: url), data.count <= 32_768 else {
            throw unavailable("demoHostNotConfigured")
        }
        return try JSONDecoder.make().decode(Host.self, from: data)
    }

    private func savePreviousConnection(_ session: IMSession) {
        let value = IMJSON.object([
            "address": .string(session.baseURL?.absoluteString ?? ""),
            "profile": .string(session.profile), "deployment": .string(session.deployment),
            "restoreEpoch": .string(session.restoreEpoch),
            "fingerprint": .string(UserDefaults.standard.string(forKey: "instanceTrustFingerprint") ?? "system"),
        ])
        if let bytes = try? JSONEncoder.make().encode(value) {
            UserDefaults.standard.set(bytes, forKey: "demoPreviousConnection")
        }
    }

    private func restorePreviousConnection(_ session: IMSession) {
        let defaults = UserDefaults.standard
        guard let data = defaults.data(forKey: "demoPreviousConnection"),
              let value = try? JSONDecoder.make().decode(IMJSON.self, from: data) else {
            return
        }
        defaults.removeObject(forKey: "demoPreviousConnection")
        session.baseURL = URL(string: value["address"].text)
        session.profile = value["profile"].text
        session.deployment = value["deployment"].text
        session.restoreEpoch = value["restoreEpoch"].text
        session.api = session.baseURL.map {
            IMAPIClient(baseURL: IMSession.apiURL($0),
                        certificate: IMTrustStore.shared.certificate(for: $0.absoluteString))
        }
        defaults.set(value["address"].text, forKey: "instanceURL")
        defaults.set(session.profile, forKey: "instanceProfile")
        defaults.set(session.deployment, forKey: "instanceDeployment")
        defaults.set(value["fingerprint"].text, forKey: "instanceTrustFingerprint")
        // 只恢复连接选择；正式账户必须由用户重新选择，不能退出演示即自动登入。
        defaults.set(true, forKey: "accountSelectionRequired:" + session.profile)
    }

    private func unavailable(_ key: String) -> IMAPIError {
        return IMAPIError(status: 0, code: "DEMO_UNAVAILABLE", message: IMStrings.text(key))
    }
}

extension IMSession {

    var isDemonstrationEnvironment: Bool {
        return demo || IMDemoEnvironmentService.shared.matches(profile: profile,
                                                               deployment: deployment,
                                                               address: baseURL?.absoluteString ?? "")
    }

    func leaveDemonstrationEnvironment() {
        IMDemoEnvironmentService.shared.disconnect()
    }
}
