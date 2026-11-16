#!/usr/bin/env python3
"""Jobs: bounded native iOS self-state build/local tests, with immutable per-run inputs."""

import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import plistlib
import re
import shutil
import signal
import subprocess
import time
import uuid

ROOT = Path(__file__).resolve().parents[4]
IOS = ROOT / "代码/前端/iOS"
MIN_FREE = 4 * 1024 ** 3


def sha(path):
    """Hash original bytes rather than a normalized representation."""
    return hashlib.sha256(path.read_bytes()).hexdigest()


def owned_inputs():
    """Exclude Pods/templates/generated code; include actual app, owned Pods and tests."""
    files = list((IOS / "JobsIM").rglob("*.swift"))
    files += list((IOS / "JobsIM/Resource").rglob("*.strings"))
    files += list((IOS / "JobsByPods").rglob("*.swift"))
    files += list((IOS / "JobsByPods").rglob("*.podspec"))
    files += list((ROOT / "测试/前端/iOS").glob("*.swift"))
    files += list((ROOT / "测试/前端/iOS/UI").glob("*.swift"))
    files += [p for p in (ROOT / "测试/前端/iOS/Fixtures").rglob("*") if p.is_file()]
    files += [p for p in (IOS / "JobsIM/Resource").rglob("*") if p.is_file()]
    files += [IOS / "JobsIM.xcodeproj/project.pbxproj", IOS / "JobsIM.xcodeproj/xcshareddata/xcschemes/JobsIM.xcscheme", Path(__file__).resolve()]
    return sorted(set(files))


def stop_owned(process):
    """Stop only this invocation's process group; do not stop simulators or other builds."""
    for sig in (signal.SIGTERM, signal.SIGKILL):
        try:
            os.killpg(process.pid, sig)
        except ProcessLookupError:
            return
        try:
            process.wait(timeout=5)
            return
        except subprocess.TimeoutExpired:
            continue


