#!/usr/bin/env python3
"""Reporter-owned public fields, real MFA disposition and synchronous package revocation."""

import argparse
import base64
from datetime import datetime, timezone
import hashlib
import hmac
import importlib.util
import json
from pathlib import Path
import secrets
import struct
import time
import uuid


SPEC = importlib.util.spec_from_file_location("personal_report_restore", Path(__file__).with_name("restore.py"))
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def otp(secret, step):
    raw = hmac.new(secret, struct.pack(">Q", step), hashlib.sha1).digest()
    at = raw[-1] & 15
    return f"{(struct.unpack('>I', raw[at:at+4])[0] & 0x7fffffff) % 1000000:06d}"


def contents(suite, actor, job):
    package, raw = suite.download(actor, job)
    extracted = {name: package.read(name) for name in package.namelist() if name.endswith(".json")}
    return {name: json.loads(raw) for name, raw in extracted.items()}, b"\n".join(extracted.values())


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--environment-file", type=Path, required=True)
    parser.add_argument("--report", type=Path, required=True)
    args = parser.parse_args()
    info = json.loads(args.environment_file.read_text())
    suite = MODULE.MODULE.PersonalExportSuite(info, args.environment_file)
    MODULE.REALM.free_space(info["runDir"])
    failure = None
    try:
        admin = suite.fixture.get("reportAdmin")
        if admin is None:
            admin = {"username": "formal_admin_" + secrets.token_hex(4), "password": "Formal admin fixture " + secrets.token_hex(20), "operation": "formal-admin-" + uuid.uuid4().hex, "secretFile": str(Path(info["runDir"]) / "Secrets/formal-report-admin-totp")}
            suite.fixture["reportAdmin"] = admin
            suite.save()
        MODULE.run_command(info, "formal-report-admin-bootstrap", [info["opsBinary"], "--mode", "bootstrap-admin", "--username", admin["username"], "--state-dir", info["stateDir"], "--totp-secret-file", admin["secretFile"], "--operation-id", admin["operation"], "--internal-base-port", str(info["internalBasePort"])], {"JOBS_IM_BOOTSTRAP_PASSWORD": admin["password"]})
        secret = base64.b32decode(Path(admin["secretFile"]).read_text().strip())
        if "reportActor" not in suite.fixture:
            suite.fixture["reportActor"] = suite.register("reporterown")
            suite.fixture["otherReportActor"] = suite.register("reporterother")
            suite.save()
        actor, other = suite.fixture["reportActor"], suite.fixture["otherReportActor"]
        if "ownReportBody" not in suite.fixture:
            suite.fixture["ownReportBody"] = {"targetType": "user", "targetId": other["user"]["id"], "reason": "REPORTER_INDEPENDENT_OWN_REASON", "requestId": str(uuid.uuid4())}
            suite.fixture["foreignReportBody"] = {"targetType": "user", "targetId": actor["user"]["id"], "reason": "OTHER_REPORTER_PRIVATE_REASON", "requestId": str(uuid.uuid4())}
            suite.save()
        own_report = suite.call("POST", "/api/reports", suite.fixture["ownReportBody"], actor)
        foreign_report = suite.call("POST", "/api/reports", suite.fixture["foreignReportBody"], other)
        suite.check(suite.call("POST", "/api/reports", suite.fixture["ownReportBody"], actor)["id"] == own_report["id"], "report submission original nonce retains one current source identity")
        ready = suite.named_task("reportBeforeReview", actor)
        job = ready["job"]["id"]
        views, raw = contents(suite, actor, job)
        reports = [value for name, value in views.items() if name.startswith("Data/governance/report/")]
        suite.check(len(reports) == 1 and reports[0]["id"] == own_report["id"] and reports[0]["reason"] == own_report["reason"] and reports[0]["state"] == "open", "formal manifest includes only the reporter's current own reason and public status")
        suite.check(b"OTHER_REPORTER_PRIVATE_REASON" not in raw and foreign_report["id"].encode() not in raw and "own_report_reasons_and_public_status" in ready["coverage"], "another reporter's content is absent and live coverage explicitly includes own reports")
        unauthorized = {"state": "dismissed", "reason": "unauthorized change", "expectedVersion": own_report["version"], "requestId": str(uuid.uuid4()), "stepUpId": str(uuid.uuid4())}
        suite.call("PATCH", "/api/admin/reports/" + own_report["id"], unauthorized, actor, 403)
        suite.check(suite.status(actor, job)["job"]["state"] == "ready", "unauthorized report disposition neither changes the source nor revokes a valid personal package")
        step = int(time.time()) // 30
        login = {"username": admin["username"], "password": admin["password"], "otp": otp(secret, step), "requestId": str(uuid.uuid4()), "deviceName": "formal-report-admin-current"}
        suite.fixture["reportAdminLoginBody"] = login
        suite.save()
        administrator = suite.call("POST", "/api/auth/login", login)
        suite.fixture["reportAdministrator"] = administrator
        suite.save()
        payload = {"state": "reviewed", "reason": "ADMINISTRATOR_PRIVATE_OPERATION_REASON", "expectedVersion": own_report["version"]}
        nonce = str(uuid.uuid4())
        proof = suite.call("POST", "/api/auth/step-up", {"action": "report.resolve", "targetId": own_report["id"], "requestId": nonce, "otp": otp(secret, step + 1), "payload": payload}, administrator)
        original = dict(payload, requestId=nonce, stepUpId=proof["authorizationId"])
        suite.fixture["reportResolutionBody"] = original
        suite.save()
        resolved = suite.call("PATCH", "/api/admin/reports/" + own_report["id"], original, administrator)
        suite.check(suite.status(actor, job)["job"]["cleanupState"] == "cleared" and suite.sql("SELECT state FROM governance.personal_export_cleanups WHERE job_id='" + job + "'") == "cleared", "real MFA-authorized disposition drains same-owner governance holders and artifact before its success response")
        suite.check(suite.call("PATCH", "/api/admin/reports/" + own_report["id"], original, administrator) == resolved, "original completed disposition is replayed without another source revision")
        second = suite.named_task("reportAfterReview", actor)
        views, raw = contents(suite, actor, second["job"]["id"])
        report = next(value for name, value in views.items() if name.startswith("Data/governance/report/"))
        suite.check(report["state"] == "reviewed" and report["reason"] == own_report["reason"] and b"ADMINISTRATOR_PRIVATE_OPERATION_REASON" not in raw and b"OTHER_REPORTER_PRIVATE_REASON" not in raw, "new current package exports the public reviewed state while retaining only the reporter's own reason")
        suite.check(suite.sql("SELECT count(*) FROM governance.personal_export_holders WHERE job_id='" + second["job"]["id"] + "' AND kind='report' AND state='active' AND journal_seq IS NOT NULL") == "1", "report body is backed by its own active durable typed holder")
        # Leave the second ready task available for the final capability-only recheck.
        for user in [other, administrator]:
            suite.call("POST", "/api/auth/logout", {}, user)
            suite.call("GET", "/api/me", session=user, expect=401)
    except Exception as error:
        failure = str(error)
    finally:
        suite.save()
    result = {"status": "passed" if failure is None else "failed", "failure": failure, "httpChecks": suite.checks, "semanticAssertions": suite.assertions, "binarySHA256": info["binarySHA256"], "scriptSHA256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(), "administratorBootstrap": "owned realm real operator and TOTP; no SQL privilege grant", "checkedAt": datetime.now(timezone.utc).isoformat(), "assertions": suite.transcript}
    args.report.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({key: value for key, value in result.items() if key != "assertions"}, ensure_ascii=False))
    if failure:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
