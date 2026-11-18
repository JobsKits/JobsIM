//
//  IMAuthThumbnail+Make.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import ImageIO
import UIKit

extension UIImage {

    static func make(downsampledURL url: URL, maxPixelSize: Int) -> UIImage? {
        guard url.isFileURL, maxPixelSize > 0, maxPixelSize <= 1600,
            let source = CGImageSourceCreateWithURL(url as CFURL, [kCGImageSourceShouldCache: false] as CFDictionary),
            let thumbnail = CGImageSourceCreateThumbnailAtIndex(source, 0, [
                kCGImageSourceCreateThumbnailFromImageAlways: true,
                kCGImageSourceCreateThumbnailWithTransform: true,
                kCGImageSourceThumbnailMaxPixelSize: maxPixelSize,
                kCGImageSourceShouldCacheImmediately: true,
            ] as CFDictionary)
        else {
            return nil
        }
        return UIImage(cgImage: thumbnail)
    }
}
