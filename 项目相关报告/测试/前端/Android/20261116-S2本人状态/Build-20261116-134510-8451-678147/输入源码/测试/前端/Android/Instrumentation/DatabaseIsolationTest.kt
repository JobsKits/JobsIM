package com.jobs.im

import androidx.room.useWriterConnection
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import kotlinx.coroutines.runBlocking
import org.junit.Assert.*
import org.junit.Test
import org.junit.runner.RunWith

@RunWith(AndroidJUnit4::class)
class DatabaseIsolationTest {
    @Test
    fun liveOutboxSurvivesIndependentDemoWrites() = runBlocking {
        val context = InstrumentationRegistry.getInstrumentation().targetContext
        val live = ChatDatabase.open(context, false)
        val demo = ChatDatabase.open(context, true)
        val suffix = java.util.UUID.randomUUID().toString()
        val ns = "test-live-$suffix"
        val row =
            MessageRow(
                "$ns|m",
                ns,
                "test",
                "m",
                senderId = "test",
                text = "real",
                state = "pending",
            )
        try {
            live.messages().put(row)
            live.useWriterConnection { connection ->
                connection.usePrepared("PRAGMA synchronous") { statement ->
                    assertTrue(statement.step())
                    assertEquals(2L, statement.getLong(0))
                }
                connection.usePrepared("SELECT sqlite_version()") { statement ->
                    assertTrue(statement.step())
                    android.util.Log.i("JobsIMTest", "SQLite=" + statement.getText(0))
                }
            }
            demo.messages().put(row.copy(namespace = "test-demo-$suffix", text = "demo"))
            assertEquals("real", live.messages().pending(ns).single().text)
            assertTrue(demo.messages().pending(ns).isEmpty())
            assertEquals("m", live.messages().pending(ns).single().id)
        } finally {
            live.messages().remove(ns, "m")
            demo.messages().remove("test-demo-$suffix", "m")
            live.close()
            demo.close()
        }
    }
}
