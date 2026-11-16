//
//  IMContactRequestsVC.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import UIKit

import JobsByUIKit
import JobsIMCore
import JobsSwiftBaseDefines
import JobsSwiftDSL

import SnapKit

@MainActor
final class IMContactRequestsVC: IMBaseVC, UITableViewDataSource, UITableViewDelegate {
    private var items: [IMJSON] = []
    private lazy var table = UITableView.make(style: .insetGrouped).byDataSource(self).byDelegate(self)
        .byBackgroundColor(IMUI.background)
    override func viewDidLoad() {
        screenTitle = IMStrings.text("requests")
        super.viewDidLoad()
        table.byAddTo(content) {
            make in make.edges.equalToSuperview()
        }
        table.byEmptyButtonProvider { [weak self] in
            IMUI.button(IMStrings.text("empty") + "\n" + IMStrings.text("reload")) {
                self?.reload()
            }
        }
        table.refreshControl = UIRefreshControl.jobsMake { control in
            control.addTarget(self, action: #selector(reload), for: .valueChanged)
        }
        reload()
    }
    @objc private func reload() {
        perform { [weak self] in
            guard let self else {
                return
            }
            _ = await self.session.contactsSnapshot()
            self.items = (try self.session.store.list("contact-requests", namespace: self.session.namespace))
                .filter { $0["recipientId"] == self.session.user["id"] }
            self.table.reloadData()
            self.table.byReloadEmptyViewAuto()
            self.table.refreshControl?.endRefreshing()
        }
    }
    func tableView(_ tableView: UITableView, numberOfRowsInSection section: Int) -> Int {
        return items.count
    }
    private func requesterTitle(_ item: IMJSON) -> String {
        let profile = item["requester"]
        let name = profile.firstText("displayName", "username")
        let username = profile["username"].text
        if !name.isEmpty {
            return username.isEmpty || name == username ? name : name + " · @" + username
        }
        return item.firstText("requesterName", "requesterId")
    }
    func tableView(_ tableView: UITableView, cellForRowAt indexPath: IndexPath) -> UITableViewCell {
        let item = items[indexPath.row]
        return UITableViewCell.make(style: .subtitle, reuseIdentifier: nil)
            .byText(requesterTitle(item))
            .bySecondaryText(
                IMStrings.text(item["state"].text == "accepted" ? "acceptedContact" : item["state"].text)
            )
            .byAccessibilityIdentifier("contact-request-" + item["id"].text)
    }
    func tableView(_ tableView: UITableView, didSelectRowAt indexPath: IndexPath) {
        tableView.deselectRow(at: indexPath, animated: true)
        let item = items[indexPath.row]
        guard item["state"].text == "pending" else {
            return
        }
        let sheet = UIAlertController.make(title: requesterTitle(item), message: nil, style: .actionSheet)
        for action in ["accept", "reject"] {
            sheet.addAction(
                UIAlertAction.make(
                    IMStrings.text(action), style: action == "reject" ? .destructive : .default
                ) { [weak self] in
                    self?.perform {
                        _ = try await self?.session.action(
                            "contacts/requests/" + item["id"].text + "/" + action)
                        self?.reload()
                    }
                })
        }
        sheet.addAction(UIAlertAction.make(IMStrings.text("cancel"), style: .cancel) {})
        sheet.popoverPresentationController?.sourceView = tableView.cellForRow(at: indexPath)
        present(sheet, animated: true)
    }
}
