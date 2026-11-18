//
//  IMUserProfileVC.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import JobsByUIKit
import JobsIMCore
import JobsIMTransport
import JobsSwiftDSL
import SnapKit
import UIKit

@MainActor
final class IMUserProfileVC: IMBaseVC, UITextViewDelegate {
    private let observedUserID: String
    private let namespace = IMSession.shared.namespace
    private let credentialFamily = IMSession.shared.currentCredentialPair?.familyID
    private var profile: IMJSON
    private var editingRevision = UUID()
    private var saving = false
    private var unavailable = false
    private let detailKeys = ["profileUID", "profileRegisteredAt", "profileEmail", "profilePhone"]
    private lazy var scroll = UIScrollView.jobsMake {
        $0.byKeyboardDismissMode(.interactive)
    }
    private lazy var stack = IMUI.stack(spacing: 12)
    private lazy var avatarHolder = UIView.jobsMake {
        $0.byBackgroundColor(IMUI.background)
    }
    private lazy var avatar = IMAvatarView.make()
    private lazy var username = IMUI.field(IMStrings.text("username"))
    private lazy var displayName = IMUI.field(IMStrings.text("displayName"))
    private lazy var usernameTitle = IMUI.label(
        IMStrings.text("username"), style: .caption1, color: IMUI.secondary)
    private lazy var nameTitle = IMUI.label(
        IMStrings.text("displayName"), style: .caption1, color: IMUI.secondary)
    private lazy var signatureTitle = IMUI.label("", style: .caption1, color: IMUI.secondary)
    private lazy var signature = UITextView.jobsMake {
        $0.byFont(IMUI.font(.body)).byTextColor(IMUI.foreground).byBackgroundColor(IMUI.panel)
            .byTextContainerInset(UIEdgeInsets(top: 12, left: 12, bottom: 12, right: 12))
            .byCornerRadius(12).byDelegate(self)
    }
    private lazy var detailTitles = Dictionary(
        uniqueKeysWithValues: detailKeys.map {
            ($0, IMUI.label(IMStrings.text($0), style: .caption1, color: IMUI.secondary))
        })
    private lazy var details = Dictionary(
        uniqueKeysWithValues: detailKeys.map {
            ($0, IMUI.label())
        })
    private lazy var status = IMUI.label("", style: .footnote, color: IMUI.secondary)
    private lazy var save = IMUI.button(IMStrings.text("save"), primary: true) { [weak self] in
        self?.saveProfile()
    }
    private var hasCurrentContext: Bool {
        return session.namespace == namespace && session.currentCredentialPair?.familyID == credentialFamily
    }
    private var isCurrentUser: Bool {
        return observedUserID == session.user["id"].text && !observedUserID.isEmpty
    }

    init(user: IMJSON) {
        profile = user
        observedUserID = user["id"].text
        super.init(nibName: nil, bundle: nil)
    }

    required init?(coder: NSCoder) {
        fatalError("init(coder:) is unavailable")
    }

