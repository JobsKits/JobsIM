package com.jobs.im

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import java.net.URL
import java.security.KeyStore
import java.security.cert.CertificateFactory
import java.security.cert.X509Certificate
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec
import javax.net.ssl.HttpsURLConnection
import javax.net.ssl.SSLContext
import javax.net.ssl.TrustManagerFactory
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import org.json.JSONObject

class TokenVault(private val context: Context) {
    private fun key(): SecretKey {
        val store = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        return (store.getKey("JobsIM.session", null) as? SecretKey)
            ?: KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore").run {
                init(
                    KeyGenParameterSpec.Builder(
                            "JobsIM.session",
                            KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT,
                        )
                        .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                        .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                        .build()
                )
                generateKey()
            }
    }

    private fun writeEncrypted(profile: String, plaintext: String) {
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.ENCRYPT_MODE, key())
        cipher.updateAAD(profile.toByteArray())
        val bytes = cipher.iv + cipher.doFinal(plaintext.toByteArray())
        val file =
            android.util.AtomicFile(
                java.io.File(context.noBackupFilesDir, Isolation.digest(profile) + ".session")
            )
        var stream: java.io.FileOutputStream? = null
        try {
            stream = file.startWrite()
            stream.write(bytes)
            stream.fd.sync()
            file.finishWrite(stream)
        } catch (error: Exception) {
            stream?.let { file.failWrite(it) }
            throw error
        }
    }

    fun save(profile: String, token: String) = writeEncrypted(profile, token)

    fun saveSession(session: NativeSession, commitId: String) =
        writeEncrypted(
            session.binding.profile,
            session.stored().put("commitId", commitId).toString(),
        )

    fun savedCommit(binding: SessionBinding): String =
        decrypt(binding.profile)
            ?.let {
                if (!it.startsWith("{")) "" else JSONObject(it).optString("commitId")
            }
            .orEmpty()

    private fun decrypt(profile: String): String? {
        val file =
            android.util.AtomicFile(
                java.io.File(context.noBackupFilesDir, Isolation.digest(profile) + ".session")
            )
        val bytes =
            try {
                file.openRead().use { readLimited(it, 128 * 1024) }
            } catch (_: java.io.FileNotFoundException) {
                return null
            }
        require(bytes.size > 28 && bytes.size <= 128 * 1024)
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.DECRYPT_MODE, key(), GCMParameterSpec(128, bytes.copyOfRange(0, 12)))
        cipher.updateAAD(profile.toByteArray())
        return String(cipher.doFinal(bytes.copyOfRange(12, bytes.size)))
    }

    fun loadLegacy(profile: String): String? = decrypt(profile)?.takeUnless { it.startsWith("{") }

    fun load(profile: String): String? =
        decrypt(profile)?.let {
            if (it.startsWith("{")) JSONObject(it).getString("token") else it
        }

    fun loadSession(binding: SessionBinding): NativeSession? =
        decrypt(binding.profile)?.let {
            if (!it.startsWith("{")) null else NativeSession.restore(JSONObject(it), binding)
        }

    fun clear(profile: String) {
        android.util
            .AtomicFile(
                java.io.File(context.noBackupFilesDir, Isolation.digest(profile) + ".session")
            )
            .delete()
    }
    // A login restores the live account explicitly; cached ciphertext is never used across
    // profiles.
}

class ApiFailure(val status: Int, val code: String) : Exception("HTTP $status $code")

