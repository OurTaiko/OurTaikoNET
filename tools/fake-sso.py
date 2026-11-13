"""A stand-in account center for comparisons: every player has a nickname made from their id,
nobody can sign in. Both servers being compared ask this one, so they see the same names."""
import json
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def do_POST(self):
        length = int(self.headers.get("Content-Length") or 0)
        try:
            body = json.loads(self.rfile.read(length) or b"{}")
        except ValueError:
            body = {}
        path = self.path
        if path in ("/internal/v1/users/lookup", "/internal/v1/users/lookup-by-id"):
            users = [{"id": i, "nickname": "player-" + i[:6], "avatarUrl": ""} for i in body.get("userIds", [])]
            self.answer(200, {"users": users})
        elif path == "/internal/v1/users/search":
            self.answer(200, {"userIds": [], "next": ""})
        else:
            self.answer(401, {"code": "UNAUTHORIZED", "message": "nobody signs in here"})

    def do_GET(self):
        self.answer(404, {"code": "NOT_FOUND", "message": "not here"})

    def answer(self, status, body):
        data = json.dumps(body).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)


ThreadingHTTPServer(("127.0.0.1", int(sys.argv[1])), Handler).serve_forever()
