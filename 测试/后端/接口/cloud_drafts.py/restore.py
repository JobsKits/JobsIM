#!/usr/bin/env python3
"""New encrypted draft backup and isolated safety-floor replay; never an old realm."""

import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import secrets
import shutil
import signal
import ssl
import subprocess
import time
import urllib.request
import uuid


SPEC = importlib.util.spec_from_file_location("jobsim_draft_restore_suite", Path(__file__).with_name("cloud_drafts.py"))
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def private_json(path, value):
    MODULE.MODULE.private_json(path, value)


def clean_environment(info):
    environment = {key: value for key, value in os.environ.items() if not key.startswith(("PG", "JOBS_IM_")) and key not in ("DATABASE_URL", "JOURNAL_DATABASE_URL") and not key.endswith("_DATABASE_URL")}
    environment.update(PGPASSFILE=info["pgPassFile"], GOMAXPROCS="2", JOBS_IM_DATABASE_URL=info["businessURL"], JOBS_IM_JOURNAL_DATABASE_URL=info["journalURL"])
    return environment


def command(info, name, arguments, extra=None, stdin=None):
    if shutil.disk_usage(info["runDir"]).free < 4 * 1024 ** 3:
        raise AssertionError("4GiB operator space gate")
    log = Path(info["runDir"]) / "Logs" / (name + ".log")
    with log.open("ab") as output:
        result = subprocess.run(arguments, env=clean_environment(info) | (extra or {}), input=stdin, stdout=output, stderr=subprocess.STDOUT, timeout=180)
    if result.returncode:
        raise AssertionError(name + " failed; original private log preserved")
    return {"step": name, "exitCode": result.returncode, "privateLog": log.name, "logSHA256": hashlib.sha256(log.read_bytes()).hexdigest()}


