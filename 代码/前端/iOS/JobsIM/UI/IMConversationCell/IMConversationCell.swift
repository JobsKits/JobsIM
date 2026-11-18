//
//  IMConversationCell.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import UIKit

import JobsByUIKit
import JobsIMCore
import JobsSwiftBaseDefines
import JobsSwiftDSL

import SnapKit

@MainActor
final class IMConversationCell: UITableViewCell {

    private lazy var avatar = IMAvatarView.make()
    private lazy var name = IMUI.label("", style: .headline)
    private lazy var summary = IMUI.label("", style: .subheadline, color: IMUI.secondary)
    private lazy var meta = IMUI.label("", style: .caption1, color: IMUI.secondary)
    private lazy var badge = IMUI.label("", style: .caption1, color: JobsCor.white)

    override init(style: UITableViewCell.CellStyle, reuseIdentifier: String?) {
        super.init(style: style, reuseIdentifier: reuseIdentifier)
        byBackgroundColor(IMUI.background)
        avatar
            .byBackgroundColor(IMUI.panel)
            .byCornerRadius(22)
            .byClipsToBounds(true)
            .byAddTo(contentView) { [unowned self] make in
                make.leading.equalToSuperview().inset(24)
                make.centerY.equalToSuperview()
                make.width.height.equalTo(54)
            }
        meta.byAddTo(contentView) { [unowned self] make in
            make.trailing.equalToSuperview().inset(24)
            make.top.equalToSuperview().inset(20)
        }
        name.byAddTo(contentView) { [unowned self] make in
            make.leading.equalTo(avatar.snp.trailing).offset(16)
            make.trailing.lessThanOrEqualTo(meta.snp.leading).offset(-12)
            make.top.equalToSuperview().inset(18)
        }
        summary
            .byNumberOfLines(1)
            .byAddTo(contentView) { [unowned self] make in
                make.leading.equalTo(name)
                make.trailing.equalToSuperview().inset(52)
                make.top.equalTo(name.snp.bottom).offset(7)
                make.bottom.equalToSuperview().inset(18).priority(999)
            }
        badge
            .byBackgroundColor(IMUI.greenFill)
            .byTextAlignment(.center)
            .byCornerRadius(10)
            .byClipsToBounds(true)
            .byAddTo(contentView) { [unowned self] make in
                make.trailing.equalTo(meta)
                make.top.equalTo(meta.snp.bottom).offset(10)
                make.width.greaterThanOrEqualTo(20)
                make.height.equalTo(20)
            }
    }

    required init?(coder: NSCoder) {
        fatalError("init(coder:) is unavailable")
    }

    func configure(_ item: IMJSON) {
        accessibilityIdentifier = "conversation." + item["id"].text
        let title = item.firstText("title", "displayName", "username", "text")
        name.byText(title)
        avatar.show(item)
        let preview =
            item["lastMessage"].text.isEmpty ? item["lastMessage"]["text"].text : item["lastMessage"].text
        summary.byText(preview.isEmpty ? item.firstText("username", "kind", "text") : preview)
        let unread = item["unreadCount"].text
        badge
            .byText(unread)
            .byHidden(unread.isEmpty || unread == "0")
        meta.byText(
            item["updatedAt"].text.count >= 16 ? String(item["updatedAt"].text.dropFirst(11).prefix(5)) : "")
        accessibilityLabel = [title, summary.text, meta.text].compactMap {
            $0
        }.joined(separator: ", ")
    }
}
