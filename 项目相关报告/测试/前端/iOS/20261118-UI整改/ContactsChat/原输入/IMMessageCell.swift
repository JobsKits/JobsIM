//
//  IMMessageCell.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import JobsByUIKit
import JobsIMCore
import JobsSwiftBaseDefines
import JobsSwiftDSL
import SnapKit
import UIKit

@MainActor
final class IMMessageCell: UITableViewCell {

    private lazy var avatar = IMAvatarView.make()
    private lazy var bubble = UIView.jobsMake { view in
        view.byCornerRadius(20)
    }
    private lazy var body = IMUI.label()
    private lazy var preview = UIImageView.jobsMake { view in
        view.byContentMode(.scaleAspectFill).byClipsToBounds(true).byCornerRadius(12)
    }
    private var previewTask: Task<Void, Never>?
    private var previewRevision = UUID()
    private lazy var detail = IMUI.label("", style: .caption2, color: IMUI.secondary)

    override init(style: UITableViewCell.CellStyle, reuseIdentifier: String?) {
        super.init(style: style, reuseIdentifier: reuseIdentifier)
        byBackgroundColor(IMUI.background)
        avatar.byAddTo(contentView) { make in
            make.top.equalToSuperview().inset(7)
            make.width.height.equalTo(32)
        }
        bubble.byAddTo(contentView) { [unowned self] make in
            make.top.equalToSuperview().inset(7)
            make.width.lessThanOrEqualTo(contentView.snp.width).multipliedBy(0.72)
        }
        let stack = IMUI.stack(spacing: 10)
        stack.byAddArrangedSubviews([preview, body])
        preview.snp.makeConstraints { make in
            make.width.equalTo(190)
            make.height.equalTo(150).priority(999)
        }
        stack.byAddTo(bubble) {
            make in make.edges.equalToSuperview().inset(12)
        }
        detail.byAddTo(contentView) { [unowned self] make in
            make.top.equalTo(bubble.snp.bottom).offset(5)
            make.bottom.equalToSuperview().inset(8)
        }
        bySelectionStyle(.none)
    }

    required init?(coder: NSCoder) {
        fatalError("init(coder:) is unavailable")
    }

    func configure(_ item: IMJSON, own: Bool, sender: IMJSON = .null) {
        avatar.show(sender)
        avatar.snp.remakeConstraints { make in
            make.top.equalToSuperview().inset(7)
            make.width.height.equalTo(32)
            if own {
                make.trailing.equalToSuperview().inset(12)
            } else {
                make.leading.equalToSuperview().inset(12)
            }
        }
        let deleted = item["deleted"] == .bool(true)
        let type = item["type"].text
        previewTask?.cancel()
        previewRevision = UUID()
        preview.byImage(nil).byHidden(true)
        if type == "image", !deleted, item["contentUnavailable"] != .bool(true),
            item["status"].text != "frozen"
        {
            let stamp = previewRevision
            let ns = IMSession.shared.namespace
            previewTask = Task { [weak self] in
                guard let image = await IMMediaLoader.image(item, namespace: ns), let self,
                    self.previewRevision == stamp
                else {
                    return
                }
                self.preview.byImage(image).byHidden(false).byAccessibilityIdentifier("message.image")
                self.preview.isAccessibilityElement = true
                self.preview.accessibilityLabel = IMStrings.text("image")
                // 图片到达后让自动高度重新计算，不改列表身份或滚动位置。
                if let table = self.superview as? UITableView {
                    table.beginUpdates()
                    table.endUpdates()
                }
            }
        }
        var text = item["text"].text
        if deleted {
            text = IMStrings.text("delete")
        } else if item["contentUnavailable"] == .bool(true) {
            text = IMStrings.text(
                IMForwardService.isForward(item) ? "forwardUnavailable" : "contentUnavailable")
        } else if item["type"].text == "forward" {
            text = IMStrings.text("forwardUnknown")
        } else if type != "text" {
            text =
                IMStrings.text(["image": "image", "video": "video", "audio": "audio"][type] ?? "file") + "\n"
                + text
        }
        if !item["replyToId"].text.isEmpty && !deleted {
            text = IMStrings.text("reply") + "\n" + text
        }
        body
            .byText(text)
            .byTextColor(own ? JobsCor.white : IMUI.foreground)
        bubble.byBackgroundColor(own ? IMUI.greenFill : IMUI.panel)
        bubble.snp.remakeConstraints { [unowned self] make in
            make.top.equalToSuperview().inset(7)
            make.width.lessThanOrEqualTo(contentView.snp.width).multipliedBy(0.72)
            if own {
                make.trailing.equalTo(avatar.snp.leading).offset(-8)
            } else {
                make.leading.equalTo(avatar.snp.trailing).offset(8)
            }
        }
        let time =
            item["createdAt"].text.count >= 16 ? String(item["createdAt"].text.dropFirst(11).prefix(5)) : ""
        let status = item["status"].text
        let translated =
            status == "frozen"
            ? IMStrings.text("sendAdmissionPaused")
            : status == "queued"
                ? IMStrings.text("unknown")
                : status == "demo"
                    ? IMStrings.text("demo")
                    : status == "rejected" ? IMStrings.text("error") : own ? IMStrings.text("accepted") : ""
        let reactions = item["reactions"].list.map {
            $0.firstText("emoji")
        }.joined(separator: " ")
        detail.byText(
            [
                time, translated, reactions,
                item["forwarded"] == .bool(true) ? IMStrings.text("forwarded") : "",
                item["mentionedMe"] == .bool(true) && !deleted && item["contentUnavailable"] != .bool(true)
                    ? IMStrings.text("mentionedYou") : "",
            ].filter {
                !$0.isEmpty
            }.joined(separator: "  "))
        detail.snp.remakeConstraints { [unowned self] make in
            make.top.equalTo(bubble.snp.bottom).offset(5)
            make.bottom.equalToSuperview().inset(8)
            if own {
                make.trailing.equalTo(bubble)
            } else {
                make.leading.equalTo(bubble)
            }
        }
    }
}
