//
//  IMDemoEnvironmentOverlay.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import JobsByUIKit
import JobsSwiftBaseDefines
import JobsSwiftDSL
import UIKit

@MainActor
final class IMDemoEnvironmentOverlay: UIView {
    private var relativePosition = CGPoint(x: 1, y: 0.8)
    private var dragOrigin = CGPoint.zero
    private var keyboardFrame: CGRect?
    private lazy var button = UIButton.sys()
        .byTitleFont(IMUI.font(.caption1))
        .byNumberOfLines(0)
        .byAccessibilityIdentifier("demo.environment")
        .onTap { _ in
            IMSession.shared.leaveDemonstrationEnvironment()
        }
    private lazy var dragGesture = UIPanGestureRecognizer.byConfig {
        [weak self] (gesture: UIPanGestureRecognizer) in
        self?.drag(gesture)
    }.byCancelsTouchesInView(true)

    override init(frame: CGRect) {
        super.init(frame: frame)
        byBackgroundColor(JobsCor.clear)
            .byAutoresizingMask([.flexibleWidth, .flexibleHeight])
        button.jobs_addGestureRetView(dragGesture).byAddTo(self)
    }

    required init?(coder: NSCoder) {
        fatalError("init(coder:) is unavailable")
    }

    static func make() -> IMDemoEnvironmentOverlay {
        return IMDemoEnvironmentOverlay(frame: .zero)
    }

    func attach(to window: UIWindow, visible: Bool) {
        guard visible else {
            removeFromSuperview()
            return
        }
        button.byConfiguration(
            UIButton.Configuration.filled()
                .byBaseForegroundColor(JobsCor.white)
                .byBaseBackgroundColor(IMUI.greenFill)
                .byCornerStyle(.capsule)
                .byTitle(IMStrings.text("demoEnvironment"))
                .byContentInsets(NSDirectionalEdgeInsets(top: 10, leading: 16, bottom: 10, trailing: 16))
        )
        .byTitleFont(IMUI.font(.caption1))
        .byIMDemoAccessibility()
        byFrame(window.bounds)
        if superview !== window {
            byAddTo(window)
        }
        window.bringSubviewToFront(self)
        setNeedsLayout()
    }

    override func didMoveToWindow() {
        super.didMoveToWindow()
        NotificationCenter.default.removeObserver(self)
        if window != nil {
            NotificationCenter.default.addObserver(
                self, selector: #selector(keyboardChanged(_:)),
                name: UIResponder.keyboardWillChangeFrameNotification, object: nil)
        }
    }

    override func layoutSubviews() {
        super.layoutSubviews()
        guard let window else {
            return
        }
        let area = availableArea(in: window)
        button.byHidden(area.width < 80 || area.height < 44)
        let maximumWidth = min(220, max(44, area.width))
        let preferred = button.sizeThatFits(CGSize(width: maximumWidth, height: 80))
        let width = min(maximumWidth, max(100, preferred.width))
        let height = min(80, max(44, preferred.height))
        let limits = centerLimits(area: area, size: CGSize(width: width, height: height))
        button.byFrame(
            CGRect(
                x: limits.minX + limits.width * relativePosition.x - width / 2,
                y: limits.minY + limits.height * relativePosition.y - height / 2,
                width: width, height: height))
    }

    override func hitTest(_ point: CGPoint, with event: UIEvent?) -> UIView? {
        guard !isHidden, alpha > 0.01, !button.isHidden, button.frame.contains(point) else {
            return nil
        }
        return super.hitTest(point, with: event)
    }

    @objc private func keyboardChanged(_ notification: Notification) {
        guard let window,
            let screenFrame = notification.userInfo?[UIResponder.keyboardFrameEndUserInfoKey] as? CGRect
        else {
            return
        }
        let frame = window.convert(screenFrame, from: nil)
        keyboardFrame = frame.intersects(window.bounds) ? frame : nil
        setNeedsLayout()
    }

    private func availableArea(in window: UIWindow) -> CGRect {
        var area = bounds.inset(by: window.safeAreaInsets).insetBy(dx: 12, dy: 12)
        if let keyboardFrame, keyboardFrame.width >= bounds.width * 0.5 {
            area.size.height = max(0, min(area.maxY, keyboardFrame.minY - 12) - area.minY)
        }
        return area
    }

    private func centerLimits(area: CGRect, size: CGSize) -> CGRect {
        return CGRect(
            x: area.minX + size.width / 2, y: area.minY + size.height / 2,
            width: max(0, area.width - size.width), height: max(0, area.height - size.height))
    }

    private func drag(_ gesture: UIPanGestureRecognizer) {
        guard let window else {
            return
        }
        if gesture.state == .began {
            dragOrigin = button.center
        }
        let translation = gesture.translation(in: self)
        let limits = centerLimits(area: availableArea(in: window), size: button.bounds.size)
        let center = CGPoint(
            x: min(limits.maxX, max(limits.minX, dragOrigin.x + translation.x)),
            y: min(limits.maxY, max(limits.minY, dragOrigin.y + translation.y)))
        button.byCenter(center)
        relativePosition = CGPoint(
            x: limits.width > 0 ? (center.x - limits.minX) / limits.width : 0,
            y: limits.height > 0 ? (center.y - limits.minY) / limits.height : 0)
    }
}
