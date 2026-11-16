#!/usr/bin/env python3
"""Current-role and explicit experimental-purpose checks, never a pre-upgrade claim."""

import argparse
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import time
import uuid


SPEC = importlib.util.spec_from_file_location("personal_export_compatibility", Path(__file__).with_name("personal_exports.py"))
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


def roles(suite):
    boot = suite.call("GET", "/api/bootstrap")
    suite.check(boot["features"]["role_admission"] is True, "current role protection is discoverable")
    owner, successor, member = [suite.register(label) for label in ["formalrolea", "formalroleb", "formalrolec"]]
    suite.fixture.update(compatOwner=owner, compatSuccessor=successor, compatMember=member)
    suite.save()
    for actor in [successor, member]:
        suite.friend(owner, actor)
    group = suite.call("POST", "/api/conversations", {"kind": "group", "title": "Current role formal compatibility", "memberIds": [successor["user"]["id"], member["user"]["id"]]}, owner)
    conversation = group["id"]
    source_body = suite.payload(owner, conversation, "current role source", mentionedUserIds=[member["user"]["id"]])
    source = suite.send(owner, conversation, source_body)
    suite.check(suite.message(member, source["id"])["mentionedMe"] is True, "current member structured mention survives the source drain wrapper")
    metadata = suite.conversation(owner, conversation)
    original = dict(suite.announce_body(owner, conversation, source), roleGeneration=metadata["roleGeneration"])
    committed = suite.put_announcement(owner, conversation, original)
    suite.check(committed["current"] and committed["status"] == "applied", "current role announcement commits with its original role and membership snapshot")
    old_all = suite.payload(owner, conversation, "original unsubmitted all", mentionAll=True)
    pending = dict(suite.announce_body(owner, conversation, source), roleGeneration=metadata["roleGeneration"])
    suite.call("PATCH", "/api/conversations/" + conversation, {"ownerId": successor["user"]["id"]}, owner)
    suite.call("PATCH", "/api/conversations/" + conversation, {"ownerId": owner["user"]["id"]}, successor)
    returned = suite.conversation(owner, conversation)
    suite.check(returned["membershipGeneration"] == metadata["membershipGeneration"] and int(returned["roleGeneration"]) == int(metadata["roleGeneration"]) + 2, "owner-role boomerang changes role generation without discarding legitimate history membership")
    suite.check(suite.send(owner, conversation, old_all, 409)["error"]["code"] == "SEND_ADMISSION_CHANGED", "old unsubmitted all cannot borrow returned owner authority")
    suite.call("POST", "/api/conversations/" + conversation + "/message-results", old_all, owner, 404)
    suite.check(suite.put_announcement(owner, conversation, pending, 409)["error"]["code"] == "QUALIFICATION_CHANGED", "old unsubmitted announcement cannot borrow returned role")
    suite.check(suite.put_announcement(owner, conversation, original)["operationId"] == committed["operationId"], "already committed complete announcement body remains read-only metadata under current read ACL")
    fresh = dict(suite.announce_body(owner, conversation, source), roleGeneration=returned["roleGeneration"])
    suite.put_announcement(owner, conversation, fresh)
    suite.call("PUT", "/api/messages/" + source["id"] + "/favorite", {}, member)
    suite.call("PUT", "/api/messages/" + source["id"] + "/pin", {}, owner)
    suite.call("POST", "/api/messages/" + source["id"] + "/reactions", {"emoji": "👍"}, member)
    suite.check(suite.announcement(member, conversation)["state"] == "active", "personal interactions do not invalidate an announcement content version")
    current = suite.message(owner, source["id"])
    suite.call("PATCH", "/api/messages/" + source["id"], {"text": "new content version", "expectedVersion": current["version"], "requestId": str(uuid.uuid4())}, owner)
    suite.check(suite.announcement(member, conversation)["state"] == "unavailable", "source edit drains and makes the previous announcement reference unavailable")
    suite.check(suite.call("POST", "/api/conversations/" + conversation + "/message-results", source_body, owner)["id"] == source["id"], "original committed message remains queryable without creating another intent")
    for actor in [owner, successor, member]:
        suite.call("POST", "/api/auth/logout", {}, actor)
        suite.call("GET", "/api/me", session=actor, expect=401)


