#!/usr/bin/env python3
"""Fixed-binary isolated sync backup/restore probe; no client ACK qualification."""

import argparse
import base64
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import secrets
import shutil
import signal
import socket
import subprocess
import sys
import time
import urllib.parse

sys.dont_write_bytecode = True
EXPECTED_BINARY = '57a86767d12f005da08c728ff544a2b66076b2db03a203e3221d3490e355f290'
ROOT = Path(__file__).resolve().parents[4]
PRIVATE_DIRECTORY = None


# Hash immutable inputs without loading a complete executable into memory.
def digest(path):
    with Path(path).open('rb') as source:
        return hashlib.file_digest(source, 'sha256').hexdigest()


# Persist private runtime evidence atomically with current-user-only access.
def private_json(path, value):
    path = Path(path)
    temporary = path.with_name(path.name + '.writing')
    descriptor = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(descriptor, 'w', encoding='utf-8') as output:
        json.dump(value, output, ensure_ascii=False, indent=2)
        output.write('\n')
        output.flush()
        os.fsync(output.fileno())
    temporary.replace(path)


# Replace only a database component; credentials remain in process environment.
def database_url(original, name):
    parsed = urllib.parse.urlsplit(original)
    if parsed.scheme not in ('postgres', 'postgresql') or not parsed.hostname:
        raise AssertionError('Explicit PostgreSQL connection URL required')
    return urllib.parse.urlunsplit(parsed._replace(path='/' + name))


# Remove inherited domain overrides before selecting this owned realm.
def owned_env(info, business=None):
    environment = {key: value for key, value in os.environ.items() if not key.startswith('JOBS_IM_')}
    environment.update(PGPASSFILE=info['pgPassFile'], JOBS_IM_DATABASE_URL=business or info['businessURL'], JOBS_IM_JOURNAL_DATABASE_URL=info['journalURL'], JOBS_IM_PUBLIC_URL=info['serverURL'], JOBS_IM_NATS_URL=info['natsURL'], JOBS_IM_NATS_TOKEN_FILE=info['natsTokenFile'])
    return environment


# Capture operator output privately; public failures never echo arguments or DSNs.
def command(argv, environment=None, data=None, timeout=180):
    result = subprocess.run([str(value) for value in argv], env=environment, input=data, text=True, capture_output=True, timeout=timeout)
    if result.returncode:
        if PRIVATE_DIRECTORY:
            private_json(PRIVATE_DIRECTORY / ('command-failure-' + secrets.token_hex(4) + '.json'), {'exitCode': result.returncode, 'stdout': result.stdout[-16000:], 'stderr': result.stderr[-16000:]})
        raise RuntimeError('Private operator command failed; details suppressed')
    return result.stdout.strip()


# Use environment-only PostgreSQL connection selection and stdin-only SQL.
def sql(info, statement, database=None):
    environment = owned_env(info)
    address = urllib.parse.urlsplit(database or info['businessURL'])
    if address.scheme not in ('postgres', 'postgresql') or not address.hostname or address.password is not None:
        raise AssertionError('PostgreSQL host URL without embedded password required; use PGPASSFILE')
    for key in ('PGDATABASE', 'PGHOST', 'PGHOSTADDR', 'PGPORT', 'PGUSER', 'PGPASSWORD', 'PGSERVICE', 'PGSERVICEFILE'):
        environment.pop(key, None)
    environment.update(PGHOST=address.hostname, PGPORT=str(address.port or 5432), PGDATABASE=urllib.parse.unquote(address.path.lstrip('/')))
    if address.username:
        environment['PGUSER'] = urllib.parse.unquote(address.username)
    options = {'sslmode': 'PGSSLMODE', 'connect_timeout': 'PGCONNECT_TIMEOUT', 'application_name': 'PGAPPNAME', 'options': 'PGOPTIONS'}
    for key, value in urllib.parse.parse_qsl(address.query):
        if key not in options:
            raise AssertionError('Explicit PostgreSQL URL option not supported by this private runner')
        environment[options[key]] = value
    return command([Path(info['pgBin']) / 'psql', '-XqAt', '-v', 'ON_ERROR_STOP=1'], environment, statement, 30)


# Create only a fresh named database on one authorized existing cluster.
def create_database(info, original, name):
    if not name.replace('_', '').isalnum() or not name.startswith('jobsim_sync_'):
        raise AssertionError('Owned fixed-prefix database name required')
    sql(info, 'CREATE DATABASE "' + name + '" TEMPLATE template0', database_url(original, 'postgres'))
    return database_url(original, name)


