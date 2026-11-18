package com.jobs.im

import android.Manifest
import android.content.pm.PackageManager
import android.media.MediaMetadataRetriever
import android.os.SystemClock
import androidx.core.content.ContextCompat
import androidx.test.ext.junit.runners.AndroidJUnit4
import androidx.test.platform.app.InstrumentationRegistry
import java.io.File
import java.util.UUID
import org.junit.Assert.*
import org.junit.Assume.assumeTrue
import org.junit.Test
import org.junit.runner.RunWith

/** Opt-in real microphone tests. The caller supplies a disposable, foreground test installation. */
@RunWith(AndroidJUnit4::class)
class VoiceRecordingNativeTest {
    private val instrumentation get() = InstrumentationRegistry.getInstrumentation()
    private val context get() = instrumentation.targetContext

    private fun owner() = VoiceRecordingOwner(
        "JobsIM-test-voice-" + UUID.randomUUID(), UUID.randomUUID().toString(),
        "synthetic-original-device-family", "synthetic-original-admission", "7",
    )

    private fun prerequisites() {
        val arguments = InstrumentationRegistry.getArguments()
        assumeTrue("Real microphone capture needs explicit voiceCapture=true on an isolated installation",
            arguments.getString("voiceCapture") == "true")
        assumeTrue("A microphone is required", context.packageManager.hasSystemFeature(PackageManager.FEATURE_MICROPHONE))
        assumeTrue("Grant microphone permission in the disposable test installation before running",
            ContextCompat.checkSelfPermission(context, Manifest.permission.RECORD_AUDIO) == PackageManager.PERMISSION_GRANTED)
        assumeTrue("Keep the isolated target application foreground; this test does not launch the user's app",
            instrumentation.uiAutomation.rootInActiveWindow?.packageName?.toString() == context.packageName)
    }

    private fun main(action: () -> Unit) { instrumentation.runOnMainSync(action) }

    private fun clips(): Set<File> = File(context.cacheDir, "VoiceRecordings").listFiles()
        ?.filter { it.isFile && it.name.startsWith("JobsIM-voice-") && it.extension == "m4a" }
        ?.toSet().orEmpty()

    private fun capture(action: (VoiceRecordingEngine, VoiceRecordingOwner, MutableList<VoiceRecordingPhase>) -> Unit) {
        prerequisites()
        lateinit var engine: VoiceRecordingEngine
        val phases = mutableListOf<VoiceRecordingPhase>()
        val original = owner()
        main {
            engine = VoiceRecordingEngine(context) { phases.add(it.phase) }
            engine.begin(original)
            assertEquals("Actual MediaRecorder start must succeed; failure is not a passed capture test",
                VoiceRecordingPhase.RECORDING, engine.state.phase)
        }
        try { action(engine, original, phases) }
        finally { main { engine.discard() } }
    }

    @Test fun realAacStopProducesPreviewWithoutStartingUpload() = capture { engine, _, phases ->
        SystemClock.sleep(1400)
        main {
            engine.stop()
            assertEquals(VoiceRecordingPhase.PREVIEW, engine.state.phase)
            assertFalse(engine.state.playing)
            assertFalse("Stop must not prepare an upload", phases.contains(VoiceRecordingPhase.SENDING))
        }
        val file = clips().single()
        assertTrue(file.length() in 1..VoiceRecordingRules.MAX_BYTES.toLong())
        val metadata = MediaMetadataRetriever()
        try {
            metadata.setDataSource(file.absolutePath)
            assertEquals("yes", metadata.extractMetadata(MediaMetadataRetriever.METADATA_KEY_HAS_AUDIO))
            assertTrue((metadata.extractMetadata(MediaMetadataRetriever.METADATA_KEY_DURATION)?.toLongOrNull() ?: 0) > 0)
        } finally { metadata.release() }
        SystemClock.sleep(150)
        main { assertEquals(VoiceRecordingPhase.PREVIEW, engine.state.phase) }
    }

    @Test fun cancelReleasesRecorderAndDeletesOnlyTheOwnedClip() = capture { engine, _, _ ->
        val files = clips()
        assertEquals(1, files.size)
        val sibling = File(context.cacheDir, "JobsIM-test-voice-preserved-" + UUID.randomUUID()).apply { writeText("fixture") }
        try {
            main { engine.discard() }
            assertTrue(files.none(File::exists))
            assertTrue(sibling.isFile)
            main { assertEquals(VoiceRecordingPhase.READY, engine.state.phase) }
        } finally { sibling.delete() }
    }

    @Test fun changedFamilyRejectsClipAndReturningToOriginalCannotRestoreIt() = capture { engine, original, phases ->
        SystemClock.sleep(1400)
        main { engine.stop(); assertEquals(VoiceRecordingPhase.PREVIEW, engine.state.phase) }
        val files = clips()
        main {
            assertNull(engine.prepareUpload(original.copy(binding = "synthetic-successor-family")))
            assertEquals(VoiceRecordingPhase.ERROR, engine.state.phase)
            assertEquals("contextChanged", engine.state.error)
            assertNull(engine.prepareUpload(original))
            assertFalse(phases.contains(VoiceRecordingPhase.SENDING))
        }
        assertTrue(files.none(File::exists))
    }
}
