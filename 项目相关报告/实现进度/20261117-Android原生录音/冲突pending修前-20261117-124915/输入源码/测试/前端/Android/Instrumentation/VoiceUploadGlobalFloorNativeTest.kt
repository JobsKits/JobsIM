package com.jobs.im

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import java.io.File
import java.nio.file.Files
import java.util.UUID
import kotlinx.coroutines.runBlocking
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class VoiceUploadGlobalFloorNativeTest {
    @Test fun verifiedGlobalCreatesDenyAfterTerminalReceiptWasAlreadyGc() = runBlocking {
        val app = InstrumentationRegistry.getInstrumentation().targetContext
        val fixture = UUID.randomUUID()
        val root = File(app.noBackupFilesDir.canonicalFile, "JobsIM-test-VoiceUploads-" + fixture)
        fun id() = UUID.randomUUID().toString()
        val profile = id(); val deployment = id(); val user = id(); val device = id()
        val owner = VoiceUploadOwner(Isolation.namespace(profile, deployment, user, false, "1", "personal", device),
            profile, "https://global.invalid", deployment, user, device, id(), "1")
        val source = VoiceUploadSource(owner, id(), "sa1.original", "1")
        val bytes = ByteArray(1024) { (it % 251).toByte() }
        try {
            val queue = VoiceUploadQueue(app, fixture)
            val original = queue.prepare(source, bytes, 1200)
            queue.uploadStarted(original.requestId, owner, true)
            val confirmed = queue.objectConfirmed(original.requestId, id(), original.size, original.sha256)
            val server = id()
            val accepted = confirmed.row().copy(id = server, state = "received")
            queue.complete(original.requestId, accepted)
            val receipt = File(root, original.requestId)
            assertFalse(File(receipt, "clip.m4a").exists())
            // Bounded receipt GC can precede a later verified global cleanup event.
            receipt.listFiles().orEmpty().forEach { assertTrue(it.delete()) }
            assertTrue(receipt.delete())
            PersonalExportFiles.syncDirectory(root)
            assertTrue(VoiceUploadQueue(app, fixture).list().entries.isEmpty())
            val holder = accepted.copy(text = "", attachmentId = "", retracted = true)
            VoiceUploadQueue(app, fixture).eraseMessages(
                "live|" + profile + "|" + deployment + "|" + user + "|personal", server, listOf(holder))
            assertTrue(root.listFiles().orEmpty().any { it.name.startsWith(".message-erased-") })
            val restored = VoiceUploadQueue(app, fixture)
            try { restored.prepare(source, bytes, 1200, original.requestId, original.clientMsgId); fail("Late original seed must remain denied after receipt GC") }
            catch (_: IllegalStateException) { }
            assertFalse(receipt.exists())
        } finally {
            check(root.parentFile == app.noBackupFilesDir.canonicalFile)
            fun erase(file: File) {
                if (!Files.isSymbolicLink(file.toPath()) && file.isDirectory) file.listFiles()?.forEach(::erase)
                check(file.delete() || !file.exists())
            }
            if (root.exists()) erase(root)
        }
    }

    @Test fun globalDenySurvivesTerminalReceiptGcAndRejectsLateOriginalPrepare() = runBlocking {
        val app = InstrumentationRegistry.getInstrumentation().targetContext
        val fixture = UUID.randomUUID()
        val root = File(app.noBackupFilesDir.canonicalFile, "JobsIM-test-VoiceUploads-" + fixture)
        fun id() = UUID.randomUUID().toString()
        val profile = id(); val deployment = id(); val user = id(); val device = id()
        val owner = VoiceUploadOwner(Isolation.namespace(profile, deployment, user, false, "1", "personal", device),
            profile, "https://global.invalid", deployment, user, device, id(), "1")
        val source = VoiceUploadSource(owner, id(), "sa1.original", "1")
        val bytes = ByteArray(1024) { (it % 251).toByte() }
        try {
            val queue = VoiceUploadQueue(app, fixture)
            val original = queue.prepare(source, bytes, 1200)
            queue.uploadStarted(original.requestId, owner, true)
            val confirmed = queue.objectConfirmed(original.requestId, id(), original.size, original.sha256)
            val server = id()
            val holder = confirmed.row().copy(id = server, state = "received", text = "", attachmentId = "", retracted = true)
            queue.eraseMessages("live|" + profile + "|" + deployment + "|" + user + "|personal", server, listOf(holder))
            assertTrue(root.listFiles().orEmpty().any { it.name.startsWith(".message-erased-") })
            val receipt = File(root, original.requestId)
            assertFalse(File(receipt, "clip.m4a").exists())
            // Model bounded terminal-receipt GC; the independent minimal deny fact must remain authoritative.
            receipt.listFiles().orEmpty().forEach { assertTrue(it.delete()) }
            assertTrue(receipt.delete())
            PersonalExportFiles.syncDirectory(root)
            val restored = VoiceUploadQueue(app, fixture)
            try { restored.prepare(source, bytes, 1200, original.requestId, original.clientMsgId); fail("Global-cleared bytes cannot be prepared again") }
            catch (_: IllegalStateException) { }
            assertFalse(receipt.exists())
        } finally {
            check(root.parentFile == app.noBackupFilesDir.canonicalFile)
            fun erase(file: File) {
                if (!Files.isSymbolicLink(file.toPath()) && file.isDirectory) file.listFiles()?.forEach(::erase)
                check(file.delete() || !file.exists())
            }
            if (root.exists()) erase(root)
        }
    }
}
