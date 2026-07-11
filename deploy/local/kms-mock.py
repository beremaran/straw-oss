from http.server import BaseHTTPRequestHandler, HTTPServer
import base64
import json
import secrets


class Handler(BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get("content-length", "0"))
        raw = self.rfile.read(length)
        target = self.headers.get("x-amz-target", "")

        if target.endswith("GenerateDataKey"):
            data_key = secrets.token_bytes(32)
            body = {
                "CiphertextBlob": base64.b64encode(data_key).decode("ascii"),
                "KeyId": json.loads(raw or b"{}").get("KeyId", "arn:aws:kms:us-east-1:000000000000:key/dev"),
                "Plaintext": base64.b64encode(data_key).decode("ascii"),
            }
        elif target.endswith("Decrypt"):
            body = {"KeyId": "arn:aws:kms:us-east-1:000000000000:key/dev", "Plaintext": json.loads(raw or b"{}").get("CiphertextBlob", "")}
        else:
            self.send_response(400)
            self.end_headers()
            return

        encoded = json.dumps(body).encode("utf-8")
        self.send_response(200)
        self.send_header("content-type", "application/x-amz-json-1.1")
        self.send_header("content-length", str(len(encoded)))
        self.end_headers()
        self.wfile.write(encoded)

    def log_message(self, fmt, *args):
        return


HTTPServer(("0.0.0.0", 8088), Handler).serve_forever()
