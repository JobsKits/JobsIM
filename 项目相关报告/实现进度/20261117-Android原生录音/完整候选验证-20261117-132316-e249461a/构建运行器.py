#!/usr/bin/env python3
"""Jobs: freeze the current Android candidate and verify offline in a new private tree."""

import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import signal
import subprocess
import time
import uuid
import xml.etree.ElementTree as ET


MIN_FREE = 4 * 1024 ** 3
SKIP = {"build", ".gradle", "Dist", ".git", "node_modules", ".dart_tool", "DerivedData", "__pycache__"}


def digest(file):
    """Hash exact source or artifact bytes."""
    return hashlib.sha256(file.read_bytes()).hexdigest()


def write_json(file, value):
    """Preserve this run's structured evidence without changing prior records."""
    file.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n")


def collect(root):
    """Select the whole owned project and all current external tests, excluding runtime outputs."""
    files = []
    for base in [root / "代码/前端/Android", root / "测试/前端/Android"]:
        for file in base.rglob("*"):
            if file.is_file() and not any(part in SKIP for part in file.relative_to(base).parts) and file.name not in {"local.properties", ".DS_Store"}:
                if file.is_symlink():
                    raise RuntimeError("UNEXPECTED_INPUT_SYMLINK: " + str(file))
                files.append(file)
    return sorted(set(files))


def methods(root, kind):
    """Record every current JUnit method directly from the frozen external test sources."""
    result = {}
    for file in sorted((root / "测试/前端/Android" / kind).glob("*.kt")):
        source = file.read_text()
        names = re.findall(r"@Test\s+fun\s+(\w+)\s*\(", source)
        if len(names) != len(re.findall(r"@Test\b", source)):
            raise RuntimeError("UNRECOGNIZED_TEST_DECLARATION: " + str(file))
        if names:
            result["com.jobs.im." + file.stem] = sorted(names)
    return result


def stop_owned(process):
    """Stop only the process group created for this run, preserving unrelated tools and caches."""
    try:
        os.killpg(process.pid, signal.SIGTERM)
    except ProcessLookupError:
        return
    deadline = time.monotonic() + 5
    while time.monotonic() < deadline:
        if process.poll() is not None:
            return
        time.sleep(0.2)
    try:
        os.killpg(process.pid, signal.SIGKILL)
    except ProcessLookupError:
        pass
    process.wait(timeout=5)


