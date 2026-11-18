//
//  IMGroupGovernanceVC.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月17日，星期二.
//

import Foundation
import JobsByUIKit
import JobsIMCore
import JobsIMTransport
import JobsSwiftDSL
import SnapKit
import UIKit

@MainActor
final class IMGroupGovernanceVC: IMBaseVC, UITableViewDataSource, UITableViewDelegate {

    private let conversation: String
    private let service = IMGroupGovernanceService()
    private var observed: IMJSON = .null
    private var members: [IMJSON] = []
    private var invitations: [IMJSON] = []
    private var applications: [IMJSON] = []
    private var cursors = ["", "", ""]
    private var original = false
    private var task: Task<Void, Never>?
    private var namespace: IMNamespace?
    private var manager: Bool {
        return ["owner", "admin"].contains(observed["role"].text) && observed["state"].text == "active"
    }
    private var actions: [String] {
        if conversation.isEmpty {
            return ["joinGroup", "groupJoinHelp"]
        }
        var keys = ["reload", "groupPolicyHelp"]
        if original {
            keys += ["checkGroupOriginal", "retryGroupOriginal"]
        } else if observed["canRecover"] == .bool(true) {
            keys += ["recoverGroup"]
        } else if observed["role"].text == "owner" && observed["state"].text == "active" {
            keys += ["groupMuteAll", "groupSlowSeconds", "groupCapacity", "createInvite", "dissolveGroup"]
        } else if manager {
            keys += ["createInvite"]
        }
        return keys
    }
    private lazy var table = UITableView.make(style: .insetGrouped)
        .byDelegate(self)
        .byDataSource(self)
        .byBackgroundColor(IMUI.background)

    init(conversation: String = "") {
        self.conversation = conversation
        super.init(nibName: nil, bundle: nil)
    }

    required init?(coder: NSCoder) {
        fatalError("init(coder:) is unavailable")
    }

    override func viewDidLoad() {
        screenTitle = IMStrings.text(conversation.isEmpty ? "joinGroup" : "groupGovernance")
        super.viewDidLoad()
        namespace = session.namespace
        table.rowHeight = UITableView.automaticDimension
        table.estimatedRowHeight = 68
        table.byAddTo(content) { make in
            make.edges.equalToSuperview()
        }
        run { [weak self] in
            try await self?.reload()
        }
    }

    override func viewDidDisappear(_ animated: Bool) {
        super.viewDidDisappear(animated)
        task?.cancel()
    }

    private func run(_ action: @escaping @MainActor () async throws -> Void) {
        guard task == nil, namespace == session.namespace else {
            return
        }
        table.isUserInteractionEnabled = false
        task = Task { [weak self] in
            guard let self else {
                return
            }
            defer {
                self.task = nil
                self.table.isUserInteractionEnabled = true
            }
            do {
                try await action()
            } catch {
                if !Task.isCancelled && self.namespace == self.session.namespace {
                    IMUI.alert(self, message: error.localizedDescription)
                }
            }
            if !Task.isCancelled && self.namespace == self.session.namespace {
                self.original = (try? self.service.pending(self.conversation)) != nil
                self.table.reloadData()
            }
        }
    }

    private func reload() async throws {
        guard !conversation.isEmpty else {
            return
        }
        let value = try await service.load(conversation)
        guard namespace == session.namespace, !Task.isCancelled else {
            return
        }
        observed = value
        original = try service.pending(conversation) != nil
        members = []
        invitations = []
        applications = []
        cursors = ["", "", ""]
        if ["active", "owner_unavailable"].contains(observed["state"].text) {
            try await page(0)
            if manager {
                try await page(1)
                try await page(2)
            }
        }
        table.reloadData()
    }

