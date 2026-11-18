//
//  QLPreviewController+Make.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import JobsSwiftDSL
import QuickLook

extension QLPreviewController {
    @MainActor
    static func make(dataSource: QLPreviewControllerDataSource) -> QLPreviewController {
        return QLPreviewController.jobsMake {
            controller in
            controller.dataSource = dataSource
        }
    }
}
