//
//  IMSettingsVC.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import JobsByUIKit
import JobsIMCore
import JobsIMTransport
import JobsSwiftBaseDefines
import JobsSwiftDSL
import SnapKit
import UIKit
import UserNotifications

@MainActor
final class IMSettingsVC: IMBaseVC, UITableViewDataSource, UITableViewDelegate {

    private let sections = [
        ["displayName", "avatar", "connection"], ["theme", "language", "font", "selfState"],
        ["notifications", "devices", "recoveryCodes", "backup", "myReports"], ["logout"],
    ]
    private lazy var table = UITableView.make(style: .insetGrouped)
        .byDelegate(self)
        .byDataSource(self)
        .byBackgroundColor(IMUI.background)

    override func viewDidLoad() {
        screenTitle = IMStrings.text("settings")
        super.viewDidLoad()
        table.byAddTo(content) { [unowned self] make in
            make.edges.equalToSuperview()
        }
    }

    func numberOfSections(in tableView: UITableView) -> Int {
        sections.count
    }

    func tableView(_ tableView: UITableView, numberOfRowsInSection section: Int) -> Int {
        sections[section].count
    }

    func tableView(_ tableView: UITableView, titleForHeaderInSection section: Int) -> String? {
        section == 0 ? session.user.firstText("displayName", "username") : nil
    }

    func tableView(_ tableView: UITableView, cellForRowAt indexPath: IndexPath) -> UITableViewCell {
        let key = sections[indexPath.section][indexPath.row]
        let cell = UITableViewCell.make(style: .value1, reuseIdentifier: nil)
        var detail = ""
        if key == "connection" {
            detail = session.demo ? IMStrings.text("demo") : session.baseURL?.host ?? ""
        } else if key == "theme" {
            detail = IMStrings.text(UserDefaults.standard.string(forKey: "theme") ?? "system")
        } else if key == "language" {
            detail =
                IMStrings.languages.first {
                    $0.0 == (UserDefaults.standard.string(forKey: "language") ?? "zh-Hans")
                }?.1 ?? ""
        } else if key == "font" {
            let value = UserDefaults.standard.integer(forKey: "fontScale")
            detail = IMStrings.text(value == 4 ? "large" : value == -2 ? "small" : "standard")
        }
        cell
            .byText(IMStrings.text(key == "logout" && session.demo ? "backReal" : key))
            .bySecondaryText(detail)
            .byAccessoryType(.disclosureIndicator)
        return cell
    }

