//
//  PHPickerConfiguration+Make.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import PhotosUI

extension PHPickerConfiguration {
    static func make(filter: PHPickerFilter) -> PHPickerConfiguration {
        var configuration = PHPickerConfiguration(photoLibrary: .shared())
        configuration.filter = filter
        configuration.selectionLimit = 1
        return configuration
    }
}
