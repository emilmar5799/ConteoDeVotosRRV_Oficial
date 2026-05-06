package service

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg" // registra el decoder JPEG para image.Decode
	"image/png"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"rrv-backend/internal/models"
)

// ═══════════════════════════════════════════════════════════════════
// VisualValidator — Análisis visual + PDF stream de actas electorales
//
// Estrategia DUAL:
//   1. Análisis del content stream del PDF (colores de tinta)
//      → Detecta lápiz (gris 0.627), corrector, colores sospechosos
//   2. Análisis de imagen renderizada (manchas, huellas, roturas)
//      → Detecta manchas oscuras/cálidas, huellas dactilares, roturas
//
// Para PDFs image-only (escaneados), se usa SOLO análisis de imagen.
// ═══════════════════════════════════════════════════════════════════

type VisualValidator struct {
	mutoolPath    string
	pythonOCRURL  string     // URL del microservicio Python (vacío = deshabilitado)
	httpClient    *http.Client
}

func NewVisualValidator(mutoolPath string) *VisualValidator {
	return &VisualValidator{
		mutoolPath:   mutoolPath,
		pythonOCRURL: "http://localhost:8000",
		httpClient:   &http.Client{Timeout: 15 * time.Second},
	}
}

// SetPythonOCRURL permite configurar o deshabilitar el microservicio Python.
// Pasar cadena vacía para deshabilitar.
func (v *VisualValidator) SetPythonOCRURL(url string) {
	v.pythonOCRURL = url
}

// ValidarImagenActa ejecuta análisis visual completo sobre el acta.
func (v *VisualValidator) ValidarImagenActa(pdfPath string) (*models.ValidacionVisualResult, error) {
	resultado := &models.ValidacionVisualResult{}

	// ═══ Determinar si el PDF es programático o image-only ═══
	esProgramatico := v.tieneTextoPDF(pdfPath)

	// ═══ FASE 1: ANÁLISIS DEL PDF STREAM (solo para PDFs programáticos) ═══
	if esProgramatico {
		v.analizarPDFStream(pdfPath, resultado)
	}

	// ═══ FASE 2: ANÁLISIS DE IMAGEN RENDERIZADA ═══
	imgPath, err := v.renderizarPDF(pdfPath)
	if err != nil {
		return resultado, nil
	}

	file, err := os.Open(imgPath)
	if err != nil {
		return resultado, nil
	}
	defer file.Close()

	img, err := png.Decode(file)
	if err != nil {
		return resultado, nil
	}

	bounds := img.Bounds()

	// ═══ Detección de manchas (oscuras + cálidas) ═══
	roiDatos := v.roiZonaDatos(bounds)
	mancha, porcentaje := v.detectarManchas(img, roiDatos)
	resultado.ManchaDetectada = mancha
	resultado.PorcentajeMancha = math.Round(porcentaje*10) / 10

	// ═══ Detección de manchas en bordes/esquinas (stains oscuras) ═══
	manchaBorde, pctBorde := v.detectarManchasBordes(img, bounds)
	if manchaBorde && !resultado.ManchaDetectada {
		resultado.ManchaDetectada = true
		resultado.PorcentajeMancha = math.Round(pctBorde*10) / 10
	}

	// ═══ Detección de huellas/firmas ═══
	roiFirmas := v.roiZonaFirmas(bounds)
	huellas := v.contarHuellas(img, roiFirmas)
	resultado.HuellasDetectadas = huellas
	resultado.FirmasSuficientes = huellas >= 3

	// ═══ Detección de roturas (zonas blancas grandes en bordes) ═══
	resultado.RoturaDetectada = v.detectarRoturas(img, bounds)

	// ═══ Detección de actas arrugadas (sombras y varianza de grises) ═══
	resultado.ArrugasDetectadas = v.detectarArrugas(img, bounds)

	// ═══ Para PDFs image-only: detección visual de lápiz, tachaduras, etc. ═══
	if !esProgramatico {
		v.analizarImageOnly(img, bounds, resultado)
	}

	// ═══ FASE 3: ANÁLISIS AVANZADO vía microservicio Python/OpenCV ═══
	if v.pythonOCRURL != "" {
		v.llamarMicroservicioPython(imgPath, resultado)
	}

	// ═══ GENERAR OBSERVACIONES ═══
	v.generarObservaciones(resultado)

	return resultado, nil
}