def stop_owned_app(info):
    actual = subprocess.run(["/bin/ps", "-p", str(info["serverPID"]), "-o", "command="], capture_output=True, text=True, timeout=10, check=True).stdout.strip()
    if actual != " ".join(info["serverCommand"]):
        raise AssertionError("Owned draft application command changed; no signal sent")
    os.kill(info["serverPID"], signal.SIGTERM)
    for _ in range(200):
        state = subprocess.run(["/bin/ps", "-p", str(info["serverPID"]), "-o", "stat="], capture_output=True, text=True, timeout=5).stdout.strip()
        if not state or state.startswith("Z"):
            return
        time.sleep(.05)
    raise AssertionError("Owned source did not stop; no force signal sent")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--environment-file", type=Path, required=True)
    parser.add_argument("--report", type=Path, required=True)
    args = parser.parse_args()
    info = json.loads(args.environment_file.read_text())
    if not info.get("cloudDraftRealm") or info["serverURL"] != "https://127.0.0.1:54443":
        raise SystemExit("Explicit new owned draft realm required")
    run = Path(info["runDir"])
    progress_path = run / "Secrets/draft-restore-progress.json"
    if progress_path.exists():
        raise SystemExit("Existing restore progress preserved; reconcile original operation before an explicit resume")
    suite = MODULE.DraftSuite(info, args.environment_file)
    owner, peer = suite.fixture["owner"], suite.fixture["peer"]
    progress = {"stage": "preparing", "operationId": "draft-restore-" + secrets.token_hex(10), "steps": [], "cases": {}}
    private_json(progress_path, progress)
    failure = None
    target_info = None
    try:
        for kind in ["clear", "replace", "reply"]:
            conversation = suite.call("POST", "/api/conversations", {"kind": "group", "title": "Draft restore " + kind, "memberIds": [peer["user"]["id"]]}, owner)["id"]
            source = suite.send(peer, conversation, suite.payload(peer, conversation, "draft restore source " + kind))
            text = "INDEPENDENT_BACKUP_OWNED_TEXT_" + kind
            _, body = suite.set_text(owner, text, source, conversation)
            queued = suite.body(owner, "original unsubmitted epoch-one input", source, conversation)
            progress["cases"][kind] = {"conversation": conversation, "source": source, "body": body, "queued": queued, "text": text}
            private_json(progress_path, progress)
        suite.wait_network_origins()
        backup_dir = Path.home() / "Documents/JobsIMBackups" / progress["operationId"]
        backup_dir.mkdir(parents=True, mode=0o700)
        backup, key = backup_dir / "draft-before-mutations.jimbackup", run / "Secrets/draft-recovery.key"
        progress.update(backup=str(backup), key=str(key))
        progress["steps"].append(command(info, "draft-backup", [info["opsBinary"], "--mode", "backup", "--pg-bin", info["pgBin"], "--state-dir", info["stateDir"], "--tls-dir", str(run / "TLS"), "--file", str(backup), "--key-file", str(key)]))
        progress["stage"] = "backup_created"
        private_json(progress_path, progress)
        clear = progress["cases"]["clear"]
        current = suite.draft(owner, clear["conversation"])
        clear_body = {"requestId": str(uuid.uuid4()), "revisionEpoch": suite.epoch, "expectedVersion": current["version"], "draftAdmission": current["draftAdmission"]}
        suite.call("DELETE", "/api/conversations/" + clear["conversation"] + "/draft", clear_body, owner)
        replace = progress["cases"]["replace"]
        suite.set_text(owner, "NEW_OWNED_TEXT_NOT_PRESENT_IN_BACKUP", conversation=replace["conversation"])
        reply = progress["cases"]["reply"]
        suite.call("DELETE", "/api/messages/" + reply["source"]["id"], session=peer)
        suite.wait_network_origins()
        progress["stage"] = "post_backup_facts_committed"
        private_json(run / "Secrets/pre-restore-fixture.json", suite.fixture)
        private_json(progress_path, progress)
        stop_owned_app(info)
        progress["stage"] = "source_stopped"
        private_json(progress_path, progress)
        name = "jobsim_draft_restore_" + secrets.token_hex(8)
        create_args = [str(Path(info["pgBin"]) / "psql"), info["mainAdminURL"], "-XqAt", "-v", "ON_ERROR_STOP=1"]
        progress["steps"].append(command(info, "draft-target-create", create_args, stdin=("CREATE DATABASE \"" + name + "\" TEMPLATE template0;\n").encode()))
        target_url = info["mainAdminURL"].replace("/postgres?", "/" + name + "?")
        target_state = run / "Restored/State"
        progress.update(targetURL=target_url, targetDatabase=name, targetState=str(target_state))
        private_json(progress_path, progress)
        restore_args = [info["opsBinary"], "--mode", "restore", "--pg-bin", info["pgBin"], "--state-dir", info["stateDir"], "--target-state-dir", str(target_state), "--tls-dir", str(run / "TLS"), "--file", str(backup), "--key-file", str(key), "--operation-id", progress["operationId"], "--offline"]
        progress["steps"].append(command(info, "draft-isolated-restore", restore_args, {"JOBS_IM_RESTORE_DATABASE_URL": target_url}))
        progress["stage"] = "restore_completed"
        private_json(progress_path, progress)
        target_info = dict(info, businessURL=target_url, businessDatabase=name, stateDir=str(target_state), tlsCert=str(target_state / "TLS/server.pem"), tlsKey=str(target_state / "TLS/server.key"), caPath=str(target_state / "TLS/ca.pem"), restoredDraftRealm=True)
        argv = [info["binary"], "--service=all", "--addr", "127.0.0.1:54443", "--internal-base-port", "55101", "--state-dir", str(target_state), "--tls-cert", target_info["tlsCert"], "--tls-key", target_info["tlsKey"]]
        environment = clean_environment(target_info) | {"JOBS_IM_PUBLIC_URL": info["serverURL"], "JOBS_IM_NATS_URL": info["natsURL"], "JOBS_IM_NATS_TOKEN_FILE": info["natsTokenFile"]}
        with (run / "Logs/restored-app.log").open("ab", buffering=0) as output:
            process = subprocess.Popen(argv, env=environment, stdout=output, stderr=output, start_new_session=True)
        target_info.update(serverPID=process.pid, serverCommand=argv)
        target_seed = run / "restored-environment.json"
        private_json(target_seed, target_info)
        trust = ssl.create_default_context(cafile=target_info["caPath"])
        for _ in range(150):
            if process.poll() is not None:
                raise AssertionError("Restored candidate exited; private raw log retained")
            try:
                with urllib.request.urlopen(info["serverURL"] + "/api/bootstrap", context=trust, timeout=2) as response:
                    bootstrap = json.load(response)
                break
            except Exception:
                time.sleep(.1)
        else:
            raise AssertionError("Restored strict HTTPS readiness timed out")
        if bootstrap["deploymentId"] != info["deploymentId"] or bootstrap["restoreEpoch"] != "2":
            raise AssertionError("Real restore did not preserve deployment and advance epoch once")
        target_info["restoreEpoch"] = bootstrap["restoreEpoch"]
        private_json(target_seed, target_info)
        target = MODULE.DraftSuite(target_info, target_seed)
        target.epoch = "2"
        target.call("GET", "/api/me", session=owner, expect=401)
        new_owner = target.login(owner)
        target.fixture["owner"] = new_owner
        target.save()
        cleared = target.draft(new_owner, clear["conversation"])
        replaced = target.draft(new_owner, replace["conversation"])
        retained = target.draft(new_owner, reply["conversation"])
        target.check(cleared["contentState"] == "empty" and cleared["ownedText"] is None, "independent clear floor prevents backup text resurrection")
        target.check(replaced["contentState"] == "unavailable" and replaced["ownedText"] is None, "a newer owned digest absent from backup cannot fall back to older text")
        target.check(retained["ownedText"] == reply["text"] and retained["replyState"] == "unavailable" and retained["reply"] is None, "restored source recall retires reply but retains matching independently owned ciphertext")
        target.check(all(view["revisionEpoch"] == "2" and view["version"] == "0" for view in [cleared, replaced, retained]), "restored mutable drafts rebase to the new epoch zero without old CAS dominance")
        rejected = target.put(new_owner, reply["queued"], reply["conversation"], 409)
        target.check(rejected["error"]["code"] == "RESTORE_EPOCH_CHANGED", "old unsubmitted draft body cannot be repaired with the new epoch")
        target.check(target.sql("SELECT count(*) FROM message.draft_reply_holders WHERE state IN('prepared','active','revoking')") == "0", "restore does not grant any old prepared or active reply holder")
        target.register_schema(new_owner, "2")
        snapshot = target.snapshot(new_owner, 4)
        target.pages(new_owner, snapshot)
        target.validate(new_owner, snapshot)
        target.set_text(new_owner, "explicit newly authored restored-epoch text", conversation=replace["conversation"])
        target.wait_network_origins()
        target_info["status"] = "restored_draft_candidate_ready"
        private_json(target_seed, target_info)
        progress["stage"] = "verified"
        private_json(progress_path, progress)
        result = {"status": "passed", "realEncryptedBackupAndOfflineRestore": True, "backupSHA256": hashlib.sha256(backup.read_bytes()).hexdigest(), "backupBytes": backup.stat().st_size, "scopeRetainedEpochAdvanced": True, "clearDigestAndReplyFloorsReplayed": True, "oldTextAndOldAdmissionNotRevived": True, "matchingOwnedTextSurvivesSourceRecall": True, "S2NewEpochSnapshotAndNewTextCommit": True, "httpChecks": suite.checks + target.checks, "semanticAssertions": suite.assertions + target.assertions, "verifiedPages": target.verified_pages, "binarySHA256": info["binarySHA256"], "operatorBinarySHA256": info["opsBinarySHA256"], "targetSeedPath": str(target_seed), "privateSteps": progress["steps"]}
    except Exception as error:
        failure = str(error)
        result = {"status": "failed", "failure": failure, "lastCompletedStage": progress["stage"], "privateProgressPreserved": True, "binarySHA256": info["binarySHA256"]}
    args.report.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({key: value for key, value in result.items() if key != "privateSteps"}, ensure_ascii=False))
    if failure:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
