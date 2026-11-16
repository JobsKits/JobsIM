#!/usr/bin/env python3
"""Decode a final formal ZIP and compare five real owned media sources byte for byte."""

import argparse
from datetime import datetime, timezone
import hashlib
import importlib.util
import json
from pathlib import Path
import uuid


SPEC = importlib.util.spec_from_file_location("personal_all_media", Path(__file__).with_name("personal_exports.py"))
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


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
        old = suite.fixture["finalFormalActor"]
        if "allMediaLoginBody" not in suite.fixture:
            suite.fixture["allMediaLoginBody"] = {"username": old["user"]["username"], "password": suite.passwords[old["user"]["id"]], "requestId": str(uuid.uuid4()), "deviceName": "final-owned-all-media"}
            suite.save()
        actor = suite.call("POST", "/api/auth/login", suite.fixture["allMediaLoginBody"])
        suite.fixture["allMediaActor"] = actor
        suite.save()
        if "allMediaConversation" not in suite.fixture:
            suite.fixture["allMediaConversation"] = suite.call("POST", "/api/conversations", {"kind": "self"}, actor)["id"]
            suite.save()
        conversation = suite.fixture["allMediaConversation"]
        work = Path(info["runDir"]) / "Run/MediaSamples"
        source_list = [("image.png", "image"), ("animated.gif", "image"), ("voice.m4a", "audio"), ("clip.mp4", "video"), ("ordinary-file.txt", "file")]
        for name, kind in source_list:
            samples = suite.fixture.setdefault("allMediaSamples", {})
            sample = samples.setdefault(name, {})
            data = (work / name).read_bytes() if kind != "file" else b"Real ordinary formal file bytes\n" * 97
            if "asset" not in sample:
                sample["asset"] = suite.upload(actor, name, data)
                sample["digest"] = hashlib.sha256(data).hexdigest()
                suite.save()
            if "body" not in sample:
                sample["body"] = suite.payload(actor, conversation, name, type=kind, attachmentId=sample["asset"]["id"])
                suite.save()
            if "message" not in sample:
                sample["message"] = suite.send(actor, conversation, sample["body"])
                suite.save()
        task = suite.named_task("finalAllMedia", actor)
        archive, _ = suite.download(actor, task["job"]["id"])
        suite.check(archive.testzip() is None, "all actual ZIP entries pass CRC after the complete encrypted authentication pass")
        for name, kind in source_list:
            sample = suite.fixture["allMediaSamples"][name]
            content = archive.read("Media/" + sample["asset"]["id"])
            message = json.loads(archive.read("Messages/" + conversation + "/" + sample["message"]["id"] + ".json"))
            suite.check(hashlib.sha256(content).hexdigest() == sample["digest"] and message["type"] == kind, "actual " + name + " is exported with exact original authenticated bytes and message type")
        suite.call("POST", "/api/me/data-exports/" + task["job"]["id"] + "/cancel", {"requestId": str(uuid.uuid4())}, actor)
        suite.call("POST", "/api/auth/logout", {}, actor)
        suite.call("GET", "/api/me", session=actor, expect=401)
    except Exception as error:
        failure = str(error)
    finally:
        suite.save()
    result = {"status": "passed" if failure is None else "failed", "failure": failure, "httpChecks": suite.checks, "semanticAssertions": suite.assertions, "binarySHA256": info["binarySHA256"], "scriptSHA256": hashlib.sha256(Path(__file__).read_bytes()).hexdigest(), "actualFixtures": ["PNG", "animated GIF", "AAC M4A", "MP4 video", "ordinary file"], "mediaToolsDownloaded": False, "checkedAt": datetime.now(timezone.utc).isoformat(), "assertions": suite.transcript}
    args.report.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({key: value for key, value in result.items() if key != "assertions"}, ensure_ascii=False))
    if failure:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
