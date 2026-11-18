//
//  IMReceiptBuild.swift
//  JobsIM
//
//  Created by Jobs on 2026年11月17日，星期二.
//

import CryptoKit
import Darwin
import Foundation
import JobsIMCore

enum IMReceiptBuild {

    private final class EnumerationState: @unchecked Sendable {
        private let lock = NSLock()
        private var failed = false

        func fail() {
            lock.lock()
            failed = true
            lock.unlock()
        }

        func didFail() -> Bool {
            lock.lock()
            defer {
                lock.unlock()
            }
            return failed
        }
    }

    struct Measurement: Sendable {
        let digest: String
        let components: [IMJSON]
    }

    private static let maximum = 512 * 1024 * 1024
    private static let magic: Set<[UInt8]> = [
        [0xFE, 0xED, 0xFA, 0xCE], [0xCE, 0xFA, 0xED, 0xFE],
        [0xFE, 0xED, 0xFA, 0xCF], [0xCF, 0xFA, 0xED, 0xFE],
        [0xCA, 0xFE, 0xBA, 0xBE], [0xBE, 0xBA, 0xFE, 0xCA],
        [0xCA, 0xFE, 0xBA, 0xBF], [0xBF, 0xBA, 0xFE, 0xCA],
    ]

    private static func withFile<T>(_ file: URL, limit: Int, action: (Int32, Int) throws -> T) throws -> T {
        var named = stat()
        guard lstat(file.path, &named) == 0, named.st_mode & mode_t(S_IFMT) == mode_t(S_IFREG),
            named.st_size >= 0, named.st_size <= off_t(limit) else {
            try IMReceiptProtocol.fail("RECEIPT_BUILD_UNVERIFIED")
        }
        let descriptor = Darwin.open(file.path, O_RDONLY | O_NOFOLLOW | O_CLOEXEC)
        guard descriptor >= 0 else {
            try IMReceiptProtocol.fail("RECEIPT_BUILD_UNVERIFIED")
        }
        defer {
            Darwin.close(descriptor)
        }
        var opened = stat()
        guard fstat(descriptor, &opened) == 0, opened.st_dev == named.st_dev,
            opened.st_ino == named.st_ino, opened.st_size == named.st_size else {
            try IMReceiptProtocol.fail("RECEIPT_BUILD_UNVERIFIED")
        }
        let result = try action(descriptor, Int(opened.st_size))
        var current = stat()
        guard lstat(file.path, &current) == 0, current.st_mode & mode_t(S_IFMT) == mode_t(S_IFREG),
            current.st_dev == opened.st_dev, current.st_ino == opened.st_ino, current.st_size == opened.st_size,
            current.st_mtimespec.tv_sec == opened.st_mtimespec.tv_sec,
            current.st_mtimespec.tv_nsec == opened.st_mtimespec.tv_nsec else {
            try IMReceiptProtocol.fail("RECEIPT_BUILD_UNVERIFIED")
        }
        return result
    }

    private static func read(_ descriptor: Int32, maximum: Int) throws -> Data {
        var buffer = [UInt8](repeating: 0, count: maximum)
        while true {
            let count = buffer.withUnsafeMutableBytes { raw in
                return Darwin.read(descriptor, raw.baseAddress, raw.count)
            }
            if count < 0, errno == EINTR {
                continue
            }
            guard count >= 0 else {
                try IMReceiptProtocol.fail("RECEIPT_BUILD_UNVERIFIED")
            }
            return Data(buffer.prefix(count))
        }
    }

