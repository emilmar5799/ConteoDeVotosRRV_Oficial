package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// RealOCRService implementa extracción de texto de PDFs.
// Estrategia PRIMARIA: mutool draw -F txt (extrae texto digital embebido — 100% preciso).
// Estrategia FALLBACK: Tesseract OCR sobre imagen rasterizada (para PDFs escaneados sin texto digital).
type RealOCRService struct {
	tesseractPath string
	mutoolPath    string
	language      string
}

// NewRealOCRService crea un nuevo servicio OCR real.
func NewRealOCRService(tesseractPath, mutoolPath, language string) (*RealOCRService, error) {
	if tesseractPath == "" {
		tesseractPath = `C:\Program Files\Tesseract-OCR\tesseract.exe`
	}
	if mutoolPath == "" {
		absDir, _ := filepath.Abs(".")
		localMutool := filepath.Join(absDir, "mutool.exe")
		if _, err := os.Stat(localMutool); err == nil {
			mutoolPath = localMutool
		} else {
			mutoolPath = "mutool"
		}
	}
	if language == "" {
		language = "spa"
	}

	// Verificar que tesseract está disponible
	cmd := exec.Command(tesseractPath, "--version")
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf(
			"tesseract no encontrado en '%s': %v\n"+
				"Output: %s\n\n"+
				"=== INSTRUCCIONES DE INSTALACIÓN ===\n"+
				"1. Descarga Tesseract desde: https://github.com/UB-Mannheim/tesseract/wiki\n"+
				"2. Ejecuta el instalador .exe\n"+
				"3. IMPORTANTE: Marca la casilla 'Add to PATH' durante la instalación\n"+
				"4. Asegúrate de instalar el idioma 'Spanish' (spa)\n"+
				"5. Reinicia la terminal después de instalar",
			tesseractPath, err, string(output))
	}

	// Verificar que mutool está disponible
	cmd = exec.Command(mutoolPath, "-v")
	if output, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf(
			"mutool no encontrado en '%s': %v\n"+
				"Output: %s\n\n"+
				"=== INSTRUCCIONES DE INSTALACIÓN ===\n"+
				"1. Asegúrate de que mutool.exe está en la carpeta de tu proyecto",
			mutoolPath, err, string(output))
	}

	return &RealOCRService{
		tesseractPath: tesseractPath,
		mutoolPath:    mutoolPath,
		language:      language,
	}, nil
}

// ProcesarPDF extrae texto de un PDF usando una estrategia híbrida:
// 1. PRIMERO intenta extraer texto digital embebido con mutool (100% preciso para PDFs digitales)
// 2. SOLO si el texto embebido es insuficiente, usa Tesseract OCR como fallback
func (s *RealOCRService) ProcesarPDF(pdfPath string) (string, error) {
	// ═══════════════════════════════════════════════════════════
	// ESTRATEGIA 1: Extraer texto embebido con mutool (PRIMARIA)
	// ═══════════════════════════════════════════════════════════
	cmd := exec.Command(s.mutoolPath, "draw", "-F", "txt", pdfPath)
	embeddedBytes, err := cmd.CombinedOutput()
	embeddedText := ""
	if err == nil {
		embeddedText = strings.TrimSpace(string(embeddedBytes))
	}

	// Si hay texto embebido sustancial (más de 20 chars, contiene dígitos),
	// usarlo como fuente primaria — es MUCHO más preciso que OCR
	if len(embeddedText) > 20 && strings.ContainsAny(embeddedText, "0123456789") {
		// Marcar que viene de texto embebido para que el parser lo sepa
		return "--- FUENTE: TEXTO_EMBEBIDO ---\n" + embeddedText, nil
	}

	// ═══════════════════════════════════════════════════════════
	// ESTRATEGIA 2: OCR con Tesseract (FALLBACK para escaneos)
	// ═══════════════════════════════════════════════════════════
	imgPath, err := s.convertPDFToImage(pdfPath)
	if err != nil {
		return "", fmt.Errorf("error convirtiendo PDF a imagen: %w", err)
	}
	defer os.Remove(imgPath)

	textOCR, err := s.RunOCR(imgPath)
	if err != nil {
		return "", fmt.Errorf("error ejecutando OCR: %w", err)
	}

	return "--- FUENTE: TESSERACT_OCR ---\n" + textOCR, nil
}

// ProcesarImagen ejecuta OCR directamente sobre una imagen.
func (s *RealOCRService) ProcesarImagen(imagePath string) (string, error) {
	return s.RunOCR(imagePath)
}

// convertPDFToImage convierte la primera página de un PDF a imagen PNG usando mutool.
func (s *RealOCRService) convertPDFToImage(pdfPath string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "ocr_acta_*")
	if err != nil {
		return "", fmt.Errorf("error creando directorio temporal: %w", err)
	}

	outputPath := filepath.Join(tmpDir, "page.png")

	cmd := exec.Command(s.mutoolPath,
		"draw",
		"-o", outputPath,
		"-r", "300",
		pdfPath,
		"1",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("mutool falló: %v\nOutput: %s", err, string(output))
	}

	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("mutool no generó archivo de imagen")
	}

	return outputPath, nil
}

// RunOCR ejecuta Tesseract sobre una imagen y retorna el texto extraído.
func (s *RealOCRService) RunOCR(imagePath string) (string, error) {
	tmpFile, err := os.CreateTemp("", "ocr_output_*")
	if err != nil {
		return "", fmt.Errorf("error creando archivo temporal: %w", err)
	}
	tmpFile.Close()
	outputBase := tmpFile.Name()
	os.Remove(outputBase) // Tesseract agrega .txt automáticamente

	absDir, _ := filepath.Abs(".")
	tessdataDir := filepath.Join(absDir, "tessdata")

	// --psm 3: Fully automatic page segmentation
	cmd := exec.Command(s.tesseractPath,
		imagePath,
		outputBase,
		"--tessdata-dir", tessdataDir,
		"-l", s.language,
		"--psm", "3",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tesseract falló: %v\nOutput: %s", err, string(output))
	}

	textPath := outputBase + ".txt"
	textBytes, err := os.ReadFile(textPath)
	os.Remove(textPath)
	if err != nil {
		return "", fmt.Errorf("no se pudo leer output de tesseract: %w", err)
	}

	return string(textBytes), nil
}
