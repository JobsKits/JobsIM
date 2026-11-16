#!/usr/bin/env python3
"""Final capability-only candidate: actual decoded ZIP and full reporter secret exclusion."""

import argparse
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import uuid


SPEC = importlib.util.spec_from_file_location("personal_final_suite", Path(__file__).with_name("personal_exports.py"))
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def decoded(suite, actor, job):
    package, _ = suite.download(actor, job)
    suite.check(package.testzip() is None, "actual ZIP CRC and complete entry extraction succeed")
    return {name: package.read(name) for name in package.namelist()}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--environment-file", type=Path, required=True)
    parser.add_argument("--report", type=Path, required=True)
    args = parser.parse_args()
    info = json.loads(args.environment_file.read_text())
    suite = MODULE.PersonalExportSuite(info, args.environment_file)
    MODULE.REALM.free_space(info["runDir"])
    failure = None
    try:
        bootstrap = suite.call("GET", "/api/bootstrap")
        suite.check(bootstrap["features"]["chat_export"] is True and bootstrap["features"]["after_receipt"] is False and bootstrap["features"]["strong_protection"] is False, "actual capability enables formal export while irreversible and strong protection gates remain off")
        actor = suite.fixture["finalFormalActor"]
        job = suite.fixture["finalFormalJob"]
        current = suite.status(actor, job)
        files = decoded(suite, actor, job)
        all_data = b"\n".join(files.values())
        suite.check(b"FINAL_CURRENT_FORMAL_TEXT" in all_data and b"FINAL_OWN_REPORT_REASON" in all_data and "own_report_reasons_and_public_status" in current["coverage"], "decoded nonexperimental formal ZIP contains its actual own message and own report")
        suite.call("GET", "/api/me/exports/" + job, session=actor, expect=404)
        suite.fixture.setdefault("finalCancelBody", {"requestId": str(uuid.uuid4())})
        suite.save()
        suite.call("POST", "/api/me/data-exports/" + job + "/cancel", suite.fixture["finalCancelBody"], actor)
        suite.call("POST", "/api/auth/logout", {}, actor)
        suite.call("GET", "/api/me", session=actor, expect=401)
        if "finalReportLoginBody" not in suite.fixture:
            old = suite.fixture["reportActor"]
            suite.fixture["finalReportLoginBody"] = {"username": old["user"]["username"], "password": suite.passwords[old["user"]["id"]], "requestId": str(uuid.uuid4()), "deviceName": "final-report-secret-verification"}
            suite.save()
        reporter = suite.call("POST", "/api/auth/login", suite.fixture["finalReportLoginBody"])
        suite.fixture["finalReporter"] = reporter
        suite.save()
        task = suite.named_task("finalReportSecrets", reporter)
        files = decoded(suite, reporter, task["job"]["id"])
        reports = [json.loads(raw) for name, raw in files.items() if name.startswith("Data/governance/report/")]
        all_data = b"\n".join(files.values())
        suite.check(len(reports) == 1 and reports[0]["state"] == "reviewed" and reports[0]["reason"] == "REPORTER_INDEPENDENT_OWN_REASON", "actual decompressed reporter entry contains the current public disposition and original own reason")
        suite.check(b"ADMINISTRATOR_PRIVATE_OPERATION_REASON" not in all_data and b"OTHER_REPORTER_PRIVATE_REASON" not in all_data, "decompressed entire package excludes actual stored administrator and other reporter private reasons")
        suite.check(all(marker not in all_data for marker in [b'"refreshToken"', b'"decryptionKey"', b'"draftAdmission"', b'"replyAdmission"', b'"forwardAdmission"', b'"sendAdmission"']), "entire decompressed current package excludes auth and temporary source grants")
        suite.call("POST", "/api/me/data-exports/" + task["job"]["id"] + "/cancel", {"requestId": str(uuid.uuid4())}, reporter)
        suite.call("POST", "/api/auth/logout", {}, reporter)
        suite.call("GET", "/api/me", session=reporter, expect=401)
    except Exception as error:
        failure = str(error)
    finally:
        suite.save()
    result = {"status": "passed" if failure is None else "failed", "failure": failure, "httpChecks": suite.checks, "semanticAssertions": suite.assertions, "binarySHA256": info["binarySHA256"], "operatorBinarySHA256": info["opsBinarySHA256"], "scriptSHA256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(), "chat_export": True, "experimentalEnabled": False, "after_receipt": False, "checkedAt": datetime.now(timezone.utc).isoformat(), "assertions": suite.transcript}
    args.report.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({key: value for key, value in result.items() if key != "assertions"}, ensure_ascii=False))
    if failure:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
