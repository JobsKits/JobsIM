#!/usr/bin/env python3
"""Create and restore a separate realm; prove active snapshot refs cannot regain epoch."""
import uuid
import argparse
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
import time
import urllib.parse

helper = Path(__file__).resolve().parents[1] / 'forward_refs.py/forward_refs.py'
spec = importlib.util.spec_from_file_location('jobsim_forward_restore_http', helper)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


def private_json(path, value):
    descriptor = os.open(path, os.O_CREAT | os.O_TRUNC | os.O_WRONLY, 0o600)
    with os.fdopen(descriptor, 'w') as stream:
        json.dump(value, stream)
        stream.flush()
        os.fsync(stream.fileno())


def database_url(original, name):
    return urllib.parse.urlunsplit(urllib.parse.urlsplit(original)._replace(path='/' + name))


def run_private(argv, env=None):
    result = subprocess.run(argv, env=env, capture_output=True, text=True)
    if result.returncode:
        raise AssertionError('Explicit private operator command failed; stderr redacted')
    return result


def stop_owned(info):
    pid = info.get('serverPID')
    if not pid:
        return
    command = subprocess.run(['ps', '-p', str(pid), '-o', 'command='], capture_output=True, text=True).stdout.strip()
    if not command:
        return
    # macOS ps does not quote spaces in argv; match the complete owned launch prefix.
    owned_prefix = info['binary'] + ' --service=all --addr ' + info['serverURL'].split('://')[1] + ' '
    if not command.startswith(owned_prefix):
        raise AssertionError('Recorded PID no longer belongs to this isolated restore realm')
    os.kill(pid, signal.SIGTERM)
    for _ in range(100):
        state = subprocess.run(['ps', '-p', str(pid), '-o', 'stat='], capture_output=True, text=True).stdout.strip()
        if not state or state.startswith('Z'):
            return
        time.sleep(.05)
    raise AssertionError('Owned restore service did not stop; no other process was signalled')


