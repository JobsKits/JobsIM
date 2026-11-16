//
//  IMStrings.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import Foundation

enum IMStrings {

    static let languages = [
        ("zh-Hans", "简体中文"), ("zh-Hant", "繁體中文"), ("bo", "བོད་ཡིག"), ("en", "English"), ("ja", "日本語"),
        ("fr", "Français"), ("es", "Español"), ("de", "Deutsch"), ("ru", "Русский"), ("ko", "한국어"),
    ]

    static func text(_ key: String) -> String {
        let language = UserDefaults.standard.string(forKey: "language") ?? "zh-Hans"
        let path = Bundle.main.path(forResource: language, ofType: "lproj")
        let bundle = path.flatMap(Bundle.init(path:)) ?? .main
        return bundle.localizedString(forKey: key, value: key, table: nil)
    }
}
