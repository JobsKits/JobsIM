//
//  IMIntegrationFixture.swift
//  JobsIMTests
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import Foundation
import XCTest

import JobsIMCore
import JobsIMTransport
@testable import JobsIM

@MainActor
enum IMIntegrationFixture {
    static func certificate() throws -> IMInstanceCertificate {
        let file = try XCTUnwrap(
            Bundle(for: IMLifecycleTests.self).url(forResource: "IntegrationCA", withExtension: "pem"))
        return try IMInstanceCertificate(
            address: "https://127.0.0.1:8443", certificate: Data(contentsOf: file),
            verifiedFingerprint: "a5303379ba7fafcb197ed761a6ac132c6f9c2e762a06c205bba6d6372cb4a9d8")
    }
    static func client() throws -> IMAPIClient {
        let certificate = try certificate()
        try IMTrustStore.shared.save(certificate)
        return IMAPIClient(baseURL: URL(string: "https://127.0.0.1:8443/api/")!, certificate: certificate)
    }
    static func register(_ client: IMAPIClient) async throws -> IMJSON {
        return try await client.request(
            "auth/register", method: "POST",
            body: [
                "username": .string(
                    "native_" + UUID().uuidString.replacingOccurrences(of: "-", with: "").prefix(16)),
                "password": .string(UUID().uuidString + UUID().uuidString),
                "requestId": .string(UUID().uuidString),
                "deviceName": .string("JobsIM native lifecycle test"),
            ])
    }
}
