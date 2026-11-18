//
//  IMAccountDeletionVC.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月17日，星期二。
//

import JobsByUIKit
import JobsIMCore
import JobsSwiftDSL
import SnapKit
import UIKit

@MainActor
final class IMAccountDeletionVC: IMBaseVC {

    private let username: String
    private let verificationOnly: Bool
    private var verificationRequestID: String?
    private var mounted = false
    private var limit = 10
    private var work: Task<Void, Never>?
    private lazy var service = IMAccountDeletionService(username: username)
    private lazy var password = IMUI.field(IMStrings.text("currentPassword"), secure: true)
        .byTextContentType(.password)
        .byAccessibilityIdentifier("accountDeletion.password")
    private lazy var recovery = IMUI.field(IMStrings.text("deletionRecoveryCode"), secure: true)
        .byTextContentType(.oneTimeCode)
    private lazy var otp = IMUI.field(IMStrings.text("authOTP"), secure: true)
        .byTextContentType(.oneTimeCode)
        .byKeyboardType(.numberPad)
    private lazy var notice = IMUI.label("", style: .footnote, color: IMUI.secondary)
    private lazy var originals = IMUI.stack(spacing: 12)
    private lazy var scroll = UIScrollView.jobsMake { view in
        view.byKeyboardDismissMode(.interactive)
    }
    private lazy var stack = IMUI.stack(spacing: 16)
    private lazy var identity = IMUI.label(username, style: .headline)
    private lazy var explanation = IMUI.label(
        IMStrings.text(verificationOnly ? "authDeletionVerificationHelp" : "deletionHelp"),
        style: .footnote, color: IMUI.secondary)
    private lazy var proofExplanation = IMUI.label(IMStrings.text("deletionProofHelp"), style: .footnote, color: IMUI.secondary)
    private lazy var reloadButton = IMUI.button(IMStrings.text("reload")) { [weak self] in
        self?.resumeCleanup()
    }
    private lazy var importButton = IMUI.button(IMStrings.text("deletionImportReceipt")) { [weak self] in
        self?.importReceipt()
    }
    private lazy var submit = IMUI.button(
        IMStrings.text(verificationOnly ? "authContinue" : "deletionRequest"), primary: true
    ) { [weak self] in
        guard let self else {
            return
        }
        if self.verificationOnly {
            self.perform(id: self.verificationRequestID, operation: "check")
            return
        }
        IMUI.confirm(self, title: IMStrings.text("accountDeletion"), message: IMStrings.text("deletionHelp")) { [weak self] in
            self?.perform(id: nil, operation: "submit")
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
        screenTitle = IMStrings.text(verificationOnly ? "authIdentityVerification" : "accountDeletion")
        super.viewDidLoad()
        scroll.byAddTo(content) { [unowned self] make in
            make.top.left.right.equalToSuperview()
            make.bottom.equalTo(self.view.keyboardLayoutGuide.snp.top)
        }
        stack.byAddTo(scroll) { make in
            make.edges.equalTo(self.scroll.contentLayoutGuide).inset(20)
            make.width.equalTo(self.scroll.frameLayoutGuide).offset(-40)
        }
        stack.byAddArrangedSubviews([identity, explanation])
        if !verificationOnly {
            stack.byAddArrangedSubview(proofExplanation)
        }
        stack.byAddArrangedSubview(password)
        if !verificationOnly {
            stack.byAddArrangedSubviews([recovery, otp])
        }
        stack.byAddArrangedSubviews([submit, notice])
        if !verificationOnly {
            stack.byAddArrangedSubviews([reloadButton, importButton, originals])
        }
        submit.byAccessibilityIdentifier(verificationOnly ? "verification.continue" : "accountDeletion.submit")
        for field in verificationOnly ? [password] : [password, recovery, otp] {
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
        if !verificationOnly {
            resumeCleanup()
        }
    }

    override func viewDidDisappear(_ animated: Bool) {
        super.viewDidDisappear(animated)
        mounted = false
        NotificationCenter.default.removeObserver(self, name: UIApplication.didEnterBackgroundNotification, object: nil)
        clearSensitiveState()
    }

    @objc private func clearSensitiveState() {
        work?.cancel()
        password.byText("")
        recovery.byText("")
        otp.byText("")
    }

    private func run(_ action: @escaping @MainActor () async throws -> Void) {
        guard mounted, work == nil else {
            return
        }
        work = Task { [weak self] in
            guard let self else {
                return
            }
            defer {
                self.work = nil
                if self.mounted {
                    self.reload()
                }
            }
            do {
                try await action()
                if self.mounted && !Task.isCancelled && !self.verificationOnly {
                    self.notice.byText(IMStrings.text("deletionChecked"))
                }
            } catch {
                if self.mounted && !Task.isCancelled {
                    self.notice.byText(IMStrings.text(self.verificationOnly ? "authVerificationFailed" : "deletionPending"))
                }
            }
        }
        reload()
    }

    private func resumeCleanup() {
        guard !verificationOnly else {
            return
        }
        run { [weak self] in
            try await self?.service.resumeLocalCleanup()
        }
    }

    private func perform(id: String?, operation: String) {
        guard mounted, work == nil else {
            return
        }
        if verificationOnly {
            guard operation == "check", let id, id == verificationRequestID else {
                notice.byText(IMStrings.text("authVerificationNotFound"))
                return
            }
        }
        let credential: IMAccountDeletionService.Credential
        do {
            credential = try IMAccountDeletionService.credential(
                password: password.text ?? "", recoveryCode: verificationOnly ? "" : recovery.text ?? "",
                otp: verificationOnly ? "" : otp.text ?? "")
        } catch {
            notice.byText(IMStrings.text(verificationOnly ? "authVerificationFailed" : "deletionProofHelp"))
            return
        }
        password.byText("")
        recovery.byText("")
        otp.byText("")
        view.endEditing(true)
        run { [weak self] in
            guard let self else {
                return
            }
            switch operation {
            case "submit":
                let original = try await self.service.prepare()
                _ = try await self.service.perform(original["id"].text, credential: credential, action: .submit)
            case "check", "retry":
                guard let id else {
                    return
                }
                let result = try await self.service.perform(id, credential: credential, action: operation == "check" ? .check : .retry)
                if self.verificationOnly {
                    let pending = ["pending", "requested", "cancel_pending", "processing"]
                        .contains(result?["status"].text ?? "")
                    let key = result == nil ? "authVerificationNotFound"
                        : pending ? "authVerificationPending" : "authVerificationComplete"
                    self.notice.byText(IMStrings.text(key))
                }
            case "query":
                guard let id else {
                    return
                }
                _ = try await self.service.query(id, credential: credential)
            case "cancel":
                guard let id else {
                    return
                }
                _ = try await self.service.cancel(id, credential: credential)
            default:
                break
            }
        }
    }

    private func importReceipt() {
        guard work == nil, !verificationOnly else {
            return
        }
        IMUI.prompt(self, title: IMStrings.text("deletionImportReceipt")) { [weak self] code in
            guard let self, self.mounted else {
                return
            }
            do {
                try self.service.importReceipt(code.trimmingCharacters(in: .whitespacesAndNewlines))
                self.reload()
            } catch {
                self.notice.byText(IMStrings.text("deletionReceiptInvalid"))
            }
        }
    }

    private func reload() {
        for view in originals.arrangedSubviews {
            originals.removeArrangedSubview(view)
            view.removeFromSuperview()
        }
        do {
            let rows = try service.rows()
            if verificationOnly {
                if verificationRequestID == nil {
                    verificationRequestID = rows.first {
                        !["unsent", "rejected", "cancelled", "retired"].contains($0["state"].text)
                            && ($0["state"].text != "completed" || $0["cleanup"] != .bool(true))
                    }?["id"].text
                }
                let original = rows.first { $0["id"].text == verificationRequestID }
                submit.byEnabled(work == nil && original != nil)
                if original == nil {
                    notice.byText(IMStrings.text("authVerificationNotFound"))
                }
                return
            }
            submit.byEnabled(work == nil && session.currentCredentialPair?.username == username
                && !rows.contains(where: { $0["familyId"].text == session.currentCredentialPair?.familyID
                    && !["rejected", "cancelled", "retired"].contains($0["state"].text) }))
            if rows.isEmpty {
                originals.byAddArrangedSubview(IMUI.button(IMStrings.text("deletionEmpty") + "\n" + IMStrings.text("reload")) { [weak self] in
                    self?.resumeCleanup()
                })
            }
            for row in rows.prefix(limit) {
                let state = row["state"].text
                let key = ["pending", "requested", "cancel_pending", "processing", "completed", "cancelled", "retired"].contains(state)
                    ? "deletionState_" + state : "deletionState_unknown"
                originals.byAddArrangedSubview(IMUI.label(row["createdAt"].text, style: .caption1, color: IMUI.secondary))
                originals.byAddArrangedSubview(IMUI.label(IMStrings.text(key), style: .headline))
                if let until = IMSyncProtocol.date(row["result"]["cancelUntil"].text) {
                    originals.byAddArrangedSubview(IMUI.label(DateFormatter.localizedString(from: until, dateStyle: .medium, timeStyle: .short), style: .footnote))
                }
                if state == "completed" && row["cleanup"] != .bool(true) {
                    originals.byAddArrangedSubview(IMUI.label(IMStrings.text("deletionCleanupPending"), style: .footnote))
                }
                var actions = [("check", "deletionCheck")]
                if ["unknown", "unsent", "pending"].contains(state) {
                    actions.append(("retry", "deletionRetry"))
                }
                if row["deletionId"] != .null && state != "completed" {
                    actions.append(("query", "deletionQuery"))
                }
                if state == "requested", row["cancelSent"] != .bool(true),
                    let until = IMSyncProtocol.date(row["result"]["cancelUntil"].text), until > Date() {
                    actions.append(("cancel", "deletionCancel"))
                }
                for (operation, title) in actions {
                    let button = IMUI.button(IMStrings.text(title)) { [weak self] in
                        guard let self else {
                            return
                        }
                        if operation == "cancel" {
                            IMUI.confirm(self, title: IMStrings.text("deletionCancel"), message: IMStrings.text("deletionCancelHelp")) { [weak self] in
                                self?.perform(id: row["id"].text, operation: operation)
                            }
                        } else {
                            self.perform(id: row["id"].text, operation: operation)
                        }
                    }
                    button.byEnabled(work == nil)
                    originals.byAddArrangedSubview(button)
                }
                if row["deletionId"] != .null {
                    originals.byAddArrangedSubview(IMUI.button(IMStrings.text("deletionCopyReceipt")) { [weak self] in
                        guard let self, let receipt = try? self.service.receipt(row["id"].text) else {
                            return
                        }
                        UIPasteboard.general.string = receipt
                        self.notice.byText(IMStrings.text("copied"))
                    })
                }
            }
            if rows.count > limit {
                originals.byAddArrangedSubview(IMUI.button(IMStrings.text("loadMore")) { [weak self] in
                    guard let self else {
                        return
                    }
                    self.limit += 10
                    self.reload()
                })
            }
        } catch {
            notice.byText(IMStrings.text(verificationOnly ? "authVerificationFailed" : "connectionNeeded"))
            submit.byEnabled(false)
        }
    }
}
