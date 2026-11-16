//
//  UIImage+Make.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import UIKit

extension UIImage {
    static func make(named name: String) -> UIImage? {
        return UIImage(named: name)
    }

    static func make(contentsOfFile path: String) -> UIImage? {
        return UIImage(contentsOfFile: path)
    }
}
