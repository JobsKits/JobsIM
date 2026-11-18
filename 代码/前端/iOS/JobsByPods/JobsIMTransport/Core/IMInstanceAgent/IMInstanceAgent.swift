//
//  IMInstanceAgent.swift
//  JobsIMTransport
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import Foundation

import JobsIMCore
@preconcurrency import JobsNetworking
@preconcurrency import Alamofire

public final class IMInstanceAgent: JobsAgent, @unchecked Sendable {
    public let config: JobsRequestConfig
    public let headerHook: JobsHeaderHook = EmptyHeaderHook()
    private let session: Session
    private let certificate: IMInstanceCertificate
    public init(config: JobsRequestConfig, certificate: IMInstanceCertificate) {
        self.config = config
        self.certificate = certificate
        let configuration = URLSessionConfiguration.ephemeral
        configuration.urlCache = nil
        configuration.urlCredentialStorage = nil
        configuration.timeoutIntervalForRequest = 120
        let manager = ServerTrustManager(evaluators: [
            (URL(string: certificate.address)?.host ?? ""): IMInstanceEvaluator(certificate: certificate)
        ])
        session = Session(
            configuration: configuration, serverTrustManager: manager,
            redirectHandler: Redirector(
                behavior: .modify { _, request, _ in
                    guard let url = request.url, certificate.permits(url) else {
                        return nil
                    }
                    return request
                }))
    }
    public func send<T: Decodable>(
        _ request: JobsRequest, as type: T.Type, completion: @escaping (Result<T, JobsError>) -> Void
    ) -> JobsRequestToken {
        return observe(request, as: type, onEvent: { _ in }, completion: completion)
    }
    public func observe<T: Decodable>(
        _ request: JobsRequest, as type: T.Type,
        onEvent: @escaping (Result<(T, JobsResponseSource), JobsError>) -> Void,
        completion: @escaping (Result<T, JobsError>) -> Void
    ) -> JobsRequestToken {
        let box = IMCompletion<T>(completion: completion, onEvent: onEvent)
        let token = JobsRequestToken()
        guard let url = URL(string: request.path, relativeTo: config.baseURL)?.absoluteURL,
            certificate.permits(url)
        else { box.complete(.failure(.invalidRequest(reason: "Origin mismatch"))); return token }
        let data: Data?
        do {
            data = try request.body.map {
                try JSONSerialization.data(withJSONObject: $0.mapValues { $0.raw ?? kCFNull as Any })
            }
        } catch { box.complete(.failure(.invalidRequest(reason: "Invalid JSON"))); return token }
        let prepared = URLRequest.make(
            url: url, method: request.method.rawValue, headers: request.headers, body: data,
            timeout: request.timeout ?? 4)
        let task = session.request(prepared)
        token.setCancel { task.cancel() }
        task.responseData { response in
            token.setCancel {}
            box.complete(Self.decode(response, type: type))
        }
        return token
    }
    func upload<T: Decodable>(
        _ request: JobsUploadRequest, as type: T.Type, completion: @escaping (Result<T, JobsError>) -> Void
    ) {
        let box = IMCompletion<T>(completion: completion, onEvent: { _ in })
        guard let url = URL(string: request.path, relativeTo: config.baseURL)?.absoluteURL,
            certificate.permits(url)
        else { box.complete(.failure(.invalidRequest(reason: "Origin mismatch"))); return }
        session.upload(
            multipartFormData: { form in
                for (key, value) in request.form {
                    form.append(Data(String(describing: value.raw ?? "").utf8), withName: key)
                }
                for file in request.files {
                    switch file {
                    case .file(let url, let name, let fileName, let mime):
                        form.append(url, withName: name, fileName: fileName, mimeType: mime)
                    case .data(let data, let name, let fileName, let mime):
                        form.append(data, withName: name, fileName: fileName, mimeType: mime)
                    }
                }
            }, to: url, method: .post, headers: HTTPHeaders(request.headers)
        ).responseData { response in box.complete(Self.decode(response, type: type)) }
    }
    func download(_ request: JobsDownloadRequest, completion: @escaping (Result<URL, JobsError>) -> Void) {
        let box = IMDownloadCompletion(completion)
        guard certificate.permits(request.absoluteURL) else {
            box.complete(.failure(.invalidRequest(reason: "Origin mismatch"))); return
        }
        session.request(request.absoluteURL, headers: HTTPHeaders(request.headers)).responseData { response in
            guard response.error == nil, let status = response.response?.statusCode,
                (200..<300).contains(status), let data = response.data
            else {
                box.complete(
                    .failure(.http(statusCode: response.response?.statusCode ?? 0, data: response.data)));
                return
            }
            do {
                try data.write(to: request.destinationURL, options: .atomic);
                box.complete(.success(request.destinationURL))
            } catch { box.complete(.failure(.transport(underlying: "Private download could not be saved"))) }
        }
    }

