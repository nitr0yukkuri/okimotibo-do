from __future__ import annotations

import os
from dataclasses import asdict

from fastapi import FastAPI, HTTPException, Request
from fastapi.concurrency import run_in_threadpool
from fastapi.middleware.cors import CORSMiddleware

from emotion_classifier import EmotionClassifier, EmotionPredictionError


MAX_IMAGE_BYTES = 2 * 1024 * 1024


def allowed_origins() -> list[str]:
    configured = os.getenv(
        "ALLOWED_ORIGINS",
        "http://localhost:5173,http://127.0.0.1:5173",
    )
    return [origin.strip() for origin in configured.split(",") if origin.strip()]


classifier = EmotionClassifier(
    device=os.getenv("PYFEAT_DEVICE", "cpu"),
    threshold=float(os.getenv("EMOTION_MIN_CONFIDENCE", "0.7")),
)

app = FastAPI(title="Okimochi Emotion API", version="0.1.0")
app.add_middleware(
    CORSMiddleware,
    allow_origins=allowed_origins(),
    allow_credentials=False,
    allow_methods=["GET", "POST"],
    allow_headers=["Content-Type"],
)


@app.get("/healthz")
def healthz() -> dict[str, object]:
    return {"ok": True, "modelLoaded": classifier.model_loaded}


@app.post("/v1/emotion")
async def predict_emotion(request: Request) -> dict[str, object]:
    content_type = request.headers.get("content-type", "").split(";", 1)[0].lower()
    if content_type not in {"image/jpeg", "image/png", "image/webp"}:
        raise HTTPException(status_code=415, detail="JPEG, PNG, or WebP image is required")

    image_bytes = await request.body()
    if not image_bytes:
        raise HTTPException(status_code=400, detail="image body is empty")
    if len(image_bytes) > MAX_IMAGE_BYTES:
        raise HTTPException(status_code=413, detail="image exceeds 2 MiB")

    try:
        prediction = await run_in_threadpool(classifier.predict, image_bytes)
    except EmotionPredictionError as error:
        raise HTTPException(status_code=422, detail=str(error)) from error

    return asdict(prediction)
