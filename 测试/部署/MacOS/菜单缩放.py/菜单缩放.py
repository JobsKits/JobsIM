#!/usr/bin/env python3
# 用途：真实 PTY／fzf 缩放与键盘回归；只运行临时假服务，不部署或停止用户实例。
# 运行：显式 --yes；证据写入新的中央报告子目录，Python 仅为测试依赖。
"""隔离验证 MacOS 成对入口的备用屏、窗口缩放与菜单生命周期。"""

from __future__ import annotations

import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import pty
import select
import shutil
import signal
import struct
import subprocess
import tempfile
import termios
import time


ROOT = Path(__file__).resolve().parents[4]
SOURCES = [ROOT / f"部署/MacOS/{folder}/【MacOS】{action}JobsIM.command/【MacOS】{action}JobsIM.command"
           for folder, action in [("Deployment", "部署"), ("Undeployment", "反部署")]]
SIZES = [(48, 210), (18, 80), (40, 160), (15, 55), (32, 120)]
ALT_ON = b"\x1b[?1049h"
ALT_OFF = b"\x1b[?1049l"
HARNESS = r'''
setopt NO_NOMATCH
eval "$(<"$1")"
RUN_DIR="$2"; FZF_EXECUTABLE="$3"; mode="$4"
DEPLOY_MODE=local; PUBLIC_URL=''; QR_PAGE="$RUN_DIR/offline.html"; HTTPS_PORT=19443
BROWSER_MENU_PID=''; BROWSER_MENU_IDENTITY=''; CLEANING=1; RUN_ACTIVE=0; LOCK_HELD=0
MOCK_CLEANED=0
TUNNEL_PID=''
# 仅替换业务清理；生产 INT/TERM/HUP trap 与真实菜单身份核验保持原样。
cleanup() {
    (( ZSH_SUBSHELL == 0 && MOCK_CLEANED == 0 )) || return 0
    MOCK_CLEANED=1
    stop_browser_menu
    if [[ -n "${sentinel:-}" ]]; then
        kill -TERM "$sentinel" 2>/dev/null || true
        wait "$sentinel" 2>/dev/null || true
        sentinel=''
        print -r -- 'FIXTURE_CLEANED'
    fi
}
sleep 120 &
sentinel=$!
SERVER_PID="$sentinel"
print -r -- "$sentinel" > "$RUN_DIR/service.pid"
if [[ "$mode" == browser-interrupt* || "$mode" == baseline-interrupt ]]; then
    CLEANING=0
    set -e
    monitor_im_session
    exit 86
elif [[ "$mode" == pick-* ]]; then
    code=0
    pick_operation '选择测试' '默认取消；↑/↓选择；回车确认；Esc取消' $'cancel\t取消本轮' $'chosen\t选择本轮' || code=$?
    print -r -- "PICK=$code:${SELECTED_OPERATION:-}"
else
    start_im_browser_menu
    code=0; wait "$BROWSER_MENU_PID" || code=$?
    print -r -- "MENU=$code:$(cat "$RUN_DIR/menu-result")"
    kill -0 "$sentinel" || exit 89
    print -r -- 'SERVICE_STILL_ALIVE'
    if [[ "$mode" == browser-resume ]]; then
        print -r -- 'PAUSED_WAITING_ENTER'
        IFS= read -r input || exit 88
        start_im_browser_menu
        code=0; wait "$BROWSER_MENU_PID" || code=$?
        print -r -- "RESUMED=$code:$(cat "$RUN_DIR/menu-result")"
        kill -0 "$sentinel" || exit 87
    fi
fi
cleanup
'''


