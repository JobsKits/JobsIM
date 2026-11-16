#!/usr/bin/env python3
"""Keep historical reports intact while regressing current forwarding backend."""
import argparse
from datetime import datetime, timedelta, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import uuid

helper = Path(__file__).resolve().parents[1] / 'mentions_announcements.py/mentions_announcements.py'
spec = importlib.util.spec_from_file_location('jobsim_forward_compat_http', helper)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class CurrentSuite(module.Suite):
    def export_rejection(self):
        self.epoch = self.call('GET', '/api/bootstrap')['restoreEpoch']
        actor = self.register('export_reason')
        conversation = self.call('POST', '/api/conversations', {'kind': 'self'}, actor)
        source = self.send(actor, conversation['id'], self.payload(actor, conversation['id'], 'Explicit forward export rejection'))
        self.send(actor, conversation['id'], {'clientMsgId': str(uuid.uuid4()), 'type': 'forward', 'forwardOfId': source['id'], 'forwardSourceVersion': source['contentVersion'], 'forwardAdmission': source['forwardAdmission'], 'restoreEpoch': self.epoch, 'sendAdmission': conversation['sendAdmission']})
        job = self.call('POST', '/api/me/exports', {'requestId': str(uuid.uuid4())}, actor, 202)['job']
        for _ in range(100):
            current = self.call('GET', '/api/me/exports/' + job['id'], session=actor)
            if current['state'] == 'failed' and current.get('cleanupState') == 'cleared':
                break
            time.sleep(.1)
        self.check(current['state'] == 'failed' and current.get('failureCode') == 'FORWARD_EXPORT_UNSUPPORTED', 'Experimental export explicitly reports unsupported forward holders')
        self.check(current.get('cleanupState') == 'cleared', 'No partial encrypted package is left available')
        self.check(self.sql("SELECT count(*) FROM message.export_holders h JOIN message.messages m ON m.id=h.message_id WHERE h.job_id='" + job['id'] + "' AND m.type='forward' AND h.state='active'") == '0', 'No forward reference was published under an incomplete holder graph')
        return {'httpAssertions': self.checks, 'semanticAssertions': self.assertions, 'explicitFailureCode': True, 'noPartialPackage': True, 'noUnprotectedForwardHolder': True}

    def announce_body(self, actor, conversation, source, **extra):
        body = super().announce_body(actor, conversation, source, **extra)
        body['roleGeneration'] = self.conversation(actor, conversation)['roleGeneration']
        return body

    def run(self):
        bootstrap = self.call('GET', '/api/bootstrap')
        self.epoch = bootstrap['restoreEpoch']
        self.check(all(bootstrap['features'].get(flag) is True for flag in ('send_admission', 'role_admission', 'mentions', 'announcements', 'forwarding')), 'Current implementation flags remain visible')
        owner, successor, member = [self.register(label) for label in ('compat_a', 'compat_b', 'compat_c')]
        for peer in (successor, member):
            self.friend(owner, peer)
        group = self.call('POST', '/api/conversations', {'kind': 'group', 'title': 'Forward compatibility', 'memberIds': [successor['user']['id'], member['user']['id']]}, owner)
        gid, prefix = group['id'], '/api/conversations/' + group['id']
        original_source_body = self.payload(owner, gid, 'Announcement compatibility source', mentionedUserIds=[member['user']['id']])
        source = self.send(owner, gid, original_source_body)
        self.check(self.message(member, source['id'])['mentionedMe'], 'Selected current member is mentioned')
        self.check(not self.message(successor, source['id'])['mentionedMe'], 'Unselected member has no reminder')
        self.call('PUT', '/api/blocks/' + owner['user']['id'], {}, member)
        hidden = self.message(member, source['id'])
        self.check(not hidden['mentionedMe'] and hidden['text'] == source['text'], 'Block suppresses reminder and retains common group body')
        self.call('DELETE', '/api/blocks/' + owner['user']['id'], session=member)
        plain = self.send(owner, gid, self.payload(owner, gid, '@displayName plain text'))
        self.check(not self.message(member, plain['id'])['mentionedMe'], 'Plain text does not synthesize mention')
        pending_all = self.payload(owner, gid, 'Old unsubmitted @all', mentionAll=True)
        pending_plain = self.payload(owner, gid, 'Old unsubmitted plain')
        publish = self.announce_body(owner, gid, source)
        applied = self.put_announcement(owner, gid, publish)
        pending = self.announce_body(owner, gid, source)
        pending_clear = {'requestId': str(uuid.uuid4()), 'expectedVersion': applied['result']['version'], 'membershipGeneration': group['membershipGeneration'], 'roleGeneration': group['roleGeneration']}
        member_floor = self.sql("SELECT string_agg(user_id::text||':'||membership_generation::text||':'||joined_at::text,',' ORDER BY user_id) FROM conversation.members WHERE conversation_id='" + gid + "'")
        self.call('PATCH', prefix, {'ownerId': successor['user']['id']}, owner)
        self.call('PATCH', prefix, {'ownerId': owner['user']['id']}, successor)
        returned = self.conversation(owner, gid)
        self.check(returned['roleGeneration'] == '3' and returned['sendAdmission'] != group['sendAdmission'], 'Role boomerang changes admission')
        self.check(member_floor == self.sql("SELECT string_agg(user_id::text||':'||membership_generation::text||':'||joined_at::text,',' ORDER BY user_id) FROM conversation.members WHERE conversation_id='" + gid + "'"), 'Role transition retains history floor')
        for body in (pending_all, pending_plain):
            self.check(self.send(owner, gid, body, 409)['error']['code'] == 'SEND_ADMISSION_CHANGED', 'Unsubmitted original token stays frozen')
            self.call('POST', prefix + '/message-results', body, owner, 404)
        self.check(self.put_announcement(owner, gid, pending, 409)['error']['code'] == 'QUALIFICATION_CHANGED', 'Original role cannot create after returning')
        self.check(self.call('DELETE', prefix + '/announcement', pending_clear, owner, 409)['error']['code'] == 'QUALIFICATION_CHANGED', 'Original role cannot clear after returning')
        self.check(self.put_announcement(owner, gid, publish)['operationId'] == publish['requestId'], 'Completed old role-bound publication replays fact first')
        self.check(self.call('POST', prefix + '/message-results', original_source_body, owner)['id'] == source['id'], 'Committed message history remains readable')
        missing = self.announce_body(owner, gid, source)
        del missing['roleGeneration']
        self.check(self.put_announcement(owner, gid, missing, 409)['error']['code'] == 'QUALIFICATION_CHANGED', 'Missing original role cannot create')
        all_sent = self.send(owner, gid, self.payload(owner, gid, 'Fresh current owner @all', mentionAll=True))
        self.check(self.message(successor, all_sent['id'])['mentionedMe'], 'New explicit current @all works')
        self.send(member, gid, self.payload(member, gid, 'Not owner @all', mentionAll=True), 403)
        self.call('PUT', '/api/messages/' + source['id'] + '/favorite', {}, member)
        self.call('POST', '/api/messages/' + source['id'] + '/reactions', {'emoji': 'ok'}, successor)
        self.call('PUT', '/api/messages/' + source['id'] + '/pin', {}, owner)
        active = self.announcement(member, gid)
        self.check(active['state'] == 'active' and active['message']['contentVersion'] == source['contentVersion'], 'Favorite/react/pin retain original announcement content')
        edited = self.call('PATCH', '/api/messages/' + source['id'], {'text': 'Updated current source', 'expectedVersion': self.message(owner, source['id'])['version'], 'requestId': str(uuid.uuid4())}, owner)
        stale = self.announcement(member, gid)
        self.check(stale['state'] == 'unavailable' and 'message' not in stale, 'Edit invalidates source reference without stale body')
        fresh_body = self.announce_body(owner, gid, edited)
        fresh = self.put_announcement(owner, gid, fresh_body)
        self.check(not self.put_announcement(owner, gid, publish)['current'], 'Old fact cannot roll newer announcement back')
        self.call('DELETE', '/api/messages/' + source['id'], session=owner)
        self.check(self.announcement(member, gid)['state'] == 'unavailable', 'Recall invalidates source under exclusive fence')
        clear = {'requestId': str(uuid.uuid4()), 'expectedVersion': fresh['result']['version'], 'membershipGeneration': returned['membershipGeneration'], 'roleGeneration': returned['roleGeneration']}
        self.call('DELETE', prefix + '/announcement', clear, owner)
        self.check(self.announcement(member, gid)['state'] == 'empty', 'Current role clear succeeds')
        next_source = self.send(owner, gid, self.payload(owner, gid, 'Expiry and row-lock source'))
        self.put_announcement(owner, gid, self.announce_body(owner, gid, next_source, expiresAt=(datetime.now(timezone.utc) + timedelta(seconds=2)).isoformat()))
        time.sleep(2.1)
        expired = self.announcement(member, gid)
        self.check(expired['state'] == 'expired' and 'message' not in expired, 'Expiry has no stale source body')
        self.race_source_edit(owner, gid, next_source, owner)
        return {'httpAssertions': self.checks, 'semanticAssertions': self.assertions, 'roleBoomerang': True, 'immutableOriginalOperations': True, 'structuredMentionsAndBlocks': True, 'interactionKeepsContentVersion': True, 'sourceEditRecallExpiryAndClear': True, 'parentRowSourceEditFenceRace': True}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--environment-file', type=Path, required=True)
    parser.add_argument('--export-only', action='store_true')
    parser.add_argument('--report', type=Path, help='Separate candidate report preserves earlier fixed-build evidence')
    args = parser.parse_args()
    environment = json.loads(args.environment_file.read_text())
    assert environment.get('forwardTestRealm') is True or environment.get('refreshTestRealm') is True
    if args.export_only:
        suite = CurrentSuite(environment)
        try:
            result = suite.export_rejection()
        finally:
            suite.logout()
        report = {'status': 'passed', 'testedAtUTC': datetime.now(timezone.utc).isoformat(), **result, 'strictHTTPS': True, 'binarySHA256': hashlib.sha256(Path(environment['binary']).read_bytes()).hexdigest(), 'scriptSHA256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(), 'scope': 'Experimental export only; forward chain-level export holders remain unsupported and explicitly rejected'}
        destination = args.report or Path(__file__).resolve().parents[4] / '项目相关报告/测试/后端/执行记录' / uuid.uuid4().hex / '转发导出明确拒绝实测.json'
        data = (json.dumps(report, ensure_ascii=False, indent=2) + '\n').encode()
        destination.write_bytes(data)
        destination.with_suffix('.json.sha256').write_text(hashlib.sha256(data).hexdigest() + '  ' + destination.name + '\n')
        print(json.dumps(report, ensure_ascii=False))
        return
    scripts = Path(__file__).resolve().parents[1]
    runs = {}
    with tempfile.TemporaryDirectory(prefix='jobsim-forward-compat-') as temporary:
        for label, script, arguments in [
            ('core', scripts / 'integration.py', ['--server', environment['serverURL'], '--ca-file', environment['caPath']]),
            ('searchFavorites', scripts / 'search_favorites.py', ['--environment-file', str(args.environment_file), '--report', str(Path(temporary) / 'search.json')]),
        ]:
            result = subprocess.run([sys.executable, str(script)] + arguments, capture_output=True, text=True)
            if result.returncode:
                raise AssertionError(label + ' real baseline failed; child output redacted')
            runs[label] = json.loads(next(line for line in result.stdout.splitlines() if line.startswith('{')))
    suite = CurrentSuite(environment)
    try:
        runs['roleMentionsAnnouncement'] = suite.run()
    finally:
        suite.logout()
    report = {'status': 'passed', 'testedAtUTC': datetime.now(timezone.utc).isoformat(), 'strictHTTPS': True, 'runs': runs, 'binarySHA256': hashlib.sha256(Path(environment['binary']).read_bytes()).hexdigest(), 'scriptSHA256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(), 'scope': 'Real existing core, signed pagination and current role/announcement regressions on forward realm; historical reports unchanged; no full I or strong protection claim'}
    destination = args.report or Path(__file__).resolve().parents[4] / '项目相关报告/测试/后端/执行记录' / uuid.uuid4().hex / '转发构建既有接口回归.json'
    data = (json.dumps(report, ensure_ascii=False, indent=2) + '\n').encode()
    destination.write_bytes(data)
    destination.with_suffix('.json.sha256').write_text(hashlib.sha256(data).hexdigest() + '  ' + destination.name + '\n')
    print(json.dumps(report, ensure_ascii=False))


if __name__ == '__main__':
    main()