def purpose(suite):
    login = suite.fixture.get("purposeLoginBody")
    if login is None:
        original = suite.fixture["formalProbeBody"]
        login = {"username": original["username"], "password": original["password"], "requestId": str(uuid.uuid4()), "deviceName": "formal-purpose-current"}
        suite.fixture["purposeLoginBody"] = login
        suite.save()
    actor = suite.call("POST", "/api/auth/login", login)
    suite.fixture["purposeActor"] = actor
    suite.save()
    if "purposeFormalBody" not in suite.fixture:
        suite.fixture["purposeFormalBody"] = suite.original()
        suite.save()
    body = suite.fixture["purposeFormalBody"]
    formal = suite.create(actor, body)
    suite.fixture["purposeFormalJob"] = formal["job"]["id"]
    suite.save()
    suite.ready(actor, formal["job"]["id"])
    collision = suite.call("POST", "/api/me/exports", {"requestId": body["requestId"]}, actor, 409)
    suite.check(collision["error"]["code"] == "IDEMPOTENCY_CONFLICT", "enabled experimental creation cannot reuse a formal request identity or expose its key")
    suite.call("GET", "/api/me/exports/" + formal["job"]["id"], session=actor, expect=404)
    suite.call("POST", "/api/me/exports/" + formal["job"]["id"] + "/download-ticket", {}, actor, 404)
    suite.binary(actor, "/api/me/exports/" + formal["job"]["id"] + "/download", 404)
    if "purposeExperimentalBody" not in suite.fixture:
        suite.fixture["purposeExperimentalBody"] = {"requestId": str(uuid.uuid4())}
        suite.save()
    experimental = suite.call("POST", "/api/me/exports", suite.fixture["purposeExperimentalBody"], actor, 202)
    suite.fixture["purposeExperimental"] = experimental
    suite.save()
    for _ in range(100):
        current = suite.call("GET", "/api/me/exports/" + experimental["job"]["id"], session=actor)
        if current["state"] == "ready":
            break
        if current["state"] not in ["requested", "building"]:
            raise AssertionError("Explicit ordinary experimental package failed")
        time.sleep(.1)
    else:
        raise AssertionError("Explicit experimental package did not become ready")
    suite.check("decryptionKey" in experimental and experimental["keyReturned"] and suite.sql("SELECT purpose FROM governance.export_jobs WHERE id='" + current["id"] + "'") == "experimental_chat", "explicit experimental key protocol stays separate and still builds its ordinary authorized chat")
    suite.call("GET", "/api/me/data-exports/" + current["id"], session=actor, expect=404)
    suite.create(actor, {"requestId": suite.fixture["purposeExperimentalBody"]["requestId"], "restoreEpoch": suite.epoch}, 409)
    suite.result(actor, {"requestId": suite.fixture["purposeExperimentalBody"]["requestId"], "restoreEpoch": suite.epoch}, 404)
    package, _ = suite.download(actor, formal["job"]["id"])
    suite.check(package.testzip() is None, "formal endpoint still delivers its independently sealed valid ZIP while experiments are enabled")
    for route, job in [("data-exports", formal["job"]["id"]), ("exports", current["id"])]:
        suite.call("POST", "/api/me/" + route + "/" + job + "/cancel", {"requestId": str(uuid.uuid4())}, actor)
    suite.call("POST", "/api/auth/logout", {}, actor)
    suite.call("GET", "/api/me", session=actor, expect=401)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--environment-file", type=Path, required=True)
    parser.add_argument("--group", choices=["roles", "purpose"], required=True)
    parser.add_argument("--report", type=Path, required=True)
    args = parser.parse_args()
    info = json.loads(args.environment_file.read_text())
    MODULE.REALM.free_space(info["runDir"])
    suite = MODULE.PersonalExportSuite(info, args.environment_file)
    failure = None
    try:
        globals()[args.group](suite)
    except Exception as error:
        failure = str(error)
    finally:
        suite.save()
    result = {"status": "passed" if failure is None else "failed", "failure": failure, "group": args.group, "httpChecks": suite.checks, "semanticAssertions": suite.assertions, "binarySHA256": info["binarySHA256"], "scriptSHA256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(), "checkedAt": datetime.now(timezone.utc).isoformat(), "notAPreUpgradeMigrationTest": True, "assertions": suite.transcript}
    args.report.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({key: value for key, value in result.items() if key != "assertions"}, ensure_ascii=False))
    if failure:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
