#!/usr/bin/env python3
"""Explicit isolated realm: real old-schema migration and strict refresh family tests."""
import argparse
import concurrent.futures
from datetime import datetime, timezone
import hashlib
import http.cookiejar
import importlib.util
import json
import os
from pathlib import Path
import secrets
import shutil
import socket
import ssl
import subprocess
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid

helper = Path(__file__).resolve().parents[1] / 'forward_restore.py/forward_restore.py'
spec = importlib.util.spec_from_file_location('jobsim_refresh_realm', helper)
realm = importlib.util.module_from_spec(spec)
spec.loader.exec_module(realm)


class RefreshSuite(realm.module.ForwardSuite):
    def __init__(self, environment):
        super().__init__(environment)
        self.browsers = []

    def wait_auth_window(self, username=None):
        subjects = "subject='ip:127.0.0.1' AND count>=25"
        if username:
            assert all(c.isalnum() or c == '_' for c in username)
            subjects = '(' + subjects + ") OR (subject='user:" + username + "' AND count>=4)"
        while int(self.sql("SELECT count(*) FROM auth.rate_windows WHERE (" + subjects + ") AND window_start=date_trunc('minute',now())")):
            time.sleep(min(5, 60 - time.time() % 60 + .05))

    def call(self, method, path, body=None, session=None, expect=200):
        if method == 'POST' and path in ('/api/auth/register', '/api/auth/login', '/api/auth/reauthenticate'):
            self.wait_auth_window((body or {}).get('username'))
        return super().call(method, path, body, session, expect)

    def refresh_body(self, auth):
        return {'requestId': str(uuid.uuid4()), 'familyId': auth['familyId'], 'refreshGeneration': auth['refreshGeneration'], 'refreshToken': auth['refreshToken']}

    def refresh(self, auth, expect=200, body=None):
        result = self.call('POST', '/api/auth/refresh', body or self.refresh_body(auth), expect=expect)
        if expect == 200:
            self.sessions.append(result)
        return result

    def web(self, label):
        jar = http.cookiejar.CookieJar()
        opener = urllib.request.build_opener(urllib.request.HTTPSHandler(context=self.trust), urllib.request.HTTPCookieProcessor(jar))
        body = {'username': 'refresh_web_' + label + secrets.token_hex(5), 'password': 'Refresh web test ' + secrets.token_hex(16), 'requestId': str(uuid.uuid4()), 'deviceName': 'web-refresh-test'}
        auth = self.web_call(opener, 'POST', '/api/auth/register', body)
        opener.auth_family = auth['familyId']
        self.check('token' not in auth and 'refreshToken' not in auth, 'Web authentication returns no upstream secrets')
        csrf = self.web_call(opener, 'GET', '/api/auth/csrf')['csrfToken']
        self.web_call(opener, 'POST', '/api/auth/confirm', {}, csrf=csrf)
        self.browsers.append(opener)
        return opener, jar, body, auth

    def web_call(self, opener, method, path, body=None, csrf=None, expect=200, origin=None):
        if method == 'POST' and path in ('/api/auth/register', '/api/auth/login', '/api/auth/reauthenticate'):
            self.wait_auth_window((body or {}).get('username'))
        headers = {'X-JobsIM-Client': 'web', 'Content-Type': 'application/json'}
        if getattr(opener, 'auth_family', None):
            headers['X-JobsIM-Auth-Family'] = opener.auth_family
        if method != 'GET':
            headers['Origin'] = origin or self.base
        if csrf is not None:
            headers['X-CSRF-Token'] = csrf
        request = urllib.request.Request(self.base + path, method=method, data=None if body is None else json.dumps(body).encode(), headers=headers)
        try:
            with opener.open(request, timeout=25) as response:
                status, raw = response.status, response.read()
        except urllib.error.HTTPError as error:
            status, raw = error.code, error.read()
        if status != expect:
            code = json.loads(raw).get('error', {}).get('code', 'unknown')
            raise AssertionError(f'{method} {path}: expected {expect}, got {status}/{code}; response redacted')
        self.checks += 1
        return json.loads(raw) if raw else None

    def expire_access(self, auth):
        self.sql("UPDATE auth.sessions SET access_expires_at=now()-interval '1 second' WHERE family_id='" + auth['familyId'] + "'")

    def run(self, legacy):
        boot = self.call('GET', '/api/bootstrap')
        self.epoch = boot['restoreEpoch']
        self.check(boot['features'].get('auth_refresh') is True, 'New immutable refresh capability')
        if legacy is not None:
            self.call('GET', '/api/me', session=legacy['auth'], expect=401)
            self.check(self.sql("SELECT refresh_generation::text||':'||(family_id IS NULL)::text||':'||(revoked_at IS NOT NULL)::text FROM auth.sessions WHERE device_id='" + legacy['auth']['deviceId'] + "'") == '0:true:true', 'Actual old seven-day session is revoked, not promoted to refresh')
            self.call('POST', '/api/auth/register', legacy['body'], expect=401)

        first = self.register('refresh_a')
        second = self.login(first)
        self.check(first['familyId'] != second['familyId'] and first['deviceId'] != second['deviceId'], 'Independent authentications have independent families')
        for key in ('accessExpiresAt', 'idleExpiresAt', 'absoluteExpiresAt'):
            self.check(datetime.fromisoformat(first[key]).utcoffset().total_seconds() == 0, 'Expiry is UTC')
        lifetime = datetime.fromisoformat(first['absoluteExpiresAt']) - datetime.fromisoformat(first['accessExpiresAt'])
        self.check(abs(lifetime.total_seconds() - (90 * 86400 - 900)) < 2, '15-minute access and frozen ninety-day absolute start')
        self.expire_access(first)
        self.check(self.call('GET', '/api/me', session=first, expect=401)['error']['code'] == 'ACCESS_EXPIRED', 'Expired access does not silently become refresh')
        rotated = self.refresh(first)
        self.check(rotated['familyId'] == first['familyId'] and rotated['refreshGeneration'] == '2' and rotated['absoluteExpiresAt'] == first['absoluteExpiresAt'], 'Rotation advances exactly one generation and retains absolute deadline')
        self.call('GET', '/api/me', session=rotated)
        self.call('GET', '/api/me', session=first, expect=401)
        history = self.sql("SELECT string_agg(state,',' ORDER BY refresh_generation) FROM auth.refresh_token_history WHERE family_id='" + first['familyId'] + "'")
        self.check(history == 'used,active', 'History keeps consumed digest and only one active generation')
        self.check(self.refresh(first, 401)['error']['code'] == 'REFRESH_REUSE_DETECTED', 'Consumed token revokes only its own family')
        self.call('GET', '/api/me', session=rotated, expect=401)
        self.refresh(rotated, 401)
        self.call('GET', '/api/me', session=second)
        self.refresh(second)

        racing_auth = self.register('refresh_race')
        original = self.refresh_body(racing_auth)
        def racing():
            request = urllib.request.Request(self.base + '/api/auth/refresh', method='POST', data=json.dumps(original).encode(), headers={'Content-Type': 'application/json'})
            try:
                with urllib.request.urlopen(request, context=self.trust, timeout=25) as response:
                    return response.status, json.load(response)
            except urllib.error.HTTPError as error:
                return error.code, json.load(error)
        with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
            results = list(pool.map(lambda _: racing(), range(2)))
        self.checks += 2
        self.check(sorted(code for code, _ in results) == [200, 401], 'Exactly one concurrent consume, other detects reuse')
        success = next(body for code, body in results if code == 200)
        self.sessions.append(success)
        self.call('GET', '/api/me', session=success, expect=401)
        self.check(self.sql("SELECT count(*) FROM auth.refresh_token_history WHERE family_id='" + racing_auth['familyId'] + "'") == '2', 'No concurrent second successor generation')

        actor = self.register('refresh_unknown')
        original = self.refresh_body(actor)
        self.lose_refresh_response(actor, original)
        self.check(self.sql("SELECT refresh_generation FROM auth.sessions WHERE family_id='" + actor['familyId'] + "'") == '2', 'Actual unconsumed HTTP response committed one rotation')
        replacement = {'username': actor['user']['username'], 'password': self.passwords[actor['user']['id']], 'previousFamilyId': actor['familyId'], 'deviceName': 'verified-reauthentication', 'requestId': str(uuid.uuid4())}
        self.call('POST', '/api/auth/reauthenticate', dict(replacement, password='Wrong valid password phrase'), expect=401)
        new = self.call('POST', '/api/auth/reauthenticate', replacement)
        self.sessions.append(new)
        self.check(new['familyId'] != actor['familyId'], 'Unknown result requires verified old-family revoke then new authentication')
        self.check(self.call('POST', '/api/auth/reauthenticate', replacement)['familyId'] == new['familyId'], 'New authentication original delivery remains distinct from refresh')
        self.refresh(actor, 401)
        self.call('GET', '/api/me', session=new)
        wrong_owner = self.register('refresh_other')
        denied = {'username': wrong_owner['user']['username'], 'password': self.passwords[wrong_owner['user']['id']], 'previousFamilyId': new['familyId'], 'deviceName': 'wrong-family', 'requestId': str(uuid.uuid4())}
        self.call('POST', '/api/auth/reauthenticate', denied, expect=403)
        self.call('GET', '/api/me', session=new)

        bounded = self.register('refresh_absolute')
        self.sql("UPDATE auth.sessions SET absolute_expires_at=now()+interval '5 seconds',idle_expires_at=now()+interval '4 seconds',expires_at=now()+interval '4 seconds' WHERE family_id='" + bounded['familyId'] + "'")
        bounded = self.refresh(bounded)
        self.check(bounded['idleExpiresAt'] == bounded['absoluteExpiresAt'] and bounded['accessExpiresAt'] == bounded['absoluteExpiresAt'], 'Idle and access are clamped to original absolute deadline')
        self.sql("UPDATE auth.sessions SET absolute_expires_at=now()-interval '1 second',idle_expires_at=now()-interval '1 second',expires_at=now()-interval '1 second' WHERE family_id='" + bounded['familyId'] + "'")
        self.check(self.refresh(bounded, 401)['error']['code'] == 'REFRESH_EXPIRED', 'Absolute expiry cannot be renewed')
        idle = self.register('refresh_idle')
        self.sql("UPDATE auth.sessions SET idle_expires_at=now()-interval '1 second',expires_at=now()-interval '1 second' WHERE family_id='" + idle['familyId'] + "'")
        self.check(self.refresh(idle, 401)['error']['code'] == 'REFRESH_EXPIRED', 'Idle expiry cannot be renewed')
        self.web_cases()
	
        self.unknown_initial_authentication()
        self.security_and_scope_guards()
        self.retirement_cases()
        self.complete_unknown_closure()
        return {'status': 'passed', 'httpAssertions': self.checks, 'semanticAssertions': self.assertions, 'strictHTTPS': True, 'realOldSchemaMigration': legacy is not None, 'bffProcessRestartReexercised': self.environment.get('allowOwnedRestartForTests', True), 'fullUnknownPredecessorClosure': True, 'singleConsumeConcurrentReuseFamilyOnly': True, 'realLostSuccessResponseThenVerifiedReauth': True, 'absoluteIdleBoundaries': 'explicit per-test SQL deadline fixtures, not ninety days of wall-clock waiting', 'webBFFSecretsNeverReachJavaScript': True, 'scope': 'Isolated same-host PostgreSQL functional lifecycle; no full I or strong-protection acceptance'}

    def complete_unknown_closure(self):
        self.check(self.call('GET', '/api/bootstrap')['features'].get('auth_retirement_chain') is True, 'Closure semantics has its own immutable capability')
        actor = self.register('chain')
        control = self.login(actor)
        proof = {'username': actor['user']['username'], 'password': self.passwords[actor['user']['id']]}
        r0 = dict(proof, requestId=str(uuid.uuid4()), deviceName='unknown-ancestor')
        self.wait_auth_window(proof['username'])
        endpoint = urllib.parse.urlsplit(self.base)
        data = json.dumps(r0).encode()
        with socket.create_connection((endpoint.hostname, endpoint.port), timeout=15) as connection:
            with self.trust.wrap_socket(connection, server_hostname=endpoint.hostname) as secure:
                secure.sendall((f'POST /api/auth/login HTTP/1.1\r\nHost: {endpoint.netloc}\r\nContent-Type: application/json\r\nContent-Length: {len(data)}\r\nConnection: close\r\n\r\n').encode() + data)
                for _ in range(100):
                    if self.sql("SELECT count(*) FROM auth.local_auth_requests WHERE request_id='" + r0['requestId'] + "'") == '1':
                        break
                    time.sleep(.02)
                else:
                    raise AssertionError('Unknown ancestor did not commit in response-loss window')
        family0 = self.sql("SELECT family_id::text FROM auth.local_auth_requests WHERE request_id='" + r0['requestId'] + "'")
        # R1 is intentionally persisted only in test memory and never sent.
        r1 = dict(proof, requestId=str(uuid.uuid4()), previousRequestId=r0['requestId'], deviceName='never-arrived-successor')
        r2 = dict(proof, requestId=str(uuid.uuid4()), previousRequestId=r1['requestId'], previousRequestIds=[r0['requestId'], r1['requestId']], deviceName='verified-full-closure')
        fresh = self.call('POST', '/api/auth/reauthenticate', r2)
        self.sessions.append(fresh)
        self.check(self.sql("SELECT (revoked_at IS NOT NULL)::text FROM auth.sessions WHERE family_id='" + family0 + "'") == 'true', 'Full closure revokes committed R0 even though R1 never reached server')
        self.check(self.call('POST', '/api/auth/reauthenticate', r1, expect=401)['error']['code'] == 'AUTHENTICATION_RETIRED', 'Late never-arrived R1 cannot create orphan family')
        self.check(self.call('POST', '/api/auth/login', r0, expect=401)['error']['code'] in ('AUTHENTICATION_RETIRED', 'DELIVERY_EXPIRED', 'DELIVERY_INVALIDATED'), 'Original committed ancestor cannot redeliver retired secrets')
        self.call('GET', '/api/me', session=control)
        self.call('GET', '/api/me', session=fresh)
        self.check(self.sql("SELECT count(*) FROM auth.authentication_retirements WHERE user_id='" + actor['user']['id'] + "' AND request_id IN ('" + r0['requestId'] + "','" + r1['requestId'] + "')") == '2', 'Unbuilt and built authentication facts both have durable retirement fences')
        # A committed R0->R1 edge is retained when R2 closes the entire path.
        next_actor = self.register('edge')
        next_proof = {'username': next_actor['user']['username'], 'password': self.passwords[next_actor['user']['id']]}
        a0 = dict(next_proof, requestId=str(uuid.uuid4()), deviceName='edge-r0')
        f0 = self.call('POST', '/api/auth/login', a0)
        self.sessions.append(f0)
        a1 = dict(next_proof, requestId=str(uuid.uuid4()), previousRequestId=a0['requestId'], deviceName='edge-r1')
        f1 = self.call('POST', '/api/auth/reauthenticate', a1)
        self.sessions.append(f1)
        missing = dict(next_proof, requestId=str(uuid.uuid4()), previousRequestId=a0['requestId'], deviceName='missing-edge')
        self.check(self.call('POST', '/api/auth/reauthenticate', missing, expect=409)['error']['code'] == 'PREVIOUS_AUTHENTICATION_SUPERSEDED', 'Incomplete known-successor closure cannot branch')
        a2 = dict(next_proof, requestId=str(uuid.uuid4()), previousRequestId=a1['requestId'], previousRequestIds=[a0['requestId'], a1['requestId']], deviceName='edge-r2')
        f2 = self.call('POST', '/api/auth/reauthenticate', a2)
        self.sessions.append(f2)
        self.call('GET', '/api/me', session=f1, expect=401)
        self.call('GET', '/api/me', session=f2)
        self.call('GET', '/api/me', session=next_actor)
        self.check(self.sql("SELECT retired_by_request_id FROM auth.authentication_retirements WHERE user_id='" + next_actor['user']['id'] + "' AND request_id='" + a0['requestId'] + "'") == a1['requestId'], 'Existing retirement edge is preserved instead of rewriting journal facts')
        foreign = dict(next_proof, requestId=str(uuid.uuid4()), previousRequestIds=[r2['requestId']], deviceName='foreign-closure')
        self.call('POST', '/api/auth/reauthenticate', foreign, expect=403)
        self.call('GET', '/api/me', session=fresh)

    def lose_refresh_response(self, auth, body):
        endpoint = urllib.parse.urlsplit(self.base)
        data = json.dumps(body).encode()
        with socket.create_connection((endpoint.hostname, endpoint.port), timeout=15) as connection:
            with self.trust.wrap_socket(connection, server_hostname=endpoint.hostname) as secure:
                secure.sendall((f'POST /api/auth/refresh HTTP/1.1\r\nHost: {endpoint.netloc}\r\nContent-Type: application/json\r\nContent-Length: {len(data)}\r\nConnection: close\r\n\r\n').encode() + data)
                for _ in range(100):
                    if self.sql("SELECT refresh_generation FROM auth.sessions WHERE family_id='" + auth['familyId'] + "'") == '2':
                        break
                    time.sleep(.02)
                else:
                    raise AssertionError('Refresh did not commit within controlled real response-loss window')

    def unknown_initial_authentication(self):
        actor = self.register('initial_unknown')
        body = {'username': actor['user']['username'], 'password': self.passwords[actor['user']['id']], 'previousFamilyId': actor['familyId'], 'requestId': str(uuid.uuid4()), 'deviceName': 'initial-response-loss'}
        endpoint = urllib.parse.urlsplit(self.base)
        data = json.dumps(body).encode()
        with socket.create_connection((endpoint.hostname, endpoint.port), timeout=15) as connection:
            with self.trust.wrap_socket(connection, server_hostname=endpoint.hostname) as secure:
                secure.sendall((f'POST /api/auth/reauthenticate HTTP/1.1\r\nHost: {endpoint.netloc}\r\nContent-Type: application/json\r\nContent-Length: {len(data)}\r\nConnection: close\r\n\r\n').encode() + data)
                for _ in range(100):
                    count = self.sql("SELECT count(*) FROM auth.local_auth_requests WHERE request_id='" + body['requestId'] + "'")
                    if count == '1':
                        break
                    time.sleep(.02)
                else:
                    raise AssertionError('Initial reauthentication did not commit in controlled real response-loss window')
        unknown_family = self.sql("SELECT s.family_id::text FROM auth.local_auth_requests a JOIN auth.sessions s ON s.user_id=a.user_id AND s.device_id=a.device_id WHERE a.request_id='" + body['requestId'] + "'")
        self.sql("UPDATE auth.local_auth_requests SET expires_at=now()-interval '1 second' WHERE request_id='" + body['requestId'] + "'")
        for _ in range(100):
            if self.sql("SELECT octet_length(delivery_ciphertext) FROM auth.local_auth_requests WHERE request_id='" + body['requestId'] + "'") == '0':
                break
            time.sleep(.05)
        else:
            raise AssertionError('Expired initial secret was not cleared by its real worker')
        self.check(self.call('POST', '/api/auth/reauthenticate', body, expect=401)['error']['code'] == 'DELIVERY_EXPIRED', 'Expired initial delivery cannot reveal old secrets')
        controlled = {'username': actor['user']['username'], 'password': self.passwords[actor['user']['id']], 'previousRequestId': body['requestId'], 'requestId': str(uuid.uuid4()), 'deviceName': 'controlled-initial-unknown-recovery'}
        fresh = self.call('POST', '/api/auth/reauthenticate', controlled)
        self.sessions.append(fresh)
        self.check(fresh['familyId'] != unknown_family and self.sql("SELECT (revoked_at IS NOT NULL)::text FROM auth.sessions WHERE family_id='" + unknown_family + "'") == 'true', 'New verified authentication revokes original unknown family using retained nonsecret mapping')
        self.check(self.sql("SELECT octet_length(delivery_ciphertext)::text FROM auth.local_auth_requests WHERE request_id='" + body['requestId'] + "'") == '0', 'Mapping remains usable after all initial delivery ciphertext is cleared')
        self.call('GET', '/api/me', session=fresh)

    def security_and_scope_guards(self):
        actor = self.register('security')
        another = self.login(actor)
        codes = self.call('POST', '/api/auth/recovery-codes', {'password': self.passwords[actor['user']['id']], 'requestId': str(uuid.uuid4())}, actor)
        self.call('POST', '/api/auth/recover', {'username': actor['user']['username'], 'code': codes['codes'][0], 'password': 'Changed refresh test ' + secrets.token_hex(16), 'requestId': str(uuid.uuid4())})
        self.refresh(actor, 401)
        self.refresh(another, 401)
        self.check(self.sql("SELECT count(*) FROM auth.sessions WHERE user_id='" + actor['user']['id'] + "' AND revoked_at IS NULL") == '0', 'Real account recovery revokes every family of that account')
        for column, label in [('security_version', 'security'), ('restore_epoch', 'restore'), ('source_generation', 'source')]:
            stale = self.register(label + '_guard')
            self.sql("UPDATE auth.sessions SET " + column + "=0 WHERE family_id='" + stale['familyId'] + "'")
            self.call('GET', '/api/me', session=stale, expect=401)
            self.refresh(stale, 401)
        precise = self.register('precise')
        self.sql("UPDATE auth.refresh_token_history SET refresh_generation=9007199254740993 WHERE family_id='" + precise['familyId'] + "'; UPDATE auth.sessions SET refresh_generation=9007199254740993 WHERE family_id='" + precise['familyId'] + "'")
        precise['refreshGeneration'] = '9007199254740993'
        successor = self.refresh(precise)
        self.check(successor['refreshGeneration'] == '9007199254740994', 'Large generation remains an exact decimal string')

    def retirement_cases(self):
        actor = self.register('retirement')
        control = self.login(actor)
        never_sent = {'username': actor['user']['username'], 'password': self.passwords[actor['user']['id']], 'requestId': str(uuid.uuid4()), 'deviceName': 'late-original-authentication'}
        linked = {'username': actor['user']['username'], 'password': self.passwords[actor['user']['id']], 'previousRequestId': never_sent['requestId'], 'requestId': str(uuid.uuid4()), 'deviceName': 'verified-retirement'}
        fresh = self.call('POST', '/api/auth/reauthenticate', linked)
        self.sessions.append(fresh)
        self.check(self.call('POST', '/api/auth/login', never_sent, expect=401)['error']['code'] == 'AUTHENTICATION_RETIRED', 'Never-arrived initial login cannot create when it arrives after verified retirement')
        self.check(self.sql("SELECT count(*) FROM auth.local_auth_requests WHERE request_id='" + never_sent['requestId'] + "'") == '0', 'Late original authentication created no orphan family')
        self.check(self.call('POST', '/api/auth/reauthenticate', dict(linked, requestId=str(uuid.uuid4())), expect=409)['error']['code'] == 'PREVIOUS_AUTHENTICATION_SUPERSEDED', 'A retired predecessor cannot branch a second unknown family')
        self.call('GET', '/api/me', session=control)
        self.call('GET', '/api/me', session=fresh)
        # Account creation is not inferred from a local timeout or a request ID.
        username = 'never_registered_' + secrets.token_hex(5)
        password = 'Not registered proof ' + secrets.token_hex(16)
        original_id = str(uuid.uuid4())
        request = {'username': username, 'password': password, 'previousRequestId': original_id, 'requestId': str(uuid.uuid4()), 'deviceName': 'no-account-proof'}
        self.call('POST', '/api/auth/reauthenticate', request, expect=401)
        self.check(self.sql("SELECT count(*) FROM auth.users WHERE username='" + username + "'") == '0', 'Reauthentication cannot create a never-registered account')
        registered = self.call('POST', '/api/auth/register', {'username': username, 'password': password, 'requestId': original_id, 'deviceName': 'original-registration'})
        self.sessions.append(registered)
        self.check(registered['user']['username'] == username, 'Uncreated registration can still use its original ID')

    def web_cases(self):
        opener, jar, request, auth = self.web('a')
        cookie = next(iter(jar))
        self.check(cookie.name == '__Host-jobsim_session' and cookie.secure and cookie.has_nonstandard_attr('HttpOnly'), 'Only opaque secure HttpOnly browser cookie')
        digest = hashlib.sha256(cookie.value.encode()).hexdigest()
        self.check(self.sql("SELECT count(*) FROM auth.bff_sessions WHERE cookie_digest=decode('" + digest + "','hex') AND state='active' AND upstream_credentials_ciphertext IS NOT NULL") == '1', 'BFF stores sealed upstream pair')
        self.check(self.sql("SELECT count(*) FROM auth.refresh_token_history WHERE token_digest=decode('" + digest + "','hex')") == '0', 'Cookie is not an upstream refresh token')
        self.expire_access(auth)
        self.web_call(opener, 'GET', '/api/me', expect=401)
        context = self.web_call(opener, 'GET', '/api/auth/refresh-context')
        body = {'requestId': str(uuid.uuid4()), 'familyId': context['familyId'], 'refreshGeneration': context['refreshGeneration']}
        self.web_call(opener, 'POST', '/api/auth/refresh', body, expect=403)
        self.web_call(opener, 'POST', '/api/auth/refresh', body, csrf=context['csrfToken'], origin='https://invalid.example', expect=403)
        fresh = self.web_call(opener, 'POST', '/api/auth/refresh', body, csrf=context['csrfToken'])
        self.check('token' not in fresh and 'refreshToken' not in fresh and fresh['refreshGeneration'] == '2', 'Expired access can refresh via opaque cookie and CSRF without disclosing secrets')
        self.web_call(opener, 'GET', '/api/me')
        self.check(self.web_call(opener, 'POST', '/api/auth/refresh', body, csrf=context['csrfToken'], expect=409)['error']['code'] == 'REFRESH_REAUTH_REQUIRED', 'Stale tab never borrows latest BFF refresh token')
        self.web_call(opener, 'GET', '/api/me', expect=401)
        self.check(self.sql("SELECT count(*) FROM auth.bff_sessions WHERE cookie_digest=decode('" + digest + "','hex') AND upstream_credentials_ciphertext IS NULL") == '1', 'Rejected stale browser generation clears sealed secrets')
        alice, shared_jar, _, alice_auth = self.web('shared_a')
        bob_body = {'username': 'shared_b_' + secrets.token_hex(5), 'password': 'Shared browser password ' + secrets.token_hex(16), 'requestId': str(uuid.uuid4()), 'deviceName': 'same-cookie-jar-second-tab'}
        bob_auth = self.web_call(alice, 'POST', '/api/auth/register', bob_body)
        alice.auth_family = None
        pending_csrf = self.web_call(alice, 'GET', '/api/auth/csrf')['csrfToken']
        self.web_call(alice, 'POST', '/api/auth/logout', {}, csrf=pending_csrf, expect=409)
        alice.auth_family = bob_auth['familyId']
        csrf = self.web_call(alice, 'GET', '/api/auth/csrf')['csrfToken']
        self.web_call(alice, 'POST', '/api/auth/confirm', {}, csrf=csrf)
        alice.auth_family = alice_auth['familyId']
        for method, path, payload in [('GET', '/api/me', None), ('POST', '/api/conversations', {'kind': 'self'}), ('POST', '/api/auth/logout', {})]:
            denied = self.web_call(alice, method, path, payload, csrf=csrf, expect=409)
            self.check(denied['error']['code'] == 'AUTH_CONTEXT_CHANGED', 'Old tab cannot borrow another login cookie for reads/writes/logout')
        self.web_call(alice, 'POST', '/api/auth/refresh', {'requestId': str(uuid.uuid4()), 'familyId': alice_auth['familyId'], 'refreshGeneration': '1'}, csrf=csrf, expect=409)
        alice.auth_family = bob_auth['familyId']
        self.check(self.web_call(alice, 'GET', '/api/me')['id'] == bob_auth['user']['id'], 'Rejected old-tab requests did not revoke new Bob family')
        self.check(len(list(shared_jar)) == 1, 'Both simulated tabs share one browser cookie jar, not isolated cookies')
        if self.environment.get('allowOwnedRestartForTests', True):
            before = self.web_call(alice, 'GET', '/api/auth/context')
            realm.stop_owned(self.environment)
            realm.start_owned(self.environment, Path(self.environment['caPath']).parent)
            after = self.web_call(alice, 'GET', '/api/auth/context')
            self.check(before['familyId'] == after['familyId'] and before['refreshGeneration'] == after['refreshGeneration'], 'BFF process restart preserves sealed credential generation without issuing refresh')

    def logout(self):
        super().logout()
        for opener in self.browsers:
            try:
                csrf = self.web_call(opener, 'GET', '/api/auth/csrf')['csrfToken']
                self.web_call(opener, 'POST', '/api/auth/logout', {}, csrf=csrf)
            except Exception:
                pass


