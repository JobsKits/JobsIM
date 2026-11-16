//
//  IMMediaTests.swift
//  JobsIMTests
//
//  Created by Jobs on 2026年11月14日，星期六.
//

import AVFoundation
import CryptoKit
import XCTest

import JobsIMCore
import JobsIMTransport
@testable import JobsIM

@MainActor
final class IMMediaTests: XCTestCase {

    func testRealNativeM4AUploadBindDownloadAndPlayback() async throws {
        let client = try IMIntegrationFixture.client()
        let identity = UUID().uuidString.replacingOccurrences(of: "-", with: "").lowercased()
        let response = try await client.request(
            "auth/register", method: "POST",
            body: [
                "username": .string("media_" + identity.prefix(16)),
                "password": .string(UUID().uuidString + UUID().uuidString),
                "requestId": .string(UUID().uuidString),
                "deviceName": .string("JobsIM Native media verification"),
            ])
        let token = response["token"].text
        XCTAssertFalse(token.isEmpty)
        let context = try await client.request("auth/context", token: token)
        let conversation = try await client.request(
            "conversations", method: "POST",
            body: [
                "kind": .string("group"), "title": .string("Native recorded audio"),
                "memberIds": .array([]), "requestId": .string(UUID().uuidString),
            ], token: token)

        let directory = FileManager.default.urls(for: .documentDirectory, in: .userDomainMask)[0]
            .appendingPathComponent("Verification", isDirectory: true)
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        let original = directory.appendingPathComponent("NativeRecorded.m4a")
        let downloaded = directory.appendingPathComponent("NativeDownloaded.m4a")
        let session = AVAudioSession.sharedInstance()
        try session.setCategory(.playAndRecord, mode: .default, options: [.defaultToSpeaker])
        try session.setActive(true)
        defer {
            try? session.setActive(false)
        }
        let recorder = try AVAudioRecorder.make(url: original)
        XCTAssertTrue(recorder.record(forDuration: 1.5))
        try await Task.sleep(for: .seconds(1.8))
        recorder.stop()
        let audio = try AVAudioFile(forReading: original)
        XCTAssertGreaterThan(audio.length, 0)
        let originalBytes = try Data(contentsOf: original)
        XCTAssertGreaterThan(originalBytes.count, 512)
        let media = try await client.upload(original, mimeType: "audio/mp4", token: token)
        XCTAssertEqual(media["contentType"].text, "audio/mp4")
        let clientID = UUID().uuidString
        let body: [String: IMJSON] = [
            "clientMsgId": .string(clientID), "type": .string("audio"), "text": .string("NativeRecorded.m4a"),
            "attachmentId": media["id"], "restoreEpoch": context["restoreEpoch"],
            "sendAdmission": conversation["sendAdmission"],
        ]
        let message = try await client.request(
            "conversations/\(conversation["id"].text)/messages", method: "POST", body: body, token: token)
        XCTAssertEqual(message["attachmentId"], media["id"])
        let reconciled = try await client.request(
            "conversations/\(conversation["id"].text)/messages", method: "POST", body: body, token: token)
        XCTAssertEqual(reconciled["id"], message["id"])
        let history = try await client.request(
            "conversations/\(conversation["id"].text)/messages?after=0&limit=100", token: token)
        XCTAssertTrue(history["items"].list.contains { $0["id"] == message["id"] })
        XCTAssertNotNil(IMCursor(raw: history["cursor"].text, expectedEpoch: context["restoreEpoch"].text))
        _ = try await client.download(media["id"].text, to: downloaded, token: token)
        let received = try Data(contentsOf: downloaded)
        XCTAssertEqual(SHA256.hash(data: received), SHA256.hash(data: originalBytes))
        let player = try AVAudioPlayer(contentsOf: downloaded)
        XCTAssertGreaterThan(player.duration, 1)
        XCTAssertTrue(player.prepareToPlay())
        XCTAssertTrue(player.play())
        try await Task.sleep(for: .milliseconds(150))
        XCTAssertTrue(player.isPlaying)
        player.stop()
        _ = try await client.request("auth/logout", method: "POST", token: token)
    }
}