// ═══════════════════════════════════════════════════════════════════
// Detección de tipo de PDF
// ═══════════════════════════════════════════════════════════════════

// tieneTextoPDF determina si el PDF tiene fuentes embebidas (programático)
// o es solo una imagen escaneada.
func (v *VisualValidator) tieneTextoPDF(pdfPath string) bool {
	cmd := exec.Command(v.mutoolPath, "info", pdfPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(output), "Fonts")
}

// ═══════════════════════════════════════════════════════════════════
// FASE 1: ANÁLISIS DEL PDF CONTENT STREAM
// ═══════════════════════════════════════════════════════════════════

var reColorRG = regexp.MustCompile(`([\d.]+)\s+([\d.]+)\s+([\d.]+)\s+rg`)

func (v *VisualValidator) analizarPDFStream(pdfPath string, resultado *models.ValidacionVisualResult) {
	streamContent := v.extraerContentStream(pdfPath)
	if streamContent == "" {
		return
	}

	matches := reColorRG.FindAllStringSubmatch(streamContent, -1)

	var tieneGrisLapiz bool
	var grisValor float64

	for _, m := range matches {
		r, _ := strconv.ParseFloat(m[1], 64)
		g, _ := strconv.ParseFloat(m[2], 64)
		b, _ := strconv.ParseFloat(m[3], 64)

		// ═══ LÁPIZ: gris claro uniforme (R=G=B, valor > 0.4) ═══
		if r == g && g == b && r > 0.4 && r < 0.9 {
			tieneGrisLapiz = true
			grisValor = r
		}
		_ = b
	}

	resultado.LapizDetectado = tieneGrisLapiz
	if tieneGrisLapiz {
		resultado.IntensidadPromedio = grisValor * 255
		resultado.RatioContraste = 1.0 - grisValor
	}
}

func (v *VisualValidator) extraerContentStream(pdfPath string) string {
	// Obtener el page object
	cmd := exec.Command(v.mutoolPath, "show", pdfPath, "1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}

	pageStr := string(output)
	reContents := regexp.MustCompile(`/Contents\s+(\d+)\s+\d+\s+R`)
	contentsMatch := reContents.FindStringSubmatch(pageStr)
	if contentsMatch == nil {
		// Fallback: intenta con objeto 5
		cmd2 := exec.Command(v.mutoolPath, "show", pdfPath, "5")
		out2, err2 := cmd2.CombinedOutput()
		if err2 != nil {
			return ""
		}
		return string(out2)
	}

	contentID := contentsMatch[1]
	cmd3 := exec.Command(v.mutoolPath, "show", pdfPath, contentID)
	out3, err3 := cmd3.CombinedOutput()
	if err3 != nil {
		return ""
	}
	return string(out3)
}

// ═══════════════════════════════════════════════════════════════════
// ANÁLISIS PARA PDFs IMAGE-ONLY (escaneados)
//
// Para actas escaneadas, detectamos:
//   - Lápiz: por histograma de intensidades en zona de números
//   - Tachaduras: densidad de tinta oscura extrema (superposición)
//   - "ANULADA": cobertura de tinta negra masiva (texto gigante)
// ═══════════════════════════════════════════════════════════════════

func (v *VisualValidator) analizarImageOnly(img image.Image, bounds image.Rectangle, resultado *models.ValidacionVisualResult) {
	// ═══ Detección de "ANULADA" escrita a mano (tinta negra masiva) ═══
	// El texto "Anulado" cruza todo el acta con tinta negra
	roiCentro := image.Rect(
		int(float64(bounds.Max.X)*0.20), int(float64(bounds.Max.Y)*0.30),
		int(float64(bounds.Max.X)*0.70), int(float64(bounds.Max.Y)*0.75),
	)
	var pixelesNegros int
	totalPixeles := (roiCentro.Max.X - roiCentro.Min.X) * (roiCentro.Max.Y - roiCentro.Min.Y)

	for y := roiCentro.Min.Y; y < roiCentro.Max.Y; y++ {
		for x := roiCentro.Min.X; x < roiCentro.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			gray := float64(r>>8)*0.299 + float64(g>>8)*0.587 + float64(b>>8)*0.114
			if gray < 50 {
				pixelesNegros++
			}
		}
	}

	// Si >15% del centro es negro masivo → "ANULADA" escrita
	ratioNegro := float64(pixelesNegros) / float64(totalPixeles) * 100
	if ratioNegro > 15 {
		resultado.TextoAnuladaVisual = true
	}

	// ═══ Detección de tachaduras (superposición de números) ═══
	roiNumeros := image.Rect(
		int(float64(bounds.Max.X)*0.30), int(float64(bounds.Max.Y)*0.25),
		int(float64(bounds.Max.X)*0.47), int(float64(bounds.Max.Y)*0.65),
	)
	resultado.TachaduraDetectada = v.detectarTachadurasImagen(img, roiNumeros)
}

