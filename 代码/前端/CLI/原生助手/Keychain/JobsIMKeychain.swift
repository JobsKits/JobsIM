//
//  JobsIMKeychain.swift
//  JobsIMKeychain
//
//  Created by Jobs on 2026年11月15日，星期日.
//

import Foundation
import Security

@main
private enum JobsIMKeychain {

    private static let maximumDataSize = 8 * 1024
    private static let maximumWireSize = 12 * 1024
    private static let service = "JobsIM.CLI"

    private struct Request: Decodable {
        let operation: String
        let account: String
        let data: Data?
    }

    private struct Response: Encodable {
        let status: String
        let data: Data?
    }

    static func main() {
        var input = Data()
        defer {
            input.resetBytes(in: input.startIndex..<input.endIndex)
        }
        do {
            while let chunk = try FileHandle.standardInput.read(upToCount: min(4096, maximumWireSize + 1 - input.count)), !chunk.isEmpty {
                input.append(chunk)
                guard input.count <= maximumWireSize else {
                    emit(Response(status: "unavailable", data: nil))
                    return
                }
            }
            let request = try JSONDecoder.make().decode(Request.self, from: input)
            guard request.account.utf8.count == 64,
                  request.account.utf8.allSatisfy({ byte in
                      (byte >= 48 && byte <= 57) || (byte >= 97 && byte <= 102)
                  }),
                  ["read", "write", "delete"].contains(request.operation),
                  request.operation == "write" || request.data == nil else {
                emit(Response(status: "unavailable", data: nil))
                return
            }
            process(request)
        } catch {
            emit(Response(status: "unavailable", data: nil))
        }
    }

    private static func process(_ request: Request) {
        // A separate, device-bound item is the atomic credential replacement unit.
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: request.account,
            kSecAttrSynchronizable as String: false,
            kSecUseDataProtectionKeychain as String: true,
            kSecUseAuthenticationUI as String: kSecUseAuthenticationUIFail,
        ]
        switch request.operation {
        case "read":
            var search = query
            search[kSecMatchLimit as String] = kSecMatchLimitOne
            search[kSecReturnData as String] = true
            var result: CFTypeRef?
            let status = SecItemCopyMatching(search as CFDictionary, &result)
            if status == errSecItemNotFound {
                emit(Response(status: "not_found", data: nil))
                return
            }
            guard status == errSecSuccess,
                  var data = result as? Data,
                  !data.isEmpty,
                  data.count <= maximumDataSize else {
                emit(Response(status: "unavailable", data: nil))
                return
            }
            defer {
                data.resetBytes(in: data.startIndex..<data.endIndex)
            }
            emit(Response(status: "ok", data: data))
        case "write":
            guard var data = request.data,
                  !data.isEmpty,
                  data.count <= maximumDataSize else {
                emit(Response(status: "unavailable", data: nil))
                return
            }
            defer {
                data.resetBytes(in: data.startIndex..<data.endIndex)
            }
            let attributes: [String: Any] = [
                kSecValueData as String: data,
                kSecAttrAccessible as String: kSecAttrAccessibleWhenUnlockedThisDeviceOnly,
                kSecAttrSynchronizable as String: false,
            ]
            let update = SecItemUpdate(query as CFDictionary, attributes as CFDictionary)
            if update == errSecItemNotFound {
                var insert = query.merging(attributes) { _, newValue in
                    return newValue
                }
                insert.removeValue(forKey: kSecUseAuthenticationUI as String)
                let status = SecItemAdd(insert as CFDictionary, nil)
                emit(Response(status: status == errSecSuccess ? "ok" : "unavailable", data: nil))
            } else {
                emit(Response(status: update == errSecSuccess ? "ok" : "unavailable", data: nil))
            }
        case "delete":
            let status = SecItemDelete(query as CFDictionary)
            emit(Response(status: status == errSecSuccess || status == errSecItemNotFound ? "ok" : "unavailable", data: nil))
        default:
            emit(Response(status: "unavailable", data: nil))
        }
    }

    private static func emit(_ response: Response) {
        guard var data = try? JSONEncoder.make().encode(response), data.count <= maximumWireSize else {
            return
        }
        defer {
            data.resetBytes(in: data.startIndex..<data.endIndex)
        }
        try? FileHandle.standardOutput.write(contentsOf: data)
    }
}

private extension JSONDecoder {
    static func make() -> JSONDecoder {
        return JSONDecoder()
    }
}

private extension JSONEncoder {
    static func make() -> JSONEncoder {
        return JSONEncoder()
    }
}
