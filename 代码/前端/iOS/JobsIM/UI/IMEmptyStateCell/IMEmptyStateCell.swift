//
//  IMEmptyStateCell.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import JobsByUIKit
import JobsSwiftDSL
import SnapKit
import UIKit

@MainActor
final class IMEmptyStateCell: UITableViewCell {
    private var reload: (() -> Void)?
    private lazy var empty = IMUI.emptyButton { [weak self] in
        self?.reload?()
    }

    override init(style: UITableViewCell.CellStyle, reuseIdentifier: String?) {
        super.init(style: style, reuseIdentifier: reuseIdentifier)
        byBackgroundColor(IMUI.panel)
            .byIMThemeBackground(normal: IMUI.panel, selected: IMUI.selection)
            .bySelectionStyle(.none)
        empty.byAddTo(contentView) { make in
            make.edges.equalToSuperview().inset(16)
            make.height.greaterThanOrEqualTo(136)
        }
    }

    required init?(coder: NSCoder) {
        fatalError("init(coder:) is unavailable")
    }

    static func make(reload: @escaping () -> Void) -> IMEmptyStateCell {
        let cell = IMEmptyStateCell(style: .default, reuseIdentifier: nil)
        cell.reload = reload
        return cell
    }
}