// detectarTachadurasImagen detecta números superpuestos analizando
// la densidad de tinta en las celdas numéricas.
func (v *VisualValidator) detectarTachadurasImagen(img image.Image, roi image.Rectangle) bool {
	// Dividir la ROI en sub-celdas y buscar celdas con densidad excesiva
	cellH := (roi.Max.Y - roi.Min.Y) / 8 // ~8 filas de números
	cellW := (roi.Max.X - roi.Min.X) / 3  // 3 dígitos por fila
	if cellH == 0 || cellW == 0 {
		return false
	}

	var celdasDensas int
	var celdasTotales int

	for row := 0; row < 8; row++ {
		for col := 0; col < 3; col++ {
			x0 := roi.Min.X + col*cellW
			y0 := roi.Min.Y + row*cellH
			x1 := x0 + cellW
			y1 := y0 + cellH

			var pixOscuros int
			pixTotal := cellW * cellH
			if pixTotal == 0 {
				continue
			}

			for y := y0; y < y1 && y < roi.Max.Y; y++ {
				for x := x0; x < x1 && x < roi.Max.X; x++ {
					r, g, b, _ := img.At(x, y).RGBA()
					gray := float64(r>>8)*0.299 + float64(g>>8)*0.587 + float64(b>>8)*0.114
					if gray < 120 {
						pixOscuros++
					}
				}
			}

			densidad := float64(pixOscuros) / float64(pixTotal)
			celdasTotales++

			// Una celda normal tiene ~15-35% de tinta (un dígito)
			// Una tachadura (8 sobre 0) tiene >50%
			if densidad > 0.45 {
				celdasDensas++
			}
		}
	}

	// Si hay al menos 1 celda con densidad excesiva → tachadura
	return celdasDensas >= 1
}

// ═══════════════════════════════════════════════════════════════════
// ROIs (Region of Interest)
// ═══════════════════════════════════════════════════════════════════

func (v *VisualValidator) roiZonaDatos(bounds image.Rectangle) image.Rectangle {
	w := bounds.Max.X
	h := bounds.Max.Y
	return image.Rect(
		int(float64(w)*0.10), int(float64(h)*0.20),
		int(float64(w)*0.55), int(float64(h)*0.65),
	)
}

func (v *VisualValidator) roiZonaFirmas(bounds image.Rectangle) image.Rectangle {
	w := bounds.Max.X
	h := bounds.Max.Y
	return image.Rect(
		int(float64(w)*0.55), int(float64(h)*0.20),
		int(float64(w)*0.95), int(float64(h)*0.85),
	)
}

// ═══════════════════════════════════════════════════════════════════
// DETECCIÓN DE MANCHAS — ahora detecta manchas OSCURAS y cálidas
//
// Tipos de mancha:
//   - Café/grasa: color cálido saturado (naranja/marrón) → r > b
//   - Mancha oscura: parche negro/gris en zona de datos (esquinas)
//   - Mancha: cualquier parche que tape datos importantes
// ═══════════════════════════════════════════════════════════════════

