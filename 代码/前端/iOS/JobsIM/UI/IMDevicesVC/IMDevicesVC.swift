//
//  IMDevicesVC.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import UIKit

import JobsByUIKit
import JobsIMCore
import JobsSwiftDSL

import SnapKit

@MainActor
final class IMDevicesVC: IMBaseVC, UITableViewDataSource, UITableViewDelegate {
    private let boundNamespace = IMSession.shared.namespace
    private var devices: [IMJSON] = []
    private var currentID = ""
    private lazy var table = UITableView.make(style: .insetGrouped)
        .byDelegate(self).byDataSource(self).byBackgroundColor(IMUI.background)

    override func viewDidLoad() {
        screenTitle = IMStrings.text("devices")
        super.viewDidLoad()
        table.byAddTo(content) {
            make in make.edges.equalToSuperview()
        }
        table.byEmptyButtonProvider { [weak self] in
            IMUI.emptyButton {
                self?.requestReload()
            }
        }
        table.refreshControl = UIRefreshControl.jobsMake { control in
            control.addTarget(self, action: #selector(requestReload), for: .valueChanged)
        }
        requestReload()
    }
    @objc private func requestReload() {
        perform { [weak self] in
            guard let self else {
                return
            }
            defer {
                self.table.refreshControl?.endRefreshing()
            }
            let context = try await self.session.action("auth/context", method: "GET")
            let result = try await self.session.action("auth/sessions", method: "GET")
            guard self.session.namespace == self.boundNamespace else {
                return
            }
            self.currentID = context["deviceId"].text
            self.devices = result["items"].list
            self.table.reloadData()
            self.table.byReloadEmptyViewAuto()
        }
    }
    func tableView(_ tableView: UITableView, numberOfRowsInSection section: Int) -> Int {
        return devices.count
    }
    func tableView(_ tableView: UITableView, cellForRowAt indexPath: IndexPath) -> UITableViewCell {
        let item = devices[indexPath.row]
        let cell = UITableViewCell.make(style: .subtitle, reuseIdentifier: nil)
        let current = item.firstText("deviceId", "id") == currentID
        cell.byText(item.firstText("name", "deviceName", "deviceId", "id"))
            .bySecondaryText(current ? IMStrings.text("currentDevice") : item["createdAt"].text)
            .byIMThemeAccessoryType(.disclosureIndicator)
        return cell
    }
    func tableView(_ tableView: UITableView, didSelectRowAt indexPath: IndexPath) {
        tableView.deselectRow(at: indexPath, animated: true)
        let item = devices[indexPath.row]
        let id = item.firstText("deviceId", "id")
        let current = id == currentID
        IMUI.confirm(
            self, title: IMStrings.text("revokeDevice"),
            message: item.firstText("name", "deviceName", "deviceId", "id")
        ) { [weak self] in
            self?.perform {
                guard let self, self.session.namespace == self.boundNamespace else {
                    return
                }
                _ = try await self.session.action("auth/sessions/" + id, method: "DELETE")
                guard self.session.namespace == self.boundNamespace else {
                    return
                }
                if current {
                    await self.session.logout()
                } else {
                    self.requestReload()
                }
            }
        }
    }
}
