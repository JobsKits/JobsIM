#!/usr/bin/env python3
"""Reference-only forwarding through real HTTPS, source revocation and byte streams."""
import argparse
import concurrent.futures
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import secrets
import socket
import ssl
import subprocess
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
import zlib
import struct

helper = Path(__file__).resolve().parents[1] / 'mentions_announcements.py/mentions_announcements.py'
spec = importlib.util.spec_from_file_location('jobsim_forward_http', helper)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class ForwardSuite(module.Suite):
    def __init__(self, environment):
        super().__init__(environment)
        self.passwords = {}

    def register(self, label):
        password = 'Forward integration ' + secrets.token_hex(16)
        session = self.call('POST', '/api/auth/register', {'username': 'fwd_' + label + secrets.token_hex(5), 'password': password, 'requestId': str(uuid.uuid4()), 'deviceName': 'forward-reference-test'})
        self.passwords[session['user']['id']] = password
        self.sessions.append(session)
        return session

    def login(self, previous, password=None):
        result = self.call('POST', '/api/auth/login', {'username': previous['user']['username'], 'password': password or self.passwords[previous['user']['id']], 'requestId': str(uuid.uuid4()), 'deviceName': 'forward-relogin'})
        self.sessions.append(result)
        return result

    def forward_body(self, actor, destination, source):
        current = self.message(actor, source['id'])
        target = self.conversation(actor, destination)
        self.check(isinstance(current.get('forwardAdmission'), str) and current['forwardAdmission'].startswith('fa1.'), 'current reader source proof required')
        return {'clientMsgId': str(uuid.uuid4()), 'type': 'forward', 'forwardOfId': current['id'], 'forwardSourceVersion': current['contentVersion'], 'forwardAdmission': current['forwardAdmission'], 'restoreEpoch': self.epoch, 'sendAdmission': target['sendAdmission']}

    def forward(self, actor, destination, source):
        body = self.forward_body(actor, destination, source)
        return self.send(actor, destination, body), body

    def upload(self, actor, name, data):
        boundary = 'JobsIMForward' + secrets.token_hex(12)
        raw = b'--' + boundary.encode() + b'\r\nContent-Disposition: form-data; name="file"; filename="' + name.encode() + b'"\r\nContent-Type: application/octet-stream\r\n\r\n' + data + b'\r\n--' + boundary.encode() + b'--\r\n'
        request = urllib.request.Request(self.base + '/api/media', method='POST', data=raw, headers={'Authorization': 'Bearer ' + actor['token'], 'Content-Type': 'multipart/form-data; boundary=' + boundary})
        with urllib.request.urlopen(request, context=self.trust, timeout=40) as response:
            self.check(response.status == 200, 'real media upload')
            result = json.load(response)
        self.checks += 1
        return result

    def binary(self, actor, path, expect=200, headers=None):
        request = urllib.request.Request(self.base + path, headers={'Authorization': 'Bearer ' + actor['token'], **(headers or {})})
        try:
            with urllib.request.urlopen(request, context=self.trust, timeout=40) as response:
                status, data, metadata = response.status, response.read(), response.headers
        except urllib.error.HTTPError as error:
            status, data, metadata = error.code, error.read(), error.headers
        self.check(status == expect, 'binary route expected status; data redacted')
        self.checks += 1
        return data, metadata

    def lost_response(self, actor, conversation, body):
        endpoint = urllib.parse.urlsplit(self.base)
        payload = json.dumps(body).encode()
        with socket.create_connection((endpoint.hostname, endpoint.port), timeout=15) as connection:
            with self.trust.wrap_socket(connection, server_hostname=endpoint.hostname) as secure:
                header = (f'POST /api/conversations/{conversation}/messages HTTP/1.1\r\nHost: {endpoint.netloc}\r\nAuthorization: Bearer {actor["token"]}\r\nContent-Type: application/json\r\nContent-Length: {len(payload)}\r\nConnection: close\r\n\r\n').encode()
                secure.sendall(header + payload)
                for _ in range(100):
                    count = self.sql("SELECT count(*) FROM message.messages WHERE sender_id='" + actor['user']['id'] + "' AND client_msg_id='" + body['clientMsgId'] + "'")
                    if count == '1':
                        break
                    time.sleep(.02)
                else:
                    raise AssertionError('original commit not observed; no response was consumed')
                # The real socket is closed without reading any response bytes.

    def run(self):
        bootstrap = self.call('GET', '/api/bootstrap')
        self.epoch = bootstrap['restoreEpoch']
        self.check(bootstrap['features'].get('forwarding') is True and bootstrap['features'].get('text_forward') is False, 'new forwarding protocol only')
        author, actor, recipient, outsider = [self.register(label) for label in ('a', 'b', 'c', 'd')]
        for a, b in [(author, actor), (actor, recipient), (recipient, outsider)]:
            self.friend(a, b)
        source_group = self.call('POST', '/api/conversations', {'kind': 'group', 'title': 'Forward source', 'memberIds': [actor['user']['id']]}, author)
        target_group = self.call('POST', '/api/conversations', {'kind': 'group', 'title': 'Forward destination', 'memberIds': [recipient['user']['id']]}, actor)
        onward_group = self.call('POST', '/api/conversations', {'kind': 'group', 'title': 'Forward chain', 'memberIds': [outsider['user']['id']]}, recipient)
        sg, tg, og = source_group['id'], target_group['id'], onward_group['id']
        source = self.send(author, sg, self.payload(author, sg, 'dynamic forward needle'))
        self.message(recipient, source['id'], 403)
        body = self.forward_body(actor, tg, source)
        wrong = dict(body, clientMsgId=str(uuid.uuid4()), forwardAdmission=source['forwardAdmission'])
        self.check(self.send(actor, tg, wrong, 409)['error']['code'] == 'FORWARD_ADMISSION_CHANGED', 'another reader proof cannot be borrowed')
        self.check(self.send(actor, tg, dict(body, clientMsgId=str(uuid.uuid4()), forwardAdmission=''), 409)['error']['code'] == 'FORWARD_ADMISSION_CHANGED', 'missing source proof cannot create')
        self.send(actor, tg, dict(body, clientMsgId=str(uuid.uuid4()), forwardSourceVersion='99'), 409)
        self.send(actor, tg, dict(body, clientMsgId=str(uuid.uuid4()), text='not copied'), 400)
        self.send(actor, tg, dict(body, clientMsgId=str(uuid.uuid4()), mentionedUserIds=[recipient['user']['id']]), 400)
        self.call('POST', '/api/conversations/' + tg + '/message-results', body, actor, 404)
        first = self.send(actor, tg, body)
        self.check(first['forwarded'] and first['type'] == 'text' and first['text'] == source['text'], 'authorized reference resolves for target')
        delivered = self.message(recipient, first['id'])
        self.check(delivered['text'] == source['text'] and not delivered.get('forwardUnavailable'), 'target reads transfer without source membership')
        self.message(outsider, first['id'], 403)
        self.check(self.sql("SELECT type||':'||length(text)||':'||COALESCE(attachment_id::text,'') FROM message.messages WHERE id='" + first['id'] + "'") == 'forward:0:', 'forward database row has no copied body or attachment')
        self.check(self.send(actor, tg, body)['id'] == first['id'], 'same original forward retries have one fact')
        self.send(actor, tg, dict(body, forwardAdmission=source['forwardAdmission']), 409)
        self.call('PATCH', '/api/messages/' + first['id'], {'text': 'cannot edit copied source', 'expectedVersion': first['version'], 'requestId': str(uuid.uuid4())}, actor, 403)
        lost_body = self.forward_body(actor, tg, source)
        self.lost_response(actor, tg, lost_body)
        recovered = self.call('POST', '/api/conversations/' + tg + '/message-results', lost_body, actor)
        self.check(self.send(actor, tg, lost_body)['id'] == recovered['id'], 'unread actual successful response reconciles and retries the same commit')
        second, _ = self.forward(recipient, og, first)
        self.check(self.message(outsider, second['id'])['text'] == source['text'], 'second-level target reads without either original membership')
        chain = second
        for _ in range(6):
            chain, _ = self.forward(outsider, og, chain)
        rejected = self.send(outsider, og, self.forward_body(outsider, og, chain), 400)
        self.check(rejected['error']['code'] == 'FORWARD_DEPTH_LIMIT', 'depth nine cannot commit')
        self.call('PUT', '/api/messages/' + first['id'] + '/favorite', {}, recipient)
        self.check(any(m['id'] == first['id'] for m in self.call('GET', '/api/search?q=dynamic%20forward%20needle', session=recipient)['items']), 'search dynamically sees current forwarded source')
        self.check(any(m['id'] == first['id'] for m in self.call('GET', '/api/favorites', session=recipient)['items']), 'favorite remains a dynamic reference')
        unpublished = self.forward_body(actor, tg, source)
        updated = self.call('PATCH', '/api/messages/' + source['id'], {'text': 'source content replaced', 'expectedVersion': source['version'], 'requestId': str(uuid.uuid4())}, author)
        for target, session in [(first, recipient), (second, outsider), (chain, outsider)]:
            unavailable = self.message(session, target['id'])
            self.check(unavailable.get('forwardUnavailable') and unavailable['text'] == '' and not unavailable.get('attachmentURL') and not unavailable.get('attachmentId') and int(unavailable['contentVersion']) > int(target['contentVersion']), 'ancestor edit publishes permanent descendant placeholder')
        self.check(self.send(actor, tg, body).get('forwardUnavailable') is True, 'old committed request cannot revive old text')
        self.send(actor, tg, unpublished, 409)
        self.check(not any(m['id'] == first['id'] for m in self.call('GET', '/api/search?q=dynamic%20forward%20needle', session=recipient)['items']), 'invalid source cannot match old text search')
        self.check(not any(m['id'] == first['id'] for m in self.call('GET', '/api/favorites', session=recipient)['items']), 'invalid favorite does not occupy a result')
        fresh, _ = self.forward(actor, tg, updated)
        self.check(fresh['text'] == updated['text'], 'new explicit source selection works after edit')
        self.media_cases(author, actor, recipient, outsider, sg, tg)
        membership_source = self.send(author, sg, self.payload(author, sg, 'membership-bound source'))
        membership_forward, _ = self.forward(actor, tg, membership_source)
        old_intent = self.forward_body(actor, tg, membership_source)
        membership = self.conversation(actor, sg)['membershipGeneration']
        self.call('DELETE', '/api/conversations/' + sg + '/members/' + actor['user']['id'] + '?membershipGeneration=' + membership, session=author)
        self.check(self.message(recipient, membership_forward['id']).get('forwardUnavailable') is True, 'leaving original source revokes target reference')
        self.call('POST', '/api/conversations/' + sg + '/members', {'userId': actor['user']['id']}, author)
        self.send(actor, tg, old_intent, 403)
        self.check(self.message(recipient, membership_forward['id']).get('forwardUnavailable') is True, 'source rejoin cannot revive original reference')
        current_source = self.send(author, sg, self.payload(author, sg, 'new member period'))
        principal_forward, _ = self.forward(actor, tg, current_source)
        self.call('POST', '/api/auth/logout', {}, actor)
        self.check(not self.message(recipient, principal_forward['id']).get('forwardUnavailable'), 'ordinary logout does not revoke sent reference')
        actor = self.login(actor)
        codes = self.call('POST', '/api/auth/recovery-codes', {'password': self.passwords[actor['user']['id']], 'requestId': str(uuid.uuid4())}, actor)
        queued = self.forward_body(actor, tg, current_source)
        new_password = 'Forward recovered ' + secrets.token_hex(16)
        self.call('POST', '/api/auth/recover', {'username': actor['user']['username'], 'code': codes['codes'][0], 'password': new_password, 'requestId': str(uuid.uuid4())})
        self.check(self.message(recipient, principal_forward['id']).get('forwardUnavailable') is True, 'principal safety operation revokes sent reference')
        actor = self.login(actor, new_password)
        self.passwords[actor['user']['id']] = new_password
        self.check(self.send(actor, tg, queued, 409)['error']['code'] == 'FORWARD_ADMISSION_CHANGED', 'security recovery cannot reauthorize an old unsubmitted source proof')
        self.check(self.message(recipient, principal_forward['id']).get('forwardUnavailable') is True, 'reauthentication never revives permanent reference')
        new_forward, _ = self.forward(actor, tg, current_source)
        self.check(not new_forward.get('forwardUnavailable'), 'new explicit principal intent works')
        self.export_rejection(recipient)
        return {'status': 'passed', 'httpAssertions': self.checks, 'semanticAssertions': self.assertions, 'strictHTTPS': True, 'referenceOnlyStorage': True, 'sourceProofAndDestinationAdmissionFrozen': True, 'actualUnreadResponseReconciled': True, 'lookup404ExplicitOriginalRetry': True, 'depthEight': True, 'targetHasNoSourceACL': True, 'sourceEditRecallMembershipAndPrincipalInvalidations': True, 'mediaTypesAndRange': True, 'streamCancelReleasesPublicFence': True, 'ordinaryLogoutDoesNotRevoke': True, 'searchAndFavoritesDynamic': True, 'experimentalForwardExportRejected': True, 'scope': 'Isolated same-host HTTPS/PG and real media; UI, physical devices, cross-host, strong protection and full phase I not accepted'}

    def media_cases(self, author, actor, recipient, outsider, source_group, target_group):
        work = Path(self.environment['runDir']) / 'ForwardMedia'
        work.mkdir(exist_ok=True)
        ffmpeg = subprocess.run(['which', 'ffmpeg'], capture_output=True, text=True).stdout.strip()
        if not ffmpeg:
            raise RuntimeError('Existing ffmpeg required; no dependency download attempted')
        for name, arguments in [('tone.m4a', ['-f', 'lavfi', '-i', 'sine=frequency=440:duration=0.2', '-c:a', 'aac']), ('clip.mp4', ['-f', 'lavfi', '-i', 'color=c=green:s=32x32:d=0.2', '-c:v', 'libx264', '-pix_fmt', 'yuv420p']), ('animation.gif', ['-f', 'lavfi', '-i', 'testsrc=size=16x16:duration=0.3:rate=10'])]:
            subprocess.run([ffmpeg, '-hide_banner', '-loglevel', 'error', '-y', *arguments, str(work/name)], check=True, capture_output=True)
        def chunk(kind, data):
            return struct.pack('>I', len(data)) + kind + data + struct.pack('>I', zlib.crc32(kind+data)&0xffffffff)
        png = b'\x89PNG\r\n\x1a\n' + chunk(b'IHDR', struct.pack('>IIBBBBB', 16, 16, 8, 2, 0, 0, 0)) + chunk(b'IDAT', zlib.compress((b'\0'+bytes([30,100,70])*16)*16)) + chunk(b'IEND', b'')
        samples = [('image.png', png, 'image'), ('animation.gif', (work/'animation.gif').read_bytes(), 'image'), ('tone.m4a', (work/'tone.m4a').read_bytes(), 'audio'), ('clip.mp4', (work/'clip.mp4').read_bytes(), 'video'), ('document.txt', b'ordinary file reference\n', 'file')]
        for name, data, kind in samples:
            uploaded = self.upload(author, name, data)
            body = self.payload(author, source_group, name, type=kind, attachmentId=uploaded['id'])
            source = self.send(author, source_group, body)
            forwarded, _ = self.forward(actor, target_group, source)
            self.check(forwarded['type'] == kind and not forwarded.get('attachmentId') and forwarded.get('attachmentURL'), 'forward media never grants original object identity')
            self.binary(recipient, '/api/media/' + uploaded['id'], 403)
            received, headers = self.binary(recipient, forwarded['attachmentURL'])
            self.check(received == data and headers['X-Content-SHA256'] == hashlib.sha256(data).hexdigest() and headers['X-Content-Size'] == str(len(data)), 'auth stream retains original byte hash and size')
            partial, metadata = self.binary(recipient, forwarded['attachmentURL'], 206, {'Range': 'bytes=0-9'})
            self.check(partial == data[:10] and metadata['Content-Range'].startswith('bytes 0-9/'), 'real byte range relay')
            self.binary(outsider, forwarded['attachmentURL'], 403)
            self.call('DELETE', '/api/messages/' + source['id'], session=author)
            self.binary(recipient, forwarded['attachmentURL'], 404)
            self.check(self.message(recipient, forwarded['id']).get('forwardUnavailable') is True, 'source recall closes media URL')
        data = b'JobsIM-stream-' + b'F' * (60*1024*1024)
        asset = self.upload(author, 'large.bin', data)
        source = self.send(author, source_group, self.payload(author, source_group, 'large.bin', type='file', attachmentId=asset['id']))
        forwarded, _ = self.forward(actor, target_group, source)
        response = urllib.request.urlopen(urllib.request.Request(self.base+forwarded['attachmentURL'], headers={'Authorization': 'Bearer '+recipient['token']}), context=self.trust, timeout=40)
        try:
            response.read(4096)
            with concurrent.futures.ThreadPoolExecutor(max_workers=1) as pool:
                retracting = pool.submit(self.call, 'DELETE', '/api/messages/'+source['id'], None, author)
                time.sleep(.2)
                self.check(not retracting.done(), 'source revocation waits for outer public response stream')
                response.close()
                retracting.result(timeout=20)
        finally:
            response.close()
        self.binary(recipient, forwarded['attachmentURL'], 404)

    def export_rejection(self, actor):
        request = self.call('POST', '/api/me/exports', {'requestId': str(uuid.uuid4())}, actor, 202)
        job = request['job']
        for _ in range(100):
            status = self.call('GET', '/api/me/exports/'+job['id'], session=actor)
            if status['state'] not in ('requested', 'building', 'invalidating'):
                break
            time.sleep(.1)
        self.check(status['state'] != 'ready', 'experimental forward export must not publish a copied package')
        self.check(self.sql("SELECT count(*) FROM message.export_holders h JOIN message.messages m ON m.id=h.message_id WHERE h.job_id='"+job['id']+"' AND m.type='forward' AND h.state='active'") == '0', 'no unprotected forward export holder')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--environment-file', type=Path, required=True)
    args = parser.parse_args()
    environment = json.loads(args.environment_file.read_text())
    if environment.get('forwardTestRealm') is not True:
        raise RuntimeError('Requires explicit isolated forwardTestRealm metadata')
    suite = ForwardSuite(environment)
    try:
        report = suite.run()
        report['testedAtUTC'] = datetime.now(timezone.utc).isoformat()
        report['scriptSHA256'] = hashlib.sha256(Path(__file__).read_bytes()).hexdigest()
        destination = Path(__file__).resolve().parents[4] / '项目相关报告/测试/后端/执行记录' / uuid.uuid4().hex / '普通转发真实闭环.json'
        data = (json.dumps(report, ensure_ascii=False, indent=2)+'\n').encode()
        destination.write_bytes(data)
        destination.with_suffix('.json.sha256').write_text(hashlib.sha256(data).hexdigest()+'  '+destination.name+'\n')
        print(json.dumps(report, ensure_ascii=False))
    finally:
        suite.logout()


if __name__ == '__main__':
    main()
