#!/usr/bin/env python3
"""Jobs: bounded offline Android build/JVM verification; never install or replace Dist."""

import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import tempfile
import time
import uuid
import xml.etree.ElementTree as ET


ROOT = Path(__file__).resolve().parents[4]
ANDROID = ROOT / "代码/前端/Android"
MIN_FREE = 4 * 1024 ** 3
AUTH_SHA = "e295688485e0caa73932670e48da227eae804108416625b62fba5b0e01cee8b3"


def digest(file):
    """Hash the actual file bytes without interpreting source or credentials."""
    return hashlib.sha256(file.read_bytes()).hexdigest()


def inputs():
    """Collect owned compilation inputs and the existing Gradle wrapper metadata."""
    files = [file for file in (ANDROID / "app/src").rglob("*") if file.is_file()]
    tests = ROOT / "测试/前端/Android"
    files += list((tests / "Unit").glob("*.kt")) + list((tests / "Instrumentation").glob("*.kt"))
    files += [file for file in (tests / "InstrumentationAssets").rglob("*") if file.is_file()]
    files += [ANDROID / file for file in ("app/build.gradle.kts", "build.gradle.kts", "settings.gradle.kts", "gradle.properties", "gradlew", "gradle/wrapper/gradle-wrapper.jar", "gradle/wrapper/gradle-wrapper.properties")]
    files.append(Path(__file__).resolve())
    return sorted(set(files))


def stop_owned(process):
    """Stop only the process group created for this single no-daemon invocation."""
    try:
        os.killpg(process.pid, signal.SIGTERM)
    except ProcessLookupError:
        return
    deadline = time.monotonic() + 5
    while time.monotonic() < deadline:
        try:
            os.killpg(process.pid, 0)
        except ProcessLookupError:
            process.wait(timeout=1)
            return
        time.sleep(0.2)
    try:
        os.killpg(process.pid, signal.SIGKILL)
    except ProcessLookupError:
        pass
    process.wait(timeout=5)


