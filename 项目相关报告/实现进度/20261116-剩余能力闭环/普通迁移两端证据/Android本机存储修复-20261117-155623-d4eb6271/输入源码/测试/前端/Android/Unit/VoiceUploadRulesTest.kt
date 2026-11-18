package com.jobs.im

import java.util.UUID
import org.junit.Assert.*
import org.junit.Test

class VoiceUploadRulesTest {
    private fun id() = UUID.randomUUID().toString()
    private val owner = VoiceUploadOwner("", "connection-profile", "https://example.invalid", id(), id(), id(), id(), "7").let {
        it.copy(namespace = Isolation.namespace(it.profile, it.deployment, it.user, false, it.epoch, "personal", it.device))
    }
    private val source = VoiceUploadSource(owner, id(), "sa1.synthetic-original", "2", id())
    private val intent = VoiceUploadIntent(id(), id(), source, 128, "a".repeat(64), 1000)

    @Test fun originalOwnerIsStableAcrossProcessObjectsButNeverAcrossFamilyOrEpoch() {
        assertTrue(VoiceUploadRules.mayWrite(intent, owner.copy()))
        for (different in listOf(owner.copy(profile = "other"), owner.copy(origin = "https://other.invalid"),
                owner.copy(deployment = id()), owner.copy(user = id()), owner.copy(device = id()),
                owner.copy(family = id()), owner.copy(epoch = "8"), owner.copy(namespace = "other")))
            assertFalse(VoiceUploadRules.mayWrite(intent, different))
        assertFalse(VoiceUploadRules.mayWrite(intent.copy(frozen = true, freezeReason = "OWNER_CHANGED"), owner))
    }

    @Test fun quotaAndDiskShortageRefuseNewTaskBeforeHttp() {
        val reserve = VoiceUploadRules.SPACE_RESERVE
        assertTrue(VoiceUploadRules.capacity(0, 0, 0, 128, reserve + 256))
        assertFalse(VoiceUploadRules.capacity(32, 32, 0, 128, reserve + 256))
        assertFalse(VoiceUploadRules.capacity(0, 128, 0, 128, reserve + 256))
        assertFalse(VoiceUploadRules.capacity(0, 0, VoiceUploadRules.MAX_TOTAL_BYTES, 128, reserve + 256))
        assertFalse(VoiceUploadRules.capacity(0, 0, 0, 128, reserve + 255))
    }

    @Test fun preparedOrUnknownObjectCannotBeConvertedToAnOutgoingMessage() {
        assertThrows(IllegalStateException::class.java) { intent.row() }
        assertThrows(IllegalStateException::class.java) { intent.copy(phase = VoiceUploadPhase.UPLOAD_UNKNOWN, objectId = id()).row() }
    }

    @Test fun originalReplyAndBodyAreFixedBeforeMediaUploadCompletes() {
        val confirmed = intent.copy(phase = VoiceUploadPhase.OBJECT_CONFIRMED, objectId = id())
        val row = confirmed.row()
        assertEquals(source.replyTo, row.replyToId)
        assertEquals(intent.clientMsgId, row.clientMsgId)
        assertTrue(VoiceUploadRules.sameMessage(confirmed, row))
        assertFalse(VoiceUploadRules.sameMessage(confirmed, row.copy(replyToId = id())))
        assertFalse(VoiceUploadRules.sameMessage(confirmed, row.copy(clientMsgId = id())))
        assertFalse(VoiceUploadRules.sameMessage(confirmed, row.copy(sendAdmission = "replacement")))
    }

    @Test fun everyLanguageIncludesQueueAndExplicitRecoveryActions() { assertTrue(VoiceUploadText.complete()) }

    @Test fun namespaceMustExactlyMatchStableOwnerAndCannotContainAnInjectedComponent() {
        VoiceUploadRules.validate(source)
        assertThrows(IllegalArgumentException::class.java) { VoiceUploadRules.validate(source.copy(owner = owner.copy(namespace = owner.namespace + "|extra"))) }
        val injected = owner.copy(profile = "first|second").let {
            it.copy(namespace = Isolation.namespace(it.profile, it.deployment, it.user, false, it.epoch, "personal", it.device))
        }
        assertThrows(IllegalArgumentException::class.java) { VoiceUploadRules.validate(source.copy(owner = injected)) }
        VoiceUploadRules.validate(source.copy(owner = owner.copy(origin = "https://example.invalid/nested-base")))
    }

    @Test fun globalRequiresConversationSenderClientObjectAndCanonicalScopeButCrossesEpoch() {
        val confirmed = intent.copy(phase = VoiceUploadPhase.OBJECT_CONFIRMED, objectId = id())
        val server = id()
        val scope = "live|" + owner.profile + "|" + owner.deployment + "|" + owner.user + "|personal"
        val row = confirmed.row().copy(id = server, state = "received")
        assertEquals(VoiceUploadRules.messageIdentity(confirmed), VoiceUploadRules.globalIdentity(scope, server, row))
        assertTrue(VoiceUploadRules.matchesGlobal(confirmed, scope, server, row))
        assertFalse(VoiceUploadRules.matchesGlobal(confirmed, scope, server, row.copy(conversationId = id())))
        assertFalse(VoiceUploadRules.matchesGlobal(confirmed, scope, server, row.copy(senderId = id())))
        assertFalse(VoiceUploadRules.matchesGlobal(confirmed, scope, server, row.copy(attachmentId = id())))
        assertFalse(VoiceUploadRules.matchesGlobal(confirmed, scope, server, row.copy(attachmentId = "corrupt-object")))
        assertEquals(VoiceUploadRules.messageIdentity(confirmed), VoiceUploadRules.globalIdentity(scope, server, row.copy(attachmentId = "corrupt-object")))
        assertFalse(VoiceUploadRules.matchesGlobal(confirmed, scope, server, row.copy(namespace = row.namespace + "|extra")))
        assertNull(VoiceUploadRules.globalIdentity(scope, server, row.copy(senderId = id())))
        assertNull(VoiceUploadRules.globalIdentity(scope, server, row.copy(clientMsgId = "opaque")))
        assertNull(VoiceUploadRules.globalIdentity(scope, server, row.copy(conversationId = "not-a-conversation")))
        assertNull(VoiceUploadRules.globalIdentity(scope, server, row.copy(namespace = row.namespace + "|extra")))
        assertNotEquals(VoiceUploadRules.messageIdentity(confirmed), VoiceUploadRules.globalIdentity(scope, server, row.copy(conversationId = id())))
        val old = row.copy(namespace = Isolation.namespace(owner.profile, owner.deployment, owner.user, false, "1", "personal", id()) + "|sync-retained",
            text = "", attachmentId = "", retracted = true)
        assertTrue(VoiceUploadRules.matchesGlobal(confirmed, scope, server, old))
    }
}
