package handlers

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"

	"rrv-backend/internal/models"
	"rrv-backend/internal/repository"
	"rrv-backend/internal/service"
)

// UploadHandler maneja la subida de imágenes y PDFs para procesamiento OCR.
type UploadHandler struct {
	actaRepo              *repository.ActaRepository
	eventoRepo            *repository.EventoRepository
	ocrService            *service.OCRService
	actaService           *service.ActaService
	inconsistenciaService *service.InconsistenciaService
	cqrsProjector         *service.CQRSProjector
	uploadDir             string
	retryCfg              service.RetryConfig
}

// NewUploadHandler crea un nuevo handler de subida de archivos.
func NewUploadHandler(
	actaRepo *repository.ActaRepository,
	eventoRepo *repository.EventoRepository,
	ocrService *service.OCRService,
	actaService *service.ActaService,
	inconsistenciaService *service.InconsistenciaService,
	cqrsProjector *service.CQRSProjector,
	uploadDir string,
) *UploadHandler {
	// Crear directorio de uploads si no existe
	os.MkdirAll(uploadDir, 0755)
	return &UploadHandler{
		actaRepo:              actaRepo,
		eventoRepo:            eventoRepo,
		ocrService:            ocrService,
		actaService:           actaService,
		inconsistenciaService: inconsistenciaService,
		cqrsProjector:         cqrsProjector,
		uploadDir:             uploadDir,
		retryCfg:              service.DefaultRetryConfig(),
	}
}

