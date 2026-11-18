//
//  IMPersonalStateVC.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月16日，星期一.
//

import JobsByUIKit
import JobsIMCore
import JobsIMTransport
import JobsSwiftBaseDefines
import JobsSwiftDSL
import SnapKit
import UIKit

@MainActor
final class IMPersonalStateVC: IMBaseVC, UITableViewDataSource, UITableViewDelegate {

    private struct Row {
        let key: String
        let detail: String
        let action: (() -> Void)?
    }

    private let conversation: String?
    private let editor: IMPersonalEditorSnapshot?
    private let adopt: ((String, IMJSON) throws -> Void)?
    private let personal = IMPersonalStateService()
    private var rows: [Row] = []
    private var viewValue = IMJSON.null
    private var draftValue = IMJSON.null
    private var readValue = IMJSON.null
    private var busy = false
    private var work: Task<Void, Never>?
    private var operationNote = ""
    private lazy var table = UITableView.make(style: .insetGrouped)
        .byDelegate(self)
        .byDataSource(self)
        .byRowHeight(UITableView.automaticDimension)
        .byEstimatedRowHeight(72)
        .byBackgroundColor(IMUI.background)

    init(conversation: String? = nil, editor: IMPersonalEditorSnapshot? = nil,
         adopt: ((String, IMJSON) throws -> Void)? = nil) {
        self.conversation = conversation
        self.editor = editor
        self.adopt = adopt
        super.init(nibName: nil, bundle: nil)
    }

    required init?(coder: NSCoder) {
        fatalError("init(coder:) is unavailable")
    }

    override func viewDidLoad() {
        screenTitle = IMStrings.text("selfState")
        super.viewDidLoad()
        table.byAddTo(content) { make in
            make.edges.equalToSuperview()
        }
        table.byEmptyButtonProvider { [weak self] in
            IMUI.emptyButton {
                self?.reload()
            }
        }
    }

    override func viewWillAppear(_ animated: Bool) {
        super.viewWillAppear(animated)
        reload()
    }

    override func viewDidDisappear(_ animated: Bool) {
        super.viewDidDisappear(animated)
        work?.cancel()
    }

    private func reload() {
        run { [weak self] in
            try await self?.fetch()
        }
    }

    private func run(_ action: @escaping @MainActor () async throws -> Void) {
        guard !busy else {
            return
        }
        busy = true
        table.reloadData()
        work = Task { [weak self] in
            guard let self else {
                return
            }
            defer {
                self.busy = false
                self.table.reloadData()
                self.table.byReloadEmptyViewAuto()
            }
            do {
                try self.personal.requireCurrent()
                try await action()
                try self.personal.requireCurrent()
            } catch {
                if Task.isCancelled {
                    return
                }
                self.operationNote = error.localizedDescription
                // A stale response is never a reason to expose an old projection or alter an intent.
                if (try? self.personal.requireCurrent()) == nil {
                    self.rows = []
                } else {
                    try? await self.fetch()
                }
                IMUI.alert(self, message: error.localizedDescription)
            }
        }
    }

    private func fetch() async throws {
        try personal.requireCurrent()
        if let conversation {
            viewValue = .null
            readValue = .null
            for kind in ["view", "read"] {
                do {
                    let value = try await personal.read(kind, conversation: conversation)
                    if kind == "view" {
                        viewValue = value
                    } else {
                        readValue = value
                    }
                } catch let error as IMAPIError where error.status == 403
                    && ["NOT_A_MEMBER", "HISTORY_NOT_ALLOWED", "FORBIDDEN"].contains(error.code) {
                    // Retained personal draft text does not grant current conversation access.
                    continue
                }
            }
            let descriptor = try await session.verifyRestoreEpoch()
            try personal.requireCurrent()
            draftValue = descriptor["features"]["cloud_drafts"] == .bool(true)
                ? try await personal.read("draft", conversation: conversation) : .null
        }
        try personal.requireCurrent()
        rebuild()
    }

