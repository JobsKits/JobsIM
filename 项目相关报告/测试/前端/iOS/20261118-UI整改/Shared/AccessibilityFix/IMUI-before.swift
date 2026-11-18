//
//  IMUI.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import JobsByUIKit
import JobsSwiftBaseDefines
import JobsSwiftDSL
import SnapKit
import UIKit

@MainActor
enum IMUI {

    static let foreground = UIColor.make(light: 0x17201B, dark: 0xEDF4EF)
    static let green = UIColor.make(light: 0x287C66, dark: 0x65C8AB)
    // 文字强调色随主题变化，承载白字的实心底色保持足够深。
    static let greenFill = UIColor.make(hex: 0x287C66)
    static let background = UIColor.make(light: 0xFAF9F6, dark: 0x101512)
    static let panel = UIColor.make(light: 0xFFFFFF, dark: 0x1B241F)
    static let secondary = UIColor.make(light: 0x66726C, dark: 0xABB7AE)
    static let separator = UIColor.make(light: 0xD7E0DA, dark: 0x34453A)
    static let selection = UIColor.make(light: 0xF2F6F3, dark: 0x28382F)

    @discardableResult
    static func cellTheme(_ cell: UITableViewCell) -> UITableViewCell {
        cell
            .byBackgroundColor(panel)
            .byContentView {
                $0.byBackgroundColor(JobsCor.clear)
            }
            .byTitleCor(foreground)
            .byDetailTitleCor(secondary)
            .byTitleFont(font(.body))
            .byDetailTitleFont(font(.subheadline))
            .bySelectedBackgroundView(UIView.jobsMake {
                $0.byBackgroundColor(selection)
            })
    }

    static func label(_ text: String = "", style: UIFont.TextStyle = .body, color: UIColor? = nil) -> UILabel
    {
        UILabel.jobsMake { label in
            label
                .byText(text)
                .byDynamicFont(font(style), style: style)
                .byTextColor(color ?? IMUI.foreground)
                .byNumberOfLines(0)
        }
    }

    static func font(_ style: UIFont.TextStyle) -> UIFont {
        let scale = UserDefaults.standard.integer(forKey: "fontScale")
        let font = JobsFont.preferredFont(forTextStyle: style)
        return font.withSize(font.pointSize + CGFloat(scale))
    }

    static func button(_ title: String, primary: Bool = false, action: @escaping () -> Void) -> UIButton {
        UIButton.sys()
            .byTitle(title)
            .byTitleColor(primary ? JobsCor.white : green)
            .byBackgroundColor(primary ? greenFill : JobsCor.clear)
            .byCornerRadius(14)
            .byAccessibilityLabel(title)
            .onTap { _ in
                action()
            }
    }

    static func field(_ placeholder: String, secure: Bool = false) -> UITextField {
        UITextField.jobsMake { field in
            field
                .byAttributedPlaceholder(
                    .make(
                        string: placeholder,
                        attributes: [
                            .foregroundColor: IMUI.secondary, .font: font(.body),
                        ])
                )
                .bySecureTextEntry(secure)
                .byFont(font(.body))
                .byTextColor(IMUI.foreground)
                .byBackgroundColor(panel)
                .byCornerRadius(12)
                .byAutocorrectionType(.no)
                .byAutocapitalizationType(.none)
                .byAccessibilityLabel(placeholder)
        }
    }

    static func stack(spacing: CGFloat = 12) -> UIStackView {
        UIStackView.jobsMake { stack in
            stack
                .byAxis(.vertical)
                .bySpacing(spacing)
        }
    }

    static func alert(_ controller: UIViewController, title: String? = nil, message: String) {
        let alert = UIAlertController.make(
            title: title ?? IMStrings.text("error"), message: message, style: .alert)
        alert.addAction(UIAlertAction.make(IMStrings.text("done"), style: .cancel) {})
        presentDialog(alert, on: controller)
    }

    static func presentDialog(_ alert: UIAlertController, on controller: UIViewController) {
        let present = {
            guard controller.viewIfLoaded?.window != nil else {
                return
            }
            controller.present(alert, animated: true)
        }
        if let current = controller.presentedViewController {
            if current.isBeingDismissed, let coordinator = current.transitionCoordinator {
                coordinator.animate(alongsideTransition: nil) {
                    _ in present()
                }
            } else {
                current.dismiss(animated: true, completion: present)
            }
        } else {
            present()
        }
    }

    static func confirm(
        _ controller: UIViewController, title: String, message: String, action: @escaping () -> Void
    ) {
        let alert = UIAlertController.make(title: title, message: message, style: .alert)
        alert.addAction(UIAlertAction.make(IMStrings.text("cancel"), style: .cancel) {})
        alert.addAction(UIAlertAction.make(IMStrings.text("continue"), style: .default, handler: action))
        presentDialog(alert, on: controller)
    }

    static func prompt(
        _ controller: UIViewController, title: String, value: String = "", secure: Bool = false,
        action: @escaping (String) -> Void
    ) {
        let alert = UIAlertController.make(title: title, message: nil, style: .alert)
        alert.addTextField { field in
            field
                .byText(value)
                .bySecureTextEntry(secure)
        }
        alert.addAction(UIAlertAction.make(IMStrings.text("cancel"), style: .cancel) {})
        alert.addAction(
            UIAlertAction.make(IMStrings.text("continue")) {
                action(alert.textFields?.first?.text ?? "")
            })
        presentDialog(alert, on: controller)
    }
}