    func originalJSON(_ path: String, body: Data, accessToken: String, cancellation: JobsRequestToken,
                      completion: @escaping (Result<IMJSON, JobsError>) -> Void) {
        let box = IMCompletion<IMJSON>(completion: completion, onEvent: { _ in })
        guard !cancellation.isCancelled,
            let url = URL(string: path, relativeTo: config.baseURL)?.absoluteURL, certificate.permits(url) else {
            box.complete(.failure(.invalidRequest(reason: "Origin mismatch")))
            return
        }
        let request = URLRequest.make(url: url, method: "POST",
            headers: ["Authorization": "Bearer " + accessToken, "Accept": "application/json"], body: body, timeout: 15)
        let task = session.request(request).redirect(using: Redirector(behavior: .doNotFollow))
        cancellation.setCancel { task.cancel() }
        task.responseData { response in
            cancellation.setCancel {}
            box.complete(Self.decode(response, type: IMJSON.self))
        }
    }

    func personalExport(_ id: String, ticket: String, destination: URL, accessToken: String,
                        cancellation: JobsRequestToken, completion: @escaping (Result<URL, JobsError>) -> Void) {
        let box = IMDownloadCompletion(completion)
        let url = config.baseURL.appendingPathComponent("me/data-exports/" + id + "/download")
        guard !cancellation.isCancelled, certificate.permits(url) else {
            box.complete(.failure(.invalidRequest(reason: "Origin mismatch")))
            return
        }
        let request = URLRequest.make(url: url, method: "GET", headers: ["Authorization": "Bearer " + accessToken,
            "X-JobsIM-Download-Token": ticket, "Accept": "application/zip"], body: nil, timeout: 120)
        let task = session.download(request, to: { _, _ in (destination, []) })
            .redirect(using: Redirector(behavior: .doNotFollow))
        cancellation.setCancel { task.cancel() }
        task.downloadProgress { progress in
            if progress.completedUnitCount > 1_073_741_824 || progress.totalUnitCount > 1_073_741_824 {
                cancellation.cancel()
            }
        }.responseURL { response in
            cancellation.setCancel {}
            guard response.error == nil, response.response?.statusCode == 200, let file = response.fileURL else {
                var failure: Data?
                if let file = response.fileURL,
                    let size = (try? FileManager.default.attributesOfItem(atPath: file.path)[.size]) as? NSNumber,
                    size.uint64Value <= 65536 {
                    failure = try? Data(contentsOf: file)
                }
                box.complete(.failure(.http(statusCode: response.response?.statusCode ?? 0, data: failure)))
                return
            }
            guard response.response?.mimeType?.lowercased() == "application/zip",
                response.response?.value(forHTTPHeaderField: "Cache-Control")?.lowercased().contains("no-store") == true,
                response.response?.value(forHTTPHeaderField: "Content-Disposition")?.lowercased().hasPrefix("attachment;") == true,
                let size = (try? FileManager.default.attributesOfItem(atPath: file.path)[.size]) as? NSNumber,
                size.uint64Value > 0, size.uint64Value <= 1_073_741_824
            else {
                box.complete(.failure(.invalidRequest(reason: "Export ZIP response invalid")))
                return
            }
            box.complete(.success(file))
        }
    }

