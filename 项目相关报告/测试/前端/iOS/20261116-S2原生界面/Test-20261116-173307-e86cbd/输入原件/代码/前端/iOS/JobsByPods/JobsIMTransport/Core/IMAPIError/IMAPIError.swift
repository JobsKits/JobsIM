//
//  IMAPIError.swift
//  JobsIMTransport
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import Foundation

public struct IMAPIError: LocalizedError, Sendable {

    public let status: Int
    public let code: String
    public let message: String

    public init(status: Int, code: String, message: String) {
        self.status = status
        self.code = code
        self.message = message
    }

    public var errorDescription: String? {
        message
    }
}
