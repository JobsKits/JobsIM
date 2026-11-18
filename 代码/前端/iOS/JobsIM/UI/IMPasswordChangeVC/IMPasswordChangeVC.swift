//
//  IMPasswordChangeVC.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月16日，星期一.
//

import JobsByUIKit
import JobsIMCore
import JobsSwiftBaseDefines
import JobsSwiftDSL
import SnapKit
import UIKit

@MainActor
final class IMPasswordChangeVC: IMBaseVC {

    private let username: String
    private let verificationOnly: Bool
    private var verificationRequestID: String?
    private var mounted = false
    private var busy = false
    private var work: Task<Void, Never>?
    private lazy var changes = IMPasswordChangeService(username: username, isCurrent: { [weak self] in
        return self?.mounted == true
    })
    private lazy var oldPassword = IMUI.field(IMStrings.text("currentPassword"), secure: true)
        .byTextContentType(.password)
        .byAccessibilityIdentifier("passwordChange.current")
    private lazy var newPassword = IMUI.field(IMStrings.text("newPassword"), secure: true)
        .byTextContentType(.newPassword)
        .byAccessibilityIdentifier("passwordChange.new")
    private lazy var confirmation = IMUI.field(IMStrings.text("confirmPassword"), secure: true)
        .byTextContentType(.newPassword)
        .byAccessibilityIdentifier("passwordChange.confirm")
    private lazy var otp = IMUI.field(IMStrings.text("authOTP"), secure: true)
        .byTextContentType(.oneTimeCode)
        .byKeyboardType(.numberPad)
    private lazy var notice = IMUI.label("", style: .footnote, color: IMUI.secondary)
    private lazy var originals = IMUI.stack(spacing: 10)
    private lazy var scroll = UIScrollView.jobsMake { view in
        view.byKeyboardDismissMode(.interactive)
    }
    private lazy var stack = IMUI.stack(spacing: 16)
    private lazy var identity = IMUI.label(username, style: .headline)
    private lazy var explanation = IMUI.label(
        IMStrings.text(verificationOnly ? "authPasswordVerificationHelp" : "passwordChangeHelp"),
        style: .footnote, color: IMUI.secondary)
    private lazy var reloadButton = IMUI.button(IMStrings.text("reload")) { [weak self] in
        self?.reload()
    }
    private lazy var submit = IMUI.button(
        IMStrings.text(verificationOnly ? "authContinue" : "passwordChange"), primary: true
    ) { [weak self] in
        guard let self else {
            return
        }
        if self.verificationOnly {
            self.run(id: self.verificationRequestID, action: .check)
        } else {
            self.start()
        }
    }

    init(username: String, verificationOnly: Bool = false) {
        self.username = username.lowercased().trimmingCharacters(in: .whitespacesAndNewlines)
        self.verificationOnly = verificationOnly
        super.init(nibName: nil, bundle: nil)
    }

    required init?(coder: NSCoder) {
        fatalError("init(coder:) is unavailable")
    }

    override func viewDidLoad() {
        screenTitle = IMStrings.text(verificationOnly ? "authIdentityVerification" : "passwordChange")
        super.viewDidLoad()
        scroll.byAddTo(content) { [unowned self] make in
            make.top.left.right.equalToSuperview()
            make.bottom.equalTo(self.view.keyboardLayoutGuide.snp.top)
        }
        stack.byAddTo(scroll) { make in
            make.edges.equalTo(self.scroll.contentLayoutGuide).inset(20)
            make.width.equalTo(self.scroll.frameLayoutGuide).offset(-40)
        }
        stack.byAddArrangedSubviews([identity, explanation, oldPassword, newPassword])
        if !verificationOnly {
            stack.byAddArrangedSubviews([confirmation, otp])
        }
        stack.byAddArrangedSubviews([submit, notice])
        if !verificationOnly {
            stack.byAddArrangedSubviews([reloadButton, originals])
        }
        submit.byAccessibilityIdentifier(verificationOnly ? "verification.continue" : "passwordChange.submit")
        for field in verificationOnly ? [oldPassword, newPassword] : [oldPassword, newPassword, confirmation, otp] {
            field.snp.makeConstraints { make in
                make.height.greaterThanOrEqualTo(48)
            }
        }
    }

    override func viewWillAppear(_ animated: Bool) {
        super.viewWillAppear(animated)
        mounted = true
        NotificationCenter.default.addObserver(self, selector: #selector(clearSensitiveState),
                                               name: UIApplication.didEnterBackgroundNotification, object: nil)
        reload()
    }

    override func viewDidDisappear(_ animated: Bool) {
        super.viewDidDisappear(animated)
        mounted = false
        NotificationCenter.default.removeObserver(self, name: UIApplication.didEnterBackgroundNotification, object: nil)
        work?.cancel()
        clearProof()
    }

