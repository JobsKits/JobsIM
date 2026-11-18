//
//  IMChatSettingsVC.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import JobsByUIKit
import JobsIMCore
import JobsSwiftDSL
import SnapKit
import UIKit

@MainActor
final class IMChatSettingsVC: IMBaseVC {
    private let user: IMJSON
    private let boundNamespace = IMSession.shared.namespace
    private let conversationID: String
    private let recoverDrafts: (() -> Void)?
    private var relationship: IMJSON = .null
    private lazy var scroll = UIScrollView.jobsMake {
        view in
        view.byAlwaysBounceVertical(true)
    }
    private lazy var stack = IMUI.stack(spacing: 16)
    private lazy var block = IMUI.button(IMStrings.text("block")) { [weak self] in
        self?.changeBlock()
    }
    private lazy var star = IMUI.button(IMStrings.text("starContact")) { [weak self] in
        self?.changeStar()
    }
    private lazy var remove = IMUI.button(IMStrings.text("removeContact")) { [weak self] in
        self?.removeContact()
    }
    private lazy var profile = IMUI.button(user.firstText("displayName", "username", "id")) { [weak self] in
        guard let self, self.session.namespace == self.boundNamespace else {
            return
        }
        self.navigationController?.pushViewController(IMUserProfileVC(user: self.user), animated: true)
    }
    private lazy var report = IMUI.button(IMStrings.text("report")) { [weak self] in
        guard let self, self.session.namespace == self.boundNamespace else {
            return
        }
        self.navigationController?.pushViewController(
            IMReportComposerVC(
                targetType: "user", targetID: self.user["id"].text, targetUserID: self.user["id"].text,
                conversationID: self.conversationID), animated: true)
    }
    private lazy var reload = IMUI.button(IMStrings.text("reload")) { [weak self] in
        self?.reloadRelationship()
    }
    private lazy var recover = IMUI.button(IMStrings.text("savedEditingDrafts")) { [weak self] in
        guard let self, self.session.namespace == self.boundNamespace else {
            return
        }
        self.recoverDrafts?()
    }

    init(user: IMJSON, conversationID: String, recoverDrafts: (() -> Void)? = nil) {
        self.user = user
        self.conversationID = conversationID
        self.recoverDrafts = recoverDrafts
        super.init(nibName: nil, bundle: nil)
        hidesBottomBarWhenPushed = true
    }
    required init?(coder: NSCoder) {
        fatalError("init(coder:) is unavailable")
    }
    override func viewDidLoad() {
        screenTitle = IMStrings.text("chatSettings")
        super.viewDidLoad()
        scroll.byAddTo(content) {
            $0.edges.equalToSuperview()
        }
        for button in [star, block, remove] {
            button.byEnabled(false)
        }
        stack.byAddArrangedSubviews([profile, star, block, remove, report, reload])
        if recoverDrafts != nil {
            stack.byAddArrangedSubview(recover)
        }
        for view in stack.arrangedSubviews {
            view.snp.makeConstraints {
                $0.height.greaterThanOrEqualTo(44)
            }
        }
        stack.byAddTo(scroll) {
            $0.edges.equalTo(self.scroll.contentLayoutGuide).inset(24)
            $0.width.equalTo(self.scroll.frameLayoutGuide).offset(-48)
        }
    }
    override func viewWillAppear(_ animated: Bool) {
        super.viewWillAppear(animated)
        reloadRelationship()
    }
    private func reloadRelationship() {
        let original = session.namespace
        perform { [weak self] in
            guard let self, self.session.namespace == self.boundNamespace else {
                return
            }
            let value = try await self.session.contactRelationship(userID: self.user["id"].text)
            guard self.session.namespace == original else {
                return
            }
            self.relationship = value
            self.updateButtons()
        }
    }
    private func updateButtons() {
        block.byTitle(IMStrings.text(relationship["blockedByMe"] == .bool(true) ? "unblock" : "block"))
        star.byTitle(
            IMStrings.text(
                IMContactLocalStars.ids(session: session).contains(user["id"].text)
                    ? "unstarContact" : "starContact"))
        for button in [star, block, remove] {
            button.byEnabled(true)
        }
        let friend = relationship["friend"] == .bool(true)
        remove.byHidden(!friend)
        star.byHidden(!friend)
    }
    private func changeBlock() {
        guard session.namespace == boundNamespace else {
            return
        }
        let kind = relationship["blockedByMe"] == .bool(true) ? "unblock" : "block"
        perform { [weak self] in
            guard let self, self.session.namespace == self.boundNamespace else {
                return
            }
            try await IMContactService().perform(kind, userID: self.user["id"].text)
            self.reloadRelationship()
        }
    }
    private func changeStar() {
        guard session.namespace == boundNamespace else {
            return
        }
        do {
            let selected = IMContactLocalStars.ids(session: session).contains(user["id"].text)
            try IMContactLocalStars.set(!selected, userID: user["id"].text, session: session)
            updateButtons()
        } catch {
            IMUI.alert(self, message: error.localizedDescription)
        }
    }
    private func removeContact() {
        guard session.namespace == boundNamespace else {
            return
        }
        IMContactUX.confirmRemoval(on: self) { [weak self] in
            self?.perform {
                guard let self, self.session.namespace == self.boundNamespace else {
                    return
                }
                try await IMContactService().perform("remove", userID: self.user["id"].text)
                try IMContactLocalStars.set(false, userID: self.user["id"].text, session: self.session)
                self.navigationController?.popViewController(animated: true)
            }
        }
    }
}