// HandleUpload procesa POST /api/rrv/actas/upload
// Flujo: Recibir archivo → SHA256 → Verificar duplicado → OCR → Validar → Detectar inconsistencias → Guardar → CQRS → Eventos
func (h *UploadHandler) HandleUpload(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	// 1. Recibir archivo multipart
	file, header, err := c.Request.FormFile("archivo")
	if err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Message: "No se recibió archivo. Use el campo 'archivo' en multipart/form-data",
			Errors:  []string{err.Error()},
		})
		return
	}
	defer file.Close()

	// Validar extensión
	ext := filepath.Ext(header.Filename)
	extensionesValidas := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".pdf": true, ".bmp": true, ".tiff": true}
	if !extensionesValidas[ext] {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Message: fmt.Sprintf("Extensión no válida: %s. Se aceptan: jpg, jpeg, png, pdf, bmp, tiff", ext),
		})
		return
	}

	// 2. Leer contenido y calcular SHA256
	content, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Message: "Error leyendo archivo",
			Errors:  []string{err.Error()},
		})
		return
	}

	hashBytes := sha256.Sum256(content)
	fileHash := fmt.Sprintf("%x", hashBytes)

	// 3. Registrar evento: archivo recibido
	h.registrarEvento(ctx, "PENDIENTE", models.EventoActaRecibida,
		fmt.Sprintf("Archivo recibido: %s (%d bytes)", header.Filename, len(content)),
		map[string]any{"filename": header.Filename, "size": len(content), "hash": fileHash},
	)

	// 4. Verificar duplicado por hash (con retry)
	var existente *models.ActaRRV
	err = service.WithRetry(ctx, h.retryCfg, "FindByHash", func() error {
		var e error
		existente, e = h.actaRepo.FindByHash(ctx, fileHash)
		return e
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Message: "Error verificando duplicados",
			Errors:  []string{err.Error()},
		})
		return
	}
	if existente != nil {
		// Detectar inconsistencia: duplicado con datos diferentes
		h.inconsistenciaService.DetectarYRegistrar(ctx, existente, "UPLOAD")

		h.registrarEvento(ctx, existente.ActaID, models.EventoDuplicadoDetectado,
			fmt.Sprintf("Archivo duplicado detectado: hash=%s, acta_id=%s", fileHash, existente.ActaID),
			map[string]any{"hash": fileHash, "acta_id_existente": existente.ActaID},
		)
		c.JSON(http.StatusConflict, models.APIResponse{
			Success: false,
			Message: fmt.Sprintf("Archivo duplicado: este contenido ya fue procesado como acta %s", existente.ActaID),
			Data:    existente,
		})
		return
	}

	// 5. Guardar archivo temporalmente
	savedPath := filepath.Join(h.uploadDir, fmt.Sprintf("%s%s", fileHash[:16], ext))
	if err := os.WriteFile(savedPath, content, 0644); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Message: "Error guardando archivo temporal",
			Errors:  []string{err.Error()},
		})
		return
	}

	// 6. Procesar OCR (simulado)
	acta := h.ocrService.ProcesarArchivo(fileHash, header.Filename)
	acta.FechaRecepcion = time.Now()

	h.registrarEvento(ctx, acta.ActaID, models.EventoOCRProcesado,
		fmt.Sprintf("OCR procesado: acta_id=%s, %d candidatos detectados", acta.ActaID, len(acta.Candidatos)),
		map[string]any{"acta_id": acta.ActaID, "candidatos": len(acta.Candidatos)},
	)

	// 7. Validar datos del acta
	erroresValidacion := h.actaService.ValidarActa(acta)
	acta.Estado = h.actaService.DeterminarEstado(acta, erroresValidacion)
	acta.Errores = erroresValidacion

	if len(erroresValidacion) > 0 {
		h.registrarEvento(ctx, acta.ActaID, models.EventoValidacionError,
			fmt.Sprintf("Validación con errores: %d problemas encontrados", len(erroresValidacion)),
			map[string]any{"errores": erroresValidacion},
		)
	} else {
		h.registrarEvento(ctx, acta.ActaID, models.EventoValidacionOK,
			"Validación exitosa: todos los campos correctos",
			nil,
		)
	}

	// 8. Detectar inconsistencias contra datos de referencia (las 4 del documento)
	inconsistencias := h.inconsistenciaService.DetectarYRegistrar(ctx, acta, "UPLOAD")
	if len(inconsistencias) > 0 {
		h.registrarEvento(ctx, acta.ActaID, "INCONSISTENCIAS_DETECTADAS",
			fmt.Sprintf("%d inconsistencias detectadas y registradas en Logs_Inconsistencias", len(inconsistencias)),
			map[string]any{"cantidad": len(inconsistencias)},
		)
	}

	// 9. Verificar duplicado por acta_id
	existentePorID, _ := h.actaRepo.FindByActaID(ctx, acta.ActaID)
	if existentePorID != nil {
		h.registrarEvento(ctx, acta.ActaID, models.EventoDuplicadoDetectado,
			fmt.Sprintf("Acta duplicada por ID: acta_id=%s", acta.ActaID),
			map[string]any{"acta_id": acta.ActaID},
		)
		c.JSON(http.StatusConflict, models.APIResponse{
			Success: false,
			Message: fmt.Sprintf("Acta con ID %s ya existe en el sistema", acta.ActaID),
			Data:    existentePorID,
		})
		return
	}

	// 10. Guardar acta en MongoDB (con retry para tolerancia a fallos)
	err = service.WithRetry(ctx, h.retryCfg, "InsertActa", func() error {
		return h.actaRepo.InsertActa(ctx, acta)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Message: "Error guardando acta en base de datos",
			Errors:  []string{err.Error()},
		})
		return
	}

	h.registrarEvento(ctx, acta.ActaID, models.EventoActaGuardada,
		fmt.Sprintf("Acta guardada exitosamente: estado=%s", acta.Estado),
		map[string]any{"estado": acta.Estado},
	)

	// 11. Actualizar vista materializada CQRS (asíncrono)
	h.cqrsProjector.Proyectar(ctx)

	// 12. Respuesta exitosa
	statusCode := http.StatusCreated
	message := fmt.Sprintf("Acta %s procesada y guardada exitosamente", acta.ActaID)
	if acta.Estado == models.EstadoError {
		message = fmt.Sprintf("Acta %s guardada con errores de validación", acta.ActaID)
	}

	c.JSON(statusCode, models.APIResponse{
		Success: true,
		Message: message,
		Data:    acta,
		Errors:  erroresValidacion,
	})
}

// registrarEvento es un helper para registrar eventos del pipeline.
func (h *UploadHandler) registrarEvento(ctx context.Context, actaID, tipoEvento, descripcion string, metadata map[string]any) {
	evento := &models.EventoRRV{
		ActaID:      actaID,
		TipoEvento:  tipoEvento,
		Descripcion: descripcion,
		Metadata:    metadata,
		Fecha:       time.Now(),
	}
	// Registro fire-and-forget — no debe bloquear el flujo principal
	_ = h.eventoRepo.InsertEvento(ctx, evento)
}
