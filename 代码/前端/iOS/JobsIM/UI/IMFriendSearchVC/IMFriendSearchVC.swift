//
//  IMFriendSearchVC.swift
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
class IMFriendSearchVC: IMBaseVC, UITableViewDataSource, UITableViewDelegate, UISearchBarDelegate {
    enum Scope {
        case contacts, people
    }
    private let scope: Scope
    private let boundNamespace = IMSession.shared.namespace
    private var people: [IMJSON]
    private var results: [IMJSON] = []
    private var queryTask: Task<Void, Never>?
    private lazy var search = UISearchBar.jobsMake {
        bar in
        bar.byPlaceholder(IMStrings.text(self.scope == .people ? "searchPeople" : "searchContacts"))
            .byDelegate(
                self)
    }
    private lazy var table = UITableView.make().byDelegate(self).byDataSource(self)
        .byRowHeight(UITableView.automaticDimension).byEstimatedRowHeight(90)
        .byKeyboardDismissMode(.interactive).byBackgroundColor(IMUI.background)

    init(scope: Scope, people: [IMJSON] = []) {
        self.scope = scope
        self.people = people
        self.results = scope == .contacts ? people : []
        super.init(nibName: nil, bundle: nil)
        hidesBottomBarWhenPushed = true
    }
    required init?(coder: NSCoder) {
        fatalError("init(coder:) is unavailable")
    }
    override func viewDidLoad() {
        screenTitle = IMStrings.text(scope == .people ? "addFriend" : "searchContacts")
        super.viewDidLoad()
        search.byAddTo(content) {
            make in
            make.top.leading.trailing.equalToSuperview()
            make.height.equalTo(56)
        }
        table.byAddTo(content) {
            make in
            make.top.equalTo(self.search.snp.bottom)
            make.leading.trailing.bottom.equalToSuperview()
        }
        table.register(IMConversationCell.self, forCellReuseIdentifier: "person")
        table.byEmptyButtonProvider { [weak self] in
            IMUI.emptyButton {
                self?.searchNow()
            }
        }
    }
    override func viewDidAppear(_ animated: Bool) {
        super.viewDidAppear(animated)
        search.becomeFirstResponder()
        searchNow()
    }
    override func viewDidDisappear(_ animated: Bool) {
        super.viewDidDisappear(animated)
        queryTask?.cancel()
    }
    func searchBar(_ searchBar: UISearchBar, textDidChange searchText: String) {
        searchNow()
    }
    func searchBarSearchButtonClicked(_ searchBar: UISearchBar) {
        view.byEndEditing(true)
        searchNow()
    }
    private func searchNow() {
        queryTask?.cancel()
        let query = search.text ?? ""
        let original = session.namespace
        queryTask = Task { [weak self] in
            try? await Task.sleep(for: .milliseconds(250))
            guard let self, !Task.isCancelled, self.session.namespace == self.boundNamespace else {
                return
            }
            let rows: [IMJSON]
            if self.scope == .contacts {
                let snapshot = await self.session.contactsSnapshot()
                rows = snapshot.filter {
                    query.isEmpty
                        || $0.firstText("displayName", "username").localizedCaseInsensitiveContains(query)
                }
            } else if query.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
                rows = []
            } else {
                let allowed = CharacterSet.urlQueryAllowed.subtracting(CharacterSet(charactersIn: "&+=?#"))
                let encoded = query.addingPercentEncoding(withAllowedCharacters: allowed) ?? ""
                rows = await self.session.list("users?q=" + encoded, collection: "people-search")
            }
            guard !Task.isCancelled, self.session.namespace == original, self.search.text == query else {
                return
            }
            self.results = rows
            self.table.reloadData()
            self.table.byReloadEmptyViewAuto()
        }
    }
    func tableView(_ tableView: UITableView, numberOfRowsInSection section: Int) -> Int {
        results.count
    }
    func tableView(_ tableView: UITableView, cellForRowAt indexPath: IndexPath) -> UITableViewCell {
        let cell =
            tableView.dequeueReusableCell(withIdentifier: "person", for: indexPath) as! IMConversationCell
        cell.configure(results[indexPath.row])
        IMUI.cellTheme(cell)
        return cell
    }
    func tableView(_ tableView: UITableView, didSelectRowAt indexPath: IndexPath) {
        tableView.deselectRow(at: indexPath, animated: true)
        guard session.namespace == boundNamespace else {
            return
        }
        let person = results[indexPath.row]
        guard person["id"] != session.user["id"] else {
            navigationController?.pushViewController(IMUserProfileVC(user: person), animated: true)
            return
        }
        if scope == .contacts {
            perform { [weak self] in
                guard let self, self.session.namespace == self.boundNamespace else {
                    return
                }
                let original = self.session.namespace
                let conversation = try await self.session.directConversation(user: person)
                guard self.session.namespace == original else {
                    return
                }
                self.navigationController?.pushViewController(
                    IMChatVC(conversation: conversation), animated: true)
            }
        } else {
            IMUI.confirm(
                self, title: IMStrings.text("request"), message: person.firstText("displayName", "username")
            ) { [weak self] in
                self?.perform {
                    guard let self, self.session.namespace == self.boundNamespace else {
                        return
                    }
                    try await IMContactService().perform("request", userID: person["id"].text)
                    IMUI.alert(
                        self, title: IMStrings.text("done"),
                        message: IMStrings.text("done"))
                }
            }
        }
    }
}
