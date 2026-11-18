//
//  IMAuthVC.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import JobsByUIKit
import JobsIMTransport
import JobsSwiftBaseDefines
import JobsSwiftDSL
import SnapKit
import UIKit

@MainActor
final class IMAuthVC: IMBaseVC, UIDocumentPickerDelegate {

    private var mode = "login"
    private var requestID = UUID().uuidString
    private var inputSignature = ""
    private lazy var address = IMUI.field(IMStrings.text("serviceURL"))
    private lazy var username = IMUI.field(IMStrings.text("username"))
    private lazy var password = IMUI.field(IMStrings.text("password"), secure: true)
    private lazy var otp = IMUI.field(IMStrings.text("authOTP"), secure: true)
    private lazy var code = IMUI.field(IMStrings.text("recoveryCode"))
    private lazy var status = IMUI.label(
        IMStrings.text("connectionNeeded"), style: .footnote, color: IMUI.secondary)
    private lazy var primary = IMUI.button(IMStrings.text("login"), primary: true) { [weak self] in
        self?.submit()
    }
    private lazy var reauthenticate = IMUI.button(IMStrings.text("authVerifyAgain")) { [weak self] in
        guard let self else { return }
        IMUI.confirm(
            self, title: IMStrings.text("authVerifyAgain"), message: IMStrings.text("authVerifyExplanation")
        ) {
            self.submit(explicitNew: true)
        }
    }

    override func viewDidLoad() {
        screenTitle = "JobsIM"
        super.viewDidLoad()
        assemble()
    }

