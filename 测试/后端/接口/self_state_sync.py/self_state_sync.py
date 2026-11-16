#!/usr/bin/env python3
"""S2 own-state protocol against an explicitly owned dual-PG HTTPS realm."""

import argparse
import concurrent.futures
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import time
import urllib.parse
import uuid


HELPER = Path(__file__).resolve().parents[1] / "sync_snapshot.py/sync_snapshot.py"
SPEC = importlib.util.spec_from_file_location("jobsim_s2_sync_http", HELPER)
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)
KINDS = {"preferences", "conversation_view", "read_state", "read_observation"}


def private_json(path, value):
    MODULE.private_json(path, value)


class SelfStateSuite(MODULE.SyncSuite):
    def __init__(self, environment, seed):
        super().__init__(environment, seed)
        self.fixture_path = Path(environment["runDir"]) / "Secrets/s2-fixture.json"
        self.fixture = json.loads(self.fixture_path.read_text()) if self.fixture_path.exists() else {}
        self.passwords = self.fixture.get("passwords", {})
        self.stores = self.fixture.get("stores", {})
        self.epoch = environment["restoreEpoch"]
        self.schema = "2"
        self.transcript = []

    def check(self, condition, description):
        super().check(condition, description)
        self.transcript.append({"assertion": description, "passed": True})

    def save(self):
        self.fixture.update(passwords=self.passwords, stores=self.stores)
        private_json(self.fixture_path, self.fixture)

    def sql(self, command, journal=False):
        environment = {
            key: value for key, value in os.environ.items()
            if not key.startswith(("PG", "JOBS_IM_"))
            and key not in ("DATABASE_URL", "JOURNAL_DATABASE_URL")
            and not key.endswith("_DATABASE_URL")
        }
        environment["PGPASSFILE"] = self.environment["pgPassFile"]
        dsn = self.environment["journalURL" if journal else "businessURL"]
        parsed = urllib.parse.urlparse(dsn)
        port = 62382 if journal else 62381
        owner = self.environment["journalUsername" if journal else "mainUsername"]
        database = self.environment["journalDatabase" if journal else "businessDatabase"]
        if (parsed.hostname, parsed.port, parsed.username, parsed.path) != ("127.0.0.1", port, owner, "/" + database) or parsed.password:
            raise AssertionError("S2 owned DSN guard failed; values suppressed")
        result = subprocess.run(
            [str(Path(self.environment["pgBin"]) / "psql"), dsn, "-XqAt", "-v", "ON_ERROR_STOP=1"],
            input=command, capture_output=True, text=True, env=environment, timeout=30,
        )
        if result.returncode:
            raise AssertionError("S2 owned SQL failed; private database log retained")
        return result.stdout.strip()

    def register_schema(self, actor, schema):
        store = self.stores.setdefault(actor["deviceId"], str(uuid.uuid4()))
        self.save()
        body = {"requestId": str(uuid.uuid4()), "storeGeneration": store}
        if schema == "2":
            body["schemaVersion"] = schema
        result = self.call("PUT", "/api/sync/device", body, actor)
        repeated = self.call("PUT", "/api/sync/device", body, actor)
        self.check(result == repeated and not result["qualifiedAck"], "registration retries retain the same transport-only fact")
        return result

    def snapshot(self, actor, page_size=4, schema=None):
        selected = schema or self.schema
        body = {"requestId": str(uuid.uuid4()), "storeGeneration": self.stores[actor["deviceId"]], "schemaVersion": selected, "pageSize": page_size}
        result = self.call("POST", "/api/sync/snapshots", body, actor, 201)
        self.check(self.call("POST", "/api/sync/snapshots", body, actor) == result, "original S2 manifest is stable on retry")
        return result

    def changes(self, actor, cursor, limit=7):
        path = "/api/sync/changes?" + urllib.parse.urlencode({"cursor": cursor, "limit": limit, "schemaVersion": self.schema})
        result = self.call("GET", path, session=actor)
        for event in result["items"]:
            if event["kind"] == "redacted":
                self.check(set(event) == {"kind", "from", "to", "rangeDigest"}, "redacted intervals disclose no private IDs")
                continue
            import base64
            raw = base64.urlsafe_b64decode(event["eventEnvelope"] + "===")
            self.check(hashlib.sha256(raw).hexdigest() == event["eventDigest"], "event digest binds the original envelope bytes")
            self.check(json.loads(raw) == {"position": event["position"], "originEventId": event["originEventId"], "ref": event["ref"]}, "display event equals the verified pointer envelope")
            self.verify_body(event["projection"])
        return result

    def wait_network_origins(self):
        # No public sync request occurs here: HTTP compensation cannot satisfy it.
        for _ in range(120):
            lag = int(self.sql("SELECT " + "+".join(
                f"(SELECT revision FROM {owner}.sync_clock WHERE singleton)-(SELECT revision FROM {owner}.sync_publication WHERE singleton)"
                for owner in ["auth", "directory", "conversation", "message", "sync"]
            )))
            missing = int(self.sql("SELECT count(*) FROM (" + " UNION ALL ".join(
                f"SELECT o.origin_event_id FROM {owner}.sync_outbox o LEFT JOIN sync.origins s ON s.origin_event_id=o.origin_event_id WHERE s.origin_event_id IS NULL"
                for owner in ["auth", "directory", "conversation", "message", "sync"]
            ) + ") pending"))
            if lag == 0 and missing == 0:
                self.check(True, "actual PubAck and network consumer commit precede any HTTP replay")
                return
            time.sleep(.1)
        raise AssertionError("actual JetStream publication/consumption did not catch up")

    def read_body(self, actor, conversation, ids):
        member = self.conversation(actor, conversation)
        return {"requestId": str(uuid.uuid4()), "revisionEpoch": self.epoch, "membershipGeneration": member["membershipGeneration"], "observedMessageIds": ids}

    def read(self, actor, conversation):
        return self.call("GET", f"/api/conversations/{conversation}/read", session=actor)

    def group(self, label):
        owner, member = self.fixture["owner"], self.fixture["member"]
        return self.call("POST", "/api/conversations", {"kind": "group", "title": label, "memberIds": [member["user"]["id"]]}, owner)["id"]

    def group_a(self):
        boot = self.call("GET", "/api/bootstrap")
        self.check(boot["deploymentId"] == self.environment["deploymentId"] and boot["restoreEpoch"] == "1", "identity comes from the actual strict-HTTPS bootstrap")
        self.check(boot["features"]["sync_jetstream"] and not boot["features"]["after_receipt"], "configured NATS never qualifies durable device ACK")
        main = self.sql("SELECT system_identifier::text FROM pg_control_system()")
        journal = self.sql("SELECT system_identifier::text FROM pg_control_system()", True)
        self.check(main != journal, "journal uses another real physical PostgreSQL cluster")
        for label in ["owner", "member", "outsider"]:
            if label not in self.fixture:
                self.fixture[label] = self.register("s2" + label)
                self.save()
        owner, member = self.fixture["owner"], self.fixture["member"]
        self.friend(owner, member)
        self.fixture["conversation"] = self.group("S2 protocol primary")
        self.fixture["member2"] = self.login(member)
        self.check(member["deviceId"] != self.fixture["member2"]["deviceId"], "second authenticated device is a distinct identity")
        self.wait_network_origins()

    def group_b(self):
        member = self.fixture["member"]
        first = self.register_schema(member, "1")
        second = self.register_schema(member, "2")
        self.check(first["deviceId"] == second["deviceId"] and first["storeGeneration"] == second["storeGeneration"] and first["streamId"] != second["streamId"], "schema upgrade separates streams without changing physical chat identity")
        denied = self.call("PUT", "/api/sync/device", {"requestId": str(uuid.uuid4()), "storeGeneration": str(uuid.uuid4()), "schemaVersion": "2"}, member, 409)
        self.check(denied["error"]["code"] == "STORE_GENERATION_CHANGED", "different physical store cannot inherit a schema stream")
        for label in ["owner", "outsider", "member2"]:
            self.register_schema(self.fixture[label], "2")
        first_status = self.call("GET", "/api/sync/context", session=member)
        second_status = self.call("GET", "/api/sync/context?schemaVersion=2", session=member)
        self.check("own_read_points" not in first_status["coverage"]["included"] and "own_read_points" in second_status["coverage"]["included"], "schema-one coverage remains unchanged while schema-two adds own state")
        self.check("cloud_drafts" in second_status["coverage"]["excluded"], "unfinished drafts stay explicitly excluded")
        self.fixture["s1_initial"] = self.snapshot(member, schema="1")
        self.fixture["s2_initial"] = self.snapshot(member)
        self.check(not any(p["ref"]["kind"] in KINDS for p in self.pages(member, self.fixture["s1_initial"])), "schema-one pages never expose schema-two private kinds")
        initial = self.pages(member, self.fixture["s2_initial"])
        self.check({"preferences", "conversation_view", "read_state"}.issubset({p["ref"]["kind"] for p in initial}), "schema-two initial snapshot includes virtual private defaults")
        self.validate(member, self.fixture["s2_initial"])

    def group_c(self):
        owner, member, outsider = [self.fixture[k] for k in ["owner", "member", "outsider"]]
        conversation = self.fixture["conversation"]
        before = self.read(member, conversation)
        self.check(before["unreadCount"] == "0" and before["version"] == "0", "default own read is exact zero without a persisted cursor")
        messages = [self.send(owner, conversation, self.payload(owner, conversation, "S2 ordinary message " + str(i))) for i in range(3)]
        own = self.send(member, conversation, self.payload(member, conversation, "S2 own message"))
        self.fixture["messages"] = messages
        body = self.read_body(member, conversation, [messages[1]["id"]])
        later = self.call("PUT", f"/api/conversations/{conversation}/read", body, member)
        self.check(later["result"]["readPoint"] == {"epoch": "0", "sequence": "0"}, "observing later message cannot cross an unobserved predecessor")
        self.check(self.read(member, conversation)["unreadCount"] == "2", "exact unread removes a sparse observation but does not count own messages")
        body = self.read_body(member, conversation, [messages[0]["id"]])
        result = self.call("PUT", f"/api/conversations/{conversation}/read", body, member)
        self.check(result["result"]["readPoint"]["sequence"] == messages[1]["creationSequence"], "stored later observation joins the now-complete prefix")
        self.check(self.read(member, conversation)["unreadCount"] == "1", "compacted prefix keeps the remaining eligible unread exact")
        bodies = [self.read_body(member, conversation, [message["id"]]) for message in [messages[2], own]]
        with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
            results = list(pool.map(lambda request: self.call("PUT", f"/api/conversations/{conversation}/read", request, member), bodies))
        self.check(len(results) == 2 and self.read(member, conversation)["unreadCount"] == "0", "two real owner-RPC writers merge observations without losing a committed read")
        self.call("GET", f"/api/conversations/{conversation}/read", session=outsider, expect=403)
        self.call("PUT", f"/api/conversations/{conversation}/read", dict(body, sourceContextId="foreign"), member, 400)
        self.fixture["read_original"] = body
        self.wait_network_origins()

    def group_d(self):
        member = self.fixture["member"]
        conversation = self.fixture["conversation"]
        view = self.call("GET", f"/api/conversations/{conversation}/view", session=member)
        patch = {"requestId": str(uuid.uuid4()), "revisionEpoch": self.epoch, "expectedVersion": view["version"], "changes": {"hidden": True}}
        alternate = dict(patch, requestId=str(uuid.uuid4()), changes={"archived": True})
        with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
            def attempt(body):
                try:
                    return self.call("PATCH", f"/api/conversations/{conversation}/view", body, member), body
                except AssertionError as error:
                    if "409/VIEW_VERSION_CONFLICT" not in str(error):
                        raise
                    return None, body
            outcomes = list(pool.map(attempt, [patch, alternate]))
        self.check(sum(result is not None for result, _ in outcomes) == 1, "competing view CAS commits exactly one original intent")
        result, original = next((result, body) for result, body in outcomes if result)
        marked = {"requestId": str(uuid.uuid4()), "revisionEpoch": self.epoch, "expectedVersion": result["result"]["version"], "changes": {"manualUnread": True, "expectedManualUnreadGeneration": "0"}}
        current = self.call("PATCH", f"/api/conversations/{conversation}/view", marked, member)
        self.call("PUT", f"/api/conversations/{conversation}/read", self.read_body(member, conversation, []), member)
        self.check(self.call("GET", f"/api/conversations/{conversation}/view", session=member)["manualUnread"], "automatic read observation cannot erase an explicit manual unread marker")
        old = self.call("POST", f"/api/conversations/{conversation}/view-results", original, member)
        self.check(not old["current"] and old["result"]["version"] == result["result"]["version"], "old view proof returns its original fact without rolling back current")
        pref = self.call("GET", "/api/me/preferences", session=member)
        body = {"requestId": str(uuid.uuid4()), "revisionEpoch": self.epoch, "expectedVersion": pref["version"], "fontPreset": "large"}
        applied = self.call("PATCH", "/api/me/preferences", body, member)
        self.check(self.call("POST", "/api/me/preferences-results", body, member)["result"] == applied["result"], "preference proof binds the complete original payload")
        second = dict(body, requestId=str(uuid.uuid4()), expectedVersion=applied["result"]["version"], fontPreset="small")
        self.call("PATCH", "/api/me/preferences", second, member)
        self.check(not self.call("POST", "/api/me/preferences-results", body, member)["current"], "completed old preference cannot overwrite a later choice")
        self.fixture.update(view_original=original, preference_original=body)
        self.wait_network_origins()

    def group_e(self):
        self.wait_network_origins()
        kinds = self.sql("SELECT string_agg(DISTINCT kind,',' ORDER BY kind) FROM sync.sync_outbox")
        self.check({"conversation_view", "read_state", "read_observation"}.issubset(set(kinds.split(","))), "all own sync source kinds emitted real transaction Outbox rows")
        self.check(int(self.sql("SELECT count(*) FROM directory.sync_outbox WHERE kind='preferences'")) >= 2, "preference trigger committed its own owner pointers")
        self.check(int(self.sql("SELECT count(*) FROM sync.sync_outbox WHERE kind='read_observation' AND state='removed'")) > 0, "sparse read compaction publishes removed observations")
        before = self.sql("SELECT revision FROM sync.sync_clock")
        time.sleep(.6)
        self.check(self.sql("SELECT revision FROM sync.sync_clock") == before, "network import and allocation do not recursively mutate the sync source clock")
        member = self.fixture["member"]
        delta, events = self.drain(member, self.fixture["s2_initial"]["boundaryToken"])
        visible = [event for event in events if event["kind"] == "reference"]
        self.check(KINDS.issubset({event["ref"]["kind"] for event in visible}), "schema-two replays every new private kind from the original boundary")
        self.check(len({event["originEventId"] for event in visible}) == len(visible), "network import allocates one visible position per immutable origin")
        self.fixture["cursor"] = delta["cursor"]
        self.schema = "1"
        _, events = self.drain(member, self.fixture["s1_initial"]["boundaryToken"])
        self.check(not any(event.get("ref", {}).get("kind") in KINDS for event in events), "schema-one changes continue to filter new private events")
        self.schema = "2"

    def group_f(self):
        member = self.fixture["member"]
        snapshot = self.snapshot(member, 2)
        items = self.pages(member, snapshot)
        self.check(len(snapshot["pageManifest"]) > 1, "new private baseline is validated across multiple complete pages")
        read_views = [p["object"] for p in items if p["ref"]["kind"] == "read_state" and "object" in p]
        self.check(read_views and all(view["countStatus"] == "not_computed" and "unreadCount" not in view for view in read_views), "snapshot read projection never fabricates a late-derived count at the old cut")
        self.validate(member, snapshot)
        later = self.send(self.fixture["owner"], self.fixture["conversation"], self.payload(self.fixture["owner"], self.fixture["conversation"], "S2 appended after baseline"))
        _, events = self.drain(member, snapshot["boundaryToken"])
        self.check(any(e.get("ref", {}).get("id") == later["id"] for e in events), "first changes from the original validated boundary includes later append")
        outdated = self.snapshot(member, 3)
        current = self.call("GET", "/api/me/preferences", session=member)
        self.call("PATCH", "/api/me/preferences", {"requestId": str(uuid.uuid4()), "revisionEpoch": self.epoch, "expectedVersion": current["version"], "fontPreset": "standard"}, member)
        self.validate(member, outdated, 409)
        self.check(self.sql("SELECT count(*) FROM sync.origins WHERE ref::text LIKE '%S2 ordinary message%'") == "0" and self.sql("SELECT count(*) FROM sync.events WHERE ref::text LIKE '%S2 ordinary message%'") == "0", "synchronization pointer stores contain no chat body")

    def group_g(self):
        owner, member = self.fixture["owner"], self.fixture["member"]
        conversation = self.fixture["conversation"]
        stale = self.snapshot(member)
        queued = self.read_body(member, conversation, [])
        self.call("DELETE", f"/api/conversations/{conversation}/members/{member['user']['id']}?membershipGeneration={queued['membershipGeneration']}", session=owner)
        self.call("GET", f"/api/conversations/{conversation}/read", session=member, expect=403)
        retained = self.snapshot(member)
        projections = self.pages(member, retained)
        self.check(any(p.get("object", {}).get("writable") is False for p in projections if p["ref"]["kind"] == "read_state"), "former member retains metadata but never regains cloud read authority")
        self.call("POST", f"/api/conversations/{conversation}/members", {"userId": member["user"]["id"]}, owner)
        rejected = self.call("PUT", f"/api/conversations/{conversation}/read", queued, member, 409)
        self.check(rejected["error"]["code"] == "QUALIFICATION_CHANGED", "read intent cannot borrow a later membership after leave and rejoin")
        self.validate(member, stale, 409)
        current = self.snapshot(member)
        self.check(current["permissionGeneration"] != stale["permissionGeneration"], "permission ABA never restores the old manifest generation")
        source = self.message(owner, self.fixture["messages"][0]["id"])
        self.call("DELETE", "/api/messages/" + source["id"], {"requestId": str(uuid.uuid4()), "expectedVersion": source["version"]}, owner)
        self.check(self.classifications(member, [source["id"]])[0]["classification"] == "global", "historically entitled member still receives the minimal global tombstone")
        self.check(self.read(member, conversation)["unreadCount"] == "0", "new membership cannot inherit old-interval unread bodies")

    def discard_after_commit(self, actor, method, path, body, query):
        endpoint = urllib.parse.urlsplit(self.base)
        payload = json.dumps(body).encode()
        with socket.create_connection((endpoint.hostname, endpoint.port), timeout=15) as connection:
            with self.trust.wrap_socket(connection, server_hostname=endpoint.hostname) as secure:
                headers = (f"{method} {path} HTTP/1.1\r\nHost: {endpoint.netloc}\r\nAuthorization: Bearer {actor['token']}\r\nContent-Type: application/json\r\nContent-Length: {len(payload)}\r\nConnection: close\r\n\r\n").encode()
                secure.sendall(headers + payload)
                for _ in range(100):
                    if self.sql(query) == "1":
                        self.check(True, "real write committed before socket closed without consuming response bytes")
                        return
                    time.sleep(.03)
                raise AssertionError("unknown write did not commit within the bounded observation window")

    def group_h(self):
        member = self.fixture["member"]
        conversation = self.fixture["conversation"]
        read = self.read_body(member, conversation, [])
        view = self.call("GET", f"/api/conversations/{conversation}/view", session=member)
        view_body = {"requestId": str(uuid.uuid4()), "revisionEpoch": self.epoch, "expectedVersion": view["version"], "changes": {"pinnedRank": 7}}
        pref = self.call("GET", "/api/me/preferences", session=member)
        pref_body = {"requestId": str(uuid.uuid4()), "revisionEpoch": self.epoch, "expectedVersion": pref["version"], "fontPreset": "large"}
        intents = [("PUT", f"/api/conversations/{conversation}/read", read, "sync.read_operations", f"/api/conversations/{conversation}/read-results"), ("PATCH", f"/api/conversations/{conversation}/view", view_body, "sync.conversation_view_operations", f"/api/conversations/{conversation}/view-results"), ("PATCH", "/api/me/preferences", pref_body, "directory.source_preference_operations", "/api/me/preferences-results")]
        self.fixture["unknown_intents"] = intents
        self.save()
        for method, path, body, table, lookup in intents:
            query = f"SELECT count(*) FROM {table} WHERE user_id='{member['user']['id']}' AND request_id='{body['requestId']}'"
            self.discard_after_commit(member, method, path, body, query)
            result = self.call("POST", lookup, body, member)
            self.check(result["status"] == "applied" and self.sql(query) == "1", "only complete original-body lookup reconciles the lost success")
            missing = dict(body, requestId=str(uuid.uuid4()))
            self.call("POST", lookup, missing, member, 404)
            self.check(self.sql(query) == "1", "unknown read-only lookup cannot create or duplicate an original fact")
        self.wait_network_origins()
        fresh = self.snapshot(member)
        self.pages(member, fresh)
        self.validate(member, fresh)
        delta, _ = self.drain(member, fresh["boundaryToken"])
        self.nats_stop()
        try:
            pref = self.call("GET", "/api/me/preferences", session=member)
            self.call("PATCH", "/api/me/preferences", {"requestId": str(uuid.uuid4()), "revisionEpoch": self.epoch, "expectedVersion": pref["version"], "fontPreset": "small"}, member)
            self.check(int(self.sql("SELECT c.revision-p.revision FROM directory.sync_clock c CROSS JOIN directory.sync_publication p")) > 0, "broker outage retains a durable unacknowledged owner pointer")
            offline, events = self.drain(member, delta["cursor"])
            self.check(any(e.get("ref", {}).get("kind") == "preferences" for e in events), "authenticated owner HTTP replay compensates a real NATS outage")
        finally:
            self.nats_start()
        self.wait_network_origins()
        recovered, events = self.drain(member, offline["cursor"])
        self.check(not events and recovered["head"] == offline["head"], "actual NATS restart and duplicate ingest allocate no additional position")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--environment-file", type=Path, required=True)
    parser.add_argument("--group", choices=list("abcdefgh"), required=True)
    parser.add_argument("--report", type=Path, required=True)
    arguments = parser.parse_args()
    environment = json.loads(arguments.environment_file.read_text())
    if not environment.get("s2PhaseB") or environment["serverURL"] != "https://127.0.0.1:50443" or Path(environment["runDir"]).name != "PhaseB":
        raise SystemExit("Explicit new S2 Phase B realm required")
    if shutil.disk_usage(environment["runDir"]).free < 4 * 1024 ** 3:
        raise SystemExit("4GiB free space gate")
    suite = SelfStateSuite(environment, arguments.environment_file)
    started = time.monotonic()
    status, failure = "passed", None
    try:
        getattr(suite, "group_" + arguments.group)()
    except Exception as error:
        status, failure = "failed", str(error)
    finally:
        suite.save()
    report = {"group": arguments.group, "status": status, "httpChecks": suite.checks, "semanticAssertions": suite.assertions, "verifiedPages": suite.verified_pages, "elapsedSeconds": round(time.monotonic() - started, 3), "checkedAt": datetime.now(timezone.utc).isoformat(), "binarySHA256": environment["binarySHA256"], "failure": failure, "assertions": suite.transcript, "strictHTTPS": True, "realPhysicalPGClusters": 2, "qualifiedAck": False, "draftsActivated": False}
    arguments.report.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({key: report[key] for key in ["group", "status", "httpChecks", "semanticAssertions", "verifiedPages", "elapsedSeconds", "failure"]}, ensure_ascii=False))
    if status != "passed":
        raise SystemExit(1)


if __name__ == "__main__":
    main()
