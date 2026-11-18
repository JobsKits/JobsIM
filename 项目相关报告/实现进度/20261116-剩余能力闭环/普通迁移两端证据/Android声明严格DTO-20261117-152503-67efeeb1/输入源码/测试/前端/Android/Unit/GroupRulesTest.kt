package com.jobs.im

import org.junit.Assert.*
import org.junit.Test

class GroupRulesTest {
    @Test fun observedGenerationsNeverBecomeFloatingPointOrZero() {
        assertEquals("9007199254740993", GroupRules.counter("9007199254740993"))
        for (value in listOf("0", "-1", "01", "9223372036854775808")) assertThrows(Exception::class.java) { GroupRules.counter(value) }
    }
    @Test fun invitationDiagnosticNeverContainsTheBearer() {
        assertEquals("GroupInvitation(redacted)", GroupInvitation("d", "g", "i", "synthetic-secret-share-token").toString())
    }
    @Test fun allTenLanguagesHaveCompleteGroupControls() { assertTrue(GroupText.complete()) }
}
