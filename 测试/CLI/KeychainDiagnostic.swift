//
//  KeychainDiagnostic.swift
//  JobsIMCLITests
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import Foundation
import Security

@main
private enum KeychainDiagnostic {
    static func main() {
        let account = UUID().uuidString
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: "JobsIM.CLI.IndependentDiagnostic",
            kSecAttrAccount as String: account,
            kSecUseDataProtectionKeychain as String: true,
            kSecAttrSynchronizable as String: false,
        ]
        var insert = query
        insert[kSecValueData as String] = Data("JobsIM nonsecret diagnostic".utf8)
        insert[kSecAttrAccessible as String] = kSecAttrAccessibleWhenUnlockedThisDeviceOnly
        let added = SecItemAdd(insert as CFDictionary, nil)
        var search = query
        search[kSecReturnData as String] = true
        search[kSecMatchLimit as String] = kSecMatchLimitOne
        var value: CFTypeRef?
        let read = SecItemCopyMatching(search as CFDictionary, &value)
        let deleted = SecItemDelete(query as CFDictionary)
        print("{\"addStatus\":\(added),\"readStatus\":\(read),\"deleteStatus\":\(deleted),\"missingEntitlementCode\":\(errSecMissingEntitlement)}")
    }
}
