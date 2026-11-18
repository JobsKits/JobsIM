//
//  IMReportMediaPreviewVC.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import JobsByUIKit
import JobsSwiftDSL
import QuickLook
import SnapKit
import UIKit

@MainActor
final class IMReportMediaPreviewVC: IMBaseVC {

    private let preview: QLPreviewController
    private lazy var close = IMUI.button(IMStrings.text("done")) { [weak self] in
        self?.dismiss(animated: true)
    }
    .byAccessibilityIdentifier("report.preview.close")
    .byTitleFont(IMUI.font(.body))
    .byNumberOfLines(1)
    .byAddConstraintsClosure { make in
        // 文字按钮不能沿用导航图标的44点方框，否则“完成”会被拆成两行。
        make.width.greaterThanOrEqualTo(68)
        make.height.equalTo(44)
    }

    init(dataSource: QLPreviewControllerDataSource) {
        preview = QLPreviewController.make(dataSource: dataSource)
        super.init(nibName: nil, bundle: nil)
    }

    required init?(coder: NSCoder) {
        fatalError("init(coder:) is unavailable")
    }

    override func viewDidLoad() {
        screenTitle = IMStrings.text("reportMaterials")
        rightActions = [close]
        super.viewDidLoad()
        // QuickLook嵌入应用导航，保证模板隐藏系统栏时仍有明确关闭入口。
        addChild(preview)
        preview.view.byAddTo(content) {
            $0.edges.equalToSuperview()
        }
        preview.didMove(toParent: self)
    }
}