def main():
    """Run one authorized native step; real/UI phases require explicit methods and a fresh run UUID."""
    parser = argparse.ArgumentParser(description="iOS S2编译／本地测试或显式新域HTTPS/UI；4GiB门禁，不启动模拟器或下载依赖。")
    parser.add_argument("--execute", action="store_true")
    parser.add_argument("--mode", choices=["build", "test", "real", "ui"], default="build")
    parser.add_argument("--simulator", required=True)
    parser.add_argument("--only", action="append", default=[])
    parser.add_argument("--build-report", type=Path)
    parser.add_argument("--run-id", help="真实三阶段共享的公开UUID；不包含凭据")
    parser.add_argument("--max-seconds", type=int, choices=range(30, 601), default=600)
    options = parser.parse_args()
    print("JobsIM：自有iOS输入单独归档，官方编译器-j1，4GiB／600秒以内；不清库、不改旧产物。", flush=True)
    if not options.execute:
        return 0
    if shutil.disk_usage(ROOT).free < MIN_FREE:
        raise SystemExit("DISK_BELOW_4_GIB")
    devices = json.loads(subprocess.check_output(["xcrun", "simctl", "list", "devices", "available", "--json"], timeout=15))["devices"]
    device = next((d for group in devices.values() for d in group if d["udid"] == options.simulator), None)
    if not device or not device["name"].startswith("JobsIM") or options.mode != "build" and device["state"] != "Booted":
        raise SystemExit("OWNED_SIMULATOR_STATE_UNCONFIRMED")
    if options.mode == "test" and (not options.only or any(not f.startswith("JobsIMTests/IM") or "Integration" in f for f in options.only)):
        raise SystemExit("EXPLICIT_LOCAL_TEST_CLASSES_REQUIRED")
    if options.mode in ["real", "ui"]:
        if not options.run_id or str(uuid.UUID(options.run_id)) != options.run_id:
            raise SystemExit("REAL_RUN_UUID_REQUIRED")
        prefix = "JobsIMTests/IMSelfStateIntegrationTests/" if options.mode == "real" else "JobsIMUITests/IMSelfStateUITests/"
        if not options.only or any(not f.startswith(prefix) for f in options.only):
            raise SystemExit("EXPLICIT_SELF_STATE_REAL_METHODS_REQUIRED")
    report = ROOT / "项目相关报告/测试/前端/iOS/20261116-S2原生界面" / (options.mode.capitalize() + "-" + datetime.now().strftime("%Y%m%d-%H%M%S-") + uuid.uuid4().hex[:6])
    report.mkdir(parents=True)
    runtime = Path.home() / "Library/Application Support/JobsIM/Build/iOS"
    compiler_wrapper = runtime / "AuthoritativeSync/Tools/swiftc-j1"
    if not compiler_wrapper.is_file():
        raise SystemExit("EXISTING_SINGLE_JOB_COMPILER_WRAPPER_MISSING")
    products = runtime / "PersonalStateUI/Products/Debug-iphonesimulator"
    command = ["xcodebuild", "-workspace", str(IOS / "JobsIM.xcworkspace"), "-scheme", "JobsIM", "-destination", "platform=iOS Simulator,id=" + options.simulator,
               "-derivedDataPath", str(runtime / "DerivedData"), "-jobs", "1", "ARCHS=arm64", "ONLY_ACTIVE_ARCH=YES", "CODE_SIGN_IDENTITY=-",
               "SWIFT_ENABLE_BATCH_MODE=NO", "SWIFT_USE_INTEGRATED_DRIVER=NO", "SWIFT_EXEC=" + str(compiler_wrapper), "CONFIGURATION_BUILD_DIR=" + str(products)]
    if options.mode == "build":
        command += ["build-for-testing"]
    else:
        if not options.build_report:
            raise SystemExit("FIXED_BUILD_REPORT_REQUIRED")
        prior = json.loads((options.build_report / "结果.json").read_text())
        if prior.get("status") != "passed" or any(sha(ROOT / name) != value for name, value in prior["inputsSHA256"].items()):
            raise SystemExit("COMPILED_INPUT_MISMATCH")
        if any(sha(Path(name)) != value for name, value in prior["binarySHA256"].items()):
            raise SystemExit("COMPILED_BINARY_MISMATCH")
        source = options.build_report / "原始测试配置.xctestrun"
        config = plistlib.loads(source.read_bytes())
        source_root = prior["testConfigurationRoot"]
        def isolate(value):
            if isinstance(value, str):
                return value.replace("__TESTROOT__", source_root)
            if isinstance(value, list):
                return [isolate(item) for item in value]
            if isinstance(value, dict):
                result = {key: isolate(item) for key, item in value.items()}
                if "TestHostPath" in result:
                    result.setdefault("EnvironmentVariables", {})["JOBS_IM_SYNC_INJECTED_TESTS"] = "1"
                    result.setdefault("TestingEnvironmentVariables", {})["JOBS_IM_SYNC_INJECTED_TESTS"] = "1"
                    if options.run_id:
                        result["EnvironmentVariables"]["JOBS_IM_SELF_STATE_RUN"] = options.run_id
                        result["TestingEnvironmentVariables"]["JOBS_IM_SELF_STATE_RUN"] = options.run_id
                return result
            return value
        fixed = report / "隔离本地测试.xctestrun"
        fixed.write_bytes(plistlib.dumps(isolate(config)))
        command = ["xcodebuild", "-xctestrun", str(fixed), "-destination", "platform=iOS Simulator,id=" + options.simulator,
                   "-parallel-testing-enabled", "NO", "-resultBundlePath", str(report / "结果.xcresult"), "test-without-building"]
        command += ["-only-testing:" + item for item in options.only]
    environment = os.environ.copy()
    environment["JOBS_IM_OFFICIAL_SWIFTC"] = subprocess.check_output(["xcrun", "--find", "swiftc"], text=True, timeout=10).strip()
    environment["JOBS_IM_SWIFT_J1_AUDIT"] = str(report / "编译器并发审计.jsonl")
    environment["SIMCTL_CHILD_JOBS_IM_SYNC_INJECTED_TESTS"] = "1"
    before = {str(p.relative_to(ROOT)): sha(p) for p in owned_inputs()}
    for name in before:
        target = report / "输入原件" / name
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(ROOT / name, target)
    metadata = {"status": "failed", "command": command, "apiUsed": options.mode in ["real", "ui"], "runID": options.run_id, "testBoundary": "DEBUG shared IMSession restore isolation; real suite explicitly enables its own API; UI launches actual app with flag0", "inputsSHA256": before,
                "compilerWrapperSHA256": sha(compiler_wrapper), "startedAt": datetime.now(timezone.utc).isoformat()}
    samples = []
    started = time.monotonic()
    result = 1
    process = None
    try:
        with (report / "原始日志.txt").open("wb") as log:
            process = subprocess.Popen(command, cwd=IOS, env=environment, stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
            while process.poll() is None:
                free = shutil.disk_usage(ROOT).free
                samples.append({"seconds": round(time.monotonic() - started, 2), "freeBytes": free})
                if free < MIN_FREE or time.monotonic() - started >= options.max_seconds:
                    stop_owned(process)
                    raise RuntimeError("DISK_OR_TIME_GATE")
                time.sleep(1)
        metadata["exitCode"] = process.returncode
        metadata["inputsUnchanged"] = all(sha(ROOT / name) == value for name, value in before.items())
        if process.returncode != 0 or not metadata["inputsUnchanged"]:
            raise RuntimeError("NATIVE_STEP_FAILED_OR_INPUT_CHANGED")
        if options.mode != "build":
            raw = (report / "原始日志.txt").read_text(errors="replace")
            outcomes = re.findall(r"Test Case '-\[([^ ]+) ([^\]]+)\]' (passed|failed|skipped) \(", raw)
            expected = set()
            for selector in options.only:
                parts = selector.split("/")
                if len(parts) == 3:
                    expected.add((parts[1], parts[2]))
                else:
                    folder = ROOT / "测试/前端/iOS" / ("UI" if parts[0] == "JobsIMUITests" else "")
                    expected.update((parts[1], name) for name in re.findall(r"func (test\w+)\(", (folder / (parts[1] + ".swift")).read_text()))
            actual = {(kind.split(".")[-1], method): status for kind, method, status in outcomes}
            metadata["methodOutcomes"] = [{"class": k[0], "method": k[1], "status": v} for k, v in actual.items()]
            if set(actual) != expected or any(v != "passed" for v in actual.values()):
                raise RuntimeError("EXPECTED_NATIVE_METHODS_NOT_ALL_PASSED")
            if any(name.endswith("/testVerifyNativeUIFixtureClosed") for name in options.only):
                container = Path(subprocess.check_output(["xcrun", "simctl", "get_app_container", options.simulator, "com.jobs.im", "data"], text=True, timeout=15).strip())
                evidence = container / "Documents/NativeSelfStateEvidence" / (options.run_id + ".json")
                facts = json.loads(evidence.read_text())
                allowed = {"username", "runID", "requestID", "userID", "profile", "familyID", "deviceID", "epoch", "deployment", "accessExpiresAt", "closed"}
                if len(facts) != 6 or any(set(row) - allowed or row.get("runID") != options.run_id or row.get("closed") != "true" for row in facts):
                    raise RuntimeError("OWNED_FAMILY_CLOSURE_UNCONFIRMED")
                shutil.copy2(evidence, report / "本轮六会话关闭事实.json")
                metadata["ownedFamiliesConfirmedClosed"] = True
        if options.mode == "build":
            binaries = [products / "JobsIM.app/JobsIM", products / "JobsIM.app/PlugIns/JobsIMTests.xctest/JobsIMTests"]
            if not all(p.is_file() for p in binaries):
                raise RuntimeError("NATIVE_PRODUCTS_MISSING")
            metadata["binarySHA256"] = {str(p): sha(p) for p in binaries}
            configs = sorted((runtime / "DerivedData/Build/Products").glob("*.xctestrun"), key=lambda p: p.stat().st_mtime, reverse=True)
            source = next((p for p in configs if str(products).encode() in p.read_bytes()), None)
            if source is None:
                raise RuntimeError("CURRENT_TEST_CONFIGURATION_MISSING")
            shutil.copy2(source, report / "原始测试配置.xctestrun")
            metadata["testConfigurationRoot"] = str(source.parent)
            metadata["testConfigurationSHA256"] = sha(source)
            for name in ["JobsIM.app", "JobsIMUITests-Runner.app"]:
                product = products / name
                if product.is_dir():
                    shutil.copytree(product, report / "产品" / name)
        metadata["status"] = "passed"
        result = 0
    except BaseException as error:
        if process and process.poll() is None:
            stop_owned(process)
        metadata["error"] = str(error)
    finally:
        metadata["seconds"] = round(time.monotonic() - started, 2)
        metadata["finishedAt"] = datetime.now(timezone.utc).isoformat()
        (report / "资源门禁.json").write_text(json.dumps(samples, indent=2) + "\n")
        (report / "结果.json").write_text(json.dumps(metadata, ensure_ascii=False, indent=2) + "\n")
        print(json.dumps({"status": metadata["status"], "report": str(report), "seconds": metadata["seconds"], "error": metadata.get("error")}, ensure_ascii=False), flush=True)
    return result


if __name__ == "__main__":
    raise SystemExit(main())