    private func page(_ index: Int) async throws {
        let suffix = ["members", "invites", "join-requests"][index]
        let value = try await service.read(conversation, suffix: suffix + "?limit=50&after=" + cursors[index])
        guard namespace == session.namespace, !Task.isCancelled else {
            return
        }
        cursors[index] = value["nextCursor"].text
        if index == 0 {
            members += value["items"].list
        } else if index == 1 {
            invitations += value["items"].list
        } else {
            applications += value["items"].list
        }
    }

    func numberOfSections(in tableView: UITableView) -> Int {
        return conversation.isEmpty ? 1 : manager ? 4 : 2
    }

    func tableView(_ tableView: UITableView, titleForHeaderInSection section: Int) -> String? {
        return section == 0 ? nil : IMStrings.text(["members", "groupInvites", "groupApplications"][section - 1])
    }

    func tableView(_ tableView: UITableView, numberOfRowsInSection section: Int) -> Int {
        guard section > 0 else {
            return actions.count
        }
        let count = [members.count, invitations.count, applications.count][section - 1]
        return max(count, 1) + (cursors[section - 1].isEmpty ? 0 : 1)
    }

    func tableView(_ tableView: UITableView, cellForRowAt indexPath: IndexPath) -> UITableViewCell {
        let cell = UITableViewCell.make(style: .subtitle, reuseIdentifier: nil)
        cell.textLabel?.numberOfLines = 0
        cell.detailTextLabel?.numberOfLines = 0
        if indexPath.section == 0 {
            let key = actions[indexPath.row]
            cell.byText(IMStrings.text(key))
            if key == "groupMuteAll" {
                cell.bySecondaryText(IMStrings.text(observed["policy"]["muteAll"] == .bool(true) ? "enabled" : "disabled"))
            } else if key == "groupSlowSeconds" {
                cell.bySecondaryText(observed["policy"]["slowSeconds"].text + " s")
            } else if key == "groupCapacity" {
                cell.bySecondaryText(observed["policy"]["maxMembers"].text)
            }
            return cell
        }
        let rows = [members, invitations, applications][indexPath.section - 1]
        if rows.isEmpty {
            return IMEmptyStateCell.make { [weak self] in
                self?.run { [weak self] in
                    try await self?.reload()
                }
            }
        } else if indexPath.row == rows.count {
            cell.byText(IMStrings.text("loadMore"))
        } else if indexPath.section == 1 {
            let value = rows[indexPath.row]
            cell.byText(value.firstText("displayName", "username"))
                .bySecondaryText(IMStrings.text(value["role"].text == "admin" ? "groupAdmin" : value["role"].text == "owner" ? "owner" : "member"))
        } else if indexPath.section == 2 {
            let value = rows[indexPath.row]
            cell.byText(value["expiresAt"].text)
                .bySecondaryText(value["uses"].text + " / " + value["maxUses"].text + " · " + IMStrings.text(value["revokedAt"] == .null ? "approvalRequired" : "inviteRevoked"))
        } else {
            let value = rows[indexPath.row]
            cell.byText(value.firstText("displayName", "username"))
                .bySecondaryText(IMStrings.text(value["state"].text == "pending" ? "groupRequestPending" : value["state"].text == "approved" ? "groupRequestApproved" : "groupRequestRejected"))
        }
        return cell
    }

    func tableView(_ tableView: UITableView, didSelectRowAt indexPath: IndexPath) {
        tableView.deselectRow(at: indexPath, animated: true)
        if indexPath.section == 0 {
            action(actions[indexPath.row])
            return
        }
        let rows = [members, invitations, applications][indexPath.section - 1]
        if rows.isEmpty {
            run { [weak self] in
                try await self?.reload()
            }
        } else if indexPath.row == rows.count {
            run { [weak self] in
                try await self?.page(indexPath.section - 1)
            }
        } else if indexPath.section == 1 {
            memberActions(rows[indexPath.row], anchor: tableView.cellForRow(at: indexPath))
        } else if indexPath.section == 2 {
            invitationActions(rows[indexPath.row], anchor: tableView.cellForRow(at: indexPath))
        } else if rows[indexPath.row]["state"].text == "pending" {
            let row = rows[indexPath.row]
            choose([(IMStrings.text("accept"), { [weak self] in
                self?.run {
                    try await self?.service.decide(self?.conversation ?? "", observed: self?.observed ?? .null, requestID: row["id"].text, approve: true)
                    try await self?.reload()
                }
            }), (IMStrings.text("reject"), { [weak self] in
                self?.run {
                    try await self?.service.decide(self?.conversation ?? "", observed: self?.observed ?? .null, requestID: row["id"].text, approve: false)
                    try await self?.reload()
                }
            })], anchor: tableView.cellForRow(at: indexPath))
        }
    }

