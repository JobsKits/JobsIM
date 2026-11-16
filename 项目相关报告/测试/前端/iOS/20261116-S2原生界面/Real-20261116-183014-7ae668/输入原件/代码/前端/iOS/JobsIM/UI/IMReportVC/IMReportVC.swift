//
//  IMReportVC.swift
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
final class IMReportVC: IMBaseVC, UITableViewDataSource, UITableViewDelegate {
    private var items: [IMJSON] = []
    private lazy var table = UITableView.make(style: .insetGrouped).byDataSource(self).byDelegate(self)
        .byBackgroundColor(IMUI.background)
    static func present(on controller: IMBaseVC, type: String, id: String) {
        IMUI.prompt(controller, title: IMStrings.text("reportReason")) { reason in
            guard !reason.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else {
                return
            }
            controller.perform {
                try await IMReportService().submit(type: type, id: id, reason: reason)
                IMUI.alert(
                    controller, title: IMStrings.text("done"),
                    message: IMStrings.text(controller.session.demo ? "demo" : "reportSubmitted"))
            }
        }
    }
    override func viewDidLoad() {
        screenTitle = IMStrings.text("myReports")
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
            self.items = await self.session.list("reports")
            self.items += try self.session.store.list("report-outbox", namespace: self.session.namespace).map
            { $0.setting("state", .string("unknown")) }
            self.table.reloadData()
            self.table.byReloadEmptyViewAuto()
            self.table.refreshControl?.endRefreshing()
        }
    }
    func tableView(_ tableView: UITableView, numberOfRowsInSection section: Int) -> Int {
        return items.count
    }
    func tableView(_ tableView: UITableView, cellForRowAt indexPath: IndexPath) -> UITableViewCell {
        let item = items[indexPath.row]
        return UITableViewCell.make(style: .subtitle, reuseIdentifier: nil)
            .byText(item["reason"].text)
            .bySecondaryText(
                [
                    IMStrings.text(
                        [
                            "message": "reportMessage", "user": "reportUser",
                            "conversation": "reportConversation",
                        ][item["targetType"].text] ?? "report"), IMStrings.text(item["state"].text),
                    item["id"].text,
                ].joined(
                    separator: " · "))
    }
    func tableView(_ tableView: UITableView, didSelectRowAt indexPath: IndexPath) {
        tableView.deselectRow(at: indexPath, animated: true)
        let item = items[indexPath.row]
        guard item["state"].text == "unknown" else {
            return
        }
        perform { [weak self] in
            try await IMReportService().retry(item)
            self?.reload()
        }
    }

}
