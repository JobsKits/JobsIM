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
final class IMUserProfileVC: IMBaseVC {
    private let observedUserID: String
    private let namespace = IMSession.shared.namespace
    private let credentialFamily = IMSession.shared.currentCredentialPair?.familyID
    private var profile: IMJSON
    private lazy var avatar = IMAvatarView.make()
    private lazy var username = IMUI.field(IMStrings.text("username"))
    private lazy var displayName = IMUI.field(IMStrings.text("displayName"))
    private lazy var status = IMUI.label("", style: .footnote, color: IMUI.secondary)
    private lazy var save = IMUI.button(IMStrings.text("save"), primary: true) { [weak self] in
        self?.saveProfile()
    }
    private lazy var changeAvatar = IMUI.button(IMStrings.text("avatar")) { [weak self] in
        guard let self, self.isCurrentUser, self.hasCurrentContext else {
            return
        }
        let controller = IMAvatarVC()
        controller.byHidesBottomBarWhenPushed(true)
        self.navigationController?.pushViewController(controller, animated: true)
    }
    private var hasCurrentContext: Bool {
        session.namespace == namespace && session.currentCredentialPair?.familyID == credentialFamily
    }
    private var isCurrentUser: Bool {
        observedUserID == session.user["id"].text && !observedUserID.isEmpty
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
        let scroll = UIScrollView.jobsMake {
            $0.byKeyboardDismissMode(.interactive)
        }
        scroll.byAddTo(content) { [unowned self] make in
            make.top.leading.trailing.equalToSuperview()
            make.bottom.equalTo(self.view.keyboardLayoutGuide.snp.top)
        }
        let stack = IMUI.stack(spacing: 16)
        stack.byAddTo(scroll) { make in
            make.edges.equalTo(scroll.contentLayoutGuide).inset(24)
            make.width.equalTo(scroll.frameLayoutGuide).offset(-48)
        }
        let avatarHolder = UIView.jobsMake {
            $0.byBackgroundColor(IMUI.background)
        }
        avatarHolder.snp.makeConstraints {
            $0.height.equalTo(96)
        }
        avatar.byAddTo(avatarHolder) { make in
            make.size.equalTo(96)
            make.center.equalToSuperview()
        }
        username.byAccessibilityIdentifier("profile.username")
        displayName.byAccessibilityIdentifier("profile.displayName")
        save.byAccessibilityIdentifier("profile.save")
        stack.byAddArrangedSubviews([
            avatarHolder, status,
            IMUI.label(IMStrings.text("username"), style: .caption1), username,
            IMUI.label(IMStrings.text("displayName"), style: .caption1), displayName,
            changeAvatar, save,
            IMUI.button(IMStrings.text("refresh")) {
                [weak self] in self?.reloadProfile()
            }
                .byAccessibilityIdentifier("profile.reload"),
        ])
        for item in [username, displayName, save, changeAvatar] {
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

    private func render() {
        avatar.show(profile)
        username.byText(profile["username"].text).byEnabled(isCurrentUser)
        displayName.byText(profile["displayName"].text).byEnabled(isCurrentUser)
        changeAvatar.byHidden(!isCurrentUser)
        save.byHidden(!isCurrentUser)
        status.byText(IMStrings.text(isCurrentUser ? "editProfile" : "profileReadOnly"))
    }

    private func reloadProfile() {
        guard hasCurrentContext, !observedUserID.isEmpty else {
            return
        }
        if session.demo {
            if isCurrentUser {
                profile = session.user
            }
            render()
            return
        }
        perform { [weak self] in
            guard let self else {
                return
            }
            do {
                let value = try await self.session.fetchUserProfile(self.observedUserID)
                guard self.hasCurrentContext else {
                    return
                }
                self.profile = value
                self.render()
            } catch {
                guard self.hasCurrentContext else {
                    return
                }
                if let failure = error as? IMAPIError, [401, 403, 404].contains(failure.status) {
                    self.profile = .null
                    self.render()
                    self.username.byEnabled(false)
                    self.displayName.byEnabled(false)
                    self.save.byHidden(true)
                    self.changeAvatar.byHidden(true)
                }
                self.status.byText(IMStrings.text("profileUnavailable"))
                throw error
            }
        }
    }

    private func saveProfile() {
        guard isCurrentUser, hasCurrentContext else {
            return
        }
        let name = username.text ?? ""
        let display = displayName.text ?? ""
        view.byEndEditing(true)
        save.byEnabled(false)
        perform { [weak self] in
            guard let self else {
                return
            }
            defer {
                self.save.byEnabled(true)
            }
            let value = try await self.session.updateOwnProfile(username: name, displayName: display)
            guard self.hasCurrentContext else {
                return
            }
            self.profile = value
            self.render()
            self.status.byText(IMStrings.text("profileUpdated"))
        }
    }
}
