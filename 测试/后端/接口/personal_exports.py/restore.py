#!/usr/bin/env python3
"""Bounded integrity and encrypted restore checks in the existing owned formal realm."""

import argparse
from datetime import datetime, timezone
import fcntl
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import secrets
import signal
import ssl
import subprocess
import time
import urllib.parse
import urllib.request
import uuid


SPEC = importlib.util.spec_from_file_location("personal_export_suite", Path(__file__).with_name("personal_exports.py"))
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)
REALM = MODULE.REALM


def child_environment(info):
    child = {key: value for key, value in os.environ.items()
             if not key.startswith(("PG", "JOBS_IM_")) and key not in ("DATABASE_URL", "JOURNAL_DATABASE_URL")}
    child.update(PGPASSFILE=info["pgPassFile"], GOMAXPROCS="2", JOBS_IM_DATABASE_URL=info["businessURL"],
                 JOBS_IM_JOURNAL_DATABASE_URL=info["journalURL"], JOBS_IM_PUBLIC_URL=info["serverURL"],
                 JOBS_IM_NATS_URL=info["natsURL"], JOBS_IM_NATS_TOKEN_FILE=info["natsTokenFile"])
    return child


def run_command(info, label, arguments, extra=None):
    log = Path(info["runDir"]) / "Run" / (label + ".log")
    REALM.command(arguments, Path(info["runDir"]), log, child_environment(info) | (extra or {}))
    return {"step": label, "logSHA256": hashlib.sha256(log.read_bytes()).hexdigest(), "exitCode": 0}


def stop_owned(info):
    actual = subprocess.run(["/bin/ps", "-p", str(info["serverPID"]), "-o", "command="],
                            capture_output=True, text=True, timeout=5, check=True).stdout.strip()
    if actual != " ".join(info["serverCommand"]) or hashlib.sha256(Path(info["binary"]).read_bytes()).hexdigest() != info["binarySHA256"]:
        raise AssertionError("Exact owned candidate PID/argv/binary mismatch; no signal sent")
    os.kill(info["serverPID"], signal.SIGTERM)
    for _ in range(150):
        state = subprocess.run(["/bin/ps", "-p", str(info["serverPID"]), "-o", "stat="], capture_output=True, text=True, timeout=5).stdout.strip()
        if not state or state.startswith("Z"):
            return
        time.sleep(.05)
    raise AssertionError("Owned candidate stop timed out; no force signal sent")


