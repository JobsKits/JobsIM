#!/usr/bin/env python3
"""Owned-text cloud drafts; source references never become copied quote bodies."""

import argparse
import concurrent.futures
from datetime import datetime, timezone
import importlib.util
import json
from pathlib import Path
import shutil
import time
import uuid


HELPER = Path(__file__).resolve().parents[1] / "self_state_sync.py/self_state_sync.py"
SPEC = importlib.util.spec_from_file_location("jobsim_cloud_drafts_http", HELPER)
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class DraftSuite(MODULE.SelfStateSuite):
    def __init__(self, environment, seed):
        super().__init__(environment, seed)
        self.fixture_path = Path(environment["runDir"]) / "Secrets/draft-fixture.json"
        self.fixture = json.loads(self.fixture_path.read_text())
        self.passwords = self.fixture["passwords"]
        self.stores = self.fixture.get("stores", {})
        self.conflicts = 0

    def draft(self, actor, conversation=None):
        return self.call("GET", "/api/conversations/" + (conversation or self.fixture["conversation"]) + "/draft", session=actor)

    def body(self, actor, text, source=None, conversation=None):
        current = self.draft(actor, conversation)
        reply = None
        if source is not None:
            message = self.message(actor, source["id"])
            self.check(message["conversationId"] == (conversation or self.fixture["conversation"]) and message.get("replyAdmission", "").startswith("dr1."), "reply selection freezes the current same-conversation source proof")
            reply = {"messageId": message["id"], "sourceVersion": message["contentVersion"], "replyAdmission": message["replyAdmission"]}
        return {"requestId": str(uuid.uuid4()), "revisionEpoch": self.epoch, "expectedVersion": current["version"], "draftAdmission": current["draftAdmission"], "ownedText": text, "reply": reply}

    def put(self, actor, body, conversation=None, expect=200):
        return self.call("PUT", "/api/conversations/" + (conversation or self.fixture["conversation"]) + "/draft", body, actor, expect)

    def result(self, actor, body, conversation=None, method="PUT", expect=200):
        return self.call("POST", "/api/conversations/" + (conversation or self.fixture["conversation"]) + "/draft-results", {"method": method, "body": body}, actor, expect)

    def set_text(self, actor, text, source=None, conversation=None):
        body = self.body(actor, text, source, conversation)
        self.fixture["last_original_body"] = body
        self.save()
        result = self.put(actor, body, conversation)
        self.check(result["status"] == "applied", "original owned text draft reached positive durable commit")
        return result, body

    def basic(self):
        actor, outsider = self.fixture["owner"], self.fixture["outsider"]
        conversation = self.fixture["conversation"]
        original = self.fixture["smokeBody"]
        self.check(self.result(actor, original)["current"], "original successful full body proves the current draft")
        self.result(actor, dict(original, ownedText="changed under same nonce"), expect=409)
        before = self.sql("SELECT count(*) FROM sync.draft_operations")
        self.result(actor, dict(original, requestId=str(uuid.uuid4())), expect=404)
        self.check(self.sql("SELECT count(*) FROM sync.draft_operations") == before, "unknown read-only result lookup creates no operation")
        self.call("GET", f"/api/conversations/{conversation}/draft", session=outsider, expect=403)
        first = self.body(actor, "concurrent independent input A")
        second = dict(first, requestId=str(uuid.uuid4()), ownedText="concurrent independent input B")
        self.fixture["cas_bodies"] = [first, second]
        self.save()
        def attempt(body):
            try:
                return self.put(actor, body)
            except AssertionError as error:
                if "409/DRAFT_VERSION_CONFLICT" not in str(error) and "409/DRAFT_OPERATION_PENDING" not in str(error):
                    raise
                self.conflicts += 1
                return None
        with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
            results = list(pool.map(attempt, [first, second]))
        self.check(sum(result is not None and result["status"] == "applied" for result in results) == 1, "two devices cannot silently overwrite the same draft CAS version")
        self.check(not self.result(actor, original)["current"] and "ownedText" not in json.dumps(self.result(actor, original)), "old successful metadata cannot revive its old text")
        current = self.draft(actor)
        clear = {"requestId": str(uuid.uuid4()), "revisionEpoch": self.epoch, "expectedVersion": current["version"], "draftAdmission": current["draftAdmission"]}
        self.fixture["clear_body"] = clear
        self.save()
        cleared = self.call("DELETE", f"/api/conversations/{conversation}/draft", clear, actor)
        self.check(cleared["status"] == "applied" and self.draft(actor)["ownedText"] is None, "explicit clear removes the current owned bytes")
        self.check(self.result(actor, clear, method="DELETE")["current"], "DELETE has the same original-body read-only proof contract")
        self.wait_network_origins()

    def source(self):
        actor, peer = self.fixture["owner"], self.fixture["peer"]
        conversation = self.fixture["conversation"]
        source = self.send(peer, conversation, self.payload(peer, conversation, "original quoted source"))
        _, body = self.set_text(actor, "independent owned text survives source edit", source)
        self.call("PUT", "/api/messages/" + source["id"] + "/favorite", {}, actor)
        self.check(self.draft(actor)["replyState"] == "available", "private favorite interaction does not invalidate a reply content reference")
        before = self.draft(actor)
        current = self.message(peer, source["id"])
        self.call("PATCH", "/api/messages/" + source["id"], {"requestId": str(uuid.uuid4()), "expectedVersion": current["version"], "text": "source edited"}, peer)
        after = self.draft(actor)
        self.check(after["ownedText"] == body["ownedText"] and after["replyState"] == "unavailable" and after["reply"] is None, "source edit retires only reply qualification and preserves owned text")
        self.check(int(after["version"]) > int(before["version"]) and not self.result(actor, body)["current"], "reference invalidation advances draft version and makes old success non-current")
        stale = self.body(actor, "new independent input with an expired original quote")
        stale["reply"] = body["reply"]
        self.fixture["retired_body"] = stale
        self.save()
        retired = self.put(actor, stale)
        recovery = self.draft(actor)
        self.check(retired["status"] == "retired" and recovery["recovery"]["ownedText"] == stale["ownedText"], "unpublished invalid reply preserves separately recoverable independent input")
        self.set_text(actor, recovery["recovery"]["ownedText"])
        self.check(self.draft(actor).get("recovery") is None, "explicit new authored replacement retires superseded recovery bytes")
        source = self.send(peer, conversation, self.payload(peer, conversation, "recall source"))
        _, body = self.set_text(actor, "independent owned text survives global source recall", source)
        self.call("DELETE", "/api/messages/" + source["id"], session=peer)
        self.check(self.draft(actor)["ownedText"] == body["ownedText"] and self.draft(actor)["replyState"] == "unavailable", "global source recall does not erase independently authored draft text")
        self.wait_network_origins()

    def membership(self):
        owner, peer = self.fixture["owner"], self.fixture["peer"]
        conversation = self.fixture["conversation"]
        source = self.send(owner, conversation, self.payload(owner, conversation, "membership source"))
        _, applied = self.set_text(peer, "peer independent input survives leave", source)
        queued = self.body(peer, "old unsent intent", source)
        self.fixture["old_membership_intent"] = queued
        self.save()
        generation = self.conversation(peer, conversation)["membershipGeneration"]
        self.call("DELETE", f"/api/conversations/{conversation}/members/{peer['user']['id']}?membershipGeneration={generation}", session=owner)
        retained = self.draft(peer)
        self.check(not retained["writable"] and retained["ownedText"] == applied["ownedText"] and retained["replyState"] == "unavailable", "former member keeps only private authored text without cloud reply grant")
        self.call("POST", f"/api/conversations/{conversation}/members", {"userId": peer["user"]["id"]}, owner)
        denied = self.put(peer, queued, expect=409)
        self.check(denied["error"]["code"] == "DRAFT_ADMISSION_CHANGED", "membership ABA cannot repair an unsent original draft admission")
        self.check(self.draft(peer)["replyState"] == "unavailable", "new membership never revives the old reply holder")
        self.wait_network_origins()

    def forwarding(self):
        owner, peer = self.fixture["owner"], self.fixture["peer"]
        conversation = self.fixture["conversation"]
        source = self.send(owner, conversation, self.payload(owner, conversation, "ancestor source"))
        forwarded, _ = self.forward(peer, conversation, source)
        _, body = self.set_text(peer, "independent text survives ancestor edit", forwarded)
        source = self.message(owner, source["id"])
        self.call("PATCH", "/api/messages/" + source["id"], {"requestId": str(uuid.uuid4()), "expectedVersion": source["version"], "text": "ancestor changed"}, owner)
        current = self.draft(peer)
        self.check(current["ownedText"] == body["ownedText"] and current["replyState"] == "unavailable", "recursive forward source invalidation drains the derived draft reply holder")
        self.wait_network_origins()

    def principal(self):
        actor, peer = self.fixture["owner"], self.fixture["peer"]
        conversation = self.fixture["conversation"]
        source = self.send(peer, conversation, self.payload(peer, conversation, "principal source"))
        _, body = self.set_text(actor, "independent text survives principal recovery", source)
        queued = self.body(actor, "old principal unsent input", source)
        self.fixture["old_principal_intent"] = queued
        self.save()
        codes = self.call("POST", "/api/auth/recovery-codes", {"requestId": str(uuid.uuid4()), "password": self.passwords[actor["user"]["id"]]}, actor)
        import secrets
        password = "Cloud draft recovered " + secrets.token_hex(16)
        self.call("POST", "/api/auth/recover", {"requestId": str(uuid.uuid4()), "username": actor["user"]["username"], "code": codes["codes"][0], "password": password})
        self.passwords[actor["user"]["id"]] = password
        self.fixture["owner"] = self.login(actor, password)
        self.save()
        actor = self.fixture["owner"]
        self.check(self.draft(actor)["ownedText"] == body["ownedText"] and self.draft(actor)["replyState"] == "unavailable", "account security recovery preserves owned text but never the old reply grant")
        denied = self.put(actor, queued, expect=409)
        self.check(denied["error"]["code"] == "DRAFT_ADMISSION_CHANGED", "fresh authentication cannot silently repair old unsent draft payloads")
        self.wait_network_origins()

    def sync(self):
        actor = self.fixture["owner"]
        self.register_schema(actor, "2")
        self.wait_network_origins()
        snapshot = self.snapshot(actor, 3)
        items = self.pages(actor, snapshot)
        drafts = [p for p in items if p["ref"]["kind"] == "draft"]
        self.check(drafts and all(p["object"]["userId"] == actor["user"]["id"] and "draftAdmission" not in p["object"] for p in drafts), "S2 drafts synchronize only own bodies without reusable admission proof")
        self.validate(actor, snapshot)
        self.set_text(actor, "S2 exact owned draft delta")
        self.wait_network_origins()
        _, events = self.drain(actor, snapshot["boundaryToken"])
        self.check(any(e.get("ref", {}).get("kind") == "draft" and e.get("projection", {}).get("object", {}).get("ownedText") == "S2 exact owned draft delta" for e in events), "real draft trigger and JetStream originate the current S2 body projection")
        self.check(self.sql("SELECT count(*) FROM sync.origins WHERE ref::text LIKE '%S2 exact owned draft delta%'") == "0" and self.sql("SELECT count(*) FROM journal.entries WHERE payload::text LIKE '%S2 exact owned draft delta%'", True) == "0", "neither sync pointers nor independent safety journal duplicate owned draft plaintext")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--environment-file", type=Path, required=True)
    parser.add_argument("--group", choices=["basic", "source", "membership", "forwarding", "principal", "sync"], required=True)
    parser.add_argument("--report", type=Path, required=True)
    arguments = parser.parse_args()
    environment = json.loads(arguments.environment_file.read_text())
    if not environment.get("cloudDraftRealm") or environment["serverURL"] != "https://127.0.0.1:54443" or shutil.disk_usage(environment["runDir"]).free < 4 * 1024 ** 3:
        raise SystemExit("Explicit owned fresh cloud draft HTTPS realm and 4GiB margin required")
    suite = DraftSuite(environment, arguments.environment_file)
    start = time.monotonic()
    failure = None
    try:
        getattr(suite, arguments.group)()
    except Exception as error:
        failure = str(error)
    finally:
        suite.save()
    result = {"group": arguments.group, "status": "passed" if failure is None else "failed", "failure": failure, "httpChecks": suite.checks, "semanticAssertions": suite.assertions, "expectedCASConflictsSeparate": suite.conflicts, "verifiedPages": suite.verified_pages, "elapsedSeconds": round(time.monotonic() - start, 3), "checkedAt": datetime.now(timezone.utc).isoformat(), "binarySHA256": environment["binarySHA256"], "assertions": suite.transcript}
    arguments.report.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({key: result[key] for key in result if key != "assertions"}, ensure_ascii=False))
    if failure:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
