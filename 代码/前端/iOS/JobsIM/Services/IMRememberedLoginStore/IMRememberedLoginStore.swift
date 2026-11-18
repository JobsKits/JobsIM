//
//  IMRememberedLoginStore.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import Foundation
import CryptoKit
import JobsIMCore
import JobsSwiftDSL

@MainActor
final class IMRememberedLoginStore {

    struct Candidate: Codable {

        let username: String
        let password: String
        let userID: String?
    }

    private struct Record: Codable {

        var selected: String
        var candidates: [String: Candidate]
    }

    static let shared = IMRememberedLoginStore()

    func restore(for profile: String, username: String? = nil) throws -> Candidate? {
        guard !profile.isEmpty, let record = try read(profile) else {
            return nil
        }
        let name = username.map(normalize) ?? record.selected
        return record.candidates[name]
    }

    func persistAfterVerifiedAuthentication(for profile: String,
                                            username: String,
                                            password: String,
                                            enabled: Bool) throws {
        let name = normalize(username)
        guard enabled else {
            try forget(for: profile, username: name)
            return
        }
        let session = IMSession.shared
        guard session.profile == profile, !session.isDemonstrationEnvironment,
              let pair = session.currentCredentialPair,
              pair.username == name, session.user["username"].text == name,
              !password.isEmpty, password.utf8.count <= 4096 else {
            throw IMSession.SessionError.noInstance
        }
        var record = try read(profile) ?? Record(selected: name, candidates: [:])
        record.selected = name
        record.candidates[name] = Candidate(username: name, password: password, userID: pair.userID)
        // 用户明确勾选的密码和账户索引一并存入本机解锁后可读的Keychain。
        try write(record, profile: profile)
    }

    func forget(for profile: String, username: String? = nil) throws {
        guard !profile.isEmpty else {
            return
        }
        guard let username else {
            try IMKeychain.write(nil, key: key(profile))
            return
        }
        guard var record = try read(profile) else {
            return
        }
        let name = normalize(username)
        record.candidates.removeValue(forKey: name)
        if record.candidates.isEmpty {
            try IMKeychain.write(nil, key: key(profile))
            return
        }
        if record.selected == name {
            record.selected = record.candidates.keys.sorted().first ?? ""
        }
        try write(record, profile: profile)
    }

    func forget(for profile: String, userID: String, legacyUsername: String) throws {
        guard !profile.isEmpty, UUID(uuidString: userID) != nil else {
            throw IMSession.SessionError.invalidInstance
        }
        guard var record = try read(profile) else {
            return
        }
        let name = normalize(legacyUsername)
        record.candidates = record.candidates.filter { key, candidate in
            if let owner = candidate.userID {
                return owner != userID
            }
            // 旧记录没有UID，只能按已核验注销原单的用户名匹配，不能猜测其它账户归属。
            return name.isEmpty || key != name || normalize(candidate.username) != name
        }
        if record.candidates.isEmpty {
            try IMKeychain.write(nil, key: key(profile))
            return
        }
        if record.candidates[record.selected] == nil {
            record.selected = record.candidates.keys.sorted().first ?? ""
        }
        try write(record, profile: profile)
    }

    private func normalize(_ value: String) -> String {
        return value.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
    }

    private func key(_ profile: String) -> String {
        let digest = SHA256.hash(data: Data(profile.utf8)).map {
            String(format: "%02x", $0)
        }.joined()
        return "remembered-login-v1:" + digest
    }

    private func read(_ profile: String) throws -> Record? {
        guard let value = try IMKeychain.readValidated(key(profile)) else {
            return nil
        }
        return try JSONDecoder.make().decode(Record.self, from: Data(value.utf8))
    }

    private func write(_ record: Record, profile: String) throws {
        let bytes = try JSONEncoder.make().encode(record)
        try IMKeychain.write(String(decoding: bytes, as: UTF8.self), key: key(profile))
    }
}