def run_case(source: Path, mode: str, report: Path, fixture: Path) -> dict:
    """载入正式函数，仅去掉唯一 main 调用；包管理器和实例动作均不调用。"""
    name = f"{source.parents[1].name}-{mode}"
    work = fixture / name
    work.mkdir()
    content = source.read_text()
    definitions, last = content.rsplit('\nmain "$@"', 1)
    assert not last.strip(), "入口末尾合同已变化，拒绝猜测如何载入"
    if mode.startswith("baseline"):
        assert definitions.count("--no-height") == 3
        definitions = definitions.replace("--no-height", "--height=40%")
        definitions = definitions.replace(",ctrl-c:print(__jobsim_interrupt)+accept", "")
        definitions = definitions.replace('[[ "${result%%$\'\\n\'*}" != __jobsim_interrupt ]] || return 130', ':')
    copied = work / "生产函数.command"
    copied.write_text(definitions + "\n")
    harness = work / "fixture.command"
    harness.write_text(HARNESS)
    child, master = pty.fork()
    if child == 0:
        os.chdir("/")
        environment = {**os.environ, "TERM": "xterm-256color", "NO_COLOR": "1",
                       "FZF_DEFAULT_OPTS": "--height=4 --multi", "FZF_DEFAULT_COMMAND": "exit 99"}
        os.execve("/bin/zsh", ["/bin/zsh", str(harness), str(copied), str(work), shutil.which("fzf"), mode], environment)
    transcript = bytearray()
    resize_bytes = []
    waited = False

    def pump(seconds: float) -> bytes:
        start = len(transcript)
        deadline = time.monotonic() + seconds
        while time.monotonic() < deadline:
            if select.select([master], [], [], 0.02)[0]:
                try:
                    data = os.read(master, 65536)
                except OSError:
                    break
                if not data:
                    break
                transcript.extend(data)
                if b"\x1b[6n" in data:
                    os.write(master, b"\x1b[30;1R")
        return bytes(transcript[start:])

    def await_text(target: bytes, start: int = 0) -> None:
        deadline = time.monotonic() + 8
        while target not in transcript[start:] and time.monotonic() < deadline:
            pump(0.05)
        assert target in transcript[start:], f"未出现预期提示：{target!r}"

    try:
        fcntl.ioctl(master, termios.TIOCSWINSZ, struct.pack("HHHH", 30, 140, 0, 0))
        await_text("选择测试 >".encode() if mode.startswith("pick-") else "打开网页 >".encode())
        for rows, columns in SIZES:
            fcntl.ioctl(master, termios.TIOCSWINSZ, struct.pack("HHHH", rows, columns, 0, 0))
            os.killpg(child, signal.SIGWINCH)
            resize_bytes.append(len(pump(0.18)))
        assert all(resize_bytes), "窗口尺寸变化没有产生重绘"
        if mode == "browser-resume":
            os.write(master, b"\x1b")
            await_text(b"PAUSED_WAITING_ENTER")
            start = len(transcript)
            os.write(master, b"\r")
            await_text("打开网页 >".encode(), start)
            os.write(master, b"\r")
        elif mode in ("browser-interrupt", "browser-interrupt-empty"):
            if mode.endswith("-empty"):
                os.write(master, b"no-match-jobsim-fixture")
                pump(0.15)
            os.write(master, b"\x03")
        elif mode == "pick-choose":
            os.write(master, b"\x1b[B")
            pump(0.1)
            os.write(master, b"\r")
        elif mode == "pick-cancel":
            os.write(master, b"\x1b")
        else:
            os.write(master, b"\r")
        deadline = time.monotonic() + 8
        status = None
        while time.monotonic() < deadline:
            pump(0.05)
            pid, status = os.waitpid(child, os.WNOHANG)
            if pid:
                waited = True
                break
        assert waited, "本轮隔离控制器未退出"
        code = os.waitstatus_to_exitcode(status)
        data = bytes(transcript)
        expected = 130 if mode.startswith("browser-interrupt") else 0
        assert code == expected, f"退出码 {code} != {expected}"
        if mode.startswith("baseline"):
            assert ALT_ON not in data and ALT_OFF not in data
        else:
            assert data.count(ALT_ON) == data.count(ALT_OFF) >= 1, "备用屏进入／退出不成对"
            assert data.rfind(ALT_OFF) < data.rfind(b"FIXTURE_CLEANED"), "退出未先归还原终端屏幕"
        if mode in ("browser-choose", "baseline"):
            assert b"MENU=0:local\t" in data and b"SERVICE_STILL_ALIVE" in data
        elif mode == "browser-resume":
            assert b"MENU=130:" in data and b"RESUMED=0:local\t" in data
            assert data.count(ALT_ON) == 2
        elif mode == "pick-choose":
            assert b"PICK=0:chosen" in data
        elif mode == "pick-cancel":
            assert b"PICK=130:" in data
        assert b"FIXTURE_CLEANED" in data
        return {"source": str(source.relative_to(ROOT)), "mode": mode, "passed": True,
                "exitCode": code, "resizeOutputBytes": resize_bytes,
                "alternateScreenEnters": data.count(ALT_ON), "alternateScreenExits": data.count(ALT_OFF)}
    finally:
        (report / f"{name}.pty.bin").write_bytes(transcript)
        if not waited:
            os.killpg(child, signal.SIGTERM)
        os.close(master)
        if not waited:
            deadline = time.monotonic() + 3
            while time.monotonic() < deadline:
                if os.waitpid(child, os.WNOHANG)[0]:
                    break
                time.sleep(0.05)
            else:
                os.killpg(child, signal.SIGKILL)
                os.waitpid(child, 0)


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--yes", action="store_true", help="授权仅运行隔离菜单与临时假服务")
    args = parser.parse_args()
    print("真实 PTY／fzf 菜单缩放专项：仅临时假服务，不启停用户部署。")
    if not args.yes:
        parser.error("需要 --yes；未创建测试目录")
    fzf = shutil.which("fzf")
    if not fzf:
        parser.error("缺少现有 fzf；本测试不安装依赖")
    report = ROOT / "项目相关报告/测试/部署/MacOS/20261118-终端菜单缩放" / str(time.time_ns())
    report.mkdir(parents=True)
    fixture = Path(tempfile.mkdtemp(prefix="JobsIM 菜单缩放 "))
    summary = {"fzfVersion": subprocess.check_output([fzf, "--version"], text=True).strip(),
               "fixture": str(fixture), "terminalGuiDragVerified": False,
               "scope": "真实PTY协议/窗口尺寸/键盘；未连接Terminal图形界面，未启停现有服务",
               "sources": {str(p.relative_to(ROOT)): hashlib.sha256(p.read_bytes()).hexdigest() for p in SOURCES}, "cases": []}
    try:
        for source in SOURCES:
            for mode in ["browser-interrupt", "browser-interrupt-empty", "baseline", "browser-choose", "browser-resume", "pick-choose", "pick-cancel"]:
                result = run_case(source, mode, report, fixture)
                summary["cases"].append(result)
                print(f"PASS {source.parents[1].name} {mode}", flush=True)
    finally:
        (report / "summary.json").write_text(json.dumps(summary, ensure_ascii=False, indent=2) + "\n")
        print(f"证据：{report}", flush=True)


if __name__ == "__main__":
    main()
