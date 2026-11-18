//
//  IMConversationsVC.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import UIKit

@MainActor
final class IMConversationsVC: IMListVC {
    init() {
        super.init(kind: .conversations)
    }
    required init?(coder: NSCoder) {
        fatalError("init(coder:) is unavailable")
    }
}
