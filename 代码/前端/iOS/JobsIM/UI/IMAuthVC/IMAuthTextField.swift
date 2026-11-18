//
//  IMAuthTextField.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import JobsByUIKit
import JobsSwiftBaseDefines
import JobsSwiftDSL
import UIKit

@MainActor
final class IMAuthTextField: UITextField {

    private let allowsReveal: Bool
    private(set) var passwordVisible = false
    private lazy var revealButton = IMAuthAppearance.button("", compact: true) { [weak self] in
        guard let self else {
            return
        }
        self.setPasswordVisible(!self.passwordVisible)
    }

    init(_ key: String, secure: Bool = false) {
        allowsReveal = secure
        super.init(frame: .zero)
        let placeholder = IMStrings.text(key)
        byFont(IMAuthAppearance.font(14))
            .byTextColor(IMUI.foreground)
            .byBackgroundColor(IMAuthAppearance.input)
            .byCornerRadius(11)
            .byBorderWidth(1)
            .byBorderColor(IMAuthAppearance.outline)
            .byAttributedPlaceholder(.make(string: placeholder, attributes: [
                .foregroundColor: IMAuthAppearance.secondary,
                .font: IMAuthAppearance.font(14),
            ]))
            .byAutocorrectionType(.no)
            .byAutocapitalizationType(.none)
            .bySecureTextEntry(secure)
            .byAccessibilityLabel(placeholder)
        if secure {
            revealButton.byFrame(CGRect(x: 0, y: 0, width: 44, height: 44))
            byRightView(revealButton, mode: .always)
            refreshReveal()
        }
    }

    required init?(coder: NSCoder) {
        fatalError("init(coder:) has not been implemented")
    }

    @discardableResult
    func byAuthIdentifier(_ identifier: String) -> Self {
        byAccessibilityIdentifier(identifier)
        refreshReveal()
        return self
    }

    func setPasswordVisible(_ visible: Bool) {
        guard allowsReveal else {
            return
        }
        let value = text
        let range = selectedTextRange
        passwordVisible = visible
        // 安全输入切换会重置系统光标，保留正在编辑的位置。
        bySecureTextEntry(!visible)
            .byText(value)
            .bySelectedTextRange(range)
        refreshReveal()
    }

    private func refreshReveal() {
        guard allowsReveal else {
            return
        }
        revealButton
            .byImage((passwordVisible ? "eye.slash" : "eye").sysImg(pointSize: 19))
            .byConfiguration {
                $0.byPreferredSymbolConfigurationForImage(IMAuthAppearance.smallIconConfiguration)
            }
            .byAccessibilityLabel(IMStrings.text(passwordVisible ? "authHidePassword" : "authShowPassword"))
            .byAccessibilityIdentifier((accessibilityIdentifier ?? "auth.secret") + ".visibility")
    }

    override func textRect(forBounds bounds: CGRect) -> CGRect {
        return super.textRect(forBounds: bounds).insetBy(dx: 12, dy: 3)
    }

    override func editingRect(forBounds bounds: CGRect) -> CGRect {
        return super.editingRect(forBounds: bounds).insetBy(dx: 12, dy: 3)
    }

    override func rightViewRect(forBounds bounds: CGRect) -> CGRect {
        guard allowsReveal else {
            return super.rightViewRect(forBounds: bounds)
        }
        // UITextField会按按钮固有尺寸重算rightView；显式保留44点触控，与小图标解耦。
        return CGRect(x: bounds.maxX - 47, y: bounds.midY - 22, width: 44, height: 44)
    }

    override func traitCollectionDidChange(_ previousTraitCollection: UITraitCollection?) {
        super.traitCollectionDidChange(previousTraitCollection)
        byBorderColor(IMAuthAppearance.outline.resolvedColor(with: traitCollection))
    }
}