    func tableView(_ tableView: UITableView, didSelectRowAt indexPath: IndexPath) {
        tableView.deselectRow(at: indexPath, animated: true)
        let key = sections[indexPath.section][indexPath.row]
        if key == "theme" {
            choose(
                key,
                choices: ["system", "light", "dark"].map {
                    ($0, IMStrings.text($0))
                }
            ) { value in
                UserDefaults.standard.set(value, forKey: "theme")
            }
        } else if key == "language" {
            choose(key, choices: IMStrings.languages) { value in
                UserDefaults.standard.set(value, forKey: "language")
            }
        } else if key == "font" {
            choose(
                key,
                choices: [
                    ("-2", IMStrings.text("small")), ("0", IMStrings.text("standard")),
                    ("4", IMStrings.text("large")),
                ] + (session.personalStatesEnabled ? [("retry", IMStrings.text("retryOriginalState"))] : []),
                refreshImmediately: !session.personalStatesEnabled
            ) { value in
                if self.session.personalStatesEnabled {
                    let namespace = self.session.namespace
                    let family = self.session.currentCredentialPair?.familyID
                    Task { [weak self] in
                        guard let self, self.session.namespace == namespace,
                            self.session.currentCredentialPair?.familyID == family
                        else {
                            return
                        }
                        do {
                            let presets = ["-2": "small", "0": "standard", "4": "large"]
                            if value == "retry" {
                                try await self.session.retryPersonalState("me/preferences")
                            } else {
                                try await self.session.setFontPreference(presets[value] ?? "standard")
                            }
                        } catch {
                            self.session.lastError = error.localizedDescription
                            IMUI.alert(self, message: error.localizedDescription)
                        }
                    }
                } else {
                    UserDefaults.standard.set(Int(value) ?? 0, forKey: "fontScale")
                }
            }
        } else if key == "selfState" {
            guard session.personalStatesEnabled else {
                IMUI.alert(self, message: IMStrings.text("connectionNeeded"))
                return
            }
            navigationController?.pushViewController(IMPersonalStateVC(), animated: true)
        } else if key == "logout" {
            perform { [weak self] in
                guard let self else { return }
                await self.session.logout()
                if !self.session.demo && self.session.token != nil {
                    throw IMAPIError(
                        status: 0, code: "LOCAL_LOGOUT_NOT_SAVED", message: IMStrings.text("connectionNeeded")
                    )
                }
            }
        } else if key == "connection" {
            if session.demo {
                session.exitDemo()
                return
            }
            present(UINavigationController.make(IMAuthVC()), animated: true)
        } else if key == "notifications" {
            let ns = session.namespace
            let sheet = UIAlertController.make(
                title: IMStrings.text("notifications"), message: IMStrings.text("notificationPrivacy"),
                style: .actionSheet)
            sheet.addAction(
                UIAlertAction.make(
                    IMStrings.text("notificationEnabled") + " · "
                        + IMStrings.text(IMNotifications.shared.enabled(namespace: ns) ? "on" : "off")
                ) { [weak self] in
                    self?.perform {
                        if IMNotifications.shared.enabled(namespace: ns) {
                            UserDefaults.standard.set(false, forKey: "notifications:" + ns.key)
                        } else {
                            try await IMNotifications.shared.request(namespace: ns)
                        }
                    }
                })
            sheet.addAction(
                UIAlertAction.make(
                    IMStrings.text("notificationPreview") + " · "
                        + IMStrings.text(
                            UserDefaults.standard.bool(forKey: "notificationPreview:" + ns.key) ? "on" : "off"
                        )
                ) {
                    let key = "notificationPreview:" + ns.key
                    UserDefaults.standard.set(!UserDefaults.standard.bool(forKey: key), forKey: key)
                })
            sheet.addAction(UIAlertAction.make(IMStrings.text("cancel"), style: .cancel) {})
            sheet.popoverPresentationController?.sourceView = view
            present(sheet, animated: true)
        } else if key == "myReports" {
            navigationController?.pushViewController(IMReportVC(), animated: true)
        } else if key == "avatar" {
            navigationController?.pushViewController(IMAvatarVC(), animated: true)
        } else if key == "backup" {
            exportHistory()
        } else if key == "recoveryCodes" {
            IMUI.prompt(self, title: IMStrings.text("password"), secure: true) { [weak self] password in
                self?.perform {
                    guard let self else {
                        return
                    }
                    let ns = self.session.namespace
                    let result = try await self.session.action(
                        "auth/recovery-codes",
                        body: ["password": .string(password), "requestId": .string(UUID().uuidString)])
                    guard self.session.namespace == ns else { return }
                    let codes = result["codes"].list.map {
                        $0.text
                    }.joined(separator: "\n")
                    IMUI.alert(
                        self, title: IMStrings.text("recoveryCodes"),
                        message: codes.isEmpty
                            ? IMStrings.text(self.session.demo ? "demo" : "codesUnavailable")
                            : IMStrings.text("recoveryWarning") + "\n\n" + codes)
                }
            }
        } else if key == "devices" {
            navigationController?.pushViewController(IMDevicesVC(), animated: true)
        } else {
            IMUI.prompt(self, title: IMStrings.text(key), value: session.user["displayName"].text) {
                [weak self] value in
                self?.perform {
                    guard let self else {
                        return
                    }
                    let ns = self.session.namespace
                    let user = try await self.session.action(
                        "me", method: "PATCH", body: ["displayName": .string(value)])
                    guard self.session.namespace == ns else { return }
                    self.session.user =
                        self.session.demo ? self.session.user.setting("displayName", .string(value)) : user
                    if !self.session.demo {
                        UserDefaults.standard.set(
                            try JSONEncoder.make().encode(user), forKey: "user:" + self.session.profile)
                    }
                    self.table.reloadData()
                }
            }
        }
    }

    private func choose(_ key: String, choices: [(String, String)], refreshImmediately: Bool = true,
                        save: @escaping (String) -> Void) {
        let sheet = UIAlertController.make(title: IMStrings.text(key), message: nil, style: .actionSheet)
        for option in choices {
            sheet.addAction(
                UIAlertAction.make(option.1) { [weak self] in
                    save(option.0)
                    // A cloud write must acquire its captured owner before any root replacement.
                    if refreshImmediately {
                        self?.session.changed?()
                    }
                })
        }
        sheet.addAction(UIAlertAction.make(IMStrings.text("cancel"), style: .cancel) {})
        sheet.popoverPresentationController?.sourceView = view
        present(sheet, animated: true)
    }

    private func exportHistory() {
        perform { [weak self] in
            guard let self else {
                return
            }
            let data = try IMHistoryExport.data(store: self.session.store, namespace: self.session.namespace)
            let file = FileManager.default.temporaryDirectory.appendingPathComponent(
                "JobsIM-history-" + self.session.namespace.key + "-" + UUID().uuidString + ".json")
            try data.write(to: file, options: [.atomic, .completeFileProtection])
            let controller = UIActivityViewController.make([file])
            controller.completionWithItemsHandler = { _, _, _, _ in
                try? FileManager.default.removeItem(at: file)
            }
            controller.popoverPresentationController?.sourceView = self.view
            self.present(controller, animated: true)
        }
    }
}
