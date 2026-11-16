//
//  IMKeychain.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import Foundation
import Security

struct IMKeychain {

    private struct KeychainError: LocalizedError {
        let status: OSStatus
        var errorDescription: String? {
            return "Keychain OSStatus \(status)"
        }
    }

    static func read(_ key: String) -> String? {
        return try? readValidated(key)
    }

    static func readValidated(_ key: String) throws -> String? {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: "com.jobs.im",
            kSecAttrAccount as String: key, kSecReturnData as String: true,
            kSecMatchLimit as String: kSecMatchLimitOne,
        ]
        var result: CFTypeRef?
        let status = SecItemCopyMatching(query as CFDictionary, &result)
        if status == errSecItemNotFound {
            return nil
        }
        guard status == errSecSuccess else {
            throw KeychainError(status: status)
        }
        guard let data = result as? Data, let value = String(data: data, encoding: .utf8) else {
            throw KeychainError(status: errSecDecode)
        }
        return value
    }

    static func write(_ value: String?, key: String) throws {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: "com.jobs.im",
            kSecAttrAccount as String: key,
        ]
        guard let value else {
            let status = SecItemDelete(query as CFDictionary)
            guard status == errSecSuccess || status == errSecItemNotFound else {
                throw KeychainError(status: status)
            }
            return
        }
        let attributes: [String: Any] = [
            kSecValueData as String: Data(value.utf8),
            kSecAttrAccessible as String: kSecAttrAccessibleWhenUnlockedThisDeviceOnly,
        ]
        let update = SecItemUpdate(query as CFDictionary, attributes as CFDictionary)
        if update == errSecItemNotFound {
            let insert = SecItemAdd(query.merging(attributes) { _, new in new } as CFDictionary, nil)
            guard insert == errSecSuccess else {
                throw KeychainError(status: insert)
            }
        } else if update != errSecSuccess {
            throw KeychainError(status: update)
        }
    }
}
