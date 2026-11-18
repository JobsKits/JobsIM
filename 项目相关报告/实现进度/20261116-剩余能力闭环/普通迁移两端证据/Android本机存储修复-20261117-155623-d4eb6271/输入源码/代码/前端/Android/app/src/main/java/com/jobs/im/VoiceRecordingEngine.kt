// Created by Jobs on 2026年11月17日，星期二.
package com.jobs.im

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.content.pm.PackageManager
import android.media.AudioAttributes
import android.media.AudioFocusRequest
import android.media.AudioManager
import android.media.AudioRecordingConfiguration
import android.media.MediaPlayer
import android.media.MediaRecorder
import android.media.MediaMetadataRetriever
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.os.SystemClock
import java.io.FileOutputStream

/** Foreground-only capture. No record or permission callback may automatically send a message. */
internal class VoiceRecordingEngine(context: Context, private val changed: (VoiceRecordingState) -> Unit) {
    private val app = context.applicationContext
    private val handler = Handler(Looper.getMainLooper())
    private val audio = app.getSystemService(AudioManager::class.java)
    private var recorder: MediaRecorder? = null
    private var recordingCallback: AudioManager.AudioRecordingCallback? = null
    private var output: FileOutputStream? = null
    private var player: MediaPlayer? = null
    private var focus: AudioFocusRequest? = null
    private var upload: VoiceRecordingUpload? = null
    private var started = 0L
    private var listening = false
    var state = VoiceRecordingState()
        private set
    init { runCatching { VoiceRecordingFiles.prune(app) } }
    private val noisy = object : BroadcastReceiver() {
        override fun onReceive(context: Context?, intent: Intent?) {
            if (intent?.action == AudioManager.ACTION_AUDIO_BECOMING_NOISY) interrupt()
        }
    }

    private fun publish(value: VoiceRecordingState) { state = value; changed(value) }

    fun begin(owner: VoiceRecordingOwner) {
        discard()
        if (!app.packageManager.hasSystemFeature(PackageManager.FEATURE_MICROPHONE)) {
            publish(VoiceRecordingState(VoiceRecordingPhase.ERROR, "unavailable"))
            return
        }
        try {
            check(takeFocus(AudioManager.AUDIOFOCUS_GAIN_TRANSIENT_EXCLUSIVE))
            val file = VoiceRecordingFiles.create(app)
            upload = VoiceRecordingUpload(VoiceRecordingLease(owner), file)
            output = FileOutputStream(file)
            @Suppress("DEPRECATION")
            val capture = if (Build.VERSION.SDK_INT >= 31) MediaRecorder(app) else MediaRecorder()
            recorder = capture
            capture.setAudioSource(MediaRecorder.AudioSource.MIC)
            capture.setOutputFormat(MediaRecorder.OutputFormat.MPEG_4)
            capture.setAudioEncoder(MediaRecorder.AudioEncoder.AAC)
            capture.setAudioChannels(1)
            capture.setAudioSamplingRate(44100)
            capture.setAudioEncodingBitRate(64000)
            capture.setMaxDuration(VoiceRecordingRules.MAX_MILLIS)
            capture.setMaxFileSize(VoiceRecordingRules.MAX_BYTES.toLong())
            capture.setOutputFile(checkNotNull(output).fd)
            capture.setOnErrorListener { active, _, _ ->
                handler.post { if (recorder === active) interrupt() }
            }
            capture.setOnInfoListener { active, what, _ ->
                if (what == MediaRecorder.MEDIA_RECORDER_INFO_MAX_DURATION_REACHED ||
                    what == MediaRecorder.MEDIA_RECORDER_INFO_MAX_FILESIZE_REACHED)
                    handler.post { if (recorder === active) stop(atLimit = true) }
            }
            if (Build.VERSION.SDK_INT >= 29) {
                val callback = object : AudioManager.AudioRecordingCallback() {
                    override fun onRecordingConfigChanged(configs: MutableList<AudioRecordingConfiguration>) {
                        if (recorder === capture && configs.any { it.isClientSilenced }) interrupt()
                    }
                }
                recordingCallback = callback
                capture.registerAudioRecordingCallback(app.mainExecutor, callback)
            }
            capture.prepare()
            capture.start()
            started = SystemClock.elapsedRealtime()
            listen()
            publish(VoiceRecordingState(VoiceRecordingPhase.RECORDING))
        } catch (_: Exception) {
            fail("unavailable")
        }
    }

    fun elapsedMillis(): Long = if (state.phase == VoiceRecordingPhase.RECORDING)
        (SystemClock.elapsedRealtime() - started).coerceIn(0, VoiceRecordingRules.MAX_MILLIS.toLong())
        else state.durationMillis

