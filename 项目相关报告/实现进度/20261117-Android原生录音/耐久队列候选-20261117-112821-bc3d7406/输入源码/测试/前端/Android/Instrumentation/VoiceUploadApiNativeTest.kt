package com.jobs.im

import androidx.test.ext.junit.runners.AndroidJUnit4
import java.util.UUID
import kotlinx.coroutines.runBlocking
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class VoiceUploadApiNativeTest {
    private fun id() = UUID.randomUUID().toString()
    private val bytes = ByteArray(1024) { (it % 241).toByte() }
    private fun intent() = VoiceUploadIntent(id(), id(), VoiceUploadSource(
        VoiceUploadOwner("", "fixture-profile", "https://upload.invalid", id(), id(), id(), id(), "1").let {
            it.copy(namespace = Isolation.namespace(it.profile, it.deployment, it.user, false, it.epoch, "personal", it.device))
        },
        id(), "sa1.original", "2", id()), bytes.size.toLong(), VoiceUploadRules.digest(bytes), 1200,
        VoiceUploadPhase.UPLOAD_UNKNOWN)

    private class Fixture(val intent: VoiceUploadIntent) : Api("https://upload.invalid", "synthetic-original-token") {
        val calls = mutableListOf<Pair<String, JSONObject?>>()
        var uploads = 0
        var body = JSONObject()
        var failure: ApiFailure? = null
        var after: () -> Unit = {}
        var uploadFields: Map<String, String> = emptyMap()
        override suspend fun request(path: String, method: String, data: JSONObject?): JSONObject {
            calls.add(path to data)
            after()
            failure?.let { throw it }
            return body
        }
        override suspend fun upload(name: String, mime: String, bytes: ByteArray, path: String, fields: Map<String, String>): JSONObject {
            uploads++
            uploadFields = fields
            assertEquals(intent.name, name); assertEquals("audio/mp4", mime)
            assertEquals(intent.sha256, VoiceUploadRules.digest(bytes))
            after()
            failure?.let { throw it }
            return body
        }
        fun committed(objectId: String = UUID.randomUUID().toString()) = JSONObject().put("protocol", "stable-upload-v1")
            .put("requestId", intent.requestId).put("uploadState", "committed").put("id", objectId).put("name", intent.name)
            .put("contentType", "audio/mp4").put("size", intent.size).put("sha256", intent.sha256).put("url", "/api/media/" + objectId)
    }

    @Test fun missingResultIsReadOnlyAndKeepsExactOriginalTuple() = runBlocking {
        val original = intent(); val fake = Fixture(original); fake.failure = ApiFailure(404, "UPLOAD_NOT_FOUND")
        val adapter = VoiceUploadApi(fake, { original.source.owner }, { true })
        assertEquals(VoiceUploadResult.Missing, adapter.lookup(original))
        assertEquals(0, fake.uploads)
        val body = fake.calls.single().second!!
        assertEquals(setOf("requestId", "userId", "deviceId", "familyId", "restoreEpoch", "name", "size", "sha256"),
            body.keys().asSequence().toSet())
        assertEquals(original.requestId, body.getString("requestId"))
        assertEquals(original.source.owner.family, body.getString("familyId"))
        assertFalse(body.toString().contains("synthetic-original-token"))
    }

    @Test fun preparedNeverBecomesAUsableMediaUrl() = runBlocking {
        val original = intent(); val fake = Fixture(original); val objectId = id()
        fake.body = JSONObject().put("protocol", "stable-upload-v1").put("requestId", original.requestId)
            .put("uploadState", "prepared").put("id", objectId)
        val adapter = VoiceUploadApi(fake, { original.source.owner }, { true })
        assertEquals(VoiceUploadResult.Prepared(objectId), adapter.lookup(original))
        fake.body.put("url", "/api/media/" + objectId)
        try { adapter.lookup(original); fail("Prepared must not advertise usable delivery") }
        catch (_: IllegalStateException) { }
        assertEquals(0, fake.uploads)
    }

    @Test fun committedRequiresOriginalDigestSizeNameAndCanonicalUrl() = runBlocking {
        val original = intent(); val fake = Fixture(original); fake.body = fake.committed()
        val adapter = VoiceUploadApi(fake, { original.source.owner }, { true })
        assertTrue(adapter.uploadOriginal(original, bytes) is VoiceUploadResult.Committed)
        assertEquals(original.requestId, fake.uploadFields["requestId"])
        assertEquals(original.source.owner.device, fake.uploadFields["deviceId"])
        fake.body = fake.committed().put("sha256", "0".repeat(64))
        try { adapter.lookup(original); fail("A mismatched committed result must be rejected") }
        catch (_: IllegalStateException) { }
    }

    @Test fun lateFamilyChangeCannotPromoteTheOriginalResult() = runBlocking {
        val original = intent(); val fake = Fixture(original); fake.body = fake.committed()
        var current = original.source.owner
        fake.after = { current = current.copy(family = id()) }
        val adapter = VoiceUploadApi(fake, { current }, { true })
        try { adapter.lookup(original); fail("A successor family must not accept the old result") }
        catch (_: IllegalStateException) { }
        assertEquals(0, fake.uploads)
    }
}
