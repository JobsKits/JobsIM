//
//  UINavigationBarAppearance+IMTheme.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import UIKit

extension UINavigationBarAppearance {
    @MainActor
    static func makeJobsIM(traits: UITraitCollection, titleFont: UIFont) -> UINavigationBarAppearance {
        let appearance = UINavigationBarAppearance()
        appearance.configureWithOpaqueBackground()
        appearance.backgroundColor = IMUI.background.resolvedColor(with: traits)
        appearance.shadowColor = IMUI.separator.resolvedColor(with: traits)
        appearance.titleTextAttributes = [
            .foregroundColor: IMUI.foreground.resolvedColor(with: traits), .font: titleFont,
        ]
        appearance.largeTitleTextAttributes = [
            .foregroundColor: IMUI.foreground.resolvedColor(with: traits), .font: IMUI.font(.largeTitle),
        ]
        let accent = IMUI.green.resolvedColor(with: traits)
        let secondary = IMUI.secondary.resolvedColor(with: traits)
        for button in [
            appearance.buttonAppearance, appearance.doneButtonAppearance, appearance.backButtonAppearance,
        ] {
            for state in [button.normal, button.highlighted, button.focused] {
                state.titleTextAttributes = [.foregroundColor: accent]
            }
            button.disabled.titleTextAttributes = [.foregroundColor: secondary]
        }
        return appearance
    }
}