    private func rebuild() {
        rows = [Row(key: "selfStateHelp", detail: operationNote, action: nil)]
        if let conversation {
            if readValue != .null {
                let point = readValue["readPoint"]
                let current = readValue["writable"] == .bool(true)
                    && !["needs_current_observation", "membership_unavailable"].contains(readValue["continuityStatus"].text)
                rows.append(Row(key: current ? "readProgress" : "readRetained", detail: point["epoch"].text + ":" + point["sequence"].text, action: nil))
                rows.append(Row(key: readValue["countStatus"] == .string("exact_current_source") ? "serverUnread" : "countNotComputed",
                                detail: readValue["countStatus"] == .string("exact_current_source") ? readValue["unreadCount"].text : "", action: nil))
                let local = (try? session.store.list("unread:" + conversation, namespace: personal.namespace).count) ?? 0
                rows.append(Row(key: "localUnread", detail: String(local), action: nil))
            }
            if viewValue != .null {
                let observed = viewValue
                let muted = IMSyncProtocol.date(observed["mutedUntil"].text).map { $0 > Date() } ?? false
                let actions: [(String, String, IMJSON)] = [
                    (observed["manualUnread"] == .bool(true) ? "clearManualUnread" : "markManualUnread", "manualUnread", .bool(observed["manualUnread"] != .bool(true))),
                    (observed["pinnedRank"] == .null ? "pinChat" : "unpinChat", "pinnedRank", observed["pinnedRank"] == .null ? .number(0) : .null),
                    (muted ? "unmuteChat" : "muteChat", "mutedUntil", muted ? .null : .string(Date().addingTimeInterval(86400).ISO8601Format())),
                    (observed["archived"] == .bool(true) ? "unarchiveChat" : "archiveChat", "archived", .bool(observed["archived"] != .bool(true))),
                    (observed["hidden"] == .bool(true) ? "showChat" : "hideChat", "hidden", .bool(observed["hidden"] != .bool(true))),
                ]
                for (key, field, value) in actions {
                    rows.append(Row(key: key, detail: "", action: { [weak self] in
                        self?.run { [weak self] in
                            guard let self else {
                                return
                            }
                            _ = try await self.personal.changeView([field: value], conversation: conversation, expected: observed)
                            try await self.fetch()
                        }
                    }))
                }
            }
            appendDraftActions(conversation)
        } else {
            let conversations = (try? session.store.list("conversations", namespace: personal.namespace)) ?? []
            // Retired, unpublished text is only returned by exact GET; keep a discoverable route after leaving.
            for id in (try? personal.retainedDraftConversationIDs()) ?? [] {
                let title = conversations.first { $0["id"].text == id }?.firstText("title", "displayName") ?? IMStrings.text("retainedDraft")
                rows.append(Row(key: "cloudDraft", detail: title, action: { [weak self] in
                    self?.navigationController?.pushViewController(IMPersonalStateVC(conversation: id), animated: true)
                }))
            }
        }
        appendOperations()
        rows.append(Row(key: "reload", detail: "", action: { [weak self] in
            self?.reload()
        }))
        table.reloadData()
        table.byReloadEmptyViewAuto()
    }

    private func appendDraftActions(_ conversation: String) {
        guard draftValue != .null else {
            return
        }
        let observed = draftValue
        rows.append(Row(key: "cloudDraftHelp", detail: observed["ownedText"].text, action: nil))
        if observed["replyState"] == .string("unavailable") {
            rows.append(Row(key: "draftReplyUnavailable", detail: "", action: nil))
        }
        if let editor, observed["writable"] == .bool(true) {
            rows.append(Row(key: "saveCloudDraft", detail: "", action: { [weak self] in
                self?.writeDraft(editor, expected: observed, clear: false)
            }))
        }
        rows.append(Row(key: "clearCloudDraft", detail: "", action: { [weak self] in
            guard let self else {
                return
            }
            IMUI.confirm(self, title: IMStrings.text("clearCloudDraft"), message: IMStrings.text("cloudDraftHelp")) { [weak self] in
                self?.writeDraft(IMPersonalEditorSnapshot(text: "", reply: .null, mentionIDs: [], mentionAll: false), expected: observed, clear: true)
            }
        }))
        for recovery in [false, true] {
            let text = recovery ? observed["recovery"]["ownedText"].text : observed["ownedText"].text
            guard !text.isEmpty else {
                continue
            }
            rows.append(Row(key: recovery ? "recoverCloudDraft" : adopt == nil ? "copyOwnedDraft" : "loadCloudDraft", detail: "", action: { [weak self] in
                self?.adoptDraft(expected: observed, recovery: recovery, includeReply: false)
            }))
            if !recovery, adopt != nil, observed["replyState"] == .string("available") {
                rows.append(Row(key: "loadDraftWithReply", detail: IMStrings.text("reply"), action: { [weak self] in
                    self?.adoptDraft(expected: observed, recovery: false, includeReply: true)
                }))
            }
        }
    }

