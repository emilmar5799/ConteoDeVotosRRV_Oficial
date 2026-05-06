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
	mutoolPath   string
	pythonOCRURL string // URL del microservicio Python (vacío = deshabilitado)
	httpClient   *http.Client
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

	if !resultado.LapizDetectado {
		v.detectarLapizImagen(img, bounds, resultado)
	}

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
	v.descartarManchaSiEsArruga(resultado)

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

		// Lápiz: gris uniforme en rango de grafito (R≈G≈B, 0.4–0.9).
		// Los fondos claros del formulario (> 0.9) y el negro puro (< 0.4) quedan excluidos.
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

func (v *VisualValidator) detectarLapizImagen(img image.Image, bounds image.Rectangle, resultado *models.ValidacionVisualResult) {
	roiCandidatos := image.Rect(
		int(float64(bounds.Max.X)*0.32), int(float64(bounds.Max.Y)*0.29),
		int(float64(bounds.Max.X)*0.40), int(float64(bounds.Max.Y)*0.47),
	)
	ok, promedio := detectarFilaCandidatosLapiz(img, roiCandidatos)
	if ok {
		resultado.LapizDetectado = true
		resultado.IntensidadPromedio = promedio
		resultado.RatioContraste = 1.0 - promedio/255.0
	}
}

func detectarFilaCandidatosLapiz(img image.Image, roi image.Rectangle) (bool, float64) {
	const rows = 4
	const cols = 3
	cellW := (roi.Max.X - roi.Min.X) / cols
	cellH := (roi.Max.Y - roi.Min.Y) / rows
	if cellW <= 0 || cellH <= 0 {
		return false, 0
	}

	for row := 0; row < rows; row++ {
		celdasSospechosas := 0
		var rowGrayPx int
		var rowGraySum float64
		for col := 0; col < cols; col++ {
			x0 := roi.Min.X + col*cellW + maxInt(2, cellW/5)
			x1 := roi.Min.X + (col+1)*cellW - maxInt(2, cellW/5)
			y0 := roi.Min.Y + row*cellH + maxInt(2, cellH/5)
			y1 := roi.Min.Y + (row+1)*cellH - maxInt(2, cellH/5)
			if x1 <= x0 || y1 <= y0 {
				continue
			}

			var grayPx, darkPx int
			var cellGraySum float64
			for y := y0; y < y1; y++ {
				for x := x0; x < x1; x++ {
					r, g, b, _ := img.At(x, y).RGBA()
					r8, g8, b8 := float64(r>>8), float64(g>>8), float64(b>>8)
					gray := r8*0.299 + g8*0.587 + b8*0.114
					maxC := math.Max(r8, math.Max(g8, b8))
					minC := math.Min(r8, math.Min(g8, b8))
					sat := maxC - minC

					esGrafito := gray >= 95 && gray <= 205 && sat <= 24
					esOscuro := gray < 80

					if esGrafito {
						grayPx++
						cellGraySum += gray
					}
					if esOscuro {
						darkPx++
					}
				}
			}

			avg := 0.0
			if grayPx > 0 {
				avg = cellGraySum / float64(grayPx)
			}
			if grayPx >= 95 && grayPx <= 260 && darkPx <= 60 && avg >= 145 && avg <= 185 {
				celdasSospechosas++
				rowGrayPx += grayPx
				rowGraySum += cellGraySum
			}
		}
		if celdasSospechosas >= 2 && rowGrayPx > 0 {
			return true, rowGraySum / float64(rowGrayPx)
		}
	}
	return false, 0
}

func medirTrazosGrafito(mask []bool, w, h int, img image.Image, roi image.Rectangle) (int, float64) {
	visited := make([]bool, len(mask))
	dirs := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	var totalArea int
	var sumaGray float64

	for i := range mask {
		if !mask[i] || visited[i] {
			continue
		}

		queue := []int{i}
		visited[i] = true
		area := 0
		minX, maxX := w, 0
		minY, maxY := h, 0
		var compGray float64

		for len(queue) > 0 {
			idx := queue[0]
			queue = queue[1:]
			x := idx % w
			y := idx / w
			area++
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}

			r, g, b, _ := img.At(roi.Min.X+x, roi.Min.Y+y).RGBA()
			compGray += float64(r>>8)*0.299 + float64(g>>8)*0.587 + float64(b>>8)*0.114

			for _, d := range dirs {
				nx, ny := x+d[0], y+d[1]
				if nx < 0 || nx >= w || ny < 0 || ny >= h {
					continue
				}
				ni := ny*w + nx
				if mask[ni] && !visited[ni] {
					visited[ni] = true
					queue = append(queue, ni)
				}
			}
		}

		bw := maxX - minX + 1
		bh := maxY - minY + 1
		if area >= 6 && area <= 900 && bw >= 2 && bh >= 3 && bw <= w/3 && bh <= h/4 {
			totalArea += area
			sumaGray += compGray
		}
	}

	if totalArea == 0 {
		return 0, 0
	}
	return totalArea, sumaGray / float64(totalArea)
}

