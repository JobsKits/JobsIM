//
//  IMAddFriendVC.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import JobsIMCore
import UIKit

@MainActor
final class IMAddFriendVC: IMFriendSearchVC {
    init() {
        super.init(scope: .people)
    }
    required init?(coder: NSCoder) {
        fatalError("init(coder:) is unavailable")
    }
}
