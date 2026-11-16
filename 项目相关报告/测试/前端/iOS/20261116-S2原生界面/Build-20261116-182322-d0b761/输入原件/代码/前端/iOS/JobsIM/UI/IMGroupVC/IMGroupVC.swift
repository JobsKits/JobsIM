//
//  IMGroupVC.swift
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
final class IMGroupVC: IMBaseVC, UITableViewDataSource, UITableViewDelegate {

    private var conversation: IMJSON
    private var members: [IMJSON] = []
    private var announcement: IMJSON = .null
    private var announcementEnabled = false
    private var announcementRoleProtected = false
    private var announcementPending = false
    private let announcements = IMAnnouncementService()
    private var isOwner: Bool {
        return conversation["ownerId"] == session.user["id"] && conversation["kind"].text == "group"
            && conversation["readOnly"] != .bool(true)
    }
    private var actions: [String] {
        var keys = isOwner ? ["rename", "addMember"] : []
        if announcementEnabled {
            keys.append("announcement")
            if isOwner && announcementRoleProtected && !announcement["version"].text.isEmpty
                && announcement["state"].text != "empty"
                && !announcementPending
            {
                keys.append("clearAnnouncement")
            }
            if announcementPending { keys.append("reconcileOriginal") }
        }
        return keys + (conversation["kind"].text == "group" ? ["leave"] : []) + ["report"]
    }
    private lazy var table = UITableView.make(style: .insetGrouped)
        .byDelegate(self)
        .byDataSource(self)
        .byBackgroundColor(IMUI.background)

    init(conversation: IMJSON) {
        self.conversation = conversation
        super.init(nibName: nil, bundle: nil)
    }

    required init?(coder: NSCoder) {
        fatalError("init(coder:) is unavailable")
    }

