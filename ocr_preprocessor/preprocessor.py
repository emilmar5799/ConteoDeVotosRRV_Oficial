"""
Pipeline de preprocesamiento de imágenes para actas electorales.

Resuelve cuatro anomalías físicas críticas:
  1. Manchas / gotas de tinta      → isolate_ink_stains()
  2. Números sobreescritos         → detect_overwritten_digits()
  3. Confusión alfanumérica        → validate_numeric_fields()
  4. Huellas dactilares            → isolate_fingerprints_from_text()
"""

from __future__ import annotations
import dataclasses
from typing import List, Optional
import cv2
import numpy as np
from skimage.morphology import skeletonize, disk
from skimage.filters import threshold_sauvola


# ─────────────────────────────────────────────────────────────────────────────
# Resultado del análisis
# ─────────────────────────────────────────────────────────────────────────────

@dataclasses.dataclass
class AnomalyFlags:
    mancha_detectada: bool = False
    porcentaje_mancha: float = 0.0
    numeros_sobreescritos: bool = False
    celdas_sobreescritas: List[str] = dataclasses.field(default_factory=list)
    confusion_alfanumerica: bool = False
    campos_sospechosos: List[str] = dataclasses.field(default_factory=list)
    huellas_zona_numeros: bool = False
    flag_revision_manual: bool = False
    observaciones: List[str] = dataclasses.field(default_factory=list)

    def _post_init_(self):
        self.flag_revision_manual = (
            self.mancha_detectada
            or self.numeros_sobreescritos
            or self.confusion_alfanumerica
        )


# ─────────────────────────────────────────────────────────────────────────────
# Utilidades de bajo nivel
# ─────────────────────────────────────────────────────────────────────────────

def to_gray(img: np.ndarray) -> np.ndarray:
    if len(img.shape) == 3:
        return cv2.cvtColor(img, cv2.COLOR_BGR2GRAY)
    return img


def binarize_sauvola(gray: np.ndarray, window: int = 51) -> np.ndarray:
    """Umbralización adaptativa Sauvola — robusta ante iluminación no uniforme."""
    thresh = threshold_sauvola(gray, window_size=window, k=0.2)
    binary = (gray < thresh).astype(np.uint8) * 255
    return binary


def count_skeleton_branch_points(skel: np.ndarray) -> int:
    """Cuenta los puntos de bifurcación del esqueleto (3+ vecinos activos)."""
    kernel = np.ones((3, 3), dtype=np.uint8)
    neighbor_count = cv2.filter2D(skel.astype(np.float32), -1, kernel.astype(np.float32))
    # Punto de bifurcación: pixel activo con 3 o más vecinos activos (incluido él mismo)
    branch_pts = (skel > 0) & (neighbor_count >= 4)
    return int(np.sum(branch_pts))


# ─────────────────────────────────────────────────────────────────────────────
# 1. MANCHAS Y GOTAS DE TINTA
# ─────────────────────────────────────────────────────────────────────────────

def isolate_ink_stains(
    img: np.ndarray,
    flags: AnomalyFlags,
    min_char_area: int = 40,
    max_char_area: int = 6000,
    min_aspect: float = 0.15,
    max_aspect: float = 7.0,
) -> np.ndarray:
    """
    Separa manchas/gotas de tinta del texto real y las elimina de la imagen.

    Estrategia:
      - Binariza con Sauvola (tolera iluminación no uniforme de fotos móviles).
      - Analiza cada componente conectado:
          * Ratio alto (≈ circular o amorfo): probable mancha.
          * Área gigante o ínfima: ruido o mancha grande.
          * Caracteres válidos: aspect ratio entre 0.15 y 7, área en rango.
      - Pinta las manchas detectadas en blanco sobre la imagen limpia.
    """
    gray = to_gray(img)
    binary = binarize_sauvola(gray)

    # Componentes conectados
    n_labels, labels, stats, _ = cv2.connectedComponentsWithStats(binary, connectivity=8)

    stain_mask = np.zeros_like(binary)
    char_mask = np.zeros_like(binary)

    for lbl in range(1, n_labels):
        x, y, w, h, area = (
            stats[lbl, cv2.CC_STAT_LEFT],
            stats[lbl, cv2.CC_STAT_TOP],
            stats[lbl, cv2.CC_STAT_WIDTH],
            stats[lbl, cv2.CC_STAT_HEIGHT],
            stats[lbl, cv2.CC_STAT_AREA],
        )
        aspect = w / (h + 1e-6)

        is_char = (
            min_char_area < area < max_char_area
            and min_aspect < aspect < max_aspect
        )

        if is_char:
            char_mask[labels == lbl] = 255
        else:
            stain_mask[labels == lbl] = 255

    stain_px = int(np.sum(stain_mask > 0))
    total_px = binary.size
    flags.porcentaje_mancha = round(stain_px / total_px * 100, 2)

    if flags.porcentaje_mancha > 1.5:
        flags.mancha_detectada = True
        flags.observaciones.append(
            f"MANCHA: {flags.porcentaje_mancha:.1f}% de la imagen afectada por ruido/tinta"
        )

    # Limpiar: en zonas de mancha, restaurar fondo blanco
    clean = img.copy()
    stain_dilated = cv2.dilate(stain_mask, disk(3).astype(np.uint8))
    clean[stain_dilated > 0] = 255

    return clean