    private static func decode<T: Decodable>(_ response: AFDataResponse<Data>, type: T.Type) -> Result<
        T, JobsError
    > {
        guard let status = response.response?.statusCode else {
            return .failure(.transport(underlying: "TLS or transport validation failed"))
        }
        guard (200..<300).contains(status) else {
            return .failure(.http(statusCode: status, data: response.data))
        }
        guard response.error == nil else {
            return .failure(.transport(underlying: "Response incomplete"))
        }
        do {
            return .success(
                try JSONDecoder.make().decode(
                    type, from: response.data?.isEmpty == false ? response.data! : Data("{}".utf8)))
        } catch { return .failure(.decode(underlying: "Invalid response", data: nil)) }
    }
}

final class IMPasswordChangeTransport: @unchecked Sendable {

    private let session: Session
    private let baseURL: URL

    init(baseURL: URL, certificate: IMInstanceCertificate?) {
        self.baseURL = baseURL
        let configuration = URLSessionConfiguration.ephemeral
        configuration.urlCache = nil
        configuration.urlCredentialStorage = nil
        configuration.httpCookieStorage = nil
        configuration.httpShouldSetCookies = false
        let trust = certificate.map { value in
            ServerTrustManager(evaluators: [(baseURL.host ?? ""): IMInstanceEvaluator(certificate: value)])
        }
        session = Session(configuration: configuration, serverTrustManager: trust,
                          redirectHandler: Redirector(behavior: .doNotFollow))
    }

    func request(_ path: String, body: Data, token: String?, cancellation: JobsRequestToken,
                 completion: @escaping (Result<IMJSON, JobsError>) -> Void) {
        let box = IMCompletion<IMJSON>(completion: completion, onEvent: { _ in })
        guard !cancellation.isCancelled, baseURL.scheme == "https", baseURL.host != nil,
            baseURL.user == nil, baseURL.password == nil, baseURL.query == nil, baseURL.fragment == nil,
            ["auth/password-change", "auth/password-change-results", "auth/deletion-requests",
             "auth/deletion-results", "auth/data-rights/proofs", "auth/data-rights/query",
             "auth/data-rights/cancel"].contains(path), body.count <= 8192 else {
            box.complete(.failure(.invalidRequest(reason: "Verified HTTPS account-operation route required")))
            return
        }
        var headers = ["Accept": "application/json"]
        if let token {
            headers["Authorization"] = "Bearer " + token
        }
        let url = baseURL.appendingPathComponent(path)
        let prepared = URLRequest.make(url: url, method: "POST", headers: headers, body: body, timeout: 20)
        let task = session.request(prepared)
        cancellation.setCancel { task.cancel() }
        task.responseData { response in
            cancellation.setCancel {}
            guard let status = response.response?.statusCode, let bytes = response.data,
                bytes.count <= 32768, response.error == nil else {
                box.complete(.failure(.transport(underlying: "Password-change result unknown")))
                return
            }
            guard status == 200 else {
                box.complete(.failure(.http(statusCode: status, data: bytes)))
                return
            }
            do {
                box.complete(.success(try JSONDecoder.make().decode(IMJSON.self, from: bytes)))
            } catch {
                box.complete(.failure(.decode(underlying: "Invalid password-change result", data: nil)))
            }
        }
    }
}

final class IMMigrationTransport: @unchecked Sendable {

    private let session: Session
    private let baseURL: URL

    init(baseURL: URL, certificate: IMInstanceCertificate?) {
        self.baseURL = baseURL
        let configuration = URLSessionConfiguration.ephemeral
        configuration.urlCache = nil
        configuration.urlCredentialStorage = nil
        configuration.httpCookieStorage = nil
        configuration.httpShouldSetCookies = false
        let trust = certificate.map { value in
            ServerTrustManager(evaluators: [(baseURL.host ?? ""): IMInstanceEvaluator(certificate: value)])
        }
        session = Session(configuration: configuration, serverTrustManager: trust,
                          redirectHandler: Redirector(behavior: .doNotFollow))
    }

