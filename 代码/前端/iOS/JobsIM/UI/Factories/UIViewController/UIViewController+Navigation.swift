//
//  UIViewController+Navigation.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import UIKit

extension UIViewController {
    @discardableResult
    func byHidesBottomBarWhenPushed(_ value: Bool) -> Self {
        hidesBottomBarWhenPushed = value
        return self
    }
}
