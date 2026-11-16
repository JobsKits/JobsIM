#!/usr/bin/env python3
"""Jobs: run only the nine isolated authoritative-sync methods on the owned AVD."""

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
import tempfile
import time
import uuid


ROOT = Path(__file__).resolve().parents[4]
AUTH_SHA = "e295688485e0caa73932670e48da227eae804108416625b62fba5b0e01cee8b3"
SERIAL = "emulator-5582"
AVD = "JobsIMReview_20261114_1424"
API_METHOD = "retainedApiObjectCannotAdoptANewFamilyWhenInvokedAfterSwitch"
FILTER = "com.jobs.im.AuthoritativeSyncTest,com.jobs.im.NativeSessionLifecycleTest#" + API_METHOD


def digest(file):
    """Hash the APK or source bytes that are actually used."""
    return hashlib.sha256(file.read_bytes()).hexdigest()


def stop_group(process):
    """Stop this wrapper's adb child group without touching the emulator process."""
    try:
        os.killpg(process.pid, signal.SIGTERM)
    except ProcessLookupError:
        return
    deadline = time.monotonic() + 3
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


def main():
    """Verify AVD ownership, install the fixed pair, then execute exactly nine methods."""
    parser = argparse.ArgumentParser(description="仅现有JobsIM专用5582的9项仪器，不运行HTTPS、不清库、不启停模拟器。")
    parser.add_argument("--build-report", type=Path, required=True)
    parser.add_argument("--previous-build-report", type=Path, help="重跑时显式验证此前本任务已安装中间包")
    parser.add_argument("--execute", action="store_true")
    options = parser.parse_args()
    print("JobsIM：只验证并复用5582；安装已封存自有两包、执行8同步＋1旧Api family；4GiB硬门禁、总360秒。", flush=True)
    if not options.execute:
        return 0
    build = options.build_report.resolve()
    report = ROOT / "项目相关报告/测试/前端/Android/20261116-权威同步" / (datetime.now().strftime("Instrument-%Y%m%d-%H%M%S-") + str(os.getpid()) + "-" + uuid.uuid4().hex[:6])
    report.mkdir(parents=True)
    shutil.copy2(Path(__file__), report / "sync_instrument.py")
    runtime_root = Path.home() / "Library/Application Support/JobsIM/Build/Android"
    runtime_root.mkdir(parents=True, exist_ok=True)
    runtime = Path(tempfile.mkdtemp(prefix="sync-instrument-", dir=runtime_root))
    adb = Path(os.environ.get("ANDROID_SDK_ROOT", str(Path.home() / "Library/Android/sdk"))) / "platform-tools/adb"
    summary = {"status": "failed", "startedAt": datetime.now(timezone.utc).isoformat(), "buildReport": str(build.relative_to(ROOT)), "serial": SERIAL, "avd": AVD, "filter": FILTER, "apiUsed": False, "fixtureIsolation": "Each method uses fresh UUID SQLite/profile/cache scope; no live server or account seed", "runnerSourceSHA256": digest(Path(__file__))}
    deadline = time.monotonic() + 360
    samples = []
    installed = False

    def run(arguments, name, seconds=30):
        """Save original output and enforce storage/time gates for each owned adb command."""
        if shutil.disk_usage(ROOT).free < 4 * 1024 ** 3:
            raise RuntimeError("DISK_BELOW_4_GIB")
        with (report / (name + ".txt")).open("wb") as log:
            process = subprocess.Popen([str(adb), "-s", SERIAL, *arguments], stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
            started = time.monotonic()
            try:
                while process.poll() is None:
                    free = shutil.disk_usage(ROOT).free
                    samples.append({"secondsRemaining": round(deadline-time.monotonic(), 2), "freeBytes": free})
                    if free < 4 * 1024 ** 3 or time.monotonic() >= min(deadline, started + seconds):
                        stop_group(process)
                        raise RuntimeError("DISK_OR_TIME_GATE")
                    time.sleep(0.5)
            except BaseException:
                stop_group(process)
                raise
        output = (report / (name + ".txt")).read_text(errors="replace")
        if process.returncode:
            raise RuntimeError("ADB_COMMAND_FAILED:" + name)
        return output

    def installed_apk(package, label):
        """Pull only the requested owned package and calculate its installed bytes."""
        paths = run(["shell", "pm", "path", package], label + "-path").strip().splitlines()
        if len(paths) != 1 or not paths[0].startswith("package:/data/app/") or not paths[0].endswith("/base.apk"):
            raise RuntimeError("OWNED_PACKAGE_PATH_INVALID:" + package)
        target = runtime / (label + ".apk")
        run(["pull", paths[0][8:], str(target)], label + "-pull")
        return digest(target)

    result = 1
    try:
        build_info = json.loads((build / "构建结果.json").read_text())
        if build_info.get("status") != "passed":
            raise RuntimeError("BUILD_IS_NOT_VERIFIED")
        for file, expected in build_info["inputSHA256"].items():
            if digest(ROOT / file) != expected:
                raise RuntimeError("COMPILED_INPUT_CHANGED:" + file)
        targets = {name: build / name for name in ("app-debug.apk", "app-debug-androidTest.apk")}
        for name, file in targets.items():
            if digest(file) != build_info["artifacts"][name]["sha256"]:
                raise RuntimeError("FIXED_APK_CHANGED:" + name)
        avd = run(["emu", "avd", "name"], "AVD归属").splitlines()[0].strip()
        if avd != AVD:
            raise RuntimeError("AVD_NOT_OWNED")
        commands = subprocess.run(["ps", "-axo", "pid=,comm="], capture_output=True, text=True, timeout=10, check=True).stdout
        matches = []
        for line in commands.splitlines():
            if "qemu-system-aarch64" not in line:
                continue
            pid = line.strip().split(None, 1)[0]
            arguments = subprocess.run(["ps", "-p", pid, "-o", "args="], capture_output=True, text=True, timeout=10, check=True).stdout
            if re.search(r"-avd\s+" + re.escape(AVD) + r"(?:\s|$)", arguments) and re.search(r"-port\s+5582(?:\s|$)", arguments):
                matches.append({"pid": int(pid), "command": arguments.strip()})
        if len(matches) != 1:
            raise RuntimeError("EMULATOR_PID_OWNERSHIP_UNCONFIRMED")
        summary["emulatorProcess"] = matches[0]
        previous = installed_apk("com.jobs.im", "安装前应用")
        allowed = {AUTH_SHA, build_info["artifacts"]["app-debug.apk"]["sha256"]}
        if options.previous_build_report:
            prior = options.previous_build_report.resolve()
            prior_info = json.loads((prior / "构建结果.json").read_text())
            prior_sha = prior_info.get("artifacts", {}).get("app-debug.apk", {}).get("sha256")
            if prior_info.get("status") != "passed" or digest(prior / "app-debug.apk") != prior_sha:
                raise RuntimeError("PREVIOUS_FIXED_BUILD_INVALID")
            allowed.add(prior_sha)
        if previous not in allowed:
            raise RuntimeError("CURRENT_APP_IS_NOT_FIXED_JOBS_IM_BASELINE")
        summary["previousInstalledAppSHA256"] = previous
        for name, file in targets.items():
            run(["install", "-r", "-t", str(file)], "安装-" + name, seconds=90)
        installed = True
        summary["installedSHA256"] = {"app-debug.apk": installed_apk("com.jobs.im", "安装后应用"), "app-debug-androidTest.apk": installed_apk("com.jobs.im.test", "安装后测试")}
        if any(summary["installedSHA256"][name] != build_info["artifacts"][name]["sha256"] for name in targets):
            raise RuntimeError("INSTALLED_APK_MISMATCH")
        output = run(["shell", "am", "instrument", "-w", "-r", "-e", "class", FILTER, "com.jobs.im.test/androidx.test.runner.AndroidJUnitRunner"], "仪器原始结果", seconds=180)
        context = {}; outcomes = []
        for line in output.splitlines():
            if line.startswith("INSTRUMENTATION_STATUS: ") and "=" in line:
                key, value = line[len("INSTRUMENTATION_STATUS: "):].split("=", 1); context[key] = value
            elif line.startswith("INSTRUMENTATION_STATUS_CODE:"):
                code = int(line.split(":", 1)[1])
                if code != 1 and "test" in context and "class" in context:
                    outcomes.append({"class": context["class"], "test": context["test"], "code": code})
                context = {}
        source = ROOT / "测试/前端/Android/Instrumentation/AuthoritativeSyncTest.kt"
        expected = {"com.jobs.im.AuthoritativeSyncTest#" + name for name in re.findall(r"@Test\s+fun (\w+)\(", source.read_text())}
        expected.add("com.jobs.im.NativeSessionLifecycleTest#" + API_METHOD)
        observed = {row["class"] + "#" + row["test"] for row in outcomes}
        summary["outcomes"] = outcomes
        summary["totals"] = {"tests": len(outcomes), "passed": sum(row["code"] == 0 for row in outcomes), "failedOrSkipped": sum(row["code"] != 0 for row in outcomes)}
        if len(expected) == len(outcomes) == 9 and observed == expected and all(row["code"] == 0 for row in outcomes) and "INSTRUMENTATION_FAILED" not in output:
            summary["status"] = "passed"; result = 0
    except BaseException as error:
        summary["error"] = str(error)
        if installed:
            for package in ["com.jobs.im.test", "com.jobs.im"]:
                subprocess.run([str(adb), "-s", SERIAL, "shell", "am", "force-stop", package], capture_output=True, timeout=10, check=False)
    finally:
        summary["finishedAt"] = datetime.now(timezone.utc).isoformat()
        (report / "资源门禁.json").write_text(json.dumps(samples, indent=2) + "\n")
        (report / "仪器结果.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n")
        print(json.dumps({"status": summary["status"], "report": str(report), "totals": summary.get("totals"), "error": summary.get("error")}, ensure_ascii=False), flush=True)
    return result


if __name__ == "__main__":
    raise SystemExit(main())
