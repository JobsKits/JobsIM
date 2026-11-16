#!/usr/bin/env python3
"""Owned fresh synchronization realm; no ACK or client atomic-store qualification."""

import argparse
import base64
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import secrets
import signal
import subprocess
import time
from datetime import datetime, timezone
import urllib.parse
import uuid

helper = Path(__file__).resolve().parents[1] / 'forward_refs.py/forward_refs.py'
spec = importlib.util.spec_from_file_location('jobsim_sync_http', helper)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


def private_json(path, value):
    path = Path(path)
    temporary = path.with_suffix(path.suffix + '.writing')
    with open(temporary, 'w', encoding='utf-8') as output:
        os.chmod(temporary, 0o600)
        json.dump(value, output, ensure_ascii=False, indent=2)
        output.write('\n')
        output.flush()
        os.fsync(output.fileno())
    temporary.replace(path)


class SyncSuite(module.ForwardSuite):
    def __init__(self, environment, seed_path):
        super().__init__(environment)
        self.seed_path = Path(seed_path)
        self.stores = {}
        self.verified_pages = 0

    def sql(self, command):
        environment = os.environ | {'PGPASSFILE': self.environment['pgPassFile']}
        result = subprocess.run([str(Path(self.environment['pgBin']) / 'psql'), self.environment['businessURL'], '-XqAt', '-v', 'ON_ERROR_STOP=1'], input=command, capture_output=True, text=True, env=environment, timeout=30)
        if result.returncode:
            raise AssertionError('Owned sync fixture SQL failed; details suppressed')
        return result.stdout.strip()

    def register_store(self, actor):
        store = str(uuid.uuid4())
        body = {'requestId': str(uuid.uuid4()), 'storeGeneration': store}
        result = self.call('PUT', '/api/sync/device', body, actor)
        self.stores[actor['deviceId']] = store
        self.check(not result['qualifiedAck'] and result['qualification'] == 'transport_only', 'registration does not qualify client storage')
        self.check(self.call('PUT', '/api/sync/device', body, actor)['streamId'] == result['streamId'], 'registration retry is stable')
        rejected = self.call('PUT', '/api/sync/device', dict(body, storeGeneration=str(uuid.uuid4())), actor, 409)
        self.check(rejected['error']['code'] == 'STORE_GENERATION_CHANGED', 'replaced local store cannot inherit old device cursor')
        return result

    def snapshot(self, actor, page_size=17):
        body = {'requestId': str(uuid.uuid4()), 'storeGeneration': self.stores[actor['deviceId']], 'schemaVersion': '1', 'pageSize': page_size}
        result = self.call('POST', '/api/sync/snapshots', body, actor, 201)
        again = self.call('POST', '/api/sync/snapshots', body, actor)
        self.check(again == result, 'original snapshot retry does not change pages or extend expiry')
        self.call('POST', '/api/sync/snapshots', dict(body, pageSize=(page_size - 1 if page_size == 100 else page_size + 1)), actor, 409)
        return result

    def verify_body(self, projection):
        if 'object' not in projection:
            self.check('bodyEnvelope' not in projection and 'bodyDigest' not in projection, 'unknown projection does not retain an old body')
            return
        raw = base64.urlsafe_b64decode(projection['bodyEnvelope'] + '===')
        self.check(hashlib.sha256(raw).hexdigest() == projection['bodyDigest'], 'body digest hashes exact envelope bytes')
        self.check(json.loads(raw) == projection['object'], 'display object matches the verified body envelope')

    def pages(self, actor, snapshot):
        items = []
        for manifest in reversed(snapshot['pageManifest']):
            page = self.call('GET', f"/api/sync/snapshots/{snapshot['snapshotId']}/pages/{manifest['page']}", session=actor)
            raw = base64.urlsafe_b64decode(page['referenceEnvelope'] + '===')
            self.check(hashlib.sha256(raw).hexdigest() == manifest['digest'] == page['pageDigest'], 'immutable page digest hashes exact reference bytes')
            self.check(json.loads(raw) == page['refs'] and len(page['refs']) == manifest['count'], 'verified refs match response refs and manifest count')
            self.check([p['ref'] for p in page['items']] == page['refs'], 'hydration is bound to the same immutable refs')
            self.check(page['installable'] and page['permissionGeneration'] == snapshot['permissionGeneration'], 'every complete page is currently installable')
            for projection in page['items']:
                self.verify_body(projection)
            items.extend(page['items'])
            self.verified_pages += 1
        return items

    def validate(self, actor, snapshot, expect=200, digests=None):
        result = self.call('POST', f"/api/sync/snapshots/{snapshot['snapshotId']}/validate", {'pageDigests': digests if digests is not None else [p['digest'] for p in snapshot['pageManifest']]}, actor, expect)
        if expect == 200:
            self.check(result['canInstall'] and result['requiresLocalAtomicInstall'] and not result['qualifiedAck'], 'final validation does not claim local commit or ACK')
            self.check(result['boundaryToken'] == snapshot['boundaryToken'], 'installation returns the original cut boundary')
        return result

    def changes(self, actor, cursor, limit=23):
        result = self.call('GET', '/api/sync/changes?' + urllib.parse.urlencode({'cursor': cursor, 'limit': limit}), session=actor)
        for item in result['items']:
            if item['kind'] == 'redacted':
                self.check(set(item) == {'kind', 'from', 'to', 'rangeDigest'}, 'redaction range reveals no forbidden object IDs')
                continue
            raw = base64.urlsafe_b64decode(item['eventEnvelope'] + '===')
            self.check(hashlib.sha256(raw).hexdigest() == item['eventDigest'], 'immutable event digest hashes exact pointer bytes')
            self.check(json.loads(raw) == {'position': item['position'], 'originEventId': item['originEventId'], 'ref': item['ref']}, 'event envelope matches its visible reference')
            self.verify_body(item['projection'])
        return result

    def drain(self, actor, cursor):
        items = []
        previous = None
        for _ in range(1000):
            result = self.changes(actor, cursor)
            self.check(previous is None or int(result['position']) >= previous, 'exact stream positions never move backward')
            previous = int(result['position'])
            items.extend(result['items'])
            cursor = result['cursor']
            if not result['hasMore']:
                return result, items
        raise AssertionError('incremental replay did not terminate')

    def classifications(self, actor, ids):
        return self.call('POST', '/api/sync/classifications', {'messageIds': ids}, actor)['items']

    def nats_stop(self):
        pid = self.environment['natsPID']
        command = subprocess.run(['ps', '-p', str(pid), '-o', 'command='], text=True, capture_output=True, check=True).stdout.strip()
        expected = self.environment['natsBinary'] + ' -c ' + self.environment['natsConfig']
        self.check(command == expected, 'only this explicitly owned NATS process can be stopped')
        os.kill(pid, signal.SIGTERM)
        for _ in range(100):
            state = subprocess.run(['ps', '-p', str(pid), '-o', 'stat='], text=True, capture_output=True).stdout.strip()
            if not state or state.startswith('Z'):
                return
            time.sleep(.05)
        raise AssertionError('owned NATS did not stop')

    def nats_start(self):
        run = Path(self.environment['runDir'])
        with open(run / 'nats.log', 'ab', buffering=0) as log:
            process = subprocess.Popen([self.environment['natsBinary'], '-c', self.environment['natsConfig']], stdout=log, stderr=log, start_new_session=True)
        self.environment['natsPID'] = process.pid
        private_json(self.seed_path, self.environment)

    def wait_published(self):
        for _ in range(120):
            lag = int(self.sql('SELECT ' + '+'.join(f'(SELECT revision FROM {s}.sync_clock WHERE singleton)-(SELECT revision FROM {s}.sync_publication WHERE singleton)' for s in ['auth', 'directory', 'conversation', 'message'])))
            if lag == 0:
                return
            time.sleep(.1)
        raise AssertionError('JetStream publish acknowledgements did not durably catch up')

    def run(self):
        registration_base = int(self.sql('SELECT count(*) FROM sync.registrations'))
        bootstrap = self.call('GET', '/api/bootstrap')
        self.epoch = bootstrap['restoreEpoch']
        self.check(bootstrap['features'].get('authoritative_sync') and bootstrap['features'].get('sync_jetstream') and not bootstrap['features']['after_receipt'], 'configured sync capability does not enable after-receipt cleanup')
        owner, member, outsider = [self.register(label) for label in ['sa', 'sb', 'sc']]
        self.friend(owner, member)
        group = self.call('POST', '/api/conversations', {'kind': 'group', 'title': 'Snapshot complete owner pages', 'memberIds': [member['user']['id']]}, owner)
        conversation = group['id']
        normal = self.send(owner, conversation, self.payload(owner, conversation, 'sync baseline source ' + secrets.token_hex(6)))
        self.check(normal['creationEpoch'] == self.epoch and normal['creationSequence'] == normal['sequence'], 'new message has its real immutable creation tuple')
        self.register_store(owner)
        self.register_store(member)
        self.register_store(outsider)
        # Explicit owned SQL fixture exercises >500 owner pointers without
        # pretending these rows were accepted through the send HTTP contract.
        self.sql(f"""DO $sync$ DECLARE i integer; n bigint; mid uuid; BEGIN FOR i IN 1..510 LOOP
UPDATE message.heads SET sequence=sequence+1 WHERE conversation_id='{conversation}' RETURNING sequence INTO n;
mid:=gen_random_uuid();
INSERT INTO message.messages(id,conversation_id,sender_id,client_msg_id,payload_digest,type,text,sequence)
VALUES(mid,'{conversation}','{owner['user']['id']}','sync_bulk_'||mid,decode(repeat('ab',32),'hex'),'text','SYNC_BODY_MUST_NOT_PERSIST_IN_POINTERS',n);
INSERT INTO message.events(conversation_id,sequence,message_id) VALUES('{conversation}',n,mid);
END LOOP; END $sync$;""")
        snapshot = self.snapshot(owner)
        items = self.pages(owner, snapshot)
        message_ids = [p['ref']['id'] for p in items if p['ref']['kind'] == 'message']
        self.check(len(message_ids) == 511 and len(set(message_ids)) == 511, 'complete snapshot crosses both public-history 100 and owner-keyset 500 boundaries')
        self.validate(owner, snapshot, 400, [])
        self.validate(owner, snapshot, 400, [p['digest'] for p in reversed(snapshot['pageManifest'])])
        self.validate(owner, snapshot)
        self.call('GET', f"/api/sync/snapshots/{snapshot['snapshotId']}/pages/999999", session=owner, expect=404)
        self.call('GET', f"/api/sync/snapshots/{snapshot['snapshotId']}/pages/0", session=outsider, expect=404)
        self.call('GET', '/api/sync/changes?' + urllib.parse.urlencode({'cursor': snapshot['boundaryToken']}), session=outsider, expect=409)
        self.call('GET', '/api/sync/changes?cursor=forged.unsigned', session=owner, expect=400)
        later = self.send(owner, conversation, self.payload(owner, conversation, 'committed after snapshot cut'))
        self.validate(owner, snapshot)
        delta, events = self.drain(owner, snapshot['boundaryToken'])
        self.check(later['id'] in {e['ref']['id'] for e in events if e['kind'] == 'reference'}, 'post-cut append appears after the original boundary')
        repeated, repeated_events = self.drain(owner, snapshot['boundaryToken'])
        self.check([(e.get('position'),e.get('eventDigest')) for e in events] == [(e.get('position'),e.get('eventDigest')) for e in repeated_events] and delta['head'] == repeated['head'], 'duplicate source replay does not allocate another user position')
        self.call('PUT', '/api/messages/' + normal['id'] + '/favorite', {}, owner)
        changed_page = next(p['page'] for p in snapshot['pageManifest'] if any(i['ref']['id'] == normal['id'] for i in self.call('GET', f"/api/sync/snapshots/{snapshot['snapshotId']}/pages/{p['page']}", session=owner)['items']))
        self.check(not self.call('GET', f"/api/sync/snapshots/{snapshot['snapshotId']}/pages/{changed_page}", session=owner)['installable'], 'source revision change cannot be committed as the old snapshot cut')
        self.validate(owner, snapshot, 409)
        before_membership = self.snapshot(member, 100)
        original_generation = self.conversation(member, conversation)['membershipGeneration']
        self.call('DELETE', f"/api/conversations/{conversation}/members/{member['user']['id']}?membershipGeneration={original_generation}", session=owner)
        self.call('POST', f'/api/conversations/{conversation}/members', {'userId': member['user']['id']}, owner)
        self.check(self.conversation(member, conversation)['membershipGeneration'] != original_generation, 'leave/rejoin has a new membership generation')
        self.call('GET', f"/api/sync/snapshots/{before_membership['snapshotId']}/pages/0", session=member, expect=409)
        self.validate(member, before_membership, 409)
        after_membership = self.snapshot(member, 100)
        self.check(after_membership['permissionGeneration'] != before_membership['permissionGeneration'], 'permission ABA never restores the old generation')
        self.message(member, normal['id'], 403)
        self.check(self.classifications(member, [normal['id']])[0]['classification'] == 'unknown', 'lost cloud read does not erase a legitimate ordinary local copy')
        current = self.message(owner, normal['id'])
        self.check((current['creationEpoch'], current['creationSequence']) == (normal['creationEpoch'], normal['creationSequence']), 'interaction does not alter original creation tuple')
        self.call('DELETE', '/api/messages/' + normal['id'], {'requestId': str(uuid.uuid4()), 'expectedVersion': current['version']}, owner)
        self.check(self.classifications(member, [normal['id']])[0]['classification'] == 'global', 'historically entitled former membership receives a minimal global tombstone')
        tombstone_baseline = self.snapshot(member, 100)
        tombstone_items = self.pages(member, tombstone_baseline)
        self.check(any(item['ref']['id'] == normal['id'] and item['state'] == 'global' for item in tombstone_items), 'new baseline includes historically entitled global facts without replaying old cursor')
        self.validate(member, tombstone_baseline)
        self.check(self.classifications(outsider, [normal['id'], str(uuid.uuid4())])[0]['classification'] == 'unknown', 'outsider cannot turn a guessed ID into a global fact')
        old_delta, old_events = self.drain(member, before_membership['boundaryToken'])
        self.check(old_delta['snapshotRequired'], 'old permission cursor remains restricted after regrant')
        self.check(any(e.get('projection', {}).get('state') == 'global' and e['ref']['id'] == normal['id'] for e in old_events if e['kind'] == 'reference'), 'global deletion is replayed despite current history denial')
        self.wait_published()
        self.nats_stop()
        try:
            bus_off = [self.send(owner, conversation, self.payload(owner, conversation, 'durable outbox during broker outage ' + str(i))) for i in range(3)]
            offline, off_events = self.drain(owner, delta['cursor'])
            self.check({m['id'] for m in bus_off}.issubset({e['ref']['id'] for e in off_events if e['kind'] == 'reference'}), 'authenticated owner replay compensates a real broker outage')
            self.check(int(self.sql('SELECT c.revision-p.revision FROM message.sync_clock c CROSS JOIN message.sync_publication p')) > 0, 'unacknowledged publication stays durable while broker is down')
        finally:
            self.nats_start()
        self.wait_published()
        recovered, recovered_events = self.drain(owner, offline['cursor'])
        self.check(not recovered_events and recovered['head'] == offline['head'], 'real broker reconnect and duplicate ingest do not create extra positions')
        self.check(self.sql("SELECT count(*) FROM sync.origins WHERE ref::text LIKE '%SYNC_BODY_MUST_NOT_PERSIST_IN_POINTERS%'") == '0', 'sync origins contain no chat body')
        self.check(self.sql("SELECT count(*) FROM sync.events WHERE ref::text LIKE '%SYNC_BODY_MUST_NOT_PERSIST_IN_POINTERS%'") == '0', 'user events contain no chat body')
        self.check(int(self.sql("SELECT count(*) FROM sync.registrations")) == registration_base + 3, 'sync registration remains distinct from qualified ACK')
        fixture = {'owner': owner, 'member': member, 'outsider': outsider, 'passwords': self.passwords, 'conversationId': conversation, 'sourceMessageId': normal['id'], 'creationEpoch': normal['creationEpoch'], 'creationSequence': normal['creationSequence'], 'snapshot': snapshot, 'memberSnapshot': after_membership, 'cursor': recovered['cursor'], 'stores': self.stores}
        private_json(Path(self.environment['runDir']) / 'Secrets/sync-http-fixture.json', fixture)
        return {'status': 'passed', 'httpChecks': self.checks, 'semanticAssertions': self.assertions, 'verifiedPages': self.verified_pages, 'bulkRows': 510, 'bulkFixture': 'owned explicit SQL; not send-API acceptance evidence', 'nats': {'actualServer': '2.15.3', 'publishAck': True, 'realDisconnectReconnect': True, 'httpCompensation': True, 'duplicateOriginNoExtraPosition': True}, 'clientAtomicInstall': False, 'qualifiedAck': False, 'fullAccountCoverage': False, 'afterReceipt': False}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--environment-file', required=True)
    parser.add_argument('--report', required=True)
    arguments = parser.parse_args()
    seed = Path(arguments.environment_file).resolve()
    environment = json.loads(seed.read_text())
    if not environment.get('freshSyncRealm') or Path(environment['runDir']).name != 'Safety' or not environment.get('allowOwnedRestartForTests'):
        raise SystemExit('Explicit owned fresh sync Safety realm required')
    suite = SyncSuite(environment, seed)
    report = suite.run()
    report['checkedAt'] = datetime.now(timezone.utc).isoformat()
    report['binarySHA256'] = environment['binarySHA256']
    report['serverURL'] = environment['serverURL']
    output = Path(arguments.report)
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(report, ensure_ascii=False))


if __name__ == '__main__':
    main()
