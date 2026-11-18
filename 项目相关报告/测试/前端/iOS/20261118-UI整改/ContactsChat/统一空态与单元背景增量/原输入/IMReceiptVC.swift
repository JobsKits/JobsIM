//
//  IMReceiptVC.swift
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
final class IMReceiptVC: IMBaseVC, UITableViewDataSource, UITableViewDelegate {

    private typealias Row = (title: String, detail: String, action: (() -> Void)?)
    private var service: IMReceiptService?
    private var mounted = false
    private var lifecycle = UUID()
    private var projection = UUID()
    private var reloadPending = false
    private var automaticObservation: UUID?
    private var work: Task<Void, Never>?
    private var context: IMJSON?
    private var declarations: [IMJSON] = []
    private var intents: [IMJSON] = []
    private var candidates: [IMJSON] = []
    private var shown = 30
    private var originalsShown = 20
    private var rows: [[Row]] = []
    private let titles = ["receiptTitle", "receiptVerifyReport", "receiptOriginals", "receiptContent"]
    private lazy var help = IMUI.label(IMStrings.text("receiptHelp"), style: .footnote, color: IMUI.secondary)
    private lazy var notice = IMUI.label("", style: .footnote, color: IMUI.secondary)
    private lazy var reloadButton = IMUI.button(IMStrings.text("reload")) { [weak self] in
        self?.reload()
    }
    private lazy var header = IMUI.stack(spacing: 8)
    private lazy var table = UITableView.make(style: .insetGrouped)
        .byDelegate(self)
        .byDataSource(self)
        .byRowHeight(UITableView.automaticDimension)
        .byEstimatedRowHeight(72)
        .byBackgroundColor(IMUI.background)

    override func viewDidLoad() {
        screenTitle = IMStrings.text("receiptTitle")
        super.viewDidLoad()
        header.byAddArrangedSubviews([help, notice, reloadButton]).byAddTo(content) { make in
            make.top.left.right.equalToSuperview().inset(16)
        }
        table.byAddTo(content) { [unowned self] make in
            make.top.equalTo(header.snp.bottom).offset(8)
            make.left.right.bottom.equalToSuperview()
        }.byEmptyButtonProvider { [weak self] in
            return IMUI.button(IMStrings.text("receiptEmpty") + "\n" + IMStrings.text("reload")) {
                self?.reload()
            }
        }
    }

    override func viewWillAppear(_ animated: Bool) {
        super.viewWillAppear(animated)
        mounted = true
        if let automaticObservation {
            IMReceiptCoordinator.shared.removeObserver(automaticObservation)
        }
        automaticObservation = IMReceiptCoordinator.shared.observe { [weak self] in
            self?.buildRows()
        }
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
        projection = UUID()
        reloadPending = false
        let owner = lifecycle
        service = try? IMReceiptService(isCurrent: { [weak self] in
            return self?.mounted == true && self?.lifecycle == owner
        })
        context = nil
        declarations = []
        intents = []
        candidates = []
        buildRows()
    }

    @objc private func clearSensitive() {
        let running = work
        work = nil
        lifecycle = UUID()
        running?.cancel()
        service = nil
        context = nil
        candidates = []
        declarations = []
        intents = []
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
        if let automaticObservation {
            IMReceiptCoordinator.shared.removeObserver(automaticObservation)
        }
        automaticObservation = nil
        clearSensitive()
        NotificationCenter.default.removeObserver(self, name: UIApplication.didEnterBackgroundNotification, object: nil)
        NotificationCenter.default.removeObserver(self, name: UIApplication.didBecomeActiveNotification, object: nil)
    }

    override func syncProjectionChanged() {
        guard mounted else {
            return
        }
        projection = UUID()
        candidates = []
        buildRows()
        if work != nil {
            reloadPending = true
        } else {
            reload()
        }
    }

