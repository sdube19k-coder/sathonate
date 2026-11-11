import http.server
import threading
import unittest

from soak import run


class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path != "/readyz":
            self.send_error(404)
            return
        self.send_response(self.server.status)
        self.end_headers()

    def log_message(self, *_):
        pass


class SoakTest(unittest.TestCase):
    def test_controlled_pass_and_fail(self):
        with http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler) as server:
            thread = threading.Thread(target=server.serve_forever)
            thread.start()
            try:
                for code, status in ((204, "PASS"), (503, "FAIL")):
                    server.status = code
                    result = run(server.server_port, 2, 10, 4)
                    self.assertEqual(result["status"], status, result)
                    self.assertEqual(result["completed"], 20, result)
                    print(f"fixture {code}: {result}")
            finally:
                server.shutdown()
                thread.join()

    def test_invalid_settings(self):
        for settings in ((0, 1, 20, 4), (9090, 1, 2, 4), (9090, 1, 20, 0)):
            with self.assertRaises(ValueError):
                run(*settings)
