#!/usr/bin/env python3
"""Exercise an explicitly selected regression deployment through public HTTPS."""

import argparse
import datetime as dt
import json
import pathlib
import secrets
import ssl
import time
import urllib.error
import urllib.request
import uuid


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--state", type=pathlib.Path, required=True)
    parser.add_argument("--deployment", required=True)
    parser.add_argument("--output", type=pathlib.Path, required=True)
    parser.add_argument("--yes", action="store_true", required=True)
    args = parser.parse_args()
    descriptor = json.loads((args.state / "connection.json").read_text())
    if descriptor["deploymentId"] != args.deployment:
        raise RuntimeError("Explicit regression deployment identity differs")
    context = ssl.create_default_context(cafile=str(args.state / "TLS/ca.crt"))
    base = descriptor["url"]
    assertions = []
    args.output.parent.mkdir(parents=True, exist_ok=True)

    def progress(status):
        args.output.write_text(json.dumps({"status": status, "deploymentId": args.deployment,
                               "httpAssertions": len(assertions), "assertions": assertions},
                              ensure_ascii=False, indent=2))

    def call(method, path, body=None, token="", expected=200):
        headers = {"Content-Type": "application/json"}
        if token:
            headers["Authorization"] = "Bearer " + token
        request = urllib.request.Request(base + path, data=json.dumps(body).encode() if body is not None else None,
                                         headers=headers, method=method)
        try:
            response = urllib.request.urlopen(request, context=context, timeout=20)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            status, payload = response.status, response.read()
        assertions.append({"method": method, "path": path, "status": status, "expected": expected})
        progress("running" if status == expected else "failed")
        if status != expected:
            raise RuntimeError(f"HTTPS assertion {len(assertions)} failed: {method} {path}: {status} != {expected}")
        return json.loads(payload) if payload else None

    boot = call("GET", "/api/bootstrap")
    assert boot["deploymentId"] == args.deployment
    call("GET", "/api/health")
    nonce = lambda: str(uuid.uuid4())
    suffix = secrets.token_hex(4)
    password = "JobsIM private regression " + secrets.token_urlsafe(24)
    accounts = []
    for label in ("one", "two", "three"):
        name = "normal_" + label + "_" + suffix
        result = call("POST", "/api/auth/register", {"username": name, "password": password,
                      "requestId": nonce(), "deviceName": "Canonical HTTPS regression"})
        accounts.append(result)
    private = args.state / ("HTTPS-regression-" + suffix + ".json")
    with private.open("x") as stream:
        private.chmod(0o600)
        json.dump({"password": password, "accounts": accounts}, stream)
    one, two, three = accounts
    a, b, c = (account["token"] for account in accounts)
    aid, bid, cid = (account["user"]["id"] for account in accounts)
    friend = call("POST", "/api/contacts/requests", {"userId": bid, "requestId": nonce()}, a)
    call("POST", "/api/contacts/requests/" + friend["id"] + "/accept", {}, b)
    direct = call("POST", "/api/conversations", {"kind": "direct", "title": "", "memberIds": [bid], "requestId": nonce()}, a)

    def conversation(identity, token):
        return call("GET", "/api/conversations/" + identity, token=token)

    def body(identity, token, text):
        return {"clientMsgId": nonce(), "type": "text", "text": text, "restoreEpoch": boot["restoreEpoch"],
                "sendAdmission": conversation(identity, token)["sendAdmission"]}

    original = body(direct["id"], a, "canonical persisted message " + suffix)
    accepted = call("POST", "/api/conversations/" + direct["id"] + "/messages", original, a)
    assert accepted["text"] == original["text"]
    assert call("GET", "/api/messages/" + accepted["id"], token=b)["text"] == original["text"]
    assert call("POST", "/api/conversations/" + direct["id"] + "/messages", original, a)["id"] == accepted["id"]
    group = call("POST", "/api/conversations", {"kind": "group", "title": "Governance regression", "memberIds": [bid], "requestId": nonce()}, a)
    gid = group["id"]
    route = "/api/conversations/" + gid

    def mutate(action, **changes):
        state = call("GET", route + "/governance", token=a)
        intent = {"requestId": nonce(), "action": action, "version": state["version"],
                  "membershipGeneration": state["membershipGeneration"], "roleGeneration": state["roleGeneration"], **changes}
        result = call("POST", route + "/governance", intent, a)
        assert call("POST", route + "/governance", intent, a) == result
        return result

    stale = body(gid, b, "stale policy admission")
    mutate("settings", muteAll=False, slowSeconds=1, maxMembers=1000)
    call("POST", route + "/messages", stale, b, 409)
    first = body(gid, b, "first slow interval message")
    sent = call("POST", route + "/messages", first, b)
    assert call("POST", route + "/messages", first, b)["id"] == sent["id"]
    denied = body(gid, b, "a second nonce during the interval")
    call("POST", route + "/messages", denied, b, 429)
    time.sleep(1.1)
    call("POST", route + "/messages", denied, b)
    members = call("GET", route + "/members?limit=1", token=a)
    assert members["nextCursor"] and len(members["items"]) == 1
    remainder = call("GET", route + "/members?after=" + members["nextCursor"] + "&limit=1", token=a)
    assert len(remainder["items"]) == 1 and not remainder["nextCursor"]

    def target():
        return next(item for item in call("GET", route + "/members", token=a)["items"] if item["id"] == bid)

    member = target()
    before_mute = body(gid, b, "old admission before mute")
    until = (dt.datetime.now(dt.timezone.utc) + dt.timedelta(minutes=30)).isoformat().replace("+00:00", "Z")
    mutate("mute", targetId=bid, targetMembershipGeneration=member["membershipGeneration"],
           targetRoleGeneration=member["roleGeneration"], mutedUntil=until)
    call("POST", route + "/messages", before_mute, b, 409)
    call("GET", route + "/messages", token=b)
    member = target()
    mutate("mute", targetId=bid, targetMembershipGeneration=member["membershipGeneration"],
           targetRoleGeneration=member["roleGeneration"], mutedUntil="")
    call("POST", route + "/messages", before_mute, b, 409)
    mutate("settings", muteAll=True, slowSeconds=0, maxMembers=1000)
    member = target()
    mutate("role", targetId=bid, targetMembershipGeneration=member["membershipGeneration"],
           targetRoleGeneration=member["roleGeneration"], role="admin")
    call("POST", route + "/messages", body(gid, b, "explicit administrator mute-all exemption"), b)
    invite = call("POST", route + "/invites", {"requestId": nonce(), "expiresAt": until,
                  "maxUses": 1, "approvalRequired": True}, a)
    joined = call("POST", route + "/join-requests", {"requestId": nonce(), "inviteId": invite["id"], "token": invite["token"]}, c)
    assert joined["state"] == "pending"
    call("POST", route + "/join-requests/" + joined["id"], {"approve": True}, a)
    assert any(item["id"] == cid for item in call("GET", route + "/members", token=a)["items"])
    call("DELETE", route + "/invites/" + invite["id"], token=a)
    mutate("dissolve")
    closed = conversation(gid, b)
    assert closed["readOnly"] and closed["state"] == "dissolved"
    call("POST", route + "/messages", before_mute, b, 403)
    call("GET", route + "/messages", token=b, expected=403)
    # Direct messages stay usable after all group-only permission transitions.
    call("POST", "/api/conversations/" + direct["id"] + "/messages", body(direct["id"], b, "normal chat still works"), b)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps({"status": "passed", "deploymentId": args.deployment,
                           "httpAssertions": len(assertions), "assertions": assertions}, ensure_ascii=False, indent=2))
    print(json.dumps({"status": "passed", "httpAssertions": len(assertions)}))


if __name__ == "__main__":
    main()
