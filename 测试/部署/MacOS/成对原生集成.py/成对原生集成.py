#!/usr/bin/env python3
"""仅新隔离实例：实际fzf、原生双PG/NATS、TLS、聊天及配对反部署。"""
import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import pty
import re
import select
import signal
import socket
import struct
import subprocess
import termios
import time
import uuid
import ssl
import urllib.request


ROOT = Path(__file__).resolve().parents[4]
RUN = ROOT / "部署/MacOS/Deployment/【MacOS】部署JobsIM.command/【MacOS】部署JobsIM.command"
UNDEPLOY = ROOT / "部署/MacOS/Undeployment/【MacOS】反部署JobsIM.command/【MacOS】反部署JobsIM.command"


def build_inputs():
    """只记录自有构建输入，排除缓存、生成物及第三方挂载。"""
    paths = [RUN, UNDEPLOY, ROOT / "部署/deploy-tools.lock.tsv"]
    for directory in [ROOT / "代码/后端/Go", ROOT / "代码/前端/Web", ROOT / "部署/公共组件/依赖版本", ROOT / "部署/公共组件/地址二维码"]:
        for path in directory.rglob("*"):
            if path.is_file() and not path.is_symlink() and not any(part in {"node_modules", "Dist", ".git", "第三方依赖", "vendor"} for part in path.relative_to(directory).parts) and path.suffix in {".go", ".mod", ".sum", ".ts", ".tsx", ".css", ".json", ".html", ".svg"}:
                paths.append(path)
    return {str(path.relative_to(ROOT)): hashlib.sha256(path.read_bytes()).hexdigest() for path in sorted(set(paths))}


def free_port():
    """让内核选择当前未被占用的回环端口。"""
    with socket.socket() as listener:
        listener.bind(("127.0.0.1", 0))
        return listener.getsockname()[1]


def start_pty(arguments, environment):
    """用真实伪终端运行正式入口，保持fzf键盘路径。"""
    child, master = pty.fork()
    if child == 0:
        os.chdir("/")
        os.execvpe(arguments[0], arguments, environment)
    fcntl.ioctl(master, termios.TIOCSWINSZ, struct.pack("HHHH", 30, 140, 0, 0))
    return child, master


def wait_pty(child, master, log, responders, ready=None, timeout=300):
    """按实际提示发送方向键；任何业务失败都保留原始PTY日志。"""
    start = time.monotonic()
    output = b""
    sent = set()
    with log.open("ab") as stream:
        while time.monotonic() - start < timeout:
            readable, _, _ = select.select([master], [], [], 0.2)
            if readable:
                try:
                    data = os.read(master, 65536)
                except OSError:
                    data = b""
                if data:
                    if b"\x1b[6n" in data:
                        os.write(master, b"\x1b[30;1R")
                    if b"\x1b[?2004$p" in data:
                        os.write(master, b"\x1b[?2004;2$y")
                    output += data
                    stream.write(data)
                    stream.flush()
                    text = output.decode("utf-8", "replace")
                    for index, (prompt, answer) in enumerate(responders):
                        if index not in sent and prompt in text:
                            time.sleep(0.25)
                            if answer.endswith(b"\r") and len(answer) > 1:
                                os.write(master, answer[:-1])
                                time.sleep(0.5)
                                os.write(master, b"\r")
                            else:
                                os.write(master, answer)
                            sent.add(index)
            if ready is not None and ready():
                return {"running": True, "responses": sorted(sent)}
            waited, status = os.waitpid(child, os.WNOHANG)
            if waited:
                return {"exitCode": os.waitstatus_to_exitcode(status), "responses": sorted(sent)}
        raise RuntimeError("正式入口等待超时；原始PTY日志保留")


