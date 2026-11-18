//
//  IMListVC.swift
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

@MainActor
class IMListVC: IMBaseVC, UITableViewDataSource, UITableViewDelegate, UISearchBarDelegate {

    enum Kind {
        case conversations, contacts, saved
    }
    private let kind: Kind
    private let localHistory: Bool
    private var items: [IMJSON] = []
    private var allItems: [IMJSON] = []
    private var poll: Task<Void, Never>?
    private var filter = 0
    private var pageGeneration = 0
    private var pageLoading = false
    private var pageCursor = "0"
    private var pageHasMore = false
    private var queryTask: Task<Void, Never>?
    private let messageLists = IMMessageListService()
    private lazy var personalLists = IMUI.button(IMStrings.text("conversationDisplay")) { [weak self] in
        self?.choosePersonalList()
    }.byAccessibilityIdentifier("self.recoveryLists")
    private var showingMessageResults: Bool {
        return !localHistory && (kind == .saved || kind == .conversations && !(search.text ?? "").isEmpty)
    }
    private lazy var pageStatus = IMUI.label("", style: .caption1, color: IMUI.secondary)
    private lazy var pageFooter = IMUI.stack(spacing: 6)
    private lazy var moreButton = IMUI.button(IMStrings.text("loadMore")) { [weak self] in
        self?.requestNextPage()
    }.byTitleFont(IMUI.font(.body))
    private lazy var table = UITableView.make()
        .byDelegate(self)
        .byDataSource(self)
        .byRowHeight(UITableView.automaticDimension)
        .byEstimatedRowHeight(94)
        .bySeparatorStyle(.none)
        .byBackgroundColor(IMUI.background)
    private lazy var search = UISearchBar.jobsMake {
        bar in
        bar
            .byPlaceholder(IMStrings.text("search"))
            .byDelegate(self)
    }

    init(kind: Kind, localHistory: Bool = false) {
        self.kind = kind
        self.localHistory = localHistory
        super.init(nibName: nil, bundle: nil)
    }

    required init?(coder: NSCoder) {
        fatalError("init(coder:) is unavailable")
    }

    override func viewDidLoad() {
        screenTitle = IMStrings.text(
            localHistory
                ? "localHistory"
                : kind == .conversations ? "messages" : kind == .contacts ? "contacts" : "saved")
        hidesScreenHeader = kind == .conversations && !localHistory
        super.viewDidLoad()
        assemble()
    }

    override func viewWillAppear(_ animated: Bool) {
        super.viewWillAppear(animated)
        poll?.cancel()
        poll = Task { [weak self] in
            await self?.reload()
            while !Task.isCancelled {
                if UIApplication.shared.applicationState == .active, self?.showingMessageResults == false {
                    await self?.reload()
                }
                try? await Task.sleep(for: .seconds(5))
            }
        }
    }

    override func viewDidDisappear(_ animated: Bool) {
        super.viewDidDisappear(animated)
        poll?.cancel()
        queryTask?.cancel()
    }

