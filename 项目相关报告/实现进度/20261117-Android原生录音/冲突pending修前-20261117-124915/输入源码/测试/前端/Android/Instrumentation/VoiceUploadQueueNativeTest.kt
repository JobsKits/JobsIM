package com.jobs.im

import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import java.io.File
import java.io.IOException
import java.nio.file.Files
import java.util.UUID
import kotlinx.coroutines.runBlocking
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class VoiceUploadQueueNativeTest {
    private val context get() = InstrumentationRegistry.getInstrumentation().targetContext
    private fun id() = UUID.randomUUID().toString()
    private fun source(): VoiceUploadSource {
        val profile = "test-profile-" + id(); val deployment = id(); val user = id(); val device = id()
        return VoiceUploadSource(VoiceUploadOwner(Isolation.namespace(profile, deployment, user, false, "1", "personal", device),
            profile, "https://queue.invalid", deployment, user, device, id(), "1"), id(), "sa1.synthetic-original", "2", id())
    }
    private val clip = ByteArray(1024) { (it % 251).toByte() }

    private fun fixture(action: suspend (UUID, File) -> Unit) = runBlocking {
        val fixture = UUID.randomUUID()
        val root = File(context.noBackupFilesDir.canonicalFile, "JobsIM-test-VoiceUploads-" + fixture)
        try { action(fixture, root) }
        finally {
            check(root.parentFile == context.noBackupFilesDir.canonicalFile)
            fun erase(file: File) {
                if (!Files.isSymbolicLink(file.toPath()) && file.isDirectory) file.listFiles()?.forEach(::erase)
                check(file.delete() || !file.exists())
            }
            if (root.exists()) erase(root)
        }
    }

    @Test fun durableManifestStageRecoversTheSameTaskWithoutSending() = fixture { fixture, root ->
        val original = source()
        val failing = VoiceUploadQueue(context, fixture) { step ->
            if (step == VoiceUploadCheckpoint.MANIFEST_SYNCED) throw IOException("synthetic termination after manifest sync")
        }
        try { failing.prepare(original, clip, 1200); fail("Fault must interrupt publication") }
        catch (_: IOException) { }
        assertTrue(root.listFiles().orEmpty().any { it.name.startsWith(".staging-") })
        val restored = VoiceUploadQueue(context, fixture)
        val first = restored.list().entries.single().intent!!
        assertEquals(VoiceUploadPhase.PREPARED, first.phase)
        assertEquals(original, first.source)
        assertEquals(VoiceUploadRules.digest(clip), first.sha256)
        assertEquals(first, VoiceUploadQueue(context, fixture).list().entries.single().intent)
        assertTrue(File(root, first.requestId + "/clip.m4a").isFile)
    }

    @Test fun clipOnlyFailureIsRetainedAndBlocksNewHttpResponsibility() = fixture { fixture, root ->
        val failing = VoiceUploadQueue(context, fixture) { step ->
            if (step == VoiceUploadCheckpoint.CLIP_SYNCED) throw IOException("synthetic termination before manifest")
        }
        try { failing.prepare(source(), clip, 1200); fail("Fault must stop preparation") }
        catch (_: IOException) { }
        val restored = VoiceUploadQueue(context, fixture)
        val entry = restored.list().entries.single()
        assertNotNull("Owner attribution is durable before unique clip bytes", entry.intent)
        assertEquals("VOICE_QUEUE_RECOVERY_REQUIRED", entry.problem)
        val file = root.listFiles()!!.single { it.name.startsWith(".staging-") }.resolve("clip.m4a")
        assertArrayEquals(clip, file.readBytes())
        try { restored.prepare(source(), clip, 1200); fail("Unattributed unique bytes must block new work") }
        catch (_: IllegalStateException) { }
        assertArrayEquals(clip, file.readBytes())
    }

    @Test fun uploadAndEnqueueUnknownKeepOriginalIdsAndReplyAcrossReconstruction() = fixture { fixture, root ->
        val store = VoiceUploadQueue(context, fixture)
        val prepared = store.prepare(source(), clip, 1200)
        store.uploadStarted(prepared.requestId, prepared.source.owner, true)
        var restored = VoiceUploadQueue(context, fixture).get(prepared.requestId)
        assertEquals(VoiceUploadPhase.UPLOAD_UNKNOWN, restored.phase)
        assertArrayEquals(clip, store.bytesForUpload(prepared.requestId, prepared.source.owner))
        val objectId = id()
        store.objectPrepared(prepared.requestId, objectId)
        store.objectConfirmed(prepared.requestId, objectId, clip.size.toLong(), prepared.sha256)
        store.enqueueStarted(prepared.requestId, prepared.source.owner)
        restored = VoiceUploadQueue(context, fixture).get(prepared.requestId)
        assertEquals(VoiceUploadPhase.ENQUEUE_UNKNOWN, restored.phase)
        assertEquals(prepared.clientMsgId, restored.row().clientMsgId)
        assertEquals(prepared.source.replyTo, restored.row().replyToId)
        store.queued(prepared.requestId, restored.row())
        assertEquals(VoiceUploadPhase.QUEUED, VoiceUploadQueue(context, fixture).get(prepared.requestId).phase)
        assertTrue(File(root, prepared.requestId + "/clip.m4a").isFile)
    }

    @Test fun changedFamilyCannotReusePreparedBytesAndDoesNotEraseTheOnlyClip() = fixture { fixture, root ->
        val store = VoiceUploadQueue(context, fixture)
        val prepared = store.prepare(source(), clip, 1200)
        store.uploadStarted(prepared.requestId, prepared.source.owner, true)
        try { store.bytesForUpload(prepared.requestId, prepared.source.owner.copy(family = id())); fail("Changed family must be rejected") }
        catch (_: IllegalStateException) { }
        assertArrayEquals(clip, File(root, prepared.requestId + "/clip.m4a").readBytes())
        store.freeze(prepared.requestId)
        try { store.bytesForUpload(prepared.requestId, prepared.source.owner); fail("A frozen task cannot regain permission") }
        catch (_: IllegalStateException) { }
    }

    @Test fun tornNewManifestDoesNotReplaceTheOriginalAtomicBody() = fixture { fixture, root ->
        val store = VoiceUploadQueue(context, fixture)
        val original = store.prepare(source(), clip, 1200)
        File(root, original.requestId + "/intent.json.new").writeText("{partial")
        assertEquals(original, VoiceUploadQueue(context, fixture).get(original.requestId))
        assertTrue(File(root, original.requestId + "/clip.m4a").isFile)
    }

    @Test fun erasureRemovesExactAccountAndPreservesAnotherUsersTask() = fixture { fixture, root ->
        val store = VoiceUploadQueue(context, fixture)
        val first = store.prepare(source(), clip, 1200)
        val other = store.prepare(source(), clip, 1200)
        val owner = first.source.owner
        store.eraseAccount(owner.profile, owner.deployment, owner.user)
        assertFalse(File(root, first.requestId).exists())
        assertEquals(other, VoiceUploadQueue(context, fixture).get(other.requestId))
        assertArrayEquals(clip, File(root, other.requestId + "/clip.m4a").readBytes())
    }

    @Test fun publicationFailureRetriesTheSameIdsAndRejectsAChangedOriginalBody() = fixture { fixture, _ ->
        val source = source(); val request = id(); val client = id()
        val failing = VoiceUploadQueue(context, fixture) { step ->
            if (step == VoiceUploadCheckpoint.PUBLISHED) throw IOException("synthetic termination after publication")
        }
        try { failing.prepare(source, clip, 1200, request, client); fail("Fault must be retained") }
        catch (_: IOException) { }
        val restored = VoiceUploadQueue(context, fixture)
        val result = restored.prepare(source, clip, 1200, request, client)
        assertEquals(request, result.requestId); assertEquals(client, result.clientMsgId)
        assertEquals(1, restored.list().entries.size)
        try { restored.prepare(source.copy(replyTo = id()), clip, 1200, request, client); fail("Original body cannot change") }
        catch (_: IllegalStateException) { }
    }

    @Test fun globalCleanupUsesOriginalSenderAndExactClientMapping() = fixture { fixture, root ->
        val queue = VoiceUploadQueue(context, fixture)
        val prepared = queue.prepare(source(), clip, 1200)
        queue.uploadStarted(prepared.requestId, prepared.source.owner, true)
        val confirmed = queue.objectConfirmed(prepared.requestId, id(), clip.size.toLong(), prepared.sha256)
        val original = confirmed.row(); val serverId = id()
        val owner = prepared.source.owner
        val scope = "live|" + owner.profile + "|" + owner.deployment + "|" + owner.user + "|personal"
        val file = File(root, prepared.requestId + "/clip.m4a")
        queue.eraseMessages(scope, serverId, listOf(original.copy(id = serverId, senderId = id())))
        assertTrue("A peer's coincident client ID is not our cleanup proof", file.isFile)
        queue.eraseMessages(scope, serverId, listOf(original.copy(id = serverId, conversationId = id())))
        assertTrue("Another conversation's coincident client ID is not our cleanup proof", file.isFile)
        queue.eraseMessages(scope, serverId, listOf(original.copy(id = serverId, attachmentId = id())))
        assertTrue("An incompatible object is not our cleanup proof", file.isFile)
        queue.assertMessageWritable(owner, prepared.source.conversation, prepared.clientMsgId)
        assertFalse("Conflicting holder metadata cannot freeze the original intent", queue.get(prepared.requestId).frozen)
        queue.eraseMessages(scope, serverId, listOf(original.copy(id = serverId, state = "received")))
        assertFalse(file.exists())
        assertTrue(queue.get(prepared.requestId).frozen)
        assertEquals(VoiceUploadPhase.CANCELLED, queue.get(prepared.requestId).phase)
    }

    @Test fun interruptedCopyRemainsAttributableForExactAccountErasure() = fixture { fixture, root ->
        val original = source()
        val failing = VoiceUploadQueue(context, fixture) { step ->
            if (step == VoiceUploadCheckpoint.CLIP_SYNCED) throw IOException("synthetic interrupted copy")
        }
        try { failing.prepare(original, clip, 1200); fail("Fault must prevent publication") }
        catch (_: IOException) { }
        val queue = VoiceUploadQueue(context, fixture)
        val owned = queue.list().entries.single()
        assertEquals(original.owner, owned.intent!!.source.owner)
        queue.eraseAccount(original.owner.profile, original.owner.deployment, original.owner.user)
        assertTrue(queue.list().entries.isEmpty())
        assertTrue(root.listFiles().orEmpty().all { it.name == ".queue.lock" || it.name.startsWith(".account-erased-") })
    }

    @Test fun completedAccountFloorRejectsLatePrepareAndLateObjectPromotion() = fixture { fixture, root ->
        val original = source()
        val queue = VoiceUploadQueue(context, fixture)
        val prepared = queue.prepare(original, clip, 1200)
        queue.uploadStarted(prepared.requestId, original.owner, true)
        val interruptedErase = VoiceUploadQueue(context, fixture) { step ->
            if (step == VoiceUploadCheckpoint.ERASURE_FLOOR_SYNCED) throw IOException("synthetic failure after permanent deny floor")
        }
        try { interruptedErase.eraseAccount(original.owner.profile, original.owner.deployment, original.owner.user); fail("Erasure fault required") }
        catch (_: IOException) { }
        val restored = VoiceUploadQueue(context, fixture)
        try { restored.prepare(original, clip, 1200); fail("Late prepare after completed account must be rejected") }
        catch (_: IllegalStateException) { }
        try { restored.objectConfirmed(prepared.requestId, id(), prepared.size, prepared.sha256); fail("Late response cannot update an erased owner") }
        catch (_: IllegalStateException) { }
        try { restored.assertNetworkAllowed(prepared.requestId, original.owner); fail("Erased owner cannot obtain HTTP admission") }
        catch (_: IllegalStateException) { }
        assertTrue(root.listFiles().orEmpty().any { it.name.startsWith(".account-erased-") })
        restored.eraseAccount(original.owner.profile, original.owner.deployment, original.owner.user)
        assertTrue(restored.list().entries.isEmpty())
        try { restored.prepare(original, clip, 1200); fail("The floor survives successful removal of all tasks") }
        catch (_: IllegalStateException) { }
    }

    @Test fun durableTerminalBeforeUnlinkResumesLocalCleanupWithoutHttp() = fixture { fixture, root ->
        val interrupted = VoiceUploadQueue(context, fixture) { step ->
            if (step == VoiceUploadCheckpoint.TERMINAL_SYNCED) throw IOException("synthetic failure before unlink")
        }
        val original = source()
        val prepared = interrupted.prepare(original, clip, 1200)
        try { interrupted.cancelUnsubmitted(prepared.requestId, original.owner); fail("Terminal fault required") }
        catch (_: IOException) { }
        val file = File(root, prepared.requestId + "/clip.m4a")
        assertTrue(file.exists())
        val entry = VoiceUploadQueue(context, fixture).list().entries.single()
        assertEquals(VoiceUploadPhase.CANCELLED, entry.intent!!.phase)
        assertTrue(entry.problem.isEmpty())
        assertFalse(file.exists())
    }
}