    private func run(_ action: @escaping @MainActor (IMReceiptService) async throws -> Void) {
        guard mounted, work == nil, let service else {
            notice.byText(IMStrings.text("receiptUnverified"))
            return
        }
        let owner = lifecycle
        let observed = projection
        work = Task { [weak self] in
            guard let self else {
                return
            }
            defer {
                if self.lifecycle == owner {
                    self.work = nil
                    if self.reloadPending {
                        self.reloadPending = false
                        self.reload()
                    } else {
                        self.buildRows()
                    }
                }
            }
            do {
                _ = try service.stateNow()
                try await action(service)
                _ = try service.stateNow()
                self.declarations = try service.declarations()
                self.intents = try service.intents()
                if self.projection == observed {
                    self.candidates = (try? service.candidates()) ?? []
                }
            } catch {
                if self.mounted, self.lifecycle == owner, !Task.isCancelled {
                    self.notice.byText(IMStrings.text("receiptUnknown"))
                    self.declarations = (try? service.declarations()) ?? []
                    self.intents = (try? service.intents()) ?? []
                    if self.projection != observed {
                        self.candidates = []
                    }
                }
            }
        }
        buildRows()
    }

    private func reload() {
        run { [weak self] service in
            self?.context = try await service.context()
            _ = try await service.refreshProfiles()
        }
    }

    private func confirm(_ key: String, action: @escaping @MainActor () -> Void) {
        let message = IMStrings.text("receiptConfirm")
            + (key == "receiptAutoContent" ? "\n\n" + IMStrings.text("receiptAutoHelp") : "")
        let sheet = UIAlertController.make(title: IMStrings.text(key), message: message, style: .alert)
        sheet.addAction(UIAlertAction.make(IMStrings.text("cancel"), style: .cancel) {})
        sheet.addAction(
            UIAlertAction.make(IMStrings.text("done")) {
                action()
            })
        present(sheet, animated: true)
    }

    private func verify(objects: Bool) {
        confirm(objects ? "receiptVerifyObjects" : "receiptVerifyContent") { [weak self] in
            self?.run { [weak self] service in
                _ = try await service.verify(includeObjects: objects)
                self?.notice.byText(
                    IMStrings.text(service.approvedProfiles.isEmpty ? "receiptNoProfile" : "receiptFound"))
                self?.context = try await service.context()
            }
        }
    }

    private func empty() -> Row {
        return (
            IMStrings.text("receiptEmpty"), IMStrings.text("reload"),
            { [weak self] in
                self?.reload()
            }
        )
    }

    private func stateTitle(_ row: IMJSON) -> String {
        return IMStrings.text(
            row["state"].text == "completed"
                ? "receiptFound"
                : row["state"].text == "frozen"
                    ? "migrationSourceChanged"
                    : row["state"].text == "prepared" ? "migrationStaging" : "receiptUnknown")
    }

    private func automaticTitle() -> String {
        guard IMReceiptCoordinator.shared.isEnabled(for: session) else {
            return IMStrings.text("receiptAutoOff")
        }
        switch IMReceiptCoordinator.shared.state {
        case .off:
            return IMStrings.text("receiptAutoOff")
        case .verifying:
            return IMStrings.text("receiptAutoVerifying")
        case .running:
            return IMStrings.text("receiptAutoRunning")
        case .paused:
            return IMStrings.text("receiptAutoPaused")
        case .blocked:
            return IMStrings.text("receiptUnverified")
        case .unknown:
            return IMStrings.text("receiptUnknown") + ": " + String(IMReceiptCoordinator.shared.unknownCount)
        }
    }

