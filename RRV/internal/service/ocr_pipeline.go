package service

import (
	"fmt"
	"log"
	"os"
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

// OCRPipeline encadena los tres servicios en el orden correcto:
//  1. RealOCRService extrae texto crudo.
//  2. ActaParser convierte texto a ActaRRV estructurada.
//  3. VisualValidator detecta anomalias visuales.
type OCRPipeline struct {
	realOCR *RealOCRService
	parser  *ActaParser
	visual  *VisualValidator
}

// NewOCRPipeline crea el pipeline real.
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
	esPDF := ext == ".pdf"

	var rawText string
	var err error
	var pyVisual *models.ValidacionVisualResult
	voteOCRPath := filePath
	visualInputPath := filePath
	referencePDFPath := ""

	if esPDF {
		rawText, err = p.realOCR.ProcesarPDF(filePath)
	} else {
		ocrInputPath := filePath
		croppedPath, cropped, cropErr := recortarImagenActa(filePath)
		if cropErr == nil && croppedPath != "" {
			defer os.Remove(croppedPath)
			ocrInputPath = croppedPath
			voteOCRPath = croppedPath
			visualInputPath = croppedPath
			if cropped {
				log.Printf("[OCR-CROP] Imagen recortada al area probable del acta: %s", filepath.Base(croppedPath))
			}
		} else if cropErr != nil {
			log.Printf("[OCR-CROP] No se pudo recortar imagen, se usa original: %v", cropErr)
		}

		cleanPath, pyResult, _ := p.visual.PreprocesarImagenConPython(ocrInputPath)
		pyVisual = pyResult

		ocrPath := ocrInputPath
		if cleanPath != "" {
			defer os.Remove(cleanPath)
			ocrPath = cleanPath
		}

		rawText, err = p.realOCR.ProcesarImagen(ocrPath)
	}
	if err != nil {
		return nil, fmt.Errorf("extraccion de texto fallo (%s): %w", ext, err)
	}

	logText := rawText
	if !esPDF {
		logText = limitarTextoAlCuerpoActa(logText)
	}
	if len(logText) > 800 {
		logText = logText[:800] + "...[truncado]"
	}
	log.Printf("[OCR-RAW] Archivo=%s ext=%s\n--- TEXTO OCR INICIO ---\n%s\n--- TEXTO OCR FIN ---", fileName, ext, logText)

	parseFileName := fileName
	if !esPDF {
		parseFileName = ""
	}
	acta := p.parser.ParseActaText(rawText, parseFileName)
	acta.FechaRecepcion = time.Now()

	if !esPDF {
		if ok, pdfErr := p.completarDesdePDFPorCodigo(acta); ok {
			if pdfPath, found := buscarPDFActaPorCodigo(acta.CodigoMesa); found {
				referencePDFPath = pdfPath
			}
			log.Printf("[OCR-PDF-LOOKUP] Acta completada desde pdf local por codigo=%s", acta.CodigoMesa)
		} else if pdfErr != nil {
			log.Printf("[OCR-PDF-LOOKUP] No se pudo completar desde pdf local codigo=%s: %v", acta.CodigoMesa, pdfErr)
		}
	}

	if !esPDF && votosEnCero(acta) {
		if votos, verr := p.realOCR.ExtraerVotosPorCasillas(voteOCRPath); verr == nil {
			acta.Candidatos = votos.Candidatos
			acta.VotosValidos = votos.Validos
			acta.VotosBlancos = votos.Blancos
			acta.VotosNulos = votos.Nulos
			acta.TotalVotos = votos.Validos + votos.Blancos + votos.Nulos
			log.Printf("[OCR-VOTES] Votos rescatados por casillas perfil=%s score=%d validos=%d blancos=%d nulos=%d",
				votos.Profile, votos.Score, votos.Validos, votos.Blancos, votos.Nulos)
		} else {
			log.Printf("[OCR-VOTES] Fallback por casillas no pudo leer votos: %v", verr)
		}
	}

	log.Printf("[OCR-PARSED] acta_id=%s dep=%q prov=%q mun=%q recinto=%q mesa=%q codigo=%q candidatos=%d validos=%d blancos=%d nulos=%d",
		acta.ActaID, acta.Departamento, acta.Provincia, acta.Municipio, acta.Recinto,
		acta.Mesa, acta.CodigoMesa, len(acta.Candidatos), acta.VotosValidos, acta.VotosBlancos, acta.VotosNulos,
	)

	if esPDF {
		acta.TipoEntrada = models.TipoPDF
	} else {
		acta.TipoEntrada = models.TipoImagen
	}

	if esPDF {
		if result, verr := p.visual.ValidarImagenActa(filePath); verr == nil {
			acta.ValidacionVisual = result
		}
	} else {
		if referencePDFPath != "" {
			if result, verr := p.visual.ValidarImagenActa(referencePDFPath); verr == nil {
				acta.ValidacionVisual = result
			}
		}
		if acta.ValidacionVisual == nil {
			result, verr := p.visual.ValidarImagenDirecta(visualInputPath)
			if verr != nil {
				return acta, nil
			}
			if pyVisual != nil {
				fusionarValidacionVisualImagen(result, pyVisual)
			}
			acta.ValidacionVisual = result
		}
	}

	return acta, nil
}

