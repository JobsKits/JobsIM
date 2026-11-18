//
//  IMAvatarVC.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import JobsByUIKit
import JobsIMCore
import JobsSwiftDSL
import SnapKit
import UIKit

@MainActor
final class IMAvatarVC: IMBaseVC, UIDocumentPickerDelegate {
    private let service = IMAvatarService()
    private let namespace = IMSession.shared.namespace
    private lazy var image = IMAvatarView.make()
    private lazy var stack = IMUI.stack(spacing: 20)
    private lazy var formats = IMUI.label(
        IMStrings.text("avatarFormats"), style: .footnote, color: IMUI.secondary)
    private lazy var choose = IMUI.button(IMStrings.text("changeAvatar"), primary: true) { [weak self] in
        guard let self, self.session.namespace == self.namespace else {
            return
        }
        self.present(UIDocumentPickerViewController.makeAvatarImages(delegate: self), animated: true)
    }.byAccessibilityIdentifier("avatar.choose")
    private lazy var retry = IMUI.button(IMStrings.text("retryAvatar")) { [weak self] in
        self?.retryCurrent()
    }

    override func viewDidLoad() {
        screenTitle = IMStrings.text("avatar")
        super.viewDidLoad()
        stack.byAddArrangedSubviews([image, formats, choose, retry]).byAddTo(content) {
            $0.top.leading.trailing.equalToSuperview().inset(24)
        }
        image.snp.makeConstraints {
            $0.height.equalTo(180)
        }
        image.show(session.user)
        Task { [weak self] in
            await self?.loadCurrent()
        }
    }

    func documentPicker(_ controller: UIDocumentPickerViewController, didPickDocumentsAt urls: [URL]) {
        guard session.namespace == namespace, let file = urls.first else {
            return
        }
        perform { [weak self] in
            guard let self, self.session.namespace == self.namespace else {
                return
            }
            try await self.service.prepare(file)
            await self.loadCurrent()
        }
    }

    private func retryCurrent() {
        perform { [weak self] in
            guard let self, self.session.namespace == self.namespace else {
                return
            }
            try await self.service.retry()
            await self.loadCurrent()
        }
    }

    private func loadCurrent() async {
        guard session.namespace == namespace, !session.demo, session.api != nil, session.token != nil else {
            return
        }
        do {
            let user = try await session.action("me", method: "GET")
            guard session.namespace == namespace else {
                return
            }
            session.user = user
            image.show(user)
        } catch {
            if session.namespace == namespace {
                session.lastError = error.localizedDescription
            }
        }
    }
}
