#!/usr/bin/env python3
"""Strict-CA, isolated CLI TTY schema1 acceptance; no PG or service control."""

import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import pty
import re
import secrets
import select
import shutil
import signal
import sqlite3
import ssl
import subprocess
import termios
import time
import urllib.error
import urllib.request
import uuid


class CheckFailure(Exception):
    """Only nonsecret assertion names are allowed in public diagnostics."""


def require(condition, name):
    if not condition:
        raise CheckFailure(name)


def objects(text):
    decoder = json.JSONDecoder()
    result = []
    at = 0
    while at < len(text):
        at = text.find("{", at)
        if at < 0:
            break
        try:
            value, size = decoder.raw_decode(text[at:])
        except ValueError:
            at += 1
            continue
        result.append(value)
        at += size
    return result


class Console:
    def __init__(self, command, floor):
        self.master, slave = pty.openpty()
        mode = termios.tcgetattr(slave)
        mode[3] &= ~termios.ECHO
        termios.tcsetattr(slave, termios.TCSANOW, mode)
        self.process = subprocess.Popen(
            command, stdin=slave, stdout=slave, stderr=slave, start_new_session=True
        )
        os.close(slave)
        self.output = bytearray()
        self.position = 0
        self.started = time.monotonic()
        self.floor = floor
        self.closed = False

    def until(self, text, seconds=30):
        deadline = min(time.monotonic() + seconds, self.started + 180)
        pattern = text.encode()
        while time.monotonic() < deadline:
            require(shutil.disk_usage(self.floor).free >= 4 * 1024**3, "disk-floor")
            found = self.output.find(pattern, self.position)
            if found >= 0:
                value = bytes(self.output[self.position:found]).decode(errors="replace")
                self.position = found + len(pattern)
                return value.replace("\r\n", "\n")
            if select.select([self.master], [], [], 0.1)[0]:
                try:
                    chunk = os.read(self.master, 65536)
                except OSError:
                    chunk = b""
                if chunk:
                    self.output.extend(chunk)
                elif self.process.poll() is not None:
                    break
        raise CheckFailure("tty-phase-" + text.replace(" ", "_")[:24])

    def send(self, value):
        os.write(self.master, value.encode() + b"\n")

    def command(self, value):
        self.send(value)
        return self.until("jobsim> ")

    def finish(self):
        if self.closed:
            return True
        normal = False
        if self.process.poll() is None:
            try:
                self.send("quit")
                normal = self.process.wait(timeout=20) == 0
            except (subprocess.TimeoutExpired, OSError):
                os.killpg(self.process.pid, signal.SIGTERM)
                try:
                    self.process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    os.killpg(self.process.pid, signal.SIGKILL)
                    self.process.wait(timeout=5)
        else:
            normal = self.process.returncode == 0
        os.close(self.master)
        self.output.clear()
        self.closed = True
        return normal


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--environment-file", type=Path, required=True)
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--data-root", type=Path, required=True)
    parser.add_argument("--report-dir", type=Path, required=True)
    args = parser.parse_args()
    info = json.loads(args.environment_file.read_text())
    require(info.get("freshSyncRealm") is True, "fresh-realm-required")
    require(info.get("persistentData") is True, "persistent-realm-required")
    require(info.get("serverURL") == "https://127.0.0.1:44443", "fixed-origin-required")
    require(info.get("binarySHA256", "").startswith("57a867"), "fixed-57-required")
    require(
        hashlib.sha256(Path(info["binary"]).read_bytes()).hexdigest()
        == info["binarySHA256"],
        "server-binary-hash",
    )
    require(not args.data_root.exists(), "fresh-data-root-required")
    require(not args.report_dir.exists(), "fresh-report-required")
    require(shutil.disk_usage(args.data_root.parent).free >= 4 * 1024**3, "disk-floor")
    args.data_root.mkdir(mode=0o700, parents=True)
    args.report_dir.mkdir(parents=True)
    trust = ssl.create_default_context(cafile=info["caPath"])
    base = info["serverURL"]
    checks = []
    consoles = []
    controls = []
    phase = "bootstrap"
    result = {
        "startedAt": datetime.now(timezone.utc).isoformat(),
        "status": "running",
        "binarySHA256": hashlib.sha256(args.binary.read_bytes()).hexdigest(),
        "serverBinarySHA256": info["binarySHA256"],
        "checks": checks,
        "nativeCredentialStorePositive": False,
        "pgUsed": False,
        "serviceChanged": False,
        "rawTTYRecorded": False,
        "cliCreationHTTPCountMeasured": False,
    }

    def save():
        (args.report_dir / "result.json").write_text(
            json.dumps(result, ensure_ascii=False, indent=2) + "\n"
        )

    def check(name, condition, **facts):
        require(condition, name)
        checks.append({"name": name, "passed": True, **facts})
        save()
        print("PASS " + name, flush=True)

    def api(method, path, body=None, token=None):
        headers = {"Content-Type": "application/json"}
        if token:
            headers["Authorization"] = "Bearer " + token
        request = urllib.request.Request(
            base + path,
            method=method,
            data=None if body is None else json.dumps(body).encode(),
            headers=headers,
        )
        try:
            with urllib.request.urlopen(request, context=trust, timeout=20) as response:
                return json.load(response)
        except urllib.error.HTTPError as error:
            try:
                code = json.load(error).get("error", {}).get("code", "HTTP_REJECTED")
            except (ValueError, OSError):
                code = "HTTP_REJECTED"
            raise CheckFailure("fixture-http-" + str(error.code) + "-" + code) from None

    def login(label, username, password, mode="register"):
        data = args.data_root / label
        command = [
            str(args.binary.resolve()), "--server", base, "--ca", info["caPath"],
            "--data-dir", str(data), mode,
        ]
        console = Console(command, args.data_root)
        consoles.append(console)
        console.until("Username: ")
        console.send(username)
        console.until("Type yes: ")
        console.send("yes")
        console.until("Password: ")
        console.send(password)
        output = console.until("jobsim> ")
        require("Explicit temporary session" in output, "explicit-process-only")
        if "Authoritative synchronization is restricted" in output:
            match = re.search(r"restricted: ([A-Z][A-Z_]+)", output)
            raise CheckFailure("initial-sync-" + (match.group(1) if match else "details-redacted"))
        return console, data

    def one(text):
        values = objects(text)
        require(len(values) == 1, "one-public-json-result")
        return values[0]

    def db_path(data, active=True):
        found = []
        for path in data.rglob("client.sqlite"):
            with sqlite3.connect(path.as_uri() + "?mode=ro", uri=True) as db:
                retired = db.execute(
                    "SELECT count(*) FROM metadata WHERE key='auth_retired' AND value='1'"
                ).fetchone()[0]
            if not active or not retired:
                found.append(path)
        require(len(found) == 1, "one-active-message-store")
        return found[0]

    def rows(path, sql, parameters=()):
        with sqlite3.connect(path.as_uri() + "?mode=ro", uri=True) as db:
            return db.execute(sql, parameters).fetchall()

    def sync(console):
        state = one(console.command("sync"))
        require(state.get("restriction", "") == "", "sync-ready")
        require(state.get("qualifiedAck") is False, "transport-is-not-ack")
        return state

    try:
        boot = api("GET", "/api/bootstrap")
        require(boot["features"].get("authoritative_sync") is True, "sync-capability")
        require(boot["features"].get("sync_jetstream") is True, "jetstream-capability")
        deployment, epoch = boot["deploymentId"], boot["restoreEpoch"]
        result.update({"deploymentId": deployment, "restoreEpoch": epoch, "origin": base})
        check("strict-ca-fixed57-bootstrap", bool(uuid.UUID(deployment)) and epoch == "1")
        suffix = secrets.token_hex(4)
        user_a, user_b = "cli_sync_a_" + suffix, "cli_sync_b_" + suffix
        password = "CLI real schema1 " + secrets.token_hex(16)
        phase = "two-tty-register"
        a, data_a = login("Alice", user_a, password)
        b, data_b = login("Bob", user_b, password)
        identity_a = one(a.command("users " + user_a))["items"][0]["id"]
        identity_b = one(b.command("users " + user_b))["items"][0]["id"]
        request = one(a.command("request " + identity_b))["result"]["id"]
        one(b.command("accept " + request))
        group = one(a.command("group CLI-Sync-" + suffix + " | " + identity_b))["id"]
        sync(a)
        sync(b)
        store_a, store_b = db_path(data_a), db_path(data_b)
        check("real-tty-two-process-only-accounts", identity_a != identity_b, groupId=group)

        phase = "real-sends-and-raw-sync"
        message_a = one(a.command("send " + group + " CLI-sync-own-" + suffix))["messageId"]
        message_b = one(b.command("send " + group + " CLI-sync-peer-" + suffix))["messageId"]
        state_a, state_b = sync(a), sync(b)
        check(
            "two-way-authoritative-snapshot-and-delta",
            rows(store_a, "SELECT count(*) FROM messages WHERE id IN (?,?)", (message_a, message_b))[0][0] == 2
            and rows(store_b, "SELECT count(*) FROM messages WHERE id IN (?,?)", (message_a, message_b))[0][0] == 2,
            alicePosition=state_a.get("position"), bobPosition=state_b.get("position"),
        )

        phase = "definite-media-rejection-and-original-proof"
        rejected = a.command("send-media " + group + " file " + str(uuid.uuid4()) + " definite-media-reject")
        require("Pending original ID:" in rejected, "actual-rejected-original-row")
        pending = rows(store_a, "SELECT client_msg_id,payload,payload_hash,state FROM outbox WHERE state='pending'")
        require(len(pending) == 1, "one-rejected-original-row")
        client_id, original_payload, original_hash, original_state = pending[0]
        original = json.loads(original_payload)
        control = api("POST", "/api/auth/login", {
            "username": user_b, "password": password, "requestId": str(uuid.uuid4()),
            "deviceName": "CLI owned peer collision fixture",
        })
        controls.append(control)
        peer_conversation = api("GET", "/api/conversations/" + group, token=control["token"])
        collision = api("POST", "/api/conversations/" + group + "/messages", {
            "clientMsgId": client_id, "type": "text", "text": "owned-peer-same-cid-" + suffix,
            "restoreEpoch": epoch, "sendAdmission": peer_conversation["sendAdmission"],
        }, control["token"])
        sync(a)
        after = rows(store_a, "SELECT payload,payload_hash,state FROM outbox WHERE client_msg_id=?", (client_id,))[0]
        check(
            "peer-real-same-client-id-never-acks-own-original",
            collision["senderId"] == identity_b and collision["clientMsgId"] == client_id
            and after == (original_payload, original_hash, original_state),
            clientMsgId=client_id, originalBodySHA256=hashlib.sha256(original_payload.encode()).hexdigest(),
            actualRejection="definite missing-media response, not unknown submission",
        )
        phase = "global-with-unsubmitted-original"
        one(b.command("retract " + message_b))
        sync(a)
        after = rows(store_a, "SELECT payload,payload_hash,state FROM outbox WHERE client_msg_id=?", (client_id,))[0]
        check(
            "verified-global-clears-body-without-consuming-other-original",
            rows(store_a, "SELECT count(*) FROM sync_terminal WHERE id=?", (message_b,))[0][0] == 1
            and not rows(store_a, "SELECT payload FROM messages WHERE id=?", (message_b,))
            and after == (original_payload, original_hash, original_state),
        )

        phase = "old-device-cleanup"
        old_store = store_a
        check("first-alice-tty-normal-quit", a.finish())
        a2, _ = login("Alice", user_a, password, "login")
        store_a2 = db_path(data_a)
        require(store_a2 != old_store, "new-device-new-namespace")
        identity_old = json.loads(rows(old_store, "SELECT value FROM metadata WHERE key='identity'")[0][0])
        identity_new = json.loads(rows(store_a2, "SELECT value FROM metadata WHERE key='identity'")[0][0])
        physical_old = rows(old_store, "SELECT value FROM metadata WHERE key='sync-physical-generation'")[0][0]
        physical_new = rows(store_a2, "SELECT value FROM metadata WHERE key='sync-physical-generation'")[0][0]
        one(b.command("retract " + collision["id"]))
        sync(a2)
        old = rows(old_store, "SELECT payload,payload_hash,state FROM outbox WHERE client_msg_id=?", (client_id,))[0]
        check(
            "same-profile-new-device-isolates-queue-and-cleans-old-holder",
            identity_old["deviceId"] != identity_new["deviceId"]
            and identity_old["storeGeneration"] == identity_new["storeGeneration"]
            and physical_old != physical_new
            and rows(old_store, "SELECT count(*) FROM sync_terminal WHERE id=?", (collision["id"],))[0][0] == 1
            and not rows(old_store, "SELECT payload FROM messages WHERE id=?", (collision["id"],))
            and old[:2] == (original_payload, original_hash) and old[2] == "frozen"
            and not rows(store_a2, "SELECT client_msg_id FROM outbox"),
            namespaceChanged=True, oldOriginalPreserved=True,
        )

        phase = "removed-member-local-retention-and-global"
        member = next(item for item in one(a2.command("members " + group))["items"] if item["id"] == identity_b)
        one(a2.command("remove-member " + group + " " + identity_b + " " + member["membershipGeneration"]))
        sync(b)
        local = one(b.command("local-history " + group))
        check(
            "removed-member-legal-ordinary-local-history-only",
            local.get("cloudPermission") is False
            and any(item["id"] == message_a for item in local["items"]),
        )
        one(a2.command("retract " + message_a))
        sync(b)
        local = one(b.command("local-history " + group))
        check(
            "global-clears-historical-holder-after-cloud-membership-revoked",
            all(item["id"] != message_a for item in local["items"])
            and rows(store_b, "SELECT count(*) FROM sync_terminal WHERE id=?", (message_a,))[0][0] == 1
            and not rows(store_b, "SELECT payload FROM sync_retained WHERE id=?", (message_a,)),
        )
        end_boot = api("GET", "/api/bootstrap")
        check("fixed-instance-and-epoch-unchanged", end_boot["deploymentId"] == deployment and end_boot["restoreEpoch"] == epoch)
        result["status"] = "passed"
    except CheckFailure as error:
        result.update({"status": "failed", "failedPhase": phase, "failure": str(error)})
        print("FAIL " + phase + ": " + str(error), flush=True)
    except Exception as error:
        result.update({"status": "failed", "failedPhase": phase, "failure": type(error).__name__, "detailsRedacted": True})
        print("FAIL " + phase + ": " + type(error).__name__ + " (details redacted)", flush=True)
    finally:
        cleanup = []
        for control in controls:
            try:
                api("POST", "/api/auth/logout", {}, control["token"])
                cleanup.append({"kind": "owned-peer-control-family", "logout": True})
            except Exception:
                cleanup.append({"kind": "owned-peer-control-family", "logout": False})
        for console in consoles:
            try:
                cleanup.append({"kind": "own-tty", "normalQuit": console.finish()})
            except Exception:
                cleanup.append({"kind": "own-tty", "normalQuit": False})
        result.update({"finishedAt": datetime.now(timezone.utc).isoformat(), "cleanup": cleanup})
        save()
    return 0 if result["status"] == "passed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