def main():
    """合成资料的完整新实例只在显式授权后创建。"""
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--yes", action="store_true")
    parser.add_argument("--local-only", action="store_true", help="实际仅本机启动并仅停保留，不建立穿透或清理数据")
    parser.add_argument("--report", type=Path)
    args = parser.parse_args()
    if not args.yes:
        parser.error("需要--yes，仅授权本轮新隔离实例")
    os.umask(0o077)
    token = uuid.uuid4().hex
    private = Path.home() / "Library/Application Support/JobsIM/DeploymentTests/Paired" / token
    private.mkdir(parents=True, mode=0o700)
    state, backups, recovery, tls = [private / name for name in ["State", "Backups", "Recovery", "TLSInput"]]
    tls.mkdir(mode=0o700)
    report = args.report or ROOT / "项目相关报告/测试/部署/20261118-成对部署反部署/MacOS" / ("原生集成-" + token)
    report.mkdir(parents=True, exist_ok=False)
    openssl = os.environ.get("JOBS_IM_OPENSSL", "/opt/homebrew/bin/openssl")
    with (private / "tls.log").open("wb") as log:
        def command(arguments):
            subprocess.run(arguments, check=True, stdout=log, stderr=log, timeout=30)
        command([openssl, "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "2", "-keyout", str(tls / "ca.key"), "-out", str(tls / "ca.crt"), "-subj", "/CN=JobsIM Synthetic Paired Test CA", "-addext", "basicConstraints=critical,CA:TRUE", "-addext", "keyUsage=critical,keyCertSign,cRLSign", "-addext", "subjectKeyIdentifier=hash"])
        command([openssl, "req", "-new", "-newkey", "rsa:2048", "-nodes", "-keyout", str(tls / "server.key"), "-out", str(tls / "server.csr"), "-subj", "/CN=localhost"])
        (tls / "server.ext").write_text("subjectAltName=DNS:localhost,IP:127.0.0.1\nbasicConstraints=critical,CA:FALSE\nkeyUsage=critical,digitalSignature,keyEncipherment\nextendedKeyUsage=serverAuth\nsubjectKeyIdentifier=hash\nauthorityKeyIdentifier=keyid,issuer\n")
        command([openssl, "x509", "-req", "-in", str(tls / "server.csr"), "-CA", str(tls / "ca.crt"), "-CAkey", str(tls / "ca.key"), "-CAcreateserial", "-days", "2", "-out", str(tls / "server.crt"), "-extfile", str(tls / "server.ext")])
    ports = [free_port() for _ in range(4)]
    role_base = 49101
    while True:
        probes = []
        try:
            for port in range(role_base, role_base + 9):
                probe = socket.socket()
                probe.bind(("127.0.0.1", port))
                probes.append(probe)
            break
        except OSError:
            role_base += 20
        finally:
            for probe in probes:
                probe.close()
    environment = dict(os.environ, TERM="xterm-256color", GOMAXPROCS="2", GOFLAGS="-p=1", JOBS_IM_DEPENDENCY_CHECK_TIMEOUT_SECONDS="5", JOBS_IM_PG_PORT=str(ports[0]), JOBS_IM_JOURNAL_PORT=str(ports[1]), JOBS_IM_HTTPS_PORT=str(ports[2]), JOBS_IM_NATS_PORT=str(ports[3]), JOBS_IM_INTERNAL_PORT=str(role_base), JOBS_IM_BACKUP_DIR=str(backups), JOBS_IM_RECOVERY_DIR=str(recovery), JOBS_IM_PUBLIC_URL=f"https://localhost:{ports[2]}", JOBS_IM_TLS_CERT=str(tls / "server.crt"), JOBS_IM_TLS_KEY=str(tls / "server.key"), JOBS_IM_TLS_CA=str(tls / "ca.crt"))
    before = {str(p.relative_to(ROOT)): hashlib.sha256(p.read_bytes()).hexdigest() for p in [RUN, UNDEPLOY]}
    summary = {"mode": "local-private-stop" if args.local_only else "server-backup-undeploy", "privateFixture": str(private), "ports": ports, "roleBase": role_base, "inputs": before, "buildInputs": build_inputs(), "realInstanceTouched": False, "physicalJournalDeletionAuthorized": False}
    controller = None
    try:
        controller, master = start_pty(["/bin/zsh", str(RUN), "run", "--yes", "--state", str(state)], environment)
        def healthy():
            if not (state / "connection.json").is_file():
                return False
            return subprocess.run(["/usr/bin/curl", "--fail", "--silent", "--cacert", str(state / "TLS/ca.crt"), "--max-time", "1", f"https://127.0.0.1:{ports[2]}/api/health"], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0
        responders = [("部署模式 >", b"\r"), ("本地访问范围 >", b"\r")] if args.local_only else [("部署模式 >", b"\x1b[B\r"), ("已了解公开范围", b"cancel\r")]
        # 二维码先留屏，测试若读到提示则显式回车打开菜单；健康等待不依赖菜单选择。
        responders.append(("二维码保留在终端供查看", b"\r"))
        summary["deployment"] = wait_pty(controller, master, report / "部署PTY.log", responders, healthy)
        if not summary["deployment"].get("running"):
            raise RuntimeError("正式部署未启动；查看PTY原件")
        descriptor = json.loads((state / "connection.json").read_text())
        summary["deploymentID"] = descriptor["deploymentId"]
        summary["ledgerCreated"] = (state / ".Deployment/origin").is_file()
        context = ssl.create_default_context(cafile=str(state / "TLS/ca.crt"))
        with urllib.request.urlopen(f"https://localhost:{ports[2]}/", context=context, timeout=5) as response:
            homepage = response.read()
        assets = re.findall(rb'(?:src|href)="(/assets/[^"?#]+)"', homepage)
        summary["servedAssets"] = []
        runtime = next(state.glob("WebDist.*"))
        for asset in assets:
            route = asset.decode()
            with urllib.request.urlopen(f"https://localhost:{ports[2]}{route}", context=context, timeout=5) as response:
                actual = response.read()
            expected = (runtime / route.lstrip("/")).read_bytes()
            if actual != expected:
                raise RuntimeError("严格HTTPS静态资源与本轮运行资产不同")
            summary["servedAssets"].append({"route": route, "sha256": hashlib.sha256(actual).hexdigest(), "runtimeBytesMatch": True})
        summary["backendBinarySHA256"] = hashlib.sha256((state / "Bin/JobsIMd").read_bytes()).hexdigest()
        smoke_environment = dict(environment, NODE_EXTRA_CA_CERTS=str(state / "TLS/ca.crt"), JOBS_IM_TEST_URL=f"https://localhost:{ports[2]}")
        with (report / "严格TLS双账户链路.log").open("wb") as log:
            smoke = subprocess.run(["/opt/homebrew/bin/node", str(ROOT / "测试/部署/MacOS/ContractSmoke.mjs")], env=smoke_environment, stdout=log, stderr=log, timeout=60)
        summary["contractSmokeExit"] = smoke.returncode
        if smoke.returncode:
            raise RuntimeError("严格TLS聊天链路失败")
        if args.local_only:
            summary["noTunnelStarted"] = not (state / "Processes/tunnel.pid").exists()
            with (report / "仅停保留.log").open("wb") as log:
                stopped = subprocess.run(["/bin/zsh", str(UNDEPLOY), "stop", "--yes", "--no-backup", "--state", str(state)], env=environment, stdout=log, stderr=log, timeout=60)
            summary["stopExit"] = stopped.returncode
            summary["allDataRetained"] = all((state / p).exists() for p in ["MainDatabase/PG_VERSION", "JournalDatabase/PG_VERSION", "Application/instance.json", "Secrets/operator.password", "TLS/ca.key"])
            summary["inputStable"] = {str(p.relative_to(ROOT)): hashlib.sha256(p.read_bytes()).hexdigest() for p in [RUN, UNDEPLOY]} == before
            summary["buildInputsAfter"] = build_inputs()
            summary["buildInputsStable"] = summary["buildInputsAfter"] == summary["buildInputs"]
            summary["status"] = "passed-local-private-stop" if all([stopped.returncode == 0, summary["noTunnelStarted"], summary["allDataRetained"], summary["inputStable"], summary["buildInputsStable"]]) else "failed"
            if summary["status"] != "passed-local-private-stop":
                raise RuntimeError("仅本机启动/不建穿透/仅停保留验证失败")
            return
        unchild, unmaster = start_pty(["/bin/zsh", str(UNDEPLOY), "undeploy", "--yes", "--state", str(state)], environment)
        summary["undeployment"] = wait_pty(unchild, unmaster, report / "反部署PTY.log", [("反部署 >", b"\x1b[B\r"), ("数据处理 >", b"\r"), ("按回车执行以上清理", b"\r")], timeout=180)
        os.close(unmaster)
        if summary["undeployment"].get("exitCode") != 0:
            raise RuntimeError("正式反部署未成功；保留源资料")
        summary["businessRemoved"] = not (state / "MainDatabase").exists() and not (state / "Bin").exists() and [p.name for p in (state / "Application").iterdir()] == ["instance.json"]
        summary["journalAndKeysPreserved"] = all((state / p).exists() for p in ["JournalDatabase/PG_VERSION", "Application/instance.json", "Secrets/operator.password", "Secrets/journal.password", "TLS/server.key", ".Deployment/undeploy/phase"])
        summary["archives"] = [p.name for p in backups.glob("*.cms")]
        summary["recoveryKeyPreserved"] = (recovery / "key.pem").is_file()
        summary["portsReleased"] = True
        for port in ports:
            with socket.socket() as probe:
                if probe.connect_ex(("127.0.0.1", port)) == 0:
                    summary["portsReleased"] = False
        summary["status"] = "passed-partial-protected-retained" if all([summary["businessRemoved"], summary["journalAndKeysPreserved"], summary["archives"], summary["recoveryKeyPreserved"], summary["portsReleased"]]) else "failed"
        summary["after"] = {str(p.relative_to(ROOT)): hashlib.sha256(p.read_bytes()).hexdigest() for p in [RUN, UNDEPLOY]}
        summary["inputStable"] = summary["after"] == before
        summary["buildInputsAfter"] = build_inputs()
        summary["buildInputsStable"] = summary["buildInputsAfter"] == summary["buildInputs"]
        if not summary["inputStable"] or not summary["buildInputsStable"]:
            summary["status"] = "failed-input-drift"
        if summary["status"] != "passed-partial-protected-retained":
            raise RuntimeError("清理、保护、资源释放或输入稳定性检查未全部通过")
    except Exception as error:
        summary["status"] = "failed"
        summary["error"] = str(error)
        raise
    finally:
        if controller:
            try:
                if os.getpgid(controller) == controller:
                    os.killpg(controller, signal.SIGTERM)
                else:
                    os.kill(controller, signal.SIGTERM)
            except ProcessLookupError:
                pass
            try:
                wait_pty(controller, master, report / "部署收尾PTY.log", [], timeout=30)
            except (OSError, ChildProcessError, RuntimeError):
                pass
            os.close(master)
        (report / "summary.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n")
        print(json.dumps(summary, ensure_ascii=False))


if __name__ == "__main__":
    main()