# Reject occupied ports before any process or database is created.
def check_ports():
    for port in [47443, 48222, *range(48101, 48110)]:
        with socket.socket() as probe:
            probe.bind(('127.0.0.1', port))


# Generate an independent local CA and leaf with explicit SKI/AKI and IP SAN.
def create_tls(directory, openssl):
    directory.mkdir(mode=0o700)
    configuration = directory / 'openssl.cnf'
    configuration.write_text('''[req]
distinguished_name=dn
prompt=no
x509_extensions=ca_ext
[dn]
CN=JobsIM isolated sync restore CA
[ca_ext]
basicConstraints=critical,CA:true
keyUsage=critical,keyCertSign,cRLSign
subjectKeyIdentifier=hash
authorityKeyIdentifier=keyid:always
[server_ext]
basicConstraints=critical,CA:false
keyUsage=critical,digitalSignature,keyEncipherment
extendedKeyUsage=serverAuth
subjectAltName=IP:127.0.0.1,DNS:localhost
subjectKeyIdentifier=hash
authorityKeyIdentifier=keyid:always,issuer
''')
    command([openssl, 'req', '-new', '-x509', '-newkey', 'rsa:2048', '-nodes', '-days', '7', '-config', configuration, '-keyout', directory / 'ca.key', '-out', directory / 'ca.pem'])
    command([openssl, 'req', '-new', '-newkey', 'rsa:2048', '-nodes', '-subj', '/CN=127.0.0.1', '-keyout', directory / 'server.key', '-out', directory / 'server.csr'])
    command([openssl, 'x509', '-req', '-in', directory / 'server.csr', '-CA', directory / 'ca.pem', '-CAkey', directory / 'ca.key', '-set_serial', str(secrets.randbits(120) + 1), '-days', '3', '-extfile', configuration, '-extensions', 'server_ext', '-out', directory / 'server.pem'])
    for name in ('ca.key', 'server.key'):
        (directory / name).chmod(0o600)
    command([openssl, 'verify', '-CAfile', directory / 'ca.pem', '-verify_ip', '127.0.0.1', directory / 'server.pem'])
    for name in ('ca.pem', 'server.pem'):
        extensions = command([openssl, 'x509', '-in', directory / name, '-noout', '-ext', 'subjectKeyIdentifier,authorityKeyIdentifier'])
        if 'Subject Key Identifier' not in extensions or 'Authority Key Identifier' not in extensions:
            raise AssertionError('New certificates require explicit SKI and AKI')


# Append only fixed descriptions and counters to the public raw transcript.
class Transcript:
    def __init__(self, path, resume=False):
        self.output = Path(path).open('a' if resume else 'x', encoding='utf-8')

    # Flush each completed step so interruption retains actual partial evidence.
    def write(self, message):
        line = datetime.now(timezone.utc).isoformat() + ' ' + message
        print(line, flush=True)
        self.output.write(line + '\n')
        self.output.flush()

    # Close the raw transcript after the final outcome has been recorded.
    def close(self):
        self.output.close()


# Exchange bounded NATS requests without SDK installation or token argv.
class NATSProbe:
    def __init__(self, info):
        self.info = info

    # Handle server keepalives and accept only one bounded response payload.
    def request(self, subject, payload=b'', message_id=None):
        address = urllib.parse.urlsplit(self.info['natsURL'])
        if address.scheme != 'nats' or address.hostname != '127.0.0.1' or address.port != 48222:
            raise AssertionError('Network probe restricted to the new owned loopback broker')
        token = Path(self.info['natsTokenFile']).read_text().strip()
        inbox = '_INBOX.JSIM_RESTORE_' + secrets.token_hex(12)
        with socket.create_connection((address.hostname, address.port), timeout=8) as connection:
            connection.settimeout(10)
            stream = connection.makefile('rb')
            if not stream.readline(65537).startswith(b'INFO '):
                raise AssertionError('Owned NATS protocol greeting unavailable')
            connect = json.dumps({'verbose': False, 'pedantic': True, 'auth_token': token, 'protocol': 1, 'headers': True, 'lang': 'python', 'version': 'sync-restore-probe'}).encode()
            connection.sendall(b'CONNECT ' + connect + b'\r\nSUB ' + inbox.encode() + b' 1\r\nUNSUB 1 1\r\n')
            if message_id is None:
                connection.sendall(f'PUB {subject} {inbox} {len(payload)}\r\n'.encode() + payload + b'\r\n')
            else:
                header = f'NATS/1.0\r\nNats-Msg-Id: {message_id}\r\n\r\n'.encode()
                connection.sendall(f'HPUB {subject} {inbox} {len(header)} {len(header)+len(payload)}\r\n'.encode() + header + payload + b'\r\n')
            for _ in range(30):
                line = stream.readline(65537)
                if line == b'PING\r\n':
                    connection.sendall(b'PONG\r\n')
                elif line.startswith(b'MSG '):
                    size = int(line.split()[-1])
                    if size < 1 or size > 1 << 20:
                        raise AssertionError('Owned NATS response exceeds the probe bound')
                    raw = stream.read(size)
                    if len(raw) != size or stream.read(2) != b'\r\n':
                        raise AssertionError('Owned NATS response framing invalid')
                    response = json.loads(raw)
                    if response.get('error'):
                        raise AssertionError('Owned JetStream API rejected the request')
                    return response
                elif not line or line.startswith((b'-ERR', b'HMSG')):
                    raise AssertionError('Owned NATS request failed; transport details suppressed')
            raise AssertionError('Owned NATS request produced no bounded reply')


