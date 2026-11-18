//
//  IMAuthSettingsVC.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import JobsByUIKit
import JobsIMTransport
import JobsSwiftBaseDefines
import JobsSwiftDSL
import SnapKit
import UIKit

@MainActor
final class IMAuthSettingsVC: IMBaseVC, UIDocumentPickerDelegate {

    private var demoEnabled: Bool
    private var working = false
    private let onDemoChange: (Bool) async throws -> Void
    private let onContextChanged: () -> Void
    private let onReauthenticate: (IMAuthSettingsVC) -> Void
    private lazy var scroll = UIScrollView.jobsMake {
        $0.byKeyboardDismissMode(.interactive)
    }
    private lazy var stack = IMUI.stack(spacing: 16)
    private lazy var connectionTitle = IMAuthAppearance.label(IMStrings.text("connection"), size: 17, strong: true)
    private lazy var address = IMAuthTextField("serviceURL")
        .byAuthIdentifier("auth.address")
    private lazy var status = IMAuthAppearance.label(session.lastError ?? IMStrings.text("connectionNeeded"))
        .byAccessibilityIdentifier("auth.connectionStatus")
    private lazy var connect = IMAuthAppearance.button(IMStrings.text("connect"), primary: true) { [weak self] in
        self?.verifyConnection()
    }.byAccessibilityIdentifier("auth.connect")
    private lazy var importCA = IMAuthAppearance.button(IMStrings.text("localCA")) { [weak self] in
        guard let self else {
            return
        }
        self.present(UIDocumentPickerViewController.makeImporting(delegate: self), animated: true)
    }.byAccessibilityIdentifier("auth.ca")
    private lazy var removeCA = IMAuthAppearance.button(IMStrings.text("clearTrust")) { [weak self] in
        self?.clearTrust()
    }.byAccessibilityIdentifier("auth.clearTrust")
    private lazy var demoRow = IMUI.stack(spacing: 12).byAxis(.horizontal).byAlignment(.center)
    private lazy var demoTitle = IMAuthAppearance.label(IMStrings.text("authDemoMode"), size: 14, strong: true)
    private lazy var demoHelp = IMAuthAppearance.label(IMStrings.text("authDemoHint"))
    private lazy var reauthenticate = IMAuthAppearance.button(IMStrings.text("authVerifyAgain")) { [weak self] in
        guard let self, !self.working else {
            return
        }
        self.onReauthenticate(self)
    }.byAccessibilityIdentifier("auth.settings.reauthenticate")
    private lazy var demoSwitch = UISwitch.jobsMake { [unowned self] control in
        control.byOn(self.demoEnabled)
            .byOnTintColor(IMUI.greenFill)
            .byAccessibilityLabel(IMStrings.text("authDemoMode"))
            .byAccessibilityIdentifier("auth.settings.demo")
            .byAddAction(for: .valueChanged) { [weak self] sender in
                self?.changeDemo(sender.isOn)
            }
    }

    init(
        demoEnabled: Bool, onDemoChange: @escaping (Bool) async throws -> Void,
        onContextChanged: @escaping () -> Void,
        onReauthenticate: @escaping (IMAuthSettingsVC) -> Void
    ) {
        self.demoEnabled = demoEnabled
        self.onDemoChange = onDemoChange
        self.onContextChanged = onContextChanged
        self.onReauthenticate = onReauthenticate
        super.init(nibName: nil, bundle: nil)
    }

    required init?(coder: NSCoder) {
        fatalError("init(coder:) has not been implemented")
    }

    override func viewDidLoad() {
        screenTitle = IMStrings.text("authSettings")
        super.viewDidLoad()
        scroll.byAddTo(content) { [unowned self] make in
            make.top.leading.trailing.equalToSuperview()
            make.bottom.equalTo(self.view.keyboardLayoutGuide.snp.top)
        }
        stack.byAddTo(scroll) { [unowned self] make in
            make.edges.equalTo(self.scroll.contentLayoutGuide).inset(20)
            make.width.equalTo(self.scroll.frameLayoutGuide).offset(-40)
        }
        address.byText(session.baseURL?.absoluteString ?? "")
            .byKeyboardType(.URL)
        demoRow.byAddArrangedSubviews([demoTitle, demoSwitch])
        stack.byAddArrangedSubviews([connectionTitle, address, connect, status, importCA, removeCA,
                                     demoRow, demoHelp, reauthenticate])
        let controls: [UIView] = [address, connect]
        for field in controls {
            field.snp.makeConstraints {
                $0.height.greaterThanOrEqualTo(48)
            }
        }
    }

