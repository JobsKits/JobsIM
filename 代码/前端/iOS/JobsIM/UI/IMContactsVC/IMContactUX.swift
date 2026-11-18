//
//  IMContactUX.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import JobsByUIKit
import JobsIMCore
import JobsSwiftBaseDefines
import JobsSwiftDSL
import UIKit

@MainActor
enum IMContactUX {
    static func confirmRemoval(on controller: IMBaseVC, action: @escaping () -> Void) {
        let alert = UIAlertController.make(
            title: IMStrings.text("removeContact"), message: IMStrings.text("removeContactWarning"),
            style: .alert)
        alert.addAction(
            UIAlertAction.make(IMStrings.text("deleteAction"), style: .destructive) {
                action()
            })
        // Both buttons use explicit order; UIKit's cancel style would move the safe action to the left.
        alert.addAction(
            UIAlertAction.make(IMStrings.text("accidentalSwipe")) {
            })
        IMUI.presentDialog(alert, on: controller)
    }
}
