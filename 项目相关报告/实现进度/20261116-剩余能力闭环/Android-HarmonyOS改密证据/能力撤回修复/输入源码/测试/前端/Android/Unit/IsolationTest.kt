package com.jobs.im

import org.junit.Assert.*
import org.junit.Test

class IsolationTest {
    @Test
    fun certificateInputIsBoundedBeforeWholeFileAllocation() {
        assertEquals(65536, readLimited(java.io.ByteArrayInputStream(ByteArray(65536)), 65536).size)
        assertThrows(IllegalArgumentException::class.java) {
            readLimited(java.io.ByteArrayInputStream(ByteArray(65537)), 65536)
        }
    }

    @Test
    fun sameServerNameCannotMergeProfiles() {
        assertNotEquals(
            Isolation.namespace("a", "same", "u", false),
            Isolation.namespace("b", "same", "u", false),
        )
    }

    @Test
    fun demoCanNeverEnterLiveTransport() {
        val row =
            MessageRow(
                "k",
                "live|p|d|u",
                "c",
                "m",
                senderId = "u",
                text = "pending",
                state = "pending",
            )
        assertTrue(Isolation.mayTransmit(row, row.namespace, false))
        assertFalse(Isolation.mayTransmit(row, row.namespace, true))
        assertFalse(Isolation.mayTransmit(row, "another-profile", false))
        assertFalse(Isolation.mayTransmit(row.copy(state = "sent"), row.namespace, false))
    }

    @Test
    fun restoreEpochChangeCannotResendOldOutbox() {
        val row =
            MessageRow(
                "k",
                "scope",
                "c",
                "original-id",
                senderId = "u",
                text = "old",
                state = "pending",
                restoreEpoch = "1",
            )
        assertTrue(Isolation.mayTransmit(row, "scope", false, "1"))
        assertFalse(Isolation.mayTransmit(row, "scope", false, "2"))
        assertFalse(Isolation.mayTransmit(row.copy(state = "frozen"), "scope", false, "1"))
        assertNotEquals(
            Isolation.namespace("p", "d", "u", false, "1"),
            Isolation.namespace("p", "d", "u", false, "2"),
        )
        assertEquals("original-id", row.clientMsgId.ifBlank { row.id })
    }

    @Test
    fun endpointsRejectSecretsAndInsecureSchemes() {
        assertEquals("https://chat.example.test", Isolation.endpoint("https://chat.example.test/"))
        listOf(
                "http://chat.example.test",
                "https://u:secret@chat.example.test",
                "https://chat.example.test/?token=secret",
                "https://chat.example.test/#secret",
            )
            .forEach {
                assertThrows(IllegalArgumentException::class.java) { Isolation.endpoint(it) }
            }
    }

    @Test
    fun allTenLanguagesHaveEveryControlLabel() {
        assertEquals(10, Strings.languages.size)
        assertTrue(Strings.complete())
    }
}