func votosEnCero(acta *models.ActaRRV) bool {
	if acta.VotosValidos != 0 || acta.VotosBlancos != 0 || acta.VotosNulos != 0 {
		return false
	}
	for _, c := range acta.Candidatos {
		if c.Votos != 0 {
			return false
		}
	}
	return true
}

// fusionarValidacionVisual copia los flags del pipeline Python al resultado nativo Go,
// sin sobreescribir detecciones que Go ya marco como positivas.
func fusionarValidacionVisual(destino, pythonResult *models.ValidacionVisualResult) {
	if pythonResult == nil {
		return
	}
	if pythonResult.ManchaDetectada && !destino.ManchaDetectada {
		destino.ManchaDetectada = true
		destino.PorcentajeMancha = pythonResult.PorcentajeMancha
	}
	if pythonResult.NumerosSobreescritos {
		destino.NumerosSobreescritos = true
		destino.CeldasSobreescritas = pythonResult.CeldasSobreescritas
	}
	if pythonResult.ConfusionAlfanumerica {
		destino.ConfusionAlfanumerica = true
		destino.CamposSospechosos = pythonResult.CamposSospechosos
	}
	if pythonResult.HuellasZonaNumeros {
		destino.HuellasZonaNumeros = true
	}
	destino.FlagRevisionManual = destino.FlagRevisionManual || pythonResult.FlagRevisionManual
	destino.PipelinePythonUsado = pythonResult.PipelinePythonUsado

	existentes := make(map[string]bool, len(destino.Observaciones))
	for _, o := range destino.Observaciones {
		existentes[o] = true
	}
	for _, o := range pythonResult.Observaciones {
		if !existentes[o] {
			destino.Observaciones = append(destino.Observaciones, o)
		}
	}
}

// fusionarValidacionVisualImagen conserva la mancha calculada por Go para fotos.
func fusionarValidacionVisualImagen(destino, pythonResult *models.ValidacionVisualResult) {
	if pythonResult == nil {
		return
	}
	manchaDetectada := destino.ManchaDetectada
	porcentajeMancha := destino.PorcentajeMancha
	fusionarValidacionVisual(destino, pythonResult)
	destino.ManchaDetectada = manchaDetectada
	destino.PorcentajeMancha = porcentajeMancha
}

// MockPipeline adapta el OCRService (mock) a la interfaz ActaProcessor.
type MockPipeline struct {
	svc *OCRService
}

func NewMockPipeline() *MockPipeline {
	return &MockPipeline{svc: NewOCRService()}
}

func (m *MockPipeline) ProcesarArchivo(filePath, fileName string) (*models.ActaRRV, error) {
	acta := m.svc.ProcesarArchivo(fileName, fileName)
	acta.FechaRecepcion = time.Now()
	return acta, nil
}
