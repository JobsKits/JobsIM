#!/usr/bin/env python3
"""Isolated strict-CA CLI original-object receipt TTY acceptance; no SQL writes."""
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
import ssl
import sys
import urllib.error
import urllib.request
import uuid

sys.dont_write_bytecode = True
HERE = Path(__file__).resolve().parent
SPEC = importlib.util.spec_from_file_location(
    "jobsim_receipt_tty", HERE.parent / "self-state-https.py" / "self-state-https.py"
)
SUPPORT = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(SUPPORT)
TTY, require, one = SUPPORT.TTY, SUPPORT.require, SUPPORT.one


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--environment-file", type=Path, required=True)
    parser.add_argument("--binary", type=Path, required=True)
    parser.add_argument("--verification-file", type=Path, required=True)
    parser.add_argument("--data-root", type=Path, required=True)
    parser.add_argument("--report-dir", type=Path, required=True)
    args = parser.parse_args()
    for field in ("environment_file", "binary", "verification_file", "data_root", "report_dir"):
        setattr(args, field, getattr(args, field).resolve())
    info = json.loads(args.environment_file.read_text())
    base = info["serverURL"]
    require(base == "https://127.0.0.1:56443", "isolated-object-realm-origin")
    require(not args.data_root.exists(), "new-private-data-root")
    require(not args.report_dir.exists(), "new-report")
    require(shutil.disk_usage(args.data_root.parent).free >= 4 * 1024**3, "disk-floor")
    os.umask(0o077)
    args.data_root.mkdir(mode=0o700, parents=True)
    args.report_dir.mkdir(mode=0o700, parents=True)
    trust = ssl.create_default_context(cafile=info["caPath"])

    class NoRedirect(urllib.request.HTTPRedirectHandler):
        def redirect_request(self, request, fp, code, message, headers, new_url):
            return None

    opener = urllib.request.build_opener(
        urllib.request.ProxyHandler({}), NoRedirect(),
        urllib.request.HTTPSHandler(context=trust),
    )
    result = {
        "startedAt": datetime.now(timezone.utc).isoformat(), "status": "running",
        "checks": [], "cleanup": [], "realHTTPS": True, "rawTTYRecorded": False,
        "pgUsed": False, "sqlWrites": False, "serviceChanged": False,
        "cliHTTPCountMeasured": False, "unknownResponseInjected": False,
        "nativeCredentialStorePositive": False, "processOnlyExplicitPerPTY": True,
        "binarySHA256": hashlib.sha256(args.binary.read_bytes()).hexdigest(),
        "verificationSHA256": hashlib.sha256(args.verification_file.read_bytes()).hexdigest(),
    }
    console, control, data, state = None, None, None, None
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
        if phase != "cleanup":
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
        require(status in expected if isinstance(expected, tuple) else status == expected,
                "fixture-http-" + method + "-" + str(status))
        return json.loads(raw) if raw else {}

    try:
        boot = api("GET", "/api/bootstrap")
        require(boot["deploymentId"] == info["deploymentId"] and
                boot["restoreEpoch"] == info["restoreEpoch"], "same-new-instance")
        require(all(boot["features"].get(key) is True for key in [
            "authoritative_sync", "sync_jetstream", "auth_refresh",
            "durable_receipt_protocol", "durable_object_receipt_protocol",
        ]), "required-real-protocol-flags")
        require(not boot["features"].get("after_receipt", False), "no-cloud-deletion")
        result.update({"origin": base, "deploymentId": boot["deploymentId"],
                       "restoreEpoch": boot["restoreEpoch"]})
        name = "cli_obj_" + secrets.token_hex(6)
        password = "Owned-object-fixture-" + secrets.token_urlsafe(24)
        data = args.data_root / "TTY"
        phase = "tty-register"
        console = TTY.Console([
            str(args.binary.resolve()), "--server", base, "--ca", info["caPath"],
            "--data-dir", str(data), "register",
        ], args.data_root)
        console.until("Username: ")
        console.send(name)
        console.until("Type yes: ")
        console.send("yes")
        console.until("Password: ")
        console.send(password)
        output = console.until("jobsim> ")
        require("Explicit temporary session" in output, "explicit-process-only")
        state = SUPPORT.auth_state(data)
        control = api("POST", "/api/auth/login", {
            "username": name, "password": password, "requestId": str(uuid.uuid4()),
            "deviceName": "CLI object receipts owned control",
        })
        token = control["token"]
        check("strict-ca-real-tty-and-two-isolated-families", control["deviceId"] != state["deviceId"])

        phase = "ordinary-media-fixture"
        group = one(console.command("group CLI original object"))["id"]
        original = args.data_root / "user original file.bin"
        content = b"CLI original object\x00\xff\n" + secrets.token_bytes(257)
        original.write_bytes(content)
        upload = one(console.command("upload " + str(original)))
        obj = upload["id"]
        conv = api("GET", "/api/conversations/" + group, token=token)
        message = api("POST", "/api/conversations/" + group + "/messages", {
            "clientMsgId": secrets.token_hex(16), "type": "file", "text": "object fixture",
            "attachmentId": obj, "restoreEpoch": boot["restoreEpoch"],
            "sendAdmission": conv["sendAdmission"],
        }, token)
        sync = one(console.command("sync"))
        check("real-upload-and-authoritative-message-descriptor", sync["restriction"] == "" and message["attachmentId"] == obj)
        database = SUPPORT.message_db(data)
        status = one(console.command("storage-status"))
        check("registration-is-not-storage-qualification", status["storage"]["qualified"] is False and status["storage"].get("objectQualified", False) is False)

        phase = "approved-storage"
        declared = one(console.command("storage-declare cli-object-full-v1 " + result["binarySHA256"] + " " + result["verificationSHA256"]))
        storage = declared["storage"]
        check("exact-approved-file-storage-contract", storage["qualified"] is True and storage["objectQualified"] is True and storage["storageContract"] == "native_sqlite_full_file_fsync_v1")
        rejected = console.command("receive-text " + message["id"])
        check("strict-text-command-does-not-ack-media", "RECEIPT_TEXT_ONLY" in rejected)
        parent = one(console.command("receive-message " + message["id"]))
        check("complete-ordinary-descriptor-parent-recorded", parent["status"] == "recorded" and parent["receiptKind"] == "message_content" and parent["objectComplete"] is False)

        phase = "complete-original-object"
        receipt = one(console.command("receive-object " + parent["operationId"]))
        file_status = one(console.command("receipt-file-status " + receipt["operationId"]))
        check("whole-authenticated-original-downloaded-and-durable", receipt["objectComplete"] is True and receipt["objectDigest"] == hashlib.sha256(content).hexdigest() and receipt["sizeBytes"] == str(len(content)) and file_status["localFileVerified"] is True)
        check("object-receipt-does-not-complete-targets-or-deletion", receipt["targetRangeComplete"] is False and receipt["deletionEligible"] is False and file_status["afterReceiptEnabled"] is False)
        original_result = one(console.command("reconcile-receipt " + receipt["operationId"]))
        check("pure-original-object-result-agrees", original_result == receipt)

        phase = "import-integrity"
        wrong = args.data_root / "wrong original.bin"
        wrong.write_bytes(bytes([content[0] ^ 1]) + content[1:])
        before = SUPPORT.local_rows(database, "SELECT count(*) FROM receipt_objects")[0][0]
        rejected = console.command("receive-object-file " + parent["operationId"] + " " + str(wrong))
        after = SUPPORT.local_rows(database, "SELECT count(*) FROM receipt_objects")[0][0]
        check("wrong-local-original-never-installs-receipt", "RECEIPT_OBJECT_INTEGRITY" in rejected and before == after and wrong.exists())
        imported = one(console.command("receive-object-file " + parent["operationId"] + " " + str(original)))
        check("explicit-import-preserves-user-original", imported["objectComplete"] is True and original.read_bytes() == content and imported["operationId"] != receipt["operationId"])
        rows = SUPPORT.local_rows(database, "SELECT descriptor FROM receipt_objects WHERE epoch=?", (boot["restoreEpoch"],))
        descriptors = [json.loads(row[0]) for row in rows]
        require(len(descriptors) == 2, "two-managed-object-copies")
        managed = [database.parent / descriptor["relativePath"] for descriptor in descriptors]
        check("sqlite-only-retains-relative-object-metadata", all(not Path(d["relativePath"]).is_absolute() and d["sha256"] == hashlib.sha256(content).hexdigest() for d in descriptors) and all(p.read_bytes() == content for p in managed))

        phase = "global-lifecycle"
        api("DELETE", "/api/messages/" + message["id"], token=token)
        one(console.command("sync"))
        check("global-clears-all-managed-holders-not-user-original", all(not p.exists() for p in managed) and original.read_bytes() == content and SUPPORT.local_rows(database, "SELECT count(*) FROM receipt_object_cleanup")[0][0] == 0)
        historical = one(console.command("reconcile-receipt " + receipt["operationId"]))
        local = one(console.command("receipt-file-status " + receipt["operationId"]))
        check("historical-recorded-result-cannot-recreate-global-file", historical == receipt and local["localFileVerified"] is False and all(not p.exists() for p in managed))
        result["status"] = "passed"
    except TTY.CheckFailure as error:
        result.update({"status": "failed", "phase": phase, "failure": str(error)})
    except Exception as error:
        result.update({"status": "failed", "phase": phase, "failureClass": type(error).__name__})
    finally:
        phase = "cleanup"
        if console is not None:
            cleanup = {"kind": "own-tty", "normalExit": False,
                       "logoutSuccessState": False, "deviceAbsentFromActiveSessions": False}
            try:
                console.send("logout")
                console.process.wait(timeout=20)
                cleanup["normalExit"] = console.process.returncode == 0
                state = SUPPORT.auth_state(data)
                cleanup["logoutSuccessState"] = state["status"] == "signed_out"
                if control:
                    sessions = api("GET", "/api/auth/sessions", token=control["token"])
                    cleanup["deviceAbsentFromActiveSessions"] = not any(row["deviceId"] == state["deviceId"] for row in sessions["items"])
            except Exception:
                pass
            finally:
                cleanup["normalExit"] = console.finish() and cleanup["normalExit"]
            result["cleanup"].append(cleanup)
        if control:
            cleanup = {"kind": "owned-control-family", "logoutHTTP200": False, "originalAT401": False}
            try:
                api("POST", "/api/auth/logout", {}, control["token"], expected=200)
                cleanup["logoutHTTP200"] = True
                api("GET", "/api/me", token=control["token"], expected=401)
                cleanup["originalAT401"] = True
            except Exception:
                pass
            result["cleanup"].append(cleanup)
        result["finishedAt"] = datetime.now(timezone.utc).isoformat()
        if not all(row.get("logoutSuccessState", row.get("logoutHTTP200", False)) for row in result["cleanup"]):
            result["cleanupUnconfirmed"] = True
        save()
    return 0 if result["status"] == "passed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
