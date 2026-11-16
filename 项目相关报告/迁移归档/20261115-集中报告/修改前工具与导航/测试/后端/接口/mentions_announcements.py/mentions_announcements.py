#!/usr/bin/env python3
"""Exercise structured mentions, reference announcements and read-only result lookup."""

import argparse
import concurrent.futures
import hashlib
import json
import os
from pathlib import Path
import secrets
import ssl
import subprocess
import time
import urllib.error
import urllib.request
import uuid
from datetime import datetime, timedelta, timezone


class Suite:
    def __init__(self, environment):
        self.environment = environment
        self.base = environment["serverURL"]
        if not self.base.startswith("https://"):
            raise ValueError("Strict HTTPS instance required")
        self.trust = ssl.create_default_context(cafile=environment["caPath"])
        self.checks = 0
        self.assertions = 0
        self.sessions = []
        self.epoch = ""

    def check(self, condition, description):
        if not condition:
            raise AssertionError(description)
        self.assertions += 1

    def call(self, method, path, body=None, session=None, expect=200):
        headers = {"Content-Type": "application/json"}
        if session is not None:
            headers["Authorization"] = "Bearer " + session["token"]
        request = urllib.request.Request(
            self.base + path,
            method=method,
            data=None if body is None else json.dumps(body).encode(),
            headers=headers,
        )
        try:
            with urllib.request.urlopen(request, context=self.trust, timeout=25) as response:
                status, raw = response.status, response.read()
        except urllib.error.HTTPError as error:
            status, raw = error.code, error.read()
        if status != expect:
            try:
                code = json.loads(raw).get("error", {}).get("code", "unknown")
            except (ValueError, AttributeError):
                code = "non-json"
            raise AssertionError(f"{method} {path}: expected {expect}, received {status}/{code}; body redacted")
        self.checks += 1
        return json.loads(raw) if raw else None

    def register(self, label):
        session = self.call("POST", "/api/auth/register", {
            "username": "mention_" + label + secrets.token_hex(5),
            "password": "Mention integration " + secrets.token_hex(16),
            "requestId": str(uuid.uuid4()),
            "deviceName": "mention-announcement-test",
        })
        self.sessions.append(session)
        return session

    def friend(self, sender, receiver):
        request = self.call("POST", "/api/contacts/requests", {
            "userId": receiver["user"]["id"], "requestId": str(uuid.uuid4()),
        }, sender)
        self.call("POST", "/api/contacts/requests/" + request["id"] + "/accept", {}, receiver)

    def conversation(self, session, conversation_id):
        return self.call("GET", "/api/conversations/" + conversation_id, session=session)

    def payload(self, session, conversation_id, text="structured source", **extra):
        current = self.conversation(session, conversation_id)
        return {
            "clientMsgId": str(uuid.uuid4()), "type": "text", "text": text,
            "restoreEpoch": self.epoch, "sendAdmission": current["sendAdmission"], **extra,
        }

    def send(self, session, conversation_id, body, expect=200):
        return self.call("POST", "/api/conversations/" + conversation_id + "/messages", body, session, expect)

    def message(self, session, message_id, expect=200):
        return self.call("GET", "/api/messages/" + message_id, session=session, expect=expect)

    def announcement(self, session, conversation_id, expect=200):
        return self.call("GET", "/api/conversations/" + conversation_id + "/announcement", session=session, expect=expect)

    def announce_body(self, session, conversation_id, source, **extra):
        current = self.announcement(session, conversation_id)
        member = self.conversation(session, conversation_id)
        return {
            "messageId": source["id"], "sourceVersion": source.get("contentVersion", source["version"]),
            "expectedVersion": current["version"],
            "membershipGeneration": member["membershipGeneration"],
            "requestId": str(uuid.uuid4()), **extra,
        }

    def put_announcement(self, session, conversation_id, body, expect=200):
        return self.call("PUT", "/api/conversations/" + conversation_id + "/announcement", body, session, expect)

    def sql(self, command):
        operator_env = dict(os.environ)
        if self.environment.get('pgPassFile'):
            operator_env['PGPASSFILE'] = self.environment['pgPassFile']
        result = subprocess.run([
            str(Path(self.environment["pgBin"]) / "psql"),
            self.environment["businessURL"], "-XAtc", command,
        ], capture_output=True, text=True, check=False, env=operator_env)
        if result.returncode != 0:
            raise AssertionError("Private fixture query failed; details redacted")
        return result.stdout.strip()

    def run(self):
        bootstrap = self.call("GET", "/api/bootstrap")
        self.epoch = bootstrap["restoreEpoch"]
        self.check(bootstrap["features"].get("mentions") is True and bootstrap["features"].get("announcements") is True, "implemented features are discoverable")
        owner, member, other, outsider = [self.register(label) for label in ("a", "b", "c", "d")]
        capabilities = self.call("GET", "/api/capabilities", session=owner)
        self.check(capabilities["features"].get("mentions") is True and capabilities["features"].get("announcements") is True, "authenticated capabilities keep built-in features")
        for peer in (member, other):
            self.friend(owner, peer)
        owner_id, member_id, other_id = [s["user"]["id"] for s in (owner, member, other)]
        group = self.call("POST", "/api/conversations", {
            "kind": "group", "title": "Structured mention reference", "memberIds": [member_id, other_id],
        }, owner)
        conversation_id = group["id"]
        prefix = "/api/conversations/" + conversation_id
        empty = self.announcement(owner, conversation_id)
        self.check(empty["state"] == "empty" and empty["version"] == "0" and empty["canManage"], "initial empty announcement")
        self.check(not self.announcement(member, conversation_id)["canManage"], "member cannot manage announcement")
        self.announcement(outsider, conversation_id, 403)

        plain = self.send(owner, conversation_id, self.payload(owner, conversation_id, "@nickname is ordinary text"))
        self.check(not self.message(member, plain["id"])["mentionedMe"], "plain text must not create a mention")
        original = self.payload(owner, conversation_id, mentionedUserIds=[member_id])
        explicit = self.send(owner, conversation_id, original)
        received = self.message(member, explicit["id"])
        self.check(received["mentionedMe"] and received["mentionedUserIds"] == [member_id], "selected current member receives mention")
        self.check(not self.message(other, explicit["id"])["mentionedMe"], "unselected member not mentioned")
        self.check(not explicit["mentionedMe"], "sender not self-reminded")
        self.check(self.send(owner, conversation_id, original)["id"] == explicit["id"], "same message retry has one fact")
        self.send(owner, conversation_id, dict(original, mentionedUserIds=[other_id]), 409)
        self.send(owner, conversation_id, self.payload(owner, conversation_id, mentionedUserIds=[member_id, member_id]), 400)
        self.send(owner, conversation_id, self.payload(owner, conversation_id, mentionedUserIds=[outsider["user"]["id"]]), 400)
        self.send(owner, conversation_id, self.payload(owner, conversation_id, mentionAll=True, mentionedUserIds=[member_id]), 400)
        self.send(member, conversation_id, self.payload(member, conversation_id, mentionAll=True), 403)
        first_all_body = self.payload(owner, conversation_id, mentionAll=True)
        all_message = self.send(owner, conversation_id, first_all_body)
        self.check(self.message(other, all_message["id"])["mentionedMe"], "owner all targets real current members")
        self.send(owner, conversation_id, first_all_body)
        self.send(owner, conversation_id, self.payload(owner, conversation_id, mentionAll=True))
        rejected = self.send(owner, conversation_id, self.payload(owner, conversation_id, mentionAll=True), 429)
        self.check(rejected["error"]["code"] == "MENTION_RATE_LIMIT", "all rate independent and retries free")
        self.send(owner, conversation_id, self.payload(owner, conversation_id, mentionedUserIds=[other_id]))
        self.send(owner, conversation_id, self.payload(owner, conversation_id, mentionedUserIds=[str(uuid.uuid4()) for _ in range(21)]), 400)

        self.call("PUT", "/api/blocks/" + owner_id, {}, member)
        blocked = self.message(member, explicit["id"])
        self.check(not blocked["mentionedMe"] and blocked["text"], "block suppresses targeted reminder without hiding common group body")
        self.call("DELETE", "/api/blocks/" + owner_id, session=member)
        edited = self.call("PATCH", "/api/messages/" + explicit["id"], {
            "text": "edited original selected mention", "expectedVersion": explicit["version"], "requestId": str(uuid.uuid4()),
        }, owner)
        self.check(edited["id"] == explicit["id"] and self.message(member, explicit["id"])["mentionedMe"], "edit preserves original message identity and mention qualification")
        self.check(self.sql("SELECT count(*) FROM message.mentions WHERE message_id='" + explicit["id"] + "'") == "1", "edit creates no duplicate mention event")

        lookup_body = self.payload(owner, conversation_id, "never submitted lookup")
        absent = self.call("POST", prefix + "/message-results", lookup_body, owner, 404)
        self.check(absent["error"]["code"] == "MESSAGE_RESULT_NOT_FOUND", "lookup never creates unsubmitted valid-admission intent")
        self.check(self.sql("SELECT count(*) FROM message.messages WHERE sender_id='" + owner_id + "' AND client_msg_id='" + lookup_body["clientMsgId"] + "'") == "0", "lookup no write")
        self.check(self.call("POST", prefix + "/message-results", original, owner)["id"] == explicit["id"], "lookup returns authoritative edited current original fact")
        self.call("POST", prefix + "/message-results", dict(original, text="changed original"), owner, 409)
        self.call("POST", prefix + "/message-results", original, other, 404)
        self.call("POST", prefix + "/message-results", original, outsider, 403)

        announcement_source = self.send(owner, conversation_id, self.payload(owner, conversation_id, "announcement source body"))
        body = self.announce_body(owner, conversation_id, announcement_source)
        applied = self.put_announcement(owner, conversation_id, body)
        self.check(applied["current"] and applied["result"]["version"] == "1" and "message" not in applied["result"], "publication stores metadata only")
        self.check(self.put_announcement(owner, conversation_id, body)["result"]["version"] == "1", "lost-response retry cannot republish")
        operation_path = prefix + "/announcement/operations/" + body["requestId"]
        self.check(self.call("GET", operation_path, session=owner)["current"], "original operation query")
        self.put_announcement(owner, conversation_id, dict(body, sourceVersion="2"), 409)
        active = self.announcement(member, conversation_id)
        self.check(active["state"] == "active" and active["message"]["text"] == announcement_source["text"], "member resolves current source")
        self.put_announcement(member, conversation_id, self.announce_body(member, conversation_id, announcement_source), 403)
        self.put_announcement(owner, conversation_id, dict(self.announce_body(owner, conversation_id, announcement_source), membershipGeneration="999"), 409)
        self.put_announcement(owner, conversation_id, dict(self.announce_body(owner, conversation_id, announcement_source), expectedVersion="0"), 409)
        self.put_announcement(owner, conversation_id, dict(self.announce_body(owner, conversation_id, announcement_source), sourceVersion="999"), 409)
        self.put_announcement(owner, conversation_id, dict(self.announce_body(owner, conversation_id, announcement_source), expectedVersion="01"), 400)
        self.put_announcement(owner, conversation_id, self.announce_body(owner, conversation_id, announcement_source, expiresAt=(datetime.now(timezone.utc) - timedelta(seconds=1)).isoformat()), 400)

        original_content = announcement_source["contentVersion"]
        self.call("PUT", "/api/messages/" + announcement_source["id"] + "/favorite", {}, member)
        self.call("POST", "/api/messages/" + announcement_source["id"] + "/reactions", {"emoji": "ok"}, other)
        self.call("PUT", "/api/messages/" + announcement_source["id"] + "/pin", {}, owner)
        unchanged = self.announcement(member, conversation_id)
        self.check(unchanged["state"] == "active" and unchanged["message"]["contentVersion"] == original_content, "personal favorite, reaction and pin cannot invalidate announcement content")
        self.call("DELETE", "/api/messages/" + announcement_source["id"] + "/favorite", session=member)
        self.call("DELETE", "/api/messages/" + announcement_source["id"] + "/reactions/ok", session=other)
        self.call("DELETE", "/api/messages/" + announcement_source["id"] + "/pin", session=owner)
        announcement_source = self.message(owner, announcement_source["id"])
        self.check(announcement_source["contentVersion"] == original_content and announcement_source["version"] != original_content, "content and interaction versions are separate")
        second = self.put_announcement(owner, conversation_id, self.announce_body(owner, conversation_id, announcement_source))
        self.check(second["result"]["version"] == "2" and not self.call("GET", operation_path, session=owner)["current"], "new publication supersedes original operation")
        self.check(not self.put_announcement(owner, conversation_id, body)["current"], "old retry never rolls back newer announcement")
        edited_source = self.call("PATCH", "/api/messages/" + announcement_source["id"], {
            "text": "new source body must not appear under old sourceVersion", "expectedVersion": announcement_source["version"], "requestId": str(uuid.uuid4()),
        }, owner)
        invalid = self.announcement(member, conversation_id)
        self.check(invalid["state"] == "unavailable" and "message" not in invalid, "source edit invalidates old reference without old body")
        refreshed = self.put_announcement(owner, conversation_id, self.announce_body(owner, conversation_id, edited_source))
        self.check(self.announcement(other, conversation_id)["message"]["contentVersion"] == edited_source["contentVersion"], "explicit refresh binds new source version")
        self.call("DELETE", "/api/messages/" + announcement_source["id"], session=owner)
        invalid = self.announcement(member, conversation_id)
        self.check(invalid["state"] == "unavailable" and "message" not in invalid, "source recall leaves no cached body")
        clear = {
            "expectedVersion": refreshed["result"]["version"], "membershipGeneration": group["membershipGeneration"], "requestId": str(uuid.uuid4()),
        }
        cleared = self.call("DELETE", prefix + "/announcement", clear, owner)
        self.check(cleared["result"]["state"] == "empty" and self.announcement(member, conversation_id)["state"] == "empty", "clear reference only")
        self.check(self.call("DELETE", prefix + "/announcement", clear, owner)["current"], "clear retries immutable original")

        fresh = self.send(owner, conversation_id, self.payload(owner, conversation_id, "source before member rejoin"))
        self.put_announcement(owner, conversation_id, self.announce_body(owner, conversation_id, fresh))
        old_generation = self.conversation(member, conversation_id)["membershipGeneration"]
        self.call("DELETE", prefix + "/members/" + member_id + "?membershipGeneration=" + old_generation, session=owner)
        self.announcement(member, conversation_id, 403)
        self.call("POST", prefix + "/members", {"userId": other_id}, owner)
        # Re-add needs owner friendship after the earlier block lifecycle.
        self.call("POST", "/api/contacts/" + owner_id + "/reopen", {"requestId": str(uuid.uuid4())}, member)
        self.friend(owner, member)
        self.call("POST", prefix + "/members", {"userId": member_id}, owner)
        self.check(self.conversation(member, conversation_id)["membershipGeneration"] != old_generation, "real rejoin advances generation")
        invalid = self.announcement(member, conversation_id)
        self.check(invalid["state"] == "unavailable" and "message" not in invalid, "rejoin cannot read pre-join announcement source")
        self.message(member, explicit["id"], 403)
        history = self.call("GET", prefix + "/messages?after=" + self.epoch + ":0", session=member)
        self.check(all(m["id"] != explicit["id"] for m in history["items"]), "old mention not inherited through rejoin")
        newest = self.send(owner, conversation_id, self.payload(owner, conversation_id, "fresh source", mentionedUserIds=[member_id]))
        self.check(self.message(member, newest["id"])["mentionedMe"], "new generation can receive fresh explicit mention")
        expiring = self.announce_body(owner, conversation_id, newest, expiresAt=(datetime.now(timezone.utc) + timedelta(seconds=1)).isoformat())
        self.put_announcement(owner, conversation_id, expiring)
        time.sleep(1.2)
        expired = self.announcement(member, conversation_id)
        self.check(expired["state"] == "expired" and "message" not in expired, "expiry never returns old source body")

        other_group = self.call("POST", "/api/conversations", {"kind": "group", "title": "Other source", "memberIds": [other_id]}, owner)
        other_source = self.send(owner, other_group["id"], self.payload(owner, other_group["id"]))
        self.put_announcement(owner, conversation_id, self.announce_body(owner, conversation_id, other_source), 409)
        self.call("POST", "/api/conversations/" + other_group["id"] + "/message-results", original, owner, 409)
        # This fixture sets only the group restriction; it does not claim the
        # administrator lifecycle path is exercised by this focused suite.
        self.sql("UPDATE conversation.conversations SET state='owner_unavailable' WHERE id='" + conversation_id + "'")
        try:
            self.check(not self.announcement(owner, conversation_id)["canManage"], "restricted group has no announcement management")
            rejected = self.put_announcement(owner, conversation_id, self.announce_body(owner, conversation_id, newest), 409)
            self.check(rejected["error"]["code"] == "GROUP_OWNER_UNAVAILABLE", "owner unavailable refuses new announcement writes")
        finally:
            self.sql("UPDATE conversation.conversations SET state='active' WHERE id='" + conversation_id + "'")
        current_operation = self.put_announcement(owner, conversation_id, self.announce_body(owner, conversation_id, newest))
        self.call("PATCH", prefix, {"ownerId": other_id}, owner)
        self.put_announcement(owner, conversation_id, self.announce_body(owner, conversation_id, newest), 403)
        original_operation = prefix + "/announcement/operations/" + current_operation["operationId"]
        self.check(self.call("GET", original_operation, session=owner)["current"], "former owner retains own committed result under current read ACL")
        self.put_announcement(other, conversation_id, self.announce_body(other, conversation_id, newest))
        self.check(not self.call("GET", original_operation, session=owner)["current"], "new owner change supersedes original result")

        rate_group = self.call("POST", "/api/conversations", {"kind": "group", "title": "Explicit rate", "memberIds": [other_id]}, owner)
        # Keep this bounded burst inside one configured default fixed window.
        remaining = 60 - time.time() % 60
        if remaining < 8:
            time.sleep(remaining + .02)
        rate_body = self.payload(owner, rate_group["id"], mentionedUserIds=[other_id])
        self.send(owner, rate_group["id"], rate_body)
        self.send(owner, rate_group["id"], rate_body)
        for _ in range(19):
            self.send(owner, rate_group["id"], dict(rate_body, clientMsgId=str(uuid.uuid4())))
        limited = self.send(owner, rate_group["id"], dict(rate_body, clientMsgId=str(uuid.uuid4())), 429)
        self.check(limited["error"]["code"] == "MENTION_RATE_LIMIT", "explicit member rate enforced independently")

        # Hold the parent row while publication waits. Source edits must wait on
        # the same-host fence, and the source RPC must not callback into the lock.
        self.race_source_edit(other, conversation_id, newest, owner)
        return {
            "status": "passed", "httpAssertions": self.checks, "semanticAssertions": self.assertions,
            "strictHTTPS": True, "structuredMentions": True, "memberGenerationBound": True,
            "blockSuppressesTargetedReminder": True, "mentionAllOwnerAndIndependentRate": True,
            "announcementReferenceNoStoredBody": True, "sourceEditRecallExpiryAndJoinBoundary": True,
            "immutableOriginalOperation": True, "readOnlyMessageResultNeverCreates": True,
            "sameHostFenceSerializesSourceEdit": True, "interactionDoesNotInvalidateAnnouncementContent": True,
            "scope": "Real HTTPS and PostgreSQL; no frontend, hardware, push or distributed-host acceptance claim",
        }

    def race_source_edit(self, owner, conversation_id, source, author):
        command = [str(Path(self.environment["pgBin"]) / "psql"), self.environment["businessURL"], "-XAt"]
        private_env = dict(os.environ)
        if self.environment.get('pgPassFile'):
            private_env['PGPASSFILE'] = self.environment['pgPassFile']
        blocker = subprocess.Popen(command, env=private_env, stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True, bufsize=1)
        try:
            blocker.stdin.write("BEGIN; SELECT id FROM conversation.conversations WHERE id='" + conversation_id + "' FOR UPDATE; SELECT 'locked';\n")
            blocker.stdin.flush()
            while blocker.stdout.readline().strip() != "locked":
                if blocker.poll() is not None:
                    raise AssertionError("Parent row fixture unavailable")
            body = self.announce_body(owner, conversation_id, source)
            with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
                publication = pool.submit(self.put_announcement, owner, conversation_id, body)
                for _ in range(100):
                    waiting = self.sql("SELECT count(*) FROM pg_stat_activity WHERE application_name='jobsim-conversation' AND wait_event_type='Lock'")
                    if int(waiting) > 0:
                        break
                    time.sleep(.02)
                else:
                    raise AssertionError("Publication did not reach controlled parent-row lock")
                editing = pool.submit(self.call, "PATCH", "/api/messages/" + source["id"], {
                    "text": "edited after announcement commit", "expectedVersion": source["version"], "requestId": str(uuid.uuid4()),
                }, author)
                time.sleep(.15)
                self.check(not editing.done(), "source edit must not cross active publication fence")
                blocker.stdin.write("COMMIT;\n\\q\n")
                blocker.stdin.flush()
                publication.result()
                editing.result()
            invalid = self.announcement(owner, conversation_id)
            self.check(invalid["state"] == "unavailable" and "message" not in invalid, "ordered source edit invalidates committed reference")
        finally:
            if blocker.poll() is None:
                blocker.stdin.close()
                blocker.wait(timeout=5)

    def logout(self):
        for session in self.sessions:
            try:
                self.call("POST", "/api/auth/logout", {}, session)
            except Exception:
                pass


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--environment-file", type=Path, required=True)
    parser.add_argument("--report", type=Path)
    args = parser.parse_args()
    environment = json.loads(args.environment_file.read_text())
    suite = Suite(environment)
    try:
        report = suite.run()
        report["testedAtUTC"] = datetime.now(timezone.utc).isoformat()
        project = Path(__file__).resolve().parents[4]
        report["backendSHA256"] = {
            name: hashlib.sha256((project / "代码/后端/Go" / name).read_bytes()).hexdigest()
            for name in ("message.go", "mentions_announcements.go", "types.go", "conversation.go", "control.go", "send_admission.go", "operations.go", "server.go")
        }
        report["scriptSHA256"] = hashlib.sha256(Path(__file__).read_bytes()).hexdigest()
        report["ownerUnavailableFixture"] = "direct PostgreSQL group-state fixture; administrator lifecycle not exercised here"
        destination = args.report or Path(__file__).resolve().parents[2] / "报告/提及与公告实测.json"
        destination.parent.mkdir(parents=True, exist_ok=True)
        data = (json.dumps(report, ensure_ascii=False, indent=2) + "\n").encode()
        destination.write_bytes(data)
        destination.with_suffix(destination.suffix + ".sha256").write_text(hashlib.sha256(data).hexdigest() + "  " + destination.name + "\n")
        print(json.dumps(report, ensure_ascii=False))
    finally:
        suite.logout()


if __name__ == "__main__":
    main()
