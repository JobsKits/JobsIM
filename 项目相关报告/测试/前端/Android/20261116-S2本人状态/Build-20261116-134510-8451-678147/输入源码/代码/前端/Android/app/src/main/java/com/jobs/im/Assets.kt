package com.jobs.im

import android.content.Context
import android.net.Uri
import android.provider.OpenableColumns
import java.io.ByteArrayOutputStream

suspend fun readAsset(context: Context, uri: Uri, limit: Int): Triple<String, String, ByteArray> =
    kotlinx.coroutines.withContext(kotlinx.coroutines.Dispatchers.IO) {
        var name = "attachment"
        context.contentResolver
            .query(uri, arrayOf(OpenableColumns.DISPLAY_NAME), null, null, null)
            ?.use { if (it.moveToFirst()) name = it.getString(0) }
        var mime = context.contentResolver.getType(uri) ?: "application/octet-stream"
        val bytes =
            context.contentResolver.openInputStream(uri)?.use { input ->
                val out = ByteArrayOutputStream()
                val buffer = ByteArray(8192)
                while (true) {
                    val n = input.read(buffer)
                    if (n < 0) break
                    require(out.size() + n <= limit) { "ATTACHMENT_LIMIT" }
                    out.write(buffer, 0, n)
                }
                out.toByteArray()
            } ?: error("CANNOT_READ_FILE")
        if (
            bytes.size >= 8 &&
                bytes.copyOfRange(0, 8).contentEquals(byteArrayOf(-119, 80, 78, 71, 13, 10, 26, 10))
        )
            mime = "image/png"
        else if (
            bytes.size >= 3 &&
                bytes[0] == (-1).toByte() &&
                bytes[1] == (-40).toByte() &&
                bytes[2] == (-1).toByte()
        )
            mime = "image/jpeg"
        Triple(name, mime, bytes)
    }

fun readLimited(input: java.io.InputStream, limit: Int): ByteArray {
    require(limit > 0)
    val output = java.io.ByteArrayOutputStream(minOf(limit, 8192))
    val buffer = ByteArray(8192)
    while (true) {
        val count = input.read(buffer)
        if (count < 0) break
        require(count <= limit - output.size()) { "ATTACHMENT_LIMIT" }
        output.write(buffer, 0, count)
    }
    return output.toByteArray()
}
