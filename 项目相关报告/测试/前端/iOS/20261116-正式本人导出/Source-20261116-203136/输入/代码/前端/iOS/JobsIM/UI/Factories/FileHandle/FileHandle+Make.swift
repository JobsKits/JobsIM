//
//  FileHandle+Make.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月16日，星期一.
//

import Foundation

extension FileHandle {

    static func makeReading(_ url: URL) throws -> FileHandle {
        return try FileHandle(forReadingFrom: url)
    }
}
