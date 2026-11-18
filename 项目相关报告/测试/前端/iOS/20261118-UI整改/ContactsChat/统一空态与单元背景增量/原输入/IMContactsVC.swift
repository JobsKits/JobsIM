//
//  IMContactsVC.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import JobsByUIKit
import JobsIMCore
import JobsSwiftBaseDefines
import JobsSwiftDSL
import SnapKit
import UIKit

@MainActor
final class IMContactsVC: IMBaseVC, UITableViewDataSource, UITableViewDelegate, UISearchBarDelegate {
    enum Mode {
        case contacts, groups, starred
    }
    private let mode: Mode
    private let boundNamespace = IMSession.shared.namespace
    private var people: [IMJSON] = []
    private var groups: [String: [IMJSON]] = [:]
    private let indexTitles =
        ["🌟"]
        + "ABCDEFGHIJKLMNOPQRSTUVWXYZ".map {
            String($0)
        } + ["#"]
    private var refreshTask: Task<Void, Never>?
    private lazy var table = UITableView.make().byDelegate(self).byDataSource(self)
        .byRowHeight(UITableView.automaticDimension).byEstimatedRowHeight(90)
        .byBackgroundColor(IMUI.background).byKeyboardDismissMode(.interactive)
        .bySectionIndexColor(IMUI.green).bySectionIndexBackgroundColor(IMUI.background)
    private lazy var search = UISearchBar.jobsMake {
        bar in
        bar.byPlaceholder(IMStrings.text("searchContacts")).byDelegate(self)
    }
    private lazy var searchShortcut = IMUI.button("") { [weak self] in
        self?.openSearch()
    }
    .byImage("magnifyingglass".sysImg).byAccessibilityLabel(IMStrings.text("searchContacts"))
    .byHidden(true)
    private lazy var header = UIView.jobsMake {
        view in
        view.byBackgroundColor(IMUI.background)
    }
    private lazy var toolbar = UIStackView.jobsMake {
        row in
        row.byAxis(.horizontal).bySpacing(8).byAlignment(.center)
    }
    private lazy var spacer = UIView.jobsMake {
        _ in
    }
    private lazy var add = IMUI.button("") { [weak self] in
        self?.navigationController?.pushViewController(IMAddFriendVC(), animated: true)
    }.byImage("person.badge.plus".sysImg).byAccessibilityLabel(IMStrings.text("addFriend"))
    private lazy var headerStack = IMUI.stack(spacing: 0)
    private var destinationButtons: [UIButton] = []
    private lazy var refreshControl = UIRefreshControl.jobsMake {
        control in
        control.addTarget(self, action: #selector(reloadContacts), for: .valueChanged)
    }

    init(mode: Mode = .contacts) {
        self.mode = mode
        super.init(nibName: nil, bundle: nil)
        hidesBottomBarWhenPushed = mode != .contacts
    }
    required init?(coder: NSCoder) {
        fatalError("init(coder:) is unavailable")
    }

    override func viewDidLoad() {
        screenTitle = IMStrings.text(
            mode == .groups ? "groupChats" : mode == .starred ? "starredFriends" : "contacts")
        hidesScreenHeader = mode == .contacts
        super.viewDidLoad()
        if mode == .contacts {
            toolbar.byAddArrangedSubviews([spacer, searchShortcut, add]).byAddTo(content) {
                make in
                make.top.equalToSuperview()
                make.leading.trailing.equalToSuperview().inset(16)
                make.height.equalTo(44)
            }
            for button in [searchShortcut, add] {
                button.snp.makeConstraints {
                    $0.width.height.equalTo(44)
                }
            }
        }
        table.byAddTo(content) {
            make in
            make.leading.trailing.bottom.equalToSuperview()
            if self.mode == .contacts {
                make.top.equalTo(self.toolbar.snp.bottom)
            } else {
                make.top.equalToSuperview()
            }
        }
        table.register(IMConversationCell.self, forCellReuseIdentifier: "contact")
        headerStack.byAddArrangedSubview(search)
        search.snp.makeConstraints {
            $0.height.equalTo(56)
        }
        if mode == .contacts {
            for (key, icon, destination) in [
                ("newFriends", "person.badge.plus", 0), ("groupChats", "person.2", 1),
                ("starredFriends", "star", 2),
            ] {
                let button = IMUI.button(IMStrings.text(key)) { [weak self] in
                    self?.openDestination(destination)
                }
                .byImage(icon.sysImg).byContentHorizontalAlignment(.leading)
                button.snp.makeConstraints {
                    $0.height.greaterThanOrEqualTo(52)
                }
                destinationButtons.append(button)
                headerStack.byAddArrangedSubview(button)
            }
        }
        headerStack.byAddTo(header) {
            $0.edges.equalToSuperview().inset(12)
        }
        table.byTableHeaderView(header)
        table.byEmptyButtonProvider { [weak self] in
            IMUI.button(IMStrings.text("empty") + "\n" + IMStrings.text("reload")) {
                self?.reloadContacts()
            }
        }
        table.byRefreshControl(refreshControl)
    }
    override func viewDidLayoutSubviews() {
        super.viewDidLayoutSubviews()
        let height = CGFloat(mode == .contacts ? 236 : 80)
        if header.frame.width != table.bounds.width || header.frame.height != height {
            header.byFrame(CGRect(x: 0, y: 0, width: table.bounds.width, height: height))
            table.byTableHeaderView(header)
        }
    }
    override func viewDidAppear(_ animated: Bool) {
        super.viewDidAppear(animated)
        applyFilter()
    }
    override func viewWillAppear(_ animated: Bool) {
        super.viewWillAppear(animated)
        reloadContacts()
    }
    override func viewDidDisappear(_ animated: Bool) {
        super.viewDidDisappear(animated)
        refreshTask?.cancel()
    }
    override func syncProjectionChanged() {
        reloadContacts()
    }

    @objc private func reloadContacts() {
        refreshTask?.cancel()
        let original = session.namespace
        refreshTask = Task { [weak self] in
            guard let self, self.session.namespace == self.boundNamespace else {
                return
            }
            let rows =
                self.mode == .groups
                ? await self.session.conversationSnapshot().filter {
                    $0["kind"].text == "group"
                }
                : await self.session.contactsSnapshot()
            guard !Task.isCancelled, self.session.namespace == original else {
                return
            }
            self.people = rows
            self.applyFilter()
            self.table.refreshControl?.endRefreshing()
        }
    }
    private func applyFilter() {
        let stars = IMContactLocalStars.ids(session: session)
        let query = search.text ?? ""
        let visible = people.filter {
            item in
            (mode != .starred || stars.contains(item["id"].text))
                && (query.isEmpty
                    || item.firstText("displayName", "title", "username").localizedCaseInsensitiveContains(
                        query))
        }
        groups = Dictionary(grouping: visible) {
            item in
            mode != .groups && stars.contains(item["id"].text)
                ? "🌟" : Self.sectionKey(item.firstText("displayName", "title", "username"))
        }.mapValues {
            $0.sorted {
                $0.firstText("displayName", "title", "username").localizedStandardCompare(
                    $1.firstText("displayName", "title", "username")) == .orderedAscending
            }
        }
        guard table.window != nil else {
            return
        }
        table.reloadData()
        table.byReloadEmptyViewAuto()
    }
    static func sectionKey(_ name: String) -> String {
        guard let first = name.unicodeScalars.first, !CharacterSet.whitespacesAndNewlines.contains(first),
            !(first.properties.isEmojiPresentation || first.properties.isEmoji && first.value > 127)
        else {
            return "#"
        }
        let latin = (name.applyingTransform(.toLatin, reverse: false) ?? name)
            .folding(
                options: [.diacriticInsensitive, .caseInsensitive], locale: Locale(identifier: "en_US_POSIX")
            ).uppercased()
        guard let scalar = latin.unicodeScalars.first, (65...90).contains(scalar.value) else {
            return "#"
        }
        return String(scalar)
    }
    private func openDestination(_ index: Int) {
        let controller: UIViewController =
            index == 0 ? IMContactRequestsVC() : IMContactsVC(mode: index == 1 ? .groups : .starred)
        controller.hidesBottomBarWhenPushed = true
        navigationController?.pushViewController(controller, animated: true)
    }
    private func openSearch() {
        navigationController?.pushViewController(IMContactSearchVC(people: people), animated: true)
    }
    func scrollViewDidScroll(_ scrollView: UIScrollView) {
        searchShortcut.byHidden(
            table.contentOffset.y + table.adjustedContentInset.top < search.frame.maxY + 12)
    }
    func searchBar(_ searchBar: UISearchBar, textDidChange searchText: String) {
        applyFilter()
    }
    func searchBarSearchButtonClicked(_ searchBar: UISearchBar) {
        view.byEndEditing(true)
    }
    func numberOfSections(in tableView: UITableView) -> Int {
        indexTitles.count
    }
    func sectionIndexTitles(for tableView: UITableView) -> [String]? {
        indexTitles
    }
    func tableView(_ tableView: UITableView, sectionForSectionIndexTitle title: String, at index: Int) -> Int
    {
        index
    }
    func tableView(_ tableView: UITableView, titleForHeaderInSection section: Int) -> String? {
        groups[indexTitles[section]]?.isEmpty == false ? indexTitles[section] : nil
    }
    func tableView(_ tableView: UITableView, numberOfRowsInSection section: Int) -> Int {
        groups[indexTitles[section]]?.count ?? 0
    }
    private func item(at index: IndexPath) -> IMJSON {
        groups[indexTitles[index.section]]![index.row]
    }
    func tableView(_ tableView: UITableView, cellForRowAt indexPath: IndexPath) -> UITableViewCell {
        let cell =
            tableView.dequeueReusableCell(withIdentifier: "contact", for: indexPath) as! IMConversationCell
        cell.configure(item(at: indexPath))
        // This cell owns its labels; default content configuration would replace them.
        cell.byBackgroundColor(IMUI.background)
            .bySelectedBackgroundView(UIView.jobsMake {
                $0.byBackgroundColor(IMUI.selection)
            })
        return cell
    }
    func tableView(_ tableView: UITableView, didSelectRowAt indexPath: IndexPath) {
        tableView.deselectRow(at: indexPath, animated: true)
        guard session.namespace == boundNamespace else {
            return
        }
        let person = item(at: indexPath)
        if mode == .groups {
            navigationController?.pushViewController(IMChatVC(conversation: person), animated: true)
        } else {
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
        }
    }
    func tableView(_ tableView: UITableView, trailingSwipeActionsConfigurationForRowAt indexPath: IndexPath)
        -> UISwipeActionsConfiguration?
    {
        guard mode != .groups, session.namespace == boundNamespace else {
            return nil
        }
        let person = item(at: indexPath)
        let action = UIContextualAction.makeContactRemoval {
            [weak self] completion in
            guard let self, self.session.namespace == self.boundNamespace else {
                completion(false)
                return
            }
            IMContactUX.confirmRemoval(on: self) { [weak self] in
                self?.perform {
                    guard let self, self.session.namespace == self.boundNamespace else {
                        return
                    }
                    try await IMContactService().perform("remove", userID: person["id"].text)
                    try IMContactLocalStars.set(false, userID: person["id"].text, session: self.session)
                    self.reloadContacts()
                }
            }
            completion(false)
        }
        return UISwipeActionsConfiguration.makeContactActions([action])
    }
}
