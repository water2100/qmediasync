"""Cookie-only 115 download/upload adapter for the Go service.

The request (including Cookie) arrives on stdin.  Stdout contains only a
machine-readable result; exception details and credentials are never emitted.
"""

import json
import sys

from p115client import P115Client, check_response
from p115client.exception import P115AuthenticationError, P115LoginError


def main() -> None:
    request = json.load(sys.stdin)
    cookie = request.get("cookie") or ""
    if not cookie:
        print(json.dumps({"ok": False, "error": "login_invalid"}))
        return

    client = P115Client(
        cookies=cookie,
        app=request.get("login_app") or "",
        console_qrcode=False,
    )
    operation = request.get("operation")
    if operation == "download_url":
        link = client.download_url(
            request["pickcode"], user_agent=request.get("user_agent") or ""
        )
        headers = getattr(link, "headers", {}) or {}
        requires_cookie = any(
            key.lower() == "cookie" and bool(value)
            for key, value in headers.items()
        )
        print(
            json.dumps(
                {
                    "ok": True,
                    "url": str(link),
                    "requires_cookie": requires_cookie,
                }
            )
        )
    elif operation == "upload_file":
        response = client.upload_file(
            request["file_path"],
            pid=request["parent_id"],
            partsize=10485760,
        )
        check_response(response)
        data = response.get("data") or {}
        file_id = data.get("file_id") or data.get("id") or ""
        print(json.dumps({"ok": bool(file_id), "file_id": str(file_id), "error": "missing_file_id" if not file_id else ""}))
    else:
        print(json.dumps({"ok": False, "error": "unknown_operation"}))


if __name__ == "__main__":
    try:
        main()
    except (P115AuthenticationError, P115LoginError):
        print(json.dumps({"ok": False, "error": "login_invalid"}))
    except Exception:
        print(json.dumps({"ok": False, "error": "operation_failed"}))
