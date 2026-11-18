#!/usr/bin/env python3
"""Exercise only the explicitly paired NativeDemo-v1 via strict-CA HTTPS.

Credentials remain in RAM; output contains checks and public IDs, never secrets.
"""

import argparse
import hashlib
import json
from pathlib import Path
import secrets
import ssl
import struct
import urllib.error
import urllib.parse
import urllib.request
import uuid


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--descriptor", required=True, type=Path)
    parser.add_argument("--gif-source", required=True, type=Path)
    parser.add_argument("--report", required=True, type=Path)
    args = parser.parse_args()
    descriptor = json.loads(args.descriptor.read_text())
    assert set(descriptor) == {"controllerURL", "caSha256", "caCertificatePEM", "demo"}
    assert descriptor["demo"] is True
    controller = descriptor["controllerURL"]
    parsed = urllib.parse.urlsplit(controller)
    assert parsed.scheme == "https" and parsed.port == 19544 and not parsed.username
    fingerprint = hashlib.sha256(ssl.PEM_cert_to_DER_cert(descriptor["caCertificatePEM"])).hexdigest()
    assert fingerprint == descriptor["caSha256"]
    context = ssl.create_default_context(cadata=descriptor["caCertificatePEM"])
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), urllib.request.HTTPSHandler(context=context))
    checks = []
    tokens = []

    def call(base, method, path, body=None, token=None, expected=200, raw=None, content_type="application/json", binary=False):
        headers = {"Content-Type": content_type}
        if token:
            headers["Authorization"] = "Bearer " + token
        data = raw if raw is not None else (json.dumps(body).encode() if body is not None else None)
        request = urllib.request.Request(base + path, data=data, headers=headers, method=method)
        try:
            with opener.open(request, timeout=20) as response:
                status, content, mime = response.status, response.read(), response.headers.get_content_type()
        except urllib.error.HTTPError as error:
            status, content, mime = error.code, error.read(), error.headers.get_content_type()
        assert status == expected, (method, path, status, "response redacted")
        checks.append({"method": method, "path": path, "status": status})
        if binary:
            return content, mime
        return json.loads(content) if content else {}

    def upload(base, token, data, version, operation):
        boundary = "JobsIMFixedGIFFixture" + secrets.token_hex(12)
        parts = []
        for name, value in [("requestId", operation), ("expectedVersion", version)]:
            parts.append((f'--{boundary}\r\nContent-Disposition: form-data; name="{name}"\r\n\r\n{value}\r\n').encode())
        parts.append((f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="two-frames.gif"\r\nContent-Type: image/gif\r\n\r\n').encode() + data)
        parts.append((f'\r\n--{boundary}--\r\n').encode())
        return call(base, "POST", "/api/me/avatar", token=token, raw=b"".join(parts), content_type="multipart/form-data; boundary=" + boundary)

    status = call(controller, "GET", "/api/demo/v1/status")
    assert status["state"] == "ready" and "accounts" not in status
    call(controller, "POST", "/api/demo/v1/ensure-start", {"command": "forbidden"}, expected=400)
    call(controller, "POST", "/api/demo/v1/stop", {}, expected=404, binary=True)
    ready = call(controller, "POST", "/api/demo/v1/ensure-start", {})
    assert ready["state"] == "ready" and len(ready["accounts"]) == 3
    base = ready["connection"]["serverURL"]
    server = urllib.parse.urlsplit(base)
    assert server.scheme == "https" and server.hostname == parsed.hostname and server.port == 19543
    assert ready["connection"]["caSha256"] == fingerprint
    bootstrap = call(base, "GET", "/api/bootstrap")
    assert bootstrap["deploymentId"] == ready["connection"]["deploymentId"]
    html, mime = call(base, "GET", "/", binary=True)
    assert mime == "text/html" and b"<html" in html.lower()
    suffix = secrets.token_hex(5)
    try:
        users = []
        for index in range(3):
            result = call(base, "POST", "/api/auth/register", {
                "username": f"gifcheck{index}_{suffix}", "password": secrets.token_urlsafe(28),
                "requestId": str(uuid.uuid4()), "deviceName": "dedicated-profile-GIF-HTTPS-check",
            })
            assert result["user"].get("admin", False) is False
            tokens.append(result["token"])
            users.append(result["user"])
        me = call(base, "GET", "/api/me", token=tokens[0])
        assert me["createdAt"] and me["contactBindings"]["email"]["state"] == "unbound"
        changed = call(base, "PATCH", "/api/me", {"displayName": "HTTPS资料核验", "signature": "签名🙂"}, tokens[0])
        assert changed["signature"] == "签名🙂"
        me = call(base, "GET", "/api/me", token=tokens[0])
        assert me["signature"] == "签名🙂" and me["displayName"] == "HTTPS资料核验"
        operation = str(uuid.uuid4())
        source = args.gif_source.read_bytes()
        first = upload(base, tokens[0], source, me.get("profileVersion", "1"), operation)
        assert first["status"] == "applied" and first["current"] is True
        avatar = first["user"]["avatarURL"]
        data, mime = call(base, "GET", avatar, token=tokens[0], binary=True)
        assert mime == "image/gif" and data.startswith((b"GIF87a", b"GIF89a"))
        assert struct.unpack("<HH", data[6:10]) == (512, 512)
        args.report.with_suffix(".live-avatar.gif").write_bytes(data)
        # DecodeAll semantic checks are native Go tests. This verifies MIME and actual live bytes.
        preview, mime = call(base, "GET", first["user"]["avatarThumbnailURL"], token=tokens[0], binary=True)
        assert mime == "image/jpeg" and preview.startswith(b"\xff\xd8")
        replay = upload(base, tokens[0], source, me.get("profileVersion", "1"), operation)
        assert replay["result"] == first["result"]
        call(base, "GET", avatar, token=tokens[1], expected=403)
        request = call(base, "POST", "/api/contacts/requests", {"userId": users[1]["id"], "requestId": str(uuid.uuid4())}, tokens[0])
        call(base, "POST", "/api/contacts/requests/" + request["id"] + "/accept", {}, tokens[1])
        call(base, "GET", avatar, token=tokens[1], binary=True)
        peer = call(base, "GET", "/api/users/" + users[0]["id"] + "/profile", token=tokens[1])
        assert peer["contactBindings"]["email"] == {"state": "hidden"}
        assert peer["contactBindings"]["phone"] == {"state": "hidden"}
        call(base, "PUT", "/api/blocks/" + users[1]["id"], {}, tokens[0])
        call(base, "GET", avatar, token=tokens[1], expected=403)
        call(base, "GET", "/api/users/" + users[0]["id"] + "/profile", token=tokens[1], expected=403)
        call(base, "GET", "/api/users/" + users[0]["id"] + "/profile", token=tokens[2], expected=403)
        report = {
            "status": "passed", "deploymentId": bootstrap["deploymentId"],
            "strictCAAndHost": True, "dedicatedPorts": [19543, 19544],
            "nativeControllerFixedActions": True, "statusOmitsCredentials": True,
            "normalNonAdminProfilesAndGIF": True, "actualAvatarSHA256": hashlib.sha256(data).hexdigest(),
            "gifSourceSHA256": hashlib.sha256(source).hexdigest(), "checks": checks,
            "scope": "Dedicated NativeDemo-v1 only; no default instance, admin or iPhone verification",
        }
        args.report.write_text(json.dumps(report, ensure_ascii=False, indent=2) + "\n")
        print(json.dumps({"status": "passed", "httpChecks": len(checks), "deploymentId": bootstrap["deploymentId"]}))
    finally:
        for token in tokens:
            try:
                call(base, "POST", "/api/auth/logout", {}, token)
            except Exception:
                pass


if __name__ == "__main__":
    main()