def run_bounded(command, environment, report, max_seconds):
    """Enforce the time and 4 GiB storage gates while retaining the original log."""
    started = time.monotonic()
    samples = []
    reason = "completed"
    with (report / "Gradle原始日志.txt").open("wb") as log:
        process = subprocess.Popen(command, cwd=ANDROID, env=environment, stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
        try:
            while process.poll() is None:
                elapsed = round(time.monotonic() - started, 2)
                free = shutil.disk_usage(ROOT).free
                samples.append({"seconds": elapsed, "freeBytes": free})
                if free < MIN_FREE or elapsed >= max_seconds:
                    reason = "disk_below_4GiB" if free < MIN_FREE else "time_limit"
                    stop_owned(process)
                    break
                time.sleep(1)
        except BaseException:
            stop_owned(process)
            raise
    (report / "资源门禁.json").write_text(json.dumps({"minimumFreeBytes": MIN_FREE, "maxSeconds": max_seconds, "reason": reason, "samples": samples}, indent=2) + "\n")
    return process.returncode, reason


def main():
    """Prepare a new report, build once with fixed bounds, and record exact evidence."""
    parser = argparse.ArgumentParser(description="离线单worker编译APK/androidTest并执行4新＋6旧JVM；不安装、不覆盖Dist。")
    parser.add_argument("--execute", action="store_true", help="明确执行已授权构建；省略只显示自述")
    parser.add_argument("--max-seconds", type=int, default=600, choices=range(30, 601), metavar="30..600")
    parser.add_argument("--report-root", type=Path, help="显式本轮中央报告目录；相对路径以仓库根解析")
    options = parser.parse_args()
    print("JobsIM：复用现有JBR/SDK/Gradle；单worker、600秒以内、4GiB硬门禁；只写本轮报告与编译中间产物。", flush=True)
    if not options.execute:
        return 0
    report_root = options.report_root or Path("项目相关报告/测试/前端/Android/20261116-权威同步")
    report_root = report_root if report_root.is_absolute() else ROOT / report_root
    report = report_root / (datetime.now().strftime("Build-%Y%m%d-%H%M%S-") + str(os.getpid()) + "-" + uuid.uuid4().hex[:6])
    report.mkdir(parents=True)
    print("报告：" + str(report), flush=True)
    summary = {"startedAt": datetime.now(timezone.utc).isoformat(), "status": "failed", "instrumentationExecuted": False, "apiUsed": False}
    result = 1
    try:
        if shutil.disk_usage(ROOT).free < MIN_FREE:
            raise RuntimeError("DISK_BELOW_4_GIB")
        java = Path(os.environ.get("JOBSIM_JAVA_HOME", "/Applications/Android Studio.app/Contents/jbr/Contents/Home"))
        sdk = Path(os.environ.get("ANDROID_SDK_ROOT", str(Path.home() / "Library/Android/sdk")))
        if not (java / "bin/java").is_file() or not (sdk / "platforms/android-37.0/android.jar").is_file():
            raise RuntimeError("EXISTING_JDK_OR_SDK_MISSING")
        protected = [ANDROID / "Dist/JobsIM-debug.apk", ANDROID / "Dist/Baselines/20261115-native-auth-refresh-debug.apk"]
        before = {str(file.relative_to(ROOT)): digest(file) for file in protected}
        if set(before.values()) != {AUTH_SHA}:
            raise RuntimeError("AUTH_BASELINE_ALREADY_CHANGED")
        source_hashes = {}
        for file in inputs():
            relative = file.relative_to(ROOT)
            target = report / "输入源码" / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(file, target)
            source_hashes[str(relative)] = digest(file)
        runtime_root = Path.home() / "Library/Application Support/JobsIM/Build/Android"
        runtime_root.mkdir(parents=True, exist_ok=True)
        runtime = Path(tempfile.mkdtemp(prefix="sync-build-", dir=runtime_root))
        init = runtime / "limits.gradle"
        init.write_text("allprojects { tasks.withType(org.gradle.api.tasks.testing.Test).configureEach { maxHeapSize = '256m'; maxParallelForks = 1; jvmArgs '-XX:ActiveProcessorCount=2' } }\n")
        shutil.copy2(init, report / "limits.gradle")
        command = [str(ANDROID / "gradlew"), "--offline", "--no-daemon", "--max-workers=1", "--console=plain", "-Dorg.gradle.jvmargs=-Xmx1536m -XX:ActiveProcessorCount=2 -Dfile.encoding=UTF-8", "-Pkotlin.compiler.execution.strategy=in-process", "--init-script", str(init), ":app:assembleDebug", ":app:testDebugUnitTest", ":app:assembleDebugAndroidTest"]
        environment = dict(os.environ, JAVA_HOME=str(java), ANDROID_HOME=str(sdk), ANDROID_SDK_ROOT=str(sdk))
        environment["PATH"] = str(java / "bin") + os.pathsep + environment.get("PATH", "")
        (report / "构建参数.json").write_text(json.dumps({"command": command, "javaHome": str(java), "sdk": str(sdk), "initSHA256": digest(init)}, ensure_ascii=False, indent=2) + "\n")
        code, reason = run_bounded(command, environment, report, options.max_seconds)
        summary.update(exitCode=code, stopReason=reason, inputSHA256=source_hashes)
        summary["inputsUnchanged"] = all(digest(ROOT / file) == expected for file, expected in source_hashes.items())
        summary["authBaselineSHA256"] = {str(file.relative_to(ROOT)): digest(file) for file in protected}
        if summary["authBaselineSHA256"] != before:
            raise RuntimeError("AUTH_BASELINE_CHANGED_DURING_BUILD")
        if code == 0:
            suites = []
            for file in sorted((ANDROID / "app/build/test-results/testDebugUnitTest").glob("TEST-*.xml")):
                suite = ET.parse(file).getroot()
                suites.append({"name": suite.get("name"), **{key: int(suite.get(key, "0")) for key in ("tests", "failures", "errors", "skipped")}})
                target = report / "JVM" / file.name
                target.parent.mkdir(exist_ok=True)
                shutil.copy2(file, target)
            summary["jvmSuites"] = suites
            summary["artifacts"] = {}
            for name, file in [("app-debug.apk", ANDROID / "app/build/outputs/apk/debug/app-debug.apk"), ("app-debug-androidTest.apk", ANDROID / "app/build/outputs/apk/androidTest/debug/app-debug-androidTest.apk")]:
                shutil.copy2(file, report / name)
                summary["artifacts"][name] = {"sha256": digest(file), "bytes": file.stat().st_size}
            schema = ANDROID / "app/schemas/com.jobs.im.ChatDatabase/9.json"
            if schema.exists():
                shutil.copy2(schema, report / "Room-schema9.json")
            passed = len(suites) == 2 and sum(suite["tests"] for suite in suites) == 10 and not any(suite[key] for suite in suites for key in ("failures", "errors", "skipped"))
            if passed and summary["inputsUnchanged"]:
                summary["status"] = "passed"
                result = 0
    except Exception as error:
        summary["error"] = str(error)
    finally:
        summary["finishedAt"] = datetime.now(timezone.utc).isoformat()
        (report / "构建结果.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n")
        print(json.dumps({"status": summary["status"], "report": str(report), "exitCode": summary.get("exitCode"), "error": summary.get("error")}, ensure_ascii=False), flush=True)
    return result


if __name__ == "__main__":
    raise SystemExit(main())
