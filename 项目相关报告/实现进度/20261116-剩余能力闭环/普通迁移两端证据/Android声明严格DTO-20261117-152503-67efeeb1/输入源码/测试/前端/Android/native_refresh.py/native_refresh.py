#!/usr/bin/env python3
"""Jobs: run the isolated native refresh scope with owned-family deadline fixtures."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import ssl
import subprocess
import time
import urllib.parse
import urllib.request
import uuid
import xml.etree.ElementTree as ET


ROOT = Path(__file__).resolve().parents[4]
ANDROID = ROOT / "代码/前端/Android"
UUID = re.compile(r"[0-9a-fA-F]{8}(?:-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12}\Z")


def private_json(path):
    if path.stat().st_mode & 0o077:
        raise ValueError("Private environment must have owner-only permissions")
    return json.loads(path.read_text())


def command(argv, **kwargs):
    result = subprocess.run(argv, capture_output=True, check=False, **kwargs)
    if result.returncode:
        raise RuntimeError("Required subprocess failed; output redacted")
    return result.stdout


def bootstrap(environment):
    address = urllib.parse.urlsplit(environment["serverURL"])
    if address.scheme != "https" or address.username or address.password or address.query or address.fragment:
        raise ValueError("A clean HTTPS origin is required")
    trust = ssl.create_default_context(cafile=environment["caPath"])
    with urllib.request.urlopen(environment["serverURL"] + "/api/bootstrap", context=trust, timeout=10) as response:
        result = json.load(response)
    if not all(result.get("features", {}).get(flag) is True for flag in ("auth_refresh", "auth_retirement_chain")):
        raise ValueError("Fresh refresh and retirement-chain capabilities are required")
    return result, address.port or 443


def sql_environment(environment):
    url = urllib.parse.urlsplit(environment["businessURL"])
    if url.scheme not in ("postgres", "postgresql"):
        raise ValueError("Explicit private PostgreSQL fixture is required")
    child = dict(os.environ)
    child.update(PGHOST=url.hostname or "localhost", PGPORT=str(url.port or 5432),
                 PGUSER=urllib.parse.unquote(url.username or ""), PGDATABASE=urllib.parse.unquote(url.path.lstrip("/")))
    if environment.get("pgPassFile"):
        child["PGPASSFILE"] = environment["pgPassFile"]
    if url.password:
        child["PGPASSWORD"] = urllib.parse.unquote(url.password)
    return child


def apply_clock(environment, request, boot):
    if request.get("deploymentId") != boot["deploymentId"] or request.get("restoreEpoch") != boot["restoreEpoch"]:
        raise ValueError("Clock fixture belongs to another deployment")
    if not all(UUID.fullmatch(request.get(key, "")) for key in ("requestId", "familyId", "userId")):
        raise ValueError("Clock fixture identifiers are invalid")
    epoch = request.get("restoreEpoch", "")
    if not isinstance(epoch, str) or not re.fullmatch(r"[1-9][0-9]{0,18}", epoch):
        raise ValueError("Original exact restore epoch is required")
    generation = request.get("refreshGeneration", "")
    if not isinstance(generation, str) or not re.fullmatch(r"0|[1-9][0-9]{0,19}", generation):
        raise ValueError("Original exact refresh generation is required")
    # Validated identifiers only; secrets stay in the child's environment, never argv or logs.
    sql = ("UPDATE auth.sessions SET access_expires_at=now()-interval '1 second' "
           "WHERE family_id='" + request["familyId"] + "' AND user_id='" + request["userId"] + "' "
           "AND refresh_generation=" + generation + " AND restore_epoch=" + epoch +
           " AND source_context_id='personal' AND revoked_at IS NULL RETURNING 1;")
    result = command([str(Path(environment["pgBin"]) / "psql"), "-XAtq", "-v", "ON_ERROR_STOP=1"],
                     input=sql.encode(), env=sql_environment(environment)).decode().strip()
    if result != "1":
        raise ValueError("Owned active original-generation family was not uniquely matched")


def copy_results(report, process_code, clock_count, expected_tests):
    xml_root = ANDROID / "app/build/outputs/androidTest-results/connected/debug"
    target = report / "仪器结果"
    target.mkdir()
    totals = dict(tests=0, failures=0, errors=0, skipped=0)
    for file in xml_root.glob("TEST-*.xml"):
        shutil.copy2(file, target / file.name)
        suite = ET.parse(file).getroot()
        for key in totals:
            totals[key] += int(suite.get(key, "0"))
    unit = ANDROID / "app/build/test-results/testDebugUnitTest"
    shutil.copytree(unit, report / "JVM", ignore=shutil.ignore_patterns("binary"))
    apk = ANDROID / "app/build/outputs/apk/debug/app-debug.apk"
    apk_hash = hashlib.sha256(apk.read_bytes()).hexdigest()
    inputs = [p for p in (ANDROID / "app/src").rglob("*") if p.is_file()]
    tests = ROOT / "测试/前端/Android"
    inputs += list((tests / "Instrumentation").glob("NativeSession*.kt"))
    inputs += list((tests / "Unit").glob("*.kt"))
    inputs += [tests / "Instrumentation/StorageLifecycleTest.kt", tests / "Instrumentation/NativeForwardActions.kt"]
    inputs += list((ANDROID / "app/schemas").rglob("8.json"))
    inputs += [ANDROID / p for p in ("app/build.gradle.kts", "build.gradle.kts", "settings.gradle.kts", "gradle.properties", "gradle/wrapper/gradle-wrapper.properties", "gradle/wrapper/gradle-wrapper.jar", "gradlew")]
    inputs += [Path(__file__).resolve()]
    summary = dict(status="passed" if process_code == 0 and totals["tests"] == expected_tests and not any(totals[k] for k in ("failures", "errors", "skipped")) and clock_count == 2 else "failed",
                   instrumentation=totals, clockFixtures=clock_count, clockBoundary="Only owned test access deadlines; no fifteen-minute wall-clock claim",
                   apkSHA256=apk_hash, apkBytes=apk.stat().st_size,
                   inputSHA256={str(p.relative_to(ROOT)): hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(set(inputs))})
    (report / "验证摘要.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n")
    return summary


def run(args, environment, boot, port):
    sdk = Path(os.environ.get("ANDROID_HOME", os.environ.get("ANDROID_SDK_ROOT", str(Path.home() / "Library/Android/sdk"))))
    adb = sdk / "platform-tools/adb"
    if not adb.is_file() or not (Path(environment["pgBin"]) / "psql").is_file():
        raise ValueError("Existing adb and PostgreSQL client are required; no installation is performed")
    devices = command([str(adb), "devices"], text=True)
    if not any(line.split()[:2] == [args.serial, "device"] for line in devices.splitlines()):
        raise ValueError("Explicit target is not connected and authorized")
    command([str(adb), "-s", args.serial, "reverse", "tcp:" + str(port), "tcp:" + str(port)])
    report = args.report_dir.resolve()
    report.mkdir(parents=True, exist_ok=False)
    classes = "com.jobs.im.NativeSessionHttpsTest" if args.https_only else "com.jobs.im.NativeSessionLifecycleTest,com.jobs.im.StorageLifecycleTest#versionTwoUpgradePreservesOriginalPendingIdentity,com.jobs.im.NativeSessionHttpsTest"
    import base64
    run_id = str(uuid.uuid4())
    argv = [str(ANDROID / "gradlew"), "-p", str(ANDROID), "--offline", "--no-daemon", ":app:testDebugUnitTest", ":app:connectedDebugAndroidTest",
            "-Pandroid.testInstrumentationRunnerArguments.class=" + classes,
            "-Pandroid.testInstrumentationRunnerArguments.nativeRefresh=true",
            "-Pandroid.testInstrumentationRunnerArguments.nativeRefreshRun=" + run_id,
            "-Pandroid.testInstrumentationRunnerArguments.liveUrl=" + environment["serverURL"],
            "-Pandroid.testInstrumentationRunnerArguments.deploymentId=" + boot["deploymentId"],
            "-Pandroid.testInstrumentationRunnerArguments.caBase64=" + base64.b64encode(Path(environment["caPath"]).read_bytes()).decode(),
            "-Pandroid.injected.androidTest.leaveApksInstalledAfterRun=true"]
    child_env = dict(os.environ, ANDROID_SERIAL=args.serial)
    seen = set()
    with (report / "构建仪器日志.txt").open("wb") as log:
        process = subprocess.Popen(argv, env=child_env, stdout=log, stderr=subprocess.STDOUT)
        try:
            while process.poll() is None:
                try:
                    raw = subprocess.run([str(adb), "-s", args.serial, "exec-out", "run-as", "com.jobs.im", "cat", "files/native-auth-clock.json"], capture_output=True, timeout=10)
                except subprocess.TimeoutExpired:
                    time.sleep(0.5)
                    continue
                if raw.returncode == 0 and raw.stdout and len(raw.stdout) < 2048:
                    try:
                        request = json.loads(raw.stdout)
                    except (ValueError, UnicodeDecodeError):
                        request = None
                    if not isinstance(request, dict) or request.get("runId") != run_id:
                        time.sleep(0.25)
                        continue
                    request_id = request.get("requestId")
                    if request_id not in seen:
                        if len(seen) >= 2:
                            raise ValueError("Unexpected extra clock request")
                        apply_clock(environment, request, boot)
                        path = "files/native-auth-clock-" + request_id + ".done"
                        command([str(adb), "-s", args.serial, "shell", "run-as com.jobs.im sh -c 'cat > " + path + ".new && mv " + path + ".new " + path + "'"], input=request_id.encode())
                        seen.add(request_id)
                        print("已应用第" + str(len(seen)) + "次本人测试家族期限夹具", flush=True)
                time.sleep(0.5)
        finally:
            if process.poll() is None:
                process.terminate()
                process.wait(timeout=30)
    summary = copy_results(report, process.returncode, len(seen), 3 if args.https_only else 10)
    screenshots = report / "原生UI"
    screenshots.mkdir()
    for name in ("Auth-01-Unknown-Light.png", "Auth-02-Unknown-Dark.png", "Auth-02b-Unknown-Actions-Dark.png", "Auth-03-Verified-Dark.png", "Auth-04-Restored-Light.png"):
        source = "/sdcard/Android/data/com.jobs.im/files/ForwardUiEvidence/" + name
        result = subprocess.run([str(adb), "-s", args.serial, "pull", source, str(screenshots / name)], capture_output=True)
        if result.returncode and summary["status"] == "passed":
            raise RuntimeError("Expected native screenshot is missing")
    print("结果=" + summary["status"] + "；报告=" + str(report), flush=True)
    return 0 if summary["status"] == "passed" else 1


def main():
    parser = argparse.ArgumentParser(description="仅指定隔离模拟器：编译/安装测试包、创建测试账号、两次本人access期限SQL夹具；不安装SDK、不停止服务、不覆盖冻结Dist")
    parser.add_argument("--environment-file", type=Path, required=True)
    parser.add_argument("--deployment-id", required=True)
    parser.add_argument("--serial", required=True)
    parser.add_argument("--report-dir", type=Path, required=True)
    parser.add_argument("--https-only", action="store_true", help="只跑三条HTTPS定向；仅在同产品APK已有本机证据时使用")
    parser.add_argument("--execute", action="store_true", help="明确执行上述定向测试；默认仅只读预检")
    args = parser.parse_args()
    environment = private_json(args.environment_file)
    boot, port = bootstrap(environment)
    if boot["deploymentId"] != args.deployment_id:
        raise ValueError("Deployment identity mismatch")
    print("Android刷新定向：只用指定设备与本人测试家族；不操作旧缓存/服务/冻结APK。", flush=True)
    return run(args, environment, boot, port) if args.execute else 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as error:
        print("测试停止：" + type(error).__name__ + "；敏感子进程输出不回显。", flush=True)
        raise SystemExit(1)
