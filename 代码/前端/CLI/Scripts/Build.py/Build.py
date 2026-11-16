#!/usr/bin/env python3
"""Build the pure-Go CLI and its separately signed macOS Keychain helper."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import platform
import plistlib
import subprocess
import sys


def run(arguments, directory=None, environment=None):
    result = subprocess.run(arguments, cwd=directory, env=environment, capture_output=True, text=True)
    if result.returncode:
        raise RuntimeError('Build or signature check failed: ' + (result.stderr or result.stdout)[-3000:])
    return result.stdout.strip()


def build(args):
    project = Path(__file__).resolve().parents[2]
    output = args.output.expanduser().resolve()
    if output.exists() and any(output.iterdir()):
        raise RuntimeError('Use a new empty bundle directory; an existing successful package is never overwritten')
    output.mkdir(parents=True, exist_ok=True, mode=0o700)
    target = args.os or {'Darwin': 'darwin', 'Linux': 'linux', 'Windows': 'windows'}[platform.system()]
    architecture = args.arch or ('arm64' if platform.machine().lower() in ('arm64', 'aarch64') else 'amd64')
    binary = output / ('jobsim.exe' if target == 'windows' else 'jobsim')
    environment = dict(os.environ, CGO_ENABLED='0', GOMAXPROCS='2', GOOS=target, GOARCH=architecture)
    run([args.go, 'build', '-p=1', '-mod=readonly', '-trimpath', '-o', str(binary), './cmd/jobsim'], project, environment)
    assets = [binary]
    if target == 'darwin':
        if platform.system() != 'Darwin':
            raise RuntimeError('macOS packaging requires its native Swift and signing tools')
        if not args.local_adhoc and not args.sign_identity:
            raise RuntimeError('Explicit authorized signing identity or --local-adhoc required')
        helpers = output / 'Helpers'
        helpers.mkdir(mode=0o700, exist_ok=True)
        helper = helpers / 'JobsIMKeychain'
        swiftc = run(['/usr/bin/xcrun', '--find', 'swiftc'])
        sdk = run(['/usr/bin/xcrun', '--sdk', 'macosx', '--show-sdk-path'])
        apple_architecture = 'x86_64' if architecture == 'amd64' else 'arm64'
        run([swiftc, '-parse-as-library', '-O', '-j', '1', '-sdk', sdk, '-target', apple_architecture + '-apple-macosx13.0', '-framework', 'Foundation', '-framework', 'Security', str(project / '原生助手/Keychain/JobsIMKeychain.swift'), '-o', str(helper)])
        application = args.application_id
        if not application:
            raise RuntimeError('Explicit application identifier required; do not invent a signing Team ID')
        entitlements = helpers / 'Keychain.entitlements'
        entitlements.write_bytes(plistlib.dumps({'com.apple.application-identifier': application, 'keychain-access-groups': [application]}))
        os.chmod(entitlements, 0o600)
        run(['/usr/bin/codesign', '--force', '--sign', '-' if args.local_adhoc else args.sign_identity, '--identifier', application, '--entitlements', str(entitlements), str(helper)])
        run(['/usr/bin/codesign', '--verify', '--strict', str(helper)])
        os.chmod(helper, 0o700)
        (helpers / 'SHA256SUMS').write_text(hashlib.sha256(helper.read_bytes()).hexdigest() + '  JobsIMKeychain\n')
        os.chmod(helpers / 'SHA256SUMS', 0o600)
        assets += [helper, helpers / 'SHA256SUMS', entitlements]
    receipt = {'status': 'built', 'target': target + '/' + architecture, 'cgoEnabled': False, 'localAdhocOnly': bool(args.local_adhoc), 'nativeCredentialRuntimeVerified': False, 'assets': {p.name: hashlib.sha256(p.read_bytes()).hexdigest() for p in assets}}
    (output / 'BuildReceipt.json').write_text(json.dumps(receipt, indent=2) + '\n')
    print(json.dumps(receipt))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--go', default=os.environ.get('GO_BINARY', 'go'))
    parser.add_argument('--output', type=Path, required=True, help='Explicit bundle directory; existing user data is not touched')
    parser.add_argument('--os', choices=['darwin', 'linux', 'windows'])
    parser.add_argument('--arch', choices=['arm64', 'amd64'])
    parser.add_argument('--sign-identity', help='Existing authorized macOS signing identity; never a private key')
    parser.add_argument('--application-id', help='Actual entitled identifier, or explicit independent local diagnostic identifier')
    parser.add_argument('--local-adhoc', action='store_true', help='Local diagnostic signature only; not a distributable signing assertion')
    args = parser.parse_args()
    try:
        build(args)
    except (OSError, RuntimeError) as error:
        print(str(error), file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