    static func measure(bundle: URL, executable: URL) throws -> Measurement {
        let root = bundle.standardizedFileURL
        let scan = EnumerationState()
        var rootInformation = stat()
        guard lstat(root.path, &rootInformation) == 0,
            rootInformation.st_mode & mode_t(S_IFMT) == mode_t(S_IFDIR),
            let enumerator = FileManager.default.enumerator(at: root, includingPropertiesForKeys: nil, errorHandler: { _, _ in
                scan.fail()
                return false
            }) else {
            try IMReceiptProtocol.fail("RECEIPT_BUILD_UNVERIFIED")
        }
        let canonical = root.resolvingSymlinksInPath().path + "/"
        var files: [(path: String, url: URL, size: Int)] = []
        var frameworks: [URL] = []
        var total = 0, entries = 0
        for case let file as URL in enumerator {
            entries += 1
            guard entries <= 8192, file.standardizedFileURL.path.hasPrefix(root.path + "/"),
                file.resolvingSymlinksInPath().path.hasPrefix(canonical) else {
                try IMReceiptProtocol.fail("RECEIPT_BUILD_UNVERIFIED")
            }
            let relative = String(file.standardizedFileURL.path.dropFirst(root.path.count + 1))
            guard relative.utf8.count <= 1024, !relative.split(separator: "/").contains("..") else {
                try IMReceiptProtocol.fail("RECEIPT_BUILD_UNVERIFIED")
            }
            var information = stat()
            guard lstat(file.path, &information) == 0 else {
                try IMReceiptProtocol.fail("RECEIPT_BUILD_UNVERIFIED")
            }
            if information.st_mode & mode_t(S_IFMT) == mode_t(S_IFDIR) {
                if file.pathExtension == "framework" {
                    frameworks.append(file)
                }
                continue
            }
            guard information.st_mode & mode_t(S_IFMT) == mode_t(S_IFREG), information.st_size >= 0,
                information.st_size <= off_t(maximum), files.count < 4096 else {
                try IMReceiptProtocol.fail("RECEIPT_BUILD_UNVERIFIED")
            }
            total += Int(information.st_size)
            guard total <= maximum else {
                try IMReceiptProtocol.fail("RECEIPT_BUILD_UNVERIFIED")
            }
            files.append((relative, file, Int(information.st_size)))
        }
        guard !scan.didFail(), executable.standardizedFileURL.path.hasPrefix(root.path + "/") else {
            try IMReceiptProtocol.fail("RECEIPT_BUILD_UNVERIFIED")
        }
        var required: Set<String> = [String(executable.standardizedFileURL.path.dropFirst(root.path.count + 1))]
        for framework in frameworks {
            let info = try withFile(framework.appendingPathComponent("Info.plist"), limit: 1024 * 1024) { descriptor, size in
                let bytes = try read(descriptor, maximum: size + 1)
                guard bytes.count == size else {
                    try IMReceiptProtocol.fail("RECEIPT_BUILD_UNVERIFIED")
                }
                return bytes
            }
            let plist = try PropertyListSerialization.propertyList(from: info, format: nil) as? [String: Any]
            guard let name = plist?["CFBundleExecutable"] as? String, !name.isEmpty, name.utf8.count <= 255,
                name != ".", name != "..", !name.contains("/"), !name.contains("\\") else {
                try IMReceiptProtocol.fail("RECEIPT_BUILD_UNVERIFIED")
            }
            let code = framework.appendingPathComponent(name).standardizedFileURL.path
            required.insert(String(code.dropFirst(root.path.count + 1)))
        }
        var components: [IMJSON] = []
        for file in files.sorted(by: { $0.path < $1.path }) {
            let digest: String? = try withFile(file.url, limit: maximum) { descriptor, size in
                guard size == file.size else {
                    try IMReceiptProtocol.fail("RECEIPT_BUILD_UNVERIFIED")
                }
                let header = try read(descriptor, maximum: 4)
                let code = magic.contains(Array(header))
                guard code || !required.contains(file.path) && file.url.pathExtension != "dylib" else {
                    try IMReceiptProtocol.fail("RECEIPT_BUILD_UNVERIFIED")
                }
                guard code else {
                    return nil
                }
                var hasher = SHA256()
                hasher.update(data: header)
                var length = header.count
                while true {
                    try Task.checkCancellation()
                    let bytes = try read(descriptor, maximum: 1024 * 1024)
                    if bytes.isEmpty {
                        break
                    }
                    length += bytes.count
                    guard length <= size else {
                        try IMReceiptProtocol.fail("RECEIPT_BUILD_UNVERIFIED")
                    }
                    hasher.update(data: bytes)
                }
                guard length == size else {
                    try IMReceiptProtocol.fail("RECEIPT_BUILD_UNVERIFIED")
                }
                return hasher.finalize().map { String(format: "%02x", $0) }.joined()
            }
            if let digest {
                components.append(.object(["path": .string(file.path), "sizeBytes": .string(String(file.size)), "sha256": .string(digest)]))
            }
        }
        guard !components.isEmpty, required.isSubset(of: Set(components.map { $0["path"].text })) else {
            try IMReceiptProtocol.fail("RECEIPT_BUILD_UNVERIFIED")
        }
        var manifest = Data("JobsIM iOS receipt code build v1\0".utf8)
        for component in components {
            manifest.append(Data((String(component["path"].text.utf8.count) + ":" + component["path"].text + "\0"
                + component["sizeBytes"].text + "\0" + component["sha256"].text + "\0").utf8))
        }
        return Measurement(digest: IMMigrationProtocol.hash(manifest), components: components)
    }
}
