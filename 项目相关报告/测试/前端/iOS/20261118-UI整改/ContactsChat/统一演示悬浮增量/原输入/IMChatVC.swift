//
//  IMChatVC.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import AVFoundation
import JobsByUIKit
import JobsIMCore
import JobsIMTransport
import JobsSwiftBaseDefines
import JobsSwiftDSL
import PhotosUI
import QuickLook
import SnapKit
import UIKit
import UniformTypeIdentifiers

@MainActor
final class IMChatVC: IMBaseVC, UITableViewDataSource, UITableViewDelegate,
    UIDocumentPickerDelegate,
    PHPickerViewControllerDelegate, QLPreviewControllerDataSource
{

    private let conversation: IMJSON
    private var localHistory: Bool {
        return conversation["localHistory"] == .bool(true)
    }
    private var focusMessageID: String?
    private var items: [IMJSON] = []
    private var polling: Task<Void, Never>?
    private var observationTask: Task<Void, Never>?
    private var replyID: String?
    private var replyReference = IMJSON.null
    private var selectedMentionIDs: [String] = []
    private var mentionAll = false
    private var featureFlags: IMJSON = .null
    private var memberProfiles: [IMJSON] = []
    private var previewMessageID: String?
    private var previewURL: URL?
    private var readOnly = false
    private weak var sendButton: UIButton?
    private weak var attachmentButton: UIButton?
    private var replacingDraft = false
    private lazy var dismissKeyboardTap = UITapGestureRecognizer.byConfig {
        [weak self] (gesture: UITapGestureRecognizer) in
        guard let self, !self.input.bounds.contains(gesture.location(in: self.input)) else {
            return
        }
        var touched = self.view.hitTest(gesture.location(in: self.view), with: nil)
        while let current = touched {
            // Moving the keyboard before a button's touch-up can cancel its action.
            if current is UIControl {
                return
            }
            touched = current.superview
        }
        self.view.byEndEditing(true)
    }.byCancelsTouchesInView(false)
    private var recorder: AVAudioRecorder?
    private var recordURL: URL?
    private let boundNamespace: IMNamespace
    private lazy var table = UITableView.make()
        .byDataSource(self)
        .byDelegate(self)
        .byRowHeight(UITableView.automaticDimension)
        .byEstimatedRowHeight(90)
        .bySeparatorStyle(.none)
        .byBackgroundColor(IMUI.background)
        .byKeyboardDismissMode(.interactive)
    private lazy var input = IMUI.field(IMStrings.text("typeMessage"))
    private lazy var compose = UIView.jobsMake {
        view in
        view.byBackgroundColor(IMUI.panel)
    }
    private lazy var note = IMUI.label("", style: .caption1, color: IMUI.green)
    private lazy var mentionButton = IMUI.button(IMStrings.text("mention")) { [weak self] in
        self?.showMentions()
    }.byTitleFont(IMUI.font(.subheadline))
    private lazy var personalButton = IMUI.button(IMStrings.text("selfState")) { [weak self] in
        self?.showPersonalState()
    }.byAccessibilityIdentifier("self.open")
    private var conversationID: String {
        conversation["id"].text
    }

    init(conversation: IMJSON, focusMessageID: String? = nil) {
        self.conversation = conversation
        self.focusMessageID = focusMessageID
        boundNamespace = IMSession.shared.namespace
        readOnly = conversation["readOnly"] == .bool(true)
        super.init(nibName: nil, bundle: nil)
        hidesBottomBarWhenPushed = true
    }

    required init?(coder: NSCoder) {
        fatalError("init(coder:) is unavailable")
    }

    override func viewDidLoad() {
        screenTitle = conversation.firstText("title", "displayName", "id")
        if !localHistory {
            rightActions = [
                IMUI.button("") { [weak self] in
                    self?.showChatSettings()
                }
                .byImage("ellipsis".sysImg).byAccessibilityLabel(IMStrings.text("chatSettings"))
            ]
        }
        super.viewDidLoad()
        assemble()
        let draft = (try? session.store.list("drafts", namespace: boundNamespace))?.first {
            $0["id"].text == conversationID
        }
        input.byText(draft?["text"].text ?? "")
        selectedMentionIDs =
            draft?["mentionedUserIds"].list.map {
                $0.text
            } ?? []
        mentionAll = draft?["mentionAll"] == .bool(true)
        input.addTarget(self, action: #selector(saveDraft), for: .editingChanged)
        input.byEnabled(!localHistory)
    }

    override func viewWillAppear(_ animated: Bool) {
        super.viewWillAppear(animated)
        session.activeConversation = conversationID
        session.markLocalRead(conversationID)
        polling?.cancel()
        polling = Task { [weak self] in
            while !Task.isCancelled {
                if UIApplication.shared.applicationState == .active {
                    await self?.refresh()
                }
                try? await Task.sleep(for: .seconds(3))
            }
        }
    }

    override func viewDidDisappear(_ animated: Bool) {
        super.viewDidDisappear(animated)
        observationTask?.cancel()
        // 媒体预览也继续验权，已观察到转发失效时须关闭仍打开的旧副本。
        if !(presentedViewController is QLPreviewController) {
            polling?.cancel()
            if session.activeConversation == conversationID {
                session.activeConversation = nil
            }
        }
        recorder?.stop()
        saveDraft()
    }

    private func assemble() {
        compose.byAddTo(content) {
            [unowned self] make in
            make.leading.trailing.equalToSuperview()
            make.bottom.equalTo(view.keyboardLayoutGuide.snp.top)
            make.height.equalTo(max(72, IMUI.font(.body).lineHeight + 32))
        }
        let attach = IMUI.button("") { [weak self] in
            self?.attachments()
        }
        attachmentButton = attach
        attach
            .byImage("plus.circle".sysImg)
            .byAccessibilityLabel(IMStrings.text("attach"))
            .byAddTo(compose) {
                [unowned self] make in
                make.leading.equalToSuperview().inset(12)
                make.centerY.equalToSuperview()
                make.width.height.equalTo(44)
            }
        let send = IMUI.button("") { [weak self] in
            self?.sendText()
        }
        sendButton = send
        send
            .byImage("arrow.up.circle.fill".sysImg)
            .byAccessibilityLabel(IMStrings.text("send"))
            .byAddTo(compose) {
                [unowned self] make in
                make.trailing.equalToSuperview().inset(14)
                make.centerY.equalToSuperview()
                make.width.height.equalTo(44)
            }
        input.byAddTo(compose) {
            [unowned self] make in
            make.leading.equalTo(attach.snp.trailing).offset(4)
            make.trailing.equalTo(send.snp.leading).offset(-4)
            make.centerY.equalToSuperview()
            make.height.greaterThanOrEqualTo(48)
        }
        let context = UIStackView.jobsMake {
            row in
            row
                .byAxis(.horizontal)
                .byAlignment(.center)
                .bySpacing(12)
        }
        let members = IMUI.button("") { [weak self] in
            self?.showGroup()
        }
        members
            .byImage("person.2.circle".sysImg)
            .byAccessibilityLabel(IMStrings.text("members"))
            .byEnabled(!localHistory).byHidden(conversation["kind"].text != "group")
        members.snp.makeConstraints {
            make in
            make.width.height.equalTo(44)
        }
        note.byNumberOfLines(1)
        mentionButton.byHidden(true)
        mentionButton.snp.makeConstraints {
            make in
            make.width.height.greaterThanOrEqualTo(44).priority(999)
        }
        context.byAddArrangedSubviews([note, mentionButton, members])
        context.byAddTo(content) {
            [unowned self] make in
            make.top.equalToSuperview()
            make.leading.trailing.equalToSuperview().inset(24)
        }
        personalButton
            .byHidden(localHistory || !session.personalStatesEnabled)
            .byAddTo(content) {
                make in
                make.top.equalTo(context.snp.bottom)
                make.leading.trailing.equalToSuperview().inset(24)
                make.height.equalTo(44)
            }
        table.byAddTo(content) {
            [unowned self] make in
            make.top.equalTo(personalButton.snp.bottom).offset(8)
            make.leading.trailing.equalToSuperview()
            make.bottom.equalTo(compose.snp.top)
        }
        table.register(IMMessageCell.self, forCellReuseIdentifier: "message")
        table.byEmptyButtonProvider { [weak self] in
            IMUI.emptyButton {
                self?.perform { [weak self] in
                    await self?.refresh()
                }
            }
        }
        note.byText(IMStrings.text(session.demo ? "demo" : "live"))
        view.jobs_addGestureRetView(dismissKeyboardTap)
    }

    @objc private func saveDraft() {
        try? session.store.save(
            [
                .object([
                    "id": .string(conversationID), "text": .string(input.text ?? ""),
                    "mentionedUserIds": .array(selectedMentionIDs.map(IMJSON.string)),
                    "mentionAll": .bool(mentionAll),
                ])
            ],
            collection: "drafts", namespace: boundNamespace)
    }

    private func editorSnapshot() throws -> IMPersonalEditorSnapshot {
        var reference = IMJSON.null
        if let replyID {
            if replyReference["messageId"].text == replyID {
                reference = replyReference
            } else if let source = items.first(where: {
                $0["id"].text == replyID
            }),
                source["deleted"] != .bool(true), source["contentUnavailable"] != .bool(true),
                (try? IMSyncProtocol.counter(source["contentVersion"])) != nil
            {
                reference = .object([
                    "messageId": .string(replyID), "sourceVersion": source["contentVersion"],
                ])
            } else {
                try IMSyncProtocol.refuse("DRAFT_SOURCE_CHANGED")
            }
        }
        return IMPersonalEditorSnapshot(
            text: input.text ?? "", reply: reference,
            mentionIDs: selectedMentionIDs, mentionAll: mentionAll)
    }

    private func showPersonalState() {
        guard !localHistory, session.namespace == boundNamespace, session.personalStatesEnabled else {
            return
        }
        do {
            let original = try editorSnapshot()
            let family = session.currentCredentialPair?.familyID
            let controller = IMPersonalStateVC(conversation: conversationID, editor: original) {
                [weak self] text, reply in
                guard let self, self.session.namespace == self.boundNamespace,
                    self.session.currentCredentialPair?.familyID == family
                else {
                    throw IMSession.SessionError.noInstance
                }
                try original.requireUnchanged(self.editorSnapshot())
                self.input.byText(text)
                self.replyID = reply == .null ? nil : reply["messageId"].text
                self.replyReference = reply
                self.selectedMentionIDs = []
                self.mentionAll = false
                self.note.byText(reply == .null ? "" : IMStrings.text("reply"))
                self.saveDraft()
            }
            navigationController?.pushViewController(controller, animated: true)
        } catch {
            IMUI.alert(self, message: error.localizedDescription)
        }
    }

    private func refresh() async {
        guard session.namespace == boundNamespace else {
            return
        }
        personalButton.byHidden(localHistory || !session.personalStatesEnabled)
        if localHistory {
            let updated = session.retainedMessages(conversationID)
            if let id = previewMessageID,
                !updated.contains(where: {
                    $0["id"].text == id
                })
            {
                previewURL = nil
                previewMessageID = nil
                if presentedViewController is QLPreviewController {
                    dismiss(animated: true)
                }
            }
            items = updated
            readOnly = true
            sendButton?.byEnabled(false)
            attachmentButton?.byEnabled(false)
            input.byEnabled(false)
            mentionButton.byHidden(true)
            note.byText(IMStrings.text("localHistory"))
            table.reloadData()
            table.byReloadEmptyViewAuto()
            return
        }
        let metadataExists =
            (try? session.store.list("conversations", namespace: boundNamespace))?.contains {
                $0["id"].text == conversationID
            } ?? false
        if !session.demo, !metadataExists || focusMessageID != nil {
            do {
                _ = try await IMMessageListService().conversation(conversationID)
            } catch {
                if let denied = error as? IMAPIError, [401, 403, 404].contains(denied.status) {
                    readOnly = true
                }
            }
        }
        guard session.namespace == boundNamespace else {
            return
        }
        await session.flushPending()
        let updated = await session.messageHistory(conversationID)
        guard session.namespace == boundNamespace else {
            return
        }
        let profiles = await session.list("conversations/" + conversationID + "/members")
        guard session.namespace == boundNamespace else {
            return
        }
        if let id = previewMessageID {
            let value = updated.first(where: {
                $0["id"].text == id
            })
            if value == nil || value?["deleted"] == .bool(true) || value?["contentUnavailable"] == .bool(true)
            {
                previewURL = nil
                previewMessageID = nil
                if presentedViewController is QLPreviewController {
                    dismiss(animated: true)
                }
            }
        }
        let changed = updated != items || profiles != memberProfiles
        memberProfiles = profiles
        items = updated
        if changed {
            let shouldScroll =
                table.contentSize.height - table.contentOffset.y - table.bounds.height < 120
            table.reloadData()
            table.byReloadEmptyViewAuto()
            if let focus = focusMessageID,
                let index = items.firstIndex(where: {
                    $0["id"].text == focus
                }),
                table.window != nil
            {
                table.byLayoutIfNeeded()
                    .byScrollToRow(IndexPath(row: index, section: 0), at: .middle, animated: false)
                focusMessageID = nil
            } else if focusMessageID == nil && shouldScroll && !items.isEmpty && table.window != nil {
                scrollToLatest()
            }
        }
        let status = session.historyStatus[boundNamespace.key + ":" + conversationID] ?? 0
        if let metadata = (try? session.store.list("conversations", namespace: boundNamespace))?.first(
            where: {
                $0["id"].text == conversationID
            })
        {
            readOnly =
                metadata["readOnly"] == .bool(true)
                || !session.demo && metadata["sendAdmission"].text.isEmpty
        }
        readOnly = readOnly || [403, 404].contains(status)
        sendButton?.byEnabled(!readOnly && !replacingDraft)
        attachmentButton?.byEnabled(!readOnly && !replacingDraft)
        if !session.demo {
            let capabilities = try? await session.action("capabilities", method: "GET")
            guard session.namespace == boundNamespace else {
                return
            }
            featureFlags = capabilities?["features"] ?? .null
        }
        mentionButton
            .byHidden(conversation["kind"].text != "group" || featureFlags["mentions"] != .bool(true))
            .byEnabled(!readOnly && !replacingDraft)
            .byTitle(
                IMStrings.text("mention")
                    + (selectedMentionIDs.isEmpty ? "" : " (" + String(selectedMentionIDs.count) + ")"))
        let key =
            session.demo
            ? "demo"
            : [403, 404].contains(status) ? "cloudUnavailable" : status == 0 ? "live" : "disconnected"
        let mentionNames = selectedMentionIDs.map {
            id in
            memberProfiles.first {
                $0["id"].text == id
            }?.firstText("displayName", "username")
                ?? IMStrings.text("member")
        }
        let selection = mentionAll ? IMStrings.text("mentionAll") : mentionNames.joined(separator: ", ")
        note.byText(IMStrings.text(key) + (selection.isEmpty ? "" : " · " + selection))
    }

    override func syncProjectionChanged() {
        guard session.namespace == boundNamespace, session.currentCredentialPair != nil else {
            items = []
            table.reloadData()
            return
        }
        if localHistory {
            items = session.retainedMessages(conversationID)
        } else {
            items = session.cachedMessages(conversationID)
            let metadata = (try? session.store.list("conversations", namespace: boundNamespace))?.first {
                $0["id"].text == conversationID
            }
            readOnly = metadata == nil || metadata?["readOnly"] == .bool(true)
            if metadata == nil {
                memberProfiles = []
            }
        }
        if let id = previewMessageID {
            let value = items.first {
                $0["id"].text == id
            }
            if value == nil || value?["deleted"] == .bool(true) || value?["contentUnavailable"] == .bool(true)
            {
                previewURL = nil
                previewMessageID = nil
                if presentedViewController is QLPreviewController {
                    dismiss(animated: true)
                }
            }
        }
        sendButton?.byEnabled(!readOnly && !localHistory && !replacingDraft)
        attachmentButton?.byEnabled(!readOnly && !localHistory && !replacingDraft)
        table.reloadData()
        table.byReloadEmptyViewAuto()
    }

    private func sendText() {
        guard !readOnly, !replacingDraft else {
            IMUI.alert(self, message: IMStrings.text("readOnly"))
            return
        }
        input.unmarkText()
        let text = input.text ?? ""
        guard !text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
        else {
            return
        }
        let reply = replyID
        let mentioned = selectedMentionIDs
        let all = mentionAll
        let clientID = UUID().uuidString
        perform { [weak self] in
            guard let self, self.session.namespace == self.boundNamespace else {
                return
            }
            do {
                try await self.session.send(
                    text, conversation: self.conversationID, reply: reply, clientID: clientID,
                    mentionedUserIDs: mentioned, mentionAll: all,
                    didEnqueue: {
                        guard self.session.namespace == self.boundNamespace, self.input.text == text else {
                            return
                        }
                        self.input.byText("")
                        self.replyID = nil
                        self.replyReference = .null
                        if self.selectedMentionIDs == mentioned && self.mentionAll == all {
                            self.selectedMentionIDs = []
                            self.mentionAll = false
                        }
                        self.saveDraft()
                    })
            } catch {
                let persisted =
                    (try? self.session.store.list(
                        "messages:" + self.conversationID, namespace: self.boundNamespace))?.contains {
                        $0["clientMsgId"].text == clientID
                    } ?? false
                if !persisted && self.session.namespace == self.boundNamespace
                    && (self.input.text ?? "").isEmpty
                {
                    self.input.byText(text)
                    self.saveDraft()
                }
                await self.refresh()
                throw error
            }
            await self.refresh()
            self.scrollToLatest()
        }
    }

    private func scrollToLatest() {
        guard !items.isEmpty, table.window != nil else {
            return
        }
        table.byLayoutIfNeeded()
            .byScrollToRow(IndexPath(row: items.count - 1, section: 0), at: .bottom, animated: false)
    }

    func tableView(_ tableView: UITableView, numberOfRowsInSection section: Int) -> Int {
        items.count
    }

    func tableView(_ tableView: UITableView, willDisplay cell: UITableViewCell, forRowAt indexPath: IndexPath)
    {
        observeVisibleRows()
    }

    func scrollViewDidEndDecelerating(_ scrollView: UIScrollView) {
        observeVisibleRows()
    }

    func scrollViewDidEndDragging(_ scrollView: UIScrollView, willDecelerate decelerate: Bool) {
        if !decelerate {
            observeVisibleRows()
        }
    }

    private func observeVisibleRows() {
        observationTask?.cancel()
        let originalMembership = conversation["membershipGeneration"].text
        observationTask = Task { [weak self] in
            do {
                try await Task.sleep(for: .milliseconds(250))
            } catch {
                return
            }
            guard let self, !Task.isCancelled, !self.localHistory,
                self.view.window != nil, UIApplication.shared.applicationState == .active,
                self.presentedViewController == nil,
                self.navigationController?.topViewController === self,
                self.session.namespace == self.boundNamespace,
                self.session.activeConversation == self.conversationID
            else {
                return
            }
            let visible = (self.table.indexPathsForVisibleRows ?? []).compactMap {
                index -> String? in
                guard self.items.indices.contains(index.row),
                    let cell = self.table.cellForRow(at: index),
                    Self.isActuallyVisible(cell, table: self.table, controller: self)
                else {
                    return nil
                }
                let message = self.items[index.row]
                guard message["senderId"] != self.session.user["id"],
                    message["deleted"] != .bool(true), message["contentUnavailable"] != .bool(true),
                    (try? IMSyncProtocol.id(message["id"])) != nil
                else {
                    return nil
                }
                return message["id"].text
            }
            await self.session.observeVisibleMessages(
                visible, conversation: self.conversationID,
                membershipGeneration: originalMembership)
        }
    }

    static func isActuallyVisible(
        _ cell: UITableViewCell, table: UITableView,
        controller: UIViewController
    ) -> Bool {
        guard controller.presentedViewController == nil, !cell.isHidden, cell.alpha >= 0.99,
            let window = table.window, cell.window === window, window.isKeyWindow,
            controller.viewIfLoaded?.window === window
        else {
            return false
        }
        let visible = cell.convert(cell.bounds, to: table).intersection(table.bounds)
        guard !visible.isEmpty, !visible.isNull else {
            return false
        }
        let point = table.convert(CGPoint(x: visible.midX, y: visible.midY), to: window)
        guard let hit = window.hitTest(point, with: nil) else {
            return false
        }
        return hit === cell || hit.isDescendant(of: cell)
    }

    func tableView(_ tableView: UITableView, cellForRowAt indexPath: IndexPath) -> UITableViewCell {
        let cell =
            tableView.dequeueReusableCell(withIdentifier: "message", for: indexPath) as! IMMessageCell
        let item = items[indexPath.row]
        cell.configure(
            item, own: item["senderId"] == session.user["id"],
            sender: memberProfiles.first {
                $0["id"] == item["senderId"]
            } ?? .object(["id": item["senderId"]]))
        cell.onAvatarLongPress = {
            [weak self] user in
            self?.navigationController?.pushViewController(IMUserProfileVC(user: user), animated: true)
        }
        return cell
    }

    func tableView(_ tableView: UITableView, didSelectRowAt indexPath: IndexPath) {
        let item = items[indexPath.row]
        if item["status"].text == "frozen" {
            messageActions(item, source: tableView.cellForRow(at: indexPath))
            return
        }
        if (!item["attachmentId"].text.isEmpty || !item["attachmentURL"].text.isEmpty)
            && item["deleted"] != .bool(true)
            && item["contentUnavailable"] != .bool(true)
        {
            openAttachment(item)
            return
        }
        view.byEndEditing(true)
    }

    func tableView(
        _ tableView: UITableView, contextMenuConfigurationForRowAt indexPath: IndexPath, point: CGPoint
    ) -> UIContextMenuConfiguration? {
        guard !localHistory else {
            return nil
        }
        let item = items[indexPath.row]
        if let cell = tableView.cellForRow(at: indexPath) as? IMMessageCell,
            cell.isAvatarPoint(point, in: tableView)
        {
            return nil
        }
        guard item["contentUnavailable"] != .bool(true), item["status"].text != "frozen" else {
            return nil
        }
        return UIContextMenuConfiguration.make {
            [weak self] _ in
            guard let self else {
                return nil
            }
            var keys = ["reply", "favorite", "pin", "report", "selectMessages"]
            if self.canEdit(item) {
                keys.append("edit")
            }
            if self.canRecall(item) {
                keys.append("delete")
            }
            if self.session.demo || self.featureFlags["forwarding"] == .bool(true),
                item["deleted"] != .bool(true), item["status"].text.isEmpty
            {
                keys.append("forward")
            }
            let actions = keys.map {
                key in
                UIAction.make(
                    title: self.actionTitle(key, item: item), attributes: key == "delete" ? .destructive : []
                ) {
                    [weak self] _ in
                    Task {
                        @MainActor in
                        self?.messageAction(key, item: item)
                    }
                }
            }
            return UIMenu.make(children: actions)
        }
    }

    private func canRecall(_ item: IMJSON) -> Bool {
        return item["senderId"] == session.user["id"]
            && item["deleted"] != .bool(true) && item["contentUnavailable"] != .bool(true)
            && (item["status"].text.isEmpty || session.demo && item["status"].text == "demo") && !readOnly
            && !localHistory && !replacingDraft && recorder?.isRecording != true
    }

    private func canEdit(_ item: IMJSON) -> Bool {
        return canRecall(item) && item["type"].text == "text" && !item["text"].text.isEmpty
    }

    private func actionTitle(_ key: String, item: IMJSON) -> String {
        if key == "delete" {
            return IMStrings.text("recallMessage")
        }
        if key == "edit" {
            return IMStrings.text("recallAndEdit")
        }
        if key == "favorite", item["favorite"] == .bool(true) {
            return IMStrings.text("removeFavorite")
        }
        if key == "pin", item["pinned"] == .bool(true) {
            return IMStrings.text("unpinMessage")
        }
        return IMStrings.text(key)
    }

    private func messageActions(_ item: IMJSON, source: UIView?) {
        guard !localHistory else {
            return
        }
        let sheet = UIAlertController.make(title: nil, message: nil, style: .actionSheet)
        var keys =
            item["status"].text == "frozen"
            ? ["reconcileOriginal"]
            : item["contentUnavailable"] == .bool(true)
                ? ["report"]
                : ["reply", "favorite", "pin", "react", "report", "selectMessages"]
                    + ((session.demo || featureFlags["forwarding"] == .bool(true))
                        && item["deleted"] != .bool(true) && item["status"].text.isEmpty ? ["forward"] : [])
        if canEdit(item) {
            keys.append("edit")
        }
        if canRecall(item) {
            keys.append("delete")
        }
        if item["status"].text == "frozen", item["type"].text == "forward",
            item["freezeReason"].text == "forward_unknown"
        {
            keys.append("retryOriginalForward")
        }
        let metadata = (try? session.store.list("conversations", namespace: boundNamespace))?.first {
            $0["id"].text == conversationID
        }
        if featureFlags["announcements"] == .bool(true), featureFlags["role_admission"] == .bool(true),
            metadata?["ownerId"] == session.user["id"],
            metadata?["kind"].text == "group", !readOnly, item["status"].text.isEmpty,
            item["deleted"] != .bool(true), item["contentUnavailable"] != .bool(true)
        {
            keys.append("publishAnnouncement")
        }
        for key in keys {
            sheet.addAction(
                UIAlertAction.make(
                    actionTitle(key, item: item), style: key == "delete" ? .destructive : .default
                ) { [weak self] in
                    self?.messageAction(key, item: item)
                })
        }
        sheet.addAction(
            UIAlertAction.make(IMStrings.text("cancel"), style: .cancel) {
            })
        sheet.popoverPresentationController?.sourceView = source ?? view
        present(sheet, animated: true)
    }

    private func messageAction(_ key: String, item: IMJSON) {
        guard session.namespace == boundNamespace else {
            return
        }
        let path = "messages/" + item["id"].text
        if key == "reconcileOriginal" {
            perform { [weak self] in
                guard let self, self.session.namespace == self.boundNamespace else {
                    return
                }
                try await self.session.reconcileOriginalMessage(item["clientMsgId"].text)
                await self.refresh()
            }
        } else if key == "retryOriginalForward" {
            IMUI.confirm(
                self, title: IMStrings.text("retryOriginalForward"), message: IMStrings.text("forwardUnknown")
            ) { [weak self] in
                self?.perform {
                    guard let self, self.session.namespace == self.boundNamespace else {
                        return
                    }
                    try await IMForwardService().retryOriginal(item["clientMsgId"].text)
                    await self.refresh()
                }
            }
        } else if key == "publishAnnouncement" {
            publishAnnouncement(item)
        } else if key == "report" {
            navigationController?.pushViewController(
                IMReportComposerVC(
                    targetType: "message", targetID: item["id"].text, targetUserID: item["senderId"].text,
                    conversationID: conversationID, messageIDs: [item["id"].text]), animated: true)
        } else if key == "selectMessages" {
            navigationController?.pushViewController(
                IMReportComposerVC(
                    targetType: "conversation", targetID: conversationID, conversationID: conversationID),
                animated: true)
        } else if key == "reply" {
            replyID = item["id"].text
            replyReference = .object(["messageId": item["id"], "sourceVersion": item["contentVersion"]])
            note.byText(IMStrings.text("reply") + " · " + item["text"].text)
            input.becomeFirstResponder()
        } else if key == "edit" {
            beginEditing(item)
        } else if key == "delete" {
            guard canRecall(item) else {
                return
            }
            IMUI.confirm(self, title: IMStrings.text("recallMessage"), message: item["text"].text) {
                [weak self] in
                self?.perform {
                    guard let self, self.session.namespace == self.boundNamespace else {
                        return
                    }
                    try await self.session.recallMessage(item, conversationID: self.conversationID)
                    await self.refresh()
                }
            }
        } else if key == "react" {
            IMUI.prompt(self, title: IMStrings.text(key), value: "👍") {
                [weak self] text in
                self?.perform {
                    guard let self, self.session.namespace == self.boundNamespace else {
                        return
                    }
                    _ = try await self.session.action(path + "/reactions", body: ["emoji": .string(text)])
                    await self.refresh()
                }
            }
        } else if key == "forward" {
            showForward(item)
        } else {
            perform { [weak self] in
                guard let self, self.session.namespace == self.boundNamespace else {
                    return
                }
                _ = try await self.session.action(
                    path + "/" + key,
                    method: (key == "favorite" && item["favorite"] == .bool(true)
                        || key == "pin" && item["pinned"] == .bool(true)) ? "DELETE" : "PUT")
                await self.refresh()
            }
        }
    }

    private func showForward(_ displayed: IMJSON) {
        perform { [weak self] in
            guard let self, self.session.namespace == self.boundNamespace else {
                return
            }
            let source: IMJSON
            do {
                source = self.session.demo ? displayed : try await IMForwardService().prepare(displayed)
            } catch {
                await self.refresh()
                throw error
            }
            let conversations = await self.session.conversationSnapshot()
            guard self.session.namespace == self.boundNamespace else {
                return
            }
            let targets = conversations.filter {
                $0["readOnly"] != .bool(true)
            }
            let picker = UIAlertController.make(
                title: IMStrings.text("forward"),
                message: targets.isEmpty ? IMStrings.text("empty") : nil, style: .actionSheet)
            if targets.isEmpty {
                picker.addAction(
                    UIAlertAction.make(IMStrings.text("reload")) { [weak self] in
                        self?.showForward(displayed)
                    })
            }
            for target in targets {
                picker.addAction(
                    UIAlertAction.make(target.firstText("title", "displayName", "id")) { [weak self] in
                        self?.perform {
                            guard let self, self.session.namespace == self.boundNamespace else {
                                return
                            }
                            if self.session.demo {
                                try await self.session.send(
                                    source["text"].text, conversation: target["id"].text)
                            } else {
                                try await IMForwardService().send(source, to: target["id"].text)
                            }
                            await self.refresh()
                        }
                    })
            }
            picker.addAction(
                UIAlertAction.make(IMStrings.text("cancel"), style: .cancel) {
                })
            picker.popoverPresentationController?.sourceView = self.view
            self.present(picker, animated: true)
        }
    }

    private func beginEditing(_ item: IMJSON) {
        guard canEdit(item) else {
            return
        }
        let current = input.text ?? ""
        let choose: (IMDraftReplacementPolicy) -> Void = {
            [weak self] policy in
            guard let self, (self.input.text ?? "") == current else {
                return
            }
            self.replacingDraft = true
            self.input.byEnabled(false)
            self.sendButton?.byEnabled(false)
            self.attachmentButton?.byEnabled(false)
            self.mentionButton.byEnabled(false)
            self.perform {
                defer {
                    self.replacingDraft = false
                    self.input.byEnabled(!self.readOnly && !self.localHistory)
                    self.sendButton?.byEnabled(!self.readOnly)
                    self.attachmentButton?.byEnabled(!self.readOnly)
                    self.mentionButton.byEnabled(!self.readOnly)
                }
                guard self.session.namespace == self.boundNamespace else {
                    throw IMSession.SessionError.noInstance
                }
                let original = try await self.session.recallForEditing(
                    item, conversationID: self.conversationID, currentDraft: current, policy: policy)
                guard self.session.namespace == self.boundNamespace, (self.input.text ?? "") == current else {
                    return
                }
                self.input.byText(original)
                self.replyID = nil
                self.replyReference = .null
                self.selectedMentionIDs = []
                self.mentionAll = false
                self.note.byText("")
                self.saveDraft()
                await self.refresh()
                self.input.becomeFirstResponder()
            }
        }
        if current.isEmpty {
            IMUI.confirm(
                self, title: IMStrings.text("recallAndEdit"), message: IMStrings.text("recallEditWarning")
            ) {
                choose(.overwrite)
            }
        } else {
            let alert = UIAlertController.make(
                title: IMStrings.text("draftConflict"), message: IMStrings.text("recallEditWarning"),
                style: .alert)
            alert.addAction(
                UIAlertAction.make(IMStrings.text("overwriteDraft"), style: .destructive) {
                    choose(.overwrite)
                })
            alert.addAction(
                UIAlertAction.make(IMStrings.text("save")) {
                    choose(.saveCurrent)
                })
            IMUI.presentDialog(alert, on: self)
        }
    }

    private func showChatSettings() {
        if conversation["kind"].text == "group" {
            showGroup()
            return
        }
        guard session.namespace == boundNamespace else {
            return
        }
        let knownPeer = conversation["peer"]
        guard
            let peer = !knownPeer["id"].text.isEmpty
                ? knownPeer
                : memberProfiles.first(where: {
                    !$0["id"].text.isEmpty && $0["id"] != session.user["id"]
                })
        else {
            perform { [weak self] in
                await self?.refresh()
            }
            return
        }
        navigationController?.pushViewController(
            IMChatSettingsVC(
                user: peer, conversationID: conversationID,
                recoverDrafts: { [weak self] in
                    self?.showSavedEditingDrafts()
                }), animated: true)
    }

    private func showSavedEditingDrafts() {
        guard session.namespace == boundNamespace else {
            return
        }
        let picker = IMSavedEditingDraftsVC(conversationID: conversationID) {
            [weak self] item in
            guard let self else {
                return
            }
            let restore = { [weak self] in
                guard let self, self.session.namespace == self.boundNamespace else {
                    return
                }
                do {
                    let text = try self.session.restoreEditingDraft(
                        id: item["id"].text, conversationID: self.conversationID)
                    self.input.byText(text)
                    self.saveDraft()
                    self.navigationController?.popToViewController(self, animated: true)
                } catch {
                    IMUI.alert(self, message: error.localizedDescription)
                }
            }
            if (self.input.text ?? "").isEmpty {
                restore()
            } else {
                IMUI.confirm(
                    self.navigationController?.visibleViewController ?? self,
                    title: IMStrings.text("draftConflict"),
                    message: IMStrings.text("restoreEditingDraft"), action: restore)
            }
        }
        navigationController?.pushViewController(picker, animated: true)
    }

    private func showGroup() {
        navigationController?.pushViewController(IMGroupVC(conversation: conversation), animated: true)
    }

    private func showMentions() {
        perform { [weak self] in
            guard let self, self.session.namespace == self.boundNamespace, !self.readOnly else {
                return
            }
            let metadata = try await IMMessageListService().conversation(self.conversationID)
            let response = try await self.session.action(
                "conversations/" + self.conversationID + "/members", method: "GET")
            guard self.session.namespace == self.boundNamespace else {
                return
            }
            self.memberProfiles = response["items"].list
            let sheet = UIAlertController.make(
                title: IMStrings.text("mention"), message: nil, style: .actionSheet)
            if metadata["ownerId"] == self.session.user["id"],
                self.featureFlags["role_admission"] == .bool(true)
            {
                sheet.addAction(
                    UIAlertAction.make(IMStrings.text("mentionAll")) { [weak self] in
                        self?.selectedMentionIDs = []
                        self?.mentionAll = true
                        self?.saveDraft()
                        self?.refreshMentionSelection()
                    })
            }
            for person in self.memberProfiles
            where person["id"] != self.session.user["id"] && person["state"].text != "tombstoned" {
                let id = person["id"].text
                let selected = self.selectedMentionIDs.contains(id)
                let title =
                    IMStrings.text(selected ? "removeMention" : "mention") + " "
                    + person.firstText("displayName", "username", "id")
                sheet.addAction(
                    UIAlertAction.make(title) { [weak self] in
                        guard let self else {
                            return
                        }
                        self.mentionAll = false
                        if selected {
                            self.selectedMentionIDs.removeAll {
                                $0 == id
                            }
                        } else {
                            self.selectedMentionIDs.append(id)
                        }
                        self.saveDraft()
                        self.refreshMentionSelection()
                    })
            }
            sheet.addAction(
                UIAlertAction.make(IMStrings.text("clearMentions")) { [weak self] in
                    self?.selectedMentionIDs = []
                    self?.mentionAll = false
                    self?.saveDraft()
                    self?.refreshMentionSelection()
                })
            sheet.addAction(
                UIAlertAction.make(IMStrings.text("cancel"), style: .cancel) {
                })
            sheet.popoverPresentationController?.sourceView = self.mentionButton
            self.present(sheet, animated: true)
        }
    }

    private func refreshMentionSelection() {
        mentionButton.byTitle(
            IMStrings.text("mention")
                + (selectedMentionIDs.isEmpty ? "" : " (" + String(selectedMentionIDs.count) + ")"))
        note.byText(
            mentionAll
                ? IMStrings.text("mentionAll")
                : selectedMentionIDs.map {
                    id in
                    memberProfiles.first {
                        $0["id"].text == id
                    }?.firstText("displayName", "username")
                        ?? IMStrings.text("member")
                }.joined(separator: ", "))
    }

    private func publishAnnouncement(_ source: IMJSON) {
        perform { [weak self] in
            guard let self, self.session.namespace == self.boundNamespace else {
                return
            }
            let service = IMAnnouncementService()
            let current = try await service.load(self.conversationID)
            guard self.session.namespace == self.boundNamespace else {
                return
            }
            IMUI.confirm(
                self, title: IMStrings.text("publishAnnouncement"),
                message: IMStrings.text("announcementReference") + "\n"
                    + String(source["text"].text.prefix(200))
            ) { [weak self] in
                self?.perform { [weak self] in
                    guard let self, self.session.namespace == self.boundNamespace else {
                        return
                    }
                    _ = try await service.publish(source, expectedVersion: current["version"].text)
                    await self.refresh()
                }
            }
        }
    }

    private func attachments() {
        guard !readOnly, !replacingDraft else {
            IMUI.alert(self, message: IMStrings.text("readOnly"))
            return
        }
        let picker = UIAlertController.make(
            title: IMStrings.text("attach"), message: nil, style: .actionSheet)
        for kind in ["image", "video", "file", "audio"] {
            picker.addAction(
                UIAlertAction.make(IMStrings.text(kind)) { [weak self] in
                    guard let self else {
                        return
                    }
                    if kind == "audio" {
                        self.recordAudio()
                    } else if kind == "file" {
                        let document = UIDocumentPickerViewController.makeImporting(delegate: self)
                        self.present(document, animated: true)
                    } else {
                        let config = PHPickerConfiguration.make(filter: kind == "video" ? .videos : .images)
                            .byPreferredAssetRepresentationMode(.compatible)
                        let photo = PHPickerViewController.make(configuration: config).byDelegate(self)
                        self.present(photo, animated: true)
                    }
                })
        }
        picker.addAction(
            UIAlertAction.make(IMStrings.text("cancel"), style: .cancel) {
            })
        picker.popoverPresentationController?.sourceView = compose
        present(picker, animated: true)
    }

    func documentPicker(_ controller: UIDocumentPickerViewController, didPickDocumentsAt urls: [URL]) {
        guard let url = urls.first else {
            return
        }
        sendFile(url, type: "file")
    }

    func picker(_ picker: PHPickerViewController, didFinishPicking results: [PHPickerResult]) {
        picker.dismiss(animated: true)
        guard let provider = results.first?.itemProvider else {
            return
        }
        let type = provider.registeredTypeIdentifiers.first ?? UTType.data.identifier
        Task {
            @MainActor [weak self] in
            do {
                let copy: URL = try await withCheckedThrowingContinuation {
                    continuation in
                    provider.loadFileRepresentation(forTypeIdentifier: type) {
                        url, error in
                        guard let url else {
                            continuation.resume(throwing: error ?? CocoaError(.fileReadUnknown))
                            return
                        }
                        let copy = FileManager.default.temporaryDirectory.appendingPathComponent(
                            UUID().uuidString + "-" + url.lastPathComponent)
                        do {
                            try FileManager.default.copyItem(at: url, to: copy)
                            continuation.resume(returning: copy)
                        } catch {
                            continuation.resume(throwing: error)
                        }
                    }
                }
                self?.sendFile(copy, type: UTType(type)?.conforms(to: .movie) == true ? "video" : "image")
            } catch {
                guard let self else {
                    return
                }
                IMUI.alert(self, message: error.localizedDescription)
            }
        }
    }

    private func sendFile(_ url: URL, type: String) {
        perform { [weak self] in
            guard let self, self.session.namespace == self.boundNamespace else {
                return
            }
            let admission = try self.session.admissionSnapshot(self.conversationID)
            let mentioned = self.selectedMentionIDs
            let all = self.mentionAll
            let access = url.startAccessingSecurityScopedResource()
            defer {
                if access {
                    url.stopAccessingSecurityScopedResource()
                }
            }
            let values = try url.resourceValues(forKeys: [.fileSizeKey, .isRegularFileKey])
            let limit = type == "image" ? 20 * 1024 * 1024 : 64 * 1024 * 1024
            guard values.isRegularFile == true, let size = values.fileSize, size > 0, size <= limit else {
                throw IMAPIError(
                    status: 413, code: "MEDIA_LIMIT", message: "Image ≤ 20 MiB · File ≤ 64 MiB")
            }
            guard !self.readOnly else {
                throw IMAPIError(
                    status: 403, code: "CONVERSATION_READ_ONLY", message: IMStrings.text("readOnly"))
            }
            let mime =
                UTType(filenameExtension: url.pathExtension)?.preferredMIMEType
                ?? "application/octet-stream"
            let attachment: IMJSON
            if self.session.demo {
                let id = UUID().uuidString
                attachment = .object(["id": .string(id), "name": .string(url.lastPathComponent)])
                let destination = self.mediaPath(id: id, filename: url.lastPathComponent)
                try FileManager.default.copyItem(at: url, to: destination)
            } else {
                _ = try await self.session.verifyRestoreEpoch()
                guard self.session.namespace == self.boundNamespace else {
                    return
                }
                attachment = try await self.session.authorized {
                    api, token in
                    return try await api.upload(url, mimeType: mime, token: token)
                }
            }
            guard self.session.namespace == self.boundNamespace else {
                return
            }
            try await self.session.send(
                url.lastPathComponent, conversation: self.conversationID, type: type,
                attachment: attachment["id"].text, admission: admission,
                mentionedUserIDs: mentioned, mentionAll: all,
                didEnqueue: { [weak self] in
                    guard let self, self.session.namespace == self.boundNamespace,
                        self.selectedMentionIDs == mentioned, self.mentionAll == all
                    else {
                        return
                    }
                    self.selectedMentionIDs = []
                    self.mentionAll = false
                    self.saveDraft()
                })
            await self.refresh()
        }
    }

    private func mediaPath(id: String, filename: String) -> URL {
        let folder = FileManager.default.urls(for: .cachesDirectory, in: .userDomainMask)[0]
            .appendingPathComponent(boundNamespace.key)
        try? FileManager.default.createDirectory(at: folder, withIntermediateDirectories: true)
        return folder.appendingPathComponent(
            id + "-" + URL(fileURLWithPath: filename).lastPathComponent)
    }

    private func openAttachment(_ item: IMJSON) {
        perform { [weak self] in
            guard let self else {
                return
            }
            let file: URL
            if self.localHistory {
                IMUI.alert(self, message: IMStrings.text("localHistory"))
                return
            }
            if self.session.demo {
                file = self.mediaPath(id: item["attachmentId"].text, filename: item["text"].text)
            } else {
                file = try await IMMediaCache.file(item, namespace: self.boundNamespace)
            }
            guard self.session.namespace == self.boundNamespace else {
                return
            }
            self.previewMessageID = item["id"].text
            self.previewURL = file
            let controller = QLPreviewController.make(dataSource: self)
            self.present(controller, animated: true)
        }
    }

    private func recordAudio() {
        AVAudioSession.sharedInstance().requestRecordPermission {
            [weak self] allowed in
            Task {
                @MainActor in
                guard let self, allowed else {
                    return
                }
                self.perform {
                    let audio = AVAudioSession.sharedInstance()
                    try audio.setCategory(.playAndRecord, mode: .default, options: [.defaultToSpeaker])
                    try audio.setActive(true)
                    let file = FileManager.default.temporaryDirectory.appendingPathComponent(
                        UUID().uuidString + ".m4a")
                    self.recordURL = file
                    self.recorder = try AVAudioRecorder.make(url: file)
                    self.recorder?.record()
                    let alert = UIAlertController.make(
                        title: IMStrings.text("recording"), message: nil, style: .alert)
                    alert.addAction(
                        UIAlertAction.make(IMStrings.text("cancel"), style: .cancel) {
                            self.recorder?.stop()
                            try? audio.setActive(false)
                        })
                    alert.addAction(
                        UIAlertAction.make(IMStrings.text("stop")) {
                            self.recorder?.stop()
                            try? audio.setActive(false)
                            self.sendFile(file, type: "audio")
                        })
                    self.present(alert, animated: true)
                }
            }
        }
    }

    func numberOfPreviewItems(in controller: QLPreviewController) -> Int {
        previewURL == nil ? 0 : 1
    }

    func previewController(_ controller: QLPreviewController, previewItemAt index: Int)
        -> QLPreviewItem
    {
        previewURL! as NSURL
    }
}
