package com.jobs.im

import android.app.Application
import java.io.File
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

object OwnDeletionCleanup {
    suspend fun erase(app: Application, database: ChatDatabase, row: OwnDeletionIntent) {
        require(row.state == "completed")
        val meta = row.meta
        listOf(meta.deployment, meta.user).forEach { SyncWire.id(it) }
        val prefix = "live|${meta.profile}|${meta.deployment}|${meta.user}|"
        val spaces = (database.messages().accountNamespaces(prefix) + meta.namespace).map { it.removeSuffix("|sync-retained") }.distinct()
        require(spaces.all { it.startsWith(prefix) })
        withContext(Dispatchers.IO) {
            for (namespace in spaces) {
                for (root in listOf(File(app.cacheDir, "Attachments"), File(app.noBackupFilesDir, "personal-exports"), File(app.noBackupFilesDir, "AvatarSources"), File(app.noBackupFilesDir, "Migrations"), File(app.noBackupFilesDir,"ReceiptOriginals"))) {
                    val directory = File(root, Isolation.digest(namespace))
                    if (directory.exists()) { eraseOwned(directory); if (root.exists()) PersonalExportFiles.syncDirectory(root) }
                }
            }
        }
        VoiceUploadQueue(app).eraseAccount(meta.profile, meta.deployment, meta.user)
        // Files are removed before deleting their index. A failed unlink keeps
        // the original namespace list available for the next completion query.
        database.withTransaction {
            val dao = database.messages()
            dao.eraseAccountMessages(prefix); dao.eraseAccountStates(prefix); dao.eraseAccountOperations(prefix)
            dao.eraseAccountSync(prefix); dao.eraseAccountScopes(prefix)
        }
    }
    private fun eraseOwned(file: File) {
        require(file.canonicalFile == file.absoluteFile) { "DELETION_STORAGE_BINDING_CHANGED" }
        if (file.isDirectory) file.listFiles()?.forEach { eraseOwned(it) } ?: error("DELETION_STORAGE_UNAVAILABLE")
        check(file.delete() || !file.exists()) { "DELETION_STORAGE_PENDING" }
    }
}