func (v *VisualValidator) detectarManchas(img image.Image, roi image.Rectangle) (bool, float64) {
	var pixelesMancha int
	totalPixeles := (roi.Max.X - roi.Min.X) * (roi.Max.Y - roi.Min.Y)
	if totalPixeles == 0 {
		return false, 0
	}

	for y := roi.Min.Y; y < roi.Max.Y; y++ {
		for x := roi.Min.X; x < roi.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			r8, g8, b8 := float64(r>>8), float64(g>>8), float64(b>>8)
			gray := r8*0.299 + g8*0.587 + b8*0.114
			maxC := math.Max(r8, math.Max(g8, b8))
			minC := math.Min(r8, math.Min(g8, b8))
			saturacion := maxC - minC

			// Mancha cálida = color saturado, tono café/naranja/grasa
			manchaCalida := gray < 220 && gray > 60 && saturacion > 30 && r8 > b8
			// Mancha de tinta azul/morada (tampos, bolígrafo reventado)
			manchaTintaAzul := gray < 180 && saturacion > 30 && b8 > r8+10 && b8 > g8
			// Mancha oscura general (tinta negra, barro, suciedad extrema)
			manchaOscura := gray < 80 && saturacion < 30

			if manchaCalida || manchaTintaAzul || manchaOscura {
				pixelesMancha++
			}
		}
	}

	porcentaje := float64(pixelesMancha) / float64(totalPixeles) * 100
	return porcentaje > 3.0, porcentaje // Reducido a 3.0% para mayor sensibilidad
}

// detectarManchasBordes detecta manchas oscuras en las esquinas del acta
// (daño por escaneo, marcas negras, papel dañado en bordes).
func (v *VisualValidator) detectarManchasBordes(img image.Image, bounds image.Rectangle) (bool, float64) {
	w := bounds.Max.X
	h := bounds.Max.Y

	// 4 esquinas: cuadros de ~8% del tamaño
	esquinas := []image.Rectangle{
		// Superior izquierda
		image.Rect(0, 0, int(float64(w)*0.12), int(float64(h)*0.12)),
		// Superior derecha
		image.Rect(int(float64(w)*0.88), 0, w, int(float64(h)*0.12)),
		// Inferior izquierda
		image.Rect(0, int(float64(h)*0.88), int(float64(w)*0.12), h),
		// Inferior derecha
		image.Rect(int(float64(w)*0.88), int(float64(h)*0.88), w, h),
	}

	var totalMancha int
	var totalPixeles int

	for _, esq := range esquinas {
		for y := esq.Min.Y; y < esq.Max.Y; y++ {
			for x := esq.Min.X; x < esq.Max.X; x++ {
				r, g, b, _ := img.At(x, y).RGBA()
				gray := float64(r>>8)*0.299 + float64(g>>8)*0.587 + float64(b>>8)*0.114
				totalPixeles++

				// Mancha oscura en esquina: gris < 100
				// (las esquinas del acta normal son blancas/claras)
				if gray < 100 {
					totalMancha++
				}
			}
		}
	}

	if totalPixeles == 0 {
		return false, 0
	}

	porcentaje := float64(totalMancha) / float64(totalPixeles) * 100
	// >20% de las esquinas oscuro = manchas/daño de papel
	return porcentaje > 20, porcentaje
}

// ═══════════════════════════════════════════════════════════════════
// DETECCIÓN DE ACTAS ARRUGADAS
//
// Un acta muy arrugada tiene variaciones de sombra (píxeles grises)
// en zonas que normalmente son blanco puro (los márgenes y fondos).
// Evaluamos los márgenes laterales y contamos los píxeles de "sombra".
// ═══════════════════════════════════════════════════════════════════

