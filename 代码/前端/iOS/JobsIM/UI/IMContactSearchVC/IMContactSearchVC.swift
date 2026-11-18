//
//  IMContactSearchVC.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import JobsIMCore
import UIKit

@MainActor
final class IMContactSearchVC: IMFriendSearchVC {
    init(people: [IMJSON]) {
        super.init(scope: .contacts, people: people)
    }
    required init?(coder: NSCoder) {
        fatalError("init(coder:) is unavailable")
    }
}