def prepare(seed_file, go, public_port=32443, internal_port=33101):
    seed = json.loads(seed_file.read_text())
    assert seed.get('forwardTestRealm') is True
    for port in [public_port] + list(range(internal_port, internal_port + 8)):
        with socket.socket() as probe:
            probe.bind(('127.0.0.1', port))
    suffix = secrets.token_hex(6)
    run = Path(seed['runDir']).parents[1] / ('Refresh.' + suffix) / 'Safety'
    run.mkdir(parents=True, mode=0o700)
    old = run / 'jobsimd-legacy'
    shutil.copy2(Path(seed['binary']).with_name('jobsimd-forward-fixed'), old)
    business_name, journal_name = 'jobsim_refresh_' + suffix, 'jobsim_refresh_journal_' + suffix
    for original, name in [(seed['businessURL'], business_name), (seed['journalURL'], journal_name)]:
        realm.run_private([str(Path(seed['pgBin']) / 'createdb'), '--maintenance-db', original, name])
    info = {**seed, 'refreshTestRealm': True, 'runDir': str(run), 'stateDir': str(run / 'State'), 'serverURL': 'https://127.0.0.1:' + str(public_port), 'internalBasePort': internal_port, 'binary': str(old), 'businessURL': realm.database_url(seed['businessURL'], business_name), 'journalURL': realm.database_url(seed['journalURL'], journal_name)}
    info.pop('serverPID', None)
    before, boot = realm.start_owned(info, Path(seed['caPath']).parent)
    assert boot['features'].get('auth_refresh') is not True
    body = {'username': 'legacy_' + suffix, 'password': 'Legacy refresh fixture ' + secrets.token_hex(16), 'requestId': str(uuid.uuid4()), 'deviceName': 'actual-seven-day-legacy'}
    auth = before.call('POST', '/api/auth/register', body)
    realm.private_json(run / 'LegacyFixture.json', {'body': body, 'auth': auth})
    realm.stop_owned(info)
    binary = run / 'jobsimd-refresh'
    root = Path(__file__).resolve().parents[4]
    realm.run_private([go, '-C', str(root / '代码/后端/Go'), 'build', '-o', str(binary), './cmd/jobsimd'])
    info['binary'] = str(binary)
    _, boot = realm.start_owned(info, Path(seed['caPath']).parent)
    assert boot['features'].get('auth_refresh') is True
    print(json.dumps({'status': 'ready', 'serverURL': info['serverURL'], 'environmentFile': str(run / 'environment.json'), 'deploymentId': boot['deploymentId'], 'restoreEpoch': boot['restoreEpoch']}))
    return run / 'environment.json'


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--environment-file', type=Path)
    parser.add_argument('--prepare-from-environment', type=Path)
    parser.add_argument('--go', default=os.environ.get('GO_BINARY', 'go'))
    parser.add_argument('--skip-legacy-fixture', action='store_true', help='Fresh rebuilt namespace; historical old binary is unavailable and is not simulated')
    parser.add_argument('--report', type=Path, help='Write a separate report without replacing historical evidence')
    args = parser.parse_args()
    environment_file = prepare(args.prepare_from_environment, args.go) if args.prepare_from_environment else args.environment_file
    if environment_file is None:
        parser.error('Explicit existing refresh environment or isolated preparation source required')
    environment = json.loads(environment_file.read_text())
    endpoint = urllib.parse.urlsplit(environment['serverURL'])
    assert environment.get('refreshTestRealm') is True and endpoint.scheme == 'https' and endpoint.hostname == '127.0.0.1'
    suite = RefreshSuite(environment)
    try:
        legacy = None if args.skip_legacy_fixture else json.loads((environment_file.parent / 'LegacyFixture.json').read_text())
        report = suite.run(legacy)
        report['testedAtUTC'] = datetime.now(timezone.utc).isoformat()
        report['binarySHA256'] = hashlib.sha256(Path(environment['binary']).read_bytes()).hexdigest()
        report['scriptSHA256'] = hashlib.sha256(Path(__file__).read_bytes()).hexdigest()
        destination = args.report or Path(__file__).resolve().parents[4] / '项目相关报告/测试/后端/执行记录' / uuid.uuid4().hex / '登录刷新家族真实实测.json'
        data = (json.dumps(report, ensure_ascii=False, indent=2) + '\n').encode()
        destination.write_bytes(data)
        destination.with_suffix('.json.sha256').write_text(hashlib.sha256(data).hexdigest() + '  ' + destination.name + '\n')
        print(json.dumps(report, ensure_ascii=False))
    finally:
        suite.logout()


if __name__ == '__main__':
    main()
