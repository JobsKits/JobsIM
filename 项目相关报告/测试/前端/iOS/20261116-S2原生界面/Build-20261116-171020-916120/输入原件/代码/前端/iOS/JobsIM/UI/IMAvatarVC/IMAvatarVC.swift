//
//  IMAvatarVC.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import JobsByUIKit
import JobsIMCore
import JobsSwiftBaseDefines
import JobsSwiftDSL
import SnapKit
import UIKit

@MainActor
final class IMAvatarVC: IMBaseVC, UIDocumentPickerDelegate {
    private let service = IMAvatarService()
    private lazy var image = IMAvatarView.make()
    override func viewDidLoad() {
        screenTitle = IMStrings.text("avatar")
        super.viewDidLoad()
        let stack = IMUI.stack(spacing: 20)
        stack.byAddTo(content) {
            make in make.top.leading.trailing.equalToSuperview().inset(24)
        }
        image.snp.makeConstraints {
            make in make.height.equalTo(180)
        }
        stack.byAddArrangedSubview(image)
        stack.byAddArrangedSubview(
            IMUI.label("JPEG / PNG · ≤ 2 MiB", style: .footnote, color: IMUI.secondary))
        stack.byAddArrangedSubview(
            IMUI.button(IMStrings.text("avatar"), primary: true) { [weak self] in
                guard let self else {
                    return
                }
                let picker = UIDocumentPickerViewController.makeImages()
                picker.delegate = self
                self.present(picker, animated: true)
            }.byAccessibilityIdentifier("avatar.choose"))
        stack.byAddArrangedSubview(
            IMUI.button(IMStrings.text("retryAvatar")) { [weak self] in
                self?.perform {
                    try await self?.service.retry()
                    await self?.loadCurrent()
                }
            })
        Task {
            await loadCurrent()
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
            try await self.service.prepare(file)
            await self.loadCurrent()
        }
    }
    private func loadCurrent() async {
        guard !session.demo, session.api != nil, session.token != nil else {
            return
        }
        let ns = session.namespace
        do {
            let user = try await session.action("me", method: "GET")
            guard session.namespace == ns else {
                return
            }
            session.user = user
            image.show(user)
        } catch { session.lastError = error.localizedDescription }
    }
}