    private func clearProof() {
        oldPassword.byText("")
        newPassword.byText("")
        confirmation.byText("")
        otp.byText("")
    }

    @objc private func clearSensitiveState() {
        work?.cancel()
        clearProof()
    }

    private func reload() {
        for view in originals.arrangedSubviews {
            originals.removeArrangedSubview(view)
            view.removeFromSuperview()
        }
        do {
            let rows = try changes.rows()
            if verificationOnly {
                if verificationRequestID == nil {
                    verificationRequestID = rows.first {
                        ["unknown", "pending", "changed"].contains($0["state"].text)
                    }?["id"].text
                }
                let original = rows.first { $0["id"].text == verificationRequestID }
                submit.byEnabled(!busy && original != nil)
                if original == nil {
                    notice.byText(IMStrings.text("authVerificationNotFound"))
                }
                return
            }
            submit.byEnabled(!busy && session.currentCredentialPair?.username == username
                && !rows.contains(where: { ["unsent", "unknown", "pending"].contains($0["state"].text)
                    && $0["familyId"].text == session.currentCredentialPair?.familyID }))
            if rows.isEmpty {
                originals.byAddArrangedSubview(IMUI.button(IMStrings.text("passwordChangeEmpty") + "\n" + IMStrings.text("reload")) { [weak self] in
                    self?.reload()
                })
            }
            for row in rows.prefix(50) {
                let state = row["state"].text
                originals.byAddArrangedSubview(IMUI.label(row["createdAt"].text, style: .caption1, color: IMUI.secondary))
                originals.byAddArrangedSubview(IMUI.label(IMStrings.text(
                    state == "changed" ? "passwordChangeSignedOut" : state == "retired" ? "passwordChangeRetired" : "passwordChangeUnknown"
                ), style: .footnote))
                for action in [IMPasswordChangeService.Action.check, .retry] {
                    let button = IMUI.button(IMStrings.text(action == .check ? "passwordChangeCheck" : "passwordChangeRetry")) { [weak self] in
                        self?.run(id: row["id"].text, action: action)
                    }
                    button.byEnabled(!busy)
                    originals.byAddArrangedSubview(button)
                }
            }
        } catch {
            notice.byText(IMStrings.text(verificationOnly ? "authVerificationFailed" : "connectionNeeded"))
            submit.byEnabled(false)
        }
    }

    private func start() {
        guard !verificationOnly else {
            return
        }
        IMUI.confirm(self, title: IMStrings.text("passwordChange"), message: IMStrings.text("passwordChangeHelp")) { [weak self] in
            self?.run(id: nil, action: .submit)
        }
    }

    private func run(id: String?, action: IMPasswordChangeService.Action) {
        guard mounted, !busy else {
            return
        }
        if verificationOnly {
            guard action == .check, let id, id == verificationRequestID else {
                notice.byText(IMStrings.text("authVerificationNotFound"))
                return
            }
        }
        let proof: [String: IMJSON]
        do {
            proof = try IMPasswordChangeService.proof(current: oldPassword.text ?? "", replacement: newPassword.text ?? "",
                confirmation: verificationOnly ? newPassword.text ?? "" : confirmation.text ?? "",
                otp: verificationOnly ? "" : otp.text ?? "")
        } catch {
            notice.byText(IMStrings.text(verificationOnly ? "authVerificationFailed" : "passwordChangeInvalid"))
            return
        }
        clearProof()
        busy = true
        notice.byText(IMStrings.text("pending"))
        reload()
        work = Task { [weak self] in
            guard let self else {
                return
            }
            defer {
                self.busy = false
                if self.mounted {
                    self.reload()
                }
            }
            do {
                let original: String
                if let id {
                    original = id
                } else {
                    original = try await self.changes.prepare()["id"].text
                }
                let result = try await self.changes.perform(original, proof: proof, action: action)
                guard self.mounted, !Task.isCancelled else {
                    return
                }
                if self.verificationOnly {
                    let key = result == nil ? "authVerificationNotFound"
                        : result?["status"].text == "pending" ? "authVerificationPending" : "authVerificationComplete"
                    self.notice.byText(IMStrings.text(key))
                } else {
                    self.notice.byText(IMStrings.text(result?["status"].text == "changed" ? "passwordChangeSignedOut"
                        : result?["status"].text == "retired" ? "passwordChangeRetired" : "passwordChangeUnknown"))
                }
            } catch {
                guard self.mounted, !Task.isCancelled else {
                    return
                }
                self.notice.byText(IMStrings.text(self.verificationOnly ? "authVerificationFailed" : "passwordChangeUnknown"))
            }
        }
    }
}
