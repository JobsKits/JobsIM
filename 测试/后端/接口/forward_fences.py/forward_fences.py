#!/usr/bin/env python3
"""Slow public TLS JSON, durable batch continuation and source-owner pending pickup."""
import argparse
import concurrent.futures
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import socket
import time
import urllib.parse
import uuid

helper = Path(__file__).resolve().parents[1] / 'forward_refs.py/forward_refs.py'
spec = importlib.util.spec_from_file_location('jobsim_forward_fences_http', helper)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


def prepare(suite, environment_file, destination):
    suite.epoch = suite.call('GET', '/api/bootstrap')['restoreEpoch']
    author, actor, recipient = [suite.register(label) for label in ('json_a', 'json_b', 'json_c')]
    suite.friend(author, actor)
    suite.friend(actor, recipient)
    source_group = suite.call('POST', '/api/conversations', {'kind': 'group', 'title': 'JSON source', 'memberIds': [actor['user']['id']]}, author)
    target = suite.call('POST', '/api/conversations', {'kind': 'group', 'title': 'JSON target', 'memberIds': [recipient['user']['id']]}, actor)
    query = 'json_fence_' + uuid.uuid4().hex
    # Escape expansion produces a large valid JSON page without oversized text.
    source = suite.send(author, source_group['id'], suite.payload(author, source_group['id'], query + '<' * 14000))
    forwarded, body = suite.forward(actor, target['id'], source)
    suite.call('PUT', '/api/messages/' + forwarded['id'] + '/favorite', {}, recipient)
    member = suite.conversation(actor, target['id'])
    suite.put_announcement(actor, target['id'], {'messageId': forwarded['id'], 'sourceVersion': forwarded['contentVersion'], 'expectedVersion': '0', 'membershipGeneration': member['membershipGeneration'], 'roleGeneration': member['roleGeneration'], 'requestId': str(uuid.uuid4())})
    fixture = {'environmentFile': str(environment_file.resolve()), 'sessions': suite.sessions, 'sourceGroupId': source_group['id'], 'targetId': target['id'], 'source': source, 'messageId': forwarded['id'], 'originalBody': body, 'recipientToken': recipient['token'], 'actorToken': actor['token'], 'query': query, 'epoch': suite.epoch}
    fd = os.open(destination, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
    with os.fdopen(fd, 'w') as output:
        json.dump(fixture, output)
        output.flush()
        os.fsync(output.fileno())
    print(json.dumps({'status': 'prepared', 'fixtureFile': str(destination), 'httpAssertions': suite.checks, 'credentials': 'private fixture only'}))


def slow_tls(suite, fixture):
    endpoint = urllib.parse.urlsplit(suite.base)
    raw = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    raw.setsockopt(socket.SOL_SOCKET, socket.SO_RCVBUF, 4096)
    raw.settimeout(45)
    raw.connect((endpoint.hostname, endpoint.port))
    connection = suite.trust.wrap_socket(raw, server_hostname=endpoint.hostname)
    request = (f'GET /api/conversations/{fixture["targetId"]}/messages?after={suite.epoch}:0&limit=200 HTTP/1.1\r\nHost: {endpoint.netloc}\r\nAuthorization: Bearer {fixture["recipientToken"]}\r\nX-JobsIM-Projection-Fence: forged-public-value\r\nX-JobsIM-Projection-Fence-Mode: exclusive\r\nConnection: close\r\n\r\n').encode()
    connection.sendall(request)
    headers = b''
    while b'\r\n\r\n' not in headers:
        headers += connection.recv(1)
        if len(headers) > 65536:
            raise AssertionError('Unexpected response header size')
    suite.check(b' 200 ' in headers.split(b'\r\n', 1)[0], 'slow JSON authenticated response')
    suite.checks += 1
    return connection


def verify(suite, fixture):
    suite.sessions = fixture['sessions']
    author, actor, recipient = suite.sessions
    suite.epoch = fixture['epoch']
    target, source_group = fixture['targetId'], fixture['sourceGroupId']
    original = fixture['originalBody']
    for _ in range(199):
        suite.send(actor, target, dict(original, clientMsgId=str(uuid.uuid4())))
    connection = slow_tls(suite, fixture)
    try:
        with concurrent.futures.ThreadPoolExecutor(max_workers=1) as pool:
            renaming = pool.submit(suite.call, 'PATCH', '/api/conversations/' + source_group, {'title': 'JSON source read drain'}, author)
            time.sleep(.2)
            suite.check(not renaming.done(), 'real slow public JSON keeps the source EX fence blocked')
            connection.close()
            renaming.result(timeout=15)
    finally:
        connection.close()
    # More than 100 descendant updates must stay durable across a bounded call.
    source = fixture['source']
    mutation = {'text': 'content after durable batch', 'expectedVersion': source['version'], 'requestId': str(uuid.uuid4())}
    pending = suite.call('PATCH', '/api/messages/' + source['id'], mutation, author, 503)
    suite.check(pending['error']['code'] == 'FORWARD_CLEANUP_PENDING', 'bounded cleanup reports durable pending rather than accepting source edit')
    suite.check(suite.sql("SELECT content_version FROM message.messages WHERE id='" + source['id'] + "'") == source['contentVersion'], 'source content did not advance before descendant cleanup')
    for _ in range(100):
        remaining = suite.sql("SELECT count(*) FROM message.forward_refs WHERE source_message_id='" + source['id'] + "' AND state<>'unavailable'")
        if remaining == '0':
            break
        time.sleep(.1)
    suite.check(remaining == '0', 'background resume closes all durable descendants without a lease')
    updated = suite.call('PATCH', '/api/messages/' + source['id'], mutation, author)
    suite.check(updated['contentVersion'] != source['contentVersion'], 'original source operation can finish after its batch resumes')
    unavailable = suite.message(recipient, fixture['messageId'])
    suite.check(unavailable.get('forwardUnavailable') and unavailable['text'] == '', 'old target body remains permanently unavailable')
    self_message = suite.send(recipient, target, suite.payload(recipient, target, fixture['query'] + ' own valid result'))
    suite.call('PUT', '/api/messages/' + self_message['id'] + '/favorite', {}, recipient)
    # Search scans many invalid references before the older authorized hit.
    page = suite.call('GET', '/api/search?q=' + fixture['query'] + '&limit=1', session=recipient)
    suite.check(page['items'][0]['id'] == self_message['id'], 'invalid references do not crowd a valid search page')
    favorite = suite.call('GET', '/api/favorites?limit=1', session=recipient)
    suite.check(favorite['items'][0]['id'] == self_message['id'], 'invalid favorites do not crowd a valid page')
    # Simulate exactly the source owner's committed pending row before the
    # outbound message RPC arrives, then finish through the real leave API.
    pending_source = suite.send(author, source_group, suite.payload(author, source_group, 'owner-pending fixture source'))
    pending_forward, _ = suite.forward(actor, target, pending_source)
    generation = suite.conversation(actor, source_group)['membershipGeneration']
    operation = 'leave:' + source_group + ':' + actor['user']['id'] + ':' + generation
    suite.sql("UPDATE conversation.members SET export_pending_operation_id='" + operation + "' WHERE conversation_id='" + source_group + "' AND user_id='" + actor['user']['id'] + "'")
    for _ in range(100):
        state = suite.sql("SELECT state FROM message.forward_refs WHERE message_id='" + pending_forward['id'] + "'")
        if state == 'unavailable':
            break
        time.sleep(.1)
    suite.check(state == 'unavailable', 'message worker recovers a membership pending commit without its outbound RPC')
    suite.call('DELETE', '/api/conversations/' + source_group + '/members/' + actor['user']['id'] + '?membershipGeneration=' + generation, session=author)
    suite.call('POST', '/api/conversations/' + source_group + '/members', {'userId': actor['user']['id']}, author)
    suite.check(suite.message(recipient, pending_forward['id']).get('forwardUnavailable') is True, 'rejoin after resumed source operation cannot revive reference')
    return {'status': 'passed', 'httpAssertions': suite.checks, 'semanticAssertions': suite.assertions, 'slowPublicTLSJSON': True, 'disconnectReleasesFence': True, 'publicFakeHeaderCannotBypass': True, 'over100DescendantsDurablyResume': True, 'sameOriginalSourceOperationContinues': True, 'membershipPendingBeforeOutboundRPCRecovered': True, 'invalidReferencesCannotCrowdLookup': True, 'scope': 'Isolated same-host real HTTPS/PG; membership pending window is an explicit SQL fixture followed by the actual source-owner API; no TTL, UI or cross-host claim'}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--environment-file', type=Path, required=True)
    parser.add_argument('--fixture-file', type=Path)
    parser.add_argument('--prepare', action='store_true')
    args = parser.parse_args()
    environment = json.loads(args.environment_file.read_text())
    if environment.get('forwardTestRealm') is not True:
        raise RuntimeError('Requires an explicit isolated forward test realm')
    suite = module.ForwardSuite(environment)
    if args.prepare:
        path = args.fixture_file or args.environment_file.parent / ('ForwardJSONFixture.' + uuid.uuid4().hex + '.json')
        prepare(suite, args.environment_file, path)
        return
    if args.fixture_file is None:
        parser.error('--fixture-file from prepare is required')
    try:
        report = verify(suite, json.loads(args.fixture_file.read_text()))
        report['testedAtUTC'] = datetime.now(timezone.utc).isoformat()
        report['scriptSHA256'] = hashlib.sha256(Path(__file__).read_bytes()).hexdigest()
        destination = Path(__file__).resolve().parents[4] / '项目相关报告/测试/后端/执行记录' / uuid.uuid4().hex / '转发公共JSON与持久续跑.json'
        data = (json.dumps(report, ensure_ascii=False, indent=2) + '\n').encode()
        destination.write_bytes(data)
        destination.with_suffix('.json.sha256').write_text(hashlib.sha256(data).hexdigest() + '  ' + destination.name + '\n')
        print(json.dumps(report, ensure_ascii=False))
    finally:
        suite.logout()


if __name__ == '__main__':
    main()