// detectarTachadurasImagen detecta números superpuestos analizando
// la densidad de tinta en las celdas numéricas.
func (v *VisualValidator) detectarTachadurasImagen(img image.Image, roi image.Rectangle) bool {
	// Dividir la ROI en sub-celdas y buscar celdas con densidad excesiva
	cellH := (roi.Max.Y - roi.Min.Y) / 8 // ~8 filas de números
	cellW := (roi.Max.X - roi.Min.X) / 3 // 3 dígitos por fila
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
	// Cubre candidatos, votos, totales y parte inferior del acta.
	// Llega hasta y=88% para capturar manchas en la zona baja,
	// y hasta x=67% para no entrar en la zona de huellas (derecha).
	return image.Rect(
		int(float64(w)*0.05), int(float64(h)*0.10),
		int(float64(w)*0.67), int(float64(h)*0.88),
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
	totalPixeles := (roi.Max.X - roi.Min.X) * (roi.Max.Y - roi.Min.Y)
	if totalPixeles == 0 {
		return false, 0
	}
	roiW := roi.Max.X - roi.Min.X
	roiH := roi.Max.Y - roi.Min.Y
	mask := make([]bool, totalPixeles)

	for y := roi.Min.Y; y < roi.Max.Y; y++ {
		for x := roi.Min.X; x < roi.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			r8, g8, b8 := float64(r>>8), float64(g>>8), float64(b>>8)
			gray := r8*0.299 + g8*0.587 + b8*0.114
			maxC := math.Max(r8, math.Max(g8, b8))
			minC := math.Min(r8, math.Min(g8, b8))
			saturacion := maxC - minC

			// Mancha cálida: color café/naranja — nunca aparece en un acta limpia
			manchaCalida := gray < 225 && gray > 70 && saturacion > 18 && r8 > g8+4 && r8 > b8+10
			// Mancha oscura difusa: suciedad, café seco, polvo de tinta
			// (excluye texto negro puro < 40 y fondos claros del formulario > 100)
			manchaOscura := gray >= 35 && gray < 125 && saturacion < 25
			// NO incluir manchaTintaAzul aquí: el formulario tiene fondos de celda
			// azules que darían falsos positivos masivos en esta zona.
			// La tinta azul derramada se detecta en detectarManchasBordes.

			manchaAzulDerramada := gray < 210 && saturacion > 35 && b8 > r8+12 && b8 > g8+4

			if manchaCalida || manchaOscura || manchaAzulDerramada {
				mask[(y-roi.Min.Y)*roiW+(x-roi.Min.X)] = true
			}
		}
	}

	pixelesMancha := contarPixelesEnManchas(mask, roiW, roiH, totalPixeles)
	porcentaje := float64(pixelesMancha) / float64(totalPixeles) * 100
	return porcentaje > 1.4, porcentaje
}

