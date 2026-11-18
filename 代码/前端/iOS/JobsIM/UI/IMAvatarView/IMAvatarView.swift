//
//  IMAvatarView.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import JobsByUIKit
import JobsIMCore
import JobsSwiftDSL
import SnapKit
import UIKit

@MainActor
final class IMAvatarView: UIView {
    private lazy var picture = UIImageView.jobsMake { image in
        image.byContentMode(.scaleAspectFill).byClipsToBounds(true)
    }
    private var loading: Task<Void, Never>?
    private var revision = UUID()

    override init(frame: CGRect) {
        super.init(frame: frame)
        byClipsToBounds(true).byCornerRadius(16)
        picture.byAddTo(self) {
            make in make.edges.equalToSuperview()
        }
        isAccessibilityElement = true
        accessibilityLabel = IMStrings.text("avatar")
        show(.null)
    }
    required init?(coder: NSCoder) {
        fatalError("init(coder:) is unavailable")
    }
    static func make() -> IMAvatarView {
        return IMAvatarView(frame: .zero)
    }
    func show(_ profile: IMJSON) {
        loading?.cancel()
        revision = UUID()
        picture.byImage(UIImage.make(named: "Logo"))
        accessibilityIdentifier = "avatar.fallback"
        let stamp = revision
        let ns = IMSession.shared.namespace
        let paths = [profile["avatarURL"].text, profile["avatarThumbnailURL"].text].filter {
            !$0.isEmpty
        }
        guard !paths.isEmpty else {
            return
        }
        let points = max(96, max(bounds.width, bounds.height))
        let pixels = min(512, max(32, Int(points * traitCollection.displayScale)))
        loading = Task { [weak self] in
            for path in paths {
                let image = await IMAvatarLoader.image(path, namespace: ns, maxPixelSize: pixels)
                guard let self, !Task.isCancelled, self.revision == stamp, IMSession.shared.namespace == ns
                else {
                    return
                }
                if let image {
                    self.picture.byImage(image)
                    self.accessibilityIdentifier = "avatar.remote"
                    return
                }
            }
        }
    }
}