def main():
    """Freeze fresh inputs, run one bounded Gradle build, then bind results and APKs to that input."""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", type=Path, required=True)
    parser.add_argument("--execute", action="store_true")
    options = parser.parse_args()
    print("JobsIM: fresh private Android/JVM/APK verification; offline, one worker, no devices or cache cleanup.", flush=True)
    if not options.execute:
        return 0
    root = options.root.resolve(strict=True)
    run_id = datetime.now().strftime("%Y%m%d-%H%M%S-") + uuid.uuid4().hex[:8]
    report = root / "项目相关报告/实现进度/20261117-Android原生录音" / ("完整候选验证-" + run_id)
    work = Path.home() / "Library/Application Support/JobsIM/Build/Android/VoiceUpload" / run_id
    report.mkdir(parents=True)
    work.mkdir(parents=True)
    shutil.copy2(Path(__file__), report / "构建运行器.py")
    started = time.monotonic()
    summary = {"startedAt": datetime.now(timezone.utc).isoformat(), "status": "failed", "report": str(report), "work": str(work), "instrumentationExecuted": False, "devicesStarted": False, "apiUsed": False, "cacheDeleted": False}
    process = None
    samples = []
    hashes = {}
    reason = "not_started"
    try:
        free = shutil.disk_usage(root).free
        if free < MIN_FREE:
            raise RuntimeError("DISK_BELOW_4_GIB")
        java = Path("/opt/homebrew/opt/openjdk@21/libexec/openjdk.jdk/Contents/Home")
        sdk = Path.home() / "Library/Android/sdk"
        candidates = sorted((Path.home() / ".gradle/wrapper/dists/gradle-9.5.0-bin").glob("*/gradle-9.5.0/bin/gradle"))
        if not (java / "bin/java").is_file() or not (sdk / "platforms/android-37.0/android.jar").is_file() or len(candidates) != 1:
            raise RuntimeError("WARM_TOOLCHAIN_MISSING")
        protected = {str(file.relative_to(root)): digest(file) for file in (root / "代码/前端/Android/Dist").rglob("*.apk")}
        summary["protectedOriginalApkSHA256Before"] = protected
        files = collect(root)
        for file in files:
            relative = file.relative_to(root)
            frozen = report / "输入源码" / relative
            target = work / relative
            frozen.parent.mkdir(parents=True, exist_ok=True)
            target.parent.mkdir(parents=True, exist_ok=True)
            before = digest(file)
            shutil.copy2(file, frozen)
            if digest(file) != before or digest(frozen) != before:
                raise RuntimeError("INPUT_CHANGED_DURING_FREEZE: " + str(relative))
            shutil.copy2(frozen, target)
            hashes[str(relative)] = before
        if any(digest(root / name) != sha for name, sha in hashes.items()):
            raise RuntimeError("LIVE_INPUT_CHANGED_BEFORE_FREEZE_COMPLETE")
        write_json(report / "输入SHA256.json", hashes)
        expected = methods(work, "Unit")
        instrument = methods(work, "Instrumentation")
        summary.update(inputCount=len(hashes), expectedJvmMethods=expected, compiledOnlyInstrumentationMethods=instrument,
                       expectedJvmMethodCount=sum(map(len, expected.values())), compiledOnlyInstrumentationMethodCount=sum(map(len, instrument.values())))
        init = work / "limits.gradle"
        init.write_text("allprojects { tasks.withType(org.gradle.api.tasks.testing.Test).configureEach { maxHeapSize = '256m'; maxParallelForks = 1; jvmArgs '-XX:ActiveProcessorCount=2' } }\n")
        shutil.copy2(init, report / "limits.gradle")
        android = work / "代码/前端/Android"
        command = [str(candidates[0]), "--offline", "--no-daemon", "--no-build-cache", "--max-workers=1", "--console=plain",
                   "-Dorg.gradle.jvmargs=-Xmx2g -XX:ActiveProcessorCount=2 -Dfile.encoding=UTF-8",
                   "-Pkotlin.compiler.execution.strategy=in-process", "-Pandroid.builder.sdkDownload=false", "--init-script", str(init),
                   ":app:testDebugUnitTest", ":app:assembleDebug", ":app:assembleDebugAndroidTest"]
        environment = dict(os.environ, JAVA_HOME=str(java), ANDROID_HOME=str(sdk), ANDROID_SDK_ROOT=str(sdk), GRADLE_USER_HOME=str(Path.home() / ".gradle"))
        environment["PATH"] = str(java / "bin") + os.pathsep + environment.get("PATH", "")
        write_json(report / "构建参数.json", {"command": command, "cwd": str(android), "javaHome": str(java), "sdk": str(sdk), "minimumFreeBytes": MIN_FREE, "maxSeconds": 900, "initialFreeBytes": free, "javaRelease": (java / "release").read_text(), "initSHA256": digest(init)})
        print(json.dumps({"candidate": run_id, "report": str(report), "work": str(work), "inputs": len(hashes), "jvmMethods": summary["expectedJvmMethodCount"], "instrumentMethodsCompileOnly": summary["compiledOnlyInstrumentationMethodCount"], "freeBytes": free}, ensure_ascii=False), flush=True)
        launch = time.monotonic()
        reason = "completed"
        with (report / "Gradle.log").open("wb") as log:
            process = subprocess.Popen(command, cwd=android, env=environment, stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
            try:
                while process.poll() is None:
                    elapsed = time.monotonic() - launch
                    free = shutil.disk_usage(root).free
                    samples.append({"seconds": round(elapsed, 3), "freeBytes": free})
                    if free < MIN_FREE or elapsed >= 900:
                        reason = "disk_below_4GiB" if free < MIN_FREE else "time_limit"
                        stop_owned(process)
                        break
                    time.sleep(1)
            except BaseException:
                reason = "runner_interrupted"
                stop_owned(process)
                raise
        summary.update(exitCode=process.returncode, buildSeconds=round(time.monotonic() - launch, 3))
        suites = []
        actual = {}
        for file in sorted((android / "app/build/test-results/testDebugUnitTest").glob("TEST-*.xml")):
            suite = ET.parse(file).getroot()
            actual[suite.get("name")] = sorted(case.get("name") for case in suite.findall("testcase"))
            suites.append({"name": suite.get("name"), **{key: int(suite.get(key, "0")) for key in ("tests", "failures", "errors", "skipped")}})
            target = report / "JVM" / file.name
            target.parent.mkdir(exist_ok=True)
            shutil.copy2(file, target)
        summary["jvmSuites"] = suites
        summary["actualJvmMethods"] = actual
        summary["totals"] = {key: sum(suite[key] for suite in suites) for key in ("tests", "failures", "errors", "skipped")}
        artifacts = {}
        for name, relative in [("app-debug.apk", "app/build/outputs/apk/debug/app-debug.apk"), ("app-debug-androidTest.apk", "app/build/outputs/apk/androidTest/debug/app-debug-androidTest.apk")]:
            file = android / relative
            if file.is_file():
                shutil.copy2(file, report / name)
                artifacts[name] = {"sha256": digest(file), "bytes": file.stat().st_size, "freshPrivateOutput": True}
        summary["artifacts"] = artifacts
        schema = android / "app/schemas/com.jobs.im.ChatDatabase/9.json"
        if schema.is_file():
            shutil.copy2(schema, report / "Room-schema9.json")
        summary["frozenInputUnchanged"] = all(digest(work / name) == sha for name, sha in hashes.items())
        summary["currentSourceDifferences"] = [{"path": name, "frozenSHA256": sha, "currentSHA256": digest(root / name)} for name, sha in hashes.items() if digest(root / name) != sha]
        protected_after = {name: digest(root / name) for name in protected}
        summary["protectedOriginalApkSHA256After"] = protected_after
        passed = process.returncode == 0 and reason == "completed" and actual == expected and summary["totals"]["tests"] == summary["expectedJvmMethodCount"] and not any(summary["totals"][key] for key in ("failures", "errors", "skipped")) and len(artifacts) == 2 and summary["frozenInputUnchanged"] and protected_after == protected
        if passed:
            summary["status"] = "passed_frozen_candidate" if summary["currentSourceDifferences"] else "passed"
    except Exception as failure:
        summary["error"] = str(failure)
    finally:
        if process is not None and process.poll() is None:
            stop_owned(process)
        write_json(report / "资源门禁.json", {"minimumFreeBytes": MIN_FREE, "maxSeconds": 900, "stopReason": reason, "samples": samples})
        summary.update(stopReason=reason, finishedAt=datetime.now(timezone.utc).isoformat(), seconds=round(time.monotonic() - started, 3), minimumFreeBytes=min((sample["freeBytes"] for sample in samples), default=shutil.disk_usage(root).free), diskSamples=len(samples))
        write_json(report / "结果.json", summary)
        print(json.dumps({"status": summary["status"], "report": str(report), "exitCode": summary.get("exitCode"), "error": summary.get("error"), "totals": summary.get("totals"), "artifacts": summary.get("artifacts"), "minimumFreeBytes": summary["minimumFreeBytes"], "inputDifferences": summary.get("currentSourceDifferences")}, ensure_ascii=False), flush=True)
    return 0 if summary["status"].startswith("passed") else 1


if __name__ == "__main__":
    raise SystemExit(main())
