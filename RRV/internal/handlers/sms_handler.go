package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"rrv-backend/internal/models"
	"rrv-backend/internal/repository"
	"rrv-backend/internal/service"
)

// SMSHandler maneja la recepción y procesamiento de mensajes SMS.
type SMSHandler struct {
	actaRepo   *repository.ActaRepository
	eventoRepo *repository.EventoRepository
	smsRepo    *repository.SMSRepository
	smsService *service.SMSService
	actaService *service.ActaService
}

// NewSMSHandler crea un nuevo handler de SMS.
func NewSMSHandler(
	actaRepo *repository.ActaRepository,
	eventoRepo *repository.EventoRepository,
	smsRepo *repository.SMSRepository,
	smsService *service.SMSService,
	actaService *service.ActaService,
) *SMSHandler {
	return &SMSHandler{
		actaRepo:   actaRepo,
		eventoRepo: eventoRepo,
		smsRepo:    smsRepo,
		smsService: smsService,
		actaService: actaService,
	}
}

// HandleSMS procesa POST /api/rrv/sms
// Flujo: Recibir JSON → Validar teléfono → Validar PIN → Verificar duplicado → Parsear → Validar acta → Guardar
func (h *SMSHandler) HandleSMS(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	// 1. Parsear request JSON
	var req models.SMSRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Message: "JSON inválido. Se requieren campos 'telefono' y 'mensaje'",
			Errors:  []string{err.Error()},
		})
		return
	}

	// 2. Calcular hash del mensaje para idempotencia
	hashMensaje := h.smsService.CalcularHashSMS(req.Mensaje)

	// 3. Registrar evento: SMS recibido
	h.registrarEvento(ctx, "PENDIENTE", models.EventoSMSRecibido,
		fmt.Sprintf("SMS recibido de %s", req.Telefono),
		map[string]any{"telefono": req.Telefono, "hash": hashMensaje},
	)

	// 4. Validar teléfono autorizado
	if !h.smsService.ValidarTelefono(req.Telefono) {
		h.registrarEvento(ctx, "RECHAZADO", models.EventoSMSTelNoAutorizado,
			fmt.Sprintf("Teléfono no autorizado: %s", req.Telefono),
			map[string]any{"telefono": req.Telefono},
		)

		// Guardar SMS rechazado para auditoría
		smsReg := &models.SMSRegistro{
			Telefono:       req.Telefono,
			MensajeRaw:     req.Mensaje,
			HashMensaje:    hashMensaje,
			ActaID:         "",
			Estado:         models.SMSRechazado,
			Errores:        []string{"Teléfono no autorizado"},
			FechaRecepcion: time.Now(),
		}
		h.smsRepo.InsertSMS(ctx, smsReg)

		c.JSON(http.StatusForbidden, models.APIResponse{
			Success: false,
			Message: "Teléfono no autorizado para enviar resultados",
			Errors:  []string{fmt.Sprintf("El número %s no está registrado como notario electoral", req.Telefono)},
		})
		return
	}

	// 5. Verificar duplicado por hash de mensaje
	smsDuplicado, _ := h.smsRepo.FindByHash(ctx, hashMensaje)
	if smsDuplicado != nil {
		h.registrarEvento(ctx, smsDuplicado.ActaID, models.EventoSMSDuplicado,
			fmt.Sprintf("SMS duplicado detectado: hash=%s", hashMensaje),
			map[string]any{"hash": hashMensaje},
		)

		c.JSON(http.StatusConflict, models.APIResponse{
			Success: false,
			Message: "SMS duplicado: este mensaje ya fue procesado",
			Data:    smsDuplicado,
		})
		return
	}

	// 6. Parsear el contenido del SMS
	parsed, erroresParseo := h.smsService.ParseSMS(req.Mensaje)

	if len(erroresParseo) > 0 {
		// Guardar SMS con errores de parseo
		smsReg := &models.SMSRegistro{
			Telefono:       req.Telefono,
			MensajeRaw:     req.Mensaje,
			HashMensaje:    hashMensaje,
			ActaID:         "",
			Estado:         models.SMSError,
			Errores:        erroresParseo,
			FechaRecepcion: time.Now(),
		}
		h.smsRepo.InsertSMS(ctx, smsReg)

		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Message: "Error parseando mensaje SMS",
			Errors:  erroresParseo,
		})
		return
	}

	// 7. Validar PIN
	if !h.smsService.ValidarPIN(parsed.PIN) {
		h.registrarEvento(ctx, parsed.ActaID, models.EventoSMSPinInvalido,
			fmt.Sprintf("PIN inválido en SMS de %s para acta %s", req.Telefono, parsed.ActaID),
			map[string]any{"telefono": req.Telefono, "acta_id": parsed.ActaID},
		)

		smsReg := &models.SMSRegistro{
			Telefono:       req.Telefono,
			MensajeRaw:     req.Mensaje,
			HashMensaje:    hashMensaje,
			ActaID:         parsed.ActaID,
			Estado:         models.SMSRechazado,
			Errores:        []string{"PIN de verificación inválido"},
			FechaRecepcion: time.Now(),
		}
		h.smsRepo.InsertSMS(ctx, smsReg)

		c.JSON(http.StatusUnauthorized, models.APIResponse{
			Success: false,
			Message: "PIN de verificación inválido",
			Errors:  []string{"El PIN proporcionado no coincide con el PIN autorizado"},
		})
		return
	}

	// 8. Convertir a ActaRRV y validar
	acta := h.smsService.ConvertirAActa(parsed, hashMensaje)
	acta.FechaRecepcion = time.Now()

	erroresValidacion := h.actaService.ValidarActa(acta)
	acta.Estado = h.actaService.DeterminarEstado(acta, erroresValidacion)
	acta.Errores = erroresValidacion

	// 9. Verificar duplicado por acta_id
	existente, _ := h.actaRepo.FindByActaID(ctx, acta.ActaID)
	if existente != nil {
		h.registrarEvento(ctx, acta.ActaID, models.EventoDuplicadoDetectado,
			fmt.Sprintf("Acta duplicada por ID vía SMS: acta_id=%s", acta.ActaID),
			nil,
		)

		smsReg := &models.SMSRegistro{
			Telefono:       req.Telefono,
			MensajeRaw:     req.Mensaje,
			HashMensaje:    hashMensaje,
			ActaID:         acta.ActaID,
			Estado:         models.SMSDuplicado,
			Errores:        []string{fmt.Sprintf("Acta %s ya existe en el sistema", acta.ActaID)},
			FechaRecepcion: time.Now(),
		}
		h.smsRepo.InsertSMS(ctx, smsReg)

		c.JSON(http.StatusConflict, models.APIResponse{
			Success: false,
			Message: fmt.Sprintf("Acta %s ya existe en el sistema", acta.ActaID),
			Data:    existente,
		})
		return
	}

	// 10. Guardar acta en MongoDB
	if err := h.actaRepo.InsertActa(ctx, acta); err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Message: "Error guardando acta",
			Errors:  []string{err.Error()},
		})
		return
	}

	// 11. Guardar registro SMS procesado
	smsReg := &models.SMSRegistro{
		Telefono:       req.Telefono,
		MensajeRaw:     req.Mensaje,
		HashMensaje:    hashMensaje,
		ActaID:         acta.ActaID,
		Estado:         models.SMSProcesado,
		FechaRecepcion: time.Now(),
	}
	h.smsRepo.InsertSMS(ctx, smsReg)

	h.registrarEvento(ctx, acta.ActaID, models.EventoActaGuardada,
		fmt.Sprintf("Acta guardada vía SMS: estado=%s, candidatos=%d", acta.Estado, len(acta.Candidatos)),
		map[string]any{"estado": acta.Estado, "candidatos": len(acta.Candidatos), "telefono": req.Telefono},
	)

	// 12. Respuesta exitosa
	message := fmt.Sprintf("SMS procesado exitosamente: acta %s guardada", acta.ActaID)
	if acta.Estado == models.EstadoError {
		message = fmt.Sprintf("SMS procesado con errores de validación: acta %s guardada", acta.ActaID)
	}

	c.JSON(http.StatusCreated, models.APIResponse{
		Success: true,
		Message: message,
		Data:    acta,
		Errors:  erroresValidacion,
	})
}

// registrarEvento es un helper para registrar eventos del pipeline.
func (h *SMSHandler) registrarEvento(ctx context.Context, actaID, tipoEvento, descripcion string, metadata map[string]any) {
	evento := &models.EventoRRV{
		ActaID:      actaID,
		TipoEvento:  tipoEvento,
		Descripcion: descripcion,
		Metadata:    metadata,
		Fecha:       time.Now(),
	}
	_ = h.eventoRepo.InsertEvento(ctx, evento)
}