    private func action(_ key: String) {
        if key == "joinGroup" {
            IMUI.prompt(self, title: IMStrings.text("joinGroup")) { [weak self] code in
                self?.run {
                    _ = try await self?.service.join(code.trimmingCharacters(in: .whitespacesAndNewlines))
                    if let self {
                        IMUI.alert(self, message: IMStrings.text("groupRequestPending"))
                    }
                }
            }
        } else if key == "recoverGroup" {
            let choices = members.filter { $0["id"] != self.session.user["id"] || $0["state"].text != "tombstoned" }
                .filter { $0["state"].text.isEmpty || $0["state"].text == "active" }
                .map { member in
                    return (member.firstText("displayName", "username"), { [weak self] in
                        guard let self else {
                            return
                        }
                        let snapshot = self.observed
                        IMUI.prompt(self, title: IMStrings.text("authOTP"), secure: true) { otp in
                            self.run {
                                _ = try await self.service.recover(self.conversation, observed: snapshot, target: member, otp: otp)
                                try await self.reload()
                            }
                        }
                    })
                }
            choose(choices, anchor: view)
        } else if key == "reload" {
            run { [weak self] in
                try await self?.reload()
            }
        } else if key == "checkGroupOriginal" || key == "retryGroupOriginal" {
            IMUI.confirm(self, title: IMStrings.text(key), message: IMStrings.text("groupPending")) { [weak self] in
                self?.run {
                    guard let self else {
                        return
                    }
                    if key == "retryGroupOriginal",
                        try self.service.pending(self.conversation)?["payload"]["action"].text == "recover" {
                        IMUI.prompt(self, title: IMStrings.text("authOTP"), secure: true) { otp in
                            self.run {
                                _ = try await self.service.reconcile(self.conversation, retry: true, otp: otp)
                                try await self.reload()
                            }
                        }
                    } else {
                        _ = try await self.service.reconcile(self.conversation, retry: key == "retryGroupOriginal")
                        try await self.reload()
                    }
                }
            }
        } else if key == "groupPolicyHelp" || key == "groupJoinHelp" {
            IMUI.alert(self, message: IMStrings.text(key))
        } else if key == "createInvite" {
            IMUI.prompt(self, title: IMStrings.text("inviteUses"), value: "1") { [weak self] text in
                guard let uses = Int(text), (1...10000).contains(uses), let self else {
                    return
                }
                self.run {
                    let value = try await self.service.create(self.conversation, route: "invites", body: [
                        "version": self.observed["version"], "membershipGeneration": self.observed["membershipGeneration"],
                        "roleGeneration": self.observed["roleGeneration"], "expiresAt": .string(ISO8601DateFormatter.make().string(from: Date().addingTimeInterval(7 * 86400))),
                        "maxUses": .number(Decimal(uses)), "approvalRequired": .bool(true),
                    ])
                    try await self.reload()
                    self.invitationActions(value, anchor: self.view)
                }
            }
        } else if key == "dissolveGroup" {
            IMUI.confirm(self, title: IMStrings.text(key), message: IMStrings.text("groupDissolveHelp")) { [weak self] in
                self?.change("dissolve")
            }
        } else if key == "groupMuteAll" {
            settings(mute: observed["policy"]["muteAll"] != .bool(true))
        } else {
            let field = key == "groupCapacity" ? "maxMembers" : "slowSeconds"
            IMUI.prompt(self, title: IMStrings.text(key), value: observed["policy"][field].text) { [weak self] text in
                guard let self, let value = Int(text),
                    (field == "maxMembers" ? 2...10000 : 0...3600).contains(value) else {
                    return
                }
                self.settings(slow: field == "slowSeconds" ? value : nil, capacity: field == "maxMembers" ? value : nil)
            }
        }
    }