    override func viewDidLoad() {
        screenTitle = IMStrings.text("profile")
        super.viewDidLoad()
        scroll.byAddTo(content) { [unowned self] make in
            make.top.leading.trailing.equalToSuperview()
            make.bottom.equalTo(self.view.keyboardLayoutGuide.snp.top)
        }
        stack.byAddTo(scroll) { [unowned self] make in
            make.edges.equalTo(self.scroll.contentLayoutGuide).inset(24)
            make.width.equalTo(self.scroll.frameLayoutGuide).offset(-48)
        }
        avatarHolder.snp.makeConstraints {
            $0.height.equalTo(96)
        }
        avatar.byAddTo(avatarHolder) { make in
            make.size.equalTo(96)
            make.center.equalToSuperview()
        }
        avatar.jobs_addGestureRetView(
            UITapGestureRecognizer.byConfig { [weak self] (_: UITapGestureRecognizer) in
                self?.openAvatar()
            })
        username.byAccessibilityIdentifier("profile.username")
            .byAddTarget(self, action: #selector(editingChanged), for: .editingChanged)
        displayName.byAccessibilityIdentifier("profile.displayName")
            .byAddTarget(self, action: #selector(editingChanged), for: .editingChanged)
        signature.byAccessibilityIdentifier("profile.signature").byAccessibilityLabel(
            IMStrings.text("profileSignature"))
        save.byAccessibilityIdentifier("profile.save")
        stack.byAddArrangedSubviews([
            avatarHolder, status, usernameTitle, username, nameTitle, displayName, signatureTitle, signature,
        ])
        signature.snp.makeConstraints {
            $0.height.equalTo(100)
        }
        for key in detailKeys {
            if let heading = detailTitles[key], let value = details[key] {
                value.byAccessibilityIdentifier("profile." + key)
                stack.byAddArrangedSubviews([heading, value])
            }
        }
        stack.byAddArrangedSubview(save)
        for item in [username, displayName, save] {
            item.snp.makeConstraints {
                $0.height.greaterThanOrEqualTo(52)
            }
        }
        render()
    }

    override func viewWillAppear(_ animated: Bool) {
        super.viewWillAppear(animated)
        reloadProfile()
    }

    private func render(replaceEdits: Bool = true) {
        avatar.show(profile)
        avatar.byUserInteractionEnabled(isCurrentUser && !unavailable)
            .byAccessibilityLabel(IMStrings.text(isCurrentUser ? "changeAvatar" : "avatar"))
        if replaceEdits {
            username.byText(profile["username"].text)
            displayName.byText(profile["displayName"].text)
            signature.byText(profile["signature"].text)
        }
        let editable = isCurrentUser && !unavailable && !saving
        username.byEnabled(editable)
        displayName.byEnabled(editable)
        signature.byEditable(editable)
        save.byHidden(!isCurrentUser || unavailable).byEnabled(!saving)
        details["profileUID"]?.byText(observedUserID)
        let registered = IMSyncProtocol.date(profile["createdAt"].text).map {
            $0.formatted(
                .dateTime.year().month().day().hour().minute().locale(
                    Locale(identifier: UserDefaults.standard.string(forKey: "language") ?? "zh-Hans")))
        }
        details["profileRegisteredAt"]?.byText(registered ?? IMStrings.text("profileFieldUnavailable"))
        details["profileEmail"]?.byText(bindingText("email"))
        details["profilePhone"]?.byText(bindingText("phone"))
        updateSignatureTitle()
        status.byText(
            IMStrings.text(
                unavailable ? "profileUnavailable" : isCurrentUser ? "editProfile" : "profileReadOnly"))
    }

    private func bindingText(_ key: String) -> String {
        let binding = profile["contactBindings"][key]
        switch binding["state"].text {
        /// Only display a value explicitly authorized by the profile response.
        case "bound":
            return binding["value"].text.isEmpty
                ? IMStrings.text("profileFieldUnavailable") : binding["value"].text
        /// A missing binding is a declared state, not an invented empty address.
        case "unbound":
            return IMStrings.text("profileUnbound")
        /// Other people's hidden bindings do not reveal whether they are bound.
        case "hidden":
            return IMStrings.text("profileHidden")
        /// Authority may explicitly report that the binding cannot currently be read.
        case "unavailable":
            return IMStrings.text("profileFieldUnavailable")
        /// Missing authority data remains unavailable for self and private for others.
        default:
            return IMStrings.text(isCurrentUser ? "profileFieldUnavailable" : "profileHidden")
        }
    }

    private func openAvatar() {
        guard isCurrentUser, !unavailable, hasCurrentContext else {
            return
        }
        navigationController?.pushViewController(
            IMAvatarVC().byHidesBottomBarWhenPushed(true), animated: true)
    }

    @objc private func editingChanged() {
        editingRevision = UUID()
        updateSignatureTitle()
    }

    func textViewDidChange(_ textView: UITextView) {
        editingChanged()
    }

    func textView(_ textView: UITextView, shouldChangeTextIn range: NSRange, replacementText text: String)
        -> Bool
    {
        let proposed = ((textView.text ?? "") as NSString).replacingCharacters(in: range, with: text)
        return proposed.unicodeScalars.count <= 200
    }

    private func updateSignatureTitle() {
        signatureTitle.byText(
            IMStrings.text("profileSignature") + " (\((signature.text ?? "").unicodeScalars.count)/200)")
    }

    private func reloadProfile() {
        guard hasCurrentContext, !observedUserID.isEmpty else {
            return
        }
        let revision = editingRevision
        perform { [weak self] in
            guard let self else {
                return
            }
            do {
                let value = try await self.session.fetchUserProfile(self.observedUserID)
                guard self.hasCurrentContext, value["id"].text == self.observedUserID else {
                    return
                }
                self.profile = value
                self.unavailable = false
                self.render(replaceEdits: self.editingRevision == revision)
            } catch {
                guard self.hasCurrentContext else {
                    return
                }
                if let failure = error as? IMAPIError, [401, 403, 404].contains(failure.status) {
                    self.profile = .null
                    self.unavailable = true
                    self.render()
                }
                self.status.byText(IMStrings.text("profileUnavailable"))
                throw error
            }
        }
    }

    private func saveProfile() {
        guard isCurrentUser, !saving, !unavailable, hasCurrentContext else {
            return
        }
        let name = username.text ?? ""
        let display = displayName.text ?? ""
        let text = signature.text ?? ""
        view.byEndEditing(true)
        saving = true
        render(replaceEdits: false)
        perform { [weak self] in
            guard let self else {
                return
            }
            defer {
                self.saving = false
                let editable = self.isCurrentUser && !self.unavailable
                self.username.byEnabled(editable)
                self.displayName.byEnabled(editable)
                self.signature.byEditable(editable)
                self.save.byEnabled(true)
            }
            let value = try await self.session.updateOwnProfile(
                username: name, displayName: display, signature: text)
            guard self.hasCurrentContext else {
                return
            }
            self.profile = value
            self.render()
            self.status.byText(IMStrings.text("profileUpdated"))
        }
    }
}
