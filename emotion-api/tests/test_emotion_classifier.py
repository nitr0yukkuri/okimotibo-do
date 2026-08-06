from __future__ import annotations

import sys
import unittest
from io import BytesIO
from pathlib import Path

import pandas as pd
from PIL import Image

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from emotion_classifier import EMOTIONS, EmotionClassifier, EmotionPredictionError, status_for_emotion


def jpeg_bytes() -> bytes:
    output = BytesIO()
    Image.new("RGB", (32, 32), "white").save(output, format="JPEG")
    return output.getvalue()


class FakeDetector:
    def __init__(self, scores: dict[str, float]) -> None:
        self.scores = scores

    def detect(self, _path: str, *, data_type: str) -> pd.DataFrame:
        assert data_type == "image"
        return pd.DataFrame([self.scores])


class StatusMappingTest(unittest.TestCase):
    def test_status_for_emotion(self) -> None:
        cases = [
            ("Happy", 0.8, "available"),
            ("Anger", 0.8, "busy"),
            ("Neutral", 0.9, "unknown"),
            ("Happy", 0.69, "unknown"),
            ("Anger", 0.69, "unknown"),
        ]
        for emotion, confidence, expected in cases:
            with self.subTest(emotion=emotion, confidence=confidence):
                self.assertEqual(status_for_emotion(emotion, confidence, 0.7), expected)

    def test_predict_returns_highest_emotion(self) -> None:
        classifier = EmotionClassifier()
        classifier._detector = FakeDetector({name: 0.8 if name == "Happy" else 0.02 for name in EMOTIONS})

        prediction = classifier.predict(jpeg_bytes())

        self.assertEqual(prediction.emotion, "Happy")
        self.assertEqual(prediction.status, "available")

    def test_predict_rejects_non_finite_scores(self) -> None:
        classifier = EmotionClassifier()
        classifier._detector = FakeDetector({name: float("nan") for name in EMOTIONS})

        with self.assertRaisesRegex(EmotionPredictionError, "face was not detected"):
            classifier.predict(jpeg_bytes())


if __name__ == "__main__":
    unittest.main()
