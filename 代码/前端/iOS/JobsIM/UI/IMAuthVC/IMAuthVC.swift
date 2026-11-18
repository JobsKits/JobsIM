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
final class IMAuthVC: IMBaseVC, UITextFieldDelegate {

    private var mode = "login"
    private var requestID = UUID().uuidString
    private var inputSignature = ""
    private var appeared = false
    private var showsOTP = false
    private var challengeProof = ""
    private var challengeRequiresReauthentication = false
    private var submitting = false
    private var remembersPassword = false
    private var restoredUsername: String?
    private var contextSignature = ""
    private var promptedForConnection = false
    private lazy var background = IMAuthBackgroundView()
    private lazy var scroll = UIScrollView.jobsMake {
        $0.byScrollEnabled(false)
            .byBounces(false)
            .byAlwaysBounceVertical(false)
            .byAccessibilityIdentifier("auth.cardScroll")
    }
    private lazy var canvas = UIView.jobsMake {
        $0.byBackgroundColor(JobsCor.clear)
    }
    private lazy var panel = IMAuthGlassView()
    private lazy var form = IMUI.stack(spacing: 12)
    private lazy var logo = UIImageView.jobsMake {
        $0.byImage(UIImage.make(named: "Logo"))
            .byContentMode(.scaleAspectFit)
            .byAccessibilityIdentifier("auth.logo")
    }
    private lazy var cardTitle = IMAuthAppearance.label(IMStrings.text("login"), size: 18, strong: true)
        .byTextAlignment(.center)
        .byAccessibilityIdentifier("auth.cardTitle")
    private lazy var cardHelp = IMAuthAppearance.label(IMStrings.text("authBrandHint"))
        .byTextAlignment(.center)
        .byAccessibilityIdentifier("auth.cardSubtitle")
    private lazy var rail = IMAuthAppearance.button(IMStrings.text("authSwitchRegister"), primary: true) { [weak self] in
        guard let self else {
            return
        }
        self.selectMode(self.mode == "login" ? "register" : "login")
    }.byConfiguration {
        $0.byBackground($0.background.byCornerRadius(0))
    }.byTitleAlignment(.center)
        .byTitleFont(IMAuthAppearance.font(18, weight: .semibold))
        .byAccessibilityIdentifier("auth.modeSwitch")
    private lazy var username = IMAuthTextField("username")
        .byAuthIdentifier("auth.username")
        .byTextContentType(.username)
        .byDelegate(self)
        .byAddAction(for: .editingChanged) { [weak self] _ in
            self?.usernameChanged()
        }
    private lazy var password = IMAuthTextField("password", secure: true)
        .byAuthIdentifier("auth.password")
        .byTextContentType(.password)
        .byDelegate(self)
        .byAddAction(for: .editingChanged) { [weak self] _ in
            self?.invalidateChangedChallenge()
        }
    private lazy var confirmPassword = IMAuthTextField("authConfirmPassword", secure: true)
        .byAuthIdentifier("auth.confirmPassword")
        .byTextContentType(.newPassword)
        .byDelegate(self)
    private lazy var otp = IMAuthTextField("authOTPChallenge")
        .byAuthIdentifier("auth.otp")
        .byTextContentType(.oneTimeCode)
        .byKeyboardType(.numberPad)
        .byDelegate(self)
    private lazy var code = IMAuthTextField("recoveryCode")
        .byAuthIdentifier("auth.recoveryCode")
        .byDelegate(self)
    private lazy var assistance = IMUI.stack(spacing: 8)
        .byAxis(.horizontal)
        .byDistribution(.fillEqually)
    private lazy var remember = IMAuthAppearance.button(IMStrings.text("authRememberPassword"), compact: true) { [weak self] in
        self?.toggleRemember()
    }.byContentHorizontalAlignment(.leading)
        .byTitleAlignment(.left)
        .byAccessibilityIdentifier("auth.rememberPassword")
    private lazy var recover = IMAuthAppearance.button(IMStrings.text("recover"), compact: true) { [weak self] in
        self?.selectMode("recover")
    }.byContentHorizontalAlignment(.trailing)
        .byTitleAlignment(.right)
        .byAccessibilityIdentifier("auth.recover")
    private lazy var status = IMAuthAppearance.label(session.lastError ?? "")
        .byAccessibilityIdentifier("auth.status")
    private lazy var primary = IMAuthAppearance.button(IMStrings.text("login"), primary: true) { [weak self] in
        self?.submit()
    }.byAccessibilityIdentifier("auth.primary")
    private lazy var settings = IMAuthAppearance.button("") { [weak self] in
        self?.openSettings()
    }.byImage("gearshape".sysImg)
        .byConfiguration {
            $0.byBackground(UIBackgroundConfiguration.clear().byBackgroundColor(IMUI.panel).byCornerRadius(22))
        }
        .byAccessibilityLabel(IMStrings.text("authSettings"))
        .byAccessibilityIdentifier("auth.settings")

