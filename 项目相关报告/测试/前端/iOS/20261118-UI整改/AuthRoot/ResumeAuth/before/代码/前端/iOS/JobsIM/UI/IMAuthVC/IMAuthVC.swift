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
final class IMAuthVC: IMBaseVC, UIDocumentPickerDelegate, UITextFieldDelegate {

    private var mode = "login"
    private var requestID = UUID().uuidString
    private var inputSignature = ""
    private var appeared = false
    private var showsOTP = false
    private lazy var confirmPassword = IMUI.field(IMStrings.text("authConfirmPassword"), secure: true)
    private lazy var scroll = UIScrollView.jobsMake {
        $0.byKeyboardDismissMode(.interactive)
    }
    private lazy var panel = UIView.jobsMake {
        $0.byBackgroundColor(IMUI.panel).byCornerRadius(24).byClipsToBounds(true)
            .byBorderColor(IMUI.separator).byBorderWidth(1)
    }
    private lazy var form = IMUI.stack(spacing: 14)
    private lazy var cardTitle = IMUI.label(IMStrings.text("login"), style: .title1)
    private lazy var cardHelp = IMUI.label(IMStrings.text("authLoginHint"), style: .footnote, color: IMUI.secondary)
    private lazy var connectionPanel = IMUI.stack(spacing: 12)
    private lazy var rail = IMUI.button(IMStrings.text("authSwitchRegister"), primary: true) { [weak self] in
        guard let self else {
            return
        }
        self.selectMode(self.mode == "login" ? "register" : "login")
    }.byCornerRadius(0).byNumberOfLines(0).byTitleAlignment(.center).byTitleFont(IMUI.font(.footnote))
    private lazy var address = IMUI.field(IMStrings.text("serviceURL"))
    private lazy var username = IMUI.field(IMStrings.text("username"))
    private lazy var password = IMUI.field(IMStrings.text("password"), secure: true)
    private lazy var otp = IMUI.field(IMStrings.text("authOTP"), secure: true)
    private lazy var code = IMUI.field(IMStrings.text("recoveryCode"))
    private lazy var status = IMUI.label(
        session.lastError ?? IMStrings.text("connectionNeeded"), style: .footnote, color: IMUI.secondary)
    private lazy var primary = IMUI.button(IMStrings.text("login"), primary: true) { [weak self] in
        self?.submit()
    }
    private lazy var reauthenticate = IMUI.button(IMStrings.text("authVerifyAgain")) { [weak self] in
        guard let self else {
            return
        }
        IMUI.confirm(
            self, title: IMStrings.text("authVerifyAgain"), message: IMStrings.text("authVerifyExplanation")
        ) {
            self.submit(explicitNew: true)
        }
    }

    override func viewDidLoad() {
        screenTitle = ""
        hidesScreenHeader = true
        super.viewDidLoad()
        assemble()
    }

