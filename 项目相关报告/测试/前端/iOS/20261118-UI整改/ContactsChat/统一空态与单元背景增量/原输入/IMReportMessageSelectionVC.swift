//
//  IMReportMessageSelectionVC.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import JobsByUIKit
import JobsIMCore
import JobsSwiftDSL
import SnapKit
import UIKit

@MainActor
final class IMReportMessageSelectionVC: IMBaseVC, UITableViewDataSource, UITableViewDelegate {
    private let conversationID: String
    private let boundNamespace = IMSession.shared.namespace
    private var selected: Set<String>
    private let completed: ([String]) -> Void
    private var messages: [IMJSON] = []
    private lazy var table = UITableView.make().byDataSource(self).byDelegate(self)
        .byBackgroundColor(IMUI.background).byRowHeight(UITableView.automaticDimension).byEstimatedRowHeight(
            64)
    init(conversationID: String, selectedIDs: [String], completed: @escaping ([String]) -> Void) {
        self.conversationID = conversationID
        self.selected = Set(selectedIDs.prefix(50))
        self.completed = completed
        super.init(nibName: nil, bundle: nil)
        hidesBottomBarWhenPushed = true
    }
    required init?(coder: NSCoder) {
        fatalError("init(coder:) is unavailable")
    }
    override func viewDidLoad() {
        screenTitle = IMStrings.text("selectMessages")
        rightActions = [
            IMUI.button(IMStrings.text("done")) { [weak self] in
                guard let self, self.session.namespace == self.boundNamespace else {
                    return
                }
                self.completed(
                    self.messages.map {
                        $0["id"].text
                    }.filter(self.selected.contains))
                self.navigationController?.popViewController(animated: true)
            }
        ]
        super.viewDidLoad()
        table.byAddTo(content) {
            $0.edges.equalToSuperview()
        }
        table.byEmptyButtonProvider { [weak self] in
            IMUI.button(IMStrings.text("empty") + "\n" + IMStrings.text("reload")) {
                self?.reloadMessages()
            }
        }
        reloadMessages()
    }
    private func reloadMessages() {
        let original = session.namespace
        Task { [weak self] in
            guard let self, self.session.namespace == self.boundNamespace else {
                return
            }
            let items = await self.session.messageHistory(self.conversationID)
            guard self.session.namespace == original else {
                return
            }
            self.messages = items.filter {
                !$0["id"].text.isEmpty
            }
            self.table.reloadData()
            self.table.byReloadEmptyViewAuto()
        }
    }
    func tableView(_ tableView: UITableView, numberOfRowsInSection section: Int) -> Int {
        messages.count
    }
    func tableView(_ tableView: UITableView, cellForRowAt indexPath: IndexPath) -> UITableViewCell {
        let item = messages[indexPath.row]
        let unavailable = item["deleted"] == .bool(true) || item["contentUnavailable"] == .bool(true)
        let cell = UITableViewCell.make(style: .subtitle, reuseIdentifier: nil)
            .byText(unavailable ? IMStrings.text("contentUnavailable") : item.firstText("text", "type", "id"))
            .bySecondaryText(item["createdAt"].text).byAccessoryType(
                selected.contains(item["id"].text) ? .checkmark : .none)
        IMUI.cellTheme(cell)
        return cell
    }
    func tableView(_ tableView: UITableView, didSelectRowAt indexPath: IndexPath) {
        tableView.deselectRow(at: indexPath, animated: true)
        guard session.namespace == boundNamespace else {
            return
        }
        let id = messages[indexPath.row]["id"].text
        if selected.contains(id) {
            selected.remove(id)
        } else if selected.count < 50 {
            selected.insert(id)
        } else {
            IMUI.alert(self, message: IMStrings.text("reportSelectionLimit"))
        }
        tableView.reloadRows(at: [indexPath], with: .none)
    }
}
