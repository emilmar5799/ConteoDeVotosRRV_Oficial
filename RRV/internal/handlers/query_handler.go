package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"

	"rrv-backend/internal/models"
	"rrv-backend/internal/repository"
)

// QueryHandler maneja los endpoints GET de consulta del pipeline RRV.
type QueryHandler struct {
	actaRepo   *repository.ActaRepository
	eventoRepo *repository.EventoRepository
	smsRepo    *repository.SMSRepository
}

// NewQueryHandler crea un nuevo handler de consultas.
func NewQueryHandler(
	actaRepo *repository.ActaRepository,
	eventoRepo *repository.EventoRepository,
	smsRepo *repository.SMSRepository,
) *QueryHandler {
	return &QueryHandler{
		actaRepo:   actaRepo,
		eventoRepo: eventoRepo,
		smsRepo:    smsRepo,
	}
}

// HandleListActas maneja GET /api/rrv/actas
// Query params opcionales: departamento, estado, tipo_entrada
func (h *QueryHandler) HandleListActas(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	filters := map[string]string{
		"departamento": c.Query("departamento"),
		"estado":       c.Query("estado"),
		"tipo_entrada": c.Query("tipo_entrada"),
	}

	actas, err := h.actaRepo.FindAll(ctx, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Message: "Error listando actas",
			Errors:  []string{err.Error()},
		})
		return
	}

	if actas == nil {
		actas = []models.ActaRRV{}
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Actas obtenidas exitosamente",
		Data:    actas,
	})
}

// HandleGetActa maneja GET /api/rrv/actas/:acta_id
func (h *QueryHandler) HandleGetActa(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	actaID := c.Param("acta_id")
	if actaID == "" {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Message: "Se requiere acta_id",
		})
		return
	}

	acta, err := h.actaRepo.FindByActaID(ctx, actaID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Message: "Error buscando acta",
			Errors:  []string{err.Error()},
		})
		return
	}

	if acta == nil {
		c.JSON(http.StatusNotFound, models.APIResponse{
			Success: false,
			Message: "Acta no encontrada: " + actaID,
		})
		return
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Acta encontrada",
		Data:    acta,
	})
}

// HandleListEventos maneja GET /api/rrv/eventos
func (h *QueryHandler) HandleListEventos(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	eventos, err := h.eventoRepo.FindAll(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Message: "Error listando eventos",
			Errors:  []string{err.Error()},
		})
		return
	}

	if eventos == nil {
		eventos = []models.EventoRRV{}
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Eventos obtenidos exitosamente",
		Data:    eventos,
	})
}

// HandleListSMS maneja GET /api/rrv/sms
func (h *QueryHandler) HandleListSMS(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	registros, err := h.smsRepo.FindAll(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Message: "Error listando SMS",
			Errors:  []string{err.Error()},
		})
		return
	}

	if registros == nil {
		registros = []models.SMSRegistro{}
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Registros SMS obtenidos exitosamente",
		Data:    registros,
	})
}

// HandleStats maneja GET /api/rrv/stats
// Retorna estadísticas agregadas del pipeline RRV.
func (h *QueryHandler) HandleStats(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	stats := models.StatsResponse{
		PorDepartamento:   make(map[string]int64),
		PorTipoEntrada:    make(map[string]int64),
		VotosPorCandidato: make(map[string]int64),
	}

	// Total de actas
	total, err := h.actaRepo.CountByFilter(ctx, bson.M{})
	if err == nil {
		stats.TotalActas = total
	}

	// Actas procesadas
	procesadas, err := h.actaRepo.CountByFilter(ctx, bson.M{"estado": "PROCESADA"})
	if err == nil {
		stats.ActasProcesadas = procesadas
	}

	// Actas con error
	errCount, err := h.actaRepo.CountByFilter(ctx, bson.M{"estado": "ERROR"})
	if err == nil {
		stats.ActasError = errCount
	}

	// Por departamento
	porDep, err := h.actaRepo.AggregateByField(ctx, "departamento")
	if err == nil {
		stats.PorDepartamento = porDep
	}

	// Por tipo de entrada
	porTipo, err := h.actaRepo.AggregateByField(ctx, "tipo_entrada")
	if err == nil {
		stats.PorTipoEntrada = porTipo
	}

	// Votos totales
	validos, nulos, blancos, err := h.actaRepo.AggregateVotos(ctx)
	if err == nil {
		stats.TotalVotosValidos = validos
		stats.TotalVotosNulos = nulos
		stats.TotalVotosBlancos = blancos
	}

	// Votos por candidato
	porCandidato, err := h.actaRepo.AggregateVotosPorCandidato(ctx)
	if err == nil {
		stats.VotosPorCandidato = porCandidato
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Estadísticas RRV calculadas",
		Data:    stats,
	})
}