# Manage only the complete argv and recorded PID created by this invocation.
class Realm:
    def __init__(self, info, transcript, suite_type):
        self.info, self.transcript, self.suite_type = info, transcript, suite_type
        self.process = None

    # Launch all ten roles with the exact same new state, database and NATS context.
    def start(self, tls):
        info = self.info
        argv = [info['binary'], '--service=all', '--addr', '127.0.0.1:47443', '--internal-base-port', '48101', '--state-dir', info['stateDir'], '--tls-cert', str(tls / 'server.pem'), '--tls-key', str(tls / 'server.key')]
        with (Path(info['runDir']) / 'server.log').open('ab', buffering=0) as output:
            self.process = subprocess.Popen(argv, env=owned_env(info), stdout=output, stderr=output, start_new_session=True)
        info.update(serverPID=self.process.pid, serverCommand=' '.join(argv))
        private_json(Path(info['runDir']) / 'environment.json', info)
        suite = self.suite_type(info, Path(info['runDir']) / 'environment.json')
        for _ in range(150):
            if self.process.poll() is not None:
                raise AssertionError('Owned role process exited; private process log retained')
            try:
                boot = suite.call('GET', '/api/bootstrap')
                suite.epoch = boot['restoreEpoch']
                self.transcript.write('PASS owned strict-HTTPS ten-role startup epoch=' + suite.epoch)
                return suite, boot
            except Exception:
                time.sleep(.1)
        raise AssertionError('Owned role readiness deadline exceeded')

    # Never signal by port/name; require this invocation's exact command and PID.
    def stop(self):
        if self.process is None or self.process.poll() is not None:
            return
        actual = command(['ps', '-p', str(self.process.pid), '-o', 'command='])
        if actual != self.info['serverCommand']:
            raise AssertionError('Owned PID command changed; no process was signalled')
        self.process.send_signal(signal.SIGTERM)
        try:
            self.process.wait(timeout=12)
        except subprocess.TimeoutExpired as error:
            raise AssertionError('Owned role did not exit; no force kill attempted') from error
        self.info.pop('serverPID', None)
        private_json(Path(self.info['runDir']) / 'environment.json', self.info)
        self.transcript.write('PASS exact owned application PID stopped; NATS and PostgreSQL untouched')


# Wait for committed network ingestion without triggering owner HTTP replay.
def wait_for(description, predicate, transcript, timeout=25):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        if predicate():
            transcript.write('PASS ' + description)
            return
        time.sleep(.2)
    raise AssertionError(description + ' timed out')


