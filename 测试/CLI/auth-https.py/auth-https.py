#!/usr/bin/env python3
"""Real CLI pseudoterminal, strict HTTPS and explicitly scoped access-expiry clock."""
import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import pty
import secrets
import select
import sqlite3
import ssl
import subprocess
import time
import urllib.error
import urllib.request
import uuid


class Console:
    def __init__(self, command):
        self.master, slave = pty.openpty()
        self.process = subprocess.Popen(command, stdin=slave, stdout=slave, stderr=slave, start_new_session=True)
        os.close(slave)
        self.output = bytearray()
        self.position = 0

    def until(self, value, seconds=25):
        deadline = time.monotonic() + seconds
        pattern = value.encode()
        while time.monotonic() < deadline:
            found = self.output.find(pattern, self.position)
            if found >= 0:
                self.position = found + len(pattern)
                return
            if select.select([self.master], [], [], .1)[0]:
                try:
                    chunk = os.read(self.master, 65536)
                except OSError:
                    chunk = b''
                if chunk:
                    self.output.extend(chunk)
                elif self.process.poll() is not None:
                    break
        raise AssertionError('Native CLI did not reach expected phase; terminal output redacted')

    def send(self, value):
        os.write(self.master, value.encode() + b'\n')

    def command(self, value):
        self.send(value)
        self.until('jobsim> ')

    def close(self):
        if self.process.poll() is None:
            self.process.terminate()
            self.process.wait(timeout=10)
        os.close(self.master)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--environment-file', type=Path, required=True)
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--report-dir', type=Path, required=True)
    args = parser.parse_args()
    environment = json.loads(args.environment_file.read_text())
    assert environment.get('refreshTestRealm') is True
    base = environment['serverURL']
    trust = ssl.create_default_context(cafile=environment['caPath'])
    operator_env = dict(os.environ, PGPASSFILE=environment['pgPassFile'])

    def sql(statement):
        result = subprocess.run([str(Path(environment['pgBin']) / 'psql'), environment['businessURL'], '-XqAt', '-v', 'ON_ERROR_STOP=1', '-c', statement], env=operator_env, capture_output=True, text=True)
        assert result.returncode == 0, 'Owned test database observation failed; details redacted'
        return result.stdout.strip()

    def call(method, path, body=None, auth=None):
        headers = {'Content-Type': 'application/json'}
        if auth:
            headers['Authorization'] = 'Bearer ' + auth['token']
        request = urllib.request.Request(base + path, method=method, data=None if body is None else json.dumps(body).encode(), headers=headers)
        try:
            with urllib.request.urlopen(request, context=trust, timeout=25) as response:
                return json.load(response)
        except urllib.error.HTTPError as error:
            payload = json.load(error)
            raise AssertionError('Real fixture HTTP rejected: ' + payload.get('error', {}).get('code', 'unknown')) from None

    while int(sql("SELECT COALESCE(max(count),0) FROM auth.rate_windows WHERE subject='ip:127.0.0.1' AND window_start=date_trunc('minute',now())")) >= 20:
        time.sleep(min(5, 60 - time.time() % 60 + .05))
    boot = call('GET', '/api/bootstrap')
    assert boot['features'].get('auth_refresh') is True
    username, password = 'cli_console_' + secrets.token_hex(4), 'CLI terminal proof ' + secrets.token_hex(16)
    data = Path(environment['runDir']) / ('CLIConsole.' + secrets.token_hex(4))
    console, control = None, None
    temporary = False
    try:
        command = [str(args.binary.resolve()), '--server', base, '--ca', environment['caPath'], '--data-dir', str(data), 'register']
        console = Console(command)
        console.until('Username: ')
        console.send(username)
        console.until('Type yes: ')
        console.send('yes')
        temporary = True
        console.until('Password: ')
        time.sleep(.05)
        console.send(password)
        console.until('Authenticated.')
        console.until('jobsim> ')
        control = call('POST', '/api/auth/login', {'username': username, 'password': password, 'requestId': str(uuid.uuid4()), 'deviceName': 'CLI terminal independent control'})
        conversation = call('POST', '/api/conversations', {'kind': 'self'}, control)
        auth_files = list(data.rglob('authentication.sqlite'))
        assert len(auth_files) == 1
        with sqlite3.connect(auth_files[0]) as connection:
            metadata = json.loads(connection.execute('SELECT payload FROM authentication_state').fetchone()[0])
        family = metadata['familyId']
        assert metadata['userId'] == control['user']['id'] and metadata['status'] == 'active'
        changed = sql("UPDATE auth.sessions s SET access_expires_at=now()-interval '1 second' FROM auth.instance i WHERE i.singleton AND s.family_id='" + family + "' AND s.user_id='" + metadata['userId'] + "' AND s.restore_epoch=i.restore_epoch RETURNING s.family_id::text")
        assert changed == family
        message = 'CLI real refresh message ' + secrets.token_hex(6)
        console.command('send ' + conversation['id'] + ' ' + message)
        history = call('GET', '/api/conversations/' + conversation['id'] + '/messages?after=0', auth=control)
        assert sum(item.get('text') == message for item in history['items']) == 1
        assert sql("SELECT refresh_generation::text FROM auth.sessions WHERE family_id='" + family + "'") == '2'
        assert sql("SELECT count(*) FROM auth.refresh_token_history WHERE family_id='" + family + "'") == '2'
        console.send('quit')
        assert console.process.wait(timeout=20) == 0
        assert sql("SELECT (revoked_at IS NOT NULL)::text FROM auth.sessions WHERE family_id='" + family + "'") == 'true'
        with sqlite3.connect(auth_files[0]) as connection:
            persisted = connection.execute('SELECT payload FROM authentication_state').fetchone()[0]
        assert json.loads(persisted)['status'] == 'signed_out'
        forbidden = [password.encode(), control['token'].encode(), control['refreshToken'].encode()]
        assert not any(value in console.output or value.decode() in persisted for value in forbidden)
        assert b'"refreshToken"' not in console.output and b'"token"' not in console.output
        # Default unattended execution must not silently choose the memory path.
        unattended = 'cli_no_key_' + secrets.token_hex(4)
        blocked = subprocess.run([str(args.binary.resolve()), '--server', base, '--ca', environment['caPath'], '--data-dir', str(data / 'Unattended'), 'register'], input=unattended + '\nnot submitted password\n', text=True, capture_output=True, timeout=30)
        assert blocked.returncode != 0 and 'unattended login is refused' in blocked.stderr
        assert sql("SELECT count(*) FROM auth.users WHERE username='" + unattended + "'") == '0'
        report = {'status': 'passed', 'testedAtUTC': datetime.now(timezone.utc).isoformat(), 'strictHTTPS': True, 'actualCLIPseudoterminal': True, 'explicitTemporaryModeSelected': temporary, 'defaultUnattendedRefusedBeforeAuthenticationHTTP': True, 'realAccessExpiredAndOneRefresh': True, 'clockFixture': 'only this owned family access deadline; not fifteen minutes wall time', 'realMessageExactlyOnceAndPeerReadback': True, 'onlineQuitRevokesOriginalFamily': True, 'nonsecretMetadataDurable': True, 'secretsNotInTerminalOrSQLite': True, 'systemKeychainPositiveRoundtrip': False, 'binarySHA256': hashlib.sha256(args.binary.read_bytes()).hexdigest(), 'scriptSHA256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(), 'scope': 'Actual macOS CLI limited process mode under a genuinely unavailable system store; secure default fails closed. No full I/three-platform/native system-store acceptance.'}
        args.report_dir.mkdir(parents=True, exist_ok=True)
        destination = args.report_dir / '终端HTTPS结果.json'
        encoded = (json.dumps(report, ensure_ascii=False, indent=2) + '\n').encode()
        destination.write_bytes(encoded)
        destination.with_suffix('.json.sha256').write_text(hashlib.sha256(encoded).hexdigest() + '  ' + destination.name + '\n')
        print(json.dumps(report, ensure_ascii=False))
    finally:
        if console:
            console.close()
        if control:
            call('POST', '/api/auth/logout', {}, control)


if __name__ == '__main__':
    main()
