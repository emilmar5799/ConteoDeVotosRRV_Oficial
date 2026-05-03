package service

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"rrv-backend/internal/models"
)

// ActaProcessor es la interfaz que consume el UploadHandler.
// Permite intercambiar el pipeline real por el mock sin cambiar el handler.
type ActaProcessor interface {
	ProcesarArchivo(filePath, fileName string) (*models.ActaRRV, error)
}

// ═══════════════════════════════════════════════════════════════════
// OCRPipeline — pipeline real: Tesseract + mutool + parser + validador
// ═══════════════════════════════════════════════════════════════════

// OCRPipeline encadena los tres servicios en el orden correcto:
//  1. RealOCRService  → extrae texto crudo (mutool primario, Tesseract fallback)
//  2. ActaParser      → convierte texto → ActaRRV estructurada
//  3. VisualValidator → detecta anomalías visuales (solo PDFs)
type OCRPipeline struct {
	realOCR *RealOCRService
	parser  *ActaParser
	visual  *VisualValidator
}

// NewOCRPipeline crea el pipeline real.
// Retorna error si Tesseract o mutool no están disponibles.
func NewOCRPipeline(tesseractPath, mutoolPath string) (*OCRPipeline, error) {
	real, err := NewRealOCRService(tesseractPath, mutoolPath, "spa")
	if err != nil {
		return nil, fmt.Errorf("no se pudo iniciar pipeline OCR real: %w", err)
	}
	return &OCRPipeline{
		realOCR: real,
		parser:  NewActaParser(),
		visual:  NewVisualValidator(real.mutoolPath),
	}, nil
}

// ProcesarArchivo ejecuta el pipeline completo sobre un archivo guardado en disco.
func (p *OCRPipeline) ProcesarArchivo(filePath, fileName string) (*models.ActaRRV, error) {
	ext := strings.ToLower(filepath.Ext(fileName))

	// ── Paso 1: extraer texto crudo ──────────────────────────────────
	var rawText string
	var err error

	if ext == ".pdf" {
		rawText, err = p.realOCR.ProcesarPDF(filePath)
	} else {
		rawText, err = p.realOCR.ProcesarImagen(filePath)
	}
	if err != nil {
		return nil, fmt.Errorf("extracción de texto falló (%s): %w", ext, err)
	}

	// ── Paso 2: parsear texto → ActaRRV ─────────────────────────────
	acta := p.parser.ParseActaText(rawText, fileName)
	acta.FechaRecepcion = time.Now()

	// ── Paso 3: validación visual (solo PDFs, requiere mutool) ───────
	if ext == ".pdf" {
		if result, verr := p.visual.ValidarImagenActa(filePath); verr == nil {
			acta.ValidacionVisual = result
		}
	}

	return acta, nil
}

// ═══════════════════════════════════════════════════════════════════
// MockPipeline — wrapper del OCRService mock para compatibilidad
// ═══════════════════════════════════════════════════════════════════

// MockPipeline adapta el OCRService (mock) a la interfaz ActaProcessor.
// Se usa cuando Tesseract no está instalado.
type MockPipeline struct {
	svc *OCRService
}

func NewMockPipeline() *MockPipeline {
	return &MockPipeline{svc: NewOCRService()}
}

func (m *MockPipeline) ProcesarArchivo(filePath, fileName string) (*models.ActaRRV, error) {
	// El mock usa el nombre de archivo como seed (no necesita leerlo)
	acta := m.svc.ProcesarArchivo(fileName, fileName)
	acta.FechaRecepcion = time.Now()
	return acta, nil
}
