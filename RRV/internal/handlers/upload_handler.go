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
	actaRepo    *repository.ActaRepository
	eventoRepo  *repository.EventoRepository
	processor   service.ActaProcessor
	actaService *service.ActaService
	uploadDir   string
}

// NewUploadHandler crea un nuevo handler de subida de archivos.
func NewUploadHandler(
	actaRepo *repository.ActaRepository,
	eventoRepo *repository.EventoRepository,
	processor service.ActaProcessor,
	actaService *service.ActaService,
	uploadDir string,
) *UploadHandler {
	os.MkdirAll(uploadDir, 0755)
	return &UploadHandler{
		actaRepo:    actaRepo,
		eventoRepo:  eventoRepo,
		processor:   processor,
		actaService: actaService,
		uploadDir:   uploadDir,
	}
}

// HandleUpload procesa POST /api/rrv/actas/upload
// Flujo: Recibir archivo → SHA256 → Verificar duplicado → OCR → Validar → Guardar → Registrar eventos
func (h *UploadHandler) HandleUpload(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
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
	extensionesValidas := map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true,
		".pdf": true, ".bmp": true, ".tiff": true,
	}
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
			Success: false, Message: "Error leyendo archivo", Errors: []string{err.Error()},
		})
		return
	}

	hashBytes := sha256.Sum256(content)
	fileHash := fmt.Sprintf("%x", hashBytes)

	h.registrarEvento(ctx, "PENDIENTE", models.EventoActaRecibida,
		fmt.Sprintf("Archivo recibido: %s (%d bytes)", header.Filename, len(content)),
		map[string]any{"filename": header.Filename, "size": len(content), "hash": fileHash},
	)

	// 3. Verificar duplicado por hash
	existente, err := h.actaRepo.FindByHash(ctx, fileHash)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false, Message: "Error verificando duplicados", Errors: []string{err.Error()},
		})
		return
	}
	if existente != nil {
		h.registrarEvento(ctx, existente.ActaID, models.EventoDuplicadoDetectado,
			fmt.Sprintf("Archivo duplicado detectado: hash=%s", fileHash),
			map[string]any{"hash": fileHash, "acta_id_existente": existente.ActaID},
		)
		c.JSON(http.StatusConflict, models.APIResponse{
			Success: false,
			Message: fmt.Sprintf("Archivo duplicado: ya fue procesado como acta %s", existente.ActaID),
			Data:    existente,
		})
		return
	}

	// 4. Guardar archivo en disco (el pipeline OCR lo necesita)
	savedPath := filepath.Join(h.uploadDir, fmt.Sprintf("%s%s", fileHash[:16], ext))
	if err := os.WriteFile(savedPath, content, 0644); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false, Message: "Error guardando archivo temporal", Errors: []string{err.Error()},
		})
		return
	}

	// 5. Ejecutar pipeline OCR real (Tesseract + parser + validador visual)
	acta, err := h.processor.ProcesarArchivo(savedPath, header.Filename)
	if err != nil {
		h.registrarEvento(ctx, "ERROR_OCR", models.EventoOCRProcesado,
			fmt.Sprintf("Error OCR: %v", err),
			map[string]any{"filename": header.Filename, "error": err.Error()},
		)
		c.JSON(http.StatusUnprocessableEntity, models.APIResponse{
			Success: false,
			Message: "Error procesando OCR del archivo",
			Errors:  []string{err.Error()},
		})
		return
	}
	acta.HashOrigen = fileHash

	h.registrarEvento(ctx, acta.ActaID, models.EventoOCRProcesado,
		fmt.Sprintf("OCR completado: acta_id=%s, %d candidatos", acta.ActaID, len(acta.Candidatos)),
		map[string]any{"acta_id": acta.ActaID, "candidatos": len(acta.Candidatos)},
	)

	// 6. Validar datos del acta
	erroresValidacion := h.actaService.ValidarActa(acta)
	acta.Estado = h.actaService.DeterminarEstado(acta, erroresValidacion)
	acta.Errores = erroresValidacion

	if len(erroresValidacion) > 0 {
		h.registrarEvento(ctx, acta.ActaID, models.EventoValidacionError,
			fmt.Sprintf("Validación con %d errores", len(erroresValidacion)),
			map[string]any{"errores": erroresValidacion},
		)
	} else {
		h.registrarEvento(ctx, acta.ActaID, models.EventoValidacionOK,
			"Validación exitosa", nil,
		)
	}

	// 7. Verificar duplicado por acta_id
	if existentePorID, _ := h.actaRepo.FindByActaID(ctx, acta.ActaID); existentePorID != nil {
		h.registrarEvento(ctx, acta.ActaID, models.EventoDuplicadoDetectado,
			fmt.Sprintf("Acta duplicada por ID: %s", acta.ActaID),
			map[string]any{"acta_id": acta.ActaID},
		)
		c.JSON(http.StatusConflict, models.APIResponse{
			Success: false,
			Message: fmt.Sprintf("Acta con ID %s ya existe en el sistema", acta.ActaID),
			Data:    existentePorID,
		})
		return
	}

	// 8. Guardar acta en MongoDB
	if err := h.actaRepo.InsertActa(ctx, acta); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false, Message: "Error guardando acta en base de datos", Errors: []string{err.Error()},
		})
		return
	}

	h.registrarEvento(ctx, acta.ActaID, models.EventoActaGuardada,
		fmt.Sprintf("Acta guardada: estado=%s", acta.Estado),
		map[string]any{"estado": acta.Estado},
	)

	// 9. Respuesta
	message := fmt.Sprintf("Acta %s procesada exitosamente", acta.ActaID)
	if acta.Estado == models.EstadoError {
		message = fmt.Sprintf("Acta %s guardada con errores de validación", acta.ActaID)
	} else if acta.Estado == models.EstadoAnulada {
		message = fmt.Sprintf("Acta %s procesada — ANULADA: %s", acta.ActaID, acta.MotivoEstado)
	}

	c.JSON(http.StatusCreated, models.APIResponse{
		Success: true,
		Message: message,
		Data:    acta,
		Errors:  erroresValidacion,
	})
}

func (h *UploadHandler) registrarEvento(ctx context.Context, actaID, tipoEvento, descripcion string, metadata map[string]any) {
	evento := &models.EventoRRV{
		ActaID:      actaID,
		TipoEvento:  tipoEvento,
		Descripcion: descripcion,
		Metadata:    metadata,
		Fecha:       time.Now(),
	}
	_ = h.eventoRepo.InsertEvento(ctx, evento)
}
