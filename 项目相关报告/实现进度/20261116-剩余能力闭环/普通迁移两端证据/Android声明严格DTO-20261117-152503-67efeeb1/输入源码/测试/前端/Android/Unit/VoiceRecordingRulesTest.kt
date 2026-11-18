package com.jobs.im

import org.junit.Assert.*
import org.junit.Test

class VoiceRecordingRulesTest {
    private val original = VoiceRecordingOwner("scope-a", "chat-a", "device-family-a", "admission-a", "7")

    @Test fun recordingAndSendingNeverGrantAnotherSend() {
        val lease = VoiceRecordingLease(original)
        assertTrue(VoiceRecordingRules.canSend(VoiceRecordingPhase.PREVIEW, lease, original))
        for (phase in VoiceRecordingPhase.entries.filter { it != VoiceRecordingPhase.PREVIEW })
            assertFalse(VoiceRecordingRules.canSend(phase, lease, original))
    }

    @Test fun sourceSwitchAndPermissionChangesRejectTheOriginalClip() {
        val lease = VoiceRecordingLease(original)
        val others = listOf(null, original.copy(namespace = "scope-b"), original.copy(conversation = "chat-b"),
            original.copy(binding = "device-family-b"), original.copy(admission = "admission-b"),
            original.copy(membership = "8"))
        for (owner in others) assertFalse(VoiceRecordingRules.canSend(VoiceRecordingPhase.PREVIEW, lease, owner))
    }

    @Test fun cancelledLeaseCannotResumeAfterReturningToIdenticalIdentity() {
        val lease = VoiceRecordingLease(original)
        lease.revoke()
        assertFalse(lease.permits(original))
        assertFalse(VoiceRecordingRules.canSend(VoiceRecordingPhase.PREVIEW, lease, original.copy()))
        assertTrue(VoiceRecordingLease(original).permits(original))
    }

    @Test fun emptyShortAndOversizedClipsCannotBecomePreviewCandidates() {
        assertFalse(VoiceRecordingRules.validClip(999, 100))
        assertFalse(VoiceRecordingRules.validClip(1000, 0))
        assertFalse(VoiceRecordingRules.validClip(1000, VoiceRecordingRules.MAX_BYTES.toLong() + 1))
        assertFalse(VoiceRecordingRules.validClip(VoiceRecordingRules.MAX_MILLIS.toLong() + 1, 100))
        assertTrue(VoiceRecordingRules.validClip(1000, 100))
        assertTrue(VoiceRecordingRules.validClip(VoiceRecordingRules.MAX_MILLIS.toLong(), VoiceRecordingRules.MAX_BYTES.toLong()))
    }

    @Test fun everyLanguageIncludesPermissionAndRecoveryActions() { assertTrue(VoiceRecordingText.complete()) }
}