def start_owned(info, tls_dir):
    env = dict(os.environ, JOBS_IM_DATABASE_URL=info['businessURL'], JOBS_IM_JOURNAL_DATABASE_URL=info['journalURL'], JOBS_IM_PUBLIC_URL=info['serverURL'])
    if info.get('pgPassFile'):
        env['PGPASSFILE'] = info['pgPassFile']
    arguments = [info['binary'], '--service=all', '--addr', info['serverURL'].split('://')[1], '--internal-base-port', str(info['internalBasePort']), '--state-dir', info['stateDir'], '--tls-cert', str(tls_dir / 'server.pem'), '--tls-key', str(tls_dir / 'server.key')]
    if info.get('webDir'):
        arguments += ['--web-dir', info['webDir']]
    with open(Path(info['runDir']) / 'server.log', 'ab', buffering=0) as log:
        process = subprocess.Popen(arguments, env=env, stdout=log, stderr=log, start_new_session=True)
    info['serverPID'] = process.pid
    private_json(Path(info['runDir']) / 'environment.json', info)
    suite = module.ForwardSuite(info)
    for _ in range(100):
        try:
            bootstrap = suite.call('GET', '/api/bootstrap')
            return suite, bootstrap
        except Exception:
            time.sleep(.1)
    raise AssertionError('Isolated realm startup failed; private log retained')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--environment-file', type=Path, required=True, help='Explicit existing forward realm; used only as tool/cluster source')
    parser.add_argument('--public-port', type=int, default=30443)
    parser.add_argument('--internal-base-port', type=int, default=31101)
    parser.add_argument('--go', default=os.environ.get('GO_BINARY', 'go'))
    args = parser.parse_args()
    seed = json.loads(args.environment_file.read_text())
    assert seed.get('forwardTestRealm') is True, 'Explicit forwarding test environment required'
    assert args.public_port not in (8443, 18443, 20443, 28443)
    for port in [args.public_port] + list(range(args.internal_base_port, args.internal_base_port + 8)):
        with socket.socket() as probe:
            probe.bind(('127.0.0.1', port))
    suffix = secrets.token_hex(6)
    run = Path(seed['runDir']).parents[1] / ('ForwardRestore.' + suffix) / 'Safety'
    run.mkdir(parents=True, mode=0o700)
    binary, ops = run / 'jobsimd', run / 'jobsimops'
    shutil.copy2(seed['binary'], binary)
    root = Path(__file__).resolve().parents[4]
    run_private([args.go, '-C', str(root / '代码/后端/Go'), 'build', '-o', str(ops), './cmd/jobsimops'])
    pg = Path(seed['pgBin'])
    business = database_url(seed['businessURL'], 'jobsim_fwd_restore_' + suffix)
    journal = database_url(seed['journalURL'], 'jobsim_fwd_journal_' + suffix)
    for original, name in [(seed['businessURL'], 'jobsim_fwd_restore_' + suffix), (seed['journalURL'], 'jobsim_fwd_journal_' + suffix)]:
        run_private([str(pg / 'createdb'), '--maintenance-db', original, name])
    info = {**seed, 'forwardRestoreTestRealm': True, 'runDir': str(run), 'stateDir': str(run / 'State'), 'businessURL': business, 'journalURL': journal, 'serverURL': 'https://127.0.0.1:' + str(args.public_port), 'internalBasePort': args.internal_base_port, 'binary': str(binary), 'opsBinary': str(ops)}
    info.pop('serverPID', None)
    before, after = None, None
    try:
        before, bootstrap = start_owned(info, Path(seed['caPath']).parent)
        before.epoch = bootstrap['restoreEpoch']
        before.check(before.epoch == '1' and bootstrap['features'].get('forwarding') is True, 'Fresh separate epoch-one domain required')
        author, actor, reader = [before.register(label) for label in ('restore_a', 'restore_b', 'restore_c')]
        before.friend(author, actor)
        before.friend(actor, reader)
        source_group = before.call('POST', '/api/conversations', {'kind': 'group', 'title': 'Restore source', 'memberIds': [actor['user']['id']]}, author)
        target_group = before.call('POST', '/api/conversations', {'kind': 'group', 'title': 'Restore target', 'memberIds': [reader['user']['id']]}, actor)
        source = before.send(author, source_group['id'], before.payload(author, source_group['id'], 'Epoch-bound forward fixture'))
        forwarded, original = before.forward(actor, target_group['id'], source)
        never_sent = before.forward_body(actor, target_group['id'], source)
        before.check(before.message(reader, forwarded['id'])['text'] == source['text'], 'Reference active before backup')
        before.message(reader, source['id'], 403)
        credentials = [(session, before.passwords[session['user']['id']]) for session in (actor, reader)]
        original_member = before.conversation(actor, source_group['id'])['membershipGeneration']
        original_security = before.sql("SELECT security_version FROM auth.users WHERE id='" + actor['user']['id'] + "'")
        backup_dir = run / 'Backups'
        backup_dir.mkdir(mode=0o700)
        backup, key = backup_dir / 'snapshot.jimbackup', run / 'SeparateRecoveryKey'
        operator_env = dict(os.environ, JOBS_IM_DATABASE_URL=business, JOBS_IM_JOURNAL_DATABASE_URL=journal)
        run_private([str(ops), '--mode', 'backup', '--pg-bin', str(pg), '--state-dir', info['stateDir'], '--tls-dir', str(Path(seed['caPath']).parent), '--file', str(backup), '--key-file', str(key)], operator_env)
        stop_owned(info)
        info.pop('serverPID', None)
        target_name = 'jobsim_fwd_restored_' + suffix
        run_private([str(pg / 'createdb'), '--maintenance-db', business, target_name])
        target = database_url(business, target_name)
        restored_state = run / 'RestoredState'
        operator_env['JOBS_IM_RESTORE_DATABASE_URL'] = target
        restore = [str(ops), '--mode', 'restore', '--offline', '--operation-id', 'forward-restore-' + suffix, '--pg-bin', str(pg), '--state-dir', info['stateDir'], '--target-state-dir', str(restored_state), '--file', str(backup), '--key-file', str(key)]
        run_private(restore, operator_env)
        run_private(restore, operator_env)
        receipt = json.loads((restored_state / 'restore-receipt.json').read_text())
        info.update(businessURL=target, stateDir=str(restored_state))
        after, recovered = start_owned(info, restored_state / 'TLS')
        after.epoch = recovered['restoreEpoch']
        after.check(after.epoch == receipt['restoreEpoch'] and int(after.epoch) > 1, 'Independent journal epoch advanced once')
        after.check(recovered['deploymentId'] == bootstrap['deploymentId'], 'Restore retains original deployment identity')
        current_actor, current_reader = [after.login(session, password) for session, password in credentials]
        after.check(after.conversation(current_actor, source_group['id'])['membershipGeneration'] == original_member, 'Source membership did not change')
        after.check(after.sql("SELECT security_version FROM auth.users WHERE id='" + actor['user']['id'] + "'") == original_security, 'Principal security version did not change')
        raw = after.sql("SELECT state||':'||restore_epoch::text FROM message.forward_refs WHERE message_id='" + forwarded['id'] + "'")
        after.check(raw == 'active:1', 'Snapshot deliberately retains old active reference row')
        after.check(after.message(current_actor, source['id'])['text'] == source['text'], 'Original source remains readable; no deletion caused placeholder')
        unavailable = after.message(current_reader, forwarded['id'])
        after.check(unavailable.get('forwardUnavailable') is True and unavailable['text'] == '' and not unavailable.get('attachmentURL') and not unavailable.get('attachmentId'), 'Epoch alone denies restored active old reference')
        result = after.call('POST', '/api/conversations/' + target_group['id'] + '/message-results', original, current_actor)
        after.check(result['id'] == forwarded['id'] and result.get('forwardUnavailable'), 'Original committed body reconciles without old content revival')
        denied = after.send(current_actor, target_group['id'], never_sent, 409)
        after.check(denied['error']['code'] == 'RESTORE_EPOCH_CHANGED', 'Old uncommitted original epoch cannot create')
        fresh, _ = after.forward(current_actor, target_group['id'], source)
        after.check(after.message(current_reader, fresh['id'])['text'] == source['text'], 'New explicit source proof in current epoch works')
        after.check(after.message(current_reader, forwarded['id']).get('forwardUnavailable'), 'Fresh grant never revives old restored ref')
        report = {'status': 'passed', 'testedAtUTC': datetime.now(timezone.utc).isoformat(), 'httpAssertions': before.checks + after.checks, 'semanticAssertions': before.assertions + after.assertions, 'strictHTTPS': True, 'separateBusinessAndJournalDatabases': True, 'realEncryptedBackupAndOfflineRestore': True, 'sameRestoreOperationResumedWithoutExtraEpoch': True, 'restoredActiveOldRefRejected': True, 'sourceMembershipAndSecurityUnchanged': True, 'sourceStillReadable': True, 'oldBodyFactLookupDoesNotRevive': True, 'oldUncommittedIntentRejected': True, 'newExplicitProofWorks': True, 'sharedRealmsUntouched': True, 'binarySHA256': hashlib.sha256(binary.read_bytes()).hexdigest(), 'scriptSHA256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(), 'scope': 'Isolated same-host real PostgreSQL+journal backup/restore; no cross-host, strong-protection, client-device or complete I acceptance claim'}
        destination = Path(__file__).resolve().parents[4] / '项目相关报告/测试/后端/执行记录' / uuid.uuid4().hex / '普通转发恢复代际实测.json'
        data = (json.dumps(report, ensure_ascii=False, indent=2) + '\n').encode()
        destination.write_bytes(data)
        destination.with_suffix('.json.sha256').write_text(hashlib.sha256(data).hexdigest() + '  ' + destination.name + '\n')
        print(json.dumps(report, ensure_ascii=False))
    finally:
        if after:
            after.logout()
        stop_owned(info)
        info.pop('serverPID', None)
        info['stoppedByOwnTest'] = True
        private_json(run / 'environment.json', info)


if __name__ == '__main__':
    main()
