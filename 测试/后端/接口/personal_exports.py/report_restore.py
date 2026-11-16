#!/usr/bin/env python3
"""Real restore of a ready formal report holder in this same owned realm."""

import argparse
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import secrets
import ssl
import time
import urllib.request
import uuid


SPEC = importlib.util.spec_from_file_location("formal_report_restore", Path(__file__).with_name("restore.py"))
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--environment-file", type=Path, required=True)
    parser.add_argument("--report", type=Path, required=True)
    args = parser.parse_args()
    info = json.loads(args.environment_file.read_text())
    if not info.get("restoredFormalExportRealm"):
        raise SystemExit("Existing explicitly owned formal restored namespace required")
    run = Path(info["runDir"])
    MODULE.REALM.free_space(run)
    suite = MODULE.MODULE.PersonalExportSuite(info, args.environment_file)
    path = run / "Secrets/formal-report-restore-progress.json"
    progress = json.loads(path.read_text()) if path.exists() else {"stage": "preparing", "operationId": "formal-report-restore-" + secrets.token_hex(10), "steps": []}
    def save():
        MODULE.REALM.private_json(path, progress)
    save()
    failure = None
    target = None
    try:
        if progress["stage"] == "preparing":
            actor = suite.fixture["reportActor"]
            task = suite.status(actor, suite.fixture["reportAfterReviewJob"])
            suite.check(task["job"]["state"] == "ready" and suite.sql("SELECT count(*) FROM governance.personal_export_holders WHERE job_id='" + task["job"]["id"] + "' AND state='active'") == "1", "actual backup source contains one active ready reporter holder")
            progress["job"] = task["job"]["id"]
            progress["originalBody"] = suite.fixture["reportAfterReviewBody"]
            progress["oldActor"] = actor
            backup_dir = Path(info["backupDir"])
            backup = backup_dir / "formal-ready-report-holder.jimbackup"
            key = run / "Secrets/formal-report-recovery.key"
            progress.update(backup=str(backup), key=str(key))
            save()
            progress["steps"].append(MODULE.run_command(info, "formal-report-encrypted-backup", [info["opsBinary"], "--mode", "backup", "--pg-bin", info["pgBin"], "--state-dir", info["stateDir"], "--tls-dir", str(Path(info["tlsCert"]).parent), "--file", str(backup), "--key-file", str(key)]))
            progress["stage"] = "backup_created"
            save()
        if progress["stage"] == "backup_created":
            MODULE.stop_owned(info)
            progress["stage"] = "source_stopped"
            save()
        if progress["stage"] == "source_stopped":
            name = info["databasePrefix"] + "_reportrestore_" + secrets.token_hex(4)
            progress.update(targetDatabase=name, targetURL=info["businessURL"].replace("/" + info["businessDatabase"] + "?", "/" + name + "?"), targetState=str(run / "ReportRestored/State"))
            save()
            MODULE.REALM.sql(info, False, 'CREATE DATABASE "' + name + '" OWNER "' + info["mainUsername"] + '" TEMPLATE template0;', True)
            progress["stage"] = "target_created"
            save()
        if progress["stage"] == "target_created":
            progress["steps"].append(MODULE.run_command(info, "formal-report-offline-restore", [info["opsBinary"], "--mode", "restore", "--pg-bin", info["pgBin"], "--state-dir", info["stateDir"], "--target-state-dir", progress["targetState"], "--tls-dir", str(Path(info["tlsCert"]).parent), "--file", progress["backup"], "--key-file", progress["key"], "--operation-id", progress["operationId"], "--offline"], {"JOBS_IM_RESTORE_DATABASE_URL": progress["targetURL"]}))
            progress["stage"] = "restore_completed"
            save()
        if progress["stage"] == "restore_completed":
            state = Path(progress["targetState"])
            target_info = dict(info, businessURL=progress["targetURL"], businessDatabase=progress["targetDatabase"], stateDir=str(state), tlsCert=str(state / "TLS/server.pem"), tlsKey=str(state / "TLS/server.key"), caPath=str(state / "TLS/ca.pem"), restoreEpoch=str(int(info["restoreEpoch"]) + 1))
            argv = list(info["serverCommand"])
            for flag, value in [("--state-dir", target_info["stateDir"]), ("--tls-cert", target_info["tlsCert"]), ("--tls-key", target_info["tlsKey"])]:
                argv[argv.index(flag) + 1] = value
            target_info["serverCommand"] = argv
            target_info["serverPID"] = MODULE.REALM.start_process(argv, run, run / "Run/report-restored-app.log", MODULE.child_environment(target_info))
            progress["targetSeed"] = str(run / "report-restored-environment.json")
            MODULE.REALM.private_json(Path(progress["targetSeed"]), target_info)
            progress["stage"] = "target_started"
            save()
        target_info = json.loads(Path(progress["targetSeed"]).read_text())
        target = MODULE.MODULE.PersonalExportSuite(target_info, progress["targetSeed"])
        for _ in range(100):
            try:
                bootstrap = target.call("GET", "/api/bootstrap")
                break
            except OSError:
                time.sleep(.1)
        else:
            raise AssertionError("Restored report-owner application readiness timeout")
        target.check(bootstrap["deploymentId"] == info["deploymentId"] and bootstrap["restoreEpoch"] == target_info["restoreEpoch"], "real second restore advances once and retains the exact deployment")
        target.call("GET", "/api/me", session=progress["oldActor"], expect=401)
        if "loginBody" not in progress:
            actor = progress["oldActor"]
            progress["loginBody"] = {"username": actor["user"]["username"], "password": target.passwords[actor["user"]["id"]], "requestId": str(uuid.uuid4()), "deviceName": "restored-report-holder-test"}
            save()
        actor = target.call("POST", "/api/auth/login", progress["loginBody"])
        target.fixture["reportActor"] = actor
        target.save()
        task = target.result(actor, progress["originalBody"])
        target.check(task["job"]["state"] == "cancelled" and task["job"]["cleanupState"] == "cleared", "ready report-backed export restores cancelled metadata only")
        for _ in range(60):
            holders = target.sql("SELECT count(*) FROM governance.personal_export_holders WHERE state<>'cleared'")
            handoffs = target.sql("SELECT count(*) FROM governance.personal_export_cleanups WHERE state<>'cleared'")
            if holders == "0" and handoffs == "0":
                break
            # Migration can re-enqueue a denial-only old holder for the cleanup
            # worker. It must never restore a key while that final handoff runs.
            target.call("POST", "/api/me/data-exports/" + progress["job"] + "/download-ticket", {}, actor, 409)
            time.sleep(.1)
        else:
            raise AssertionError("Restored report cleanup ledger did not finish")
        target.check(holders == "0" and handoffs == "0", "restored report source holder and its durable handoff ledger finish cleared without a delivery grant")
        target.call("POST", "/api/me/data-exports/" + progress["job"] + "/download-ticket", {}, actor, 409)
        target.check(target.sql("SELECT key_ciphertext IS NULL FROM governance.export_jobs WHERE id='" + progress["job"] + "'") == "t", "restored report-backed task never regains a delivery key")
        target.call("POST", "/api/auth/logout", {}, actor)
        target.call("GET", "/api/me", session=actor, expect=401)
        progress["stage"] = "verified"
        save()
    except Exception as error:
        failure = str(error)
        save()
    result = {"status": "passed" if failure is None else "failed", "failure": failure, "lastCompletedStage": progress["stage"], "httpChecks": suite.checks + (target.checks if target else 0), "semanticAssertions": suite.assertions + (target.assertions if target else 0), "binarySHA256": info["binarySHA256"], "operatorBinarySHA256": info["opsBinarySHA256"], "scriptSHA256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(), "realEncryptedReportHolderRestore": True, "checkedAt": datetime.now(timezone.utc).isoformat(), "assertions": suite.transcript + (target.transcript if target else [])}
    if Path(progress.get("backup", "missing")).is_file():
        backup = Path(progress["backup"])
        result.update(backupSHA256=hashlib.sha256(backup.read_bytes()).hexdigest(), backupBytes=backup.stat().st_size)
    args.report.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({key: value for key, value in result.items() if key != "assertions"}, ensure_ascii=False))
    if failure:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