    private func setWorking(_ value: Bool) {
        working = value
        address.byEnabled(!value)
        for button in [connect, importCA, removeCA, reauthenticate] {
            button.byEnabled(!value)
        }
        demoSwitch.byEnabled(!value)
    }

    private func changeDemo(_ enabled: Bool) {
        guard !working else {
            demoSwitch.byOn(demoEnabled)
            return
        }
        setWorking(true)
        status.byText(IMStrings.text(enabled ? "demoStarting" : "connect"))
        perform { [weak self] in
            guard let self else {
                return
            }
            defer {
                self.setWorking(false)
            }
            do {
                try await self.onDemoChange(enabled)
                self.demoEnabled = enabled
                self.demoSwitch.byOn(enabled)
                self.address.byText(self.session.baseURL?.absoluteString ?? "")
                self.status.byText(IMStrings.text(enabled ? "authDemoReady" : "connectionNeeded"))
                self.onContextChanged()
            } catch {
                self.demoSwitch.byOn(self.demoEnabled)
                self.status.byText(error.localizedDescription)
                throw error
            }
        }
    }

    private func verifyConnection() {
        guard !working else {
            return
        }
        view.byEndEditing(true)
        let target = address.text ?? ""
        setWorking(true)
        perform { [weak self] in
            guard let self else {
                return
            }
            defer {
                self.setWorking(false)
            }
            self.status.byText(IMStrings.text("connect"))
            let descriptor = try await self.session.connect(target)
            let confirmation = UIAlertController.make(
                title: descriptor["name"].text,
                message: descriptor["deploymentId"].text + "\n" + descriptor["verifiedURL"].text
                    + "\nSHA256 " + descriptor["trustFingerprint"].text,
                style: .alert)
            confirmation.addAction(UIAlertAction.make(IMStrings.text("cancel"), style: .cancel) {})
            confirmation.addAction(UIAlertAction.make(IMStrings.text("connect")) { [weak self] in
                guard let self else {
                    return
                }
                self.session.useInstance(descriptor)
                self.status.byText(IMStrings.text("connected"))
                self.onContextChanged()
            })
            IMUI.presentDialog(confirmation, on: self)
        }
    }

    private func clearTrust() {
        guard !working else {
            return
        }
        let target = address.text ?? ""
        IMUI.confirm(self, title: IMStrings.text("clearTrust"), message: target) { [weak self] in
            guard let self else {
                return
            }
            self.setWorking(true)
            self.perform { [weak self] in
                guard let self else {
                    return
                }
                defer {
                    self.setWorking(false)
                }
                IMTrustStore.shared.remove(target)
                if self.session.baseURL?.absoluteString == target {
                    await self.session.logout()
                    self.session.api = self.session.baseURL.map {
                        IMAPIClient(baseURL: IMSession.apiURL($0))
                    }
                }
                self.status.byText(IMStrings.text("connectionNeeded"))
                self.onContextChanged()
            }
        }
    }

    func documentPicker(_ controller: UIDocumentPickerViewController, didPickDocumentsAt urls: [URL]) {
        guard let file = urls.first, !working else {
            return
        }
        let target = address.text ?? ""
        perform { [weak self] in
            guard let self else {
                return
            }
            guard let size = try file.resourceValues(forKeys: [.fileSizeKey]).fileSize,
                size > 0, size < 65536
            else {
                throw IMAPIError(status: 0, code: "INVALID_CA", message: "Certificate size invalid")
            }
            let bytes = try Data(contentsOf: file)
            IMUI.prompt(self, title: IMStrings.text("trustFingerprint")) { [weak self] expected in
                guard let self else {
                    return
                }
                self.perform {
                    let certificate = try IMInstanceCertificate(
                        address: target, certificate: bytes, verifiedFingerprint: expected)
                    IMUI.confirm(self, title: IMStrings.text("localCA"), message: target + "\nSHA256 " + certificate.fingerprint) {
                        self.perform {
                            try IMTrustStore.shared.save(certificate)
                            self.status.byText(IMStrings.text("connect"))
                        }
                    }
                }
            }
        }
    }
}
