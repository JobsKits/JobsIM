//
//  IMMigrationVC.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月17日，星期二.
//

import JobsByUIKit
import JobsIMCore
import JobsSwiftDSL
import SnapKit
import UIKit

@MainActor
final class IMMigrationVC: IMBaseVC,
    UITableViewDataSource,
    UITableViewDelegate,
    UIDocumentPickerDelegate
{

    private typealias Row = (title: String, detail: String, action: (() -> Void)?)
    private var mounted = false
    private var lifecycle = UUID()
    private var work: Task<Void, Never>?
    private var service: IMMigrationService?
    private var available = false
    private var fullOriginals = false
    private var selected = Set<String>()
    private var candidates: [IMJSON] = []
    private var originals: [IMJSON] = []
    private var archives: [IMJSON] = []
    private var archiveMessages: [IMJSON] = []
    private var archiveID: String?
    private var pairing: IMJSON?
    private var target: IMJSON?
    private var retryOriginal: IMJSON?
    private var queuedFile: Data?
    private var queuedName = ""
    private var shown = 50
    private var originalsShown = 20
    private var archivesShown = 20
    private var messagesShown = 50
    private var rows: [[Row]] = []
    private var sections: [String] = []
    private var temporaryFile: URL?
    private var selectingFile = false
    private var fileOwner: (IMNamespace, String)?
    private var projectionRevision = UUID()
    private var projectionReloadPending = false

    private lazy var summary = IMUI.label(IMStrings.text("migrationHelp"), style: .footnote, color: IMUI.secondary)
    private lazy var secretHelp = IMUI.label(
        IMStrings.text("migrationSecrets"), style: .footnote, color: IMUI.secondary)
    private lazy var password = IMUI.field(IMStrings.text("currentPassword"), secure: true)
        .byTextContentType(.password)
        .byAccessibilityIdentifier("migration.password")
    private lazy var otp = IMUI.field(IMStrings.text("authOTP"), secure: true)
        .byTextContentType(.oneTimeCode)
        .byKeyboardType(.numberPad)
    private lazy var pairingCode = IMUI.field(IMStrings.text("migrationPairCode"), secure: true)
        .byTextContentType(.oneTimeCode)
        .byAccessibilityIdentifier("migration.pairing")
    private lazy var migrationID = IMUI.field(IMStrings.text("migrationIdentifier"))
        .byAccessibilityIdentifier("migration.id")
    private lazy var notice = IMUI.label("", style: .footnote, color: IMUI.secondary)
    private lazy var header = IMUI.stack(spacing: 10)
    private lazy var reloadButton = IMUI.button(IMStrings.text("reload")) { [weak self] in
        self?.reload()
    }
    private lazy var table = UITableView.make(style: .insetGrouped)
        .byDelegate(self)
        .byDataSource(self)
        .byRowHeight(UITableView.automaticDimension)
        .byEstimatedRowHeight(70)
        .byBackgroundColor(IMUI.background)

    override func viewDidLoad() {
        screenTitle = IMStrings.text("personalMigration")
        super.viewDidLoad()
        header.byAddArrangedSubviews([
            summary, secretHelp, password, otp, pairingCode, migrationID, notice, reloadButton,
        ])
        .byAddTo(content) { make in
            make.top.left.right.equalToSuperview().inset(16)
        }
        for field in [password, otp, pairingCode, migrationID] {
            field.snp.makeConstraints { make in
                make.height.greaterThanOrEqualTo(46)
            }
        }
        table.byAddTo(content) { [unowned self] make in
            make.top.equalTo(header.snp.bottom).offset(8)
            make.left.right.bottom.equalToSuperview()
        }.byEmptyButtonProvider { [weak self] in
            return IMUI.emptyButton(title: IMStrings.text("migrationEmpty")) {
                self?.reload()
            }
        }
    }

    override func viewWillAppear(_ animated: Bool) {
        super.viewWillAppear(animated)
        mounted = true
        establishService()
        NotificationCenter.default.addObserver(
            self, selector: #selector(clearSensitive),
            name: UIApplication.didEnterBackgroundNotification, object: nil)
        NotificationCenter.default.addObserver(
            self, selector: #selector(resumeContext),
            name: UIApplication.didBecomeActiveNotification, object: nil)
        reload()
    }

    private func establishService() {
        lifecycle = UUID()
        projectionRevision = UUID()
        projectionReloadPending = false
        let owner = lifecycle
        service = try? IMMigrationService(isCurrent: { [weak self] in
            return self?.mounted == true && self?.lifecycle == owner
        })
        available = false
        candidates = []
        originals = []
        archives = []
        archiveMessages = []
        archiveID = nil
        selected = []
        retryOriginal = nil
        target = nil
        buildRows()
    }

    @objc private func resumeContext() {
        guard mounted else {
            return
        }
        establishService()
        reload()
    }

    override func viewDidDisappear(_ animated: Bool) {
        super.viewDidDisappear(animated)
        mounted = false
        lifecycle = UUID()
        clearSensitive()
        service = nil
        NotificationCenter.default.removeObserver(self, name: UIApplication.didEnterBackgroundNotification, object: nil)
        NotificationCenter.default.removeObserver(self, name: UIApplication.didBecomeActiveNotification, object: nil)
    }

    @objc private func clearSensitive() {
        let running = work
        work = nil
        lifecycle = UUID()
        running?.cancel()
        service = nil
        password.byText("")
        otp.byText("")
        pairingCode.byText("")
        pairing = nil
        clearProjectionRAM()
        if !selectingFile {
            queuedFile = nil
            queuedName = ""
        }
    }

    override func syncProjectionChanged() {
        guard mounted else {
            return
        }
        projectionRevision = UUID()
        clearProjectionRAM()
        buildRows()
        guard service != nil else {
            return
        }
        if work != nil {
            projectionReloadPending = true
            return
        }
        reload()
    }

    private func clearProjectionRAM() {
        candidates = []
        selected = []
        archiveMessages = []
    }

    private func run(_ action: @escaping @MainActor (IMMigrationService) async throws -> Void) {
        guard mounted, work == nil, let service else {
            notice.byText(IMStrings.text("migrationUnavailable"))
            return
        }
        let owner = lifecycle
        let projection = projectionRevision
        work = Task { [weak self] in
            guard let self else {
                return
            }
            defer {
                if self.lifecycle == owner {
                    self.work = nil
                    if self.projectionReloadPending {
                        self.projectionReloadPending = false
                        self.reload()
                    } else {
                        self.buildRows()
                    }
                }
            }
            do {
                try service.requireCurrent()
                try await action(service)
                try service.requireCurrent()
                guard self.projectionRevision == projection else {
                    self.clearProjectionRAM()
                    return
                }
                self.originals = try service.rows()
                self.archives = try service.archives()
            } catch {
                if self.mounted, self.lifecycle == owner, !Task.isCancelled {
                    if self.projectionRevision != projection {
                        self.clearProjectionRAM()
                    }
                    self.notice.byText(IMStrings.text("migrationUnknown"))
                    self.originals = (try? service.rows()) ?? self.originals
                    self.archives = (try? service.archives()) ?? self.archives
                }
            }
        }
        buildRows()
    }

    private func reload() {
        run { [weak self] service in
            guard let self else {
                return
            }
            self.candidates = try service.candidates()
            self.originals = try service.rows()
            self.archives = try service.archives()
            self.available = (try? await service.capability()) == true
            try service.requireCurrent()
            if let archive = self.archiveID {
                self.archiveMessages = try await service.messages(archive)
            }
            self.notice.byText(self.available ? "" : IMStrings.text("migrationUnavailable"))
        }
    }

    private func proof() throws -> (String, String, String) {
        let currentPassword = password.text ?? ""
        let currentOTP = otp.text ?? ""
        let code = (pairingCode.text ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        password.byText("")
        otp.byText("")
        pairingCode.byText("")
        view.endEditing(true)
        _ = try IMMigrationService.credential(password: currentPassword, otp: currentOTP)
        guard
            session.user["admin"] != .bool(true)
                || currentOTP.range(of: "^[0-9]{6}$", options: .regularExpression) != nil
        else {
            try IMMigrationProtocol.refuse("MIGRATION_PROOF_INVALID")
        }
        return (currentPassword, currentOTP, code)
    }

    private func authorize(_ mode: String) {
        guard work == nil, available else {
            return
        }
        let owner = lifecycle
        IMUI.confirm(
            self, title: IMStrings.text("personalMigration"),
            message: IMStrings.text(mode == "pair" ? "migrationPairConsent" : "migrationConsent")
        ) { [weak self] in
            guard let self, self.mounted, self.lifecycle == owner else {
                return
            }
            do {
                let (password, otp, code) = try self.proof()
                let original = self.retryOriginal
                if mode == "source", code.isEmpty {
                    try IMMigrationProtocol.refuse()
                }
                self.run { [weak self] service in
                    guard let self else {
                        return
                    }
                    if mode == "pair" {
                        guard original == nil || original?["kind"].text == "pairing" else {
                            try IMMigrationProtocol.refuse()
                        }
                        self.pairing = try await service.pair(
                            password: password, otp: otp, originalID: original?["id"].text)
                        self.notice.byText(IMStrings.text("migrationReady"))
                    } else if mode == "source" {
                        let row: IMJSON
                        if let original {
                            guard original["kind"].text == "create" else {
                                try IMMigrationProtocol.refuse()
                            }
                            row = original
                        } else {
                            row = try service.prepareSource(ids: self.selected, originals: self.fullOriginals)
                        }
                        _ = try await service.create(row["id"].text, pairingProof: code, password: password, otp: otp)
                        self.notice.byText(IMStrings.text("migrationReady"))
                    } else {
                        guard let target = self.target, target["status"].text == "ready" else {
                            try IMMigrationProtocol.refuse()
                        }
                        let file = self.queuedFile
                        self.queuedFile = nil
                        self.queuedName = ""
                        let reported = try await service.install(
                            target["id"].text, password: password, otp: otp, file: file)
                        self.notice.byText(IMStrings.text(reported ? "migrationInstalled" : "migrationReportUnknown"))
                    }
                    self.retryOriginal = nil
                }
            } catch {
                self.notice.byText(IMStrings.text("migrationInvalid"))
            }
        }
    }

    private func copyPairing() {
        do {
            try service?.requireCurrent()
            guard let pairing, let expiry = IMSyncProtocol.date(pairing["expiresAt"].text), expiry > Date() else {
                try IMMigrationProtocol.refuse("MIGRATION_PAIRING_EXPIRED")
            }
            UIPasteboard.general.string = pairing["pairingProof"].text
            notice.byText(IMStrings.text("copied"))
        } catch {
            notice.byText(IMStrings.text("migrationExpired"))
        }
    }

    private func queryTarget() {
        let identifier = (migrationID.text ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
        run { [weak self] service in
            let value = try await service.target(identifier)
            self?.target = value
            self?.notice.byText(
                IMStrings.text(value["status"].text == "installed_reported" ? "migrationReported" : "migrationReady"))
        }
    }

    private func emptyRow() -> Row {
        return (
            IMStrings.text("migrationEmpty"), IMStrings.text("reload"),
            { [weak self] in
                self?.reload()
            }
        )
    }

    private func stateTitle(_ value: String) -> String {
        let names = ["unsent": "migrationStaging", "unknown": "migrationUnknown", "ready": "migrationReady",
            "installed": "migrationInstalled", "blocked": "migrationSourceChanged", "building": "migrationBuilding",
            "installed_reported": "migrationReported", "revoking": "migrationRevoking", "retired": "migrationRetired"]
        return IMStrings.text(names[value] ?? "migrationUnknown")
    }

    private func buildRows() {
        guard isViewLoaded else {
            return
        }
        let busy = work != nil
        reloadButton.byEnabled(!busy)
        var targetRows: [Row] = [
            (
                IMStrings.text("migrationPair"), IMStrings.text("migrationPairConsent"),
                available && !busy
                    ? { [weak self] in
                        self?.authorize("pair")
                    } : nil
            )
        ]
        if let pairing {
            targetRows.append(
                (
                    IMStrings.text("migrationCopyPair"), pairing["expiresAt"].text,
                    !busy
                        ? { [weak self] in
                            self?.copyPairing()
                        } : nil
                ))
        }
        var sourceRows: [Row] = [
            (
                IMStrings.text(fullOriginals ? "migrationMetadataOnly" : "migrationIncludeOriginal"),
                IMStrings.text(fullOriginals ? "migrationIncludeOriginal" : "migrationMetadataOnly"),
                !busy && retryOriginal == nil
                    ? { [weak self] in
                        self?.fullOriginals.toggle()
                        self?.buildRows()
                    } : nil
            ),
            (
                IMStrings.text("migrationCreate"),
                IMStrings.text("migrationSelected")
                    + " \(retryOriginal?["selection"].list.count ?? selected.count)/256",
                available && !busy
                    ? { [weak self] in
                        self?.authorize("source")
                    } : nil
            ),
        ]
        for message in candidates.prefix(shown) {
            let identifier = message["id"].text
            let text = message["text"].text.isEmpty ? IMStrings.text(message["type"].text) : message["text"].text
            sourceRows.append(
                (
                    (selected.contains(identifier) ? "✓ " : "") + String(text.prefix(120)), message["createdAt"].text,
                    !busy && retryOriginal == nil
                        ? { [weak self] in
                            guard let self else {
                                return
                            }
                            if self.selected.contains(identifier) {
                                self.selected.remove(identifier)
                            } else if self.selected.count < 256 {
                                self.selected.insert(identifier)
                            }
                            self.buildRows()
                        } : nil
                ))
        }
        if candidates.isEmpty {
            sourceRows.append(emptyRow())
        } else if shown < candidates.count {
            sourceRows.append(
                (
                    IMStrings.text("loadMore"), "",
                    { [weak self] in
                        self?.shown += 50
                        self?.buildRows()
                    }
                ))
        }
        var fileRows: [Row] = [
            (
                IMStrings.text("migrationCheck"), migrationID.text ?? "",
                !busy
                    ? { [weak self] in
                        self?.queryTarget()
                    } : nil
            ),
            (
                IMStrings.text("migrationChooseFile"), queuedName,
                !busy
                    ? { [weak self] in
                        self?.chooseFile()
                    } : nil
            ),
        ]
        if queuedFile != nil {
            fileRows.append(
                (
                    IMStrings.text("remove"), queuedName,
                    { [weak self] in
                        self?.queuedFile = nil
                        self?.queuedName = ""
                        self?.buildRows()
                    }
                ))
        }
        if let target {
            fileRows.append(
                (
                    IMStrings.text("migrationIdentifier"), target["id"].text,
                    { [weak self] in
                        UIPasteboard.general.string = target["id"].text
                        self?.notice.byText(IMStrings.text("copied"))
                    }
                ))
            if target["status"].text == "ready" {
                fileRows.append(
                    (
                        IMStrings.text("migrationDownload"), target["expiresAt"].text,
                        !busy
                            ? { [weak self] in
                                self?.downloadCapsule()
                            } : nil
                    ))
                fileRows.append(
                    (
                        IMStrings.text("migrationInstall"), IMStrings.text("migrationConsent"),
                        available && !busy
                            ? { [weak self] in
                                self?.authorize("install")
                            } : nil
                    ))
            } else {
                fileRows.append((stateTitle(target["status"].text), "", nil))
            }
        }
        var originalRows = originals.prefix(originalsShown).map { row -> Row in
            return (
                IMStrings.text(
                    row["kind"].text == "pairing"
                        ? "migrationPair" : row["kind"].text == "target" ? "migrationTarget" : "migrationSource"),
                stateTitle(row["state"].text) + " · " + row["createdAt"].text,
                !busy
                    ? { [weak self] in
                        self?.originalMenu(row)
                    } : nil
            )
        }
        if originals.isEmpty {
            originalRows.append(emptyRow())
        } else if originalsShown < originals.count {
            originalRows.append(
                (
                    IMStrings.text("loadMore"), "",
                    { [weak self] in
                        self?.originalsShown += 20
                        self?.buildRows()
                    }
                ))
        }
        var archiveRows = archives.prefix(archivesShown).map { row -> Row in
            return (
                IMStrings.text("migrationHistory") + " · " + row["messageCount"].text, row["createdAt"].text,
                !busy
                    ? { [weak self] in
                        self?.openArchive(row["id"].text)
                    } : nil
            )
        }
        if archives.isEmpty {
            archiveRows.append(emptyRow())
        } else if archivesShown < archives.count {
            archiveRows.append(
                (
                    IMStrings.text("loadMore"), "",
                    { [weak self] in
                        self?.archivesShown += 20
                        self?.buildRows()
                    }
                ))
        }
        rows = [targetRows, sourceRows, fileRows, originalRows, archiveRows]
        sections = ["migrationTarget", "migrationSource", "migrationCapsule", "migrationOriginals", "migrationHistory"]
        if archiveID != nil {
            var contents = archiveMessages.prefix(messagesShown).map { message -> Row in
                let text =
                    message["contentUnavailable"] == .bool(true)
                    ? IMStrings.text("contentUnavailable") : message["text"].text
                return (
                    String((text.isEmpty ? IMStrings.text(message["type"].text) : text).prefix(160)),
                    message["createdAt"].text,
                    !busy
                        ? { [weak self] in
                            self?.openMessage(message)
                        } : nil
                )
            }
            if archiveMessages.isEmpty {
                contents.append(emptyRow())
            } else if messagesShown < archiveMessages.count {
                contents.append(
                    (
                        IMStrings.text("loadMore"), "",
                        { [weak self] in
                            self?.messagesShown += 50
                            self?.buildRows()
                        }
                    ))
            }
            rows.append(contents)
            sections.append("migrationHistory")
        }
        if view.window != nil {
            table.reloadData()
            table.byReloadEmptyViewAuto()
        }
    }

    private func originalMenu(_ row: IMJSON) {
        let sheet = UIAlertController.make(
            title: IMStrings.text("migrationOriginals"), message: row["id"].text, style: .actionSheet)
        sheet.addAction(
            UIAlertAction.make(IMStrings.text("migrationCheck")) { [weak self] in
                self?.run { [weak self] service in
                    let value = try await service.check(row["id"].text)
                    if value?["pairingProof"] != .null, value != nil {
                        self?.pairing = value
                    }
                    self?.notice.byText(IMStrings.text(value == nil ? "migrationUnknown" : "migrationReady"))
                }
            })
        if ["unsent", "unknown"].contains(row["state"].text), row["kind"].text != "target",
            service?.mayWrite(row) == true
        {
            sheet.addAction(
                UIAlertAction.make(IMStrings.text("migrationRetry")) { [weak self] in
                    self?.retryOriginal = row
                    self?.notice.byText(IMStrings.text("migrationRetryHelp"))
                    self?.buildRows()
                })
        }
        if row["migrationId"] != .null {
            sheet.addAction(
                UIAlertAction.make(IMStrings.text("migrationCopyID")) {
                    UIPasteboard.general.string = row["migrationId"].text
                })
            if service?.mayWrite(row) == true {
                sheet.addAction(
                    UIAlertAction.make(IMStrings.text("cancel"), style: .destructive) { [weak self] in
                        self?.run { service in
                            _ = try await service.cancel(row["migrationId"].text, target: row["kind"].text == "target")
                        }
                    })
            }
        }
        if retryOriginal != nil {
            sheet.addAction(
                UIAlertAction.make(IMStrings.text("migrationNewSelection")) { [weak self] in
                    self?.retryOriginal = nil
                    self?.buildRows()
                })
        }
        sheet.addAction(UIAlertAction.make(IMStrings.text("done"), style: .cancel) {})
        sheet.popoverPresentationController?.sourceView = view
        present(sheet, animated: true)
    }

    private func openArchive(_ id: String) {
        run { [weak self] service in
            let values = try await service.messages(id)
            self?.archiveID = id
            self?.archiveMessages = values
            self?.messagesShown = 50
        }
    }

    private func openMessage(_ message: IMJSON) {
        guard message["contentUnavailable"] != .bool(true), message["deleted"] != .bool(true) else {
            IMUI.alert(self, message: IMStrings.text("contentUnavailable"))
            return
        }
        if message["objectMode"].text == "full", let archiveID {
            run { [weak self] service in
                guard let self else {
                    return
                }
                let data = try service.object(message["attachmentId"].text, archive: archiveID)
                try self.share(data, suffix: ".bin")
            }
        } else {
            IMUI.alert(
                self, title: IMStrings.text("migrationHistory"),
                message: message["text"].text + "\n" + IMStrings.text("migrationMetadataOnly"))
        }
    }

    private func downloadCapsule() {
        guard let target else {
            return
        }
        run { [weak self] service in
            let data = try await service.capsule(target["id"].text)
            try self?.share(data, suffix: ".jimmig")
        }
    }

    private func share(_ data: Data, suffix: String) throws {
        let file = FileManager.default.temporaryDirectory.appendingPathComponent(
            "JobsIM-migration-" + UUID().uuidString + suffix)
        try data.write(to: file, options: [.atomic, .completeFileProtection])
        temporaryFile = file
        let controller = UIActivityViewController.make([file])
        controller.completionWithItemsHandler = { [weak self] _, _, _, _ in
            try? FileManager.default.removeItem(at: file)
            if self?.temporaryFile == file {
                self?.temporaryFile = nil
            }
        }
        controller.popoverPresentationController?.sourceView = view
        present(controller, animated: true)
    }

    private func chooseFile() {
        guard work == nil, let service else {
            return
        }
        fileOwner = (service.namespace, service.family)
        selectingFile = true
        let picker = UIDocumentPickerViewController.makeImporting(delegate: self)
        present(picker, animated: true)
    }

    func documentPicker(_ controller: UIDocumentPickerViewController, didPickDocumentsAt urls: [URL]) {
        selectingFile = false
        guard let owner = fileOwner, owner.0 == session.namespace,
            owner.1 == session.currentCredentialPair?.familyID, let file = urls.first
        else {
            queuedFile = nil
            queuedName = ""
            fileOwner = nil
            buildRows()
            return
        }
        let access = file.startAccessingSecurityScopedResource()
        defer {
            if access {
                file.stopAccessingSecurityScopedResource()
            }
            fileOwner = nil
        }
        do {
            let info = try file.resourceValues(forKeys: [.isRegularFileKey, .isSymbolicLinkKey, .fileSizeKey])
            guard info.isRegularFile == true, info.isSymbolicLink != true,
                let size = info.fileSize, size >= 36, size <= IMMigrationProtocol.limit + 4096
            else {
                try IMMigrationProtocol.refuse("MIGRATION_CAPSULE_INVALID")
            }
            queuedFile = try Data(contentsOf: file)
            queuedName = file.lastPathComponent
            notice.byText(IMStrings.text("migrationFileQueued"))
        } catch {
            queuedFile = nil
            queuedName = ""
            notice.byText(IMStrings.text("migrationInvalid"))
        }
        buildRows()
    }

    func documentPickerWasCancelled(_ controller: UIDocumentPickerViewController) {
        selectingFile = false
        fileOwner = nil
    }

    func numberOfSections(in tableView: UITableView) -> Int {
        return rows.count
    }

    func tableView(_ tableView: UITableView, numberOfRowsInSection section: Int) -> Int {
        return rows[section].count
    }

    func tableView(_ tableView: UITableView, titleForHeaderInSection section: Int) -> String? {
        return IMStrings.text(sections[section])
    }

    func tableView(_ tableView: UITableView, cellForRowAt indexPath: IndexPath) -> UITableViewCell {
        let row = rows[indexPath.section][indexPath.row]
        return UITableViewCell.make(style: .subtitle, reuseIdentifier: nil)
            .byText(row.title)
            .bySecondaryText(row.detail)
            .byIMThemeAccessoryType(row.action == nil ? .none : .disclosureIndicator)
    }

    func tableView(_ tableView: UITableView, didSelectRowAt indexPath: IndexPath) {
        tableView.deselectRow(at: indexPath, animated: true)
        rows[indexPath.section][indexPath.row].action?()
    }
}
