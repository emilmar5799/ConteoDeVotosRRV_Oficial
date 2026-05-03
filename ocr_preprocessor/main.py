"""
Microservicio FastAPI de preprocesamiento OCR para actas electorales.

Endpoints:
  POST /preprocess          → imagen raw → imagen limpia + flags JSON
  POST /preprocess/base64   → imagen base64 → imagen base64 limpia + flags JSON
  GET  /health              → liveness check

Uso desde Go (RRV backend):
  - El ValidadorVisual llama a http://localhost:8000/preprocess con multipart/form-data.
  - Recibe JSON con AnomalyFlags y una imagen JPEG limpia en base64.
"""

import base64
import io
import json
import logging

import cv2
import numpy as np
import uvicorn
from fastapi import FastAPI, File, Form, HTTPException, UploadFile
from fastapi.responses import JSONResponse
from PIL import Image

from preprocessor import full_pipeline, AnomalyFlags

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
log = logging.getLogger(__name__)

app = FastAPI(
    title="RRV OCR Preprocessor",
    description="Pipeline OpenCV para limpieza de actas electorales",
    version="1.0.0",
)


# ─────────────────────────────────────────────────────────────────────────────
# Helpers
# ─────────────────────────────────────────────────────────────────────────────

def bytes_to_cv2(data: bytes) -> np.ndarray:
    arr = np.frombuffer(data, dtype=np.uint8)
    img = cv2.imdecode(arr, cv2.IMREAD_COLOR)
    if img is None:
        raise ValueError("No se pudo decodificar la imagen")
    return img


def cv2_to_jpeg_b64(img: np.ndarray, quality: int = 92) -> str:
    _, buf = cv2.imencode(".jpg", img, [cv2.IMWRITE_JPEG_QUALITY, quality])
    return base64.b64encode(buf.tobytes()).decode()


def flags_to_dict(flags: AnomalyFlags) -> dict:
    return {
        "mancha_detectada": flags.mancha_detectada,
        "porcentaje_mancha": flags.porcentaje_mancha,
        "numeros_sobreescritos": flags.numeros_sobreescritos,
        "celdas_sobreescritas": flags.celdas_sobreescritas,
        "confusion_alfanumerica": flags.confusion_alfanumerica,
        "campos_sospechosos": flags.campos_sospechosos,
        "huellas_zona_numeros": flags.huellas_zona_numeros,
        "flag_revision_manual": flags.flag_revision_manual,
        "observaciones": flags.observaciones,
    }


# ─────────────────────────────────────────────────────────────────────────────
# Endpoints
# ─────────────────────────────────────────────────────────────────────────────

@app.get("/health")
def health():
    return {"status": "ok", "service": "ocr_preprocessor"}


@app.post("/preprocess")
async def preprocess_image(
    file: UploadFile = File(..., description="Imagen del acta (JPEG/PNG)"),
    ocr_fields: str = Form(
        default="{}",
        description='JSON con campos OCR crudos p.ej. {"votos_c1": "1 2 B"}',
    ),
):
    """
    Recibe una imagen de acta, corre el pipeline completo y devuelve:
    - `flags`: todas las anomalías detectadas.
    - `clean_image_b64`: imagen preprocesada como JPEG en base64.
    - `corrected_fields`: campos OCR corregidos (si se enviaron).
    """
    raw = await file.read()
    if len(raw) == 0:
        raise HTTPException(status_code=400, detail="Archivo vacío")

    try:
        img = bytes_to_cv2(raw)
    except ValueError as e:
        raise HTTPException(status_code=422, detail=str(e))

    try:
        ocr_raw = json.loads(ocr_fields)
    except json.JSONDecodeError:
        ocr_raw = {}

    log.info("Procesando imagen %s (%.1f KB)", file.filename, len(raw) / 1024)

    try:
        clean_img, flags, corrected = full_pipeline(img, ocr_raw_fields=ocr_raw or None)
    except Exception as exc:
        log.exception("Error en pipeline")
        raise HTTPException(status_code=500, detail=f"Error en pipeline: {exc}")

    clean_b64 = cv2_to_jpeg_b64(clean_img)

    return JSONResponse({
        "flags": flags_to_dict(flags),
        "clean_image_b64": clean_b64,
        "corrected_fields": corrected,
    })


@app.post("/preprocess/base64")
async def preprocess_base64(body: dict):
    """
    Alternativa cuando el cliente no puede enviar multipart.
    Body JSON: { "image_b64": "...", "ocr_fields": {...} }
    """
    b64_str = body.get("image_b64", "")
    if not b64_str:
        raise HTTPException(status_code=400, detail="image_b64 requerido")

    try:
        raw = base64.b64decode(b64_str)
        img = bytes_to_cv2(raw)
    except Exception as e:
        raise HTTPException(status_code=422, detail=f"Imagen inválida: {e}")

    ocr_raw = body.get("ocr_fields") or {}

    try:
        clean_img, flags, corrected = full_pipeline(img, ocr_raw_fields=ocr_raw or None)
    except Exception as exc:
        log.exception("Error en pipeline")
        raise HTTPException(status_code=500, detail=f"Error en pipeline: {exc}")

    clean_b64 = cv2_to_jpeg_b64(clean_img)

    return JSONResponse({
        "flags": flags_to_dict(flags),
        "clean_image_b64": clean_b64,
        "corrected_fields": corrected,
    })


# ─────────────────────────────────────────────────────────────────────────────
# Entrypoint
# ─────────────────────────────────────────────────────────────────────────────

if __name__ == "__main__":
    uvicorn.run("main:app", host="0.0.0.0", port=8000, reload=False, workers=2)
