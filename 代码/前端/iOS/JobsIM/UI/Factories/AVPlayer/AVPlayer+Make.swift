//
//  AVPlayer+Make.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import AVFoundation

extension AVPlayer {

    static func make(url: URL) -> AVPlayer {
        return AVPlayer(url: url)
    }

    @discardableResult
    func byMuted(_ muted: Bool) -> Self {
        isMuted = muted
        return self
    }
}