open class Api(private val base: String, private val token: String = "", ca: ByteArray? = null) {
    private val factory = ca?.let {
        val certificate =
            CertificateFactory.getInstance("X.509").generateCertificate(it.inputStream())
                as X509Certificate
        certificate.checkValidity()
        val keys =
            KeyStore.getInstance(KeyStore.getDefaultType()).apply {
                load(null)
                setCertificateEntry("instance-ca", certificate)
            }
        val manager =
            TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm()).apply {
                init(keys)
            }
        SSLContext.getInstance("TLS")
            .apply { init(null, manager.trustManagers, null) }
            .socketFactory
    }

    open suspend fun request(
        path: String,
        method: String = "GET",
        data: JSONObject? = null,
    ): JSONObject =
        withContext(Dispatchers.IO) {
            val connection = URL(base + "/api" + path).openConnection() as HttpsURLConnection
            try {
                connection.instanceFollowRedirects = false
                connection.connectTimeout = 3500
                connection.readTimeout = 5000
                connection.requestMethod = method
                factory?.let { connection.sslSocketFactory = it }
                connection.setRequestProperty("Accept", "application/json")
                if (token.isNotBlank())
                    connection.setRequestProperty("Authorization", "Bearer $token")
                if (data != null) {
                    connection.doOutput = true
                    connection.setRequestProperty("Content-Type", "application/json")
                    connection.outputStream.use { it.write(data.toString().toByteArray()) }
                }
                val code = connection.responseCode
                if (code !in 200..299) {
                    val errorBody =
                        connection.errorStream?.bufferedReader()?.use { it.readText() }.orEmpty()
                    val errorCode = runCatching {
                        JSONObject(errorBody).optJSONObject("error")?.optString("code")
                    }
                        .getOrNull()
                        .orEmpty()
                    throw ApiFailure(code, errorCode)
                }
                val body = connection.inputStream.bufferedReader().use { it.readText() }
                if (body.isBlank()) JSONObject() else JSONObject(body)
            } finally {
                connection.disconnect()
            }
        }

    open suspend fun upload(
        name: String,
        mime: String,
        bytes: ByteArray,
        path: String = "/media",
        fields: Map<String, String> = emptyMap(),
    ): JSONObject =
        withContext(Dispatchers.IO) {
            val boundary = "JobsIM" + java.util.UUID.randomUUID().toString()
            val connection = URL(base + "/api" + path).openConnection() as HttpsURLConnection
            try {
                connection.instanceFollowRedirects = false
                connection.connectTimeout = 3500
                connection.readTimeout = 30000
                connection.requestMethod = "POST"
                connection.doOutput = true
                factory?.let { connection.sslSocketFactory = it }
                connection.setRequestProperty("Authorization", "Bearer $token")
                connection.setRequestProperty(
                    "Content-Type",
                    "multipart/form-data; boundary=$boundary",
                )
                val safeName =
                    name.replace("\\", "_").replace("\"", "_").replace("\r", "_").replace("\n", "_")
                connection.outputStream.use { output ->
                    fields.forEach { (key, value) ->
                        require(key.matches(Regex("[A-Za-z]+")))
                        output.write(
                            ("--$boundary\r\nContent-Disposition: form-data; name=\"$key\"\r\n\r\n$value\r\n")
                                .toByteArray()
                        )
                    }
                    output.write(
                        ("--$boundary\r\nContent-Disposition: form-data; name=\"file\"; filename=\"$safeName\"\r\nContent-Type: $mime\r\n\r\n")
                            .toByteArray()
                    )
                    output.write(bytes)
                    output.write("\r\n--$boundary--\r\n".toByteArray())
                }
                if (connection.responseCode !in 200..299) {
                    val body =
                        connection.errorStream?.bufferedReader()?.use { it.readText() }.orEmpty()
                    throw ApiFailure(
                        connection.responseCode,
                        runCatching { JSONObject(body).getJSONObject("error").getString("code") }
                            .getOrDefault("UPLOAD_FAILED"),
                    )
                }
                JSONObject(connection.inputStream.bufferedReader().use { it.readText() })
            } finally {
                connection.disconnect()
            }
        }

    open suspend fun download(
        path: String,
        limit: Int = 64 * 1024 * 1024,
    ): Pair<ByteArray, String> =
        withContext(Dispatchers.IO) {
            require(path.startsWith("/") && !path.startsWith("//") && !path.contains(".."))
            val connection =
                URL(base + "/api" + path.removePrefix("/api")).openConnection()
                    as HttpsURLConnection
            try {
                connection.instanceFollowRedirects = false
                connection.connectTimeout = 3500
                connection.readTimeout = 30000
                factory?.let { connection.sslSocketFactory = it }
                connection.setRequestProperty("Authorization", "Bearer $token")
                if (connection.responseCode !in 200..299) {
                    val body =
                        connection.errorStream?.bufferedReader()?.use { it.readText() }.orEmpty()
                    throw ApiFailure(
                        connection.responseCode,
                        runCatching { JSONObject(body).getJSONObject("error").getString("code") }
                            .getOrDefault("DOWNLOAD_DENIED"),
                    )
                }
                val output = java.io.ByteArrayOutputStream()
                connection.inputStream.use { input ->
                    val buffer = ByteArray(8192)
                    while (true) {
                        val n = input.read(buffer)
                        if (n < 0) break
                        require(output.size() + n <= limit) { "DOWNLOAD_LIMIT" }
                        output.write(buffer, 0, n)
                    }
                }
                output.toByteArray() to (connection.contentType ?: "application/octet-stream")
            } finally {
                connection.disconnect()
            }
        }
}
