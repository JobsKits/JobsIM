//
//  IMBaseVC.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import GKNavigationBarSwift
import JobsByUIKit
import JobsInheritance
import JobsSwiftBaseDefines
import JobsSwiftDSL
import SnapKit
import UIKit

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
                make.top.equalTo(self.view.safeAreaLayoutGuide.snp.top)
            } else {
                make.top.equalTo(self.gk_navigationBar.snp.bottom)
            }
            make.left.right.bottom.equalTo(view.safeAreaLayoutGuide)
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

    override func viewDidAppear(_ animated: Bool) {
        super.viewDidAppear(animated)
        // The template reapplies its default GK palette here; the app's effective appearance wins last.
        applyNavigationAppearance()
        (UIApplication.shared.delegate as? AppDelegate)?.refreshDemoOverlay()
    }

    override var preferredStatusBarStyle: UIStatusBarStyle {
        return traitCollection.userInterfaceStyle == .dark ? .lightContent : .darkContent
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
        byIMNavigationAppearance(hidesHeader: hidesScreenHeader)
    }

    override func traitCollectionDidChange(_ previousTraitCollection: UITraitCollection?) {
        super.traitCollectionDidChange(previousTraitCollection)
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
