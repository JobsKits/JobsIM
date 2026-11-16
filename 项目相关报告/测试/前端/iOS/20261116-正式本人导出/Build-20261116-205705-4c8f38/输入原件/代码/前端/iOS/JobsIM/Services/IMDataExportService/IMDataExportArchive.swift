//
//  IMDataExportArchive.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月16日，星期一.
//

import Foundation

enum IMDataExportArchive {

    static let maximumSize: UInt64 = 1_073_741_824
    static let maximumDirectory: UInt64 = 33_554_432

    private static func invalid() throws -> Never {
        try IMDataExportProtocol.fail("DATA_EXPORT_FILE_INVALID")
    }

    private static func number(_ data: Data, _ offset: Int, _ width: Int) throws -> UInt64 {
        guard offset >= 0, offset + width <= data.count else {
            try invalid()
        }
        return (0..<width).reduce(UInt64(0)) { $0 | (UInt64(data[offset + $1]) << (8 * $1)) }
    }

    static func validate(_ url: URL) throws {
        try Task.checkCancellation()
        let handle = try FileHandle.makeReading(url)
        defer {
            try? handle.close()
        }
        let length = try handle.seekToEnd()
        guard length <= maximumSize else {
            try IMDataExportProtocol.fail("DATA_EXPORT_FILE_LIMIT")
        }
        guard length >= 22 else {
            try invalid()
        }
        func read(_ offset: UInt64, count: Int) throws -> Data {
            guard offset <= length, UInt64(count) <= length - offset else {
                try invalid()
            }
            try handle.seek(toOffset: offset)
            let bytes = try handle.read(upToCount: count) ?? Data()
            guard bytes.count == count else {
                try invalid()
            }
            return bytes
        }
        let start = length > 65557 ? length - 65557 : 0
        let tail = try read(start, count: Int(length - start))
        var end: Int?
        for index in stride(from: tail.count - 22, through: 0, by: -1) {
            if try number(tail, index, 4) == 0x06054b50,
                index + 22 + Int(try number(tail, index + 20, 2)) == tail.count {
                end = index
                break
            }
        }
        guard let end, try number(tail, end + 4, 2) == 0, try number(tail, end + 6, 2) == 0 else {
            try invalid()
        }
        var count = try number(tail, end + 10, 2)
        var size = try number(tail, end + 12, 4)
        var offset = try number(tail, end + 16, 4)
        var directoryEnd = start + UInt64(end)
        guard try number(tail, end + 8, 2) == count else {
            try invalid()
        }
        if count == 65535 || size == UInt64(UInt32.max) || offset == UInt64(UInt32.max) {
            guard end >= 20, try number(tail, end - 20, 4) == 0x07064b50,
                try number(tail, end - 16, 4) == 0, try number(tail, end - 4, 4) == 1 else {
                try invalid()
            }
            let recordOffset = try number(tail, end - 12, 8)
            let record = try read(recordOffset, count: 56)
            let recordLength = try number(record, 4, 8)
            guard try number(record, 0, 4) == 0x06064b50, try number(record, 16, 4) == 0,
                try number(record, 20, 4) == 0, recordLength >= 44, recordLength <= length,
                recordOffset <= length - recordLength,
                recordOffset + 12 + recordLength == start + UInt64(end) - 20 else {
                try invalid()
            }
            count = try number(record, 32, 8)
            guard try number(record, 24, 8) == count else {
                try invalid()
            }
            size = try number(record, 40, 8)
            offset = try number(record, 48, 8)
            directoryEnd = recordOffset
        }
        guard size <= maximumDirectory, count <= maximumDirectory / 46 else {
            try IMDataExportProtocol.fail("DATA_EXPORT_FILE_LIMIT")
        }
        guard count > 0, offset <= directoryEnd, size == directoryEnd - offset, size >= count * 46 else {
            try invalid()
        }
        let directory = try read(offset, count: Int(size))
        var cursor = 0
        var names = Set<String>()
        for _ in 0..<count {
            try Task.checkCancellation()
            guard cursor + 46 <= directory.count, try number(directory, cursor, 4) == 0x02014b50 else {
                try invalid()
            }
            let nameLength = Int(try number(directory, cursor + 28, 2))
            let extra = Int(try number(directory, cursor + 30, 2))
            let comment = Int(try number(directory, cursor + 32, 2))
            let local = try number(directory, cursor + 42, 4)
            let compressed = try number(directory, cursor + 20, 4)
            let next = cursor + 46 + nameLength + extra + comment
            guard nameLength > 0, next <= directory.count, local + compressed + 30 < offset,
                try number(directory, cursor + 34, 2) == 0,
                let name = String(data: directory.subdata(in: (cursor + 46)..<(cursor + 46 + nameLength)), encoding: .utf8),
                !name.hasPrefix("/"), !name.contains("\\"), !name.contains("\0"),
                !name.split(separator: "/").contains(".."), names.insert(name).inserted else {
                try invalid()
            }
            let header = try read(local, count: 30)
            guard try number(header, 0, 4) == 0x04034b50,
                try number(header, 26, 2) == UInt64(nameLength),
                local + 30 + UInt64(nameLength) + (try number(header, 28, 2)) + compressed <= offset,
                try read(local + 30, count: nameLength) == Data(name.utf8) else {
                try invalid()
            }
            cursor = next
        }
        guard cursor == directory.count, names.contains("manifest.json"), names.contains("account-export.json") else {
            try invalid()
        }
    }
}
