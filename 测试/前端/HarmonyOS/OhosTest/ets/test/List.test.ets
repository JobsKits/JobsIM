import { describe, it, expect } from '@ohos/hypium';
import { Isolation, Rules, ContactRequest, GroupMember, Conversation, SessionPointer, AuthContext, Message, Bootstrap, AnnouncementMutation, AuthRules, AuthConnection, AuthResult, AuthRecord } from '../../../../../../代码/前端/HarmonyOS/entry/src/main/ets/Data/Models';
import { Strings } from '../../../../../../代码/前端/HarmonyOS/entry/src/main/ets/Data/Strings';
import { SyncRules, SyncJSON, SyncIdentity, SyncRef } from '../../../../../../代码/前端/HarmonyOS/entry/src/main/ets/Data/SyncProtocol';
import { SelfRules, SelfOperation } from '../../../../../../代码/前端/HarmonyOS/entry/src/main/ets/Data/SelfProtocol';
import { SelfText } from '../../../../../../代码/前端/HarmonyOS/entry/src/main/ets/Data/SelfText';
import { ExportRules, ExportIntent } from '../../../../../../代码/前端/HarmonyOS/entry/src/main/ets/Data/ExportProtocol';
import { ExportText } from '../../../../../../代码/前端/HarmonyOS/entry/src/main/ets/Data/ExportText';
export default function testsuite(): void {
  describe('JobsIMIsolation', () => {
    it('formalExportTenLanguagesHaveNativeDeviceScope', 0, () => {
      for (const language of Strings.languages) { expect(ExportText.rows[language].length).assertEqual(ExportText.keys.length); expect(ExportText.rows[language].every((value: string) => value.length > 0)).assertTrue(); }
      expect(ExportText.text('en', 'exportLocalList').includes('device')).assertTrue();
    });
    it('formalExportFilenameRejectsDirectoryAndControl', 0, () => {
      expect(ExportRules.fileName("attachment; filename*=UTF-8''%E6%88%91%E7%9A%84.zip")).assertEqual('我的.zip');
      for (const raw of ['../secret.zip', 'a\\b.zip', 'bad\u202e.zip']) { let denied = false; try { ExportRules.fileName('attachment; filename="' + raw + '"'); } catch (_) { denied = true; } expect(denied).assertTrue(); }
    });
    it('formalExportOriginalNeverAcceptsChangedEpochOrStore', 0, () => {
      const row: ExportIntent = { id: '11111111-1111-4111-8111-111111111111', body: '{"requestId":"11111111-1111-4111-8111-111111111111","restoreEpoch":"2"}', hash: 'a'.repeat(64), physical: '22222222-2222-4222-8222-222222222222', created: '2026-11-16T12:00:00Z', job: '', cancelBody: '', cancelHash: '' };
      ExportRules.original(row, '2', row.physical); let denied = false; try { ExportRules.original(row, '3', row.physical); } catch (_) { denied = true; } expect(denied).assertTrue();
    });
    it('formalExportTruncated200DoesNotReachSave', 0, () => {
      let denied = false; try { ExportRules.zip(21, (_offset: number, size: number) => new Uint8Array(size)); } catch (_) { denied = true; } expect(denied).assertTrue();
    });
    it('formalExportClientSizeLimitPrecedesReading', 0, () => {
      let touched = false; let denied = false; try { ExportRules.zip(ExportRules.limit + 1, (_offset: number, size: number) => { touched = true; return new Uint8Array(size); }); } catch (_) { denied = true; } expect(denied).assertTrue(); expect(touched).assertFalse();
    });
    it('schemaTwoReferenceDoesNotBypassSchemaOne', 0, () => {
      const ref: SyncRef = { owner: 'sync', kind: 'draft', id: '11111111-1111-4111-8111-111111111111', subjectUserId: '22222222-2222-4222-8222-222222222222', conversationId: '33333333-3333-4333-8333-333333333333', revision: '99', revisionEpoch: '2', state: 'active' };
      let denied = false; try { SyncRules.ref(SyncJSON.parse(JSON.stringify(ref)), '2', '1'); } catch (_) { denied = true; }
      expect(denied).assertTrue(); expect(SyncRules.ref(SyncJSON.parse(JSON.stringify(ref)), '2', '2').kind).assertEqual('draft');
    });
    it('schemaTwoTimestampsAcceptOriginalRFC3339Offsets', 0, () => {
      SelfRules.timestamp('2026-11-16T12:34:56.123456789+00:00'); SelfRules.timestamp('2026-11-16T12:34:56+08:00');
      let denied = false; try { SelfRules.timestamp('2026-11-16 12:34:56'); } catch (_) { denied = true; }
      expect(denied).assertTrue();
    });
    it('schemaTwoReadOperationHasIndependentServerIdentity', 0, () => {
      const operation: SelfOperation = { id: '11111111-1111-4111-8111-111111111111', kind: 'read', conversation: '33333333-3333-4333-8333-333333333333', epoch: '2', method: 'PUT', route: '/conversations/33333333-3333-4333-8333-333333333333/read', body: '{"membershipGeneration":"1","observedMessageIds":[]}', hash: '', state: 'pending', result: '' };
      const response = '{"operationId":"22222222-2222-4222-8222-222222222222","status":"applied","current":false,"result":{"revisionEpoch":"2","version":"1","membershipGeneration":"1","observedMessageIds":[],"readPoint":{"epoch":"0","sequence":"0"},"countStatus":"not_computed","hasMoreEligible":false,"hasUnorderedEligible":false}}';
      expect(SelfRules.operationResult(response, operation)).assertEqual('completed');
    });
    it('schemaTwoEmptyDraftDoesNotInventOwnedText', 0, () => {
      const identity: SyncIdentity = { namespace: 'fixture', profile: 'trusted', deployment: '11111111-1111-4111-8111-111111111111', user: '22222222-2222-4222-8222-222222222222', source: 'personal', epoch: '2', device: '44444444-4444-4444-8444-444444444444' };
      const raw = '{"id":"33333333-3333-4333-8333-333333333333","scopeId":"11111111-1111-4111-8111-111111111111","sourceContextId":"personal","userId":"22222222-2222-4222-8222-222222222222","conversationId":"33333333-3333-4333-8333-333333333333","revisionEpoch":"2","version":"0","contentState":"empty","ownedText":null,"replyState":"none","reply":null,"writable":true}';
      expect(SelfRules.draft(raw, identity, '33333333-3333-4333-8333-333333333333').ownedText === null).assertTrue();
    });
    it('schemaTwoControlsProvideTenLanguageStates', 0, () => {
      for (const language of Strings.languages) { expect(SelfText.text(language, 'draft').length > 0).assertTrue(); expect(SelfText.state(language, 'pending').length > 0).assertTrue(); }
    });
    it('schemaOneCountersRemainExactAboveJavaScriptIntegerRange', 0, () => {
      expect(SyncRules.next('9007199254740993')).assertEqual('9007199254740994');
      let denied = false;
      try { SyncRules.counter('9223372036854775808'); } catch (_) { denied = true; }
      expect(denied).assertTrue();
    });
    it('schemaOneDeepComparisonDoesNotReplaceRawEnvelopeHash', 0, () => {
      expect(SyncRules.same('{ "a": 1 }', '{"a":1}')).assertTrue();
      expect(SyncRules.same('{"a":"9007199254740993"}', '{"a":"9007199254740994"}')).assertFalse();
    });
    it('schemaOneDurableForwardErasesTransientBodyAndPermission', 0, () => {
      const row = mentionFixture(); row.forwarded = true; row.forwardAdmission = 'temporary-proof'; row.attachmentURL = '/api/messages/source/attachment';
      const persisted = SyncRules.persist(row);
      expect(persisted.text).assertEqual(''); expect(persisted.forwardAdmission === undefined).assertTrue(); expect(persisted.attachmentURL === undefined).assertTrue();
    });
    it('completeRefreshPairUsesOneCompactSecretAndPreciseGeneration', 0, () => {
      const pair = AuthRules.pair(authFixture(), authConnection(), 'original-request');
      pair.generation = '9007199254740993';
      const restored = AuthRules.decode(AuthRules.encode(pair));
      expect(restored.refreshToken).assertEqual(pair.refreshToken);
      expect(AuthRules.next(restored.generation)).assertEqual('9007199254740994');
      expect(AuthRules.namespace(restored)).assertEqual(AuthRules.namespace(pair));
    });
    it('deviceReauthenticationCannotReuseOldOutboxNamespace', 0, () => {
      const pair = AuthRules.pair(authFixture(), authConnection(), 'original-request');
      const original = AuthRules.namespace(pair);
      pair.deviceId = '55555555-5555-4555-8555-555555555555';
      expect(AuthRules.namespace(pair) !== original).assertTrue();
    });
    it('refreshMustKeepDeviceFamilyAndAdvanceOriginalGeneration', 0, () => {
      const original = AuthRules.pair(authFixture(), authConnection(), 'original-request');
      const result = authFixture(); result.refreshGeneration = '2';
      result.token = 'rotated-access'; result.refreshToken = 'rotated-refresh';
      const next = AuthRules.pair(result, authConnection(), 'refresh-request');
      AuthRules.rotate(next, original);
      next.deviceId = '55555555-5555-4555-8555-555555555555';
      let denied = false;
      try { AuthRules.rotate(next, original); } catch (_) { denied = true; }
      expect(denied).assertTrue();
    });
    it('linkedInitialProofKeepsSavedClosureWithoutSecretsInRecord', 0, () => {
      const record: AuthRecord = { requestId: 'new-request', purpose: 'reauthenticate', username: 'alice', deviceName: 'HarmonyOS', otpRequired: true, state: 'pending', previousRequestId: 'old-r2', previousRequestIds: ['old-r1', 'old-r2'] };
      const body = AuthRules.initialBody(record, 'current-password-proof', '123456');
      expect(body.includes('"previousRequestIds":["old-r1","old-r2"]')).assertTrue();
      expect(JSON.stringify(record).includes('current-password-proof')).assertFalse();
      expect(JSON.stringify(record).includes('123456')).assertFalse();
    });
    it('forwardIntentContainsReferencesWithoutCopiedSourceBody', 0, () => {
      const source = mentionFixture(); source.contentVersion = '9007199254740997'; source.forwardAdmission = 'source-proof'; source.attachmentId = 'private-media';
      const target: Conversation = { id: 'dest', kind: 'direct', title: '', memberIds: [], cursor: '0', sendAdmission: 'target-proof' };
      const intent = Rules.forwardIntent(source, target, 'actor', '2', 'original-id', '2026-11-15T00:00:00Z');
      const body = Rules.outgoingBody(intent); intent.sendAttempted = true;
      expect(Rules.outgoingBody(intent)).assertEqual(body);
      expect(body.includes('9007199254740997')).assertTrue();
      expect(body.includes('private-media')).assertFalse();
      expect(body.includes('"text"')).assertFalse();
    });
    it('invalidForwardProjectionErasesContentAndMediaAuthority', 0, () => {
      const cached = mentionFixture(); cached.id = 'forward'; cached.forwarded = true; cached.forwardAdmission = 'transient'; cached.attachmentId = 'source-media'; cached.attachmentURL = '/api/messages/forward/attachment';
      expect(Rules.forwardPath(cached)).assertEqual('/messages/forward/attachment');
      const masked = Rules.unavailableForward(cached);
      expect(masked.text).assertEqual(''); expect(masked.forwardUnavailable).assertTrue(); expect(Rules.forwardPath(masked)).assertEqual('');
      expect(Rules.permanentForwardRejection('FORWARD_ADMISSION_CHANGED')).assertTrue();
      expect(Rules.permanentForwardRejection('HTTP 503 FORWARD_CLEANUP_PENDING')).assertFalse();
    });
    it('differentTrustedProfilesRemainIsolated', 0, () => {
      expect(Isolation.namespace('a', 'same', 'u', false) !== Isolation.namespace('b', 'same', 'u', false)).assertTrue();
    });
    it('previewCannotShareLiveNamespace', 0, () => {
      expect(Isolation.namespace('p', 'd', 'u', true) !== Isolation.namespace('p', 'd', 'u', false)).assertTrue();
    });
    it('restoredEpochCannotSharePreviousOutboxNamespace', 0, () => {
      expect(Isolation.namespace('p', 'd', 'u', false, '1') !== Isolation.namespace('p', 'd', 'u', false, '2')).assertTrue();
    });
    it('inboxOnlyIncludesOwnPendingIncomingRequests', 0, () => {
      const request: ContactRequest = { id: 'r', requesterId: 'peer', recipientId: 'me', state: 'pending' };
      expect(Rules.incoming(request, 'me')).assertTrue();
      expect(Rules.incoming(request, 'peer')).assertFalse();
      request.state = 'accepted';
      expect(Rules.incoming(request, 'me')).assertFalse();
    });
    it('readOnlyConversationRejectsNewWrites', 0, () => {
      const conversation: Conversation = { id: 'c', kind: 'group', title: 'g', memberIds: [], cursor: '0', readOnly: true };
      expect(Rules.writable(conversation)).assertFalse();
    });
    it('removalKeepsOriginalMembershipGeneration', 0, () => {
      const member: GroupMember = { id: 'u', username: 'u', displayName: 'u', role: 'member', membershipGeneration: '9007199254740993' };
      expect(Rules.removalPath('c', member)).assertContain('membershipGeneration=9007199254740993');
    });
    it('deviceAndRestoreCursorsDoNotCrossScopes', 0, () => {
      expect(Isolation.namespace('p', 'd', 'u', false, '1', 'a') !== Isolation.namespace('p', 'd', 'u', false, '1', 'b')).assertTrue();
      expect(Rules.cursorMatches('2', '2:9007199254740993')).assertTrue();
      expect(Rules.cursorMatches('2', '1:9007199254740993')).assertFalse();
    });
    it('secureSessionRestorationChecksAccountDeviceAndEpoch', 0, () => {
      const pointer: SessionPointer = { alias: 'fixture', account: 'a', device: 'd', epoch: '2', profile: 'p', deployment: 'i' };
      const context: AuthContext = { user: { id: 'a', username: 'a', displayName: 'A' }, deviceId: 'd', restoreEpoch: '2' };
      expect(Rules.sessionMatches(pointer, context)).assertTrue();
      context.deviceId = 'other';
      expect(Rules.sessionMatches(pointer, context)).assertFalse();
    });
    it('authenticationPayloadsKeepPurposesSeparate', 0, () => {
      const login = Rules.authBody('login', 'alice', 'fixture-password-long', '', 'request-fixture');
      const recover = Rules.authBody('recover', 'alice', 'fixture-password-long', 'fixture-code', 'request-fixture');
      expect(login.includes('"code"')).assertFalse();
      expect(recover.includes('"deviceName"')).assertFalse();
    });
    it('httpsAndSecretFreeAddressRequired', 0, () => {
      expect(Isolation.endpoint('https://chat.example.test/')).assertEqual('https://chat.example.test');
      let rejected: boolean = false;
      try { Isolation.endpoint('http://chat.example.test'); } catch (_) { rejected = true; }
      expect(rejected).assertTrue();
    });
    it('sendingAdmissionRequiresExplicitCurrentInstanceSupport', 0, () => {
      const supported: Bootstrap = { deploymentId: 'i', name: 'fixture', protocolVersion: '1', restoreEpoch: '1', features: { send_admission: true } };
      expect(Rules.admissionSupported(supported, 'i')).assertTrue();
      expect(Rules.admissionSupported(supported, 'other')).assertFalse();
      supported.features = {};
      expect(Rules.admissionSupported(supported, 'i')).assertFalse();
    });
    it('originalMessageKeepsItsSendingAdmission', 0, () => {
      const row: Message = { id: 'local', conversationId: 'c', senderId: 'a', clientMsgId: 'original', type: 'text', text: 'fixture', createdAt: '2026-11-15T00:00:00Z', version: '1', sequence: '0', deleted: false, pinned: false, favorite: false, reactions: [], restoreEpoch: '1', sendAdmission: 'original-admission' };
      expect(Rules.outgoingBody(row).includes('original-admission')).assertTrue();
      expect(row.sendAdmission).assertEqual('original-admission');
      row.sendAdmission = undefined;
      expect(Rules.outgoingBody(row).includes('sendAdmission')).assertFalse();
    });
    it('lookupUsesDedicatedFavoritesAndOpaqueCursor', 0, () => {
      expect(Rules.lookupPath('favorites', '', '', '')).assertEqual('/favorites?limit=50');
      const path = Rules.lookupPath('search', 'a&b', 'group/id', 'signed+cursor=');
      expect(path.includes('q=a%26b')).assertTrue();
      expect(path.includes('cursor=signed%2Bcursor%3D')).assertTrue();
    });
    it('structuredMentionsRequireCurrentMembersAndOwnerOnlyAll', 0, () => {
      const group: Conversation = { id: 'g', kind: 'group', title: 'g', ownerId: 'owner', memberIds: [], cursor: '0', state: 'active' };
      const peer: GroupMember = { id: 'peer', username: 'peer', displayName: 'Peer', role: 'member', membershipGeneration: '2' };
      expect(Rules.selectedMentions(['peer'], false, [peer], group, 'owner')[0]).assertEqual('peer');
      let denied = false;
      try { Rules.selectedMentions([], true, [peer], group, 'peer'); } catch (_) { denied = true; }
      expect(denied).assertTrue();
      denied = false;
      try { Rules.selectedMentions(['other'], false, [peer], group, 'owner'); } catch (_) { denied = true; }
      expect(denied).assertTrue();
    });
    it('originalMentionFieldsRemainInOriginalMessagePayload', 0, () => {
      const row = mentionFixture(); row.mentionedUserIds = ['peer']; row.sendAdmission = 'original';
      const body = Rules.outgoingBody(row);
      row.mentionedUserIds = ['other'];
      expect(body.includes('"mentionedUserIds":["peer"]')).assertTrue();
      expect(body.includes('"sendAdmission":"original"')).assertTrue();
      expect(body.includes('contentVersion')).assertFalse();
    });
    it('mentionUnreadDoesNotResurrectAfterReadingOrEditing', 0, () => {
      const row = mentionFixture(); row.mentionedMe = true;
      expect(Rules.mentionUnread(undefined, row, 'me', false)).assertTrue();
      row.version = '2';
      expect(Rules.mentionUnread(false, row, 'me', false)).assertFalse();
      expect(Rules.mentionUnread(true, row, 'me', true)).assertFalse();
      row.mentionedMe = false;
      expect(Rules.mentionUnread(true, row, 'me', false)).assertFalse();
    });
    it('announcementSourceUsesContentVersionNotInteractionVersion', 0, () => {
      const row = mentionFixture(); row.version = '9007199254740993'; row.contentVersion = '2';
      expect(Rules.sourceContentVersion(row)).assertEqual('2');
      row.contentVersion = undefined;
      expect(Rules.sourceContentVersion(row)).assertEqual('9007199254740993');
      row.contentUnavailable = true;
      let denied = false;
      try { Rules.sourceContentVersion(row); } catch (_) { denied = true; }
      expect(denied).assertTrue();
    });
    it('roleProtectedWritesNeedExplicitCapability', 0, () => {
      const bootstrap: Bootstrap = { deploymentId: 'i', name: 'fixture', protocolVersion: '1', restoreEpoch: '1', features: { role_admission: true } };
      expect(Rules.roleAdmissionSupported(bootstrap, 'i')).assertTrue();
      bootstrap.features = { send_admission: true };
      expect(Rules.roleAdmissionSupported(bootstrap, 'i')).assertFalse();
      expect(Rules.permanentMentionRejection('HTTP 403 MENTION_ALL_FORBIDDEN')).assertTrue();
    });
    it('announcementIntentStoresReferenceWithoutCopiedBody', 0, () => {
      const group: Conversation = { id: 'g', kind: 'group', title: 'g', memberIds: [], cursor: '0', state: 'active', roleGeneration: '7', membershipGeneration: '9007199254740993' };
      const source = mentionFixture(); source.text = 'body-not-in-operation'; source.contentVersion = '3';
      const intent = Rules.announcementIntent(group, source, '2', 'request-original');
      const body = JSON.parse(intent.body) as AnnouncementMutation;
      expect(intent.method).assertEqual('PUT'); expect(body.sourceVersion).assertEqual('3');
      expect(body.membershipGeneration).assertEqual('9007199254740993');
      expect(intent.body.includes(source.text)).assertFalse();
      source.contentVersion = '4';
      expect((JSON.parse(intent.body) as AnnouncementMutation).sourceVersion).assertEqual('3');
    });
    it('announcementClearKeepsOriginalRequestAndCasOnly', 0, () => {
      const group: Conversation = { id: 'g', kind: 'group', title: 'g', memberIds: [], cursor: '0', state: 'active', roleGeneration: '7', membershipGeneration: '3' };
      const intent = Rules.announcementIntent(group, undefined, '2', 'clear-original');
      const body = JSON.parse(intent.body) as AnnouncementMutation;
      expect(intent.method).assertEqual('DELETE'); expect(body.expectedVersion).assertEqual('2');
      expect(intent.body.includes('messageId')).assertFalse(); expect(body.requestId).assertEqual('clear-original');
    });
    it('nativeAnnouncementExpiryPresetsAreExplicit', 0, () => {
      const now = Date.parse('2026-11-15T00:00:00.000Z');
      expect(Rules.announcementExpiry('never', now)).assertEqual('');
      expect(Date.parse(Rules.announcementExpiry('hour', now)) - now).assertEqual(3600000);
      expect(Date.parse(Rules.announcementExpiry('day', now)) - now).assertEqual(86400000);
      expect(Date.parse(Rules.announcementExpiry('week', now)) - now).assertEqual(604800000);
    });
    it('unknownAnnouncementResultsCannotBeDiscardedAsRejected', 0, () => {
      expect(Rules.announcementRejected('timeout')).assertFalse();
      expect(Rules.announcementRejected('HTTP 503 ADMISSION_RECOVERY_REQUIRED')).assertFalse();
      expect(Rules.announcementRejected('HTTP 409 ANNOUNCEMENT_SOURCE_CHANGED')).assertTrue();
    });
    it('tenLanguagesHaveMessageAndRecoveryControls', 0, () => {
      expect(Strings.languages.length).assertEqual(10);
      Strings.languages.forEach((language: string) => {
        expect(Strings.text(language, 'message') !== 'message').assertTrue();
        expect(Strings.text(language, 'recover') !== 'recover').assertTrue();
        expect(Strings.text(language, 'contentUnavailable') !== 'contentUnavailable').assertTrue();
      });
    });
  });
}

function mentionFixture(): Message {
  return { id: 'source', conversationId: 'g', senderId: 'peer', clientMsgId: 'original', type: 'text', text: 'fixture', createdAt: '2026-11-15T00:00:00Z', version: '1', sequence: '1', deleted: false, pinned: false, favorite: false, reactions: [] };
}

function authConnection(): AuthConnection {
  return { base: 'https://instance.example.test', caPath: '', profile: '11111111-1111-4111-8111-111111111111', deployment: '22222222-2222-4222-8222-222222222222', epoch: '1' };
}

function authFixture(): AuthResult {
  return { token: 'fixture-access', refreshToken: 'fixture-refresh', familyId: '44444444-4444-4444-8444-444444444444', refreshGeneration: '1', deviceId: '66666666-6666-4666-8666-666666666666', user: { id: '33333333-3333-4333-8333-333333333333', username: 'alice', displayName: 'Alice' }, restoreEpoch: '1', sourceContextId: 'personal', accessExpiresAt: '2026-11-15T12:15:00Z', idleExpiresAt: '2026-12-15T12:00:00Z', absoluteExpiresAt: '2027-02-13T12:00:00Z' };
}