    private var currentUsername: String {
        return (username.text ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
    }

    override func viewDidLoad() {
        screenTitle = ""
        hidesScreenHeader = true
        super.viewDidLoad()
        assemble()
        refreshContext(force: true)
    }

    private func assemble() {
        background.byAddTo(view) {
            $0.edges.equalToSuperview()
        }
        view.byInsertSubview(background, below: content)
        content.byBackgroundColor(JobsCor.clear)
        scroll.byAddTo(content) { [unowned self] make in
            make.top.leading.trailing.equalToSuperview()
            make.bottom.equalTo(self.view.keyboardLayoutGuide.snp.top)
        }
        canvas.byAddTo(scroll) { [unowned self] make in
            make.edges.equalTo(self.scroll.contentLayoutGuide)
            make.width.equalTo(self.scroll.frameLayoutGuide)
            make.height.greaterThanOrEqualTo(self.scroll.frameLayoutGuide).priority(.high)
        }
        panel.byAddTo(canvas) { make in
            make.centerX.centerY.equalToSuperview()
            make.top.greaterThanOrEqualToSuperview().offset(72)
            make.top.equalToSuperview().offset(72).priority(.low)
            make.bottom.lessThanOrEqualToSuperview().offset(-24)
            make.leading.greaterThanOrEqualToSuperview().offset(20)
            make.trailing.lessThanOrEqualToSuperview().offset(-20)
            make.width.equalToSuperview().offset(-40).priority(.high)
            make.width.lessThanOrEqualTo(480)
        }
        form.byAddTo(panel) { make in
            make.top.bottom.equalToSuperview().inset(20)
            make.leading.equalToSuperview().offset(16)
            make.trailing.equalToSuperview().offset(-80)
        }
        rail.byAddTo(panel) { make in
            make.top.trailing.bottom.equalToSuperview()
            make.width.equalTo(64)
        }
        settings.byAddTo(content) { make in
            make.top.equalToSuperview().offset(4)
            make.trailing.equalToSuperview().offset(-16)
            make.size.equalTo(44)
        }
        logo.snp.makeConstraints {
            $0.height.equalTo(44)
        }
        assistance.byAddArrangedSubviews([remember, recover])
        form.byAddArrangedSubviews([logo, cardTitle, cardHelp, username, password, assistance,
                                    confirmPassword, otp, code, primary, status])
        for field in [username, password, confirmPassword, otp, code] {
            field.snp.makeConstraints {
                $0.height.greaterThanOrEqualTo(46)
            }
        }
        for button in [primary, remember, recover] {
            button.snp.makeConstraints {
                $0.height.greaterThanOrEqualTo(44)
            }
        }
        status.byHidden(session.lastError == nil)
        selectMode("login", animated: false)
        #if DEBUG
            if ProcessInfo.processInfo.arguments.contains("--ui-test-generated-password") {
                password.byText(UUID().uuidString + UUID().uuidString)
            }
        #endif
    }

    override func viewWillAppear(_ animated: Bool) {
        super.viewWillAppear(animated)
        NotificationCenter.default.removeObserver(
            self, name: UIResponder.keyboardDidChangeFrameNotification, object: nil)
        NotificationCenter.default.addObserver(
            self, selector: #selector(keyboardFrameChanged(_:)),
            name: UIResponder.keyboardDidChangeFrameNotification, object: nil)
        refreshContext()
        background.setActive(true)
    }

    override func viewDidAppear(_ animated: Bool) {
        super.viewDidAppear(animated)
        if session.api == nil && !promptedForConnection {
            showConnectionAlert()
        }
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

    override func viewWillDisappear(_ animated: Bool) {
        NotificationCenter.default.removeObserver(
            self, name: UIResponder.keyboardDidChangeFrameNotification, object: nil)
        background.setActive(false)
        super.viewWillDisappear(animated)
    }

    private func refreshContext(force: Bool = false) {
        let context = session.profile + "\u{0}" + session.deployment + "\u{0}" + (session.baseURL?.absoluteString ?? "")
        guard force || context != contextSignature else {
            return
        }
        contextSignature = context
        requestID = UUID().uuidString
        inputSignature = ""
        restoredUsername = nil
        remembersPassword = false
        promptedForConnection = false
        clearChallenge()
        status.byText(session.lastError ?? "").byHidden(session.lastError == nil)
        for field in [username, password, confirmPassword, otp, code] {
            field.byText("")
        }
        password.setPasswordVisible(false)
        confirmPassword.setPasswordVisible(false)
        selectMode("login", animated: false)
        if let candidate = IMDemoEnvironmentService.shared.currentCandidate, candidate.profile == session.profile {
            username.byText(candidate.username)
            password.byText(candidate.password)
            password.setPasswordVisible(true)
            status.byText(IMStrings.text("authDemoReady")).byHidden(false)
        } else if !session.profile.isEmpty, !session.isDemonstrationEnvironment {
            do {
                if let candidate = try IMRememberedLoginStore.shared.restore(for: session.profile) {
                    username.byText(candidate.username)
                    password.byText(candidate.password)
                    restoredUsername = candidate.username
                    remembersPassword = true
                }
            } catch {
                status.byText(error.localizedDescription).byHidden(false)
            }
        }
        refreshRemember()
    }

    private func usernameChanged() {
        invalidateChangedChallenge()
        if let restoredUsername, currentUsername != restoredUsername {
            password.byText("")
            self.restoredUsername = nil
            remembersPassword = false
            refreshRemember()
        }
    }

    private func toggleRemember() {
        guard !submitting, !session.isDemonstrationEnvironment else {
            return
        }
        remembersPassword.toggle()
        if !remembersPassword, !session.profile.isEmpty, !currentUsername.isEmpty {
            do {
                try IMRememberedLoginStore.shared.forget(for: session.profile, username: currentUsername)
            } catch {
                remembersPassword = true
                IMUI.alert(self, message: error.localizedDescription)
            }
        }
        refreshRemember()
    }

    private func refreshRemember() {
        remember.bySelected(remembersPassword)
            .byImage((remembersPassword ? "checkmark.circle.fill" : "circle").sysImg(pointSize: 19))
            .byConfiguration {
                $0.byPreferredSymbolConfigurationForImage(IMAuthAppearance.smallIconConfiguration)
            }
            .byEnabled(!submitting && !session.isDemonstrationEnvironment)
    }

    private func openSettings() {
        guard !submitting else {
            return
        }
        view.byEndEditing(true)
        let controller = IMAuthSettingsVC(
            demoEnabled: session.isDemonstrationEnvironment,
            onDemoChange: { enabled in
                try await IMDemoEnvironmentService.shared.setEnabled(enabled)
            },
            onContextChanged: { [weak self] in
                self?.refreshContext(force: true)
            },
            onReauthenticate: { [weak self] controller in
                self?.verifyIdentity(from: controller)
            })
        navigationController?.pushViewController(controller, animated: true)
    }

    private func verifyIdentity(from controller: IMAuthSettingsVC) {
        guard !submitting else {
            return
        }
        guard !currentUsername.isEmpty else {
            IMUI.prompt(controller, title: IMStrings.text("username")) { [weak self, weak controller] name in
                guard let self, let controller else {
                    return
                }
                self.username.byText(name)
                self.usernameChanged()
                if !self.currentUsername.isEmpty {
                    self.verifyIdentity(from: controller)
                }
            }
            return
        }
        do {
            let deletions = try IMAccountDeletionService(username: currentUsername).rows().filter { row in
                !["unsent", "rejected", "cancelled", "retired"].contains(row["state"].text)
                    && (row["state"].text != "completed" || row["cleanup"] != .bool(true))
            }
            let changes = try IMPasswordChangeService(username: currentUsername).rows().filter {
                ["unknown", "pending", "changed"].contains($0["state"].text)
            }
            let deletion = deletions.first
            let change = changes.first
            let changeDate = change?["createdAt"].text ?? ""
            if let deletion, change == nil || deletion["createdAt"].text >= changeDate {
                controller.navigationController?.pushViewController(
                    IMAccountDeletionVC(username: currentUsername, verificationOnly: true), animated: true)
                return
            }
            if change != nil {
                controller.navigationController?.pushViewController(
                    IMPasswordChangeVC(username: currentUsername, verificationOnly: true), animated: true)
                return
            }
        } catch {
            if !session.isDemonstrationEnvironment {
                IMUI.alert(controller, message: error.localizedDescription)
                return
            }
        }
        IMUI.confirm(controller, title: IMStrings.text("authVerifyAgain"), message: IMStrings.text("authVerifyExplanation")) { [weak self] in
            guard let self else {
                return
            }
            self.navigationController?.popToViewController(self, animated: false)
            self.selectMode("login", animated: false)
            self.submit(explicitNew: true)
        }
    }

    private func showConnectionAlert() {
        promptedForConnection = true
        let alert = UIAlertController.make(title: IMStrings.text("connection"), message: IMStrings.text("authConnectionUnavailable"), style: .alert)
        alert.addAction(UIAlertAction.make(IMStrings.text("cancel"), style: .cancel) {})
        alert.addAction(UIAlertAction.make(IMStrings.text("authOpenSettings")) { [weak self] in
            self?.openSettings()
        })
        IMUI.presentDialog(alert, on: self)
    }

    private func selectMode(_ value: String, animated: Bool = true) {
        guard !submitting else {
            return
        }
        view.byEndEditing(true)
        if view.window != nil {
            view.layoutIfNeeded()
        }
        mode = value
        requestID = UUID().uuidString
        inputSignature = ""
        let registration = value == "register"
        primary.byTitle(IMStrings.text(value))
        cardTitle.byText(IMStrings.text(value))
        cardHelp.byText(IMStrings.text(value == "recover" ? "authRecoverHint" : "authBrandHint"))
        confirmPassword.byHidden(!registration)
        assistance.byHidden(value != "login")
        code.byHidden(value != "recover")
        otp.byHidden(value != "login" || !showsOTP)
        if value != "login" {
            clearChallenge()
        }
        refreshReturnKeys()
        password.byTextContentType(value == "login" ? .password : .newPassword)
        rail.byTitle(IMStrings.text(value == "login" ? "authSwitchRegister" : "authSwitchLogin"))
        form.snp.remakeConstraints { make in
            make.top.bottom.equalToSuperview().inset(20)
            make.leading.equalToSuperview().offset(registration ? 80 : 16)
            make.trailing.equalToSuperview().offset(registration ? -16 : -80)
        }
        rail.snp.remakeConstraints { make in
            make.top.bottom.equalToSuperview()
            make.width.equalTo(64)
            if registration {
                make.leading.equalToSuperview()
            } else {
                make.trailing.equalToSuperview()
            }
        }
        guard view.window != nil else {
            return
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

    private var visibleInputFields: [UITextField] {
        return [username, password, confirmPassword, otp, code].filter { !$0.isHidden }
    }

    private func refreshReturnKeys() {
        let fields = visibleInputFields
        for (index, field) in fields.enumerated() {
            field.byReturnKeyType(index == fields.count - 1 ? .done : .next)
        }
    }

    func textFieldShouldReturn(_ textField: UITextField) -> Bool {
        guard !submitting else {
            return false
        }
        let fields = visibleInputFields
        if let index = fields.firstIndex(where: { $0 === textField }), fields.indices.contains(index + 1) {
            fields[index + 1].becomeFirstResponder()
            revealFocusedField()
        } else {
            view.byEndEditing(true)
        }
        return false
    }

    @objc private func keyboardFrameChanged(_ notification: Notification) {
        revealFocusedField()
    }

    private func revealFocusedField() {
        guard view.window != nil else {
            return
        }
        view.layoutIfNeeded()
        if let field = [username, password, confirmPassword, otp, code].first(where: { $0.isFirstResponder }) {
            scroll.byScrollRectToVisible(field.convert(field.bounds, to: scroll).insetBy(dx: 0, dy: -20), animated: true)
        } else {
            scroll.byContentOffsetByAnimated(.zero)
        }
    }

    private func clearChallenge() {
        showsOTP = false
        challengeProof = ""
        challengeRequiresReauthentication = false
        otp.byText("").byHidden(true)
        refreshReturnKeys()
    }

    private func invalidateChangedChallenge() {
        let signature = contextSignature + "\u{0}" + currentUsername + "\u{0}" + (password.text ?? "")
        if showsOTP, challengeProof != signature {
            clearChallenge()
        }
    }

    func textFieldDidEndEditing(_ textField: UITextField) {
        guard textField === username, mode == "login", !submitting,
            !session.isDemonstrationEnvironment, !session.profile.isEmpty,
            !currentUsername.isEmpty, (password.text ?? "").isEmpty
        else {
            return
        }
        do {
            if let candidate = try IMRememberedLoginStore.shared.restore(for: session.profile, username: currentUsername) {
                password.byText(candidate.password)
                restoredUsername = candidate.username
                remembersPassword = true
                refreshRemember()
            }
        } catch {
            status.byText(error.localizedDescription).byHidden(false)
        }
    }

    private func setSubmitting(_ value: Bool) {
        submitting = value
        for field in [username, password, confirmPassword, otp, code] {
            field.byEnabled(!value)
        }
        for button in [primary, rail, settings, recover] {
            button.byEnabled(!value)
        }
        refreshRemember()
    }

    private func submit(explicitNew: Bool = false, originalProofConfirmed: Bool = false) {
        guard !submitting else {
            return
        }
        guard session.api != nil, session.baseURL != nil, !session.profile.isEmpty else {
            showConnectionAlert()
            return
        }
        view.byEndEditing(true)
        if mode == "register", password.text != confirmPassword.text {
            IMUI.alert(self, message: IMStrings.text("authPasswordMismatch"))
            return
        }
        let name = username.text ?? ""
        let secret = password.text ?? ""
        let recovery = code.text ?? ""
        let authenticator = mode == "login" ? (otp.text ?? "") : ""
        let newProof = explicitNew || showsOTP && challengeRequiresReauthentication
        if !newProof, !originalProofConfirmed, inputSignature.isEmpty,
            session.pendingAuthentication(name) != nil
        {
            IMUI.confirm(self, title: IMStrings.text("login"), message: IMStrings.text("authUnknownExplanation")) {
                self.submit(originalProofConfirmed: true)
            }
            return
        }
        let signature = contextSignature + "\u{0}" + mode + "\u{0}" + name + "\u{0}" + secret + "\u{0}" + recovery + "\u{0}" + authenticator
        if signature != inputSignature || newProof {
            requestID = UUID().uuidString
            inputSignature = signature
        }
        let submittedMode = mode, submittedProfile = session.profile, submittedDeployment = session.deployment
        let submittedAddress = session.baseURL, submittedRequest = requestID, savePassword = remembersPassword
        let presentationWindow = view.window
        setSubmitting(true)
        Task { [weak self] in
            guard let self else {
                return
            }
            defer {
                self.setSubmitting(false)
            }
            do {
                _ = try await self.session.authenticate(
                    mode: submittedMode, username: name, password: secret, recoveryCode: recovery,
                    requestID: submittedRequest, otp: authenticator, explicitReauthentication: newProof)
                guard self.session.profile == submittedProfile, self.session.deployment == submittedDeployment,
                    self.session.baseURL == submittedAddress
                else {
                    return
                }
                if submittedMode == "login", !self.session.isDemonstrationEnvironment,
                    let pair = self.session.currentCredentialPair, pair.profile == submittedProfile,
                    pair.deploymentID == submittedDeployment, pair.userID == self.session.user["id"].text {
                    do {
                        try IMRememberedLoginStore.shared.persistAfterVerifiedAuthentication(
                            for: submittedProfile, username: name, password: secret, enabled: savePassword)
                    } catch {
                        if let controller = presentationWindow?.rootViewController {
                            IMUI.alert(controller, message: error.localizedDescription)
                        }
                    }
                }
                self.password.byText("")
                self.confirmPassword.byText("")
                self.otp.byText("")
                self.code.byText("")
                self.inputSignature = ""
                self.clearChallenge()
                if submittedMode == "recover" {
                    self.setSubmitting(false)
                    self.selectMode("login")
                    IMUI.alert(self, title: IMStrings.text("done"), message: IMStrings.text("login"))
                }
            } catch {
                guard self.session.profile == submittedProfile, self.session.deployment == submittedDeployment,
                    self.session.baseURL == submittedAddress
                else {
                    return
                }
                if submittedMode == "login", let failure = error as? IMAPIError, failure.status == 403,
                    self.session.pendingAuthentication(name) == nil || newProof,
                    ["MFA_REQUIRED", "MFA_INVALID", "MFA_REPLAY", "MFA_NOT_ENROLLED"].contains(failure.code)
                {
                    if failure.code == "MFA_NOT_ENROLLED" {
                        self.clearChallenge()
                        IMUI.alert(self, message: IMStrings.text("authMFASetupRequired"))
                    } else {
                        self.showsOTP = true
                        self.challengeProof = self.contextSignature + "\u{0}" + self.currentUsername + "\u{0}" + secret
                        self.challengeRequiresReauthentication = newProof
                        self.otp.byHidden(false)
                        self.refreshReturnKeys()
                        self.status.byText(failure.code == "MFA_REPLAY" ? IMStrings.text("authMFANextCode") : failure.localizedDescription).byHidden(false)
                        self.setSubmitting(false)
                        self.otp.becomeFirstResponder()
                        self.revealFocusedField()
                    }
                    return
                }
                if self.session.pendingAuthentication(name) != nil {
                    self.status.byText(IMStrings.text("authUnknownExplanation")).byHidden(false)
                }
                let serverFailure = (error as? IMAPIError).map { $0.status == 0 || $0.status >= 500 } ?? false
                if error is URLError || error is IMSession.SessionError || serverFailure {
                    self.showConnectionAlert()
                } else {
                    IMUI.alert(self, message: error.localizedDescription)
                }
            }
        }
    }
}