# Create the new realm using fixed local binaries and entirely new owned resources.
def prepare(seed, openssl, transcript):
    global PRIVATE_DIRECTORY
    if not seed.get('freshSyncRealm') or digest(seed['binary']) != EXPECTED_BINARY or seed.get('binarySHA256') != EXPECTED_BINARY:
        raise AssertionError('Explicit fixed synchronization seed binary required')
    check_ports()
    suffix = secrets.token_hex(5)
    run = Path(seed['runDir']).parents[1] / ('sync-restore-' + suffix) / 'Safety'
    run.mkdir(parents=True, mode=0o700)
    for name in ('Bin', 'Secrets', 'Keys', 'NATSStore'):
        (run / name).mkdir(mode=0o700)
    PRIVATE_DIRECTORY = run / 'Secrets'
    for key, name in [('binary', 'jobsimd'), ('opsBinary', 'jobsimops')]:
        source, target = Path(seed[key]), run / 'Bin' / name
        os.link(source, target)
    passfile = run / 'Secrets' / 'pgpass'
    shutil.copyfile(seed['pgPassFile'], passfile)
    passfile.chmod(0o600)
    token = secrets.token_hex(32)
    token_file, nats_config = run / 'Secrets/nats.token', run / 'Secrets/nats.conf'
    token_file.write_text(token + '\n')
    token_file.chmod(0o600)
    private_json(nats_config, {'server_name': 'jobsim-sync-restore-' + suffix, 'host': '127.0.0.1', 'port': 48222, 'authorization': {'token': token}, 'jetstream': {'store_dir': str(run / 'NATSStore'), 'max_mem_store': 8388608, 'max_file_store': 536870912}})
    info = {'freshSyncRealm': True, 'syncRestoreTestRealm': True, 'persistentData': True, 'runDir': str(run), 'stateDir': str(run / 'State'), 'serverURL': 'https://127.0.0.1:47443', 'internalBasePort': 48101, 'binary': str(run / 'Bin/jobsimd'), 'opsBinary': str(run / 'Bin/jobsimops'), 'binarySHA256': EXPECTED_BINARY, 'pgBin': seed['pgBin'], 'pgPassFile': str(passfile), 'businessURL': seed['businessURL'], 'journalURL': seed['journalURL'], 'natsBinary': seed['natsBinary'], 'natsConfig': str(nats_config), 'natsTokenFile': str(token_file), 'natsURL': 'nats://127.0.0.1:48222', 'caPath': str(run / 'TLS/ca.pem'), 'operationId': 'sync-restore-' + suffix}
    info['businessURL'] = create_database(info, seed['businessURL'], 'jobsim_sync_restore_' + suffix)
    info['journalURL'] = create_database(info, seed['journalURL'], 'jobsim_sync_journal_' + suffix)
    business_cluster = sql(info, 'SELECT system_identifier::text FROM pg_control_system()')
    journal_cluster = sql(info, 'SELECT system_identifier::text FROM pg_control_system()', info['journalURL'])
    if business_cluster == journal_cluster:
        raise AssertionError('This probe requires the authorized independent journal physical cluster')
    create_tls(run / 'TLS', openssl)
    if digest(run / 'TLS/ca.pem') == digest(seed['caPath']):
        raise AssertionError('Fresh realm must not reuse the baseline CA')
    with (run / 'nats.log').open('ab', buffering=0) as output:
        broker = subprocess.Popen([info['natsBinary'], '-c', str(nats_config)], stdout=output, stderr=output, start_new_session=True)
    info['natsPID'] = broker.pid
    private_json(run / 'environment.json', info)
    transcript.write('PASS new databases on business cluster and separate journal physical cluster; fresh CA/SKI/AKI, state, token and broker store')
    return info