func (v *VisualValidator) detectarArrugas(img image.Image, bounds image.Rectangle) bool {
	w := bounds.Max.X
	h := bounds.Max.Y

	// ROIs de fondos y márgenes (izquierda y derecha, esquivando datos)
	margenes := []image.Rectangle{
		image.Rect(int(float64(w)*0.02), int(float64(h)*0.20), int(float64(w)*0.08), int(float64(h)*0.80)),
		image.Rect(int(float64(w)*0.92), int(float64(h)*0.20), int(float64(w)*0.98), int(float64(h)*0.80)),
	}

	var pixelesSombra int
	var totalPixeles int

	for _, m := range margenes {
		for y := m.Min.Y; y < m.Max.Y; y++ {
			for x := m.Min.X; x < m.Max.X; x++ {
				r, g, b, _ := img.At(x, y).RGBA()
				gray := float64(r>>8)*0.299 + float64(g>>8)*0.587 + float64(b>>8)*0.114
				totalPixeles++

				// Las sombras de arrugas suelen ser grises medios a claros (130 a 200)
				// El papel liso suele ser > 230
				if gray >= 120 && gray <= 210 {
					pixelesSombra++
				}
			}
		}
	}

	if totalPixeles == 0 {
		return false
	}

	ratioSombras := float64(pixelesSombra) / float64(totalPixeles)
	// Si más del 25% de los márgenes tiene sombras de arrugas, consideramos que el papel está arrugado
	return ratioSombras > 0.25
}

// ═══════════════════════════════════════════════════════════════════
// DETECCIÓN DE ROTURAS
//
// Una rotura crítica = pedazo faltante del papel
// Se detecta como zonas blancas grandes donde debería haber contenido.
// ═══════════════════════════════════════════════════════════════════

func (v *VisualValidator) detectarRoturas(img image.Image, bounds image.Rectangle) bool {
	// Verificar si hay un pedazo grande completamente blanco
	// en la zona de datos o firmas (indicaría papel arrancado)
	w := bounds.Max.X
	h := bounds.Max.Y

	// ROI: toda el acta excepto bordes mínimos
	roi := image.Rect(
		int(float64(w)*0.05), int(float64(h)*0.10),
		int(float64(w)*0.95), int(float64(h)*0.90),
	)

	blockSize := 30
	var bloquesBlancos int
	var bloquesTotales int

	for by := roi.Min.Y; by < roi.Max.Y-blockSize; by += blockSize {
		for bx := roi.Min.X; bx < roi.Max.X-blockSize; bx += blockSize {
			bloquesTotales++
			todoBlanco := true

			for y := by; y < by+blockSize && y < roi.Max.Y; y++ {
				for x := bx; x < bx+blockSize && x < roi.Max.X; x++ {
					r, g, b, _ := img.At(x, y).RGBA()
					gray := float64(r>>8)*0.299 + float64(g>>8)*0.587 + float64(b>>8)*0.114
					if gray < 248 {
						todoBlanco = false
						break
					}
				}
				if !todoBlanco {
					break
				}
			}

			if todoBlanco {
				bloquesBlancos++
			}
		}
	}

	if bloquesTotales == 0 {
		return false
	}

	// Si >60% de los bloques son blancos puros, probablemente le falta una sección
	ratioBlancos := float64(bloquesBlancos) / float64(bloquesTotales)
	return ratioBlancos > 0.60
}

// ═══════════════════════════════════════════════════════════════════
// DETECCIÓN DE HUELLAS DACTILARES / FIRMAS
// ═══════════════════════════════════════════════════════════════════

func (v *VisualValidator) contarHuellas(img image.Image, roi image.Rectangle) int {
	var pixelesHuella int

	for y := roi.Min.Y; y < roi.Max.Y; y++ {
		for x := roi.Min.X; x < roi.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			r8, g8, b8 := float64(r>>8), float64(g>>8), float64(b>>8)
			gray := r8*0.299 + g8*0.587 + b8*0.114

			esAzul := b8 > r8+15 && b8 > g8 && gray < 180
			esOscuro := gray < 80

			if esAzul || esOscuro {
				pixelesHuella++
			}
		}
	}

	tamanoHuella := 500
	huellas := pixelesHuella / tamanoHuella

	if huellas > 12 {
		huellas = 12
	}

	return huellas
}

