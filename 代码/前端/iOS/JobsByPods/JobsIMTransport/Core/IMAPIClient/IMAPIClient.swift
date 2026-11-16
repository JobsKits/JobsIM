//
//  IMAPIClient.swift
//  JobsIMTransport
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import Foundation

import JobsIMCore
@preconcurrency import JobsNetworking

public final class IMAPIClient: @unchecked Sendable {

    private let agent: JobsDefaultAgent
    private let trusted: IMInstanceAgent?

    public init(baseURL: URL, certificate: IMInstanceCertificate? = nil) {
        let config = JobsRequestConfig(
            baseURL: baseURL, timeout: 4, logger: JobsLogger(isEnabled: false), usesEphemeralSession: true)
        trusted = certificate.map { IMInstanceAgent(config: config, certificate: $0) }
        agent = JobsDefaultAgent(
            config: JobsRequestConfig(
                baseURL: baseURL, timeout: 4, logger: JobsLogger(isEnabled: false), usesEphemeralSession: true
            ))
    }

    public func request(
        _ path: String, method: String = "GET", body: [String: IMJSON] = [:], token: String? = nil
    ) async throws -> IMJSON {
        var headers = ["Accept": "application/json"]
        if let token {
            headers["Authorization"] = "Bearer " + token
        }
        let data = try JSONEncoder.make().encode(IMJSON.object(body))
        let object = try JSONSerialization.jsonObject(with: data) as? [String: Any] ?? [:]
        let values = object.mapValues {
            JobsValue($0)
        }
        let request = JobsRequest(
            path: path, method: HTTPMethod(rawValue: method) ?? .get, body: body.isEmpty ? nil : values,
            headers: headers, timeout: 4, cachePolicy: .networkOnly)
        return try await withCheckedThrowingContinuation { continuation in
            let selected: any JobsAgent = trusted ?? agent
            selected.send(request, as: IMJSON.self) { result in
                continuation.resume(
                    with: result.mapError {
                        Self.failure($0)
                    })
            }
        }
    }
    public func upload(
        _ file: URL, mimeType: String, token: String, path: String = "media", form: [String: String] = [:]
    ) async throws -> IMJSON {
        let request = JobsUploadRequest(
            path: path,
            files: [.file(url: file, name: "file", fileName: file.lastPathComponent, mimeType: mimeType)],
            form: form.mapValues { JobsValue($0) },
            headers: ["Authorization": "Bearer " + token], timeout: 120)
        return try await withCheckedThrowingContinuation { continuation in
            let completion: (Result<IMJSON, JobsError>) -> Void = { result in
                continuation.resume(with: result.mapError { Self.failure($0) })
            }
            if let trusted {
                trusted.upload(request, as: IMJSON.self, completion: completion)
            } else {
                agent.upload(request, as: IMJSON.self, completion: completion)
            }
        }
    }

    public func originalJSON(_ path: String, body: String, token: String) async throws -> IMJSON {
        guard let trusted, let bytes = body.data(using: .utf8), bytes.count <= 2048,
            let decoded = try? JSONDecoder.make().decode(IMJSON.self, from: bytes), case .object = decoded
        else {
            throw IMAPIError(status: 0, code: "EXPORT_TRANSPORT_REQUIRED", message: "Verified instance transport required")
        }
        let cancellation = JobsRequestToken()
        return try await withTaskCancellationHandler {
            try await withCheckedThrowingContinuation { continuation in
                trusted.originalJSON(path, body: bytes, accessToken: token, cancellation: cancellation) { result in
                    continuation.resume(with: result.mapError { Self.failure($0) })
                }
            }
        } onCancel: {
            cancellation.cancel()
        }
    }

    public func downloadPersonalExport(_ id: String, ticket: String, to file: URL, token: String) async throws -> URL {
        guard UUID(uuidString: id) != nil, let trusted,
            ticket.range(of: "^[A-Za-z0-9_-]{16,256}$", options: .regularExpression) != nil
        else {
            throw IMAPIError(status: 0, code: "EXPORT_TRANSPORT_REQUIRED", message: "Verified instance transport required")
        }
        let cancellation = JobsRequestToken()
        do {
            return try await withTaskCancellationHandler {
                try await withCheckedThrowingContinuation { continuation in
                    trusted.personalExport(id, ticket: ticket, destination: file, accessToken: token,
                                           cancellation: cancellation) { result in
                        continuation.resume(with: result.mapError { Self.failure($0) })
                    }
                }
            } onCancel: {
                cancellation.cancel()
            }
        } catch {
            try? FileManager.default.removeItem(at: file)
            throw error
        }
    }

    public func download(_ id: String, to file: URL, token: String, collection: String = "media") async throws
        -> URL
    {
        let url = agent.config.baseURL.appendingPathComponent(collection).appendingPathComponent(id)
        let request = JobsDownloadRequest(
            absoluteURL: url, destinationURL: file, headers: ["Authorization": "Bearer " + token],
            timeout: 120)
        return try await withCheckedThrowingContinuation { continuation in
            let completion: (Result<URL, JobsError>) -> Void = { result in
                continuation.resume(with: result.mapError { Self.failure($0) })
            }
            if let trusted {
                trusted.download(request, completion: completion)
            } else {
                agent.download(request, completion: completion)
            }
        }
    }

    private static func failure(_ error: JobsError) -> Error {
        switch error {
        /// 只呈现授权的错误字段，不输出原始响应及认证材料。
        case .http(let status, let data), .server(let status, let data):
            let value = data.flatMap { try? JSONDecoder.make().decode(IMJSON.self, from: $0) } ?? .null
            return IMAPIError(
                status: status, code: value["error"]["code"].text,
                message: value["error"]["message"].text.isEmpty
                    ? "HTTP \(status)" : value["error"]["message"].text)
        /// 传输失败没有成功提交证明。
        default:
            return IMAPIError(status: 0, code: "TRANSPORT_UNKNOWN", message: error.localizedDescription)
        }
    }

}