    private func assemble() {
        let header = IMUI.stack(spacing: 10)
        let caption = IMUI.label(
            IMStrings.text(session.demo ? "demo" : "live"), style: .subheadline, color: IMUI.green)
        header.byAddArrangedSubviews(kind == .saved ? [caption] : [caption, search])
        if kind == .contacts {
            header.byAddArrangedSubview(
                IMUI.button(IMStrings.text("requests")) { [weak self] in
                    self?.navigationController?.pushViewController(IMContactRequestsVC(), animated: true)
                })
        }
        if kind == .conversations, !localHistory, !session.demo {
            header.byAddArrangedSubview(
                IMUI.button(IMStrings.text("localHistory")) { [weak self] in
                    self?.navigationController?.pushViewController(
                        IMListVC(kind: .conversations, localHistory: true), animated: true)
                }.byAccessibilityIdentifier("sync.localHistory"))
            header.byAddArrangedSubview(personalLists)
        }
        if session.demo {
            let back = IMUI.button(IMStrings.text("backReal")) { [weak self] in
                self?.session.exitDemo()
            }
            header.byAddArrangedSubview(back)
        }
        let filters = UISegmentedControl.make([
            IMStrings.text("all"), IMStrings.text("unread"), IMStrings.text("groups"),
        ])
        filters.addTarget(self, action: #selector(changeFilter(_:)), for: .valueChanged)
        if kind == .conversations {
            header.byAddArrangedSubview(filters)
        }
        header.byAddTo(content) {
            [unowned self] make in
            make.top.leading.trailing.equalToSuperview().inset(24)
        }
        pageFooter
            .byAddArrangedSubviews([pageStatus, moreButton])
            .byAddTo(content) {
                [unowned self] make in
                make.leading.trailing.equalToSuperview().inset(24)
                make.bottom.equalToSuperview().inset(8)
            }
        moreButton.snp.makeConstraints {
            make in
            make.height.greaterThanOrEqualTo(44).priority(999)
        }
        updatePageFooter()
        table.byAddTo(content) {
            [unowned self] make in
            make.top.equalTo(header.snp.bottom).offset(14)
            make.leading.trailing.equalToSuperview()
            make.bottom.equalTo(pageFooter.snp.top).offset(-6)
        }
        table.register(IMConversationCell.self, forCellReuseIdentifier: "conversation")
        let refresh = UIRefreshControl.jobsMake {
            refresh in
            refresh.addTarget(self, action: #selector(refreshList), for: .valueChanged)
        }
        table.byRefreshControl(refresh)
        table.byEmptyButtonProvider { [weak self] in
            IMUI.emptyButton {
                self?.refreshList()
            }
        }
    }

    @objc private func refreshList() {
        Task {
            await reload()
        }
    }

    private func requestNextPage() {
        Task { [weak self] in
            await self?.loadMessagePage(reset: false)
        }
    }

    @objc private func changeFilter(_ sender: UISegmentedControl) {
        filter = sender.selectedSegmentIndex
        personalLists.byTitle(IMStrings.text("conversationDisplay"))
        applyFilter()
    }

    private func choosePersonalList() {
        let sheet = UIAlertController.make(
            title: IMStrings.text("conversationDisplay"), message: nil, style: .actionSheet)
        for (index, key) in [(0, "all"), (3, "showArchived"), (4, "showHidden")] {
            sheet.addAction(
                UIAlertAction.make(IMStrings.text(key)) { [weak self] in
                    self?.filter = index
                    self?.personalLists.byTitle(IMStrings.text(key))
                    self?.applyFilter()
                })
        }
        sheet.addAction(
            UIAlertAction.make(IMStrings.text("cancel"), style: .cancel) {
            })
        sheet.popoverPresentationController?.sourceView = personalLists
        present(sheet, animated: true)
    }

    private func applyFilter() {
        let query = search.text ?? ""
        let values =
            kind == .conversations && !localHistory && !showingMessageResults
            ? IMPersonalStateService.displayedConversations(allItems, filter: filter) : allItems
        items = values.filter {
            item in
            let searchable = item.firstText("title", "displayName", "username", "text")
            return
                (showingMessageResults || kind == .contacts || query.isEmpty
                || searchable.localizedCaseInsensitiveContains(query))
                && (kind == .conversations && !localHistory || showingMessageResults || filter != 1
                    || (!item["unreadCount"].text.isEmpty && item["unreadCount"].text != "0"))
                && (kind == .conversations && !localHistory || showingMessageResults || filter != 2
                    || item["kind"].text == "group")
        }
        table.reloadData()
        table.byReloadEmptyViewAuto()
    }

    private func reload() async {
        personalLists.byHidden(!session.personalStatesEnabled)
        if showingMessageResults {
            await loadMessagePage(reset: true)
            return
        }
        pageHasMore = false
        updatePageFooter()
        switch kind {
        /// 同步正式会话或隔离演示列表。
        case .conversations:
            if localHistory {
                allItems = session.retainedConversations()
            } else {
                allItems = await session.conversationSnapshot()
            }
        /// 联系人只使用当前账户上下文。
        case .contacts:
            let query = search.text ?? ""
            allItems =
                query.isEmpty
                ? await session.contactsSnapshot()
                : await session.list(
                    "users?q=" + (query.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? ""),
                    collection: "people-search")
        /// 收藏通过独立分页接口，不使用空搜索词。
        case .saved:
            break
        }
        table.refreshControl?.endRefreshing()
        applyFilter()
    }

    override func syncProjectionChanged() {
        if localHistory {
            allItems = session.retainedConversations()
        } else {
            let state = try? session.store.syncStateIfPresent(namespace: session.namespace)
            guard state?["restriction"].text.isEmpty == true, session.currentCredentialPair != nil else {
                allItems = []
                applyFilter()
                return
            }
            let conversations = (try? session.store.list("conversations", namespace: session.namespace)) ?? []
            let allowed = Set(
                conversations.map {
                    $0["id"].text
                })
            if kind == .conversations, !showingMessageResults {
                allItems.removeAll {
                    !allowed.contains($0["id"].text)
                }
                allItems = allItems.map {
                    value in
                    let last = session.cachedMessages(value["id"].text).last
                    let current = session.personalValue("conversation_view", conversation: value["id"].text)
                    return value.setting("lastMessage", last?["text"] ?? .string(""))
                        .setting("personalView", current ?? .null)
                }
            } else if showingMessageResults {
                let global = Set((try? session.store.syncGlobalIDs(namespace: session.namespace)) ?? [])
                allItems = allItems.compactMap {
                    value in
                    guard !global.contains(value["id"].text), allowed.contains(value["conversationId"].text)
                    else {
                        return nil
                    }
                    return session.cachedMessages(value["conversationId"].text).first {
                        $0["id"] == value["id"]
                    }
                }
            } else if kind == .contacts {
                let peers = Set(
                    ((try? session.store.list("contacts", namespace: session.namespace)) ?? []).map {
                        $0["id"].text
                    })
                allItems.removeAll {
                    !peers.contains($0["id"].text)
                }
            }
        }
        applyFilter()
    }

    func searchBar(_ searchBar: UISearchBar, textDidChange searchText: String) {
        queryTask?.cancel()
        pageGeneration += 1
        pageLoading = false
        if kind == .conversations {
            allItems = []
            pageHasMore = false
            applyFilter()
            updatePageFooter()
            queryTask = Task { [weak self] in
                try? await Task.sleep(for: .milliseconds(250))
                guard !Task.isCancelled else {
                    return
                }
                await self?.reload()
            }
            return
        }
        if kind == .contacts && !searchText.isEmpty {
            Task { [weak self] in
                guard let self else {
                    return
                }
                let encoded =
                    searchText.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) ?? ""
                let results = await self.session.list("users?q=" + encoded, collection: "people-search")
                guard self.search.text == searchText else {
                    return
                }
                self.allItems = results
                self.applyFilter()
            }
        } else {
            Task {
                await reload()
            }
        }
    }

    func searchBarSearchButtonClicked(_ searchBar: UISearchBar) {
        queryTask?.cancel()
        view.byEndEditing(true)
        Task { [weak self] in
            await self?.reload()
        }
    }

    private func loadMessagePage(reset: Bool) async {
        guard showingMessageResults else {
            return
        }
        if reset {
            pageGeneration += 1
            pageCursor = "0"
            pageHasMore = false
            pageLoading = false
        }
        guard !pageLoading, reset || pageHasMore else {
            return
        }
        let generation = pageGeneration
        let namespace = session.namespace
        let query = search.text ?? ""
        pageLoading = true
        pageStatus.byText(IMStrings.text("loadingMessages"))
        updatePageFooter()
        defer {
            if generation == pageGeneration {
                pageLoading = false
                table.refreshControl?.endRefreshing()
                updatePageFooter()
            }
        }
        do {
            let page = try await messageLists.page(
                favorites: kind == .saved, query: query, cursor: pageCursor)
            guard generation == pageGeneration, namespace == session.namespace,
                kind == .saved || search.text == query
            else {
                return
            }
            if reset {
                allItems = page.items
            } else {
                let existing = Set(
                    allItems.map {
                        $0["id"].text
                    })
                allItems += page.items.filter {
                    !existing.contains($0["id"].text)
                }
            }
            pageCursor = page.cursor
            pageHasMore = page.hasMore
            pageStatus.byText("")
            applyFilter()
        } catch {
            guard generation == pageGeneration, namespace == session.namespace else {
                return
            }
            if let denied = error as? IMAPIError, [401, 403, 404, 409].contains(denied.status) {
                allItems = []
                pageHasMore = false
                applyFilter()
            } else {
                allItems = allItems.map {
                    value in
                    guard IMForwardService.isForward(value) else {
                        return value
                    }
                    var fields = value.fields
                    for key in ["text", "attachmentId", "attachmentURL", "forwardAdmission"] {
                        fields.removeValue(forKey: key)
                    }
                    fields["contentUnavailable"] = .bool(true)
                    return .object(fields)
                }
                applyFilter()
            }
            pageStatus.byText(IMStrings.text("error") + " · " + IMStrings.text("reload"))
        }
    }

    private func updatePageFooter() {
        moreButton
            .byTitle(IMStrings.text(pageLoading ? "loadingMessages" : "loadMore"))
            .byEnabled(!pageLoading)
            .byHidden(!showingMessageResults || !pageHasMore && !pageLoading)
        pageStatus.byHidden(pageStatus.text?.isEmpty != false)
    }

    func tableView(_ tableView: UITableView, numberOfRowsInSection section: Int) -> Int {
        items.count
    }

    func tableView(_ tableView: UITableView, cellForRowAt indexPath: IndexPath) -> UITableViewCell {
        let cell =
            tableView.dequeueReusableCell(withIdentifier: "conversation", for: indexPath)
            as! IMConversationCell
        var value = items[indexPath.row]
        if showingMessageResults {
            let metadata = (try? session.store.list("conversations", namespace: session.namespace))?.first {
                $0["id"] == value["conversationId"]
            }
            value = value.setting(
                "title", .string(metadata?.firstText("title", "displayName") ?? IMStrings.text("messages"))
            )
            .setting("lastMessage", value["text"])
        }
        cell.configure(value)
        return cell
    }

    func tableView(_ tableView: UITableView, didSelectRowAt indexPath: IndexPath) {
        tableView.deselectRow(at: indexPath, animated: true)
        let item = items[indexPath.row]
        if kind == .contacts {
            createDirect(item)
        } else if showingMessageResults {
            let namespace = session.namespace
            perform { [weak self] in
                guard let self else {
                    return
                }
                let source: (conversation: IMJSON, message: IMJSON)
                do {
                    source = try await self.messageLists.locate(item)
                } catch {
                    if self.session.namespace == namespace, IMForwardService.isForward(item) {
                        self.allItems = self.allItems.map {
                            value in
                            guard value["id"] == item["id"] else {
                                return value
                            }
                            var fields = value.fields
                            for key in ["text", "attachmentId", "attachmentURL", "forwardAdmission"] {
                                fields.removeValue(forKey: key)
                            }
                            fields["contentUnavailable"] = .bool(true)
                            return .object(fields)
                        }
                        self.applyFilter()
                    }
                    throw error
                }
                guard self.session.namespace == namespace else {
                    return
                }
                self.allItems = self.allItems.map {
                    $0["id"] == item["id"] ? source.message : $0
                }
                self.applyFilter()
                self.navigationController?.pushViewController(
                    IMChatVC(conversation: source.conversation, focusMessageID: source.message["id"].text),
                    animated: true)
            }
        } else {
            navigationController?.pushViewController(IMChatVC(conversation: item), animated: true)
        }
    }

    private func createDirect(_ person: IMJSON) {
        perform { [weak self] in
            guard let self else {
                return
            }
            let original = self.session.namespace
            let result = try await self.session.directConversation(user: person)
            guard self.session.namespace == original else {
                return
            }
            self.navigationController?.pushViewController(IMChatVC(conversation: result), animated: true)
        }
    }
}
