#!/usr/bin/env python3
"""Real isolated backup/restore rejects old access, refresh, BFF and retired intents."""
import argparse
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import secrets
import shutil
import socket
import uuid

helper = Path(__file__).resolve().parents[1] / 'session_refresh.py/session_refresh.py'
spec = importlib.util.spec_from_file_location('jobsim_refresh_restore', helper)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
realm = module.realm


def run(seed, go):
    assert seed.get('refreshTestRealm') is True
    for port in [34443] + list(range(35101, 35109)):
        with socket.socket() as probe:
            probe.bind(('127.0.0.1', port))
    suffix = secrets.token_hex(6)
    directory = Path(seed['runDir']).parents[1] / ('RefreshRestore.' + suffix) / 'Safety'
    directory.mkdir(parents=True, mode=0o700)
    binary, ops = directory / 'jobsimd-refresh-restore', directory / 'jobsimops'
    shutil.copy2(seed['binary'], binary)
    root = Path(__file__).resolve().parents[4]
    realm.run_private([go, '-C', str(root / '代码/后端/Go'), 'build', '-o', str(ops), './cmd/jobsimops'])
    pg = Path(seed['pgBin'])
    private_env = dict(os.environ)
    if seed.get('pgPassFile'):
        private_env['PGPASSFILE'] = seed['pgPassFile']
    business_name, journal_name = 'jobsim_rt_restore_' + suffix, 'jobsim_rt_journal_' + suffix
    for original, name in [(seed['businessURL'], business_name), (seed['journalURL'], journal_name)]:
        realm.run_private([str(pg / 'createdb'), '--maintenance-db', original, name], private_env)
    business, journal = realm.database_url(seed['businessURL'], business_name), realm.database_url(seed['journalURL'], journal_name)
    info = {**seed, 'refreshRestoreTestRealm': True, 'runDir': str(directory), 'stateDir': str(directory / 'State'), 'businessURL': business, 'journalURL': journal, 'serverURL': 'https://127.0.0.1:34443', 'internalBasePort': 35101, 'binary': str(binary), 'opsBinary': str(ops)}
    info.pop('serverPID', None)
    before, after = None, None
    try:
        _, boot = realm.start_owned(info, Path(seed['caPath']).parent)
        before = module.RefreshSuite(info)
        before.epoch = boot['restoreEpoch']
        actor = before.register('restore_rt')
        control = before.login(actor)
        browser, _, web_body, web_auth = before.web('rbff')
        never_sent = {'username': actor['user']['username'], 'password': before.passwords[actor['user']['id']], 'requestId': str(uuid.uuid4()), 'deviceName': 'retired-after-old-snapshot'}
        backup_dir = Path(seed.get('backupRoot', directory / 'Backups')) / ('RefreshRestore.' + suffix)
        backup_dir.mkdir(parents=True, mode=0o700)
        backup, key = backup_dir / 'before-retirement.jimbackup', directory / 'SeparateRecoveryKey'
        operator_env = dict(private_env, JOBS_IM_DATABASE_URL=business, JOBS_IM_JOURNAL_DATABASE_URL=journal)
        realm.run_private([str(ops), '--mode', 'backup', '--pg-bin', str(pg), '--state-dir', info['stateDir'], '--tls-dir', str(Path(seed['caPath']).parent), '--file', str(backup), '--key-file', str(key)], operator_env)
        replacement_body = {'username': actor['user']['username'], 'password': before.passwords[actor['user']['id']], 'previousRequestId': never_sent['requestId'], 'requestId': str(uuid.uuid4()), 'deviceName': 'post-snapshot-retirement'}
        replacement = before.call('POST', '/api/auth/reauthenticate', replacement_body)
        before.sessions.append(replacement)
        before.check(before.call('POST', '/api/auth/login', never_sent, expect=401)['error']['code'] == 'AUTHENTICATION_RETIRED', 'Actual retirement applied after snapshot')
        realm.stop_owned(info)
        info.pop('serverPID', None)
        target_name = 'jobsim_rt_restored_' + suffix
        realm.run_private([str(pg / 'createdb'), '--maintenance-db', business, target_name], private_env)
        target = realm.database_url(business, target_name)
        restored = directory / 'RestoredState'
        operator_env['JOBS_IM_RESTORE_DATABASE_URL'] = target
        restore_args = [str(ops), '--mode', 'restore', '--offline', '--operation-id', 'refresh-restore-' + suffix, '--pg-bin', str(pg), '--state-dir', info['stateDir'], '--target-state-dir', str(restored), '--file', str(backup), '--key-file', str(key)]
        realm.run_private(restore_args, operator_env)
        realm.run_private(restore_args, operator_env)
        info.update(businessURL=target, stateDir=str(restored))
        _, recovered = realm.start_owned(info, restored / 'TLS')
        after = module.RefreshSuite(info)
        after.epoch = recovered['restoreEpoch']
        after.check(int(after.epoch) > int(before.epoch) and recovered['deploymentId'] == boot['deploymentId'], 'Restore advances independent epoch without changing instance identity')
        for previous in (actor, control, replacement):
            after.call('GET', '/api/me', session=previous, expect=401)
            after.refresh(previous, expect=401)
        after.check(after.sql("SELECT count(*) FROM auth.refresh_token_history WHERE family_id='" + actor['familyId'] + "' AND state='active'") == '0', 'Restored old active refresh material remains revoked')
        after.check(after.sql("SELECT count(*) FROM auth.bff_sessions WHERE family_id='" + web_auth['familyId'] + "' AND upstream_credentials_ciphertext IS NULL") == '1', 'Restored browser upstream secrets are cleared')
        after.web_call(browser, 'GET', '/api/me', expect=401)
        after.web_call(browser, 'GET', '/api/auth/refresh-context', expect=401)
        after.check(after.sql("SELECT count(*) FROM auth.authentication_retirements WHERE user_id='" + actor['user']['id'] + "' AND request_id='" + never_sent['requestId'] + "'") == '1', 'Independent journal replay reconstructs post-snapshot retirement fence')
        after.check(after.call('POST', '/api/auth/login', never_sent, expect=401)['error']['code'] == 'AUTHENTICATION_RETIRED', 'Restore cannot revive a retired late initial login')
        fresh = after.call('POST', '/api/auth/reauthenticate', {'username': actor['user']['username'], 'password': before.passwords[actor['user']['id']], 'previousRequestId': replacement_body['requestId'], 'requestId': str(uuid.uuid4()), 'deviceName': 'current-epoch-proof'})
        after.sessions.append(fresh)
        after.check(fresh['restoreEpoch'] == after.epoch and fresh['familyId'] != actor['familyId'], 'Fresh verified recovery works in current epoch only')
        after.refresh(fresh)
        new_web = after.web_call(browser, 'POST', '/api/auth/login', {**web_body, 'requestId': str(uuid.uuid4())})
        browser.auth_family = new_web['familyId']
        csrf = after.web_call(browser, 'GET', '/api/auth/csrf')['csrfToken']
        after.web_call(browser, 'POST', '/api/auth/confirm', {}, csrf=csrf)
        after.browsers.append(browser)
        after.check(after.web_call(browser, 'GET', '/api/me')['id'] == web_auth['user']['id'], 'Restored Web account must explicitly authenticate and confirm a new BFF family')
        return {'status': 'passed', 'httpAssertions': before.checks + after.checks, 'semanticAssertions': before.assertions + after.assertions, 'strictHTTPS': True, 'realEncryptedBackupOfflineRestore': True, 'sameRestoreOperationDoesNotAllocateTwice': True, 'oldAccessAndRefreshRejected': True, 'oldBFFCookieRejectedAndCiphertextCleared': True, 'postSnapshotRetirementReplayedFromIndependentJournal': True, 'retiredLateAuthenticationCannotReappear': True, 'newVerifiedAuthenticationWorks': True, 'binarySHA256': hashlib.sha256(binary.read_bytes()).hexdigest(), 'scriptSHA256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(), 'testedAtUTC': datetime.now(timezone.utc).isoformat(), 'scope': 'New isolated source/target and independent journal databases; only its recorded PID stopped; no shared realm or strong protection acceptance'}
    finally:
        if after:
            after.logout()
        realm.stop_owned(info)
        info.pop('serverPID', None)
        info['stoppedByOwnTest'] = True
        realm.private_json(directory / 'environment.json', info)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--environment-file', type=Path, required=True)
    parser.add_argument('--go', default=os.environ.get('GO_BINARY', 'go'))
    parser.add_argument('--report', type=Path, help='Separate report preserves historical namespace evidence')
    args = parser.parse_args()
    report = run(json.loads(args.environment_file.read_text()), args.go)
    destination = args.report or Path(__file__).resolve().parents[2] / '报告/登录刷新恢复拒旧实测.json'
    data = (json.dumps(report, ensure_ascii=False, indent=2) + '\n').encode()
    destination.write_bytes(data)
    destination.with_suffix('.json.sha256').write_text(hashlib.sha256(data).hexdigest() + '  ' + destination.name + '\n')
    print(json.dumps(report, ensure_ascii=False))


if __name__ == '__main__':
    main()
