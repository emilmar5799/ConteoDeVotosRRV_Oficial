package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"rrv-backend/internal/models"
	"rrv-backend/internal/repository"
	"rrv-backend/internal/service"
)

// SMSHandler maneja la recepción y procesamiento de mensajes SMS.
type SMSHandler struct {
	actaRepo              *repository.ActaRepository
	eventoRepo            *repository.EventoRepository
	smsRepo               *repository.SMSRepository
	smsService            *service.SMSService
	actaService           *service.ActaService
	twilioValidator       *service.TwilioValidator
	twilioService         *service.TwilioService
	inconsistenciaService *service.InconsistenciaService
	cqrsProjector         *service.CQRSProjector
	retryCfg              service.RetryConfig
}

// NewSMSHandler crea un nuevo handler de SMS.
func NewSMSHandler(
	actaRepo *repository.ActaRepository,
	eventoRepo *repository.EventoRepository,
	smsRepo *repository.SMSRepository,
	smsService *service.SMSService,
	actaService *service.ActaService,
	twilioValidator *service.TwilioValidator,
	twilioService *service.TwilioService,
	inconsistenciaService *service.InconsistenciaService,
	cqrsProjector *service.CQRSProjector,
) *SMSHandler {
	return &SMSHandler{
		actaRepo:              actaRepo,
		eventoRepo:            eventoRepo,
		smsRepo:               smsRepo,
		smsService:            smsService,
		actaService:           actaService,
		twilioValidator:       twilioValidator,
		twilioService:         twilioService,
		inconsistenciaService: inconsistenciaService,
		cqrsProjector:         cqrsProjector,
		retryCfg:              service.DefaultRetryConfig(),
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

	// Procesar el SMS usando la lógica compartida
	h.procesarSMS(ctx, c, req.Telefono, req.Mensaje, false)
}

// HandleTwilioWebhook procesa POST /api/rrv/sms/twilio
// Este endpoint recibe webhooks reales de Twilio cuando un notario envía un SMS.
func (h *SMSHandler) HandleTwilioWebhook(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	// 1. Parsear form data de Twilio
	if err := c.Request.ParseForm(); err != nil {
		log.Printf("[TWILIO-WEBHOOK] Error parseando form: %v", err)
		c.Data(http.StatusBadRequest, "application/xml",
			[]byte(service.FormatearTwiML("Error procesando solicitud")))
		return
	}

	from := c.Request.FormValue("From")
	body := c.Request.FormValue("Body")
	messageSid := c.Request.FormValue("MessageSid")
	accountSid := c.Request.FormValue("AccountSid")

	log.Printf("[TWILIO-WEBHOOK] SMS recibido: From=%s, MessageSid=%s, Body=%s",
		from, messageSid, truncarLog(body, 80))

	// 2. Validar firma de Twilio (seguridad anti-suplantación)
	firma := c.GetHeader("X-Twilio-Signature")
	params := service.ExtraerParamsForm(c.Request.PostForm)

	if !h.twilioValidator.ValidarFirma(firma, params) {
		log.Printf("[TWILIO-WEBHOOK] ⚠️ FIRMA INVÁLIDA — posible ataque de suplantación desde %s", c.ClientIP())

		h.registrarEvento(ctx, "", "TWILIO_FIRMA_INVALIDA",
			fmt.Sprintf("Webhook con firma Twilio inválida desde IP %s", c.ClientIP()),
			map[string]any{"ip": c.ClientIP(), "from": from},
		)

		c.Data(http.StatusForbidden, "application/xml",
			[]byte(service.FormatearTwiML("Solicitud no autorizada")))
		return
	}

	// 3. Verificar que el AccountSid coincida (capa extra de seguridad)
	if h.twilioValidator != nil && accountSid != "" {
		log.Printf("[TWILIO-WEBHOOK] AccountSid verificado: %s", accountSid[:8]+"...")
	}

	// 4. Validar campos mínimos
	if from == "" || body == "" {
		log.Println("[TWILIO-WEBHOOK] Campos From o Body vacíos")
		c.Data(http.StatusBadRequest, "application/xml",
			[]byte(service.FormatearTwiML("Mensaje vacío")))
		return
	}

	// 5. Procesar el SMS usando la lógica compartida
	h.procesarSMS(ctx, c, from, body, true)
}

// procesarSMS contiene la lógica de negocio compartida entre HandleSMS y HandleTwilioWebhook.
func (h *SMSHandler) procesarSMS(ctx context.Context, c *gin.Context, telefono, mensaje string, esTwilio bool) {

	// 1. Calcular hash del mensaje para idempotencia
	hashMensaje := h.smsService.CalcularHashSMS(mensaje)

	// 2. Registrar evento: SMS recibido
	fuenteSMS := "API_JSON"
	if esTwilio {
		fuenteSMS = "TWILIO_WEBHOOK"
	}
	h.registrarEvento(ctx, "PENDIENTE", models.EventoSMSRecibido,
		fmt.Sprintf("SMS recibido de %s vía %s", telefono, fuenteSMS),
		map[string]any{"telefono": telefono, "hash": hashMensaje, "fuente": fuenteSMS},
	)

	// 3. Validar teléfono autorizado
	if !h.smsService.ValidarTelefono(telefono) {
		h.registrarEvento(ctx, "RECHAZADO", models.EventoSMSTelNoAutorizado,
			fmt.Sprintf("Teléfono no autorizado: %s", telefono),
			map[string]any{"telefono": telefono},
		)

		// Guardar SMS rechazado para auditoría
		smsReg := &models.SMSRegistro{
			Telefono:       telefono,
			MensajeRaw:     mensaje,
			HashMensaje:    hashMensaje,
			ActaID:         "",
			Estado:         models.SMSRechazado,
			Errores:        []string{"Teléfono no autorizado"},
			FechaRecepcion: time.Now(),
		}
		h.smsRepo.InsertSMS(ctx, smsReg)

		if esTwilio {
			c.Data(http.StatusOK, "application/xml",
				[]byte(service.FormatearTwiML("❌ Número no autorizado para enviar resultados electorales.")))
		} else {
			c.JSON(http.StatusForbidden, models.APIResponse{
				Success: false,
				Message: "Teléfono no autorizado para enviar resultados",
				Errors:  []string{fmt.Sprintf("El número %s no está registrado como notario electoral", telefono)},
			})
		}
		return
	}

	// 4. Verificar duplicado por hash de mensaje
	smsDuplicado, _ := h.smsRepo.FindByHash(ctx, hashMensaje)
	if smsDuplicado != nil {
		h.registrarEvento(ctx, smsDuplicado.ActaID, models.EventoSMSDuplicado,
			fmt.Sprintf("SMS duplicado detectado: hash=%s", hashMensaje),
			map[string]any{"hash": hashMensaje},
		)

		if esTwilio {
			c.Data(http.StatusOK, "application/xml",
				[]byte(service.FormatearTwiML("⚠️ Este mensaje ya fue procesado anteriormente.")))
		} else {
			c.JSON(http.StatusConflict, models.APIResponse{
				Success: false,
				Message: "SMS duplicado: este mensaje ya fue procesado",
				Data:    smsDuplicado,
			})
		}
		return
	}

	// 5. Parsear el contenido del SMS
	parsed, erroresParseo := h.smsService.ParseSMS(mensaje)

	if len(erroresParseo) > 0 {
		smsReg := &models.SMSRegistro{
			Telefono:       telefono,
			MensajeRaw:     mensaje,
			HashMensaje:    hashMensaje,
			ActaID:         "",
			Estado:         models.SMSError,
			Errores:        erroresParseo,
			FechaRecepcion: time.Now(),
		}
		h.smsRepo.InsertSMS(ctx, smsReg)

		if h.twilioService != nil && h.twilioService.IsConfigured() {
			h.twilioService.EnviarErrorFormato(telefono, erroresParseo)
		}

		if esTwilio {
			c.Data(http.StatusOK, "application/xml",
				[]byte(service.FormatearTwiML(
					fmt.Sprintf("❌ Error en formato. Formato correcto: ACTA:MESA-ID|DEP:Dept|MUN:Mun|MESA:N|VAL:N|NUL:N|BLA:N|C1:N|C2:N|PIN:1234. Errores: %s",
						joinFirst(erroresParseo, 2)))))
		} else {
			c.JSON(http.StatusBadRequest, models.APIResponse{
				Success: false,
				Message: "Error parseando mensaje SMS",
				Errors:  erroresParseo,
			})
		}
		return
	}

	// 6. Validar PIN
	if !h.smsService.ValidarPIN(parsed.PIN) {
		h.registrarEvento(ctx, parsed.ActaID, models.EventoSMSPinInvalido,
			fmt.Sprintf("PIN inválido en SMS de %s para acta %s", telefono, parsed.ActaID),
			map[string]any{"telefono": telefono, "acta_id": parsed.ActaID},
		)

		smsReg := &models.SMSRegistro{
			Telefono:       telefono,
			MensajeRaw:     mensaje,
			HashMensaje:    hashMensaje,
			ActaID:         parsed.ActaID,
			Estado:         models.SMSRechazado,
			Errores:        []string{"PIN de verificación inválido"},
			FechaRecepcion: time.Now(),
		}
		h.smsRepo.InsertSMS(ctx, smsReg)

		if esTwilio {
			c.Data(http.StatusOK, "application/xml",
				[]byte(service.FormatearTwiML("❌ PIN de verificación inválido. Verifique su PIN autorizado.")))
		} else {
			c.JSON(http.StatusUnauthorized, models.APIResponse{
				Success: false,
				Message: "PIN de verificación inválido",
				Errors:  []string{"El PIN proporcionado no coincide con el PIN autorizado"},
			})
		}
		return
	}

	// 7. Convertir a ActaRRV y validar
	acta := h.smsService.ConvertirAActa(parsed, hashMensaje)
	acta.FechaRecepcion = time.Now()

	erroresValidacion := h.actaService.ValidarActa(acta)
	acta.Estado = h.actaService.DeterminarEstado(acta, erroresValidacion)
	acta.Errores = erroresValidacion

	// 8. Detectar inconsistencias contra datos de referencia
	inconsistencias := h.inconsistenciaService.DetectarYRegistrar(ctx, acta, "SMS")
	if len(inconsistencias) > 0 {
		h.registrarEvento(ctx, acta.ActaID, "INCONSISTENCIAS_DETECTADAS",
			fmt.Sprintf("%d inconsistencias detectadas vía SMS", len(inconsistencias)),
			map[string]any{"cantidad": len(inconsistencias)},
		)
	}

	// 9. Verificar duplicado por acta_id
	existente, _ := h.actaRepo.FindByActaID(ctx, acta.ActaID)
	if existente != nil {
		h.registrarEvento(ctx, acta.ActaID, models.EventoDuplicadoDetectado,
			fmt.Sprintf("Acta duplicada por ID vía SMS: acta_id=%s", acta.ActaID),
			nil,
		)

		smsReg := &models.SMSRegistro{
			Telefono:       telefono,
			MensajeRaw:     mensaje,
			HashMensaje:    hashMensaje,
			ActaID:         acta.ActaID,
			Estado:         models.SMSDuplicado,
			Errores:        []string{fmt.Sprintf("Acta %s ya existe en el sistema", acta.ActaID)},
			FechaRecepcion: time.Now(),
		}
		h.smsRepo.InsertSMS(ctx, smsReg)

		if esTwilio {
			c.Data(http.StatusOK, "application/xml",
				[]byte(service.FormatearTwiML(
					fmt.Sprintf("⚠️ Acta %s ya fue registrada anteriormente.", acta.ActaID))))
		} else {
			c.JSON(http.StatusConflict, models.APIResponse{
				Success: false,
				Message: fmt.Sprintf("Acta %s ya existe en el sistema", acta.ActaID),
				Data:    existente,
			})
		}
		return
	}

	// 10. Guardar acta en MongoDB (con retry)
	err := service.WithRetry(ctx, h.retryCfg, "InsertActa-SMS", func() error {
		return h.actaRepo.InsertActa(ctx, acta)
	})
	if err != nil {
		if esTwilio {
			c.Data(http.StatusOK, "application/xml",
				[]byte(service.FormatearTwiML("❌ Error interno guardando el acta. Reintente en unos minutos.")))
		} else {
			c.JSON(http.StatusInternalServerError, models.APIResponse{
				Success: false,
				Message: "Error guardando acta",
				Errors:  []string{err.Error()},
			})
		}
		return
	}

	// 11. Guardar registro SMS procesado
	smsReg := &models.SMSRegistro{
		Telefono:       telefono,
		MensajeRaw:     mensaje,
		HashMensaje:    hashMensaje,
		ActaID:         acta.ActaID,
		Estado:         models.SMSProcesado,
		FechaRecepcion: time.Now(),
	}
	h.smsRepo.InsertSMS(ctx, smsReg)

	h.registrarEvento(ctx, acta.ActaID, models.EventoActaGuardada,
		fmt.Sprintf("Acta guardada vía SMS: estado=%s, candidatos=%d", acta.Estado, len(acta.Candidatos)),
		map[string]any{"estado": acta.Estado, "candidatos": len(acta.Candidatos), "telefono": telefono, "fuente": fuente(esTwilio)},
	)

	// 12. Actualizar vista materializada CQRS (asíncrono)
	h.cqrsProjector.Proyectar(ctx)

	// 13. Enviar confirmación por Twilio (asíncrono)
	if h.twilioService != nil && h.twilioService.IsConfigured() && esTwilio {
		h.twilioService.EnviarConfirmacion(telefono, acta.ActaID, acta.Estado)
	}

	// 14. Respuesta
	if esTwilio {
		var twimlMsg string
		switch acta.Estado {
		case models.EstadoProcesada:
			twimlMsg = fmt.Sprintf("✅ Acta %s recibida y procesada correctamente. Gracias.", acta.ActaID)
		case models.EstadoError:
			twimlMsg = fmt.Sprintf("⚠️ Acta %s recibida con observaciones de validación.", acta.ActaID)
		case models.EstadoAnulada:
			twimlMsg = fmt.Sprintf("❌ Acta %s marcada como ANULADA: %s", acta.ActaID, acta.MotivoEstado)
		default:
			twimlMsg = fmt.Sprintf("📋 Acta %s recibida. Estado: %s", acta.ActaID, acta.Estado)
		}
		c.Data(http.StatusOK, "application/xml", []byte(service.FormatearTwiML(twimlMsg)))
	} else {
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
}

// HandleEnviarSMS procesa POST /api/rrv/sms/enviar
func (h *SMSHandler) HandleEnviarSMS(c *gin.Context) {
	if h.twilioService == nil || !h.twilioService.IsConfigured() {
		c.JSON(http.StatusServiceUnavailable, models.APIResponse{
			Success: false,
			Message: "Servicio Twilio no configurado. Configure las variables TWILIO_ACCOUNT_SID, TWILIO_AUTH_TOKEN y TWILIO_PHONE_NUMBER",
		})
		return
	}

	var req struct {
		Destinatario string `json:"destinatario" binding:"required"`
		Mensaje      string `json:"mensaje" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Message: "Se requieren campos 'destinatario' y 'mensaje'",
			Errors:  []string{err.Error()},
		})
		return
	}

	result, err := h.twilioService.EnviarSMS(req.Destinatario, req.Mensaje)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Message: "Error enviando SMS",
			Errors:  []string{err.Error()},
			Data:    result,
		})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "SMS enviado exitosamente",
		Data:    result,
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

// truncarLog corta un string para logging.
func truncarLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// joinFirst une los primeros N strings con "; "
func joinFirst(items []string, n int) string {
	if len(items) <= n {
		result := ""
		for i, item := range items {
			if i > 0 {
				result += "; "
			}
			result += item
		}
		return result
	}
	result := ""
	for i := 0; i < n; i++ {
		if i > 0 {
			result += "; "
		}
		result += items[i]
	}
	return fmt.Sprintf("%s (y %d más)", result, len(items)-n)
}

// fuente devuelve el tipo de fuente según si es Twilio o JSON.
func fuente(esTwilio bool) string {
	if esTwilio {
		return "TWILIO_WEBHOOK"
	}
	return "API_JSON"
}
