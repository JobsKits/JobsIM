package com.jobs.im

import androidx.room.useReaderConnection
import androidx.sqlite.SQLiteConnection
import androidx.sqlite.driver.bundled.BundledSQLiteDriver
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import java.util.UUID
import kotlinx.coroutines.runBlocking
import org.json.JSONArray
import org.json.JSONObject
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class Migration8To9Test {
    private val schemaSHA = "c2d08158cfa211cc6300c45a83ae192ff3cc8739bdc9f2f729e0b30d43683404"
    private fun uuid() = UUID.randomUUID().toString()

    private fun execute(connection: SQLiteConnection, sql: String) {
        connection.prepare(sql).use { it.step() }
    }

    private fun insert(connection: SQLiteConnection, entity: JSONObject, values: Map<String, Any>) {
        val fields = entity.getJSONArray("fields")
        val names = (0 until fields.length()).map { fields.getJSONObject(it).getString("columnName") }.toSet()
        require(values.keys.all { it in names })
        for (index in 0 until fields.length()) {
            val field = fields.getJSONObject(index)
            require(!field.getBoolean("notNull") || field.has("defaultValue") || field.getString("columnName") in values) {
                "Missing original V8 field: " + field.getString("columnName")
            }
        }
        val columns = values.keys.toList()
        val sql = "INSERT INTO `${entity.getString("tableName")}` (" + columns.joinToString(",") { "`$it`" } + ") VALUES (" + columns.joinToString(",") { "?" } + ")"
        connection.prepare(sql).use { statement ->
            columns.forEachIndexed { index, column ->
                when (val value = values.getValue(column)) {
                    is String -> statement.bindText(index + 1, value)
                    is Number -> statement.bindLong(index + 1, value.toLong())
                    else -> error("Unsupported fixture type")
                }
            }
            statement.step()
        }
    }

    private fun originalRows(connection: SQLiteConnection, entity: JSONObject): List<Map<String, Any?>> {
        val fields = entity.getJSONArray("fields")
        val columns = (0 until fields.length()).map { fields.getJSONObject(it) }
        val sql = "SELECT " + columns.joinToString(",") { "`${it.getString("columnName")}`" } + " FROM `${entity.getString("tableName")}` ORDER BY `key`"
        return connection.prepare(sql).use { statement ->
            buildList {
                while (statement.step()) add(columns.mapIndexed { index, field ->
                    field.getString("columnName") to if (statement.isNull(index)) null else when (field.getString("affinity")) {
                        "TEXT" -> statement.getText(index)
                        "INTEGER" -> statement.getLong(index)
                        else -> error("Unexpected affinity in original V8 schema")
                    }
                }.toMap())
            }
        }
    }

    @Test
    fun originalSchema8MigratesWithoutChangingOriginalIntentsOrInventingSyncQualification() = runBlocking {
        val instrumentation = InstrumentationRegistry.getInstrumentation()
        val context = instrumentation.targetContext
        val bytes = instrumentation.context.assets.open("JobsIM-schema8.json").use { it.readBytes() }
        assertEquals(schemaSHA, SyncWire.hash(bytes))
        val schema = JSONObject(bytes.toString(Charsets.UTF_8)).getJSONObject("database")
        assertEquals(8, schema.getInt("version"))
        val rawEntities = schema.getJSONArray("entities")
        val entities = (0 until rawEntities.length()).associate { rawEntities.getJSONObject(it).let { entity -> entity.getString("tableName") to entity } }
        val name = "JobsIM-test-migration-${uuid()}.db"
        val file = context.getDatabasePath(name)
        assertFalse(file.exists())
        file.parentFile!!.mkdirs()
        val profile = uuid(); val deployment = uuid(); val user = uuid(); val device = uuid(); val peer = uuid(); val conversation = uuid()
        val ns = Isolation.namespace(profile, deployment, user, false, "1", "personal", device)
        val historyId = uuid(); val pendingId = uuid(); val frozenId = uuid(); val receivedForwardId = uuid()
        val forwardPayload = "{\n  \"clientMsgId\": \"opaque-original-forward\", \"type\": \"forward\",\n  \"forwardOfId\": \"${uuid()}\", \"forwardSourceVersion\": \"7\", \"forwardAdmission\": \"TEST-original-proof\",\n  \"restoreEpoch\": \"1\", \"sendAdmission\": \"TEST-original-admission\"\n}"
        val draft = "旧草稿逐字保留\nབོད་ / emoji ✉️ / literal \\n"
        val operationPayload = "{\n  \"userId\": \"$peer\"\n}"
        val scopeKey = Isolation.digest("$profile|https://localhost:36443|$deployment")
        var database: ChatDatabase? = null
        try {
            val before = mutableMapOf<String, List<Map<String, Any?>>>()
            BundledSQLiteDriver().open(file.absolutePath).use { connection ->
                execute(connection, "BEGIN IMMEDIATE")
                for ((table, entity) in entities) {
                    execute(connection, entity.getString("createSql").replace("\${TABLE_NAME}", table))
                    val indices = if (entity.has("indices")) entity.getJSONArray("indices") else JSONArray()
                    for (index in 0 until indices.length()) execute(connection, indices.getJSONObject(index).getString("createSql").replace("\${TABLE_NAME}", table))
                }
                val setup = schema.getJSONArray("setupQueries")
                for (index in 0 until setup.length()) execute(connection, setup.getString(index))
                fun message(id: String, state: String, text: String) = linkedMapOf<String, Any>(
                    "key" to "$ns|$id", "namespace" to ns, "conversationId" to conversation, "id" to id,
                    "clientMsgId" to "original-" + id, "senderId" to user, "type" to "text", "text" to text,
                    "attachmentId" to "", "replyToId" to "", "state" to state, "createdAt" to 1794770000123L,
                    "pinned" to 0L, "favorite" to 0L, "reaction" to "", "version" to "9007199254740993000",
                    "edited" to 0L, "retracted" to 0L, "restoreEpoch" to "1", "sequence" to "9007199254740992999",
                    "membershipGeneration" to "12", "sendAdmission" to "TEST-original-admission",
                )
                insert(connection, entities.getValue("messages"), message(historyId, "received", "旧正文\n普通本地历史，不代表sync资格"))
                val pending = message(pendingId, "pending", "").apply {
                    put("type", "forward"); put("forwarded", 1L); put("clientMsgId", "opaque-original-forward")
                    put("forwardAdmission", "TEST-original-proof"); put("forwardPayload", forwardPayload)
                }
                insert(connection, entities.getValue("messages"), pending)
                insert(connection, entities.getValue("messages"), message(frozenId, "frozen", "原冻结意图").apply { put("sendAdmission", "TEST-old-qualification") })
                insert(connection, entities.getValue("messages"), message(receivedForwardId, "received", "controlled old forward body").apply {
                    put("forwarded", 1L); put("attachmentId", uuid()); put("attachmentURL", "/api/messages/$receivedForwardId/attachment")
                    put("forwardAdmission", "TEST-received-source-proof"); put("replyToId", historyId)
                    put("mentionedUserIds", "[\"$peer\"]"); put("mentionAll", 1L); put("mentionedMe", 1L)
                })
                insert(connection, entities.getValue("conversation_state"), linkedMapOf("key" to "$ns|$conversation", "namespace" to ns, "conversationId" to conversation, "cursor" to "1:9007199254740992999", "draft" to draft, "unread" to 3L, "revision" to 7L, "draftMentions" to " [ \"$peer\" ] ", "draftMentionAll" to 1L))
                insert(connection, entities.getValue("operations"), linkedMapOf("key" to "$ns|contact", "namespace" to ns, "purpose" to "contact", "requestId" to uuid(), "payload" to operationPayload, "filePath" to "", "restoreEpoch" to "1"))
                insert(connection, entities.getValue("native_scopes"), linkedMapOf("key" to scopeKey, "prefix" to "live|$profile|$deployment|", "namespace" to ns, "deviceId" to device, "reason" to "SESSION_DEVICE_CHANGED"))
                execute(connection, "PRAGMA user_version=8")
                execute(connection, "COMMIT")
                for ((table, entity) in entities) before[table] = originalRows(connection, entity)
            }
            val migrated = ChatDatabase.open(context, false, name)
            database = migrated
            migrated.useReaderConnection { reader ->
                for ((table, entity) in entities) {
                    val columns = entity.getJSONArray("fields")
                    val sql = "SELECT " + (0 until columns.length()).joinToString(",") { "`${columns.getJSONObject(it).getString("columnName")}`" } + " FROM `$table` ORDER BY `key`"
                    val actual = reader.usePrepared(sql) { statement -> buildList {
                        while (statement.step()) add((0 until columns.length()).associate { index ->
                            val field = columns.getJSONObject(index)
                            field.getString("columnName") to if (statement.isNull(index)) null else if (field.getString("affinity") == "TEXT") statement.getText(index) else statement.getLong(index)
                        })
                    } }
                    val expected = before.getValue(table).map { row ->
                        if (table != "messages" || row["id"] != receivedForwardId) row else row + mapOf(
                            "text" to "", "attachmentId" to "", "attachmentURL" to "", "forwardAdmission" to "", "replyToId" to "",
                            "mentionedUserIds" to "[]", "mentionAll" to 0L, "mentionedMe" to 0L, "forwardUnavailable" to 1L,
                        )
                    }
                    assertEquals("Original columns changed in $table", expected, actual)
                    expected.zip(actual).forEach { (old, current) -> old.forEach { (column, value) ->
                        if (value is String) assertArrayEquals("Original UTF8 bytes changed: $table/$column", value.toByteArray(), (current[column] as String).toByteArray())
                    } }
                }
                reader.usePrepared("PRAGMA user_version") { assertTrue(it.step()); assertEquals(9L, it.getLong(0)) }
            }
            assertEquals(2, migrated.messages().outbox(ns).size)
            assertEquals("pending", migrated.messages().one(ns, pendingId)!!.state)
            assertEquals("frozen", migrated.messages().one(ns, frozenId)!!.state)
            assertArrayEquals(forwardPayload.toByteArray(), migrated.messages().one(ns, pendingId)!!.forwardPayload.toByteArray())
            assertArrayEquals(draft.toByteArray(), migrated.messages().state(ns, conversation)!!.draft.toByteArray())
            assertArrayEquals(operationPayload.toByteArray(), migrated.messages().operation(ns, "contact")!!.payload.toByteArray())
            assertTrue(migrated.messages().all(ns).all { it.projectionGuard.isEmpty() })
            assertTrue(migrated.messages().one(ns, receivedForwardId)!!.forwardUnavailable)
            assertFalse(migrated.messages().one(ns, receivedForwardId)!!.retracted)
            val identity = SyncIdentity(ns, profile, deployment, user, "1", device, uuid())
            val store = SyncStore(migrated)
            assertFalse(store.responsibility(identity))
            assertFalse(migrated.syncRecords().hasNamespace(ns))
            val registered = store.ensure(identity) { true }
            SyncWire.id(registered.getString("storeGeneration")); SyncWire.id(registered.getString("registrationRequestId"))
            assertFalse(registered.has("cursor")); assertEquals("", registered.getString("streamId"))
            assertFalse(store.projectionGuard(ns)!!.ready)
            assertArrayEquals(forwardPayload.toByteArray(), migrated.messages().one(ns, pendingId)!!.forwardPayload.toByteArray())
            migrated.close(); database = null
            val reopened = ChatDatabase.open(context, false, name); database = reopened
            assertEquals(registered.getString("storeGeneration"), SyncStore(reopened).ensure(identity) { true }.getString("storeGeneration"))
            assertEquals("frozen", reopened.messages().one(ns, frozenId)!!.state)
            assertArrayEquals(forwardPayload.toByteArray(), reopened.messages().one(ns, pendingId)!!.forwardPayload.toByteArray())
        } finally {
            database?.close()
            context.deleteDatabase(name)
        }
    }
}