    private func writeDraft(_ editor: IMPersonalEditorSnapshot, expected: IMJSON, clear: Bool) {
        run { [weak self] in
            guard let self, let conversation = self.conversation else {
                return
            }
            let result = try await self.personal.saveDraft(editor, conversation: conversation, expected: expected, clear: clear)
            self.operationNote = result["status"] == .string("retired") ? IMStrings.text("draftRetired")
                : result["status"] == .string("prepared") ? IMStrings.text("pendingSelfState") : ""
            try await self.fetch()
        }
    }

    private func adoptDraft(expected: IMJSON, recovery: Bool, includeReply: Bool) {
        IMUI.confirm(self, title: IMStrings.text(recovery ? "recoverCloudDraft" : "loadCloudDraft"), message: IMStrings.text("cloudDraftHelp")) { [weak self] in
            self?.run { [weak self] in
                guard let self, let conversation = self.conversation else {
                    return
                }
                let (text, reply) = try await self.personal.textForAdoption(conversation: conversation, expected: expected,
                                                                         recovery: recovery, includeReply: includeReply)
                if let adopt = self.adopt {
                    try adopt(text, reply)
                    self.navigationController?.popViewController(animated: true)
                } else {
                    let controller = UIActivityViewController.make([text])
                    controller.popoverPresentationController?.sourceView = self.view
                    self.present(controller, animated: true)
                }
            }
        }
    }

    private func appendOperations() {
        let operations = (try? personal.operations(conversation: conversation)) ?? []
        if operations.isEmpty {
            rows.append(Row(key: "noPendingState", detail: "", action: nil))
        }
        for operation in operations {
            let title = operation["state"] == .string("rejected") ? "archiveRejectedState" : "pendingSelfState"
            rows.append(Row(key: title, detail: operation["method"].text + " · " + operation["reason"].text, action: { [weak self] in
                guard let self else {
                    return
                }
                if operation["state"] == .string("rejected") {
                    self.run {
                        try self.personal.archiveRejected(operation)
                        try await self.fetch()
                    }
                    return
                }
                let sheet = UIAlertController.make(title: IMStrings.text("pendingSelfState"), message: nil, style: .actionSheet)
                for retry in [false, true] {
                    sheet.addAction(UIAlertAction.make(IMStrings.text(retry ? "retryOriginalState" : "checkStateResult")) { [weak self] in
                        self?.run { [weak self] in
                            guard let self else {
                                return
                            }
                            let result = try await self.personal.resume(operation, retry: retry)
                            self.operationNote = result["status"] == .string("retired") ? IMStrings.text("draftRetired") : ""
                            try await self.fetch()
                        }
                    })
                }
                sheet.addAction(UIAlertAction.make(IMStrings.text("cancel"), style: .cancel) {})
                sheet.popoverPresentationController?.sourceView = self.view
                self.present(sheet, animated: true)
            }))
        }
    }

    func tableView(_ tableView: UITableView, numberOfRowsInSection section: Int) -> Int {
        return rows.count
    }

    func tableView(_ tableView: UITableView, cellForRowAt indexPath: IndexPath) -> UITableViewCell {
        let row = rows[indexPath.row]
        return UITableViewCell.make(style: .subtitle, reuseIdentifier: nil)
            .byText(IMStrings.text(row.key))
            .bySecondaryText(row.detail)
            .byTitleNumberOfLines(0)
            .byDetailTitleNumberOfLines(0)
            .byTitleCor(IMUI.foreground)
            .byDetailTitleCor(IMUI.secondary)
            .byIMThemeAccessoryType(row.action == nil ? .none : .disclosureIndicator)
            .byUserInteractionEnabled(!busy && row.action != nil)
            .byAccessibilityIdentifier("self." + row.key)
    }

    func tableView(_ tableView: UITableView, didSelectRowAt indexPath: IndexPath) {
        tableView.deselectRow(at: indexPath, animated: true)
        guard !busy else {
            return
        }
        rows[indexPath.row].action?()
    }
}
