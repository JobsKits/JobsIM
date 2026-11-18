//
//  IMPersonalCenterVC.swift
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
final class IMPersonalCenterVC: IMBaseVC, UITableViewDataSource, UITableViewDelegate {
    private let rows = ["profile", "saved", "settings", "switchAccount"]
    private lazy var avatar = IMAvatarView.make()
    private lazy var displayName = IMUI.label("", style: .title1)
    private lazy var username = IMUI.label("", style: .subheadline, color: IMUI.secondary)
    private lazy var table = UITableView.make(style: .insetGrouped)
        .byDelegate(self).byDataSource(self).byBackgroundColor(IMUI.background)

    override func viewDidLoad() {
        hidesScreenHeader = true
        screenTitle = ""
        super.viewDidLoad()
        let heading = IMUI.stack(spacing: 12).byAlignment(.center)
        heading.byAddArrangedSubviews([avatar, displayName, username])
        avatar.snp.makeConstraints {
            $0.size.equalTo(84)
        }
        avatar.byCornerRadius(24).jobs_addGestureRetView(
            UILongPressGestureRecognizer.jobsMake {
                $0.byMinDuration(0.5).byTarget(self, action: #selector(openOwnProfile(_:)))
            })
        heading.byAddTo(content) { make in
            make.top.leading.trailing.equalToSuperview().inset(24)
        }
        table.byAddTo(content) { make in
            make.top.equalTo(heading.snp.bottom).offset(12)
            make.leading.trailing.bottom.equalToSuperview()
        }
    }

    override func viewWillAppear(_ animated: Bool) {
        super.viewWillAppear(animated)
        avatar.show(session.user)
        displayName.byText(session.user.firstText("displayName", "username"))
        let name = session.user["username"].text
        username.byText(name.isEmpty ? "" : "@" + name)
        table.byReloadData()
    }

    override func traitCollectionDidChange(_ previousTraitCollection: UITraitCollection?) {
        super.traitCollectionDidChange(previousTraitCollection)
        guard isViewLoaded,
              traitCollection.hasDifferentColorAppearance(comparedTo: previousTraitCollection) else {
            return
        }
        table.byReloadData()
    }

    func tableView(_ tableView: UITableView, numberOfRowsInSection section: Int) -> Int {
        return rows.count
    }

    func tableView(_ tableView: UITableView, cellForRowAt indexPath: IndexPath) -> UITableViewCell {
        IMUI.cellTheme(UITableViewCell.make(style: .default, reuseIdentifier: nil))
            .byText(IMStrings.text(rows[indexPath.row]))
            .byIMThemeAccessoryType(.disclosureIndicator)
            .byAccessibilityIdentifier("personal." + rows[indexPath.row])
    }

    func tableView(_ tableView: UITableView, didSelectRowAt indexPath: IndexPath) {
        tableView.deselectRow(at: indexPath, animated: true)
        let controller: UIViewController
        switch rows[indexPath.row] {
        case "profile":
            controller = IMUserProfileVC(user:
            session.user)
        case "saved":
            controller = IMListVC(kind:
            .saved)
        case "settings":
            controller = IMSettingsVC()
        default:
            chooseAccount()
            return
        }
        controller.byHidesBottomBarWhenPushed(true)
        navigationController?.pushViewController(controller, animated: true)
    }

    @objc private func openOwnProfile(_ gesture: UILongPressGestureRecognizer) {
        guard gesture.state == .began else {
            return
        }
        navigationController?.pushViewController(
            IMUserProfileVC(user: session.user).byHidesBottomBarWhenPushed(true), animated: true)
    }

    private func chooseAccount() {
        let sheet = UIAlertController.make(
            title: IMStrings.text("switchAccount"),
            message: IMStrings.text("accountSwitchHint"), style: .actionSheet)
        for account in session.savedAccountChoices {
            let key = account["id"].text
            guard !key.isEmpty else {
                continue
            }
            let name = account.firstText("displayName", "username")
            let username = account["username"].text
            let title = name == username ? name : name + " · @" + username
            sheet.addAction(
                UIAlertAction.make(title) { [weak self] in
                    self?.perform { [weak self] in
                        guard let self else {
                            return
                        }
                        try await self.session.activateSavedAccount(id: key)
                    }
                })
        }
        sheet.addAction(
            UIAlertAction.make(IMStrings.text("addAccount")) { [weak self] in
                self?.session.beginAccountSwitch()
            })
        sheet.addAction(UIAlertAction.make(IMStrings.text("cancel"), style: .cancel) {})
        sheet.popoverPresentationController?.sourceView = view
        present(sheet, animated: true)
    }
}
