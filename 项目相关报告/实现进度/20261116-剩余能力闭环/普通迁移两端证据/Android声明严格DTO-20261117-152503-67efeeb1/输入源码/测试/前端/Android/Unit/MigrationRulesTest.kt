package com.jobs.im

import org.junit.Assert.*
import org.junit.Test

class MigrationRulesTest {
    @Test fun originalJsonSlicesPreserveEscapedBracesAndNestedArrays() {
        val message="""{"text":"quoted } ] \" and \u003c","nested":[{"value":1}]}"""
        val raw="""{"format":"x","manifestDigest":"x","messages":[$message],"objects":[]}"""
        assertEquals(listOf(message),MigrationProtocol.rawMessages(raw))
    }
    @Test fun duplicateRootOrTrailingDataIsRejected() {
        for(raw in listOf("""{"format":"x","manifestDigest":"x","messages":[],"messages":[],"objects":[]}""","""{"format":"x","manifestDigest":"x","messages":[],"objects":[]}{}""")) {
            assertTrue(runCatching{MigrationProtocol.rawMessages(raw)}.isFailure)
        }
    }
    @Test fun base64RequiresCanonicalWireEncoding() {
        val bytes="原件 byte".toByteArray()
        assertArrayEquals(bytes,MigrationProtocol.decode(MigrationProtocol.encode(bytes),100))
        assertArrayEquals(bytes,MigrationProtocol.decode(MigrationProtocol.encode(bytes,true),100,true))
        assertTrue(runCatching{MigrationProtocol.decode("YQ",100)}.isFailure)
        assertTrue(runCatching{MigrationProtocol.decode("YQ==",100,true)}.isFailure)
    }
    @Test fun invalidUtf8AndOversizedDecodeFailClosed() {
        assertTrue(runCatching{MigrationProtocol.utf8(byteArrayOf(0xc3.toByte(),0x28))}.isFailure)
        assertTrue(runCatching{MigrationProtocol.decode("YWJj",2)}.isFailure)
    }
    @Test fun hashUsesOriginalBytes() {
        assertEquals("ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",MigrationProtocol.hash("abc".toByteArray()))
        assertNotEquals(MigrationProtocol.hash("\\u003c".toByteArray()),MigrationProtocol.hash("<".toByteArray()))
    }
    @Test fun allMigrationLanguagesHaveEveryEntry() {
        for(language in listOf("zh-Hans","zh-Hant","bo","en","ja","fr","es","de","ru","ko")) for(key in listOf("title","help","archive","original","unknown","local","connect")) assertNotEquals(key,MigrationText.text(language,key))
    }
}
