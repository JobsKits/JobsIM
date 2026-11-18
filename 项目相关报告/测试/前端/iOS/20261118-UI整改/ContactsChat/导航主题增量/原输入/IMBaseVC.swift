//
//  IMBaseVC.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import UIKit

import JobsByUIKit
import JobsInheritance
import JobsSwiftBaseDefines
import JobsSwiftDSL

import GKNavigationBarSwift
import SnapKit

@MainActor
class IMBaseVC: BaseVC {

    let session = IMSession.shared
    var screenTitle = "JobsIM"
    var hidesScreenHeader = false
    var rightActions: [UIButton] = []
    private var syncObservation: UUID?
    lazy var content = UIView.jobsMake { container in
        container.byBackgroundColor(IMUI.background)
    }

    override func viewDidLoad() {
        super.viewDidLoad()
        view.byBackgroundColor(IMUI.background)
        let root = navigationController?.viewControllers.first === self
        jobsSetupGKNav(
            title: screenTitle, leftButton: root ? UIButton.sys().byHidden(true) : nil,
            rightButtons: rightActions)
        applyNavigationAppearance()
        gk_navigationBar.byHidden(hidesScreenHeader)
        content.byAddTo(view) { [unowned self] make in
            if self.hidesScreenHeader {
                make.top.equalTo(self.view.safeAreaLayoutGuide.snp.top).offset(self.session.demo ? 32 : 0)
            } else {
                make.top.equalTo(self.gk_navigationBar.snp.bottom)
            }
            make.left.right.bottom.equalTo(view.safeAreaLayoutGuide)
        }
        if hidesScreenHeader && session.demo {
            IMUI.label(IMStrings.text("currentlyDemo"), style: .footnote, color: IMUI.green)
                .byTextAlignment(.center)
                .byBackgroundColor(IMUI.panel)
                .byAccessibilityIdentifier("root.demoBanner")
                .byAddTo(view) { [unowned self] make in
                    make.top.leading.trailing.equalTo(self.view.safeAreaLayoutGuide)
                    make.height.equalTo(32)
                }
        }
    }

    override func viewWillAppear(_ animated: Bool) {
        super.viewWillAppear(animated)
        applyNavigationAppearance()
        if let syncObservation {
            session.removeSyncObserver(syncObservation)
        }
        syncObservation = session.observeSync { [weak self] in
            self?.syncProjectionChanged()
        }
    }

    override func viewDidDisappear(_ animated: Bool) {
        super.viewDidDisappear(animated)
        if presentedViewController == nil, let syncObservation {
            session.removeSyncObserver(syncObservation)
            self.syncObservation = nil
        }
    }

    func syncProjectionChanged() {
    }

    private func applyNavigationAppearance() {
        if navigationController?.viewControllers.first === self {
            gk_navLeftBarButtonItem = nil
        }
        gk_navBackgroundColor = IMUI.background
        gk_navTitleColor = IMUI.foreground
        gk_navigationBar.tintColor = IMUI.green
    }

    override func traitCollectionDidChange(_ previousTraitCollection: UITraitCollection?) {
        super.traitCollectionDidChange(previousTraitCollection)
        JobsThemeCenter.shared.setStyle(traitCollection.userInterfaceStyle == .dark ? .dark : .light)
        applyNavigationAppearance()
    }

    func perform(_ work: @escaping @MainActor () async throws -> Void) {
        Task { [weak self] in
            do {
                try await work()
            } catch {
                guard let self else {
                    return
                }
                IMUI.alert(self, message: error.localizedDescription)
            }
        }
    }
}