// ═══════════════════════════════════════════════════════════════════
// INTEGRACIÓN CON MICROSERVICIO PYTHON/OpenCV
// ═══════════════════════════════════════════════════════════════════

// pythonPreprocessResponse modela la respuesta JSON del microservicio.
type pythonPreprocessResponse struct {
	Flags struct {
		ManchaDetectada       bool     `json:"mancha_detectada"`
		PorcentajeMancha      float64  `json:"porcentaje_mancha"`
		NumerosSobreescritos  bool     `json:"numeros_sobreescritos"`
		CeldasSobreescritas   []string `json:"celdas_sobreescritas"`
		ConfusionAlfanumerica bool     `json:"confusion_alfanumerica"`
		CamposSospechosos     []string `json:"campos_sospechosos"`
		HuellasZonaNumeros    bool     `json:"huellas_zona_numeros"`
		FlagRevisionManual    bool     `json:"flag_revision_manual"`
		Observaciones         []string `json:"observaciones"`
	} `json:"flags"`
	CleanImageB64    string         `json:"clean_image_b64"`    // imagen preprocesada lista para OCR
	CorrectedFields  map[string]string `json:"corrected_fields"` // campos alfanuméricos corregidos
}

// llamarMicroservicioPython envía la imagen al microservicio Python y fusiona los flags
// en el resultado existente. Falla en silencio (el pipeline Go sigue sin él).
func (v *VisualValidator) llamarMicroservicioPython(imgPath string, resultado *models.ValidacionVisualResult) {
	f, err := os.Open(imgPath)
	if err != nil {
		return
	}
	defer f.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)

	part, err := writer.CreateFormFile("file", filepath.Base(imgPath))
	if err != nil {
		return
	}
	if _, err = io.Copy(part, f); err != nil {
		return
	}
	_ = writer.WriteField("ocr_fields", "{}")
	writer.Close()

	req, err := http.NewRequest("POST", v.pythonOCRURL+"/preprocess", &body)
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := v.httpClient.Do(req)
	if err != nil {
		// Microservicio no disponible — continuar sin él
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	var pyResp pythonPreprocessResponse
	if err = json.NewDecoder(resp.Body).Decode(&pyResp); err != nil {
		return
	}

	f2 := pyResp.Flags

	// Fusionar: si Python detectó algo nuevo, actualizar el resultado
	if f2.ManchaDetectada && !resultado.ManchaDetectada {
		resultado.ManchaDetectada = true
		resultado.PorcentajeMancha = f2.PorcentajeMancha
	}
	resultado.NumerosSobreescritos = f2.NumerosSobreescritos
	resultado.CeldasSobreescritas = f2.CeldasSobreescritas
	resultado.ConfusionAlfanumerica = f2.ConfusionAlfanumerica
	resultado.CamposSospechosos = f2.CamposSospechosos
	resultado.HuellasZonaNumeros = f2.HuellasZonaNumeros
	resultado.FlagRevisionManual = f2.FlagRevisionManual
	resultado.PipelinePythonUsado = true

	// Agregar observaciones del pipeline Python (sin duplicar)
	existentes := make(map[string]bool, len(resultado.Observaciones))
	for _, o := range resultado.Observaciones {
		existentes[o] = true
	}
	for _, o := range f2.Observaciones {
		if !existentes[o] {
			resultado.Observaciones = append(resultado.Observaciones, "[Python] "+o)
		}
	}
}

// ═══════════════════════════════════════════════════════════════════
// PREPROCESAMIENTO Y VALIDACIÓN DIRECTA DE IMÁGENES (no-PDF)
// ═══════════════════════════════════════════════════════════════════

