// Created by Jobs on 2026年11月17日，星期二.
package com.jobs.im

data class VoiceRecordingOwner(
    val namespace: String,
    val conversation: String,
    val binding: String,
    val admission: String,
    val membership: String,
)

enum class VoiceRecordingPhase { READY, RECORDING, PREVIEW, SENDING, ERROR }

data class VoiceRecordingState(
    val phase: VoiceRecordingPhase = VoiceRecordingPhase.READY,
    val error: String = "",
    val durationMillis: Long = 0,
    val playing: Boolean = false,
)

class VoiceRecordingLease(val owner: VoiceRecordingOwner) {
    @Volatile private var active = true
    fun revoke() { active = false }
    fun permits(current: VoiceRecordingOwner?): Boolean = active && current == owner
}

object VoiceRecordingRules {
    const val MAX_MILLIS = 300_000
    const val MAX_BYTES = 8 * 1024 * 1024
    const val MIN_MILLIS = 1000L

    fun validClip(duration: Long, bytes: Long): Boolean =
        duration in MIN_MILLIS..MAX_MILLIS.toLong() && bytes in 1..MAX_BYTES.toLong()

    fun canSend(phase: VoiceRecordingPhase, lease: VoiceRecordingLease, current: VoiceRecordingOwner?): Boolean =
        phase == VoiceRecordingPhase.PREVIEW && lease.permits(current)
}