    private func assemble() {
        let scroll = UIScrollView.jobsMake { scroll in
            scroll.byKeyboardDismissMode(.interactive)
        }
        scroll.byAddTo(content) { [unowned self] make in
            make.top.leading.trailing.equalToSuperview()
            make.bottom.equalTo(self.view.keyboardLayoutGuide.snp.top)
        }
        let stack = IMUI.stack(spacing: 18)
        stack.byAddTo(scroll) { [unowned self] make in
            make.edges.equalTo(scroll.contentLayoutGuide).inset(26)
            make.width.equalTo(scroll.frameLayoutGuide).offset(-52)
        }
        let title = IMUI.label(IMStrings.text("about"), style: .largeTitle)
        stack.byAddArrangedSubviews([title, status, address])
        address.byText(session.baseURL?.absoluteString ?? "").byAccessibilityIdentifier("auth.address")
        username.byAccessibilityIdentifier("auth.username")
        password.byAccessibilityIdentifier("auth.password")
        otp.byAccessibilityIdentifier("auth.otp").byTextContentType(.oneTimeCode).byKeyboardType(.numberPad)
        reauthenticate.byAccessibilityIdentifier("auth.reauthenticate")
        primary.byAccessibilityIdentifier("auth.primary")
        let resumePassword = IMUI.button(IMStrings.text("passwordChangeCheck")) { [weak self] in
            guard let self, !(self.username.text ?? "").trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
                return
            }
            self.navigationController?.pushViewController(IMPasswordChangeVC(username: self.username.text ?? ""), animated: true)
        }
        stack.byAddArrangedSubview(resumePassword)
        stack.byAddArrangedSubview(IMUI.button(IMStrings.text("deletionCheck")) { [weak self] in
            guard let self, !(self.username.text ?? "").trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
                return
            }
            self.navigationController?.pushViewController(IMAccountDeletionVC(username: self.username.text ?? ""), animated: true)
        })
        let connect = IMUI.button(IMStrings.text("connect")) { [weak self] in
            self?.verifyConnection()
        }
        connect.byAccessibilityIdentifier("auth.connect")
        stack.byAddArrangedSubviews([
            connect,
            IMUI.button(IMStrings.text("localCA")) { [weak self] in
                guard let self else {
                    return
                }
                let picker = UIDocumentPickerViewController.make()
                picker.delegate = self
                self.present(picker, animated: true)
            }.byAccessibilityIdentifier("auth.ca"),
            IMUI.button(IMStrings.text("clearTrust")) { [weak self] in
                guard let self else {
                    return
                }
                IMUI.confirm(self, title: IMStrings.text("clearTrust"), message: self.address.text ?? "") {
                    self.perform {
                        let target = self.address.text ?? ""
                        IMTrustStore.shared.remove(target)
                        if self.session.baseURL?.absoluteString == target {
                            await self.session.logout()
                            self.session.api = self.session.baseURL.map {
                                IMAPIClient(baseURL: IMSession.apiURL($0))
                            }
                        }
                    }
                }
            }, username, password, otp, code, primary, reauthenticate,
        ])
        let register = IMUI.button(IMStrings.text("register")) { [weak self] in
            self?.selectMode("register")
        }
        let recover = IMUI.button(IMStrings.text("recover")) { [weak self] in
            self?.selectMode("recover")
        }
        let login = IMUI.button(IMStrings.text("login")) { [weak self] in
            self?.selectMode("login")
        }
        let demo = IMUI.button(IMStrings.text("openDemo")) { [weak self] in
            self?.session.enterDemo()
        }
        stack.byAddArrangedSubviews([register, recover, login, demo])
        for view in [
            address, username, password, otp, code, primary, reauthenticate, connect, register, recover,
            login, demo,
        ] {
            view.snp.makeConstraints { [unowned self] make in
                make.height.greaterThanOrEqualTo(50)
            }
        }
        code.byHidden(true)
        username.byTextContentType(.username)
        password.byTextContentType(.password)
        #if DEBUG
            if ProcessInfo.processInfo.arguments.contains("--ui-test-generated-password") {
                password.byText(UUID().uuidString + UUID().uuidString)
            }
        #endif
    }

    private func selectMode(_ value: String) {
        mode = value
        requestID = UUID().uuidString
        primary.byTitle(IMStrings.text(value))
        code.byHidden(value != "recover")
        otp.byHidden(value != "login")
        reauthenticate.byHidden(value != "login")
        password.byTextContentType(value == "login" ? .password : .newPassword)
    }

    private func verifyConnection() {
        perform { [weak self] in
            guard let self else {
                return
            }
            self.status.byText(IMStrings.text("connect"))
            let descriptor = try await self.session.connect(self.address.text ?? "")
            let confirm = UIAlertController.make(
                title: descriptor["name"].text,
                message: descriptor["deploymentId"].text + "\n" + descriptor["verifiedURL"].text,
                style: .alert)
            confirm.addAction(UIAlertAction.make(IMStrings.text("cancel"), style: .cancel) {})
            confirm.addAction(
                UIAlertAction.make(IMStrings.text("connect")) { [weak self] in
                    self?.session.useInstance(descriptor)
                    self?.status.byText(IMStrings.text("connected"))
                })
            self.present(confirm, animated: true)
        }
    }

    func documentPicker(_ controller: UIDocumentPickerViewController, didPickDocumentsAt urls: [URL]) {
        guard let file = urls.first else {
            return
        }
        perform { [weak self] in
            guard let self else {
                return
            }
            guard let size = try file.resourceValues(forKeys: [.fileSizeKey]).fileSize, size > 0, size < 65536
            else { throw IMAPIError(status: 0, code: "INVALID_CA", message: "Certificate size invalid") }
            let bytes = try Data(contentsOf: file)
            let target = self.address.text ?? ""
            IMUI.prompt(self, title: IMStrings.text("trustFingerprint")) { expected in
                self.perform {
                    let certificate = try IMInstanceCertificate(
                        address: target, certificate: bytes, verifiedFingerprint: expected)
                    IMUI.confirm(
                        self, title: IMStrings.text("localCA"),
                        message: target + "\nSHA256 " + certificate.fingerprint
                    ) {
                        self.perform {
                            try IMTrustStore.shared.save(certificate)
                            self.status.byText(IMStrings.text("connect"))
                        }
                    }
                }
            }
        }
    }

    private func submit(explicitNew: Bool = false, originalProofConfirmed: Bool = false) {
        let name = username.text ?? ""
        let secret = password.text ?? ""
        let recovery = code.text ?? ""
        let authenticator = mode == "login" ? (otp.text ?? "") : ""
        if !explicitNew, !originalProofConfirmed, inputSignature.isEmpty,
            session.pendingAuthentication(name) != nil
        {
            IMUI.confirm(
                self, title: IMStrings.text("login"), message: IMStrings.text("authUnknownExplanation")
            ) {
                self.submit(originalProofConfirmed: true)
            }
            return
        }
        let signature =
            mode + "\u{0}" + name + "\u{0}" + secret + "\u{0}" + recovery + "\u{0}" + authenticator
        if signature != inputSignature || explicitNew {
            requestID = UUID().uuidString
            inputSignature = signature
        }
        view.byEndEditing(true)
        primary.byEnabled(false)
        perform { [weak self] in
            guard let self else {
                return
            }
            defer {
                self.primary.byEnabled(true)
            }
            do {
                _ = try await self.session.authenticate(
                    mode: self.mode, username: name, password: secret, recoveryCode: recovery,
                    requestID: self.requestID, otp: authenticator, explicitReauthentication: explicitNew)
            } catch {
                if self.session.pendingAuthentication(name) != nil {
                    self.status.byText(IMStrings.text("authUnknownExplanation"))
                }
                throw error
            }
            self.password.byText("")
            self.otp.byText("")
            self.code.byText("")
            self.inputSignature = ""
            if self.mode == "recover" {
                self.selectMode("login")
                IMUI.alert(self, title: IMStrings.text("done"), message: IMStrings.text("login"))
            }
        }
    }
}