    private func buildRows() {
        guard isViewLoaded else {
            return
        }
        let busy = work != nil
        let eligible = context.map { service?.locallyQualified($0) == true } ?? false
        let objects = context.map { service?.locallyQualified($0, objects: true) == true } ?? false
        let automaticQualified = IMReceiptCoordinator.shared.isQualified(for: session)
        reloadButton.byEnabled(!busy)
        var status: [Row] = [
            (IMStrings.text("receiptTitle"),
                IMStrings.text(eligible || automaticQualified ? "receiptYes" : "receiptUnverified")
                    + "\n" + IMStrings.text("receiptNoUpgrade"), nil)
        ]
        status.append((IMStrings.text("receiptObject"), IMStrings.text(objects ? "receiptYes" : "receiptNo"), nil))
        if let context {
            status.append(
                (
                    IMStrings.text("receiptServer"),
                    IMStrings.text(context["storage"]["qualified"] == .bool(true) ? "receiptYes" : "receiptNo"), nil
                ))
            status.append(
                (
                    IMStrings.text(context["readiness"]["ready"] == .bool(true) ? "receiptReady" : "receiptBlocked"),
                    "", nil
                ))
            for key in IMReceiptProtocol.protection {
                let title = "receipt" + key.prefix(1).uppercased() + key.dropFirst()
                status.append(
                    (
                        IMStrings.text(title),
                        IMStrings.text(context["readiness"][key] == .bool(true) ? "receiptYes" : "receiptNo"), nil
                    ))
            }
        } else {
            status.append(empty())
        }
        var verification: [Row] = [
            (
                IMStrings.text("receiptVerifyContent"), "",
                busy || service == nil
                    ? nil
                    : { [weak self] in
                        self?.verify(objects: false)
                    }
            ),
            (
                IMStrings.text("receiptVerifyObjects"), "",
                busy || service == nil
                    ? nil
                    : { [weak self] in
                        self?.verify(objects: true)
                    }
            ),
        ]
        let automatic = IMReceiptCoordinator.shared.isEnabled(for: session)
        verification.insert((IMStrings.text(automatic ? "receiptStopAuto" : "receiptAutoContent"),
            automaticTitle() + "\n" + IMStrings.text("receiptAutoHelp"), busy ? nil : { [weak self] in
                guard let self else {
                    return
                }
                if automatic {
                    IMReceiptCoordinator.shared.disable()
                } else {
                    self.confirm("receiptAutoContent") { [weak self] in
                        self?.run { _ in
                            try await IMReceiptCoordinator.shared.enable()
                        }
                    }
                }
            }), at: 0)
        if let report = service?.verification {
            verification.append(
                (
                    IMStrings.text("receiptVerifyReport"), report["verifiedAt"].text,
                    busy
                        ? nil
                        : { [weak self] in
                            guard self?.service?.verification == report else {
                                return
                            }
                            let bytes = try? JSONEncoder.make().encode(report)
                            UIPasteboard.general.string = bytes.map { String(decoding: $0, as: UTF8.self) }
                            self?.notice.byText(IMStrings.text("copied"))
                        }
                ))
            for profile in service?.approvedProfiles ?? [] {
                verification.append(
                    (
                        IMStrings.text("receiptDeclare"), profile["id"].text,
                        busy
                            ? nil
                            : { [weak self] in
                                self?.confirm("receiptDeclare") { [weak self] in
                                    self?.run { [weak self] service in
                                        _ = try await service.declare(profile["id"].text)
                                        self?.context = try await service.context()
                                    }
                                }
                            }
                    ))
            }
            if service?.approvedProfiles.isEmpty == true {
                verification.append((IMStrings.text("receiptNoProfile"), "", nil))
                verification.append(empty())
            }
        }
        let all = declarations.map { ($0, true) } + intents.map { ($0, false) }
        var originalRows = all.prefix(originalsShown).map { row, declaration -> Row in
            return (
                IMStrings.text(
                    declaration ? "receiptDeclare" : row["kind"].text == "object" ? "receiptObject" : "receiptContent"),
                stateTitle(row) + " · " + row["createdAt"].text,
                busy
                    ? nil
                    : { [weak self] in
                        self?.originalMenu(row, declaration: declaration, objects: objects)
                    }
            )
        }
        if all.isEmpty {
            originalRows.append(empty())
        } else if originalsShown < all.count {
            originalRows.append(
                (
                    IMStrings.text("loadMore"), "",
                    { [weak self] in
                        self?.originalsShown += 20
                        self?.buildRows()
                    }
                ))
        }
        var messages = candidates.prefix(shown).map { message -> Row in
            let title = message["text"].text.isEmpty ? IMStrings.text(message["type"].text) : message["text"].text
            return (
                String(title.prefix(140)), message["createdAt"].text,
                !busy && eligible
                    ? { [weak self] in
                        self?.confirm("receiptContent") { [weak self] in
                            self?.run { service in
                                _ = try await service.receiveContent(message["id"].text)
                            }
                        }
                    } : nil
            )
        }
        if candidates.isEmpty {
            messages.append(empty())
        } else if shown < candidates.count {
            messages.append(
                (
                    IMStrings.text("loadMore"), "",
                    { [weak self] in
                        self?.shown += 30
                        self?.buildRows()
                    }
                ))
        }
        rows = [status, verification, originalRows, messages]
        if view.window != nil {
            table.reloadData()
            table.byReloadEmptyViewAuto()
        }
    }

