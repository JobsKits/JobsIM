package com.jobs.im

import org.junit.Assert.*
import org.junit.Test

class OwnPasswordRulesTest {
    @Test fun passwordUsesNfcCodePointsAndRejectsReuse() {
        val result = OwnPasswordRules.normalized("original-password", "abcdefghijklmn" + "e\u0301", "abcdefghijklmn\u00e9")
        assertEquals("abcdefghijklmn\u00e9", result.second)
        assertThrows(IllegalArgumentException::class.java) { OwnPasswordRules.normalized("same-password-123", "same-password-123", "same-password-123") }
    }
    @Test fun proofBoundsAndConfirmationCannotBeSilentlyChanged() {
        for (value in listOf("short", "x".repeat(129), "a".repeat(14) + '\uD800')) {
            assertThrows(IllegalArgumentException::class.java) { OwnPasswordRules.normalized("original-password", value, value) }
        }
        assertThrows(IllegalArgumentException::class.java) { OwnPasswordRules.normalized("original-password", "replacement-password", "another-password") }
    }
    @Test fun unknownAndPreparedResponsibilityPauseOrdinaryBusiness() {
        val row = OwnPasswordIntent("id", "profile", "origin", "deployment", "1", "user", "username", "device", "family", "namespace", "created")
        assertFalse(OwnPasswordRules.blocked(row))
        assertTrue(OwnPasswordRules.blocked(row.copy(attempts = 1, state = "unknown")))
        assertTrue(OwnPasswordRules.blocked(row.copy(attempts = 2, state = "pending")))
        assertTrue(OwnPasswordRules.blocked(row.copy(attempts = 1, state = "changed")))
        for (state in listOf("retired", "rejected")) assertFalse(OwnPasswordRules.blocked(row.copy(attempts = 1, state = state)))
    }
    @Test fun credentialsCannotEnterDiagnosticToStringAndAllTenLanguagesAreComplete() {
        assertEquals("PasswordOwner(redacted)", PasswordOwner("u", "n", "d", "f", "s", "synthetic-not-a-real-token").toString())
        assertTrue(OwnPasswordText.complete())
    }
    @Test fun onlyDefinitePreAdmissionCodesCanUnpause() {
        assertTrue(OwnPasswordRules.definiteRejection(403,"PASSWORD_CHANGE_REJECTED"))
        assertFalse(OwnPasswordRules.definiteRejection(403,"DEPENDENCY_FORBIDDEN"))
        assertFalse(OwnPasswordRules.definiteRejection(401,"ACCESS_EXPIRED"))
        assertFalse(OwnPasswordRules.definiteRejection(400,"DEPENDENCY_FAILED"))
    }

}