# ─────────────────────────────────────────────────────────────────────────────
# 2. NÚMEROS SOBREESCRITOS
# ─────────────────────────────────────────────────────────────────────────────

def detect_overwritten_digits(
    img: np.ndarray,
    flags: AnomalyFlags,
    roi_rel: tuple = (0.28, 0.22, 0.50, 0.72),  # (x0%, y0%, x1%, y1%) de la imagen
    rows: int = 14,
    cols: int = 3,
    density_threshold: float = 0.38,
    branch_threshold: int = 6,
) -> None:
    """
    Detecta celdas numéricas con densidad de tinta anómalamente alta
    o topología de trazo compleja (esqueleto con muchas bifurcaciones).

    Un dígito normal ocupa ~15–32% de su celda.
    Un dígito sobreescrito (ej. "8 sobre 0") ocupa >38%.
    Además su esqueleto tiene 6+ puntos de bifurcación (dos trazos fundidos).
    """
    h_img, w_img = img.shape[:2]
    x0 = int(roi_rel[0] * w_img)
    y0 = int(roi_rel[1] * h_img)
    x1 = int(roi_rel[2] * w_img)
    y1 = int(roi_rel[3] * h_img)

    roi = img[y0:y1, x0:x1]
    gray_roi = to_gray(roi)
    binary_roi = binarize_sauvola(gray_roi, window=31)

    cell_h = (y1 - y0) // rows
    cell_w = (x1 - x0) // cols
    if cell_h < 4 or cell_w < 4:
        return

    sobreescritas = []

    for row in range(rows):
        for col in range(cols):
            cy0 = row * cell_h
            cy1 = cy0 + cell_h
            cx0 = col * cell_w
            cx1 = cx0 + cell_w

            cell = binary_roi[cy0:cy1, cx0:cx1]
            if cell.size == 0:
                continue

            # Densidad de tinta
            density = float(np.sum(cell > 0)) / cell.size

            # Topología del esqueleto
            skel = skeletonize(cell > 0).astype(np.uint8)
            branch_pts = count_skeleton_branch_points(skel)

            if density > density_threshold or branch_pts >= branch_threshold:
                label = f"fila{row+1}_col{col+1}"
                sobreescritas.append(label)

    if sobreescritas:
        flags.numeros_sobreescritos = True
        flags.celdas_sobreescritas = sobreescritas
        flags.observaciones.append(
            f"SOBREESCRITURA detectada en {len(sobreescritas)} celda(s): {', '.join(sobreescritas)}"
        )


# ─────────────────────────────────────────────────────────────────────────────
# 3. CONFUSIÓN ALFANUMÉRICA
# ─────────────────────────────────────────────────────────────────────────────

# Conjunto de sustituciones conocidas OCR → dígito
_OCR_DIGIT_MAP = {
    'O': '0', 'o': '0', 'Q': '0',
    'l': '1', 'I': '1', '|': '1', 'i': '1',
    'Z': '2', 'z': '2',
    'E': '3',
    'A': '4',
    'S': '5', 's': '5',
    'G': '6', 'b': '6',
    'T': '7',
    'B': '8',
    'g': '9', 'q': '9',
}

# Letras que son inequívocamente NO un dígito y tampoco una sustitución conocida
_SUSPICIOUS_LETTERS = set('CDEFHJKLMNPRUVWXYacdfehjkmnprstuvwxy')