    func request(_ path: String, method: String, body: Data, token: String, store: String,
                 cancellation: JobsRequestToken, completion: @escaping (Result<Data, JobsError>) -> Void) {
        let fixed: [String: String] = ["sync/migration-capabilities": "GET", "sync/migration-pairings": "POST",
                                       "sync/migration-pairing-results": "POST", "sync/migrations": "POST",
                                       "sync/migration-results": "POST"]
        let parts = path.split(separator: "/", omittingEmptySubsequences: false)
        let dynamic = parts.count >= 3 && parts.count <= 4 && parts[0] == "sync" && parts[1] == "migrations"
            && UUID(uuidString: String(parts[2])) != nil
        let originalObject = parts.count == 2 && parts[0] == "media" && UUID(uuidString: String(parts[1])) != nil && method == "GET"
        let allowed = originalObject || fixed[path] == method || dynamic && (parts.count == 3 && method == "GET"
            || parts.count == 4 && (parts[3] == "capsule" && method == "GET"
                || ["key", "validate", "installed", "cancel"].contains(String(parts[3])) && method == "POST"))
        guard allowed, !cancellation.isCancelled, UUID(uuidString: store) != nil, !token.isEmpty,
            baseURL.scheme == "https", baseURL.host != nil, baseURL.user == nil, baseURL.password == nil,
            baseURL.query == nil, baseURL.fragment == nil, body.count <= 128 * 1024 * 1024 else {
            completion(.failure(.invalidRequest(reason: "Verified HTTPS migration route required")))
            return
        }
        let binary = path.hasSuffix("/capsule") || originalObject
        let limit: Int64 = originalObject ? 64 * 1024 * 1024 : path.hasSuffix("/capsule") ? 128 * 1024 * 1024 + 4096 : 2 * 1024 * 1024
        let prepared = URLRequest.make(url: baseURL.appendingPathComponent(path), method: method,
            headers: ["Accept": binary ? "application/octet-stream" : "application/json",
                      "Authorization": "Bearer " + token, "X-JobsIM-Store-Generation": store],
            body: method == "GET" ? nil : body, timeout: 90)
        let task = session.request(prepared)
        cancellation.setCancel { task.cancel() }
        task.downloadProgress { progress in
            if progress.completedUnitCount > limit || progress.totalUnitCount > limit {
                task.cancel()
            }
        }
        task.responseData { response in
            cancellation.setCancel {}
            guard !cancellation.isCancelled, let status = response.response?.statusCode,
                let bytes = response.data, bytes.count <= limit, response.error == nil else {
                completion(.failure(.transport(underlying: "Migration result unknown")))
                return
            }
            guard status == 200 else {
                completion(.failure(.http(statusCode: status, data: bytes.prefix(32768))))
                return
            }
            completion(.success(bytes))
        }
    }
}

final class IMReceiptTransport: @unchecked Sendable {

    private let session: Session
    private let baseURL: URL

    init(baseURL: URL, certificate: IMInstanceCertificate?) {
        self.baseURL = baseURL
        let configuration = URLSessionConfiguration.ephemeral
        configuration.urlCache = nil
        configuration.urlCredentialStorage = nil
        configuration.httpCookieStorage = nil
        configuration.httpShouldSetCookies = false
        let trust = certificate.map { value in
            return ServerTrustManager(evaluators: [(baseURL.host ?? ""): IMInstanceEvaluator(certificate: value)])
        }
        session = Session(configuration: configuration, serverTrustManager: trust,
            redirectHandler: Redirector(behavior: .doNotFollow))
    }