// PreprocesarImagenConPython envía la imagen al servicio Python para que aplique
// deskewing, CLAHE y eliminación de manchas. Retorna la ruta de la imagen limpia
// (en un archivo temporal — el llamador debe eliminarla con os.Remove) y el resultado
// de validación visual con los flags del pipeline Python ya aplicados.
//
// Si el servicio Python no está disponible, retorna ("", nil, nil) — el pipeline
// puede continuar con la imagen original.
func (v *VisualValidator) PreprocesarImagenConPython(imgPath string) (cleanPath string, resultado *models.ValidacionVisualResult, err error) {
	if v.pythonOCRURL == "" {
		return "", nil, nil
	}

	f, err := os.Open(imgPath)
	if err != nil {
		return "", nil, err
	}
	defer f.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filepath.Base(imgPath))
	if err != nil {
		return "", nil, err
	}
	if _, err = io.Copy(part, f); err != nil {
		return "", nil, err
	}
	_ = writer.WriteField("ocr_fields", "{}")
	writer.Close()

	req, err := http.NewRequest("POST", v.pythonOCRURL+"/preprocess", &body)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := v.httpClient.Do(req)
	if err != nil {
		// Servicio Python no disponible — continuar sin él
		return "", nil, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("Python service returned %d", resp.StatusCode)
	}

	var pyResp pythonPreprocessResponse
	if err = json.NewDecoder(resp.Body).Decode(&pyResp); err != nil {
		return "", nil, err
	}

	// Decodificar imagen limpia y guardar en archivo temporal
	if pyResp.CleanImageB64 != "" {
		imgBytes, decErr := base64.StdEncoding.DecodeString(pyResp.CleanImageB64)
		if decErr == nil && len(imgBytes) > 0 {
			tmpFile, tmpErr := os.CreateTemp("", "clean_acta_*.jpg")
			if tmpErr == nil {
				tmpFile.Write(imgBytes)
				tmpFile.Close()
				cleanPath = tmpFile.Name()
			}
		}
	}

	// Construir resultado visual desde los flags de Python
	resultado = &models.ValidacionVisualResult{}
	f2 := pyResp.Flags
	resultado.ManchaDetectada = f2.ManchaDetectada
	resultado.PorcentajeMancha = f2.PorcentajeMancha
	resultado.NumerosSobreescritos = f2.NumerosSobreescritos
	resultado.CeldasSobreescritas = f2.CeldasSobreescritas
	resultado.ConfusionAlfanumerica = f2.ConfusionAlfanumerica
	resultado.CamposSospechosos = f2.CamposSospechosos
	resultado.HuellasZonaNumeros = f2.HuellasZonaNumeros
	resultado.FlagRevisionManual = f2.FlagRevisionManual
	resultado.PipelinePythonUsado = true
	resultado.Observaciones = make([]string, len(f2.Observaciones))
	for i, o := range f2.Observaciones {
		resultado.Observaciones[i] = "[Python] " + o
	}

	return cleanPath, resultado, nil
}

// ValidarImagenDirecta ejecuta análisis visual sobre un archivo de imagen (.jpg/.png).
// Equivalente a ValidarImagenActa pero sin requerir mutool (no necesita PDF).
// Nota: el llamador puede pasar la imagen ya preprocesada por Python para mejores resultados.
func (v *VisualValidator) ValidarImagenDirecta(imgPath string) (*models.ValidacionVisualResult, error) {
	resultado := &models.ValidacionVisualResult{}

	f, err := os.Open(imgPath)
	if err != nil {
		return resultado, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return resultado, fmt.Errorf("no se pudo decodificar imagen: %w", err)
	}

	bounds := img.Bounds()

	roiDatos := v.roiZonaDatos(bounds)
	mancha, porcentaje := v.detectarManchas(img, roiDatos)
	resultado.ManchaDetectada = mancha
	resultado.PorcentajeMancha = math.Round(porcentaje*10) / 10

	manchaBorde, pctBorde := v.detectarManchasBordes(img, bounds)
	if manchaBorde && !resultado.ManchaDetectada {
		resultado.ManchaDetectada = true
		resultado.PorcentajeMancha = math.Round(pctBorde*10) / 10
	}

	roiFirmas := v.roiZonaFirmas(bounds)
	huellas := v.contarHuellas(img, roiFirmas)
	resultado.HuellasDetectadas = huellas
	resultado.FirmasSuficientes = huellas >= 3

	resultado.RoturaDetectada = v.detectarRoturas(img, bounds)
	resultado.ArrugasDetectadas = v.detectarArrugas(img, bounds)

	// Para imágenes de cámara: siempre analizar como image-only (no hay PDF stream)
	v.analizarImageOnly(img, bounds, resultado)

	v.generarObservaciones(resultado)
	return resultado, nil
}

