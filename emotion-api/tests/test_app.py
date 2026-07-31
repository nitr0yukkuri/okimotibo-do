from __future__ import annotations

import sys
import unittest
from pathlib import Path

from fastapi.testclient import TestClient

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from app import app


class EmotionApiTest(unittest.TestCase):
    def setUp(self) -> None:
        self.client = TestClient(app)

    def test_healthz(self) -> None:
        response = self.client.get("/healthz")
        self.assertEqual(response.status_code, 200)
        self.assertTrue(response.json()["ok"])

    def test_rejects_unsupported_content_type(self) -> None:
        response = self.client.post("/v1/emotion", content=b"image", headers={"Content-Type": "text/plain"})
        self.assertEqual(response.status_code, 415)

    def test_rejects_empty_image(self) -> None:
        response = self.client.post("/v1/emotion", content=b"", headers={"Content-Type": "image/jpeg"})
        self.assertEqual(response.status_code, 400)


if __name__ == "__main__":
    unittest.main()
