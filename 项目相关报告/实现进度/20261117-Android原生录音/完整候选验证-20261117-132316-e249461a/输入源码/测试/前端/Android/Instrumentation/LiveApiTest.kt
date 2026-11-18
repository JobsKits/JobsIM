package com.jobs.im

import android.util.Base64
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import java.util.UUID
import kotlinx.coroutines.runBlocking
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Assume.assumeTrue
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class LiveApiTest {
    @Test
    fun verifiedTlsTwoAccountMessageContract() = runBlocking {
        val arguments = InstrumentationRegistry.getArguments()
        val base = arguments.getString("liveUrl")
        val encodedCa = arguments.getString("caBase64")
        assumeTrue(
            "Live service and test CA are explicit inputs",
            !base.isNullOrEmpty() && !encodedCa.isNullOrEmpty(),
        )
        val ca = Base64.decode(encodedCa, Base64.DEFAULT)
        val anonymous = Api(Isolation.endpoint(base!!), ca = ca)
        val bootstrap = anonymous.request("/bootstrap")
        assertEquals(arguments.getString("deploymentId"), bootstrap.getString("deploymentId"))
        assertEquals(1, bootstrap.getInt("protocolVersion"))
        assertTrue(
            "Untrusted local test CA must fail closed",
            runCatching { Api(base).request("/bootstrap") }.isFailure,
        )
        val suffix = System.currentTimeMillis().toString()
        val password = "Native-test-" + UUID.randomUUID().toString()
        suspend fun account(name: String): JSONObject =
            anonymous.request(
                "/auth/register",
                "POST",
                JSONObject()
                    .put("username", name)
                    .put("password", password)
                    .put("deviceName", "JobsIM instrumentation")
                    .put("requestId", UUID.randomUUID().toString()),
            )
        val a = account("android_a_$suffix")
        val b = account("android_b_$suffix")
        val apiA = Api(base, a.getString("token"), ca)
        val apiB = Api(base, b.getString("token"), ca)
        val bId = b.getJSONObject("user").getString("id")
        try {
            val request =
                apiA.request(
                    "/contacts/requests",
                    "POST",
                    JSONObject().put("userId", bId).put("requestId", UUID.randomUUID().toString()),
                )
            apiB.request(
                "/contacts/requests/" + request.getString("id") + "/accept",
                "POST",
                JSONObject(),
            )
            val conversation =
                apiA.request(
                    "/conversations",
                    "POST",
                    JSONObject()
                        .put("kind", "direct")
                        .put("title", "Native integration")
                        .put("memberIds", org.json.JSONArray(listOf(bId))),
                )
            val snapshot = conversationSnapshot(apiA, conversation.getString("id"))
            val path = "/conversations/" + conversation.getString("id") + "/messages"
            val payload =
                JSONObject()
                    .put("sendAdmission", snapshot.getString("sendAdmission"))
                    .put("clientMsgId", UUID.randomUUID().toString())
                    .put("type", "text")
                    .put("text", "Native HTTPS fixture")
            val sent = apiA.request(path, "POST", payload)
            val duplicate = apiA.request(path, "POST", payload)
            assertEquals(sent.getString("id"), duplicate.getString("id"))
            val received = apiB.request(path + "?after=0&limit=100").getJSONArray("items")
            assertTrue(
                (0 until received.length()).any {
                    received.getJSONObject(it).getString("id") == sent.getString("id")
                }
            )
            val id = sent.getString("id")
            apiA.request(
                "/messages/$id",
                "PATCH",
                JSONObject()
                    .put("text", "Native edited fixture")
                    .put("expectedVersion", sent.getString("version")),
            )
            apiB.request("/messages/$id/reactions", "POST", JSONObject().put("emoji", "👍"))
            apiA.request("/messages/$id/pin", "PUT", JSONObject())
            apiB.request("/messages/$id/favorite", "PUT", JSONObject())
            val media =
                apiA.upload(
                    "native-fixture.txt",
                    "text/plain",
                    "Native attachment fixture".toByteArray(),
                )
            apiA.request(
                path,
                "POST",
                JSONObject()
                    .put("sendAdmission", snapshot.getString("sendAdmission"))
                    .put("clientMsgId", UUID.randomUUID().toString())
                    .put("type", "file")
                    .put("text", "native-fixture.txt")
                    .put("attachmentId", media.getString("id")),
            )
            apiA.request("/messages/$id", "DELETE")
            assertEquals(
                bId,
                apiB.request("/me").let { it.optJSONObject("user") ?: it }.getString("id"),
            )
        } finally {
            apiA.request("/auth/logout", "POST", JSONObject())
            apiB.request("/auth/logout", "POST", JSONObject())
        }
    }
}