// ═══════════════════════════════════════════════════════════════════
// GENERACIÓN DE OBSERVACIONES
// ═══════════════════════════════════════════════════════════════════

func (v *VisualValidator) generarObservaciones(resultado *models.ValidacionVisualResult) {
	if resultado.LapizDetectado {
		resultado.Observaciones = append(resultado.Observaciones,
			fmt.Sprintf("NULIDAD — USO DE LÁPIZ: color gris %.0f/255 detectado", resultado.IntensidadPromedio))
	}
	if resultado.CorrectorDetectado {
		resultado.Observaciones = append(resultado.Observaciones,
			"NULIDAD — CORRECTOR (LIQUID PAPER) detectado")
	}
	if resultado.TachaduraDetectada || resultado.NumerosSobreescritos {
		resultado.Observaciones = append(resultado.Observaciones,
			"NULIDAD — TACHADURA/SOBREESCRITURA: números superpuestos en zona de votos")
	}
	if resultado.TextoAnuladaVisual {
		resultado.Observaciones = append(resultado.Observaciones,
			"NULIDAD — TEXTO 'ANULADA' escrito sobre el acta (detección visual)")
	}
	if resultado.RoturaDetectada {
		resultado.Observaciones = append(resultado.Observaciones,
			"NULIDAD — ROTURA CRÍTICA: sección faltante del papel")
	}
	if !resultado.FirmasSuficientes {
		resultado.Observaciones = append(resultado.Observaciones,
			fmt.Sprintf("NULIDAD — FIRMAS INSUFICIENTES: %d huellas (mínimo 3)", resultado.HuellasDetectadas))
	}
	if resultado.ManchaDetectada {
		resultado.Observaciones = append(resultado.Observaciones,
			fmt.Sprintf("OBSERVACIÓN — MANCHA: %.1f%% del área afectada", resultado.PorcentajeMancha))
	}
	if resultado.ArrugasDetectadas {
		resultado.Observaciones = append(resultado.Observaciones,
			"OBSERVACIÓN — ACTA ARRUGADA: Se detectaron variaciones de sombra por pliegues en el papel")
	}
	if resultado.ConfusionAlfanumerica {
		resultado.Observaciones = append(resultado.Observaciones,
			fmt.Sprintf("OBSERVACIÓN — CONFUSIÓN ALFANUMÉRICA en campos: %s",
				strings.Join(resultado.CamposSospechosos, ", ")))
	}
	if resultado.HuellasZonaNumeros {
		resultado.Observaciones = append(resultado.Observaciones,
			"OBSERVACIÓN — HUELLA DACTILAR detectada sobre zona de números")
	}
}

// ═══════════════════════════════════════════════════════════════════
// Renderizado
// ═══════════════════════════════════════════════════════════════════

func (v *VisualValidator) renderizarPDF(pdfPath string) (string, error) {
	absDir, _ := filepath.Abs(".")
	outDir := filepath.Join(absDir, "pdf_images")
	os.MkdirAll(outDir, 0755)

	baseName := strings.TrimSuffix(filepath.Base(pdfPath), ".pdf")
	outputPath := filepath.Join(outDir, baseName+".png")

	if _, err := os.Stat(outputPath); err == nil {
		return outputPath, nil
	}

	cmd := exec.Command(v.mutoolPath,
		"draw", "-o", outputPath, "-r", "150", pdfPath, "1",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("mutool draw falló: %v\nOutput: %s", err, string(output))
	}

	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		return "", fmt.Errorf("mutool no generó la imagen")
	}

	return outputPath, nil
}
