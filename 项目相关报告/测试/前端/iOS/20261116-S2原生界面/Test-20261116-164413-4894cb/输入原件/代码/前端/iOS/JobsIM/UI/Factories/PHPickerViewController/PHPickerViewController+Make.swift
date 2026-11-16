//
//  PHPickerViewController+Make.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import PhotosUI

extension PHPickerViewController {
    static func make(configuration: PHPickerConfiguration) -> PHPickerViewController {
        return PHPickerViewController(configuration: configuration)
    }
}
