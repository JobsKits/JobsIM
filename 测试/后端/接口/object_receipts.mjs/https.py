#!/usr/bin/env python3
"""Verify original object receipt protocols through an explicitly owned TLS realm."""

import argparse
import base64
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import secrets
import subprocess
import time
import uuid

DIRECTORY = Path(__file__).resolve().parent
SPEC = importlib.util.spec_from_file_location("jobsim_object_realm", DIRECTORY / "realm.py")
REALM = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(REALM)
SPEC = importlib.util.spec_from_file_location("jobsim_object_http", DIRECTORY.parent / "forward_refs.py/forward_refs.py")
HTTP = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(HTTP)


class ObjectSuite(HTTP.ForwardSuite):
    def __init__(self, environment, seed):
        super().__init__(environment)
        self.seed = seed
        self.progress = {"actors": [], "intents": []}
        self.transcript = []
        self.closed = []

    def save(self):
        REALM.save(self.seed, self.progress)

    def check(self, condition, description):
        super().check(condition, description)
        self.transcript.append(description)

    def register(self):
        body = {"username": "object_" + secrets.token_hex(8), "password": "Original object verification " + secrets.token_hex(20), "requestId": str(uuid.uuid4()), "deviceName": "owned-object-protocol-fixture"}
        record = {"originalAuth": body, "status": "initial_unknown"}
        self.progress["actors"].append(record)
        self.save()
        actor = self.call("POST", "/api/auth/register", body)
        record.update({"status": "active", "session": actor})
        self.sessions.append(actor)
        self.save()
        return actor

    def write(self, method, path, body, actor, expect=200):
        record = {"method": method, "path": path, "body": body, "status": "unknown"}
        self.progress["intents"].append(record)
        self.save()
        result = self.call(method, path, body, actor, expect)
        record.update({"status": "responded", "httpStatus": expect})
        self.save()
        return result

    def run(self):
        bootstrap = self.call("GET", "/api/bootstrap")
        self.epoch = bootstrap["restoreEpoch"]
        self.check(bootstrap["deploymentId"] == self.environment["deploymentId"] and self.epoch == self.environment["restoreEpoch"], "fixed current instance and epoch")
        self.check(bootstrap["features"].get("durable_object_receipt_protocol") is True and bootstrap["features"].get("after_receipt") is False, "object protocol does not enable retention deletion")
        actor = self.register()
        store = str(uuid.uuid4())
        self.write("PUT", "/api/sync/device", {"requestId": str(uuid.uuid4()), "storeGeneration": store, "schemaVersion": "2"}, actor)
        profile = json.loads(Path(self.environment["receiptPolicy"]).read_text())["profiles"][0]
        declaration = {"requestId": str(uuid.uuid4()), "storeGeneration": store, "revisionEpoch": self.epoch, "profileId": profile["id"], "buildDigest": profile["buildDigest"], "verificationDigest": profile["verificationDigest"]}
        qualification = self.write("PUT", "/api/sync/storage", declaration, actor)
        self.check(qualification["storage"].get("objectQualified") is True, "explicit combined approved physical store")
        cid = self.write("POST", "/api/conversations", {"kind": "self", "title": "Original object receipt", "requestId": str(uuid.uuid4())}, actor)["id"]
        original = secrets.token_bytes(1 << 20)
        attachment = self.upload(actor, "original.bin", original)
        payload = self.payload(actor, cid, "original object descriptor", type="file", attachmentId=attachment["id"])
        message = self.write("POST", f"/api/conversations/{cid}/messages", payload, actor)
        challenge = self.call("POST", "/api/sync/receipt-challenges", {"messageId": message["id"], "schemaVersion": "2"}, actor)
        envelope = base64.urlsafe_b64decode(challenge["bodyEnvelope"] + "===")
        self.check(hashlib.sha256(envelope).hexdigest() == challenge["bodyDigest"] and json.loads(envelope)["attachmentId"] == attachment["id"], "original descriptor envelope digest and attachment identity")
        parent = {"requestId": str(uuid.uuid4()), "storeGeneration": store, "revisionEpoch": self.epoch, "qualificationId": qualification["result"]["qualificationId"], "kind": "message_content", "challenge": challenge["challenge"], "bodyDigest": challenge["bodyDigest"]}
        self.call("POST", "/api/sync/object-receipt-challenges", {"parentReceiptId": parent["requestId"], "schemaVersion": "2"}, actor, 409)
        recorded = self.write("POST", "/api/sync/receipts", parent, actor)
        self.check(recorded["objectComplete"] is False, "descriptor receipt does not confirm original bytes")
        challenge = self.call("POST", "/api/sync/object-receipt-challenges", {"parentReceiptId": parent["requestId"], "schemaVersion": "2"}, actor)
        self.check(challenge["object"]["sizeBytes"] == str(len(original)) and challenge["object"]["sha256"] == hashlib.sha256(original).hexdigest(), "media owner hashes exact bound original file")
        downloaded, _ = self.binary(actor, "/api/media/" + attachment["id"])
        self.check(downloaded == original, "authenticated TLS original object bytes match")
        # This is an HTTP protocol fixture. The subsequent CLI suite supplies
        # actual native file/parent-directory fsync and FULL SQLite evidence.
        request = {"requestId": str(uuid.uuid4()), "storeGeneration": store, "revisionEpoch": self.epoch, "qualificationId": parent["qualificationId"], "kind": "object_received", "parentReceiptId": parent["requestId"], "challenge": challenge["challenge"], "objectId": attachment["id"], "sizeBytes": challenge["object"]["sizeBytes"], "objectDigest": challenge["object"]["sha256"]}
        self.call("POST", "/api/sync/object-receipt-results", request, actor, 404)
        wrong = dict(request, objectDigest=hashlib.sha256(b"preview").hexdigest())
        self.write("POST", "/api/sync/object-receipts", wrong, actor, 409)
        fact = self.write("POST", "/api/sync/object-receipts", request, actor)
        self.check(fact["objectComplete"] is True and fact["targetRangeComplete"] is False and fact["deletionEligible"] is False, "one original object fact cannot satisfy all targets or delete content")
        self.check(self.call("POST", "/api/sync/object-receipt-results", request, actor) == fact and self.write("POST", "/api/sync/object-receipts", request, actor) == fact, "original object lookup and retry return identical fact")
        self.call("POST", "/api/sync/object-receipt-results", dict(request, sizeBytes="1"), actor, 409)
        self.write("DELETE", "/api/messages/" + message["id"], {"requestId": str(uuid.uuid4()), "expectedVersion": message["version"]}, actor)
        self.check(self.call("POST", "/api/sync/object-receipt-results", request, actor) == fact, "global removal does not rewrite historical object fact")
        self.call("POST", "/api/sync/object-receipt-challenges", {"parentReceiptId": parent["requestId"], "schemaVersion": "2"}, actor, 409)

    def close_owned(self):
        for record in self.progress["actors"]:
            actor = record.get("session")
            if actor is None or record["status"] == "closed":
                continue
            self.call("POST", "/api/auth/logout", {}, actor)
            self.call("GET", "/api/me", session=actor, expect=401)
            record["status"] = "closed"
            self.save()
            self.closed.append({"deviceId": actor["deviceId"], "logout": 200, "originalAccess": 401})


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--environment-file", type=Path, required=True)
    args = parser.parse_args()
    environment = json.loads(args.environment_file.read_text())
    if not environment.get("freshObjectReceiptRealm") or environment["serverURL"] != "https://127.0.0.1:56443":
        raise RuntimeError("Only the separately owned original-object realm is allowed")
    REALM.OWNED.free_space(Path(environment["runDir"]))
    if hashlib.sha256(Path(environment["binary"]).read_bytes()).hexdigest() != environment["binarySHA256"]:
        raise RuntimeError("Frozen candidate digest changed")
    actual = subprocess.run(["/bin/ps", "-p", str(environment["serverPID"]), "-o", "command="], capture_output=True, text=True, timeout=5, check=True).stdout.strip()
    if actual != " ".join(environment["serverCommand"]):
        raise RuntimeError("Owned application PID/argv no longer matches")
    stamp = datetime.now(timezone.utc).strftime("%Y%m%d-%H%M%S") + "-" + secrets.token_hex(3)
    report = REALM.ROOT / "项目相关报告/测试/后端/20261116-完整对象收讫/真实HTTPS" / stamp
    report.mkdir(parents=True)
    suite = ObjectSuite(environment, Path(environment["runDir"]) / "Secrets" / ("protocol-" + stamp + ".json"))
    began = time.monotonic()
    failure = None
    cleanup_failure = None
    try:
        suite.run()
    except Exception as error:
        failure = str(error)
    finally:
        try:
            suite.close_owned()
        except Exception as error:
            cleanup_failure = str(error)
    result = {"status": "passed" if not failure and not cleanup_failure else "failed", "failure": failure, "cleanupFailure": cleanup_failure, "httpChecks": suite.checks, "semanticAssertions": suite.assertions, "elapsedSeconds": round(time.monotonic() - began, 3), "binarySHA256": environment["binarySHA256"], "nativeClientDurability": False, "deletionEligible": False, "closedFamilies": suite.closed, "unresolvedInitialAuth": sum(record["status"] == "initial_unknown" for record in suite.progress["actors"]), "assertions": suite.transcript}
    REALM.save(report / "结果.json", result)
    print(json.dumps({"report": str(report), "status": result["status"]}, ensure_ascii=False))
    if result["status"] != "passed":
        raise SystemExit(1)


if __name__ == "__main__":
    main()
