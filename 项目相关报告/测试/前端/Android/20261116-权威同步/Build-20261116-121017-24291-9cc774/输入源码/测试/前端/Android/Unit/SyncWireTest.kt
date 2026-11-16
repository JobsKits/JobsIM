package com.jobs.im

import java.util.Base64
import org.junit.Assert.*
import org.junit.Test

class SyncWireTest {
    @Test
    fun countersRemainExactAndRejectNonCanonicalOrOverflowValues() {
        assertEquals("9007199254740993000", SyncWire.counter("9007199254740993000"))
        listOf<Any?>(1, "01", "-1", "9223372036854775808", null).forEach {
            assertThrows(SyncFailure::class.java) { SyncWire.counter(it) }
        }
    }

    @Test
    fun clientMessageIdIsOpaqueAndBoundedByUtf8Bytes() {
        val cli = "0123456789abcdef0123456789abcdef"
        assertEquals(cli, SyncWire.clientMessageId(cli))
        assertThrows(SyncFailure::class.java) { SyncWire.id(cli) }
        assertEquals("你".repeat(42), SyncWire.clientMessageId("你".repeat(42)))
        listOf("short", "你".repeat(43)).forEach {
            assertThrows(SyncFailure::class.java) { SyncWire.clientMessageId(it) }
        }
    }

    @Test
    fun originalEnvelopeHashUsesOriginalBytesRatherThanReserializedJson() {
        val raw = "{\"text\":\"\\u4f60\\u597d\", \"position\":\"9007199254740993000\"}"
        val bytes = raw.toByteArray()
        val encoded = Base64.getUrlEncoder().withoutPadding().encodeToString(bytes)
        assertEquals(raw, SyncWire.originalBytes(encoded, SyncWire.hash(bytes)))
        assertThrows(SyncFailure::class.java) { SyncWire.originalBytes(encoded, "0".repeat(64)) }
        assertThrows(SyncFailure::class.java) { SyncWire.originalBytes(encoded + "=", SyncWire.hash(bytes)) }
    }

    @Test
    fun invalidUtf8CannotPassEvenWhenItsByteDigestMatches() {
        val bytes = byteArrayOf(0xc3.toByte(), 0x28)
        val encoded = Base64.getUrlEncoder().withoutPadding().encodeToString(bytes)
        assertThrows(SyncFailure::class.java) { SyncWire.originalBytes(encoded, SyncWire.hash(bytes)) }
    }
}