    fun stop(atLimit: Boolean = false) {
        val capture = recorder ?: return
        val duration = elapsedMillis()
        var stopped = false
        try {
            capture.stop()
            stopped = true
        } catch (_: Exception) {
            // At a platform limit, capture may already be stopped; accept only a final, readable audio container.
        } finally {
            releaseRecorder()
            abandonFocus()
        }
        val file = upload?.file
        val playable = file != null && runCatching {
            val metadata = MediaMetadataRetriever()
            try {
                metadata.setDataSource(file.absolutePath)
                metadata.extractMetadata(MediaMetadataRetriever.METADATA_KEY_HAS_AUDIO) == "yes" &&
                    (metadata.extractMetadata(MediaMetadataRetriever.METADATA_KEY_DURATION)?.toLongOrNull() ?: 0) > 0
            } finally { metadata.release() }
        }.getOrDefault(false)
        if ((!stopped && !atLimit) || file == null || !playable ||
            !VoiceRecordingRules.validClip(duration, file.length())) fail("tooShort")
        else publish(VoiceRecordingState(VoiceRecordingPhase.PREVIEW, durationMillis = duration))
    }

    fun play() {
        if (state.phase != VoiceRecordingPhase.PREVIEW) return
        if (player != null) { pause(); return }
        try {
            check(takeFocus(AudioManager.AUDIOFOCUS_GAIN_TRANSIENT))
            val preview = MediaPlayer()
            player = preview
            preview.setAudioAttributes(attributes())
            preview.setDataSource(checkNotNull(upload).file.absolutePath)
            preview.setOnPreparedListener { active ->
                if (player === active && state.phase == VoiceRecordingPhase.PREVIEW) {
                    try {
                        active.start()
                        publish(state.copy(playing = true))
                    } catch (_: Exception) { fail("unavailable") }
                }
            }
            preview.setOnCompletionListener { if (player === it) pause() }
            preview.setOnErrorListener { active, _, _ ->
                if (player === active) fail("unavailable")
                true
            }
            preview.prepareAsync()
            listen()
        } catch (_: Exception) { fail("unavailable") }
    }

    fun pause() {
        player?.release()
        player = null
        abandonFocus()
        publish(state.copy(playing = false))
    }

    fun prepareUpload(current: VoiceRecordingOwner?): VoiceRecordingUpload? {
        val candidate = upload ?: return null
        if (!VoiceRecordingRules.canSend(state.phase, candidate.lease, current)) {
            fail("contextChanged")
            return null
        }
        pause()
        publish(state.copy(phase = VoiceRecordingPhase.SENDING))
        return candidate
    }

    fun uploadFailed(candidate: VoiceRecordingUpload) {
        if (upload === candidate && candidate.lease.permits(candidate.lease.owner)) {
            if (candidate.file.isFile && candidate.file.length() > 0)
                publish(state.copy(phase = VoiceRecordingPhase.PREVIEW, error = "sendFailed"))
            else fail("unavailable")
        }
    }

    fun queued(candidate: VoiceRecordingUpload) { if (upload === candidate) discard() }

    fun interrupt() {
        if (upload != null) fail("interrupted")
    }

    fun discard() {
        upload?.lease?.revoke()
        releaseRecorder()
        player?.release()
        player = null
        abandonFocus()
        VoiceRecordingFiles.remove(app, upload?.file)
        upload = null
        publish(VoiceRecordingState())
    }

    private fun fail(key: String) {
        discard()
        publish(VoiceRecordingState(VoiceRecordingPhase.ERROR, key))
    }

    private fun releaseRecorder() {
        val previous = recorder
        recorder = null
        if (Build.VERSION.SDK_INT >= 29) recordingCallback?.let { callback ->
            runCatching { previous?.unregisterAudioRecordingCallback(callback) }
        }
        recordingCallback = null
        runCatching { previous?.release() }
        runCatching { output?.close() }
        output = null
    }

    private fun attributes(): AudioAttributes = AudioAttributes.Builder()
        .setUsage(AudioAttributes.USAGE_MEDIA).setContentType(AudioAttributes.CONTENT_TYPE_SPEECH).build()

    private fun takeFocus(gain: Int): Boolean {
        abandonFocus()
        lateinit var request: AudioFocusRequest
        request = AudioFocusRequest.Builder(gain).setAudioAttributes(attributes())
            .setAcceptsDelayedFocusGain(false)
            .setOnAudioFocusChangeListener({ change -> if (change < 0 && focus === request) interrupt() }, handler).build()
        focus = request
        return audio.requestAudioFocus(request) == AudioManager.AUDIOFOCUS_REQUEST_GRANTED
    }

    private fun listen() {
        if (listening) return
        val filter = IntentFilter(AudioManager.ACTION_AUDIO_BECOMING_NOISY)
        if (Build.VERSION.SDK_INT >= 33) app.registerReceiver(noisy, filter, Context.RECEIVER_NOT_EXPORTED)
        else { @Suppress("DEPRECATION") app.registerReceiver(noisy, filter) }
        listening = true
    }

    private fun abandonFocus() {
        focus?.let { audio.abandonAudioFocusRequest(it) }
        focus = null
        if (listening) { app.unregisterReceiver(noisy); listening = false }
    }
}
