//
//  UNNotificationRequest+Make.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import UserNotifications

extension UNNotificationRequest {
    static func make(identifier: String, content: UNNotificationContent) -> UNNotificationRequest {
        return UNNotificationRequest(identifier: identifier, content: content, trigger: nil)
    }
}
