#!/usr/bin/env python3
"""Real owner-role boomerang and legacy committed announcement compatibility."""
import argparse
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import uuid

helper_path = Path(__file__).resolve().parents[1] / 'mentions_announcements.py/mentions_announcements.py'
spec = importlib.util.spec_from_file_location('jobsim_mention_http', helper_path)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


def prepare(suite, destination):
    bootstrap = suite.call('GET', '/api/bootstrap')
    suite.check(bootstrap['features'].get('role_admission') is not True, 'prepare must use actual pre-role service')
    suite.epoch = bootstrap['restoreEpoch']
    owner, successor, unaffected = [suite.register(label) for label in ('role_a', 'role_b', 'role_c')]
    for peer in (successor, unaffected):
        suite.friend(owner, peer)
    group = suite.call('POST', '/api/conversations', {'kind': 'group', 'title': 'Role qualification fixture', 'memberIds': [successor['user']['id'], unaffected['user']['id']]}, owner)
    body = suite.payload(owner, group['id'], 'Legacy committed source')
    source = suite.send(owner, group['id'], body)
    legacy_body = suite.announce_body(owner, group['id'], source)
    operation = suite.put_announcement(owner, group['id'], legacy_body)
    suite.check(operation['status'] == 'applied', 'real pre-role original operation committed')
    fixture = {'serverURL': suite.base, 'deploymentId': bootstrap['deploymentId'], 'epoch': suite.epoch, 'sessions': suite.sessions, 'group': group, 'source': source, 'messageBody': body, 'legacyAnnouncementBody': legacy_body, 'prepareHTTPAssertions': suite.checks}
    destination.parent.mkdir(parents=True, exist_ok=True)
    descriptor = os.open(destination, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
    with os.fdopen(descriptor, 'w') as stream:
        json.dump(fixture, stream)
        stream.flush()
        os.fsync(stream.fileno())
    print(json.dumps({'status': 'legacy-fixture-prepared', 'fixtureFile': str(destination), 'httpAssertions': suite.checks, 'credentials': 'private file only'}))


def verify(suite, fixture):
    suite.check(fixture['serverURL'] == suite.base, 'fixture instance mismatch')
    suite.sessions = fixture['sessions']
    owner, successor, unaffected = suite.sessions
    group_id = fixture['group']['id']
    owner_id, successor_id, unaffected_id = [session['user']['id'] for session in suite.sessions]
    prefix = '/api/conversations/' + group_id
    bootstrap = suite.call('GET', '/api/bootstrap')
    suite.epoch = bootstrap['restoreEpoch']
    suite.check(bootstrap['features'].get('role_admission') is True, 'role capability required')
    suite.check(bootstrap['deploymentId'] == fixture['deploymentId'] and suite.epoch == fixture['epoch'], 'upgrade changed instance or epoch')
    suite.check(suite.call('GET', '/api/capabilities', session=owner)['features'].get('role_admission') is True, 'capabilities expose role protection')
    current = suite.conversation(owner, group_id)
    suite.check(current['roleGeneration'] == '1', 'existing membership migration starts role generation one')
    legacy = fixture['legacyAnnouncementBody']
    suite.check('roleGeneration' not in legacy, 'legacy payload must not be patched')
    suite.check(suite.put_announcement(owner, group_id, legacy)['operationId'] == legacy['requestId'], 'old completed no-role payload remains compatible')
    patched = dict(legacy, roleGeneration='1')
    suite.put_announcement(owner, group_id, patched, 409)
    missing = suite.announce_body(owner, group_id, fixture['source'])
    suite.check(suite.put_announcement(owner, group_id, missing, 409)['error']['code'] == 'QUALIFICATION_CHANGED', 'missing role cannot create a new publication')
    queued_announcement = dict(suite.announce_body(owner, group_id, fixture['source']), roleGeneration=current['roleGeneration'])
    queued_clear = {'expectedVersion': queued_announcement['expectedVersion'], 'membershipGeneration': current['membershipGeneration'], 'roleGeneration': current['roleGeneration'], 'requestId': str(uuid.uuid4())}
    queued_all = suite.payload(owner, group_id, 'Original unsubmitted all', mentionAll=True)
    queued_plain = suite.payload(owner, group_id, 'Original unsubmitted plain')
    committed_body = suite.payload(owner, group_id, 'Original committed history')
    committed = suite.send(owner, group_id, committed_body)
    unaffected_before = suite.conversation(unaffected, group_id)
    joined_before = suite.sql("SELECT string_agg(user_id::text||':'||membership_generation::text||':'||joined_at::text,',' ORDER BY user_id) FROM conversation.members WHERE conversation_id='" + group_id + "'")
    suite.call('PATCH', prefix, {'ownerId': successor_id}, owner)
    suite.call('PATCH', prefix, {'ownerId': owner_id}, successor)
    returned = suite.conversation(owner, group_id)
    other = suite.conversation(successor, group_id)
    still_member = suite.conversation(unaffected, group_id)
    suite.check(returned['roleGeneration'] == '3' and other['roleGeneration'] == '3', 'only original and new owner advance across both transitions')
    suite.check(still_member['roleGeneration'] == unaffected_before['roleGeneration'] and still_member['sendAdmission'] == unaffected_before['sendAdmission'], 'unaffected member role and admission unchanged')
    joined_after = suite.sql("SELECT string_agg(user_id::text||':'||membership_generation::text||':'||joined_at::text,',' ORDER BY user_id) FROM conversation.members WHERE conversation_id='" + group_id + "'")
    suite.check(joined_before == joined_after, 'ownership transfer must not change membership or history lower bound')
    suite.check(returned['sendAdmission'] != current['sendAdmission'], 'owner role boomerang cannot restore old send admission')
    for body in (queued_all, queued_plain):
        rejected = suite.send(owner, group_id, body, 409)
        suite.check(rejected['error']['code'] == 'SEND_ADMISSION_CHANGED', 'old unsubmitted intent remains frozen after role return')
        suite.call('POST', prefix + '/message-results', body, owner, 404)
    suite.check(suite.send(owner, group_id, committed_body)['id'] == committed['id'], 'current read ACL retains committed original history')
    suite.check(suite.call('POST', prefix + '/message-results', fixture['messageBody'], owner)['id'] == fixture['source']['id'], 'pre-upgrade original message remains readable')
    suite.check(suite.put_announcement(owner, group_id, queued_announcement, 409)['error']['code'] == 'QUALIFICATION_CHANGED', 'old unsubmitted announcement cannot borrow returned role')
    suite.check(suite.call('DELETE', prefix + '/announcement', queued_clear, owner, 409)['error']['code'] == 'QUALIFICATION_CHANGED', 'old clear cannot borrow returned role')
    no_role_clear = dict(queued_clear, requestId=str(uuid.uuid4()))
    del no_role_clear['roleGeneration']
    suite.check(suite.call('DELETE', prefix + '/announcement', no_role_clear, owner, 409)['error']['code'] == 'QUALIFICATION_CHANGED', 'missing role cannot create clear')
    suite.check(suite.put_announcement(owner, group_id, legacy)['current'], 'completed legacy result is checked before new qualification')
    suite.send(owner, group_id, suite.payload(owner, group_id, 'New explicit all', mentionAll=True))
    fresh = dict(suite.announce_body(owner, group_id, fixture['source']), roleGeneration=returned['roleGeneration'])
    published = suite.put_announcement(owner, group_id, fresh)
    suite.check(published['current'], 'new explicit announcement uses current role')
    suite.check(not suite.put_announcement(owner, group_id, legacy)['current'], 'legacy replay cannot roll back newer publication')
    suite.call('PATCH', prefix, {'ownerId': successor_id}, owner)
    suite.check(suite.put_announcement(owner, group_id, fresh)['operationId'] == fresh['requestId'], 'committed role-bound original remains queryable after losing ownership')
    suite.call('PATCH', prefix, {'ownerId': owner_id}, successor)
    before_noop = suite.conversation(owner, group_id)
    suite.call('PATCH', prefix, {'ownerId': owner_id}, owner)
    suite.call('PATCH', prefix, {'title': 'Role rename unchanged qualification'}, owner)
    after_noop = suite.conversation(owner, group_id)
    suite.check(before_noop['roleGeneration'] == after_noop['roleGeneration'] and before_noop['sendAdmission'] == after_noop['sendAdmission'], 'same-owner and rename do not fabricate new role grants')
    members = suite.call('GET', prefix + '/members', session=owner)['items']
    suite.check(all(isinstance(member['roleGeneration'], str) for member in members), 'member DTO role generations remain strings')
    direct = suite.call('POST', '/api/conversations', {'kind': 'direct', 'memberIds': [unaffected_id]}, owner)
    suite.check(direct['roleGeneration'] == '1', 'direct DTO exposes current role generation')
    suite.sql("UPDATE conversation.members SET role_generation=9007199254740993 WHERE conversation_id='" + group_id + "' AND user_id='" + unaffected_id + "'")
    suite.check(suite.conversation(unaffected, group_id)['roleGeneration'] == '9007199254740993', 'large decimal role generation is not coerced to JS precision')
    return {'status': 'passed', 'httpAssertions': suite.checks, 'semanticAssertions': suite.assertions, 'preUpgradeFixtureHTTPAssertions': fixture['prepareHTTPAssertions'], 'strictHTTPS': True, 'unobservedOwnerReturnRejectsAllAndAnnouncement': True, 'ordinaryOldIntentConservativelyFrozen': True, 'legacyCompletedDigestCompatibility': True, 'currentReadHistoryPreserved': True, 'membershipAndJoinedAtUnchanged': True, 'unaffectedMemberAdmissionUnchanged': True, 'sameOwnerNoopStable': True, 'roleAdmissionCapability': True, 'largeRoleGenerationFixture': True, 'scope': 'Same-host role fence and real HTTPS/PG; no client UI, hardware, cross-host or full phase-I acceptance claim'}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--environment-file', type=Path, required=True)
    parser.add_argument('--fixture-file', type=Path)
    parser.add_argument('--prepare', action='store_true')
    args = parser.parse_args()
    suite = module.Suite(json.loads(args.environment_file.read_text()))
    if args.prepare:
        destination = args.fixture_file or args.environment_file.parent / ('RoleQualification.' + uuid.uuid4().hex + '.json')
        prepare(suite, destination)
        return
    if args.fixture_file is None:
        parser.error('--fixture-file from an actual pre-upgrade prepare run is required')
    try:
        report = verify(suite, json.loads(args.fixture_file.read_text()))
        report['testedAtUTC'] = datetime.now(timezone.utc).isoformat()
        report['scriptSHA256'] = hashlib.sha256(Path(__file__).read_bytes()).hexdigest()
        root = Path(__file__).resolve().parents[4]
        report['backendSHA256'] = {name: hashlib.sha256((root/'代码/后端/Go'/name).read_bytes()).hexdigest() for name in ['conversation.go', 'send_admission.go', 'types.go', 'control.go', 'mentions_announcements.go']}
        destination = Path(__file__).resolve().parents[2] / '报告/角色准入代际实测.json'
        data = (json.dumps(report, ensure_ascii=False, indent=2) + '\n').encode()
        destination.write_bytes(data)
        destination.with_suffix('.json.sha256').write_text(hashlib.sha256(data).hexdigest() + '  ' + destination.name + '\n')
        print(json.dumps({k: v for k, v in report.items() if k != 'backendSHA256'}, ensure_ascii=False))
    finally:
        suite.logout()


if __name__ == '__main__':
    main()
