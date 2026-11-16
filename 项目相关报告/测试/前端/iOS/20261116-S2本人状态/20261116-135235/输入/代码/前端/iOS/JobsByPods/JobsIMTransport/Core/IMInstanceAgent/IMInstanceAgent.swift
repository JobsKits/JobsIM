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