    private func settings(mute: Bool? = nil, slow: Int? = nil, capacity: Int? = nil) {
        change("settings", values: ["muteAll": .bool(mute ?? (observed["policy"]["muteAll"] == .bool(true))),
                                    "slowSeconds": slow.map { .number(Decimal($0)) } ?? observed["policy"]["slowSeconds"],
                                    "maxMembers": capacity.map { .number(Decimal($0)) } ?? observed["policy"]["maxMembers"]])
    }

    private func change(_ action: String, values: [String: IMJSON] = [:], target: IMJSON? = nil) {
        let snapshot = observed
        run { [weak self] in
            guard let self else {
                return
            }
            _ = try await self.service.change(self.conversation, observed: snapshot, action: action, changes: values, target: target)
            try await self.reload()
        }
    }

    private func memberActions(_ member: IMJSON, anchor: UIView?) {
        guard manager, !original, member["id"] != session.user["id"], member["role"].text != "owner" else {
            return
        }
        var choices: [(String, () -> Void)] = []
        if observed["role"].text == "owner" {
            choices.append((IMStrings.text(member["role"].text == "admin" ? "makeMember" : "makeAdmin"), { [weak self] in
                self?.change("role", values: ["role": .string(member["role"].text == "admin" ? "member" : "admin")], target: member)
            }))
            choices.append((IMStrings.text("transferOwner"), { [weak self] in
                guard let self else {
                    return
                }
                IMUI.confirm(self, title: IMStrings.text("transferOwner"), message: member.firstText("displayName", "username")) {
                    self.change("transfer", target: member)
                }
            }))
        }
        if member["role"].text != "admin" || observed["role"].text == "owner" {
            choices.append((IMStrings.text("muteMember"), { [weak self] in
                self?.change("mute", values: ["mutedUntil": .string(ISO8601DateFormatter.make().string(from: Date().addingTimeInterval(3600)))], target: member)
            }))
            choices.append((IMStrings.text("unmuteMember"), { [weak self] in
                self?.change("mute", values: ["mutedUntil": .string("")], target: member)
            }))
        }
        choose(choices, anchor: anchor)
    }

    private func invitationActions(_ invite: IMJSON, anchor: UIView?) {
        choose([(IMStrings.text("copyInvite"), { [weak self] in
            guard let self, let code = try? self.service.invitationCode(self.conversation, invite: invite) else {
                return
            }
            UIPasteboard.general.string = code
            IMUI.alert(self, message: IMStrings.text("copied"))
        }), (IMStrings.text("revokeInvite"), { [weak self] in
            self?.run {
                try await self?.service.revoke(self?.conversation ?? "", observed: self?.observed ?? .null, invite: invite["id"].text)
                try await self?.reload()
            }
        })], anchor: anchor)
    }

    private func choose(_ choices: [(String, () -> Void)], anchor: UIView?) {
        let sheet = UIAlertController.make(title: screenTitle, message: nil, style: .actionSheet)
        for (title, action) in choices {
            sheet.addAction(UIAlertAction.make(title) {
                action()
            })
        }
        sheet.addAction(UIAlertAction.make(IMStrings.text("cancel"), style: .cancel) {})
        sheet.popoverPresentationController?.sourceView = anchor ?? view
        present(sheet, animated: true)
    }
}
