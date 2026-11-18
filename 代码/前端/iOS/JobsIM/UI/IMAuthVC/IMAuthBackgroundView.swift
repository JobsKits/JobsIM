//
//  IMAuthBackgroundView.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import AVFoundation
import JobsByUIKit
import JobsSwiftBaseDefines
import JobsSwiftDSL
import SnapKit
import UIKit

@MainActor
final class IMAuthBackgroundView: UIView {

    enum Source {
        case image(named: String)
        case bundledImage(name: String, fileExtension: String, subdirectory: String?)
        case video(name: String, fileExtension: String, posterNamed: String)
    }

    private var player: AVPlayer?
    private var videoLayer: AVPlayerLayer?
    private var readiness: NSKeyValueObservation?
    private var observations: [NSObjectProtocol] = []
    private var active = false
    private lazy var image = UIImageView.jobsMake {
        $0.byContentMode(.scaleAspectFill)
            .byClipsToBounds(true)
    }
    private lazy var shade = UIView.jobsMake {
        $0.byBackgroundColor(UIColor.make(light: 0x163B2E, dark: 0x08150F).withAlphaComponent(0.24))
    }

    init(source: Source = .bundledImage(name: "AuthBackground", fileExtension: "jpg", subdirectory: "AuthMedia")) {
        super.init(frame: .zero)
        byClipsToBounds(true)
            .byUserInteractionEnabled(false)
            .byBackgroundColor(IMUI.background)
        image.byAddTo(self) {
            $0.edges.equalToSuperview()
        }
        configure(source)
        shade.byAddTo(self) {
            $0.edges.equalToSuperview()
        }
        for name in [UIApplication.didEnterBackgroundNotification, UIApplication.didBecomeActiveNotification,
                     UIAccessibility.reduceMotionStatusDidChangeNotification] {
            observations.append(NotificationCenter.default.addObserver(forName: name, object: nil, queue: .main) { [weak self] _ in
                Task { @MainActor [weak self] in
                    self?.updatePlayback()
                }
            })
        }
    }

    required init?(coder: NSCoder) {
        fatalError("init(coder:) has not been implemented")
    }

    isolated deinit {
        for observation in observations {
            NotificationCenter.default.removeObserver(observation)
        }
    }

    private func configure(_ source: Source) {
        switch source {
        /// 默认图片随应用打包，不进行网络素材请求。
        case .image(let name):
            image.byImage(UIImage.make(named: name))
        /// 同步目录可能保留资源子目录，也可能由Xcode资源阶段平铺。
        case .bundledImage(let name, let fileExtension, let subdirectory):
            if let url = bundledURL(name, fileExtension: fileExtension, subdirectory: subdirectory) {
                image.byImage(UIImage.make(downsampledURL: url, maxPixelSize: 1600))
            }
        /// 视频只接受Bundle中的资源，静音循环且始终保留静态海报。
        case .video(let name, let fileExtension, let poster):
            if let posterURL = bundledURL(poster, fileExtension: "jpg", subdirectory: "AuthMedia") {
                image.byImage(UIImage.make(downsampledURL: posterURL, maxPixelSize: 1600))
            } else {
                image.byImage(UIImage.make(named: poster))
            }
            guard let url = bundledURL(name, fileExtension: fileExtension, subdirectory: "AuthMedia") else {
                return
            }
            let playback = AVPlayer.make(url: url).byMuted(true)
            let surface = AVPlayerLayer.make(player: playback).byVideoGravity(.resizeAspectFill)
            player = playback
            videoLayer = surface
            image.layer.byAddSublayer(surface)
            readiness = surface.observe(\.isReadyForDisplay, options: [.initial, .new]) { [weak self] _, _ in
                Task { @MainActor [weak self] in
                    self?.updatePlayback()
                }
            }
            observations.append(NotificationCenter.default.addObserver(
                forName: .AVPlayerItemDidPlayToEndTime, object: playback.currentItem, queue: .main
            ) { [weak self] _ in
                Task { @MainActor [weak self] in
                    guard let self, self.canPlay else {
                        return
                    }
                    self.player?.seek(to: .zero)
                    self.player?.play()
                }
            })
        }
    }

    private func bundledURL(_ name: String, fileExtension: String, subdirectory: String?) -> URL? {
        return Bundle.main.url(forResource: name, withExtension: fileExtension, subdirectory: subdirectory)
            ?? Bundle.main.url(forResource: name, withExtension: fileExtension)
    }

    func setActive(_ value: Bool) {
        active = value
        updatePlayback()
    }

    private var canPlay: Bool {
        return active && window != nil && UIApplication.shared.applicationState == .active
            && !UIAccessibility.isReduceMotionEnabled
    }

    private func updatePlayback() {
        videoLayer?.byHidden(!canPlay || videoLayer?.isReadyForDisplay != true)
        if canPlay {
            player?.play()
        } else {
            player?.pause()
        }
    }

    override func didMoveToWindow() {
        super.didMoveToWindow()
        updatePlayback()
    }

    override func layoutSubviews() {
        super.layoutSubviews()
        videoLayer?.byFrame(image.bounds)
    }
}

@MainActor
final class IMAuthGlassView: UIView {

    private var transparencyObservation: NSObjectProtocol?
    private lazy var blur = UIVisualEffectView.make(effect: UIBlurEffect.make(style: .systemUltraThinMaterial))
    private lazy var tint = UIView.jobsMake {
        $0.byUserInteractionEnabled(false)
    }

    override init(frame: CGRect) {
        super.init(frame: frame)
        byBackgroundColor(JobsCor.clear)
            .byCornerRadius(24)
            .byClipsToBounds(true)
            .byBorderWidth(1)
            .byBorderColor(IMAuthAppearance.outline)
        blur.byAddTo(self) {
            $0.edges.equalToSuperview()
        }
        tint.byAddTo(self) {
            $0.edges.equalToSuperview()
        }
        refreshMaterial()
        transparencyObservation = NotificationCenter.default.addObserver(
            forName: UIAccessibility.reduceTransparencyStatusDidChangeNotification, object: nil, queue: .main
        ) { [weak self] _ in
            Task { @MainActor [weak self] in
                self?.refreshMaterial()
            }
        }
    }

    required init?(coder: NSCoder) {
        fatalError("init(coder:) has not been implemented")
    }

    isolated deinit {
        if let transparencyObservation {
            NotificationCenter.default.removeObserver(transparencyObservation)
        }
    }

    override func traitCollectionDidChange(_ previousTraitCollection: UITraitCollection?) {
        super.traitCollectionDidChange(previousTraitCollection)
        refreshMaterial()
    }

    private func refreshMaterial() {
        let dark = traitCollection.userInterfaceStyle == .dark
        let color = dark ? UIColor.make(hex: 0x14221B) : JobsCor.white
        // 材质本身已提供模糊与明暗底色；轻叠色保留背景层次，避免再次遮成实色卡片。
        let opacity: CGFloat = UIAccessibility.isReduceTransparencyEnabled ? 1 : dark ? 0.18 : 0.20
        blur.byHidden(UIAccessibility.isReduceTransparencyEnabled)
        tint.byBackgroundColor(color.withAlphaComponent(opacity))
        byBorderColor(IMAuthAppearance.outline.resolvedColor(with: traitCollection))
    }
}
