//
//  URLRequest+Make.swift
//  JobsIMTransport
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import Foundation

extension URLRequest {
    static func make(url: URL, method: String, headers: [String: String], body: Data?, timeout: TimeInterval)
        -> URLRequest
    {
        var request = URLRequest(url: url, timeoutInterval: timeout)
        request.httpMethod = method
        request.allHTTPHeaderFields = headers
        request.httpBody = body
        if body != nil { request.setValue("application/json", forHTTPHeaderField: "Content-Type") }
        return request
    }
}
