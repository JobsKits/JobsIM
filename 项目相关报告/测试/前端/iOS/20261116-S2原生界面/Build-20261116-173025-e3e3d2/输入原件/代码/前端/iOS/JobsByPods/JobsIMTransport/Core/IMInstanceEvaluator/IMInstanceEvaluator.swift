//
//  IMInstanceEvaluator.swift
//  JobsIMTransport
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import Foundation
import Security

import Alamofire

final class IMInstanceEvaluator: ServerTrustEvaluating, @unchecked Sendable {
    private let certificate: IMInstanceCertificate
    init(certificate: IMInstanceCertificate) {
        self.certificate = certificate
    }
    func evaluate(_ trust: SecTrust, forHost host: String) throws {
        guard URL(string: certificate.address)?.host?.lowercased() == host.lowercased(),
            let anchor = SecCertificateCreateWithData(nil, certificate.certificateDER as CFData)
        else { throw AFError.serverTrustEvaluationFailed(reason: .noRequiredEvaluator(host: host)) }
        let policy = SecPolicyCreateSSL(true, host as CFString)
        guard SecTrustSetPolicies(trust, policy) == errSecSuccess,
            SecTrustSetAnchorCertificates(trust, [anchor] as CFArray) == errSecSuccess,
            SecTrustSetAnchorCertificatesOnly(trust, true) == errSecSuccess
        else { throw AFError.serverTrustEvaluationFailed(reason: .noRequiredEvaluator(host: host)) }
        var error: CFError?
        guard SecTrustEvaluateWithError(trust, &error) else {
            throw IMAPIError(
                status: 0, code: "TLS_IDENTITY_INVALID",
                message: "Certificate hostname, validity or chain validation failed")
        }
    }
}
