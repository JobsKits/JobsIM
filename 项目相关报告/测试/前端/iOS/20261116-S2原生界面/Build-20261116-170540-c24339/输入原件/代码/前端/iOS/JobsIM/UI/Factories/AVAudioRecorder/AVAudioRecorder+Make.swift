//
//  AVAudioRecorder+Make.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import AVFoundation

extension AVAudioRecorder {

    static func make(url: URL) throws -> AVAudioRecorder {
        try AVAudioRecorder(
            url: url,
            settings: [
                AVFormatIDKey: kAudioFormatMPEG4AAC, AVSampleRateKey: 44100, AVNumberOfChannelsKey: 1,
                AVEncoderAudioQualityKey: AVAudioQuality.high.rawValue,
            ])
    }
}
