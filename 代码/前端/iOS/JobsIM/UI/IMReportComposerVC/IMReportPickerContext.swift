//
//  IMReportPickerContext.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月18日，星期三.
//

import Foundation

// Item-provider callbacks carry an actor-isolated reference, never a UIKit controller between queues.
@MainActor
final class IMReportPickerContext {
    private weak var controller: IMReportComposerVC?

    init(controller: IMReportComposerVC) {
        self.controller = controller
    }

    func stageFile(from url: URL, folderID: String, limit: Int64) throws -> URL {
        guard let controller else {
            throw CocoaError(.userCancelled)
        }
        return try controller.stagePickedEvidence(from: url, folderID: folderID, limit: limit)
    }
}
