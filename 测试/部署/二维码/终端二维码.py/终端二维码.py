#!/usr/bin/env python3
"""只运行二维码/PTY夹具，不启动业务服务，不使用已有实例。"""
import argparse
import fcntl
import hashlib
import json
import os
from pathlib import Path
import pty
import re
import select
import struct
import subprocess
import termios
import time


ROOT = Path(__file__).resolve().parents[4]
MAC = [ROOT / "部署/MacOS/Deployment/【MacOS】部署JobsIM.command/【MacOS】部署JobsIM.command", ROOT / "部署/MacOS/Undeployment/【MacOS】反部署JobsIM.command/【MacOS】反部署JobsIM.command"]


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def pty_run(script, arguments, rows, columns, environment, menu=False):
    """保留实际PTY字节；菜单用本轮短sleep代替业务，只验证扫码停留和回车。"""
    pid, fd = pty.fork()
    if pid == 0:
        os.execve("/bin/zsh", ["/bin/zsh", str(script)] + arguments, environment)
    fcntl.ioctl(fd, termios.TIOCSWINSZ, struct.pack("HHHH", rows, columns, 0, 0))
    result = b""
    deadline = time.monotonic() + 30
    ready = None
    entered = False
    menu_seen = False
    while time.monotonic() < deadline:
        readable, _, _ = select.select([fd], [], [], .1)
        if readable:
            try:
                data = os.read(fd, 65536)
            except OSError:
                break
            if not data:
                break
            result += data
            if b"\x1b[6n" in data:
                os.write(fd, b"\x1b[30;1R")
        if menu and b"QR-READY" in result:
            if ready is None:
                ready = time.monotonic()
            if not entered and time.monotonic() - ready > .6:
                assert "打开网页 >" not in result.decode("utf-8", "replace"), "未回车便弹出fzf遮住二维码"
                os.write(fd, b"\r")
                entered = True
            if entered and "打开网页 >" in result.decode("utf-8", "replace") and not menu_seen:
                menu_seen = True
                os.write(fd, b"\x1b")
    else:
        os.kill(pid, 15)
        raise RuntimeError("二维码PTY等待超时")
    _, status = os.waitpid(pid, 0)
    os.close(fd)
    if os.waitstatus_to_exitcode(status) != 0:
        failure = script.parent / "last-failed-PTY.log"
        failure.write_bytes(result)
        raise RuntimeError(f"PTY退出{os.waitstatus_to_exitcode(status)}；原件见{failure}")
    if menu:
        assert entered and menu_seen
    return result