// detectarManchasBordes detecta manchas de color en las franjas perimetrales
// del acta: tinta azul derramada, lavados, manchas de café en bordes.
// Evalúa franjas de ~10% a lo largo de los bordes izquierdo, inferior y superior,
// buscando píxeles cálidos (café) o tinta azul oscura fuera del fondo blanco normal.
func (v *VisualValidator) detectarManchasBordes(img image.Image, bounds image.Rectangle) (bool, float64) {
	w := bounds.Max.X
	h := bounds.Max.Y

	// Franjas perimetrales (excluimos el 35 % derecho para evitar la zona de huellas)
	franjas := []image.Rectangle{
		// Borde izquierdo completo
		image.Rect(0, int(float64(h)*0.10), int(float64(w)*0.10), int(float64(h)*0.90)),
		// Borde inferior (sin zona de huellas a la derecha)
		image.Rect(int(float64(w)*0.05), int(float64(h)*0.85), int(float64(w)*0.70), h),
		// Borde superior
		image.Rect(int(float64(w)*0.05), 0, int(float64(w)*0.70), int(float64(h)*0.12)),
		// Esquina superior derecha: puede tener daño negro sin tocar la zona de huellas.
		image.Rect(int(float64(w)*0.88), 0, w, int(float64(h)*0.15)),
	}

	var totalMancha int
	var totalPixeles int

	for _, franja := range franjas {
		fw := franja.Max.X - franja.Min.X
		fh := franja.Max.Y - franja.Min.Y
		mask := make([]bool, fw*fh)
		for y := franja.Min.Y; y < franja.Max.Y; y++ {
			for x := franja.Min.X; x < franja.Max.X; x++ {
				r, g, b, _ := img.At(x, y).RGBA()
				r8 := float64(r >> 8)
				g8 := float64(g >> 8)
				b8 := float64(b >> 8)
				gray := r8*0.299 + g8*0.587 + b8*0.114
				maxC := math.Max(r8, math.Max(g8, b8))
				minC := math.Min(r8, math.Min(g8, b8))
				sat := maxC - minC
				totalPixeles++

				manchaCalida := gray < 225 && gray > 65 && sat > 18 && r8 > g8+4 && r8 > b8+10
				manchaTintaAzul := gray < 210 && sat > 35 && b8 > r8+12 && b8 > g8+4
				manchaOscura := gray < 95 && sat < 40
				if manchaCalida || manchaTintaAzul || manchaOscura {
					mask[(y-franja.Min.Y)*fw+(x-franja.Min.X)] = true
				}
			}
		}
		totalMancha += contarPixelesEnManchasBorde(mask, fw, fh, fw*fh)
	}

	if totalPixeles == 0 {
		return false, 0
	}

	porcentaje := float64(totalMancha) / float64(totalPixeles) * 100
	return porcentaje > 2.5, porcentaje
}

func contarPixelesEnManchas(mask []bool, w, h, totalPixeles int) int {
	return contarPixelesEnManchasConOpciones(mask, w, h, totalPixeles, false)
}

func contarPixelesEnManchasBorde(mask []bool, w, h, totalPixeles int) int {
	return contarPixelesEnManchasConOpciones(mask, w, h, totalPixeles, true)
}

func contarPixelesEnManchasConOpciones(mask []bool, w, h, totalPixeles int, permitirGrande bool) int {
	if w <= 0 || h <= 0 || len(mask) == 0 {
		return 0
	}

	visited := make([]bool, len(mask))
	minArea := int(math.Max(180, float64(totalPixeles)*0.0012))
	var total int
	dirs := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}

	for i := range mask {
		if !mask[i] || visited[i] {
			continue
		}

		queue := []int{i}
		visited[i] = true
		area := 0
		minX, maxX := w, 0
		minY, maxY := h, 0

		for len(queue) > 0 {
			idx := queue[0]
			queue = queue[1:]
			x := idx % w
			y := idx / w
			area++
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}

			for _, d := range dirs {
				nx, ny := x+d[0], y+d[1]
				if nx < 0 || nx >= w || ny < 0 || ny >= h {
					continue
				}
				ni := ny*w + nx
				if mask[ni] && !visited[ni] {
					visited[ni] = true
					queue = append(queue, ni)
				}
			}
		}

		bw := maxX - minX + 1
		bh := maxY - minY + 1
		if bw <= 0 || bh <= 0 {
			continue
		}
		rectangularidad := float64(area) / float64(bw*bh)
		demasiadoFino := bw < 8 || bh < 8
		demasiadoGrande := !permitirGrande &&
			(area > int(float64(totalPixeles)*0.12) || (bw > int(float64(w)*0.45) && bh > int(float64(h)*0.25)))
		formularioImpreso := rectangularidad > 0.72 && bw > w/5 && bh > h/8

		if area >= minArea && !demasiadoFino && !demasiadoGrande && !formularioImpreso {
			total += area
		}
	}

	return total
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

func (v *VisualValidator) descartarManchaSiEsArruga(resultado *models.ValidacionVisualResult) {
	if resultado.ArrugasDetectadas && resultado.ManchaDetectada && resultado.PorcentajeMancha >= 8.0 {
		resultado.ManchaDetectada = false
		resultado.PorcentajeMancha = 0
	}
}

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
	CleanImageB64   string            `json:"clean_image_b64"`  // imagen preprocesada lista para OCR
	CorrectedFields map[string]string `json:"corrected_fields"` // campos alfanuméricos corregidos
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

	v.detectarLapizImagen(img, bounds, resultado)

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
	v.descartarManchaSiEsArruga(resultado)

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
