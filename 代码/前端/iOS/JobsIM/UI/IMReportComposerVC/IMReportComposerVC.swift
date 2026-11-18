//
//  IMReportComposerVC.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import Foundation
import JobsByUIKit
import JobsIMCore
import JobsSwiftDSL
import PhotosUI
import QuickLook
import SnapKit
import UIKit
import UniformTypeIdentifiers

@MainActor
final class IMReportComposerVC: IMBaseVC, PHPickerViewControllerDelegate, UITableViewDataSource,
    UITableViewDelegate, QLPreviewControllerDataSource
{
    private let targetType: String
    private let targetID: String
    private let targetUserID: String?
    private let conversationID: String?
    private let originalNamespace: IMNamespace
    private var messageIDs: [String]
    private var evidence: [IMReportEvidenceSelection] = []
    private var previewURL: URL?
    private var busy = false
    private var submitting = false
    private var submitted = false
    private var closing = false
    private var mediaRowHeight: CGFloat {
        return max(88, IMUI.font(.body).lineHeight * 2 + IMUI.font(.subheadline).lineHeight + 24)
    }
    private lazy var reason = UITextView.jobsMake {
        view in
        view.byFont(IMUI.font(.body)).byTextColor(IMUI.foreground).byBackgroundColor(IMUI.panel)
            .byCornerRadius(12)
    }
    private lazy var mediaTable = UITableView.make().byDataSource(self).byDelegate(self)
        .byRowHeight(mediaRowHeight).byBackgroundColor(IMUI.panel).byScrollEnabled(false)
    private lazy var messagesButton = IMUI.button(IMStrings.text("selectReportMessages")) { [weak self] in
        self?.chooseMessages()
    }
    private lazy var addButton = IMUI.button(IMStrings.text("addReportMedia")) { [weak self] in
        self?.chooseMedia()
    }
    private lazy var submit = IMUI.button(IMStrings.text("submitReport"), primary: true) { [weak self] in
        self?.submitReport()
    }
    private lazy var stack = IMUI.stack(spacing: 16)
    private lazy var scroll = UIScrollView.jobsMake {
        view in
        view.byKeyboardDismissMode(.interactive)
    }
    private lazy var pending = IMUI.button(IMStrings.text("pendingReports")) { [weak self] in
        self?.navigationController?.pushViewController(IMReportVC(), animated: true)
    }
    private lazy var reasonLabel = IMUI.label(IMStrings.text("reportReason"))
    private lazy var materialsLabel = IMUI.label(IMStrings.text("reportMaterials"))

    init(
        targetType: String, targetID: String, targetUserID: String? = nil, conversationID: String? = nil,
        messageIDs: [String] = []
    ) {
        self.targetType = targetType
        self.targetID = targetID
        self.targetUserID = targetUserID
        self.conversationID = conversationID
        self.messageIDs = Array(messageIDs.prefix(50))
        self.originalNamespace = IMSession.shared.namespace
        super.init(nibName: nil, bundle: nil)
        hidesBottomBarWhenPushed = true
    }
    required init?(coder: NSCoder) {
        fatalError("init(coder:) is unavailable")
    }
    override func viewDidLoad() {
        screenTitle = IMStrings.text("report")
        super.viewDidLoad()
        scroll.byAddTo(content) {
            $0.edges.equalToSuperview()
        }
        stack.byAddArrangedSubviews([
            pending, reasonLabel, reason, materialsLabel, addButton, mediaTable, messagesButton, submit,
        ])
        .byAddTo(scroll) {
            make in
            make.edges.equalTo(self.scroll.contentLayoutGuide).inset(20)
            make.width.equalTo(self.scroll.frameLayoutGuide).offset(-40)
        }
        reason.snp.makeConstraints {
            $0.height.equalTo(120)
        }
        mediaTable.snp.makeConstraints {
            $0.height.equalTo(0)
        }
        for button in [addButton, messagesButton, submit] {
            button.snp.makeConstraints {
                $0.height.greaterThanOrEqualTo(44)
            }
        }
        messagesButton.byHidden(conversationID == nil)
        refreshSelection()
    }
    override func viewDidDisappear(_ animated: Bool) {
        super.viewDidDisappear(animated)
        if isMovingFromParent || isBeingDismissed || navigationController?.isBeingDismissed == true {
            closing = true
            for item in evidence {
                removeOwnedDraftCopy(item.url)
            }
        }
    }

    private func checkDraftContext() throws {
        guard !closing, session.namespace == originalNamespace, session.demo == originalNamespace.demo,
            try !session.store.accountIsErased(
                profile: originalNamespace.profile,
                deployment: originalNamespace.deploymentID, user: originalNamespace.account)
        else {
            throw IMSession.SessionError.noInstance
        }
    }

    private var draftNamespaceRoot: URL {
        FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
            .appendingPathComponent("JobsIM/ReportDrafts", isDirectory: true)
            .appendingPathComponent(originalNamespace.key, isDirectory: true)
    }

    private func prepareDraftFolder(_ id: String) throws -> URL {
        let support = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
        let paths = [
            support.appendingPathComponent("JobsIM", isDirectory: true),
            support.appendingPathComponent("JobsIM/ReportDrafts", isDirectory: true), draftNamespaceRoot,
            draftNamespaceRoot.appendingPathComponent(id, isDirectory: true),
        ]
        for folder in paths {
            if FileManager.default.fileExists(atPath: folder.path) {
                let values = try folder.resourceValues(forKeys: [.isDirectoryKey, .isSymbolicLinkKey])
                guard values.isDirectory == true, values.isSymbolicLink != true else {
                    throw CocoaError(.fileWriteNoPermission)
                }
            } else {
                try FileManager.default.createDirectory(
                    at: folder, withIntermediateDirectories: false, attributes: [.posixPermissions: 0o700])
            }
        }
        return paths[3]
    }

    func stagePickedEvidence(from url: URL, folderID: String, limit: Int64) throws -> URL {
        var copied: URL?
        do {
            try checkDraftContext()
            let attributes = try url.resourceValues(forKeys: [
                .isRegularFileKey, .isSymbolicLinkKey, .fileSizeKey,
            ])
            guard attributes.isRegularFile == true, attributes.isSymbolicLink != true,
                let size = attributes.fileSize, size > 0, Int64(size) <= limit else {
                throw CocoaError(.fileReadTooLarge)
            }
            try session.store.save([.object(["id": .string(folderID)])],
                                   collection: "report-draft-files", namespace: originalNamespace)
            let folder = try prepareDraftFolder(folderID)
            let destination = folder.appendingPathComponent(url.lastPathComponent)
            try FileManager.default.copyItem(at: url, to: destination)
            copied = destination
            try FileManager.default.setAttributes([
                .posixPermissions: 0o600,
                .protectionKey: FileProtectionType.completeUntilFirstUserAuthentication,
            ], ofItemAtPath: destination.path)
            try checkDraftContext()
            return destination
        } catch {
            if let copied {
                removeOwnedDraftCopy(copied)
            }
            throw error
        }
    }

    private func removeOwnedDraftCopy(_ url: URL) {
        let folder = url.deletingLastPathComponent()
        let reportRoot = draftNamespaceRoot.deletingLastPathComponent()
        for parent in [reportRoot.deletingLastPathComponent(), reportRoot, draftNamespaceRoot, folder] {
            guard let values = try? parent.resourceValues(forKeys: [.isDirectoryKey, .isSymbolicLinkKey]),
                values.isDirectory == true, values.isSymbolicLink != true
            else {
                return
            }
        }
        guard
            folder.deletingLastPathComponent().standardizedFileURL == draftNamespaceRoot.standardizedFileURL,
            UUID(uuidString: folder.lastPathComponent) != nil,
            let values = try? url.resourceValues(forKeys: [.isRegularFileKey, .isSymbolicLinkKey]),
            values.isRegularFile == true, values.isSymbolicLink != true
        else {
            return
        }
        try? FileManager.default.removeItem(at: url)
    }

    private func refreshSelection() {
        mediaTable.snp.updateConstraints {
            $0.height.equalTo(CGFloat(evidence.count) * mediaRowHeight)
        }
        mediaTable.reloadData()
        messagesButton.byTitle(IMStrings.text("selectReportMessages") + " (\(messageIDs.count)/50)")
        reason.byEditable(!busy && !submitted)
        addButton.byEnabled(!busy && !submitted && evidence.count < 10)
        messagesButton.byEnabled(!busy && !submitted)
        submit.byEnabled(!busy && !submitted).byTitle(
            IMStrings.text(submitted ? "reportSubmitted" : submitting ? "reportUploading" : "submitReport"))
    }
    private func chooseMessages() {
        guard !busy, !submitted, session.namespace == originalNamespace, let conversationID else {
            return
        }
        let picker = IMReportMessageSelectionVC(conversationID: conversationID, selectedIDs: messageIDs) {
            [weak self] ids in
            guard let self, self.session.namespace == self.originalNamespace else {
                return
            }
            self.messageIDs = ids
            self.refreshSelection()
        }
        navigationController?.pushViewController(picker, animated: true)
    }
    private func chooseMedia() {
        guard !busy, !submitted, session.namespace == originalNamespace, evidence.count < 10 else {
            return
        }
        let configuration = PHPickerConfiguration.make(filter: .any(of: [.images, .videos]))
            .bySelectionLimit(10 - evidence.count)
            .byPreferredAssetRepresentationMode(.compatible)
        let picker = PHPickerViewController.make(configuration: configuration).byDelegate(self)
        present(picker, animated: true)
    }
    func picker(_ picker: PHPickerViewController, didFinishPicking results: [PHPickerResult]) {
        picker.dismiss(animated: true)
        guard !busy else {
            return
        }
        busy = true
        refreshSelection()
        Task {
            @MainActor [weak self] in
            guard let self else {
                return
            }
            defer {
                self.busy = false
                self.refreshSelection()
            }
            do {
                for result in results.prefix(10 - self.evidence.count) {
                    try self.checkDraftContext()
                    let provider = result.itemProvider
                    guard
                        let identifier = provider.registeredTypeIdentifiers.first(where: {
                            UTType($0)?.conforms(to: .image) == true
                                || UTType($0)?.conforms(to: .movie) == true
                        }), let type = UTType(identifier), let mime = type.preferredMIMEType
                    else {
                        throw CocoaError(.fileReadUnsupportedScheme)
                    }
                    let limit: Int64 = type.conforms(to: .movie) ? 64 * 1024 * 1024 : 20 * 1024 * 1024
                    let folderID = UUID().uuidString
                    let context = IMReportPickerContext(controller: self)
                    let copy: URL = try await withCheckedThrowingContinuation {
                        continuation in
                        provider.loadFileRepresentation(forTypeIdentifier: identifier) {
                            @Sendable [context] url, error in
                            guard let url else {
                                continuation.resume(throwing: error ?? CocoaError(.fileReadUnknown))
                                return
                            }
                            let copied: Result<URL, Error>
                            // The provider URL is valid only until this callback returns.
                            if Thread.isMainThread {
                                copied = MainActor.assumeIsolated {
                                    Result {
                                        try context.stageFile(from: url, folderID: folderID, limit: limit)
                                    }
                                }
                            } else {
                                copied = DispatchQueue.main.sync {
                                    MainActor.assumeIsolated {
                                        Result {
                                            try context.stageFile(from: url, folderID: folderID, limit: limit)
                                        }
                                    }
                                }
                            }
                            continuation.resume(with: copied)
                        }
                    }
                    do {
                        try self.checkDraftContext()
                    } catch {
                        self.removeOwnedDraftCopy(copy)
                        throw error
                    }
                    self.evidence.append(
                        IMReportEvidenceSelection(url: copy, name: copy.lastPathComponent, mimeType: mime))
                }
            } catch {
                guard !self.closing, self.session.namespace == self.originalNamespace else {
                    return
                }
                IMUI.alert(self, message: error.localizedDescription)
            }
        }
    }
    func tableView(_ tableView: UITableView, numberOfRowsInSection section: Int) -> Int {
        evidence.count
    }
    func tableView(_ tableView: UITableView, cellForRowAt indexPath: IndexPath) -> UITableViewCell {
        let item = evidence[indexPath.row]
        let bytes =
            (try? FileManager.default.attributesOfItem(atPath: item.url.path)[.size] as? NSNumber)?.int64Value
            ?? 0
        let cell = UITableViewCell.make(style: .subtitle, reuseIdentifier: nil)
            .byText(item.name).bySecondaryText(item.mimeType + " · \(bytes) B").byIMThemeAccessoryType(
                .disclosureIndicator)
        IMUI.cellTheme(cell)
        return cell
    }
    func tableView(_ tableView: UITableView, didSelectRowAt indexPath: IndexPath) {
        tableView.deselectRow(at: indexPath, animated: true)
        previewURL = evidence[indexPath.row].url
        let preview = IMReportMediaPreviewVC(dataSource: self)
        present(UINavigationController.make(preview), animated: true)
    }
    func tableView(_ tableView: UITableView, trailingSwipeActionsConfigurationForRowAt indexPath: IndexPath)
        -> UISwipeActionsConfiguration?
    {
        guard !busy, !submitted else {
            return nil
        }
        let action = UIContextualAction.makeContactRemoval {
            [weak self] completion in
            guard let self, self.evidence.indices.contains(indexPath.row) else {
                completion(false)
                return
            }
            let removed = self.evidence.remove(at: indexPath.row)
            self.removeOwnedDraftCopy(removed.url)
            self.refreshSelection()
            completion(true)
        }
        return UISwipeActionsConfiguration.makeContactActions([action])
    }
    func numberOfPreviewItems(in controller: QLPreviewController) -> Int {
        previewURL == nil ? 0 : 1
    }
    func previewController(_ controller: QLPreviewController, previewItemAt index: Int) -> QLPreviewItem {
        previewURL! as NSURL
    }
    private func submitReport() {
        guard !busy, !submitted, !reason.text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
            return
        }
        IMUI.confirm(
            self, title: IMStrings.text("submitReport"),
            message: IMStrings.text("reportMaterials") + " · \(evidence.count) / \(messageIDs.count)"
        ) { [weak self] in
            guard let self, !self.busy, !self.submitted, self.session.namespace == self.originalNamespace
            else {
                return
            }
            self.busy = true
            self.submitting = true
            self.refreshSelection()
            self.perform {
                defer {
                    self.busy = false
                    self.submitting = false
                    self.refreshSelection()
                }
                guard self.session.namespace == self.originalNamespace else {
                    throw IMSession.SessionError.noInstance
                }
                let composition = IMReportComposition(
                    targetType: self.targetType, targetID: self.targetID, targetUserID: self.targetUserID,
                    reason: self.reason.text, messageIDs: self.messageIDs, evidence: self.evidence)
                _ = try await self.session.submitReport(composition)
                guard self.session.namespace == self.originalNamespace else {
                    return
                }
                self.submitted = true
                for item in self.evidence {
                    self.removeOwnedDraftCopy(item.url)
                }
                self.evidence.removeAll()
                IMUI.alert(
                    self, title: IMStrings.text("done"),
                    message: IMStrings.text(self.session.demo ? "done" : "reportSubmitted"))
            }
        }
    }
}