def decode_png(path):
    source = """ObjC.import('AppKit');ObjC.import('CoreImage');function run(argv){var d=$.CIDetector.detectorOfTypeContextOptions($.CIDetectorTypeQRCode,$.CIContext.contextWithOptions($.NSDictionary.dictionary),$({CIDetectorAccuracy:'CIDetectorAccuracyHigh'}));var i=$.CIImage.imageWithContentsOfURL($.NSURL.fileURLWithPath(argv[0]));var a=d.featuresInImage(i);if(Number(a.count)!==1)throw Error('cannot decode '+argv[0]);return ObjC.unwrap(a.objectAtIndex(0).messageString);}"""
    return subprocess.check_output(["/usr/bin/osascript", "-l", "JavaScript", "-e", source, str(path)], text=True).strip()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--report", type=Path, required=True)
    parser.add_argument("--native-preview", action="store_true", help="仅请求预览本轮生成的一张PNG")
    args = parser.parse_args()
    report = args.report.resolve()
    report.mkdir(parents=True, exist_ok=False)
    go = Path(os.environ["JOBS_IM_GO"])
    inputs = MAC + [ROOT / "部署/公共组件/地址二维码" / name for name in ["main.go", "go.mod", "go.sum"]]
    before = {str(path.relative_to(ROOT)): sha(path) for path in inputs}
    result = {"source": before, "scope": "QR-only; no Go application, PG, NATS, tunnels or user instance", "cases": []}
    body = r'''
setopt NO_NOMATCH PIPE_FAIL
set -eu
TRAPEXIT(){ return 0; }
TRAPZERR(){ exit $?; }
REPO="$1"; GO_TOOL="$2"; RUN_DIR="$3"
STATE="$RUN_DIR/state"; HTTPS_PORT=19443; PUBLIC_HOST=127.0.0.1
PUBLIC_URL="$4"; JOBS_IM_PUBLIC_URL=https://127.0.0.1:19443
COLUMNS=0; QR_PAGE=""; QR_BINARY=""
BROWSER_MENU_PID=""; TUNNEL_PID=""; DEPLOY_MODE=local
FZF_EXECUTABLE=/opt/homebrew/bin/fzf
open(){ printf '%s\n' "$@" > "$RUN_DIR/open-args"; }
prepare_im_qr
print QR-READY
if [[ "$5" == menu ]]; then
    sleep 3 & SERVER_PID=$!
    monitor_im_session
    stop_browser_menu
fi
'''
    fixtures = [
        ("80列50行", 50, 80, {}, "", False),
        ("80列30行", 30, 80, {}, "", False),
        ("窄32列", 30, 32, {}, "", False),
        ("矮12行", 12, 80, {}, "", False),
        ("NO_COLOR空值", 30, 80, {"NO_COLOR": ""}, "", False),
        ("TERM-dumb", 30, 80, {"TERM": "dumb"}, "", False),
        ("双地址", 50, 100, {}, "https://fixture.trycloudflare.com", False),
        ("扫码停留与回车菜单", 50, 80, {}, "", True),
    ]
    try:
        for index, source in enumerate(MAC):
            loader = report / f"{index}-生产二维码函数.zsh"
            text = source.read_text()
            assert text.endswith('main "$@"\n')
            loader.write_text(text[:-len('main "$@"\n')] + body)
            for name, rows, columns, extra, public, menu in fixtures:
                directory = report / f"{index}-{name}"
                directory.mkdir()
                environment = dict(os.environ, TERM="xterm-256color", GOPROXY="off", GOTOOLCHAIN="local")
                environment.pop("NO_COLOR", None)
                environment.update(extra)
                raw = pty_run(loader, [str(ROOT), str(go), str(directory), public, "menu" if menu else "qr"], rows, columns, environment, menu)
                (directory / "PTY.log").write_bytes(raw)
                output = raw.decode("utf-8", "replace")
                plain = re.sub(r"\x1b\[[0-9;?]*[A-Za-z]", "", output).replace("\r", "")
                glyphs = "终端二维码（" in plain
                preview = (directory / "open-args").exists()
                assert b"\x1b" not in (directory / "qr-output.txt").read_bytes()
                assert "仅本机，手机不可用" in plain
                expect_preview = name in {"80列30行", "窄32列", "矮12行", "NO_COLOR空值", "TERM-dumb"}
                assert preview == expect_preview
                assert glyphs == (name not in {"80列30行", "窄32列", "矮12行"})
                if name in {"NO_COLOR空值", "TERM-dumb"}:
                    assert "\x1b" not in output
                expected = (public or "https://127.0.0.1:19443") + "/"
                assert decode_png(directory / "AccessQR/01.png") == expected
                if glyphs:
                    groups = []
                    group = []
                    for line in output.replace("\r", "").splitlines():
                        clean = re.sub(r"\x1b\[[0-9;?]*[A-Za-z]", "", line)
                        if re.fullmatch(r"[ █▀▄]+", clean) and len(clean) >= 29:
                            group.append(line)
                        elif group:
                            groups.append(group)
                            group = []
                    assert groups and len(groups[-1]) > 10
                    cells = directory / "terminal-cells.txt"
                    cells.write_text("\n".join(groups[-1]) + "\n")
                    if name not in {"NO_COLOR空值", "TERM-dumb"}:
                        for background in ["dark", "light"]:
                            image = directory / ("终端字体-" + background + ".png")
                            rendered = subprocess.check_output(["/usr/bin/osascript", "-l", "JavaScript", str(Path(__file__).with_name("渲染终端码.js")), str(cells), str(image), background, "ansi"], text=True)
                            decoded = json.loads(rendered)
                            assert decoded["decoded"] == [expected], decoded
                result["cases"].append({"script": index, "name": name, "status": "passed", "ttyRows": rows, "ttyColumns": columns, "glyphs": glyphs, "localPNGPreviewRequested": preview, "pngDecodedURL": expected, "menuNotShownBeforeEnter": menu})
            directory = report / f"{index}-非TTY"
            directory.mkdir()
            done = subprocess.run(["/bin/zsh", str(loader), str(ROOT), str(go), str(directory), "", "qr"], env=dict(os.environ, TERM="xterm-256color", COLUMNS="80", GOPROXY="off", GOTOOLCHAIN="local"), check=True, capture_output=True)
            assert b"\x1b" not in done.stdout and not (directory / "open-args").exists()
            (directory / "output.log").write_bytes(done.stdout)
            result["cases"].append({"script": index, "name": "非TTY", "status": "passed", "noBlockingInput": True, "noANSIOrGUI": True})
        result["nativePreview"] = "not-requested"
        if args.native_preview:
            image = report / "0-窄32列/AccessQR/01.png"
            subprocess.run(["/usr/bin/open", "-a", "Preview", str(image)], check=True)
            result["nativePreview"] = {"requestedOwnPNG": str(image), "decodedURL": decode_png(image), "requestExit": 0}
        result["after"] = {str(path.relative_to(ROOT)): sha(path) for path in inputs}
        assert result["source"] == result["after"]
        result["status"] = "passed"
    except Exception as error:
        result["status"] = "failed"
        result["error"] = str(error)
        raise
    finally:
        result["pass"] = sum(case["status"] == "passed" for case in result["cases"])
        (report / "summary.json").write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n")
        print(json.dumps({key: value for key, value in result.items() if key not in {"source", "after", "cases"}}, ensure_ascii=False))


if __name__ == "__main__":
    main()
