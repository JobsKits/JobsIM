//
//  UIImage+Make.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import ImageIO
import UIKit
import UniformTypeIdentifiers

extension UIImage {
    static func make(named name: String) -> UIImage? {
        return UIImage(named: name)
    }

    static func make(contentsOfFile path: String) -> UIImage? {
        return UIImage(contentsOfFile: path)
    }

    static func makeAvatar(contentsOfFile path: String, maxPixelSize: Int) -> UIImage? {
        guard let source = CGImageSourceCreateWithURL(URL(fileURLWithPath: path) as CFURL, nil) else {
            return nil
        }
        let count = CGImageSourceGetCount(source)
        guard count > 1, CGImageSourceGetType(source) as String? == UTType.gif.identifier else {
            return UIImage(contentsOfFile: path)
        }
        guard count <= 120 else {
            return nil
        }
        let pixels = min(512, max(32, maxPixelSize))
        var frames: [UIImage] = []
        var delays: [Int] = []
        var decodedPixels = 0
        for index in 0..<count {
            guard let properties = CGImageSourceCopyPropertiesAtIndex(source, index, nil) as? [CFString: Any],
                let width = properties[kCGImagePropertyPixelWidth] as? Int,
                let height = properties[kCGImagePropertyPixelHeight] as? Int,
                width > 0, height > 0, width <= 4096, height <= 4096
            else {
                return nil
            }
            let scale = min(1, Double(pixels) / Double(max(width, height)))
            let expected = Int(ceil(Double(width) * scale)) * Int(ceil(Double(height) * scale))
            guard decodedPixels + expected <= 32_000_000,
                let image = CGImageSourceCreateThumbnailAtIndex(
                    source, index,
                    [
                        kCGImageSourceCreateThumbnailFromImageAlways: true,
                        kCGImageSourceCreateThumbnailWithTransform: true,
                        kCGImageSourceThumbnailMaxPixelSize: pixels,
                    ] as CFDictionary)
            else {
                return nil
            }
            decodedPixels += image.width * image.height
            guard decodedPixels <= 32_000_000 else {
                return nil
            }
            let gif = properties[kCGImagePropertyGIFDictionary] as? [CFString: Any]
            let delay =
                gif?[kCGImagePropertyGIFUnclampedDelayTime] as? Double
                ?? gif?[kCGImagePropertyGIFDelayTime] as? Double ?? 0.1
            guard delay.isFinite, delay >= 0, delay <= 600 else {
                return nil
            }
            let ticks = delay > 0 ? max(1, Int((delay * 100).rounded())) : 10
            guard ticks <= 60_000 else {
                return nil
            }
            frames.append(UIImage(cgImage: image))
            delays.append(ticks)
        }
        let duration = delays.reduce(0, +)
        guard duration <= 60_000 else {
            return nil
        }
        let divisor = delays.reduce(delays[0]) { current, delay in
            var first = current
            var second = delay
            while second != 0 {
                let remainder = first % second
                first = second
                second = remainder
            }
            return first
        }
        var timedFrames: [UIImage] = []
        for (frame, delay) in zip(frames, delays) {
            let repeats = delay / divisor
            guard timedFrames.count + repeats <= 12_000 else {
                return nil
            }
            timedFrames.append(contentsOf: repeatElement(frame, count: repeats))
        }
        return UIImage.animatedImage(with: timedFrames, duration: Double(duration) / 100)
    }
}