def integrity(suite, task, progress, save):
    actor, info = suite.fixture["peer"], suite.environment
    job = task["job"]["id"]
    row = json.loads(suite.sql("SELECT json_build_object('prefix',storage_prefix,'artifact',id::text) FROM governance.export_artifacts WHERE job_id='" + job + "'"))
    artifact = Path(info["stateDir"]) / "Exports/Artifacts" / row["prefix"] / "package.jimexport"
    if not artifact.is_file():
        # Keep the runtime layout explicit rather than falling back to another realm.
        matches = list(Path(info["stateDir"]).rglob("package.jimexport"))
        matches = [path for path in matches if row["prefix"] in str(path)]
        if len(matches) != 1:
            raise AssertionError("Exact current formal artifact path missing")
        artifact = matches[0]
    original = Path(info["runDir"]) / "Secrets" / ("integrity-original-" + row["artifact"])
    if not original.exists():
        original.write_bytes(artifact.read_bytes())
        os.chmod(original, 0o600)
    expected = hashlib.sha256(original.read_bytes()).hexdigest()
    if hashlib.sha256(artifact.read_bytes()).hexdigest() != expected:
        raise AssertionError("Prior artifact integrity fixture requires explicit reconciliation")
    lock_path = Path(info["stateDir"]) / "Exports/Locks" / (row["artifact"] + ".lock")
    if not lock_path.exists():
        paths = [path for path in Path(info["stateDir"]).rglob("*") if path.is_file() and row["artifact"] in path.name and path != artifact and path != original]
        if len(paths) != 1:
            raise AssertionError("Exact artifact fence path not found")
        lock_path = paths[0]
    progress["integrityFixture"] = {"artifact": str(artifact), "original": str(original), "originalSHA256": expected}
    save()
    def replace(raw):
        with lock_path.open("r+b") as lock:
            fcntl.flock(lock.fileno(), fcntl.LOCK_EX)
            with artifact.open("wb") as output:
                output.write(raw)
                output.flush()
                os.fsync(output.fileno())
    for kind in ["corrupt", "truncate"]:
        ticket = suite.call("POST", "/api/me/data-exports/" + job + "/download-ticket", {}, actor)
        raw = original.read_bytes()
        damaged = raw[:-13] if kind == "truncate" else raw[:len(raw)//2] + bytes([raw[len(raw)//2] ^ 1]) + raw[len(raw)//2+1:]
        try:
            replace(damaged)
            body, headers = suite.binary(actor, "/api/me/data-exports/" + job + "/download", 409, {"X-JobsIM-Download-Token": ticket["downloadToken"]})
            suite.check(not body.startswith(b"PK") and headers.get("Content-Type", "").startswith("application/json") and json.loads(body)["error"]["code"] == "EXPORT_INTEGRITY_FAILED", "real " + kind + "ed encrypted artifact emits zero ZIP plaintext")
        finally:
            replace(original.read_bytes())
    ticket = suite.call("POST", "/api/me/data-exports/" + job + "/download-ticket", {}, actor)
    suite.sql("UPDATE governance.export_artifacts SET download_expires_at=now()-interval '1 second' WHERE job_id='" + job + "'")
    body, _ = suite.binary(actor, "/api/me/data-exports/" + job + "/download", 403, {"X-JobsIM-Download-Token": ticket["downloadToken"]})
    suite.check(json.loads(body)["error"]["code"] == "EXPORT_DOWNLOAD_AUTH_REQUIRED" and not body.startswith(b"PK"), "explicit owned SQL ticket-expiry fixture denies old grant without plaintext")
    package, raw = suite.download(actor, job)
    suite.check(package.testzip() is None and raw.startswith(b"PK"), "restored exact encrypted artifact remains fully authenticated and usable")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--environment-file", type=Path, required=True)
    parser.add_argument("--report", type=Path, required=True)
    args = parser.parse_args()
    info = json.loads(args.environment_file.read_text())
    run = Path(info["runDir"])
    REALM.free_space(run)
    suite = MODULE.PersonalExportSuite(info, args.environment_file)
    progress_path = run / "Secrets/formal-restore-progress.json"
    progress = json.loads(progress_path.read_text()) if progress_path.exists() else {"stage": "preparing", "operationId": "formal-restore-" + secrets.token_hex(10), "steps": []}
    def save():
        suite.save()
        REALM.private_json(progress_path, progress)
    save()
    target = None
    failure = None
    try:
        if progress["stage"] == "preparing":
            suite.refresh_expired_owned_actors()
            peer = suite.fixture["peer"]
            conversation = suite.fixture["conversation"]
            source = suite.message(peer, suite.fixture["source"]["id"])
            draft = suite.call("GET", "/api/conversations/" + conversation + "/draft", session=peer)
            body = {"requestId": str(uuid.uuid4()), "revisionEpoch": suite.epoch, "expectedVersion": draft["version"], "draftAdmission": draft["draftAdmission"], "ownedText": "PEER_INDEPENDENT_FORMAL_RESTORE_TEXT", "reply": {"messageId": source["id"], "sourceVersion": source["contentVersion"], "replyAdmission": source["replyAdmission"]}}
            progress["draftBody"] = body
            progress["unsubmitted"] = suite.original()
            save()
            suite.call("PUT", "/api/conversations/" + conversation + "/draft", body, peer)
            task = suite.named_task("restoreReady", peer)
            progress["job"] = task["job"]["id"]
            save()
            integrity(suite, task, progress, save)
            progress["ticket"] = suite.call("POST", "/api/me/data-exports/" + progress["job"] + "/download-ticket", {}, peer)
            progress["stage"] = "integrity_verified"
            save()
        if progress["stage"] == "integrity_verified":
            backup_dir = Path(info["backupDir"])
            backup_dir.mkdir(parents=True, exist_ok=True, mode=0o700)
            backup = backup_dir / "formal-before-source-revocation.jimbackup"
            key = run / "Secrets/formal-recovery.key"
            progress.update(backup=str(backup), key=str(key))
            save()
            progress["steps"].append(run_command(info, "formal-encrypted-backup", [info["opsBinary"], "--mode", "backup", "--pg-bin", info["pgBin"], "--state-dir", info["stateDir"], "--tls-dir", str(run / "TLS"), "--file", str(backup), "--key-file", str(key)]))
            progress["stage"] = "backup_created"
            save()
        if progress["stage"] == "backup_created":
            suite.refresh_expired_owned_actors()
            peer, owner, conversation = suite.fixture["peer"], suite.fixture["owner"], suite.fixture["conversation"]
            suite.call("DELETE", "/api/messages/" + suite.fixture["source"]["id"], session=peer)
            draft = suite.call("GET", "/api/conversations/" + conversation + "/draft", session=peer)
            suite.check(draft["ownedText"] == progress["draftBody"]["ownedText"] and draft["replyState"] == "unavailable" and draft["reply"] is None, "real source global recall clears reply authority while retaining independently authored draft text")
            suite.check(suite.status(peer, progress["job"])["job"]["cleanupState"] == "cleared", "source global recall drains the original ready personal artifact before acknowledging success")
            members = suite.call("GET", "/api/conversations/" + conversation + "/members", session=owner)
            items = members.get("items", members) if isinstance(members, dict) else members
            member = next(item for item in items if item["id"] == peer["user"]["id"])
            suite.call("DELETE", "/api/conversations/" + conversation + "/members/" + peer["user"]["id"] + "?membershipGeneration=" + member["membershipGeneration"], session=owner)
            suite.call("POST", "/api/conversations/" + conversation + "/members", {"userId": peer["user"]["id"]}, owner)
            suite.call("POST", "/api/me/data-exports/" + progress["job"] + "/download-ticket", {}, peer, 409)
            suite.check(suite.sql("SELECT key_ciphertext IS NULL FROM governance.export_jobs WHERE id='" + progress["job"] + "'") == "t", "member exit and re-entry never restore the old export key or source qualification")
            progress["stage"] = "post_backup_facts_committed"
            save()
        if progress["stage"] == "post_backup_facts_committed":
            stop_owned(info)
            progress["stage"] = "source_stopped"
            save()
        if progress["stage"] == "source_stopped":
            name = info["databasePrefix"] + "_restored_" + secrets.token_hex(4)
            progress.update(targetDatabase=name, targetURL=info["businessURL"].replace("/" + info["businessDatabase"] + "?", "/" + name + "?"), targetState=str(run / "Restored/State"))
            save()
            REALM.sql(info, False, 'CREATE DATABASE "' + name + '" OWNER "' + info["mainUsername"] + '" TEMPLATE template0;', True)
            progress["stage"] = "target_created"
            save()
        if progress["stage"] == "target_created":
            progress["steps"].append(run_command(info, "formal-offline-restore", [info["opsBinary"], "--mode", "restore", "--pg-bin", info["pgBin"], "--state-dir", info["stateDir"], "--target-state-dir", progress["targetState"], "--tls-dir", str(run / "TLS"), "--file", progress["backup"], "--key-file", progress["key"], "--operation-id", progress["operationId"], "--offline"], {"JOBS_IM_RESTORE_DATABASE_URL": progress["targetURL"]}))
            progress["stage"] = "restore_completed"
            save()
        if progress["stage"] == "restore_completed":
            state = Path(progress["targetState"])
            target_info = dict(info, businessURL=progress["targetURL"], businessDatabase=progress["targetDatabase"], stateDir=str(state), tlsCert=str(state / "TLS/server.pem"), tlsKey=str(state / "TLS/server.key"), caPath=str(state / "TLS/ca.pem"), restoreEpoch="2", restoredFormalExportRealm=True)
            argv = list(info["serverCommand"])
            for flag, value in [("--state-dir", target_info["stateDir"]), ("--tls-cert", target_info["tlsCert"]), ("--tls-key", target_info["tlsKey"])]:
                argv[argv.index(flag) + 1] = value
            pid = REALM.start_process(argv, run, run / "Run/formal-restored-app.log", child_environment(target_info))
            target_info.update(serverPID=pid, serverCommand=argv)
            target_seed = run / "restored-environment.json"
            REALM.private_json(target_seed, target_info)
            progress["targetSeed"] = str(target_seed)
            progress["stage"] = "target_started"
            save()
        target_info = json.loads(Path(progress["targetSeed"]).read_text())
        target = MODULE.PersonalExportSuite(target_info, progress["targetSeed"])
        target.epoch = "2"
        for _ in range(100):
            try:
                bootstrap = target.call("GET", "/api/bootstrap")
                break
            except (OSError, ValueError):
                time.sleep(.1)
        else:
            raise AssertionError("Restored strict HTTPS readiness timed out")
        target.check(bootstrap["deploymentId"] == info["deploymentId"] and bootstrap["restoreEpoch"] == "2", "actual encrypted restore preserves deployment identity and advances authoritative epoch once")
        old_peer = suite.fixture["peer"]
        target.call("GET", "/api/me", session=old_peer, expect=401)
        peer = target.login(old_peer)
        target.fixture["peer"] = peer
        target.save()
        original = target.result(peer, suite.fixture["restoreReadyBody"])
        target.check(original["job"]["id"] == progress["job"] and original["job"]["state"] == "cancelled" and original["job"]["cleanupState"] == "cleared", "old full create body remains metadata-only and cannot restore completed delivery")
        target.call("POST", "/api/me/data-exports/" + progress["job"] + "/download-ticket", {}, peer, 409)
        target.binary(peer, "/api/me/data-exports/" + progress["job"] + "/download", 409, {"X-JobsIM-Download-Token": progress["ticket"]["downloadToken"]})
        rejected = target.create(peer, progress["unsubmitted"], 409)
        target.check(rejected["error"]["code"] == "RESTORE_EPOCH_CHANGED", "old unsubmitted export intent remains rejected without epoch or nonce repair")
        target.check(target.sql("SELECT count(*) FROM governance.export_jobs WHERE restore_epoch<2 AND key_ciphertext IS NOT NULL") == "0" and target.sql("SELECT count(*) FROM governance.export_artifacts WHERE restore_epoch<2 AND (download_token_digest IS NOT NULL OR state<>'cleared')") == "0", "restore retires every old sealed export key, ticket and artifact state")
        pending = sum(int(target.sql("SELECT count(*) FROM " + owner + ".personal_export_holders WHERE state<>'cleared'")) for owner in ["auth", "directory", "conversation", "sync"])
        target.check(pending == 0 and target.sql("SELECT count(*) FROM media.personal_export_avatar_holders WHERE state<>'cleared'") == "0", "no old prepared or active metadata/avatar holder regains authority from the backup")
        draft = target.call("GET", "/api/conversations/" + suite.fixture["conversation"] + "/draft", session=peer)
        target.check(draft["ownedText"] == progress["draftBody"]["ownedText"] and draft["reply"] is None and draft["replyState"] == "unavailable", "journal replay retains matching own draft ciphertext but never resurrects the globally recalled source reference")
        progress["stage"] = "verified"
        REALM.private_json(args.environment_file.with_name("source-retired-environment.json"), dict(info, status="retired_after_formal_restore"))
        save()
    except Exception as error:
        failure = str(error)
        save()
    result = {"status": "passed" if failure is None else "failed", "failure": failure, "lastCompletedStage": progress["stage"], "httpChecks": suite.checks + (target.checks if target else 0), "semanticAssertions": suite.assertions + (target.assertions if target else 0), "binarySHA256": info["binarySHA256"], "operatorBinarySHA256": info["opsBinarySHA256"], "scriptSHA256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(), "privateProgressPreserved": True, "checkedAt": datetime.now(timezone.utc).isoformat(), "assertions": suite.transcript + (target.transcript if target else [])}
    if Path(progress.get("backup", "missing")).is_file():
        backup = Path(progress["backup"])
        result.update(backupSHA256=hashlib.sha256(backup.read_bytes()).hexdigest(), backupBytes=backup.stat().st_size)
    args.report.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({key: value for key, value in result.items() if key != "assertions"}, ensure_ascii=False))
    if failure:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
