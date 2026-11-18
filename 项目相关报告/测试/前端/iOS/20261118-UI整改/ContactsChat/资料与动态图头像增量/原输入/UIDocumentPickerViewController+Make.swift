//
//  UIDocumentPickerViewController+Make.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import UIKit
import UniformTypeIdentifiers

extension UIDocumentPickerViewController {

    static func make() -> UIDocumentPickerViewController {
        let picker = UIDocumentPickerViewController(forOpeningContentTypes: [.item], asCopy: true)
        picker.directoryURL = FileManager.default.urls(for: .documentDirectory, in: .userDomainMask).first
        return picker
    }
    static func makeImages() -> UIDocumentPickerViewController {
        let picker = UIDocumentPickerViewController(forOpeningContentTypes: [.jpeg, .png], asCopy: true)
        picker.directoryURL = FileManager.default.urls(for: .documentDirectory, in: .userDomainMask).first
        return picker
    }

    static func makeExporting(_ file: URL, delegate: UIDocumentPickerDelegate) -> UIDocumentPickerViewController {
        let picker = UIDocumentPickerViewController(forExporting: [file], asCopy: true)
        picker.delegate = delegate
        return picker
    }

    static func makeImporting(delegate: UIDocumentPickerDelegate) -> UIDocumentPickerViewController {
        let picker = make()
        picker.delegate = delegate
        return picker
    }
}
