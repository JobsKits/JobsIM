#!/usr/bin/env python3
"""仅在临时目录验证实例搬迁门禁，不连接或修改真实运行实例。"""
import argparse
import hashlib
import json
import pathlib
import subprocess
import tempfile

ROOT = pathlib.Path(__file__).resolve().parents[4]
SCRIPTS = [
    ROOT / '部署/MacOS/Deployment/【MacOS】部署JobsIM.command/【MacOS】部署JobsIM.command',
    ROOT / '部署/MacOS/Undeployment/【MacOS】反部署JobsIM.command/【MacOS】反部署JobsIM.command',
    ROOT / '部署/Ubuntu/Deployment/【Linux】部署JobsIM.sh/【Linux】部署JobsIM.sh',
    ROOT / '部署/Ubuntu/Undeployment/【Linux】反部署JobsIM.sh/【Linux】反部署JobsIM.sh',
]
PROBE = '''set -eu
umask 077
RUN_ACTIVE=0; LOCK_HELD=0; CLEANING=1
source "$1"
REPO="$2/repo"; STATE="$2/state"; DEFAULT_INSTANCE_STATE="$STATE"
mkdir -p "$REPO/.jobsim-local" "$STATE/Application"
REF_DEPLOYMENT="11111111-1111-4111-8111-111111111111"
printf '{"deploymentId":"%s","key":"synthetic-private-stays-local"}\\n' "$REF_DEPLOYMENT" > "$STATE/Application/instance.json"
printf '%s\\n%s\\n' "$2/moved-away" "$(id -u)" > "$STATE/owner"
printf 'JobsIM-instance-ref-v1\\na4c6d9d2688afce6\\n%s\\n%s\\n' "$REF_DEPLOYMENT" "$(id -u)" > "$REPO/.jobsim-local/instance.ref"
read_instance_reference
before="$(shasum -a 256 "$STATE/owner")"
assert_owner
[[ "$before" == "$(shasum -a 256 "$STATE/owner")" && "$RELOCATE_OWNER" == 1 ]]
if apply_owner_relocation; then exit 22; fi
LOCK_HELD=1
apply_owner_relocation
[[ "$(sed -n '1p' "$STATE/owner")" == "$REPO" ]]
[[ "$(sed -n '1p' "$STATE/Application/instance.json")" == *synthetic-private-stays-local* ]]
assert_owner
original_state="$STATE"
STATE="$2/missing-instance"; DEFAULT_INSTANCE_STATE="$STATE"
if assert_owner; then exit 26; fi
[[ ! -e "$STATE" ]]
STATE="$original_state"; DEFAULT_INSTANCE_STATE="$STATE"
printf '{"deploymentId":"22222222-2222-4222-8222-222222222222"}\\n' > "$STATE/Application/instance.json"
if assert_owner; then exit 27; fi
printf '{"deploymentId":"%s","key":"synthetic-private-stays-local"}\\n' "$REF_DEPLOYMENT" > "$STATE/Application/instance.json"
printf '%s\\n%s\\n' "$2/other" "$(id -u)" > "$STATE/owner"
mkdir -p "$2/other"
if assert_owner; then exit 23; fi
rmdir "$2/other"
printf '{"deploymentId":"22222222-2222-4222-8222-222222222222"}\\n' > "$STATE/Application/instance.json"
if assert_owner; then exit 24; fi
chmod 644 "$REPO/.jobsim-local/instance.ref"
if read_instance_reference; then exit 25; fi
print 'PASS: readonly; lock; original identity; absent-old-root; deployment; private-reference'
'''


def hashes():
    return {str(p.relative_to(ROOT)): hashlib.sha256(p.read_bytes()).hexdigest() for p in SCRIPTS}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=pathlib.Path, default=ROOT / '项目相关报告/实现进度/20261116-剩余能力闭环/工作树搬迁/第二轮Shell')
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=False)
    before = hashes()
    results = []
    with tempfile.TemporaryDirectory(prefix='jobsim-relocation-') as directory:
        base = pathlib.Path(directory)
        for index, path in enumerate(SCRIPTS):
            syntax = subprocess.run(['zsh' if 'MacOS' in path.parts else 'bash', '-n', str(path)], capture_output=True, text=True)
            results.append({'path': str(path.relative_to(ROOT)), 'syntaxExit': syntax.returncode})
            if syntax.returncode:
                raise RuntimeError('生命周期入口语法失败')
            if 'MacOS' not in path.parts:
                continue
            case = base / str(index)
            case.mkdir()
            library = case / 'functions.zsh'
            library.write_text(path.read_text().replace('main "$@"', ''))
            probe = case / 'probe.zsh'
            probe.write_text(PROBE)
            run = subprocess.run(['zsh', str(probe), str(library), str(case)], capture_output=True, text=True)
            (args.output / (str(index) + '.log')).write_text(run.stdout + run.stderr)
            results[-1]['nativeExit'] = run.returncode
            if run.returncode:
                raise RuntimeError('搬迁临时目录反例失败')
    after = hashes()
    report = {'status': 'passed' if before == after else 'changed-inputs', 'nativeMacOSChecks': 16,
              'linuxNativeExecuted': False, 'usesRealInstance': False, 'inputs': before, 'after': after, 'results': results}
    (args.output / '结果.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps({'status': report['status'], 'nativeMacOSChecks': 16, 'syntaxScripts': 4}, ensure_ascii=False))
    if before != after:
        raise RuntimeError('验证期间源码改变')


if __name__ == '__main__':
    main()