def validate_numeric_fields(
    ocr_results: dict,  # {"campo": "texto_ocr_crudo"}
    flags: AnomalyFlags,
) -> dict:
    """
    Valida campos numéricos del OCR.
    Aplica dos niveles de corrección:
      1. Sustituciones seguras (O→0, l→1, S→5…) → corregidas automáticamente.
      2. Letras genuinas sin equivalente numérico → flag de revisión manual.

    Retorna el mismo dict con los valores corregidos donde sea posible.
    """
    corrected = {}
    for campo, valor in ocr_results.items():
        if valor is None:
            corrected[campo] = valor
            continue

        cleaned = valor.strip().replace(' ', '')
        resultado = []
        sospechoso = False

        for ch in cleaned:
            if ch.isdigit():
                resultado.append(ch)
            elif ch in _OCR_DIGIT_MAP:
                resultado.append(_OCR_DIGIT_MAP[ch])
            elif ch in _SUSPICIOUS_LETTERS:
                resultado.append('?')
                sospechoso = True
            # Ignorar el resto (guiones, puntos, etc.)

        corrected[campo] = ''.join(resultado)

        if sospechoso:
            flags.confusion_alfanumerica = True
            flags.campos_sospechosos.append(f"{campo}='{valor}'→'{corrected[campo]}'")
            flags.observaciones.append(
                f"CONFUSIÓN ALFANUMÉRICA en '{campo}': OCR devolvió '{valor}', corregido a '{corrected[campo]}'"
            )

    return corrected


# ─────────────────────────────────────────────────────────────────────────────
# 4. HUELLAS DACTILARES — AISLAMIENTO DE LA ZONA DE NÚMEROS
# ─────────────────────────────────────────────────────────────────────────────

def isolate_fingerprints_from_text(
    img: np.ndarray,
    flags: AnomalyFlags,
    roi_numeros_rel: tuple = (0.28, 0.22, 0.50, 0.72),
) -> np.ndarray:
    """
    Aplica un banco de filtros Gabor a la zona de números para detectar y suprimir
    patrones de huella dactilar (curvas multi-direccionales) sin dañar los dígitos.

    Principio:
      - Los dígitos tienen trazos principalmente verticales/horizontales.
      - Las huellas tienen crestas curvadas a múltiples ángulos.
      - Gabor a 8 orientaciones → las huellas responden en TODAS; los dígitos sólo en 1–2.
      - Se construye un mapa de "isotropía de respuesta": alta isotropía = huella.

    La imagen limpia se obtiene suprimiendo las zonas de alta isotropía.
    """
    gray = to_gray(img)
    h_img, w_img = gray.shape

    x0 = int(roi_numeros_rel[0] * w_img)
    y0 = int(roi_numeros_rel[1] * h_img)
    x1 = int(roi_numeros_rel[2] * w_img)
    y1 = int(roi_numeros_rel[3] * h_img)

    roi_gray = gray[y0:y1, x0:x1].astype(np.float32)

    # Banco Gabor: 8 orientaciones, parámetros ajustados para crestas de ~4–8 px
    orientations = np.linspace(0, np.pi, 8, endpoint=False)
    responses = []

    for theta in orientations:
        kernel = cv2.getGaborKernel(
            ksize=(21, 21),
            sigma=3.0,
            theta=theta,
            lambd=8.0,   # longitud de onda (px entre crestas)
            gamma=0.5,   # elongación
            psi=0,
        )
        resp = cv2.filter2D(roi_gray, cv2.CV_32F, kernel)
        responses.append(np.abs(resp))

    # Stack de respuestas
    stack = np.stack(responses, axis=0)          # (8, H, W)
    mean_resp = np.mean(stack, axis=0)           # media por pixel
    std_resp = np.std(stack, axis=0)             # std por pixel

    # Isotropía = media alta + std baja → responde igual en todas las direcciones → huella
    with np.errstate(divide='ignore', invalid='ignore'):
        isotropy = np.where(mean_resp > 5, 1.0 - (std_resp / (mean_resp + 1e-6)), 0.0)

    # Normalizar y umbralizar
    isotropy_norm = cv2.normalize(isotropy, None, 0, 255, cv2.NORM_MINMAX).astype(np.uint8)
    _, fp_mask = cv2.threshold(isotropy_norm, 180, 255, cv2.THRESH_BINARY)

    # Morfología: cerrar pequeños huecos dentro de la huella
    fp_mask = cv2.morphologyEx(fp_mask, cv2.MORPH_CLOSE, disk(5).astype(np.uint8))

    # Evaluar si hay huella sobre la zona de números
    fp_ratio = float(np.sum(fp_mask > 0)) / fp_mask.size
    if fp_ratio > 0.04:
        flags.huellas_zona_numeros = True
        flags.observaciones.append(
            f"HUELLA DACTILAR detectada sobre zona de números ({fp_ratio*100:.1f}% cubierta)"
        )

    # Reconstruir imagen limpia: dentro del roi, suprimir textura de huella
    # conservando los dígitos (líneas más rectas y definidas)
    clean = img.copy()
    roi_color = clean[y0:y1, x0:x1]

    # Ecualizar CLAHE solo en la zona de huella para mejorar contraste del dígito
    clahe = cv2.createCLAHE(clipLimit=3.0, tileGridSize=(4, 4))
    for c in range(roi_color.shape[2] if len(roi_color.shape) == 3 else 1):
        if len(roi_color.shape) == 3:
            ch = roi_color[:, :, c]
        else:
            ch = roi_color
        enhanced = clahe.apply(ch)
        # Mezclar: en zonas de huella, usar la versión CLAHE; fuera, original
        alpha = (fp_mask.astype(np.float32) / 255.0)
        blended = (alpha * enhanced + (1 - alpha) * ch.astype(np.float32)).astype(np.uint8)
        if len(roi_color.shape) == 3:
            roi_color[:, :, c] = blended
        else:
            roi_color[:, :] = blended

    clean[y0:y1, x0:x1] = roi_color
    return clean