# Exercise only the small restore cut and current-epoch network continuation.
def exercise(info, module, transcript, resume=False):
    class RestoreSuite(module.SyncSuite):
        # Keep borrowed owner-watermark SQL off the process argument list too.
        def sql(self, statement):
            return sql(self.environment, statement)
    realm = Realm(info, transcript, RestoreSuite)
    run = Path(info['runDir'])
    before = after = None
    checks = 0
    try:
        if resume:
            source = json.loads((run / 'Secrets/source-fixture.json').read_text())
            source_info = json.loads((run / 'source-environment.json').read_text())
            before = RestoreSuite(source_info, run / 'source-environment.json')
            before.passwords = source['passwords']
            owner = source['owner']
            recalled, edited = source['messages']
            old_snapshot = source['snapshot']
            creation = {message['id']: [message['creationEpoch'], message['creationSequence']] for message in (recalled, edited)}
            group = {'id': recalled['conversationId']}
            restored = Path(info['stateDir'])
            first_receipt = json.loads((restored / 'restore-receipt.json').read_text())
            initial = {'deploymentId': first_receipt['deploymentId']}
            backup_root = Path.home() / 'Documents/JobsIMBackups' / info['operationId']
            wire = json.loads((run / 'Secrets/post-backup-wire.json').read_text())
            probe = NATSProbe(info)
            stream_name = 'JSIM_' + initial['deploymentId'].replace('-', '')
            old_subject = 'jobsim.' + initial['deploymentId'] + '.1.message'
            stored = probe.request('$JS.API.STREAM.MSG.GET.' + stream_name, json.dumps({'last_by_subj': old_subject}).encode())
            original_wire = base64.b64decode(stored['message']['data'])
            original_header = base64.b64decode(stored['message']['hdrs']).decode()
            before.check(json.loads(original_wire) == wire and ('Nats-Msg-Id: ' + wire['originEventId']) in original_header, 'resume retains the exact post-backup E1 broker wire and MsgId')
            before.check(first_receipt['restoreEpoch'] == '2' and sql(info, "SELECT count(*) FROM journal.restore_operations WHERE operation_id='" + info['operationId'] + "' AND restore_epoch=2 AND state='completed'", info['journalURL']) == '1', 'resume uses the original completed restore and does not allocate another epoch')
            transcript.write('RESUME owned post-restore verification only; original backup/restore, broker PID and persisted store retained')
        else:
            before, initial = realm.start(run / 'TLS')
            before.check(before.epoch == '1', 'new realm begins at epoch one')
            owner, peer = [before.register(label) for label in ('sync_restore_a', 'sync_restore_b')]
            before.friend(owner, peer)
            group = before.call('POST', '/api/conversations', {'kind': 'group', 'title': 'Owned sync restore fixture', 'memberIds': [peer['user']['id']]}, owner)
            recalled, edited = [before.send(owner, group['id'], before.payload(owner, group['id'], label)) for label in ('NON_SECRET_RESTORE_OLD_RECALL', 'NON_SECRET_RESTORE_OLD_EDIT')]
            creation = {message['id']: [message['creationEpoch'], message['creationSequence']] for message in (recalled, edited)}
            before.register_store(owner)
            old_snapshot = before.snapshot(owner, 5)
            before.pages(owner, old_snapshot)
            before.validate(owner, old_snapshot)
            before.wait_published()
            private_json(run / 'Secrets/source-fixture.json', {'owner': owner, 'peer': peer, 'passwords': before.passwords, 'snapshot': old_snapshot, 'messages': [recalled, edited]})
            backup_root = Path.home() / 'Documents' / 'JobsIMBackups' / info['operationId']
            backup_root.mkdir(parents=True, mode=0o700)
            backup, key = backup_root / 'sync-before-mutations.jimbackup', run / 'Keys/recovery.key'
            operator = owned_env(info)
            command([info['opsBinary'], '--mode', 'backup', '--pg-bin', info['pgBin'], '--state-dir', info['stateDir'], '--tls-dir', run / 'TLS', '--file', backup, '--key-file', key], operator)
            transcript.write('PASS real encrypted backup before recall/edit; recovery key stored separately')
            before.call('DELETE', '/api/messages/' + recalled['id'], {'requestId': secrets.token_hex(16), 'expectedVersion': recalled['version']}, owner)
            before.call('PATCH', '/api/messages/' + edited['id'], {'text': 'NON_SECRET_RESTORE_NEW_EDIT', 'requestId': secrets.token_hex(16), 'expectedVersion': edited['version']}, owner)
            before.check(before.message(owner, recalled['id'])['deleted'], 'post-backup real recall committed')
            before.check(before.message(owner, edited['id'])['text'] == 'NON_SECRET_RESTORE_NEW_EDIT', 'post-backup real edit committed')
            before.wait_published()
            wire = json.loads(sql(info, "SELECT jsonb_build_object('owner','message','originEventId',origin_event_id::text,'scopeId',scope_id::text,'restoreEpoch',restore_epoch::text,'revision',revision::text,'ref',jsonb_strip_nulls(jsonb_build_object('owner','message','kind',kind,'id',object_id::text,'revision',revision::text,'revisionEpoch',revision_epoch::text,'subjectUserId',subject_user_id::text,'conversationId',conversation_id::text,'targetUserId',target_user_id::text,'state',state)))::text FROM message.sync_outbox WHERE object_id='" + edited['id'] + "' ORDER BY revision DESC LIMIT 1"))
            private_json(run / 'Secrets/post-backup-wire.json', wire)
            before.check(wire['restoreEpoch'] == '1', 'post-backup origin is an actual epoch-one outbox fact')
            probe = NATSProbe(info)
            stream_name = 'JSIM_' + initial['deploymentId'].replace('-', '')
            old_subject = 'jobsim.' + initial['deploymentId'] + '.1.message'
            # Preserve the exact original bytes already stored in JetStream, not a re-encoded copy.
            stored = probe.request('$JS.API.STREAM.MSG.GET.' + stream_name, json.dumps({'last_by_subj': old_subject}).encode())
            original_wire = base64.b64decode(stored['message']['data'])
            original_header = base64.b64decode(stored['message']['hdrs']).decode()
            original = json.loads(original_wire)
            before.check(original['originEventId'] == wire['originEventId'] and original == wire and ('Nats-Msg-Id: ' + wire['originEventId']) in original_header, 'post-backup raw wire and original MsgId match the real stored broker fact')
            private_json(run / 'source-environment.json', info)
            realm.stop()
            target = create_database(info, info['businessURL'], 'jobsim_sync_restored_' + info['operationId'].split('-')[-1])
            restored = run / 'RestoredState'
            restored.mkdir(mode=0o700)
            operator['JOBS_IM_RESTORE_DATABASE_URL'] = target
            restore = [info['opsBinary'], '--mode', 'restore', '--offline', '--operation-id', info['operationId'], '--pg-bin', info['pgBin'], '--state-dir', info['stateDir'], '--target-state-dir', restored, '--file', backup, '--key-file', key]
            command(restore, operator)
            first_receipt = json.loads((restored / 'restore-receipt.json').read_text())
            command(restore, operator)
            before.check(json.loads((restored / 'restore-receipt.json').read_text()) == first_receipt, 'stable restore operation resumes without a new epoch or changed result')
            info.update(businessURL=target, stateDir=str(restored), caPath=str(restored / 'TLS/ca.pem'))
            private_json(run / 'environment.json', info)
        after, recovered = realm.start(restored / 'TLS')
        after.passwords = before.passwords.copy()
        after.check(recovered['deploymentId'] == initial['deploymentId'] and after.epoch == '2' and first_receipt['restoreEpoch'] == '2', 'same deployment advances exactly once to epoch two')
        after.call('GET', '/api/me', session=owner, expect=401)
        previous_auth_origin = sql(info, "SELECT COALESCE(max(revision),0)::text FROM sync.origins WHERE owner='auth' AND restore_epoch=2")
        fresh = after.login(owner)
        after.check(fresh['deviceId'] != owner['deviceId'] and fresh['restoreEpoch'] == '2', 'same password yields a fresh current-epoch device')
        # No sync HTTP route has been used after startup: origins here prove actual broker ingestion.
        wait_for('new login origin reached target through real JetStream before owner HTTP compensation', lambda: int(sql(info, "SELECT count(*) FROM sync.origins WHERE owner='auth' AND restore_epoch=2 AND revision>" + previous_auth_origin)) > 0, transcript)
        after.register_store(fresh)
        old_cursor = after.call('GET', '/api/sync/changes?' + urllib.parse.urlencode({'cursor': old_snapshot['boundaryToken']}), session=fresh, expect=409)
        after.check(old_cursor['error']['code'] == 'RESTORE_EPOCH_CHANGED', 'old boundary is rejected by epoch before device/store mismatch')
        after.call('GET', f"/api/sync/snapshots/{old_snapshot['snapshotId']}/pages/0", session=fresh, expect=404)
        after.check(after.classifications(fresh, [recalled['id']])[0]['classification'] == 'global', 'journal-restored recall is globally classified')
        for message in (recalled, edited):
            current = after.message(fresh, message['id'])
            after.check([current['creationEpoch'], current['creationSequence']] == creation[message['id']], 'restored old message preserves its original creation tuple')
        recall_result, edit_result = after.message(fresh, recalled['id']), after.message(fresh, edited['id'])
        after.check(recall_result['deleted'] and recall_result['text'] == '', 'journal recall cannot revive old body')
        after.check(edit_result.get('contentUnavailable') and edit_result['text'] == '', 'journal without newer edit body produces unavailable rather than old text')
        current_snapshot = after.snapshot(fresh, 5)
        projections = after.pages(fresh, current_snapshot)
        after.check(current_snapshot['restoreEpoch'] == '2' and all(item['ref']['revisionEpoch'] == '2' and item.get('currentRevisionEpoch') == '2' for item in projections), 'all current snapshot refs and hydration use epoch-two revision provenance')
        after.validate(fresh, current_snapshot)
        created = after.send(fresh, group['id'], after.payload(fresh, group['id'], 'NON_SECRET_EPOCH_TWO_APPEND'))
        after.check(created['creationEpoch'] == '2' and created['creationSequence'] == created['sequence'], 'new message has an epoch-two creation tuple')
        wait_for('new message origin reached target through real JetStream before delta HTTP replay', lambda: int(sql(info, "SELECT count(*) FROM sync.origins WHERE restore_epoch=2 AND ref->>'id'='" + created['id'] + "'")) > 0, transcript)
        delta, events = after.drain(fresh, current_snapshot['boundaryToken'])
        after.check(any(event.get('ref', {}).get('id') == created['id'] for event in events), 'real epoch-two append is replayed after the original new snapshot boundary')
        after.wait_published()
        after.check(sql(info, "SELECT count(*) FROM sync.origins WHERE origin_event_id='" + wire['originEventId'] + "'") == '0', 'post-backup old origin is absent from the restored target before negative delivery')
        position_before = sql(info, 'SELECT COALESCE(sum(head),0)::text FROM sync.streams WHERE restore_epoch=2')
        deadline = time.monotonic() + 150
        accepted = None
        while time.monotonic() < deadline:
            acknowledgement = probe.request(old_subject, original_wire, wire['originEventId'])
            if not acknowledgement.get('duplicate', False):
                accepted = acknowledgement
                break
            transcript.write('WAIT original E1 MsgId remains inside actual broker deduplication window; store/config unchanged')
            time.sleep(10)
        after.check(accepted is not None and accepted['stream'] == stream_name, 'actual original E1 wire receives a nonduplicate publish acknowledgement')
        wait_for('consumer processed or terminated re-delivered old epoch through its real acknowledgement floor', lambda: probe.request('$JS.API.CONSUMER.INFO.' + stream_name + '.JSIM_SYNC')['ack_floor']['stream_seq'] >= accepted['seq'], transcript)
        after.check(sql(info, "SELECT count(*) FROM sync.origins WHERE origin_event_id='" + wire['originEventId'] + "'") == '0', 'network-replayed post-backup epoch-one origin is rejected rather than inserted')
        after.check(sql(info, 'SELECT COALESCE(sum(head),0)::text FROM sync.streams WHERE restore_epoch=2') == position_before, 'rejected old epoch allocates no current-epoch personal positions')
        private_json(run / 'Secrets/restored-fixture.json', {'auth': fresh, 'passwords': after.passwords, 'snapshot': current_snapshot, 'delta': delta, 'creationTuples': creation, 'newMessage': created, 'oldWirePublishAck': accepted})
        transcript.write('PASS old AT, boundary and snapshot rejection; journal deletion/edit safety; E1 creation tuple, E2 refs, full manifest and real E2 delta')
        checks = before.checks + after.checks
        return {'status': 'passed', 'testedAtUTC': datetime.now(timezone.utc).isoformat(), 'httpChecks': checks, 'semanticAssertions': before.assertions + after.assertions, 'verifiedPages': before.verified_pages + after.verified_pages, 'strictHTTPS': True, 'restoreEpochBefore': '1', 'restoreEpochAfter': '2', 'stableRestoreOperationId': True, 'oldAccess401': True, 'oldBoundary409RestoreEpochChanged': True, 'oldSnapshotPage404': True, 'newRefsCurrentEpoch': True, 'oldCreationTuplePreserved': True, 'recallGlobalAndBodyEmpty': True, 'newerEditUnavailableWithoutOldBody': True, 'completeManifestValidated': True, 'newEpochMessageAndDelta': True, 'samePersistentBroker': True, 'oldPostBackupWireNetworkPublishDuplicate': False, 'oldWireConsumerAckFloorReached': True, 'oldWireNoOriginAndNoCurrentPosition': True, 'newOriginsObservedBeforeHTTPReplay': True, 'candidateBinarySHA256': EXPECTED_BINARY, 'opsBinarySHA256': digest(info['opsBinary']), 'scriptSHA256': digest(__file__), 'runDirectory': str(run), 'backupDirectory': str(backup_root), 'scope': 'One business database restored to a new database on its existing physical cluster; independent journal database on a separate physical PostgreSQL cluster; same persisted single-node JetStream broker across application restore.', 'boundaries': {'emptyJetStreamRebuild': False, 'multipleOwnerDatabaseAggregateRestore': False, 'qualifiedAck': False, 'clientAtomicInstall': False, 'strongProtection': False, 'crossHostRecovery': False}}
    finally:
        realm.stop()
        info['applicationStoppedByOwnTest'] = True
        info['persistentBrokerLeftRunning'] = True
        private_json(run / 'environment.json', info)


