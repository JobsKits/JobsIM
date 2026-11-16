#!/usr/bin/env python3
"""Owned restored formal task: unknown holder, exact originals and expired delivery."""

import argparse
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import secrets
import time
import uuid


SPEC = importlib.util.spec_from_file_location("personal_export_negative_suite", Path(__file__).with_name("personal_exports.py"))
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--environment-file", type=Path, required=True)
    parser.add_argument("--report", type=Path, required=True)
    args = parser.parse_args()
    info = json.loads(args.environment_file.read_text())
    MODULE.REALM.free_space(info["runDir"])
    suite = MODULE.PersonalExportSuite(info, args.environment_file)
    failure = None
    try:
        if "formalProbeBody" not in suite.fixture:
            suite.fixture["formalProbeBody"] = {"username": "formal_probe_" + secrets.token_hex(6), "password": "Formal owned test " + secrets.token_hex(20), "requestId": str(uuid.uuid4()), "deviceName": "formal-negative-owned"}
            suite.save()
        if "formalProbe" not in suite.fixture:
            suite.fixture["formalProbe"] = suite.call("POST", "/api/auth/register", suite.fixture["formalProbeBody"])
            suite.save()
        actor = suite.fixture["formalProbe"]
        if "formalProbeConversation" not in suite.fixture:
            conversation = suite.call("POST", "/api/conversations", {"kind": "self"}, actor)
            suite.fixture["formalProbeConversation"] = conversation["id"]
            suite.save()
            suite.send(actor, conversation["id"], suite.payload(actor, conversation["id"], "own complete self package"))
        task = suite.named_task("negativeReady", actor)
        job = task["job"]["id"]
        create_body = suite.fixture["negativeReadyBody"]
        suite.check(suite.result(actor, create_body)["job"]["id"] == job, "original complete export intent resolves its single committed task")
        suite.result(actor, dict(create_body, restoreEpoch="1"), 409)
        suite.call("GET", "/api/me/data-exports/" + suite.fixture["restoreReadyJob"], session=actor, expect=404)
        ticket = suite.call("POST", "/api/me/data-exports/" + job + "/download-ticket", {}, actor)
        suite.fixture["negativeHolderFixture"] = {"job": job, "kind": "account", "state": "active"}
        suite.save()
        suite.sql("UPDATE auth.personal_export_holders SET state='prepared' WHERE job_id='" + job + "' AND kind='account'")
        try:
            body, _ = suite.binary(actor, "/api/me/data-exports/" + job + "/download", 409, {"X-JobsIM-Download-Token": ticket["downloadToken"]})
            suite.check(not body.startswith(b"PK") and json.loads(body)["error"]["code"] == "EXPORT_SOURCE_CHANGED", "explicit owned unknown-holder fixture rejects delivery before any ZIP byte")
        finally:
            suite.sql("UPDATE auth.personal_export_holders SET state='active' WHERE job_id='" + job + "' AND kind='account' AND state='prepared'")
        package, _ = suite.download(actor, job)
        suite.check(package.testzip() is None, "only explicit restoration of the exact fixture holder enables the unchanged package")
        suite.call("POST", "/api/me/exports", {"requestId": create_body["requestId"]}, actor, 404)
        suite.call("GET", "/api/me/exports/" + job, session=actor, expect=404)
        suite.check(suite.sql("SELECT purpose FROM governance.export_jobs WHERE id='" + job + "'") == "personal_data", "disabled experimental routes do not mutate or expose a formal task")
        suite.sql("UPDATE governance.export_jobs SET expires_at=now()-interval '1 second' WHERE id='" + job + "'")
        try:
            suite.call("POST", "/api/me/data-exports/" + job + "/download-ticket", {}, actor, 409)
        except AssertionError as error:
            # The denial can be a checked active-but-expired job before the worker publishes cancelled.
            if "received 403/EXPORT_EXPIRED" not in str(error):
                raise
            suite.checks += 1
        for _ in range(100):
            view = suite.status(actor, job)
            if view["job"]["cleanupState"] == "cleared":
                break
            time.sleep(.1)
        else:
            raise AssertionError("Expired formal artifact cleanup did not finish")
        suite.check(view["job"]["state"] == "expired" and suite.sql("SELECT key_ciphertext IS NULL FROM governance.export_jobs WHERE id='" + job + "'") == "t", "explicit owned job-expiry SQL clock fixture revokes key and drains all delivery bytes")
        suite.call("POST", "/api/auth/logout", {}, actor)
        suite.call("GET", "/api/me", session=actor, expect=401)
    except Exception as error:
        failure = str(error)
    finally:
        suite.save()
    result = {"status": "passed" if failure is None else "failed", "failure": failure, "httpChecks": suite.checks, "semanticAssertions": suite.assertions, "binarySHA256": info["binarySHA256"], "scriptSHA256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(), "SQLFixtures": ["owned exact holder active/prepared/active", "owned job expiry only"], "experimentalEnabled": False, "checkedAt": datetime.now(timezone.utc).isoformat(), "assertions": suite.transcript}
    args.report.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({key: value for key, value in result.items() if key != "assertions"}, ensure_ascii=False))
    if failure:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