# ─────────────────────────────────────────────────────────────────────────────
# PIPELINE COMPLETO
# ─────────────────────────────────────────────────────────────────────────────

def full_pipeline(
    img: np.ndarray,
    ocr_raw_fields: Optional[dict] = None,
) -> tuple[np.ndarray, AnomalyFlags]:
    """
    Ejecuta el pipeline completo de preprocesamiento.

    Args:
        img: imagen BGR cargada con cv2.imread (o np.frombuffer).
        ocr_raw_fields: dict {"campo": "texto_ocr"} para validación alfanumérica.
                        Si es None, se omite esa etapa.

    Returns:
        (clean_img, flags) donde:
          - clean_img: imagen preprocesada lista para re-OCR.
          - flags: AnomalyFlags con todas las anomalías detectadas.
    """
    flags = AnomalyFlags()

    # Paso 0: normalizar orientación / contraste global
    img = _normalize_image(img)

    # Paso 1: eliminar manchas
    img = isolate_ink_stains(img, flags)

    # Paso 2: detectar sobreescrituras (en imagen ya limpiada de manchas)
    detect_overwritten_digits(img, flags)

    # Paso 3: aislar huellas en zona de números
    img = isolate_fingerprints_from_text(img, flags)

    # Paso 4: validación alfanumérica sobre texto OCR previo
    corrected_fields = {}
    if ocr_raw_fields:
        corrected_fields = validate_numeric_fields(ocr_raw_fields, flags)

    # Actualizar flag de revisión manual
    flags.flag_revision_manual = (
        flags.mancha_detectada
        or flags.numeros_sobreescritos
        or flags.confusion_alfanumerica
        or flags.huellas_zona_numeros
    )

    return img, flags, corrected_fields


def _normalize_image(img: np.ndarray) -> np.ndarray:
    """Deskew ligero + normalización de brillo global."""
    gray = to_gray(img)

    # Detectar ángulo de inclinación con la transformada de Hough sobre bordes
    edges = cv2.Canny(gray, 50, 150, apertureSize=3)
    lines = cv2.HoughLines(edges, 1, np.pi / 180, threshold=200)

    if lines is not None:
        angles = []
        for rho, theta in lines[:, 0]:
            angle_deg = np.degrees(theta) - 90
            if abs(angle_deg) < 15:
                angles.append(angle_deg)
        if angles:
            median_angle = float(np.median(angles))
            if abs(median_angle) > 0.5:
                h, w = img.shape[:2]
                M = cv2.getRotationMatrix2D((w / 2, h / 2), median_angle, 1.0)
                img = cv2.warpAffine(img, M, (w, h),
                                     flags=cv2.INTER_LINEAR,
                                     borderMode=cv2.BORDER_REPLICATE)

    # Normalización adaptativa de brillo (útil para fotos de celular con flash desigual)
    lab = cv2.cvtColor(img, cv2.COLOR_BGR2LAB) if len(img.shape) == 3 else None
    if lab is not None:
        l_ch, a_ch, b_ch = cv2.split(lab)
        clahe = cv2.createCLAHE(clipLimit=2.0, tileGridSize=(8, 8))
        l_ch = clahe.apply(l_ch)
        lab = cv2.merge([l_ch, a_ch, b_ch])
        img = cv2.cvtColor(lab, cv2.COLOR_LAB2BGR)

    return img