# Present the authorized test scope before any new runtime resource is created.
def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--environment-file', type=Path, required=True)
    parser.add_argument('--report', type=Path, required=True)
    parser.add_argument('--raw-log', type=Path, required=True)
    parser.add_argument('--resume-owned-environment', type=Path, help='Continue only post-restore checks in the same owned failed run; no restore rerun')
    parser.add_argument('--execute-authorized', action='store_true', help='Explicit unattended execution of the already authorized new isolated realm')
    args = parser.parse_args()
    print('sync_restore.py: fixed local binary; new databases/CA/State/JetStream only; public 47443, internal 48101–48109, NATS 48222. Baseline is read-only. Backup goes to Documents/JobsIMBackups; secrets stay in the new private Safety directory. No build/download. Ctrl+C cancels; only this invocation\'s exact application PID may be stopped. New broker stays running for evidence.', flush=True)
    if not args.execute_authorized:
        if not sys.stdin.isatty() or input('Press Enter to execute; any text cancels: ') != '':
            return 2
    os.umask(0o077)
    seed_path = args.environment_file.resolve(strict=True)
    original_seed_sha = digest(seed_path)
    seed = json.loads(seed_path.read_text())
    if seed_path.parent != Path(seed['runDir']) or seed.get('serverURL') != 'https://127.0.0.1:44443':
        raise AssertionError('Explicit existing 44443 baseline seed required as read-only tool source')
    for output in (args.report, args.raw_log):
        if (output.exists() and not args.resume_owned_environment) or not output.parent.is_dir() or not output.resolve().is_relative_to(ROOT / '项目相关报告'):
            raise AssertionError('New central report/log files required; existing evidence is never overwritten')
    openssl = shutil.which('openssl')
    if not openssl or not all(Path(seed[key]).is_file() for key in ('binary', 'opsBinary', 'natsBinary', 'pgPassFile')):
        raise AssertionError('Existing local tools and private PostgreSQL passfile required; no download attempted')
    info = None
    previous_report = None
    if args.resume_owned_environment:
        resume_path = args.resume_owned_environment.resolve(strict=True)
        info = json.loads(resume_path.read_text())
        run = Path(info['runDir'])
        if not info.get('syncRestoreTestRealm') or resume_path != run / 'environment.json' or not run.parent.name.startswith('sync-restore-') or run.parents[1] != Path(seed['runDir']).parents[1] or digest(info['binary']) != EXPECTED_BINARY or info.get('serverPID'):
            raise AssertionError('Only this stopped owned restore-run environment may resume')
        previous_report = json.loads(args.report.read_text())
        if previous_report.get('status') != 'failed' or previous_report.get('runDirectory') != str(run) or previous_report.get('rawLogSHA256') != digest(args.raw_log):
            raise AssertionError('Resume requires the matching preserved failed report and original raw transcript')
        broker_command = command(['ps', '-p', str(info['natsPID']), '-o', 'command='])
        if broker_command != info['natsBinary'] + ' -c ' + info['natsConfig']:
            raise AssertionError('Original owned persisted broker PID/command required')
        for port in [47443, *range(48101, 48110)]:
            with socket.socket() as probe:
                probe.bind(('127.0.0.1', port))
        private_json(run / ('Secrets/prior-report-' + digest(args.report)[:12] + '.json'), previous_report)
    transcript = Transcript(args.raw_log, bool(args.resume_owned_environment))
    try:
        transcript.write('START authorized fixed-binary isolated restore; base seed and live 44443 domain read-only')
        spec = importlib.util.spec_from_file_location('jobsim_sync_restore_http', Path(__file__).resolve().parents[1] / 'sync_snapshot.py/sync_snapshot.py')
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        if not args.resume_owned_environment:
            info = prepare(seed, openssl, transcript)
        report = exercise(info, module, transcript, bool(args.resume_owned_environment))
        if previous_report:
            report['wrapperRetry'] = {'kind': 'NATS protocol probe headers capability correction; no business-source or restore change', 'originalRawTranscriptPreservedAndAppended': True, 'previousFailureType': previous_report['exceptionType'], 'sameRestoreOperationAndBroker': True, 'countScope': 'Successful resumed post-restore verification only; original source phase/backup/restore remain in the earlier raw transcript and private source fixture.'}
        if digest(seed_path) != original_seed_sha:
            raise AssertionError('Read-only baseline seed changed during this test; evidence requires review')
        report['baseSeedSHA256BeforeAndAfter'] = original_seed_sha
        report['baselineUntouched'] = True
        transcript.write('PASS restore suite http=' + str(report['httpChecks']) + ' semantic=' + str(report['semanticAssertions']))
        report['rawLogSHA256'] = digest(args.raw_log)
        args.report.write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
        return 0
    except BaseException as error:
        transcript.write('FAIL isolated restore suite; exception=' + type(error).__name__ + '; private details suppressed')
        if info:
            private_json(Path(info['runDir']) / 'Secrets/failure.json', {'exceptionType': type(error).__name__, 'message': str(error)})
        elif PRIVATE_DIRECTORY:
            private_json(PRIVATE_DIRECTORY / 'failure.json', {'exceptionType': type(error).__name__, 'message': str(error)})
        args.report.write_text(json.dumps({'status': 'failed', 'exceptionType': type(error).__name__, 'runDirectory': info['runDir'] if info else None, 'rawLogSHA256': digest(args.raw_log), 'candidateBinarySHA256': EXPECTED_BINARY, 'scriptSHA256': digest(__file__), 'noSourceModified': True}, ensure_ascii=False, indent=2) + '\n')
        return 1
    finally:
        transcript.close()


if __name__ == '__main__':
    raise SystemExit(main())
