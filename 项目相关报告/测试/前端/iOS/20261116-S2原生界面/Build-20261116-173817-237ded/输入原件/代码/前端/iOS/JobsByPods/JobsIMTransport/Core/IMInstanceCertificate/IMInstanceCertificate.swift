//
//  IMInstanceCertificate.swift
//  JobsIMTransport
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import Foundation
import Security
import CryptoKit

public struct IMInstanceCertificate: Codable, Sendable {
    public let address: String
    public let certificateDER: Data
    public let fingerprint: String
    public init(address: String, certificate: Data, verifiedFingerprint: String) throws {
        guard let url = URL(string: address), url.scheme == "https", url.host != nil, url.user == nil,
            url.password == nil, url.query == nil, url.fragment == nil
        else { throw IMAPIError(status: 0, code: "INVALID_ORIGIN", message: "HTTPS origin required") }
        let der: Data
        if let pem = String(data: certificate, encoding: .utf8), pem.contains("-----BEGIN CERTIFICATE-----") {
            let payload =
                pem.components(separatedBy: "-----BEGIN CERTIFICATE-----").dropFirst().first?.components(
                    separatedBy: "-----END CERTIFICATE-----"
                ).first ?? ""
            guard let decoded = Data(base64Encoded: payload, options: .ignoreUnknownCharacters) else {
                throw IMAPIError(status: 0, code: "INVALID_CA", message: "Invalid PEM")
            }
            der = decoded
        } else {
            der = certificate
        }
        guard SecCertificateCreateWithData(nil, der as CFData) != nil else {
            throw IMAPIError(status: 0, code: "INVALID_CA", message: "Invalid CA certificate")
        }
        let actual = SHA256.hash(data: der).map { String(format: "%02x", $0) }.joined()
        let expected = verifiedFingerprint.lowercased().filter { $0 != ":" && !$0.isWhitespace }
        guard expected.count == 64, expected == actual else {
            throw IMAPIError(status: 0, code: "FINGERPRINT_MISMATCH", message: "SHA256 fingerprint mismatch")
        }
        self.address = address
        certificateDER = der
        fingerprint = actual
    }
    public func permits(_ url: URL) -> Bool {
        guard let expected = URL(string: address) else {
            return false
        }
        var mount = expected.path.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        if mount != "api" && !mount.hasSuffix("/api") { mount += mount.isEmpty ? "api" : "/api" }
        let prefix = "/" + mount
        guard url.path == prefix || url.path.hasPrefix(prefix + "/") else {
            return false
        }
        return url.scheme == "https" && url.host?.lowercased() == expected.host?.lowercased()
            && (url.port ?? 443) == (expected.port ?? 443)
    }
}
