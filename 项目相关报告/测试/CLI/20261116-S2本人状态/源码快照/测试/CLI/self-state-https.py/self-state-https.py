#!/usr/bin/env python3
"""Strict-CA CLI schema2 TTY acceptance: only new users and private local state."""
# Created by Jobs on 2026年11月16日，星期一.
import argparse
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import secrets
import shutil
import sqlite3
import ssl
import sys
import time
import urllib.error
import urllib.request
import uuid

HERE = Path(__file__).resolve().parent
sys.dont_write_bytecode = True
SPEC = importlib.util.spec_from_file_location(
    "jobsim_cli_tty", HERE.parent / "sync-https.py" / "sync-https.py"
)
TTY = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(TTY)
require = TTY.require


def one(text):
    values = TTY.objects(text)
    require(len(values) == 1, "one-public-json-result")
    return values[0]


def local_rows(file, sql, values=()):
    with sqlite3.connect(file.as_uri() + "?mode=ro", uri=True) as db:
        return db.execute(sql, values).fetchall()


def auth_state(data):
    files = list(data.rglob("authentication.sqlite"))
    require(len(files) == 1, "one-private-authentication-state")
    return json.loads(local_rows(files[0], "SELECT payload FROM authentication_state")[0][0])


def message_db(data):
    files = list(data.rglob("client.sqlite"))
    require(len(files) == 1, "one-current-message-store")
    return files[0]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--environment-file", type=Path, required=True)
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--data-root", type=Path, required=True)
    parser.add_argument("--report-dir", type=Path, required=True)
    args = parser.parse_args()
    info = json.loads(args.environment_file.read_text())
    base = info["serverURL"]
    require(base == "https://127.0.0.1:54443", "owned-e2-origin-required")
    require(info.get("restoredDraftRealm") is True and info.get("cloudDraftRealm") is True, "owned-restored-draft-realm-required")
    require(not args.data_root.exists(), "new-private-data-root-required")
    require(not args.report_dir.exists(), "new-report-required")
    require(shutil.disk_usage(args.data_root.parent).free >= 4 * 1024**3, "disk-floor")
    os.umask(0o077)
    args.data_root.mkdir(mode=0o700, parents=True)
    args.report_dir.mkdir(mode=0o700, parents=True)
    trust = ssl.create_default_context(cafile=info["caPath"])
    class NoRedirect(urllib.request.HTTPRedirectHandler):
        def redirect_request(self, request, file_pointer, code, message, headers, new_url):
            return None
    opener = urllib.request.build_opener(
        urllib.request.ProxyHandler({}), NoRedirect(),
        urllib.request.HTTPSHandler(context=trust),
    )
    result = {
        "startedAt": datetime.now(timezone.utc).isoformat(), "status": "running",
        "checks": [], "cleanup": [], "rawTTYRecorded": False,
        "nativeCredentialStorePositive": False, "processOnlyExplicitPerPTY": True,
        "pgUsed": False, "serviceChanged": False, "cliHTTPCountMeasured": False,
        "unknownResponseInjected": False,
        "binarySHA256": hashlib.sha256(args.binary.read_bytes()).hexdigest(),
    }
    consoles, controls = [], []
    phase = "bootstrap"

    def save():
        (args.report_dir / "result.json").write_text(
            json.dumps(result, ensure_ascii=False, indent=2) + "\n"
        )

    def check(name, condition, **facts):
        require(condition, name)
        result["checks"].append({"name": name, "passed": True, **facts})
        save()
        print("PASS " + name, flush=True)

    def api(method, route, body=None, token=None, expected=(200, 201)):
        require(shutil.disk_usage(args.data_root).free >= 4 * 1024**3, "disk-floor")
        headers = {"Content-Type": "application/json"}
        if token:
            headers["Authorization"] = "Bearer " + token
        request = urllib.request.Request(
            base + route, data=None if body is None else json.dumps(body).encode(),
            method=method, headers=headers,
        )
        try:
            with opener.open(request, timeout=20) as response:
                raw, status = response.read(), response.status
        except urllib.error.HTTPError as error:
            status, raw = error.code, error.read()
        require(status in expected if isinstance(expected, tuple) else status == expected, "fixture-http-" + method + "-" + str(status))
        return json.loads(raw) if raw else {}

    def login(label, username, password):
        data = args.data_root / label
        console = TTY.Console(
            [str(args.binary.resolve()), "--server", base, "--ca", info["caPath"],
             "--data-dir", str(data), "register"], args.data_root,
        )
        consoles.append({"console": console, "data": data, "label": label})
        console.until("Username: ")
        console.send(username)
        console.until("Type yes: ")
        console.send("yes")
        console.until("Password: ")
        console.send(password)
        output = console.until("jobsim> ")
        require("Explicit temporary session" in output, "explicit-process-only")
        require("Authoritative synchronization is restricted" not in output, "initial-s2-ready")
        state = auth_state(data)
        return console, data, state

    def sync(console):
        value = one(console.command("sync"))
        require(value.get("schemaVersion") == "2" and value.get("restriction", "") == "", "s2-ready")
        require(value.get("qualifiedAck") is False, "sync-not-qualified-ack")
        return value

    def applied(console, command):
        value = one(console.command(command))
        for _ in range(5):
            if value.get("status") != "prepared":
                break
            value = one(console.command("resume-self " + value.get("requestId", value["operationId"])))
        require(value.get("status") == "applied", "original-operation-applied")
        return value

    try:
        boot = api("GET", "/api/bootstrap")
        require(boot["restoreEpoch"] == info["restoreEpoch"] == "2", "restored-e2-epoch-required")
        require(boot["deploymentId"] == info["deploymentId"] == "24fb0a3c-99a0-4ada-9d01-07467724e829", "owned-e2-deployment-required")
        require(all(boot["features"].get(name) is True for name in ["authoritative_sync", "sync_jetstream", "sync_self_state", "cloud_drafts", "auth_refresh"]), "real-s2-capabilities")
        result.update({"origin": base, "deploymentId": boot["deploymentId"], "restoreEpoch": boot["restoreEpoch"]})
        suffix = secrets.token_hex(5)
        names = ["cli_s2_a_" + suffix, "cli_s2_b_" + suffix]
        passwords = ["Fixture-long-" + secrets.token_urlsafe(24) for _ in range(2)]
        a, data_a, state_a = login("A", names[0], passwords[0])
        b, data_b, state_b = login("B", names[1], passwords[1])
        for index, name in enumerate(names):
            control = api("POST", "/api/auth/login", {"username": name, "password": passwords[index], "requestId": str(uuid.uuid4()), "deviceName": "CLI S2 owned control"})
            controls.append(control)
        ta, tb = controls[0]["token"], controls[1]["token"]
        check("strict-ca-two-real-tty-process-only", state_a["userId"] != state_b["userId"])

        phase = "relationship-fixture"
        request = api("POST", "/api/contacts/requests", {"userId": state_b["userId"], "requestId": str(uuid.uuid4())}, ta)
        api("POST", "/api/contacts/requests/" + request["id"] + "/accept", {"requestId": str(uuid.uuid4())}, tb)
        group = api("POST", "/api/conversations", {"kind": "group", "title": "CLI S2 isolated", "memberIds": [state_b["userId"]], "requestId": str(uuid.uuid4())}, ta)["id"]
        sync(a)
        sync(b)
        database = message_db(data_a)
        check("automatic-s2-physical-store-keeps-legacy-identity", one(a.command("sync-status"))["schemaVersion"] == "2" and len(local_rows(database, "SELECT value FROM metadata WHERE key='sync-physical-generation'")[0][0]) == 36)

        phase = "font-view"
        applied(a, "font large")
        preference = api("GET", "/api/me/preferences", token=ta)
        peer_preference = api("GET", "/api/me/preferences", token=tb)
        check("font-cas-shared-only-with-own-family", preference["fontPreset"] == "large" and peer_preference["fontPreset"] == "standard")
        applied(a, "view " + group + ' {"hidden":true,"pinnedRank":1,"manualUnread":true}')
        hidden, all_rows = one(a.command("conversations")), one(a.command("conversations all"))
        check("hidden-view-all-restoration-entry", not any(row["id"] == group for row in hidden["items"]) and any(row["id"] == group for row in all_rows["items"]))
        applied(a, "view " + group + ' {"hidden":false}')

        phase = "real-observation-holes"
        conversation = api("GET", "/api/conversations/" + group, token=tb)
        incoming = []
        for index in range(2):
            incoming.append(api("POST", "/api/conversations/" + group + "/messages", {"clientMsgId": secrets.token_hex(16), "type": "text", "text": "isolated observation " + str(index), "restoreEpoch": boot["restoreEpoch"], "sendAdmission": conversation["sendAdmission"]}, tb))
        sync(a)
        a.command("history " + group)
        before = api("GET", "/api/conversations/" + group + "/read", token=ta)
        check("history-is-not-read-observation", before["unreadCount"] == "2")
        applied(a, "read " + group + " " + incoming[1]["id"])
        hole = api("GET", "/api/conversations/" + group + "/read", token=ta)
        applied(a, "read " + group + " " + incoming[0]["id"])
        after = one(a.command("read " + group))
        view = api("GET", "/api/conversations/" + group + "/view", token=ta)
        check("explicit-observation-fills-hole-without-clearing-manual-unread", hole["unreadCount"] == "1" and after["unreadCount"] == "0" and view["manualUnread"] is True)

        phase = "owned-text-and-local-conflict"
        owned = "本人独立草稿-" + suffix
        operation = applied(a, "cloud-draft-set " + group + " |" + owned)
        current = one(a.command("cloud-draft " + group))
        check("text-draft-explicit-write-and-private-projection", current["ownedText"] == owned and "draftAdmission" not in current)
        a.command("draft " + group + " local-independent-input")
        cloud = api("GET", "/api/conversations/" + group + "/draft", token=ta)
        update = api("PUT", "/api/conversations/" + group + "/draft", {"requestId": str(uuid.uuid4()), "revisionEpoch": boot["restoreEpoch"], "expectedVersion": cloud["version"], "draftAdmission": cloud["draftAdmission"], "ownedText": "remote-newer-independent", "reply": None}, ta)
        require(update["status"] == "applied", "control-draft-applied")
        sync(a)
        check("remote-draft-does-not-overwrite-local-input", one(a.command("drafts"))[group] == "local-independent-input")
        cloud = api("GET", "/api/conversations/" + group + "/draft", token=ta)
        stale = a.command("cloud-draft-adopt " + group + " " + cloud["version"] + " |stale-local")
        require("local draft changed" in stale, "local-draft-cas-rejected")
        a.command("cloud-draft-adopt " + group + " " + cloud["version"] + " |local-independent-input")
        check("explicit-adoption-uses-local-original-text-cas", one(a.command("drafts"))[group] == "remote-newer-independent")
        repeated = applied(a, "resume-self " + operation["requestId"])
        cloud_after = api("GET", "/api/conversations/" + group + "/draft", token=ta)
        check("completed-original-result-does-not-revert-current-draft", repeated["operationId"] == operation["operationId"] and repeated["current"] is False and cloud_after["ownedText"] == "remote-newer-independent")

        phase = "reply-invalidation"
        independent = "独立回复草稿-" + suffix
        applied(a, "cloud-draft-reply " + group + " " + incoming[0]["id"] + " " + incoming[0]["contentVersion"] + " |" + independent)
        api("DELETE", "/api/messages/" + incoming[0]["id"], token=tb)
        sync(a)
        draft = one(a.command("cloud-draft " + group))
        check("source-global-removes-reply-only-keeps-owned-text", draft["ownedText"] == independent and draft["replyState"] != "available" and draft["reply"] is None)
        applied(a, "cloud-draft-clear " + group)
        cleared = one(a.command("cloud-draft " + group))
        check("explicit-cloud-clear-preserves-local-input", cleared["contentState"] == "empty" and one(a.command("drafts"))[group] == "remote-newer-independent")
        check("original-operation-bodies-remain-local-and-stable", local_rows(database, "SELECT count(*) FROM self_operations WHERE state='completed'")[0][0] >= 6)
        result["status"] = "passed"
    except TTY.CheckFailure as error:
        result.update({"status": "failed", "phase": phase, "failure": str(error)})
    except Exception as error:
        result.update({"status": "failed", "phase": phase, "failureClass": type(error).__name__})
    finally:
        for index, entry in enumerate(consoles):
            console = entry["console"]
            cleanup = {"kind": "own-tty", "label": entry["label"], "normalExit": False, "logoutSuccessState": False, "deviceAbsentFromActiveSessions": False}
            try:
                console.send("logout")
                console.process.wait(timeout=20)
                cleanup["normalExit"] = console.process.returncode == 0
                state = auth_state(entry["data"])
                cleanup["logoutSuccessState"] = state["status"] == "signed_out"
                if index < len(controls):
                    sessions = api("GET", "/api/auth/sessions", token=controls[index]["token"])
                    cleanup["deviceAbsentFromActiveSessions"] = not any(row["deviceId"] == state["deviceId"] for row in sessions["items"])
            except Exception:
                pass
            finally:
                cleanup["normalExit"] = console.finish() and cleanup["normalExit"]
            result["cleanup"].append(cleanup)
        for control in controls:
            cleanup = {"kind": "owned-control-family", "logoutHTTP200": False, "originalAT401": False}
            try:
                api("POST", "/api/auth/logout", {}, control["token"], expected=200)
                cleanup["logoutHTTP200"] = True
                api("GET", "/api/me", token=control["token"], expected=401)
                cleanup["originalAT401"] = True
            except Exception:
                pass
            result["cleanup"].append(cleanup)
        if not all(row.get("logoutSuccessState", row.get("logoutHTTP200", False)) for row in result["cleanup"]):
            result["cleanupUnconfirmed"] = True
        result["finishedAt"] = datetime.now(timezone.utc).isoformat()
        save()
    return 0 if result["status"] == "passed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
