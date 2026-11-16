//
//  IMTrustStore.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import Foundation

import JobsIMCore
import JobsIMTransport

@MainActor
final class IMTrustStore {
    static let shared = IMTrustStore()
    func certificate(for address: String) -> IMInstanceCertificate? {
        guard let data = UserDefaults.standard.data(forKey: "instanceCA:" + address),
            let value = try? JSONDecoder.make().decode(IMInstanceCertificate.self, from: data)
        else {
            return nil
        }
        return try? IMInstanceCertificate(
            address: value.address, certificate: value.certificateDER, verifiedFingerprint: value.fingerprint)
    }
    func save(_ certificate: IMInstanceCertificate) throws {
        UserDefaults.standard.set(
            try JSONEncoder.make().encode(certificate), forKey: "instanceCA:" + certificate.address)
    }
    func remove(_ address: String) {
        UserDefaults.standard.removeObject(forKey: "instanceCA:" + address)
    }
}
