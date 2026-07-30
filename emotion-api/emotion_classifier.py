from __future__ import annotations

from dataclasses import dataclass
from io import BytesIO
from math import isfinite
from pathlib import Path
from platform import system
import sys
from tempfile import NamedTemporaryFile
from threading import Lock
from types import ModuleType
from typing import Any

from PIL import Image, UnidentifiedImageError


EMOTIONS = ("Neutral", "Happy", "Sad", "Surprise", "Fear", "Disgust", "Anger")


class EmotionPredictionError(ValueError):
    pass


@dataclass(frozen=True)
class EmotionPrediction:
    emotion: str
    confidence: float
    status: str
    scores: dict[str, float]


def status_for_emotion(emotion: str, confidence: float, threshold: float) -> str:
    if confidence < threshold:
        return "unknown"
    if emotion == "Happy":
        return "available"
    if emotion == "Anger":
        return "busy"
    return "unknown"


def prepare_image_only_runtime() -> None:
    if system() != "Windows":
        return

    # Py-Feat imports TorchCodec eagerly even for images. Its Windows wheel needs
    # external FFmpeg DLLs, but this service never enters the video code path.
    torchcodec = ModuleType("torchcodec")
    decoders = ModuleType("torchcodec.decoders")

    class ImageOnlyVideoDecoder:
        def __init__(self, *_args: object, **_kwargs: object) -> None:
            raise RuntimeError("video decoding is disabled in the image-only emotion API")

    decoders.VideoDecoder = ImageOnlyVideoDecoder  # type: ignore[attr-defined]
    torchcodec.decoders = decoders  # type: ignore[attr-defined]
    sys.modules["torchcodec"] = torchcodec
    sys.modules["torchcodec.decoders"] = decoders


class EmotionClassifier:
    def __init__(self, *, device: str = "cpu", threshold: float = 0.7) -> None:
        self.device = device
        self.threshold = threshold
        self._detector: Any | None = None
        self._lock = Lock()

    @property
    def model_loaded(self) -> bool:
        return self._detector is not None

    def predict(self, image_bytes: bytes) -> EmotionPrediction:
        image_path = self._prepare_image(image_bytes)
        try:
            with self._lock:
                result = self._get_detector().detect(str(image_path), data_type="image")
            if result is None or result.empty:
                raise EmotionPredictionError("face was not detected")

            row = result.iloc[0]
            scores = {name: float(row[name]) for name in EMOTIONS if name in result.columns}
            if len(scores) != len(EMOTIONS):
                raise EmotionPredictionError("emotion model returned incomplete scores")
            if not all(isfinite(score) for score in scores.values()):
                raise EmotionPredictionError("face was not detected")

            emotion = max(scores, key=scores.get)
            confidence = scores[emotion]
            return EmotionPrediction(
                emotion=emotion,
                confidence=confidence,
                status=status_for_emotion(emotion, confidence, self.threshold),
                scores=scores,
            )
        finally:
            image_path.unlink(missing_ok=True)

    def _get_detector(self) -> Any:
        if self._detector is None:
            prepare_image_only_runtime()
            from feat import Detectorv2

            self._detector = Detectorv2(device=self.device)
        return self._detector

    @staticmethod
    def _prepare_image(image_bytes: bytes) -> Path:
        try:
            with Image.open(BytesIO(image_bytes)) as image:
                image.load()
                rgb_image = image.convert("RGB")
        except (UnidentifiedImageError, OSError) as error:
            raise EmotionPredictionError("request body is not a valid image") from error

        with NamedTemporaryFile(suffix=".jpg", delete=False) as temporary:
            path = Path(temporary.name)
        rgb_image.save(path, format="JPEG", quality=90)
        return path
