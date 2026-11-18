package com.jobs.im

import org.junit.Assert.*
import org.junit.Test

class OwnDeletionRulesTest {
    @Test fun proofPasswordUsesNfcAndBounds() {
        assertEquals("caf\u00e9", OwnDeletionRules.password("cafe\u0301"))
        for (value in listOf("", "x".repeat(513))) assertThrows(IllegalArgumentException::class.java) { OwnDeletionRules.password(value) }
    }
    @Test fun cancelledOrCompletedDeletionNeverRevivesTheOldFamily() {
        val meta = OwnPasswordIntent("id", "p", "o", "d", "1", "u", "name", "device", "family", "namespace", "created")
        for (state in listOf("unknown", "pending", "requested", "processing", "completed", "cancel_pending", "cancelled", "retired")) assertTrue(OwnDeletionRules.blocked(OwnDeletionIntent(meta, 1, state)))
        assertFalse(OwnDeletionRules.blocked(OwnDeletionIntent(meta, 1, "rejected")))
    }
    @Test fun onlyExplicitPreAdmissionCodesCanDiscardAnUnknownCreation() {
        assertTrue(OwnDeletionRules.definiteRejection(400, "INVALID_ACCOUNT_DELETION"))
        assertTrue(OwnDeletionRules.definiteRejection(403, "DATA_RIGHTS_REJECTED"))
        assertFalse(OwnDeletionRules.definiteRejection(403, "DEPENDENCY_FORBIDDEN"))
        assertFalse(OwnDeletionRules.definiteRejection(401, "ACCESS_EXPIRED"))
    }
    @Test fun deletionControlsCoverAllTenNativeLanguages() { assertTrue(OwnDeletionText.complete()) }
}