    override func viewDidLoad() {
        screenTitle = conversation["title"].text
        super.viewDidLoad()
        table.register(IMConversationCell.self, forCellReuseIdentifier: "member")
        table.rowHeight = UITableView.automaticDimension
        table.estimatedRowHeight = 94
        table.byAddTo(content) { [unowned self] make in
            make.edges.equalToSuperview()
        }
        table.byEmptyButtonProvider { [weak self] in
            IMUI.button(IMStrings.text("empty") + "\n" + IMStrings.text("reload")) {
                self?.perform {
                    await self?.reload()
                }
            }
        }
        table.refreshControl = UIRefreshControl.jobsMake { control in
            control.addTarget(self, action: #selector(requestReload), for: .valueChanged)
        }
        requestReload()
    }

    @objc private func requestReload() {
        Task {
            await reload()
        }
    }
    private func reload() async {
        let namespace = session.namespace
        if !session.demo {
            do {
                let value = try await IMMessageListService().conversation(conversation["id"].text)
                guard session.namespace == namespace else { return }
                conversation = value
            } catch {
                if let denied = error as? IMAPIError, [401, 403, 404].contains(denied.status) {
                    conversation = conversation.setting("readOnly", .bool(true))
                    members = []
                }
            }
            let capabilities = try? await session.action("capabilities", method: "GET")
            guard session.namespace == namespace else { return }
            announcementEnabled =
                capabilities?["features"]["announcements"] == .bool(true)
                && conversation["kind"].text == "group"
            announcementRoleProtected = capabilities?["features"]["role_admission"] == .bool(true)
            if announcementEnabled {
                announcementPending = (try? announcements.pending(conversation["id"].text)) != nil
                if announcementPending {
                    _ = try? await announcements.checkOriginal(conversation["id"].text)
                }
                announcement = (try? await announcements.load(conversation["id"].text)) ?? .null
                announcementPending = (try? announcements.pending(conversation["id"].text)) != nil
            } else {
                announcement = .null
            }
        }
        members = await session.list("conversations/\(conversation["id"].text)/members")
        guard session.namespace == namespace else { return }
        table.reloadData()
        table.byReloadEmptyViewAuto()
        table.refreshControl?.endRefreshing()
    }

    func numberOfSections(in tableView: UITableView) -> Int {
        2
    }

    func tableView(_ tableView: UITableView, numberOfRowsInSection section: Int) -> Int {
        section == 0 ? actions.count : members.count
    }

    func tableView(_ tableView: UITableView, titleForHeaderInSection section: Int) -> String? {
        IMStrings.text(section == 0 ? "settings" : "members")
    }

    func tableView(_ tableView: UITableView, cellForRowAt indexPath: IndexPath) -> UITableViewCell {
        let cell = UITableViewCell.make(style: .subtitle, reuseIdentifier: nil)
        if indexPath.section == 0 {
            cell.byText(IMStrings.text(actions[indexPath.row]))
        } else {
            let member = members[indexPath.row]
            let memberCell =
                tableView.dequeueReusableCell(withIdentifier: "member", for: indexPath) as! IMConversationCell
            memberCell.configure(
                member.setting(
                    "lastMessage",
                    .string(IMStrings.text(member["id"] == conversation["ownerId"] ? "owner" : "member"))))
            return memberCell
        }
        return cell
    }

    func tableView(_ tableView: UITableView, didSelectRowAt indexPath: IndexPath) {
        tableView.deselectRow(at: indexPath, animated: true)
        let path = "conversations/" + conversation["id"].text
        if indexPath.section == 1 {
            let member = members[indexPath.row]
            guard isOwner, member["id"] != session.user["id"] else {
                return
            }
            let sheet = UIAlertController.make(
                title: member.firstText("displayName", "username"), message: nil, style: .actionSheet)
            sheet.addAction(
                UIAlertAction.make(IMStrings.text("transferOwner"), style: .destructive) { [weak self] in
                    guard let self else {
                        return
                    }
                    self.perform {
                        _ = try await self.session.action(
                            path, method: "PATCH", body: ["ownerId": member["id"]])
                        await self.reload()
                    }
                })
            sheet.addAction(
                UIAlertAction.make(IMStrings.text("removeMember"), style: .destructive) { [weak self] in
                    self?.perform {
                        _ = try await self?.session.action(
                            path + "/members/" + member["id"].text + "?membershipGeneration="
                                + member["membershipGeneration"].text, method: "DELETE")
                        await self?.reload()
                    }
                })
            sheet.addAction(UIAlertAction.make(IMStrings.text("cancel"), style: .cancel) {})
            sheet.popoverPresentationController?.sourceView = tableView.cellForRow(at: indexPath)
            present(sheet, animated: true)
        } else if actions[indexPath.row] == "rename" {
            IMUI.prompt(self, title: IMStrings.text("rename"), value: conversation["title"].text) {
                [weak self] name in
                self?.perform {
                    _ = try await self?.session.action(path, method: "PATCH", body: ["title": .string(name)])
                    self?.jobsSetupGKNav(title: name)
                    await self?.reload()
                }
            }
        } else if actions[indexPath.row] == "addMember" {
            IMUI.prompt(self, title: IMStrings.text("search")) { [weak self] query in
                self?.perform {
                    guard let self else {
                        return
                    }
                    let people = await self.session.contactsSnapshot().filter {
                        query.isEmpty
                            || $0.firstText("displayName", "username").localizedCaseInsensitiveContains(query)
                    }
                    let sheet = UIAlertController.make(
                        title: IMStrings.text("addMember"), message: nil, style: .actionSheet)
                    for person in people {
                        sheet.addAction(
                            UIAlertAction.make(person.firstText("displayName", "username")) { [weak self] in
                                self?.perform {
                                    _ = try await self?.session.action(
                                        path + "/members", body: ["userId": person["id"]])
                                    await self?.reload()
                                }
                            })
                    }
                    sheet.addAction(UIAlertAction.make(IMStrings.text("cancel"), style: .cancel) {})
                    sheet.popoverPresentationController?.sourceView = self.view
                    self.present(sheet, animated: true)
                }
            }
        } else if actions[indexPath.row] == "announcement" {
            showAnnouncement()
        } else if actions[indexPath.row] == "clearAnnouncement" {
            let version = announcement["version"].text
            IMUI.confirm(
                self, title: IMStrings.text("clearAnnouncement"), message: conversation["title"].text
            ) { [weak self] in
                self?.perform { [weak self] in
                    guard let self else { return }
                    _ = try await self.announcements.clear(
                        self.conversation["id"].text, expectedVersion: version)
                    await self.reload()
                }
            }
        } else if actions[indexPath.row] == "reconcileOriginal" {
            perform { [weak self] in
                guard let self else { return }
                _ = try await self.announcements.checkOriginal(self.conversation["id"].text)
                await self.reload()
            }
        } else if actions[indexPath.row] == "report" {
            IMReportVC.present(on: self, type: "conversation", id: conversation["id"].text)
        } else {
            let confirm = UIAlertController.make(
                title: IMStrings.text("leave"), message: conversation["title"].text, style: .actionSheet)
            confirm.addAction(UIAlertAction.make(IMStrings.text("cancel"), style: .cancel) {})
            confirm.addAction(
                UIAlertAction.make(IMStrings.text("leave"), style: .destructive) { [weak self] in
                    self?.perform {
                        guard let self else {
                            return
                        }
                        _ = try await self.session.action(
                            path + "/members/" + self.session.user["id"].text + "?membershipGeneration="
                                + (self.members.first { $0["id"] == self.session.user["id"] }?[
                                    "membershipGeneration"
                                ].text ?? ""), method: "DELETE")
                        self.navigationController?.popViewController(animated: true)
                    }
                })
            confirm.popoverPresentationController?.sourceView = view
            present(confirm, animated: true)
        }
    }

    private func showAnnouncement() {
        perform { [weak self] in
            guard let self else { return }
            let namespace = self.session.namespace
            let value = try await self.announcements.load(self.conversation["id"].text)
            guard self.session.namespace == namespace else { return }
            self.announcement = value
            let active = value["state"].text == "active"
            let stateKey =
                value["state"].text == "expired"
                ? "announcementExpired"
                : value["state"].text == "empty" ? "announcementEmpty" : "announcementUnavailable"
            let preview =
                active ? String(value["message"]["text"].text.prefix(600)) : IMStrings.text(stateKey)
            let dialog = UIAlertController.make(
                title: IMStrings.text("announcement"), message: preview, style: .alert)
            if active {
                dialog.addAction(
                    UIAlertAction.make(IMStrings.text("openSource")) { [weak self] in
                        self?.perform { [weak self] in
                            guard let self else { return }
                            let source = try await IMMessageListService().locate(value["message"])
                            guard self.session.namespace == namespace,
                                source.message.firstText("contentVersion", "version")
                                    == value["sourceVersion"].text,
                                source.message["deleted"] != .bool(true),
                                source.message["contentUnavailable"] != .bool(true)
                            else {
                                throw IMSession.SessionError.invalidInstance
                            }
                            self.navigationController?.pushViewController(
                                IMChatVC(
                                    conversation: source.conversation,
                                    focusMessageID: source.message["id"].text), animated: true)
                        }
                    })
            }
            dialog.addAction(UIAlertAction.make(IMStrings.text("done"), style: .cancel) {})
            self.present(dialog, animated: true)
        }
    }
}
