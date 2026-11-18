//
//  IMSavedEditingDraftsVC.swift
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
final class IMSavedEditingDraftsVC: IMBaseVC, UITableViewDataSource, UITableViewDelegate {
    private let conversationID: String
    private let boundNamespace = IMSession.shared.namespace
    private let selected: (IMJSON) -> Void
    private var drafts: [IMJSON] = []
    private lazy var table = UITableView.make().byDataSource(self).byDelegate(self).byBackgroundColor(
        IMUI.background
    )
    .byRowHeight(UITableView.automaticDimension).byEstimatedRowHeight(72)
    init(conversationID: String, selected: @escaping (IMJSON) -> Void) {
        self.conversationID = conversationID
        self.selected = selected
        super.init(nibName: nil, bundle: nil)
        hidesBottomBarWhenPushed = true
    }
    required init?(coder: NSCoder) {
        fatalError("init(coder:) is unavailable")
    }
    override func viewDidLoad() {
        screenTitle = IMStrings.text("savedEditingDrafts")
        super.viewDidLoad()
        table.byAddTo(content) {
            $0.edges.equalToSuperview()
        }
        table.byEmptyButtonProvider { [weak self] in
            IMUI.button(IMStrings.text("empty") + "\n" + IMStrings.text("reload")) {
                self?.reloadDrafts()
            }
        }
        reloadDrafts()
    }
    private func reloadDrafts() {
        guard session.namespace == boundNamespace else {
            return
        }
        drafts = session.savedEditingDrafts(conversationID: conversationID)
        table.reloadData()
        table.byReloadEmptyViewAuto()
    }
    func tableView(_ tableView: UITableView, numberOfRowsInSection section: Int) -> Int {
        drafts.count
    }
    func tableView(_ tableView: UITableView, cellForRowAt indexPath: IndexPath) -> UITableViewCell {
        let item = drafts[indexPath.row]
        let cell = UITableViewCell.make(style: .subtitle, reuseIdentifier: nil)
            .byText(item["text"].text).bySecondaryText(item["savedAt"].text)
        IMUI.cellTheme(cell)
        return cell
    }
    func tableView(_ tableView: UITableView, didSelectRowAt indexPath: IndexPath) {
        tableView.deselectRow(at: indexPath, animated: true)
        guard session.namespace == boundNamespace else {
            return
        }
        selected(drafts[indexPath.row])
    }
}
