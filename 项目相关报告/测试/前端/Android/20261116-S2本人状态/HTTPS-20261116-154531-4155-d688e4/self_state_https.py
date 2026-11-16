#!/usr/bin/env python3
"""Jobs: three new-instance native HTTPS/UI cases; no secret seed data in reports."""

import argparse
import base64
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
from urllib.parse import urlparse
import uuid


ROOT = Path(__file__).resolve().parents[4]
SERIAL = "emulator-5582"
AVD = "JobsIMReview_20261114_1424"
PRODUCT_SHA = "a135ef54037e571f8903a0acc1821fe9e8aad1088212631872000a01d513df59"
MIN_FREE = 4 * 1024 ** 3
SUITE = "com.jobs.im.NativeSelfStateHttpsTest"


def digest(path):
    """Hash exact public input/artifact bytes."""
    return hashlib.sha256(path.read_bytes()).hexdigest()


def stop_owned(process):
    """Terminate this wrapper's adb child group; never stop emulators or services."""
    try:
        os.killpg(process.pid, signal.SIGTERM)
    except ProcessLookupError:
        return
    try:
        process.wait(timeout=3)
    except subprocess.TimeoutExpired:
        os.killpg(process.pid, signal.SIGKILL)
        process.wait(timeout=5)


def main():
    """Verify the fixed product/new public fixture, install only tests, and seal raw results."""
    parser = argparse.ArgumentParser(description="仅新E2实例3项Android真实HTTPS/UI；复用原5582，不创建/擦除AVD。")
    parser.add_argument("--build-report", type=Path, required=True)
    parser.add_argument("--environment", type=Path, required=True, help="工程外本轮私密metadata，仅提取public URL/CA/部署/epoch")
    parser.add_argument("--install-missing-product", action="store_true", help="仅当前user确无本包时安装同一a135原包；不替换未知包")
    parser.add_argument("--required-product-sha", default=PRODUCT_SHA, help="已通过本轮原生回归的新产品SHA；默认保持a135")
    parser.add_argument("--execute", action="store_true")
    options = parser.parse_args()
    print("JobsIM：新实例3项原生HTTPS/UI；6个本轮测试账号，明确登出。600秒/4GiB门禁；不重启服务、不改全局信任、不清原用户库。", flush=True)
    if not options.execute:
        return 0
    product_sha = options.required_product_sha
    if not re.fullmatch(r"[0-9a-f]{64}", product_sha):
        raise ValueError("EXACT_VERIFIED_PRODUCT_SHA_REQUIRED")
    report = ROOT / "项目相关报告/测试/前端/Android/20261116-S2本人状态" / (datetime.now().strftime("HTTPS-%Y%m%d-%H%M%S-") + str(os.getpid()) + "-" + uuid.uuid4().hex[:6])
    report.mkdir(parents=True)
    shutil.copy2(Path(__file__), report / "self_state_https.py")
    runtime_root = Path.home() / "Library/Application Support/JobsIM/Build/Android"
    runtime_root.mkdir(parents=True, exist_ok=True)
    runtime = Path(tempfile.mkdtemp(prefix="native-self-https-", dir=runtime_root))
    adb = Path(os.environ.get("ANDROID_SDK_ROOT", str(Path.home() / "Library/Android/sdk"))) / "platform-tools/adb"
    summary = {"status": "failed", "startedAt": datetime.now(timezone.utc).isoformat(), "serial": SERIAL, "suite": SUITE,
               "realHTTPS": True, "apiTestsStarted": False, "productSHA256": product_sha, "runnerSHA256": digest(Path(__file__)), "clockInjection": False}
    deadline = time.monotonic() + 600
    samples = []
    reverse_created = False
    instrumentation_started = False
    run_id = str(uuid.uuid4())
    port = None

    def run(arguments, name, timeout=30, binary=False):
        """Bound the exact owned adb invocation and keep raw output, never seed credentials."""
        if shutil.disk_usage(ROOT).free < MIN_FREE:
            raise RuntimeError("DISK_BELOW_4_GIB")
        remaining = min(timeout, deadline - time.monotonic())
        if remaining <= 0:
            raise RuntimeError("NATIVE_SELF_TIME_LIMIT")
        target = report / name
        with target.open("wb") as output:
            process = subprocess.Popen([str(adb), "-s", SERIAL, *arguments], stdout=output, stderr=subprocess.STDOUT, start_new_session=True)
            start = time.monotonic()
            try:
                while process.poll() is None:
                    free = shutil.disk_usage(ROOT).free
                    samples.append({"freeBytes": free, "secondsRemaining": round(deadline - time.monotonic(), 1)})
                    if free < MIN_FREE or time.monotonic() - start > remaining:
                        stop_owned(process)
                        raise RuntimeError("DISK_BELOW_4_GIB" if free < MIN_FREE else "NATIVE_SELF_TIME_LIMIT")
                    time.sleep(.5)
            except BaseException:
                stop_owned(process)
                raise
        if process.returncode:
            raise RuntimeError("ADB_COMMAND_FAILED:" + name)
        return target.read_bytes() if binary else target.read_text(errors="replace")

    def installed(package, label):
        """Read back only the known owned APK."""
        path = run(["shell", "pm", "path", package], label + "-path.txt").strip()
        if not re.fullmatch(r"package:/data/app/[^\r\n]+/base\.apk", path):
            raise RuntimeError("OWNED_APK_PATH_INVALID")
        local = runtime / (label + ".apk")
        run(["pull", path[8:], str(local)], label + "-pull.txt")
        return digest(local)

    result = 1
    try:
        build = (options.build_report if options.build_report.is_absolute() else ROOT / options.build_report).resolve()
        info = json.loads((build / "构建结果.json").read_text())
        if info.get("status") != "passed" or not info.get("inputsUnchanged"):
            raise RuntimeError("NEW_TEST_BUILD_NOT_VERIFIED")
        for name, sha in info["inputSHA256"].items():
            if digest(ROOT / name) != sha:
                raise RuntimeError("COMPILED_INPUT_CHANGED:" + name)
        app = build / "app-debug.apk"
        test = build / "app-debug-androidTest.apk"
        if digest(app) != product_sha or digest(test) != info["artifacts"][test.name]["sha256"]:
            raise RuntimeError("FIXED_PRODUCT_OR_NEW_TEST_CHANGED")
        metadata = (options.environment if options.environment.is_absolute() else ROOT / options.environment).resolve()
        if metadata.stat().st_mode & 0o077:
            raise RuntimeError("PRIVATE_METADATA_PERMISSIONS_REQUIRED")
        env = json.loads(metadata.read_text())
        base = env["serverURL"]
        parsed = urlparse(base)
        if parsed.scheme != "https" or parsed.hostname != "127.0.0.1" or parsed.username or parsed.password or parsed.query or parsed.fragment or parsed.path not in ("", "/"):
            raise RuntimeError("NEW_LOOPBACK_HTTPS_SCOPE_REQUIRED")
        port = parsed.port
        if port in (8443, 18443, 20443, 28443, 36443, 44443) or not port:
            raise RuntimeError("OLD_FIXED_REALM_FORBIDDEN")
        ca = Path(env["caPath"]).read_bytes()
        if b"BEGIN CERTIFICATE" not in ca or b"PRIVATE KEY" in ca:
            raise RuntimeError("PUBLIC_CA_ONLY")
        deployment, epoch = env["deploymentId"], str(env["restoreEpoch"])
        if digest(Path(env["binary"])) != env["binarySHA256"]:
            raise RuntimeError("CANDIDATE_BINARY_CHANGED")
        summary["publicFixture"] = {"serverURL": base, "deploymentId": deployment, "restoreEpoch": epoch,
                                    "caSHA256": hashlib.sha256(ca).hexdigest(), "binarySHA256": env["binarySHA256"]}
        if run(["emu", "avd", "name"], "AVD归属.txt").splitlines()[0] != AVD:
            raise RuntimeError("OWNED_AVD_REQUIRED")
        commands = subprocess.run(["ps", "-axo", "pid=,args="], capture_output=True, text=True, timeout=10, check=True).stdout
        matches = [line.strip() for line in commands.splitlines() if "qemu-system-aarch64 -avd " + AVD in line and "-port 5582" in line]
        if len(matches) != 1:
            raise RuntimeError("OWNED_AVD_PID_UNCONFIRMED")
        summary["ownedEmulatorPID"] = int(matches[0].split(None, 1)[0])
        packages = run(["shell", "pm", "list", "packages", "com.jobs.im"], "当前本包清单.txt").splitlines()
        if "package:com.jobs.im" not in packages:
            if not options.install_missing_product:
                raise RuntimeError("OWNED_PRODUCT_MISSING_REQUIRES_EXPLICIT_REBUILD_OPTION")
            current_user = run(["shell", "am", "get-current-user"], "当前Android用户.txt").strip()
            if current_user != "0":
                raise RuntimeError("OWNED_PRIMARY_TEST_USER_REQUIRED")
            run(["install", "-r", str(app)], "缺包时安装同一原产品.txt", 90)
            summary["productInstallation"] = "Package absent; installed same fixed product bytes without wipe/uninstall/clear; previous app data and Keystore recovery unverified"
        if installed("com.jobs.im", "当前产品") != product_sha:
            raise RuntimeError("INSTALLED_PRODUCT_CHANGED")
        run(["install", "-r", str(test)], "安装新测试包.txt", 90)
        test_sha = installed("com.jobs.im.test", "新测试包")
        if test_sha != digest(test):
            raise RuntimeError("INSTALLED_TEST_SHA_MISMATCH")
        summary["installedSHA256"] = {app.name: product_sha, test.name: test_sha}
        mappings = run(["reverse", "--list"], "原端口映射.txt")
        existing = [line.split() for line in mappings.splitlines() if f"tcp:{port}" in line.split()[1:2]]
        if existing and existing[0][-1] != f"tcp:{port}":
            raise RuntimeError("EXISTING_REVERSE_CONFLICT")
        if not existing:
            run(["reverse", f"tcp:{port}", f"tcp:{port}"], "建立本轮映射.txt")
            reverse_created = True
        arguments = ["shell", "am", "instrument", "-w", "-r", "-e", "class", SUITE, "-e", "nativeSelf", "true",
                     "-e", "nativeSelfRun", run_id,
                     "-e", "liveUrl", base, "-e", "caBase64", base64.b64encode(ca).decode(), "-e", "deploymentId", deployment,
                     "-e", "restoreEpoch", epoch, "com.jobs.im.test/androidx.test.runner.AndroidJUnitRunner"]
        instrumentation_started = True
        summary["apiTestsStarted"] = True
        raw = run(arguments, "真实HTTPS与UI原始结果.txt", 450)
        source = ROOT / "测试/前端/Android/Instrumentation/NativeSelfStateHttpsTest.kt"
        expected = set(re.findall(r"@Test\s+fun (\w+)\(", source.read_text()))
        context, outcomes = {}, []
        for line in raw.splitlines():
            if line.startswith("INSTRUMENTATION_STATUS: ") and "=" in line:
                key, value = line[len("INSTRUMENTATION_STATUS: "):].split("=", 1)
                context[key] = value
            elif line.startswith("INSTRUMENTATION_STATUS_CODE:"):
                code = int(line.split(":", 1)[1])
                if code != 1 and "test" in context:
                    outcomes.append({"test": context["test"], "class": context.get("class"), "code": code})
                context = {}
        summary["outcomes"] = outcomes
        summary["totals"] = {"tests": len(outcomes), "passed": sum(row["code"] == 0 for row in outcomes), "failedOrSkipped": sum(row["code"] != 0 for row in outcomes)}
        if {row["test"] for row in outcomes} != expected or len(outcomes) != 3 or any(row["class"] != SUITE or row["code"] != 0 for row in outcomes) or "INSTRUMENTATION_FAILED" in raw:
            raise RuntimeError("NATIVE_SELF_CASES_DID_NOT_ALL_PASS")
        run(["pull", f"/sdcard/Android/data/com.jobs.im/files/SelfStateUiEvidence/{run_id}", str(report / "截图")], "提取本轮截图.txt")
        screenshots = list((report / "截图").glob("*.png"))
        if len(screenshots) != 5 or any(not file.read_bytes().startswith(b"\x89PNG\r\n\x1a\n") for file in screenshots):
            raise RuntimeError("FIVE_ACTUAL_NATIVE_SCREENSHOTS_REQUIRED")
        summary["screenshots"] = {str(file.relative_to(report)): digest(file) for file in screenshots}
        summary["status"] = "passed"
        result = 0
    except BaseException as error:
        summary["error"] = str(error)
        if instrumentation_started:
            for package in ["com.jobs.im.test", "com.jobs.im"]:
                try:
                    stopped = subprocess.run([str(adb), "-s", SERIAL, "shell", "am", "force-stop", package], capture_output=True, timeout=10, check=False)
                    if stopped.returncode:
                        summary.setdefault("ownedProcessStopUnconfirmed", []).append(package)
                except (subprocess.SubprocessError, OSError):
                    summary.setdefault("ownedProcessStopUnconfirmed", []).append(package)
    finally:
        if instrumentation_started:
            try:
                steps = subprocess.run([str(adb), "-s", SERIAL, "exec-out", "run-as", "com.jobs.im", "cat", f"files/NativeSelfSteps/{run_id}.jsonl"], capture_output=True, timeout=10, check=False)
                if steps.returncode == 0:
                    records = [json.loads(line) for line in steps.stdout.splitlines() if line]
                    if any(set(row) - {"stage", "busy", "selected", "selfEnabled", "errorCode"} for row in records):
                        raise ValueError("PUBLIC_UI_STEPS_INVALID")
                    (report / "原生UI步骤.json").write_text(json.dumps(records, ensure_ascii=False, indent=2) + "\n")
            except (ValueError, TypeError, subprocess.SubprocessError, OSError):
                summary["uiStepEvidence"] = "unavailable; original raw retained"
            try:
                partial = subprocess.run([str(adb), "-s", SERIAL, "pull", f"/sdcard/Android/data/com.jobs.im/files/SelfStateUiEvidence/{run_id}", str(report / "原始当场截图")], capture_output=True, timeout=15, check=False)
                summary["partialScreenshotsPulled"] = partial.returncode == 0
                cleanup = subprocess.run([str(adb), "-s", SERIAL, "exec-out", "run-as", "com.jobs.im", "cat", f"files/NativeSelfAccounts/{run_id}.json"], capture_output=True, timeout=10, check=False)
                facts = json.loads(cleanup.stdout)
                allowed = {"runId", "username", "deploymentId", "restoreEpoch", "requestId", "userId", "familyId", "deviceId", "closed"}
                if not isinstance(facts, list) or any(not isinstance(row, dict) or set(row) - allowed or row.get("runId") != run_id or not row.get("username", "").startswith("s2_android_") for row in facts):
                    raise ValueError("NON_SECRET_OWNED_ACCOUNT_JOURNAL_INVALID")
                (report / "本轮会话关闭事实.json").write_text(json.dumps(facts, ensure_ascii=False, indent=2) + "\n")
                summary["ownedFamiliesConfirmedClosed"] = len(facts) == 6 and all(row.get("closed") is True for row in facts)
                if not summary["ownedFamiliesConfirmedClosed"]:
                    summary["status"] = "failed"
                    summary["cleanupRequired"] = "Review only this run's public account/request/family IDs; no family closure is assumed after interruption"
                    result = 1
            except (ValueError, TypeError, subprocess.SubprocessError, OSError):
                summary["status"] = "failed"
                summary["cleanupRequired"] = "Owned account closure unconfirmed; retain original run context"
                result = 1
        if reverse_created:
            try:
                run(["reverse", "--remove", f"tcp:{port}"], "撤销本轮映射.txt")
            except BaseException:
                summary["reverseCleanup"] = "failed; this owned mapping still needs explicit removal"
                summary["status"] = "failed"
                result = 1
        summary["finishedAt"] = datetime.now(timezone.utc).isoformat()
        (report / "资源门禁.json").write_text(json.dumps(samples, indent=2) + "\n")
        (report / "真实HTTPS结果.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n")
        print(json.dumps({"status": summary["status"], "report": str(report), "totals": summary.get("totals"), "error": summary.get("error")}, ensure_ascii=False), flush=True)
    return result


if __name__ == "__main__":
    raise SystemExit(main())