    private func assemble() {
        scroll.byAddTo(content) { [unowned self] make in
            make.top.leading.trailing.equalToSuperview()
            make.bottom.equalTo(self.view.keyboardLayoutGuide.snp.top)
        }
        let canvas = UIView.jobsMake {
            $0.byBackgroundColor(IMUI.background)
        }
        canvas.byAddTo(scroll) { make in
            make.edges.equalTo(self.scroll.contentLayoutGuide)
            make.width.equalTo(self.scroll.frameLayoutGuide)
        }
        let stack = IMUI.stack(spacing: 18)
        stack.byAddTo(canvas) { make in
            make.top.bottom.equalToSuperview().inset(24)
            make.centerX.equalToSuperview()
            make.leading.greaterThanOrEqualToSuperview().offset(20)
            make.trailing.lessThanOrEqualToSuperview().offset(-20)
            make.width.equalToSuperview().offset(-40).priority(.high)
            make.width.lessThanOrEqualTo(520)
        }
        let logo = UIImageView.jobsMake {
            $0.byImage(UIImage.make(named: "Logo")).byContentMode(.scaleAspectFit)
                .byAccessibilityIdentifier("auth.logo")
        }
        logo.snp.makeConstraints {
            $0.height.equalTo(72)
        }
        let tagline = IMUI.label(IMStrings.text("about"), style: .subheadline, color: IMUI.secondary)
            .byTextAlignment(.center)
        stack.byAddArrangedSubviews([logo, tagline, panel])
        form.byAddTo(panel) { make in
            make.top.bottom.equalToSuperview().inset(24)
            make.leading.equalToSuperview().offset(20)
            make.trailing.equalToSuperview().offset(-84)
        }
        rail.byAccessibilityIdentifier("auth.modeSwitch").byAddTo(panel) { make in
            make.top.trailing.bottom.equalToSuperview()
            make.width.equalTo(64)
        }
        username.byAccessibilityIdentifier("auth.username").byTextContentType(.username).byDelegate(self)
        password.byAccessibilityIdentifier("auth.password").byTextContentType(.password).byDelegate(self)
        confirmPassword.byAccessibilityIdentifier("auth.confirmPassword").byTextContentType(.newPassword).byDelegate(self)
        otp.byAccessibilityIdentifier("auth.otp").byTextContentType(.oneTimeCode).byKeyboardType(.numberPad).byDelegate(self)
        code.byAccessibilityIdentifier("auth.recoveryCode").byDelegate(self)
        primary.byAccessibilityIdentifier("auth.primary")
        form.byAddArrangedSubviews([cardTitle, cardHelp, username, password, confirmPassword, otp, code, primary])
        for field in [username, password, confirmPassword, otp, code] {
            field.snp.makeConstraints {
                $0.height.greaterThanOrEqualTo(52)
            }
        }
        primary.snp.makeConstraints {
            $0.height.greaterThanOrEqualTo(52)
        }
        let recover = IMUI.button(IMStrings.text("recover")) {
            [weak self] in self?.selectMode("recover")
        }
        recover.byAccessibilityIdentifier("auth.recover")
        let mfa = IMUI.button(IMStrings.text("authMFA")) { [weak self] in
            guard let self else {
                return
            }
            self.showsOTP.toggle()
            self.selectMode("login")
        }.byAccessibilityIdentifier("auth.mfa")
        stack.byAddArrangedSubviews([recover, mfa])
        let connection = IMUI.button(IMStrings.text("connection")) { [weak self] in
            guard let self else {
                return
            }
            self.connectionPanel.byHidden(!self.connectionPanel.isHidden)
        }.byAccessibilityIdentifier("auth.connectionOptions")
        stack.byAddArrangedSubviews([connection, status, connectionPanel])
        address.byText(session.baseURL?.absoluteString ?? "").byAccessibilityIdentifier("auth.address")
        let connect = IMUI.button(IMStrings.text("connect")) {
            [weak self] in self?.verifyConnection()
        }
            .byAccessibilityIdentifier("auth.connect")
        connectionPanel.byAddArrangedSubviews([address, connect,
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
            }
        ])
        connectionPanel.byHidden(session.baseURL != nil)
        address.snp.makeConstraints {
            $0.height.greaterThanOrEqualTo(52)
        }
        let help = IMUI.stack(spacing: 8)
        reauthenticate.byAccessibilityIdentifier("auth.reauthenticate")
        help.byAddArrangedSubviews([reauthenticate,
            IMUI.button(IMStrings.text("passwordChangeCheck")) { [weak self] in
                guard let self, !(self.username.text ?? "").trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
                    return
                }
                self.navigationController?.pushViewController(IMPasswordChangeVC(username: self.username.text ?? ""), animated: true)
            },
            IMUI.button(IMStrings.text("deletionCheck")) { [weak self] in
                guard let self, !(self.username.text ?? "").trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
                    return
                }
                self.navigationController?.pushViewController(IMAccountDeletionVC(username: self.username.text ?? ""), animated: true)
            }
        ])
        help.byHidden(true)
        stack.byAddArrangedSubviews([
            IMUI.button(IMStrings.text("authHelp")) {
                help.byHidden(!help.isHidden)
            }, help,
            IMUI.button(IMStrings.text("openDemo")) {
                [weak self] in self?.session.enterDemo()
            }
                .byAccessibilityIdentifier("auth.demo")
        ])
        selectMode("login", animated: false)
        #if DEBUG
        if ProcessInfo.processInfo.arguments.contains("--ui-test-generated-password") {
            password.byText(UUID().uuidString + UUID().uuidString)
        }
        #endif
    }

    override func viewDidAppear(_ animated: Bool) {
        super.viewDidAppear(animated)
        guard !appeared else {
            return
        }
        appeared = true
        guard !UIAccessibility.isReduceMotionEnabled else {
            return
        }
        panel.byTransform(CGAffineTransform(scaleX: 0.01, y: 0.01))
        UIView.jobsAnimateKeyframes(1, options: [.beginFromCurrentState]) { [weak self] in
            UIView.jobsAddKeyframe(withRelativeStartTime: 0, relativeDuration: 0.65) {
                self?.panel.byTransform(CGAffineTransform(scaleX: 1.1, y: 1.1))
            }
            UIView.jobsAddKeyframe(withRelativeStartTime: 0.65, relativeDuration: 0.35) {
                self?.panel.byTransform(.identity)
            }
        }
    }

    private func selectMode(_ value: String, animated: Bool = true) {
        view.byEndEditing(true)
        view.layoutIfNeeded()
        mode = value
        requestID = UUID().uuidString
        inputSignature = ""
        let registration = value == "register"
        primary.byTitle(IMStrings.text(value))
        cardTitle.byText(IMStrings.text(value))
        cardHelp.byText(IMStrings.text(registration ? "authRegisterHint" : value == "recover" ? "authRecoverHint" : "authLoginHint"))
        confirmPassword.byHidden(!registration)
        code.byHidden(value != "recover")
        otp.byHidden(value != "login" || !showsOTP)
        reauthenticate.byHidden(value != "login")
        password.byTextContentType(value == "login" ? .password : .newPassword)
        rail.byTitle(IMStrings.text(value == "login" ? "authSwitchRegister" : "authSwitchLogin"))
        form.snp.remakeConstraints { make in
            make.top.bottom.equalToSuperview().inset(24)
            make.leading.equalToSuperview().offset(registration ? 84 : 20)
            make.trailing.equalToSuperview().offset(registration ? -20 : -84)
        }
        rail.snp.remakeConstraints { make in
            make.top.bottom.equalToSuperview()
            make.width.equalTo(64)
            if registration { make.leading.equalToSuperview() } else { make.trailing.equalToSuperview() }
        }
        if animated && !UIAccessibility.isReduceMotionEnabled {
            UIView.jobsAnimateWithSpring(0.7, delay: 0.1, dampingRatio: 1, initialVelocity: 0.1,
                                        options: [.curveEaseInOut, .beginFromCurrentState]) { [weak self] in
                self?.view.layoutIfNeeded()
            }
        } else {
            view.layoutIfNeeded()
        }
    }

    func textFieldDidBeginEditing(_ textField: UITextField) {
        DispatchQueue.main.async { [weak self, weak textField] in
            guard let self, let textField else {
                return
            }
            self.scroll.byScrollRectToVisible(textField.convert(textField.bounds, to: self.scroll).insetBy(dx: 0, dy: -20), animated: true)
        }
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
        if mode == "register", password.text != confirmPassword.text {
            IMUI.alert(self, message: IMStrings.text("authPasswordMismatch"))
            return
        }
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
            self.confirmPassword.byText("")
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
