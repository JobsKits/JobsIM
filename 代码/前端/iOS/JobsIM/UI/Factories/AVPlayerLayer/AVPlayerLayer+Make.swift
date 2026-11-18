//
//  AVPlayerLayer+Make.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import AVFoundation

extension AVPlayerLayer {

    static func make(player: AVPlayer) -> AVPlayerLayer {
        return AVPlayerLayer(player: player)
    }

    @discardableResult
    func byVideoGravity(_ value: AVLayerVideoGravity) -> Self {
        videoGravity = value
        return self
    }
}
