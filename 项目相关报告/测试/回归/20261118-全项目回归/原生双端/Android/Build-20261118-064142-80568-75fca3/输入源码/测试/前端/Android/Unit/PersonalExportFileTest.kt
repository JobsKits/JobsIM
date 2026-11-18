package com.jobs.im

import java.io.ByteArrayOutputStream
import java.io.File
import java.nio.ByteBuffer
import java.nio.ByteOrder
import java.util.zip.ZipEntry
import java.util.zip.ZipOutputStream
import org.junit.Assert.*
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

class PersonalExportFileTest {
    @get:Rule val temporary = TemporaryFolder()
    private fun zip(): ByteArray = ByteArrayOutputStream().also { bytes ->
        ZipOutputStream(bytes).use { output -> output.putNextEntry(ZipEntry("account-export.json")); output.write("{}".toByteArray()); output.closeEntry() }
    }.toByteArray()
    private fun file(bytes: ByteArray): File = temporary.newFile().also { it.writeBytes(bytes) }

    @Test fun serverFilenameIsRequiredAndCannotEscapeOrHideControls() {
        assertEquals("jobsim-personal-data.zip", PersonalExportFiles.filename("attachment; filename=jobsim-personal-data.zip"))
        assertEquals("my data.zip", PersonalExportFiles.filename("attachment; filename=fallback.zip; filename*=UTF-8''my%20data.zip"))
        for (bad in listOf(null, "inline; filename=test.zip", "attachment; filename=../bad.zip", "attachment; filename=bad.zip; filename=again.zip", "attachment; filename*=UTF-8''%00bad.zip", "attachment; filename*=UTF-8''%C2%85bad.zip", "attachment; filename*=UTF-8''%E2%80%AEbad.zip", "attachment; filename=not-a-zip.txt")) {
            assertThrows(PersonalExportFailure::class.java) { PersonalExportFiles.filename(bad) }
        }
    }

    @Test fun completeZipIsRequiredEvenWhenHttpResponseWasSuccessful() {
        val original = zip()
        PersonalExportFiles.validateZIP(file(original))
        for (bad in listOf(original.copyOf(original.size - 5), original + byteArrayOf(0), "not a zip".toByteArray())) {
            assertEquals("EXPORT_FILE_INVALID", assertThrows(PersonalExportFailure::class.java) { PersonalExportFiles.validateZIP(file(bad)) }.code)
        }
    }

    @Test fun directoryLimitsAreDistinctFromMalformedDirectoryCounts() {
        val large = zip()
        ByteBuffer.wrap(large).order(ByteOrder.LITTLE_ENDIAN).putInt(large.size - 22 + 12, (PersonalExportFiles.directoryLimit + 1).toInt())
        assertEquals("EXPORT_FILE_LIMIT", assertThrows(PersonalExportFailure::class.java) { PersonalExportFiles.validateZIP(file(large)) }.code)
        val mismatched = zip()
        ByteBuffer.wrap(mismatched).order(ByteOrder.LITTLE_ENDIAN).putShort(mismatched.size - 22 + 8, 0)
        assertEquals("EXPORT_FILE_INVALID", assertThrows(PersonalExportFailure::class.java) { PersonalExportFiles.validateZIP(file(mismatched)) }.code)
    }

    @Test fun zip64DirectoryHasExactSingleDiskBoundsWithoutWholeFileAllocation() {
        val raw = zip(); val old = ByteBuffer.wrap(raw).order(ByteOrder.LITTLE_ENDIAN); val end = raw.size - 22
        val record = ByteBuffer.allocate(56).order(ByteOrder.LITTLE_ENDIAN)
        record.putInt(0, 0x06064b50); record.putLong(4, 44); record.putShort(12, 45); record.putShort(14, 45)
        record.putLong(24, 1); record.putLong(32, 1); record.putLong(40, old.getInt(end + 12).toLong()); record.putLong(48, old.getInt(end + 16).toLong())
        val locator = ByteBuffer.allocate(20).order(ByteOrder.LITTLE_ENDIAN)
        locator.putInt(0, 0x07064b50); locator.putLong(8, end.toLong()); locator.putInt(16, 1)
        val finish = raw.copyOfRange(end, raw.size)
        ByteBuffer.wrap(finish).order(ByteOrder.LITTLE_ENDIAN).also { it.putShort(8, -1); it.putShort(10, -1) }
        val extended = raw.copyOfRange(0, end) + record.array() + locator.array() + finish
        PersonalExportFiles.validateZIP(file(extended))
        val invalid = extended.copyOf(); ByteBuffer.wrap(invalid).order(ByteOrder.LITTLE_ENDIAN).putLong(end + 24, 2)
        assertThrows(PersonalExportFailure::class.java) { PersonalExportFiles.validateZIP(file(invalid)) }
    }

    @Test fun everyLanguageDistinguishesCancelledSelectionFromSavedFile() {
        assertTrue(PersonalExportStrings.complete())
        Strings.languages.keys.forEach { language ->
            val saved = Strings.text(language, "exportSaved")
            val cancelled = Strings.text(language, "exportSaveCancelled")
            assertNotEquals(saved, cancelled)
            assertNotEquals("exportSaveCancelled", cancelled)
            assertTrue(Strings.text(language, "exportLimit").contains("1GiB"))
        }
    }
}