    private func originalMenu(_ row: IMJSON, declaration: Bool, objects: Bool) {
        let sheet = UIAlertController.make(
            title: IMStrings.text("receiptOriginals"), message: stateTitle(row), style: .actionSheet)
        sheet.addAction(
            UIAlertAction.make(IMStrings.text("receiptCheck")) { [weak self] in
                self?.run { [weak self] service in
                    let value: IMJSON?
                    if declaration {
                        value = try await service.checkDeclaration(row["id"].text)
                    } else {
                        value = try await service.checkReceipt(row["id"].text)
                    }
                    self?.notice.byText(IMStrings.text(value == nil ? "receiptUnknown" : "receiptFound"))
                }
            })
        if ["prepared", "unknown"].contains(row["state"].text), row["familyId"].text == service?.family {
            sheet.addAction(
                UIAlertAction.make(IMStrings.text("receiptRetry")) { [weak self] in
                    self?.confirm("receiptRetry") { [weak self] in
                        self?.run { service in
                            if declaration {
                                _ = try await service.checkDeclaration(row["id"].text, retry: true)
                            } else {
                                _ = try await service.checkReceipt(row["id"].text, retry: true)
                            }
                        }
                    }
                })
        }
        if !declaration, row["kind"].text == "content", row["state"].text == "completed", objects {
            sheet.addAction(
                UIAlertAction.make(IMStrings.text("receiptObject")) { [weak self] in
                    self?.confirm("receiptObject") { [weak self] in
                        self?.run { service in
                            _ = try await service.receiveObject(row["id"].text)
                        }
                    }
                })
        }
        if !declaration {
            sheet.addAction(
                UIAlertAction.make(IMStrings.text("receiptRange")) { [weak self] in
                    self?.run { [weak self] service in
                        let value = try await service.target(row["messageId"].text)
                        guard let self else {
                            return
                        }
                        let body = ["originalTarget", "contentRecorded", "objectRecorded"].map {
                            return IMStrings.text("receipt" + $0.prefix(1).uppercased() + $0.dropFirst())
                                + ": " + IMStrings.text(value[$0] == .bool(true) ? "receiptYes" : "receiptNo")
                        }.joined(separator: "\n")
                        IMUI.alert(self, title: IMStrings.text("receiptRange"), message: body)
                    }
                })
        }
        sheet.addAction(UIAlertAction.make(IMStrings.text("done"), style: .cancel) {})
        sheet.popoverPresentationController?.sourceView = view
        present(sheet, animated: true)
    }

    func numberOfSections(in tableView: UITableView) -> Int {
        return rows.count
    }

    func tableView(_ tableView: UITableView, numberOfRowsInSection section: Int) -> Int {
        return rows[section].count
    }

    func tableView(_ tableView: UITableView, titleForHeaderInSection section: Int) -> String? {
        return IMStrings.text(titles[section])
    }

    func tableView(_ tableView: UITableView, cellForRowAt indexPath: IndexPath) -> UITableViewCell {
        let row = rows[indexPath.section][indexPath.row]
        return UITableViewCell.make(style: .subtitle, reuseIdentifier: nil)
            .byText(row.title)
            .bySecondaryText(row.detail)
            .byAccessoryType(row.action == nil ? .none : .disclosureIndicator)
    }

    func tableView(_ tableView: UITableView, didSelectRowAt indexPath: IndexPath) {
        tableView.deselectRow(at: indexPath, animated: true)
        rows[indexPath.section][indexPath.row].action?()
    }
}
