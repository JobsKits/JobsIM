//
//  IMMessagePage.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import Foundation
import JobsIMCore

struct IMMessagePage {

    let items: [IMJSON]
    let cursor: String
    let hasMore: Bool

    init(_ value: IMJSON) throws {
        guard case .array(let items) = value["items"],
            case .string(let cursor) = value["cursor"],
            case .bool(let hasMore) = value["hasMore"],
            !cursor.isEmpty,
            items.allSatisfy({ !$0["id"].text.isEmpty }),
            Set(items.map { $0["id"].text }).count == items.count,
            !hasMore || (!items.isEmpty && cursor != "0")
        else {
            throw IMSession.SessionError.invalidInstance
        }
        self.items = items
        self.cursor = cursor
        self.hasMore = hasMore
    }
}
