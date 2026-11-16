package com.jobs.im

import android.app.Application
import android.graphics.Bitmap
import android.graphics.Rect
import android.os.Bundle
import android.os.SystemClock
import android.view.MotionEvent
import android.view.accessibility.AccessibilityNodeInfo
import androidx.test.platform.app.InstrumentationRegistry
import java.io.File

class NativeForwardActions {
    private val instrumentation = InstrumentationRegistry.getInstrumentation()
    private val app = instrumentation.targetContext.applicationContext as Application

    fun nodes(): List<AccessibilityNodeInfo> {
        if (android.os.Build.VERSION.SDK_INT >= 33) instrumentation.uiAutomation.clearCache()
        val result = mutableListOf<AccessibilityNodeInfo>()
        fun walk(node: AccessibilityNodeInfo?) {
            if (node == null) return
            result.add(node)
            for (i in 0 until node.childCount) walk(node.getChild(i))
        }
        walk(instrumentation.uiAutomation.rootInActiveWindow)
        return result
    }

    fun matches(node: AccessibilityNodeInfo, text: String) =
        node.isVisibleToUser &&
            (node.text?.toString().orEmpty() +
                    node.contentDescription?.toString().orEmpty() +
                    node.hintText?.toString().orEmpty())
                .contains(text)

    fun scroll(forward: Boolean): Boolean {
        val action =
            if (forward) AccessibilityNodeInfo.ACTION_SCROLL_FORWARD
            else AccessibilityNodeInfo.ACTION_SCROLL_BACKWARD
        val moved =
            nodes().firstOrNull { it.isVisibleToUser && it.isScrollable }?.performAction(action)
                ?: false
        if (moved) Thread.sleep(120)
        return moved
    }

    fun top() {
        repeat(10) { if (!scroll(false)) return }
    }

    fun waitText(text: String) {
        repeat(100) {
            if (nodes().any { matches(it, text) }) return
            Thread.sleep(100)
        }
        error("Native forward text missing: $text")
    }

    fun tap(node: AccessibilityNodeInfo): Boolean {
        val rect = Rect()
        node.getBoundsInScreen(rect)
        val metrics = app.resources.displayMetrics
        if (!rect.intersect(0, 0, metrics.widthPixels, metrics.heightPixels)) return false
        var parent = node.parent
        while (parent != null) {
            if (parent.isScrollable) {
                val viewport = Rect()
                parent.getBoundsInScreen(viewport)
                if (!rect.intersect(viewport)) return false
            }
            parent = parent.parent
        }
        if (rect.width() <= 0 || rect.height() <= 0 || rect.height() > 900) return false
        val start = SystemClock.uptimeMillis()
        val down =
            MotionEvent.obtain(
                start,
                start,
                MotionEvent.ACTION_DOWN,
                rect.exactCenterX(),
                rect.exactCenterY(),
                0,
            )
        val up =
            MotionEvent.obtain(
                start,
                SystemClock.uptimeMillis(),
                MotionEvent.ACTION_UP,
                rect.exactCenterX(),
                rect.exactCenterY(),
                0,
            )
        try {
            instrumentation.sendPointerSync(down)
            instrumentation.sendPointerSync(up)
        } finally {
            down.recycle()
            up.recycle()
        }
        return true
    }

    fun click(text: String) {
        repeat(60) {
            val candidates =
                nodes()
                    .filter { matches(it, text) }
                    .sortedBy {
                        if (
                            it.text?.toString() == text || it.contentDescription?.toString() == text
                        )
                            0
                        else 1
                    }
            for (candidate in candidates) {
                var node: AccessibilityNodeInfo? = candidate
                while (node != null) {
                    if (node.className?.toString()?.contains("EditText") == true) break
                    if (
                        node.isClickable &&
                            node.isEnabled &&
                            (node.performAction(AccessibilityNodeInfo.ACTION_CLICK) ||
                                tap(candidate))
                    ) {
                        instrumentation.waitForIdleSync()
                        Thread.sleep(500)
                        return
                    }
                    node = node.parent
                }
            }
            if (it % 12 == 11) top() else scroll(true)
            Thread.sleep(100)
        }
        error("Native forward action missing: $text")
    }

    fun edit(label: String, value: String) {
        repeat(50) {
            for (candidate in nodes().filter { matches(it, label) }) {
                var node: AccessibilityNodeInfo? = candidate
                while (node != null) {
                    if (node.isEditable) {
                        val data = Bundle()
                        data.putCharSequence(
                            AccessibilityNodeInfo.ACTION_ARGUMENT_SET_TEXT_CHARSEQUENCE,
                            value,
                        )
                        if (node.performAction(AccessibilityNodeInfo.ACTION_SET_TEXT, data)) {
                            Thread.sleep(150)
                            return
                        }
                    }
                    node = node.parent
                }
            }
            Thread.sleep(100)
        }
        error("Native editable field missing: $label")
    }

    fun screenshot(name: String) {
        instrumentation.waitForIdleSync()
        Thread.sleep(300)
        val dir = File(app.getExternalFilesDir(null), "ForwardUiEvidence").apply { mkdirs() }
        File(dir, name).outputStream().use {
            instrumentation.uiAutomation
                .takeScreenshot()
                .compress(Bitmap.CompressFormat.PNG, 100, it)
        }
    }
}
