//
//  IMContactLocalStars.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import Foundation
import JobsIMCore

@MainActor
enum IMContactLocalStars {
    static func ids(session: IMSession) -> Set<String> {
        Set(
            ((try? session.store.list("contact-stars", namespace: session.namespace)) ?? []).map {
                $0["id"].text
            })
    }
    static func set(_ starred: Bool, userID: String, session: IMSession) throws {
        guard !userID.isEmpty else {
            return
        }
        if starred {
            try session.store.save(
                [.object(["id": .string(userID)])], collection: "contact-stars", namespace: session.namespace)
        } else {
            try session.store.remove(userID, collection: "contact-stars", namespace: session.namespace)
        }
    }
}
