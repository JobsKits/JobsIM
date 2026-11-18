#!/usr/bin/env python3
"""复用本轮已停止夹具，仅构建当前Web并核对真实Go严格HTTPS资源。"""
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import signal
import socket
import ssl
import subprocess
import time
import urllib.request


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def main():
    root = Path.cwd().resolve()
    report = Path(__file__).with_name("最终H5静态增量")
    report.mkdir(exist_ok=False)
    original = report.parent / "原生集成-30b83fdf26ae47c1a65f199f59e394ca/summary.json"
    baseline = json.loads(original.read_text())
    state = Path(baseline["privateFixture"]) / "State"
    spec = importlib.util.spec_from_file_location("paired_test", root / "测试/部署/MacOS/成对原生集成.py/成对原生集成.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    before = module.build_inputs()
    expected_css = "972a706c44ab376d40591542b914928e88ffa3523d7e535f8c1dd93c63616927"
    assert before["代码/前端/Web/Src/styles.css"] == expected_css
    go_inputs = {k: v for k, v in before.items() if k.startswith("代码/后端/Go/")}
    assert go_inputs == {k: v for k, v in baseline["buildInputs"].items() if k.startswith("代码/后端/Go/")}
    assert digest(state / "Bin/JobsIMd") == baseline["backendBinarySHA256"]
    ports = baseline["ports"] + list(range(baseline["roleBase"], baseline["roleBase"] + 9))
    for port in ports:
        with socket.socket() as probe:
            assert probe.connect_ex(("127.0.0.1", port)) != 0, f"自有夹具端口{port}尚未释放，停止而不接管"
    reference = root / ".jobsim-local/instance.ref"
    ref_before = digest(reference) if reference.is_file() else None
    source = module.RUN.read_text()
    assert source.endswith('main "$@"\n')
    source = source[:-len('main "$@"\n')]
    path_line = '    SCRIPT_DIR="${${(%):-%x}:A:h}"'
    assert source.count(path_line) == 1
    source = source.replace(path_line, '    SCRIPT_DIR="$JOBS_IM_TEST_SCRIPT_DIR"')
    # 仅测试加载位置替换；核心函数逐字取自最终生产源码，不经过正常启动的备份/重编后端。
    source += '''
initialize
parse_arguments run --yes --state "$JOBS_IM_DEPLOY_STATE"
DEPLOY_MODE=local
LOCAL_ACCESS=private
find_tools
assert_owner
origin_load
check_ports
acquire_lock
RUN_DIR="$STATE/.Deployment/final-h5"
mkdir "$RUN_DIR"
require_host_space 0 "$STATE"
(cd "$REPO/代码/前端/Web" && npm run build)
WEB_RUNTIME="$(mktemp -d "$STATE/WebDist.finalH5.XXXXXX")"
origin_record created "${WEB_RUNTIME:t}"
cp -R -- "$REPO/代码/前端/Web/Dist/." "$WEB_RUNTIME/"
print -r -- "$WEB_RUNTIME" > "$JOBS_IM_TEST_REPORT/runtime-path.txt"
prepare_clusters
prepare_databases
start_nats
launch_im_backend "https://127.0.0.1:$HTTPS_PORT"
print -r -- "$$" > "$JOBS_IM_TEST_REPORT/controller.pid"
print ready > "$JOBS_IM_TEST_REPORT/ready"
while [[ ! -f "$JOBS_IM_TEST_REPORT/stop" ]] && kill -0 "$PPID" 2>/dev/null; do sleep .2; done
cleanup
'''
    script = report / "生产函数加载.zsh"
    script.write_text(source)
    environment = dict(os.environ, JOBS_IM_TEST_SCRIPT_DIR=str(module.RUN.parent), JOBS_IM_TEST_REPORT=str(report), JOBS_IM_DEPLOY_STATE=str(state), JOBS_IM_PG_PORT=str(baseline["ports"][0]), JOBS_IM_JOURNAL_PORT=str(baseline["ports"][1]), JOBS_IM_HTTPS_PORT=str(baseline["ports"][2]), JOBS_IM_NATS_PORT=str(baseline["ports"][3]), JOBS_IM_INTERNAL_PORT=str(baseline["roleBase"]), JOBS_IM_BACKUP_DIR=str(state.parent / "Backups"), JOBS_IM_RECOVERY_DIR=str(state.parent / "Recovery"), JOBS_IM_OPENSSL="/opt/homebrew/bin/openssl", GOMAXPROCS="2", GOFLAGS="-p=1")
    summary = {"scope": "current-Web-production-build-and-real-Go-strict-HTTPS-assets", "reusedOwnFixture": str(state.parent), "baseline": str(original.relative_to(root)), "buildInputs": before, "buildInputCount": len(before), "backendRebuilt": False, "goInputsMatchBaseline": True, "backendBinarySHA256": baseline["backendBinarySHA256"], "defaultReferenceBefore": ref_before, "ports": ports, "serverAccountsBackupUndeployNotRepeated": True, "sourceLoaderOnlyChange": "SCRIPT_DIR resolves to production source directory; final main invocation replaced by bounded test sequence"}
    process = None
    try:
        subprocess.run(["/bin/zsh", "-n", str(script)], check=True)
        with (report / "原生启动构建与收尾.log").open("wb") as log:
            process = subprocess.Popen(["/bin/zsh", str(script)], env=environment, stdout=log, stderr=log)
            deadline = time.monotonic() + 100
            while not (report / "ready").exists():
                assert process.poll() is None, "原生构建/严格健康启动失败；查看原始日志"
                if time.monotonic() >= deadline:
                    raise RuntimeError("等待当前Web构建与实际Go启动超时")
                time.sleep(.2)
            context = ssl.create_default_context(cafile=str(state / "TLS/ca.crt"))
            base = f"https://127.0.0.1:{baseline['ports'][2]}"
            def request(route):
                with urllib.request.urlopen(base + route, context=context, timeout=5) as response:
                    assert response.status == 200
                    return response.read()
            summary["strictTLSHealth"] = json.loads(request("/api/health"))
            bootstrap = json.loads(request("/api/bootstrap"))
            summary["bootstrapDeploymentID"] = bootstrap["deploymentId"]
            assert bootstrap["deploymentId"] == baseline["deploymentID"]
            runtime = Path((report / "runtime-path.txt").read_text().strip())
            homepage = request("/")
            assert homepage == (runtime / "index.html").read_bytes() == (root / "代码/前端/Web/Dist/index.html").read_bytes()
            summary["homepageBytesMatch"] = True
            summary["servedAssets"] = []
            for asset in re.findall(rb'(?:src|href)="(/assets/[^"?#]+)"', homepage):
                route = asset.decode()
                actual = request(route)
                assert actual == (runtime / route.lstrip("/")).read_bytes() == (root / "代码/前端/Web/Dist" / route.lstrip("/")).read_bytes()
                summary["servedAssets"].append({"route": route, "sha256": hashlib.sha256(actual).hexdigest(), "bytes": len(actual), "runtimeAndCurrentDistBytesMatch": True})
            assert any(item["route"].endswith(".css") for item in summary["servedAssets"])
            assert any(item["route"].endswith(".js") for item in summary["servedAssets"])
            (report / "stop").write_text("stop own bounded instance\n")
            summary["controllerExit"] = process.wait(timeout=45)
            assert summary["controllerExit"] == 0
        summary["buildInputsAfter"] = module.build_inputs()
        summary["inputStable"] = before == summary["buildInputsAfter"]
        assert summary["inputStable"]
        summary["defaultReferenceAfter"] = digest(reference) if reference.is_file() else None
        summary["defaultReferenceUnchanged"] = ref_before == summary["defaultReferenceAfter"]
        assert summary["defaultReferenceUnchanged"]
        summary["status"] = "passed"
    except Exception as error:
        summary["status"] = "failed"
        summary["error"] = str(error)
        raise
    finally:
        (report / "stop").write_text("stop own bounded instance\n")
        if process and process.poll() is None:
            process.send_signal(signal.SIGTERM)
            try:
                process.wait(timeout=45)
            except subprocess.TimeoutExpired:
                summary["cleanupTimeout"] = True
        summary["allOwnPortsReleased"] = True
        for port in ports:
            with socket.socket() as probe:
                if probe.connect_ex(("127.0.0.1", port)) == 0:
                    summary["allOwnPortsReleased"] = False
        if not summary["allOwnPortsReleased"]:
            summary["status"] = "failed"
        summary["noTunnelCreated"] = not (state / "Processes/tunnel.pid").exists()
        (report / "summary.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n")
        print(json.dumps({key: value for key, value in summary.items() if key not in {"buildInputs", "buildInputsAfter"}}, ensure_ascii=False))


if __name__ == "__main__":
    main()