    func request(_ path: String, method: String, body: Data, token: String, store: String,
                 cancellation: JobsRequestToken, completion: @escaping (Result<Data, JobsError>) -> Void) {
        guard let parsed = URLComponents(string: path), parsed.scheme == nil, parsed.host == nil,
            parsed.fragment == nil, !parsed.path.hasPrefix("/"), parsed.percentEncodedPath == parsed.path else {
            completion(.failure(.invalidRequest(reason: "An exact receipt route is required")))
            return
        }
        let fixed: [String: String] = ["sync/storage": "PUT", "sync/storage-results": "POST",
            "sync/receipt-challenges": "POST", "sync/receipts": "POST", "sync/receipt-results": "POST",
            "sync/object-receipt-challenges": "POST", "sync/object-receipts": "POST", "sync/object-receipt-results": "POST"]
        let parts = parsed.path.split(separator: "/", omittingEmptySubsequences: false)
        let binary = parts.count == 2 && parts[0] == "media" && UUID(uuidString: String(parts[1])) != nil
        let target = parts.count == 3 && parts[0] == "sync" && parts[1] == "receipt-targets"
            && UUID(uuidString: String(parts[2])) != nil
        let query = parsed.queryItems ?? []
        let queryKeys = query.map(\.name)
        let readable = ["sync/receipt-context", "sync/receipt-profiles"].contains(parsed.path) || target
        let expectedKeys: Set<String> = parsed.path == "sync/receipt-profiles" ? ["schemaVersion", "buildDigest"]
            : target && queryKeys.contains("qualificationId") ? ["schemaVersion", "qualificationId"] : ["schemaVersion"]
        let schema = query.first(where: { $0.name == "schemaVersion" })?.value
        let digest = query.first(where: { $0.name == "buildDigest" })?.value ?? ""
        let qualification = query.first(where: { $0.name == "qualificationId" })?.value
        let validRead = readable && method == "GET" && Set(queryKeys) == expectedKeys
            && queryKeys.count == expectedKeys.count && ["1", "2"].contains(schema ?? "")
            && (qualification == nil || target && UUID(uuidString: qualification ?? "") != nil)
            && (parsed.path != "sync/receipt-profiles" || digest.range(of: "^[0-9a-f]{64}$", options: .regularExpression) != nil)
        guard validRead || query.isEmpty && (fixed[parsed.path] == method || binary && method == "GET"),
            !cancellation.isCancelled, UUID(uuidString: store) != nil, !token.isEmpty,
            baseURL.scheme == "https", baseURL.host != nil, baseURL.user == nil, baseURL.password == nil,
            baseURL.query == nil, baseURL.fragment == nil, body.count <= 32768,
            var address = URLComponents(url: baseURL.appendingPathComponent(parsed.path), resolvingAgainstBaseURL: false)
        else {
            completion(.failure(.invalidRequest(reason: "Verified HTTPS receipt route and original store required")))
            return
        }
        address.queryItems = query.isEmpty ? nil : query
        guard let url = address.url else {
            completion(.failure(.invalidRequest(reason: "Invalid receipt route")))
            return
        }
        let limit: Int64 = binary ? 64 * 1024 * 1024 : 2 * 1024 * 1024
        let prepared = URLRequest.make(url: url, method: method,
            headers: ["Accept": binary ? "application/octet-stream" : "application/json",
                "Authorization": "Bearer " + token, "X-JobsIM-Store-Generation": store],
            body: method == "GET" ? nil : body, timeout: binary ? 90 : 15)
        let task = session.request(prepared)
        cancellation.setCancel { task.cancel() }
        task.downloadProgress { progress in
            if progress.completedUnitCount > limit || progress.totalUnitCount > limit {
                task.cancel()
            }
        }
        task.responseData { response in
            cancellation.setCancel {}
            guard !cancellation.isCancelled, response.error == nil,
                let status = response.response?.statusCode, let bytes = response.data, bytes.count <= limit else {
                completion(.failure(.transport(underlying: "Receipt result unknown")))
                return
            }
            guard status == 200 else {
                completion(.failure(.http(statusCode: status, data: bytes.prefix(32768))))
                return
            }
            guard binary || response.response?.mimeType == "application/json" else {
                completion(.failure(.decode(underlying: "Invalid receipt response", data: nil)))
                return
            }
            completion(.success(bytes))
        }
    }
}

private final class IMCompletion<T>: @unchecked Sendable {
    let completion: (Result<T, JobsError>) -> Void
    let onEvent: (Result<(T, JobsResponseSource), JobsError>) -> Void
    init(
        completion: @escaping (Result<T, JobsError>) -> Void,
        onEvent: @escaping (Result<(T, JobsResponseSource), JobsError>) -> Void
    ) { self.completion = completion; self.onEvent = onEvent }
    func complete(_ result: Result<T, JobsError>) {
        onEvent(result.map { ($0, .network) }); completion(result)
    }
}
private final class IMDownloadCompletion: @unchecked Sendable {
    let completion: (Result<URL, JobsError>) -> Void
    init(_ completion: @escaping (Result<URL, JobsError>) -> Void) {
        self.completion = completion
    }
    func complete(_ result: Result<URL, JobsError>) {
        completion(result)
    }
}
