//
//  IMDataExportVC.swift
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
final class IMDataExportVC: IMBaseVC,
                            UITableViewDataSource,
                            UITableViewDelegate,
                            UIDocumentPickerDelegate {

    private struct Row {
        let title: String
        let detail: String
        let action: (() -> Void)?
    }

    private var mounted = false
    private var lifecycle = UUID()
    private var busy = false
    private var enabled = false
    private var shown = 20
    private var originals: [IMJSON] = []
    private var views: [String: IMJSON] = [:]
    private var rows: [[Row]] = []
    private var work: Task<Void, Never>?
    private var polling: Task<Void, Never>?
    private var exportFile: URL?
    private var presentingSave = false
    private lazy var exports = IMDataExportService(isCurrent: { [weak self] in
        return self?.mounted == true
    })
    private lazy var summary = IMUI.label(IMStrings.text("exportSummary"), style: .footnote, color: IMUI.secondary)
    private lazy var notice = IMUI.label("", style: .footnote, color: IMUI.secondary)
    private lazy var createButton = IMUI.button(IMStrings.text("exportCreate"), primary: true) { [weak self] in
        self?.createExport()
    }
    private lazy var reloadButton = IMUI.button(IMStrings.text("reload")) { [weak self] in
        self?.reload()
    }
    private lazy var header = IMUI.stack(spacing: 10)
    private lazy var table = UITableView.make(style: .insetGrouped)
        .byDelegate(self)
        .byDataSource(self)
        .byRowHeight(UITableView.automaticDimension)
        .byEstimatedRowHeight(64)
        .byBackgroundColor(IMUI.background)

    private func createExport() {
        run { [weak self] in
            guard let self else {
                return
            }
            _ = try await self.exports.create()
            try await self.fetch()
        }
    }

    override func viewDidLoad() {
        screenTitle = IMStrings.text("dataExport")
        super.viewDidLoad()
        header.byAddArrangedSubviews([summary, notice, createButton, reloadButton])
            .byAddTo(content) { make in
                make.top.left.right.equalToSuperview().inset(16)
            }
        table.byAddTo(content) { [unowned self] make in
            make.top.equalTo(header.snp.bottom).offset(8)
            make.left.right.bottom.equalToSuperview()
        }.byEmptyButtonProvider { [weak self] in
            IMUI.emptyButton(title: IMStrings.text("exportEmpty")) {
                self?.reload()
            }
        }
    }

    override func viewWillAppear(_ animated: Bool) {
        super.viewWillAppear(animated)
        mounted = true
        lifecycle = UUID()
        reload()
        polling = Task { [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(for: .seconds(5))
                guard let self, self.mounted, !Task.isCancelled else {
                    return
                }
                if self.views.values.contains(where: { ["requested", "building"].contains($0["job"]["state"].text) }) {
                    self.reload()
                }
            }
        }
    }

    override func viewDidDisappear(_ animated: Bool) {
        super.viewDidDisappear(animated)
        mounted = false
        lifecycle = UUID()
        work?.cancel()
        polling?.cancel()
        busy = false
        if !presentingSave {
            discardTemporary()
        }
    }

    private func reload() {
        run { [weak self] in
            try await self?.fetch()
        }
    }

    private func run(_ operation: @escaping @MainActor () async throws -> Void) {
        guard mounted, !busy else {
            return
        }
        let generation = lifecycle
        busy = true
        notice.byText(IMStrings.text("pending"))
        rebuild()
        work = Task { [weak self] in
            guard let self, self.mounted, self.lifecycle == generation else {
                return
            }
            defer {
                if self.lifecycle == generation {
                    self.busy = false
                    self.rebuild()
                }
            }
            do {
                try self.exports.requireCurrent()
                try await operation()
                try self.exports.requireCurrent()
            } catch {
                guard self.mounted, self.lifecycle == generation, !Task.isCancelled else {
                    return
                }
                self.views = [:]
                self.notice.byText(IMStrings.text((error as? IMAPIError)?.status == 429 ? "exportLimited" : "exportNotConfirmed"))
                if (try? self.exports.requireCurrent()) == nil {
                    self.originals = []
                } else {
                    self.originals = (try? self.exports.rows()) ?? self.originals
                }
            }
        }
    }

    private func fetch() async throws {
        views = [:]
        enabled = try await exports.capability()
        originals = try exports.rows()
        var failed = false
        for row in originals.prefix(shown) {
            do {
                if let value = try await exports.status(row["id"].text) {
                    views[row["id"].text] = value
                }
            } catch {
                try exports.requireCurrent()
                failed = true
            }
        }
        try exports.requireCurrent()
        originals = try exports.rows()
        notice.byText(IMStrings.text(failed ? "exportNotConfirmed" : enabled ? "exportLocalOnly" : "exportRestricted"))
        rebuild()
    }

    private func rebuild() {
        guard isViewLoaded else {
            return
        }
        createButton.byEnabled(enabled && !busy && !originals.contains(where: { $0["jobId"] == .null }))
        reloadButton.byEnabled(!busy)
        rows = originals.prefix(shown).map { original in
            let id = original["id"].text
            let value = views[id] ?? .null
            let job = value["job"]
            let status = job == .null ? "unknown" : job["state"].text
            var result = [Row(title: IMStrings.text("export_" + status), detail: time(original["createdAt"].text), action: nil)]
            if job != .null {
                result.append(Row(title: IMStrings.text("exportDeadline"), detail: time(job["expiresAt"].text), action: nil))
                if !["published", "cleared"].contains(job["cleanupState"].text) {
                    result.append(Row(title: IMStrings.text("exportCleanup"), detail: "", action: nil))
                }
            }
            result.append(Row(title: IMStrings.text("exportCheck"), detail: "", action: { [weak self] in
                self?.check(id)
            }))
            if original["jobId"] == .null, enabled {
                result.append(Row(title: IMStrings.text("exportRetry"), detail: IMStrings.text("exportPendingHint"), action: { [weak self] in
                    guard let self else {
                        return
                    }
                    IMUI.confirm(self, title: IMStrings.text("exportRetry"), message: IMStrings.text("exportPendingHint")) { [weak self] in
                        self?.check(id, retry: true)
                    }
                }))
            }
            let terminal = ["cancelled", "expired", "failed"].contains(status)
            if original["jobId"] != .null, !terminal || job["cleanupState"] != .string("cleared") {
                result.append(Row(title: IMStrings.text(original["cancelBody"] == .null ? "exportCancel" : "exportRetryCancel"), detail: "", action: { [weak self] in
                    guard let self else {
                        return
                    }
                    IMUI.confirm(self, title: IMStrings.text("exportCancel"), message: IMStrings.text("exportCancelConfirm")) { [weak self] in
                        self?.run { [weak self] in
                            guard let self else {
                                return
                            }
                            _ = try await self.exports.cancel(id)
                            try await self.fetch()
                        }
                    }
                }))
            }
            if enabled, status == "ready", job["cleanupState"] == .string("published"),
                original["cancelBody"] == .null, job["restoreEpoch"].text == exports.namespace.restoreEpoch,
                let deadline = IMSyncProtocol.date(job["expiresAt"].text), deadline > Date() {
                result.append(Row(title: IMStrings.text("exportDownload"), detail: IMStrings.text("exportDownloadHint"), action: { [weak self] in
                    self?.save(id)
                }))
            }
            return result
        }
        if originals.count > shown {
            rows.append([Row(title: IMStrings.text("loadMore"), detail: "", action: { [weak self] in
                self?.shown += 20
                self?.reload()
            })])
        }
        table.byReloadData()
            .byReloadEmptyViewAuto()
    }

    private func check(_ id: String, retry: Bool = false) {
        run { [weak self] in
            guard let self else {
                return
            }
            _ = try await self.exports.check(id, retry: retry)
            try await self.fetch()
        }
    }

    private func save(_ id: String) {
        run { [weak self] in
            guard let self else {
                return
            }
            self.discardTemporary()
            let file = try await self.exports.download(id)
            self.exportFile = file
            do {
                try self.exports.requireCurrent()
            } catch {
                self.discardTemporary()
                throw error
            }
            let picker = UIDocumentPickerViewController.makeExporting(file, delegate: self)
            self.presentingSave = true
            self.present(picker, animated: true)
        }
    }

    private func discardTemporary() {
        if let exportFile {
            try? FileManager.default.removeItem(at: exportFile.deletingLastPathComponent())
        }
        exportFile = nil
    }

    func documentPicker(_ controller: UIDocumentPickerViewController, didPickDocumentsAt urls: [URL]) {
        presentingSave = false
        discardTemporary()
    }

    func documentPickerWasCancelled(_ controller: UIDocumentPickerViewController) {
        presentingSave = false
        discardTemporary()
    }

    private func time(_ raw: String) -> String {
        guard let date = IMSyncProtocol.date(raw) else {
            return ""
        }
        return DateFormatter.localizedString(from: date, dateStyle: .medium, timeStyle: .short)
    }

    func numberOfSections(in tableView: UITableView) -> Int {
        return rows.count
    }

    func tableView(_ tableView: UITableView, numberOfRowsInSection section: Int) -> Int {
        return rows[section].count
    }

    func tableView(_ tableView: UITableView, cellForRowAt indexPath: IndexPath) -> UITableViewCell {
        let row = rows[indexPath.section][indexPath.row]
        return UITableViewCell.make(style: .subtitle, reuseIdentifier: nil)
            .byText(row.title)
            .bySecondaryText(row.detail)
            .byTitleNumberOfLines(0)
            .byDetailTitleNumberOfLines(0)
            .byTitleCor(IMUI.foreground)
            .byDetailTitleCor(IMUI.secondary)
            .bySelectionStyle(row.action == nil || busy ? .none : .default)
            .byIMThemeAccessoryType(row.action == nil ? .none : .disclosureIndicator)
            .byAccessibilityLabel(row.title + " " + row.detail)
    }

    func tableView(_ tableView: UITableView, didSelectRowAt indexPath: IndexPath) {
        tableView.deselectRow(at: indexPath, animated: true)
        guard !busy else {
            return
        }
        rows[indexPath.section][indexPath.row].action?()
    }
}
