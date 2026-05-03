package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"

	"rrv-backend/internal/models"
	"rrv-backend/internal/repository"
)

// QueryHandler maneja los endpoints GET de consulta del pipeline RRV.
// Implementa la parte de LECTURA del patrón CQRS.
type QueryHandler struct {
	actaRepo           *repository.ActaRepository
	eventoRepo         *repository.EventoRepository
	smsRepo            *repository.SMSRepository
	inconsistenciaRepo *repository.InconsistenciaRepository
	estadisticaRepo    *repository.EstadisticaRepository
	referenciaRepo     *repository.ReferenciaRepository
}

// NewQueryHandler crea un nuevo handler de consultas.
func NewQueryHandler(
	actaRepo *repository.ActaRepository,
	eventoRepo *repository.EventoRepository,
	smsRepo *repository.SMSRepository,
	inconsistenciaRepo *repository.InconsistenciaRepository,
	estadisticaRepo *repository.EstadisticaRepository,
	referenciaRepo *repository.ReferenciaRepository,
) *QueryHandler {
	return &QueryHandler{
		actaRepo:           actaRepo,
		eventoRepo:         eventoRepo,
		smsRepo:            smsRepo,
		inconsistenciaRepo: inconsistenciaRepo,
		estadisticaRepo:    estadisticaRepo,
		referenciaRepo:     referenciaRepo,
	}
}

// HandleListActas maneja GET /api/rrv/actas
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

// HandleStats maneja GET /api/rrv/stats (compatibilidad)
func (h *QueryHandler) HandleStats(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	stats := models.StatsResponse{
		PorDepartamento:   make(map[string]int64),
		PorTipoEntrada:    make(map[string]int64),
		VotosPorCandidato: make(map[string]int64),
	}

	total, err := h.actaRepo.CountByFilter(ctx, bson.M{})
	if err == nil {
		stats.TotalActas = total
	}
	procesadas, err := h.actaRepo.CountByFilter(ctx, bson.M{"estado": "PROCESADA"})
	if err == nil {
		stats.ActasProcesadas = procesadas
	}
	errCount, err := h.actaRepo.CountByFilter(ctx, bson.M{"estado": "ERROR"})
	if err == nil {
		stats.ActasError = errCount
	}
	porDep, err := h.actaRepo.AggregateByField(ctx, "departamento")
	if err == nil {
		stats.PorDepartamento = porDep
	}
	porTipo, err := h.actaRepo.AggregateByField(ctx, "tipo_entrada")
	if err == nil {
		stats.PorTipoEntrada = porTipo
	}
	validos, nulos, blancos, err := h.actaRepo.AggregateVotos(ctx)
	if err == nil {
		stats.TotalVotosValidos = validos
		stats.TotalVotosNulos = nulos
		stats.TotalVotosBlancos = blancos
	}
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

// ═══════════════════════════════════════════════════════════════════
// NUEVOS ENDPOINTS — CQRS, Inconsistencias, Event Replay, Referencia
// ═══════════════════════════════════════════════════════════════════

// HandleStatsFull maneja GET /api/rrv/stats/full
// Lee directamente de la vista materializada CQRS (lectura optimizada).
func (h *QueryHandler) HandleStatsFull(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	stats, err := h.estadisticaRepo.GetEstadistica(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Message: "Error obteniendo estadísticas CQRS",
			Errors:  []string{err.Error()},
		})
		return
	}

	if stats == nil {
		c.JSON(http.StatusOK, models.APIResponse{
			Success: true,
			Message: "Sin datos aún — la vista se genera tras la primera acta procesada",
			Data:    &models.StatsFullResponse{},
		})
		return
	}

	// Calcular pendientes
	totalProcesadas := stats.TotalActasProcesadas + stats.TotalActasError + stats.TotalActasAnuladas + stats.TotalActasObservadas

	response := models.StatsFullResponse{
		TotalActasEsperadas:    stats.TotalActasEsperadas,
		TotalActasProcesadas:   stats.TotalActasProcesadas,
		TotalActasError:        stats.TotalActasError,
		TotalActasAnuladas:     stats.TotalActasAnuladas,
		TotalActasPendientes:   stats.TotalActasEsperadas - totalProcesadas,
		TotalVotosValidos:      stats.TotalVotosValidos,
		TotalVotosNulos:        stats.TotalVotosNulos,
		TotalVotosBlancos:      stats.TotalVotosBlancos,
		VotosPorCandidato:      stats.VotosPorCandidato,
		PorDepartamento:        stats.PorDepartamento,
		PorTipoEntrada:         stats.PorTipoEntrada,
		PorcentajeAvance:       stats.PorcentajeAvance,
		TasaParticipacion:      stats.TasaParticipacion,
		MargenVictoria:         stats.MargenVictoria,
		PorcentajePublicadas:   stats.PorcentajeAvance, // misma métrica
		TotalInconsistencias:   stats.TotalInconsistencias,
		InconsistenciasPorTipo: stats.InconsistenciasPorTipo,
		UltimaActualizacion:    stats.UltimaActualizacion.Format(time.RFC3339),
	}

	// Total recintos
	totalRecintos, _ := h.referenciaRepo.CountTotalRecintos(ctx)
	response.TotalRecintos = totalRecintos

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Estadísticas completas RRV (vista CQRS)",
		Data:    response,
	})
}

// HandleInconsistencias maneja GET /api/rrv/inconsistencias
func (h *QueryHandler) HandleInconsistencias(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	logs, err := h.inconsistenciaRepo.FindAll(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Message: "Error listando inconsistencias",
			Errors:  []string{err.Error()},
		})
		return
	}

	if logs == nil {
		logs = []models.LogInconsistencia{}
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Inconsistencias obtenidas exitosamente",
		Data:    logs,
	})
}

// HandleEventReplay maneja GET /api/rrv/eventos/replay/:acta_id
// Implementa Event Sourcing: reconstruye el historial completo de un acta desde sus eventos.
func (h *QueryHandler) HandleEventReplay(c *gin.Context) {
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

	// Obtener todos los eventos del acta (orden cronológico)
	eventos, err := h.eventoRepo.FindByActaID(ctx, actaID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{
			Success: false,
			Message: "Error obteniendo eventos",
			Errors:  []string{err.Error()},
		})
		return
	}

	// Obtener inconsistencias del acta
	inconsistencias, _ := h.inconsistenciaRepo.FindByActaID(ctx, actaID)

	// Obtener el acta actual
	acta, _ := h.actaRepo.FindByActaID(ctx, actaID)

	replay := gin.H{
		"acta_id":          actaID,
		"total_eventos":    len(eventos),
		"eventos":          eventos,
		"inconsistencias":  inconsistencias,
		"estado_actual":    acta,
		"reconstruido_en":  time.Now().Format(time.RFC3339),
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Event Sourcing replay: historial completo del acta",
		Data:    replay,
	})
}

// HandleReferenciaTotales maneja GET /api/rrv/referencia/totales
// Retorna los totales de datos de referencia cargados del CSV.
func (h *QueryHandler) HandleReferenciaTotales(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	totalActas, _ := h.referenciaRepo.CountTotalActas(ctx)
	totalRecintos, _ := h.referenciaRepo.CountTotalRecintos(ctx)
	totalHabilitados, _ := h.referenciaRepo.SumTotalHabilitados(ctx)
	departamentos, _ := h.referenciaRepo.GetDepartamentos(ctx)

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Datos de referencia electoral",
		Data: gin.H{
			"total_actas_esperadas":      totalActas,
			"total_recintos":             totalRecintos,
			"total_votantes_habilitados": totalHabilitados,
			"departamentos":              departamentos,
			"total_departamentos":        len(departamentos),
		},
	})
}

// ═══════════════════════════════════════════════════════════════════
// BANCO DE CONSULTAS — Endpoints para el Dashboard Analítico
// ═══════════════════════════════════════════════════════════════════

// HandleMesasPorRecinto maneja GET /api/rrv/consultas/mesas-por-recinto (Query 1).
func (h *QueryHandler) HandleMesasPorRecinto(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	results, err := h.referenciaRepo.GetMesasPorRecinto(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Mesas por recinto y departamento",
		Data:    results,
	})
}

// HandleVotosPorMunicipio maneja GET /api/rrv/consultas/votos-por-municipio (Query 2).
func (h *QueryHandler) HandleVotosPorMunicipio(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	results, err := h.referenciaRepo.GetVotosPorMunicipio(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Registro de votos por municipio",
		Data:    results,
	})
}

// HandleVotosPorDepartamento maneja GET /api/rrv/consultas/votos-por-departamento (Query 3).
func (h *QueryHandler) HandleVotosPorDepartamento(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	results, err := h.referenciaRepo.GetVotosPorDepartamento(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Cantidad de votos por departamento",
		Data:    results,
	})
}

// HandleTopRecintos maneja GET /api/rrv/consultas/top-recintos?candidato=P1&limite=5 (Query 4).
func (h *QueryHandler) HandleTopRecintos(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	candidato := c.DefaultQuery("candidato", "P1")
	validos := map[string]bool{"P1": true, "P2": true, "P3": true, "P4": true}
	if !validos[candidato] {
		c.JSON(http.StatusBadRequest, models.APIResponse{
			Success: false,
			Message: "Candidato inválido. Use P1, P2, P3 o P4",
		})
		return
	}

	limite := 5
	if l := c.Query("limite"); l != "" {
		if n, e := parseInt(l); e == nil && n > 0 {
			limite = n
		}
	}

	results, err := h.referenciaRepo.GetTopRecintosPorCandidato(ctx, candidato, limite)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Top recintos con más votos para " + candidato,
		Data:    results,
	})
}

// HandleNulosPorDepartamento maneja GET /api/rrv/consultas/nulos-por-departamento (Query 5).
func (h *QueryHandler) HandleNulosPorDepartamento(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	results, err := h.referenciaRepo.GetNulosPorDepartamento(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Votos nulos por departamento con porcentaje vs válidos",
		Data:    results,
	})
}

// HandleBoletasAnuladas maneja GET /api/rrv/consultas/boletas-anuladas (Query 6).
// Retorna actas marcadas como ANULADA en el flujo RRV/TREP.
func (h *QueryHandler) HandleBoletasAnuladas(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	actas, err := h.actaRepo.FindAll(ctx, map[string]string{"estado": "ANULADA"})
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Message: err.Error()})
		return
	}
	if actas == nil {
		actas = []models.ActaRRV{}
	}

	type BoletaAnulada struct {
		IDBoleta string `json:"id_boleta"`
		Recinto  string `json:"recinto"`
		Mesa     string `json:"mesa"`
		Motivo   string `json:"motivo_anulacion"`
	}
	var boletas []BoletaAnulada
	for _, a := range actas {
		boletas = append(boletas, BoletaAnulada{
			IDBoleta: a.ActaID,
			Recinto:  a.Recinto,
			Mesa:     a.Mesa,
			Motivo:   a.MotivoEstado,
		})
	}
	if boletas == nil {
		boletas = []BoletaAnulada{}
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Boletas anuladas en el TREP",
		Data:    boletas,
	})
}

// HandleTREPvsOficial maneja GET /api/rrv/consultas/trep-vs-oficial (Queries 7 y 8).
// Compara totales del TREP (actas_rrv) vs Cómputo Oficial (Transcripciones).
func (h *QueryHandler) HandleTREPvsOficial(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()

	trepValidos, trepNulos, trepBlancos, _ := h.actaRepo.AggregateVotos(ctx)
	trepCandidatos, _ := h.actaRepo.AggregateVotosPorCandidato(ctx)
	oficial, _ := h.referenciaRepo.GetTotalesOficial(ctx)

	trepTotal := trepValidos + trepNulos + trepBlancos
	var oficialTotal int64
	if oficial != nil {
		oficialTotal = oficial["total_votos"]
	}

	comparacion := gin.H{
		"totales": gin.H{
			"trep":    trepTotal,
			"oficial": oficialTotal,
			"diferencia": trepTotal - oficialTotal,
		},
		"votos_validos": gin.H{
			"trep":       trepValidos,
			"oficial":    safeInt64(oficial, "votos_validos"),
			"diferencia": trepValidos - safeInt64(oficial, "votos_validos"),
		},
		"votos_nulos": gin.H{
			"trep":       trepNulos,
			"oficial":    safeInt64(oficial, "votos_nulos"),
			"diferencia": trepNulos - safeInt64(oficial, "votos_nulos"),
		},
		"votos_blancos": gin.H{
			"trep":       trepBlancos,
			"oficial":    safeInt64(oficial, "votos_blancos"),
			"diferencia": trepBlancos - safeInt64(oficial, "votos_blancos"),
		},
		"por_candidato": gin.H{
			"trep": trepCandidatos,
			"oficial": gin.H{
				"P1": safeInt64(oficial, "P1"),
				"P2": safeInt64(oficial, "P2"),
				"P3": safeInt64(oficial, "P3"),
				"P4": safeInt64(oficial, "P4"),
			},
		},
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Comparación TREP vs Cómputo Oficial",
		Data:    comparacion,
	})
}

// HandleMesasAbstencion maneja GET /api/rrv/consultas/mesas-abstencion?umbral=20 (Query 11).
func (h *QueryHandler) HandleMesasAbstencion(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()

	umbral := 20.0
	if u := c.Query("umbral"); u != "" {
		if f, e := parseFloat(u); e == nil && f > 0 {
			umbral = f
		}
	}

	results, err := h.referenciaRepo.GetMesasConAltaAbstencion(ctx, umbral)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Mesas con abstención superior al " + formatFloat(umbral) + "%",
		Data:    results,
	})
}

// HandleActasPorHora maneja GET /api/rrv/consultas/actas-por-hora (Query 12).
func (h *QueryHandler) HandleActasPorHora(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	results, err := h.actaRepo.GetActasPorHora(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Actas TREP recibidas en el centro de cómputo por hora",
		Data:    results,
	})
}

// HandleTiempoActas maneja GET /api/rrv/consultas/tiempo-actas-departamento (Query 14).
func (h *QueryHandler) HandleTiempoActas(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	results, err := h.actaRepo.GetTiempoActasPorDepartamento(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Tiempo entre primera y última acta por departamento",
		Data:    results,
	})
}

// HandleParticipacionPorDepartamento maneja GET /api/rrv/consultas/participacion-departamento (Query 16).
func (h *QueryHandler) HandleParticipacionPorDepartamento(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	results, err := h.referenciaRepo.GetParticipacionPorDepartamento(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Porcentaje de participación ciudadana por departamento",
		Data:    results,
	})
}

// HandleActasInconsistentes maneja GET /api/rrv/consultas/inconsistencias-trep-oficial (Query 17).
// Compara actas TREP procesadas vs registros del Oficial (Transcripciones) por departamento.
func (h *QueryHandler) HandleActasInconsistentes(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()

	trepPorDep, _ := h.actaRepo.AggregateByField(ctx, "departamento")
	trepVotosPorDep := make(map[string]gin.H)

	actas, _ := h.actaRepo.FindAll(ctx, map[string]string{})
	for _, a := range actas {
		dep := a.Departamento
		if _, ok := trepVotosPorDep[dep]; !ok {
			trepVotosPorDep[dep] = gin.H{
				"actas":         int64(0),
				"votos_validos": int64(0),
				"votos_nulos":   int64(0),
				"votos_blancos": int64(0),
			}
		}
		trepVotosPorDep[dep]["votos_validos"] = trepVotosPorDep[dep]["votos_validos"].(int64) + int64(a.VotosValidos)
		trepVotosPorDep[dep]["votos_nulos"] = trepVotosPorDep[dep]["votos_nulos"].(int64) + int64(a.VotosNulos)
		trepVotosPorDep[dep]["votos_blancos"] = trepVotosPorDep[dep]["votos_blancos"].(int64) + int64(a.VotosBlancos)
	}
	for dep, cnt := range trepPorDep {
		if h, ok := trepVotosPorDep[dep]; ok {
			h["actas"] = cnt
		}
	}

	oficialPorDep, _ := h.referenciaRepo.GetVotosPorDepartamento(ctx)
	type InconsistenciaDep struct {
		Departamento      string `json:"departamento"`
		TREPActas         int64  `json:"trep_actas"`
		TREPVotosValidos  int64  `json:"trep_votos_validos"`
		OficialVotos      int64  `json:"oficial_votos_validos"`
		DiferenciaVotos   int64  `json:"diferencia_votos"`
		TieneInconsistencia bool `json:"tiene_inconsistencia"`
	}

	oficialMap := make(map[string]int64)
	for _, o := range oficialPorDep {
		oficialMap[o.Departamento] = o.TotalVotos
	}

	var resultado []InconsistenciaDep
	for dep, trep := range trepVotosPorDep {
		trepValidos := trep["votos_validos"].(int64)
		oficialVotos := oficialMap[dep]
		diff := trepValidos - oficialVotos
		resultado = append(resultado, InconsistenciaDep{
			Departamento:      dep,
			TREPActas:         trep["actas"].(int64),
			TREPVotosValidos:  trepValidos,
			OficialVotos:      oficialVotos,
			DiferenciaVotos:   diff,
			TieneInconsistencia: diff != 0,
		})
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Actas inconsistentes entre TREP y Oficial por departamento",
		Data:    resultado,
	})
}

// HandleResultadosGeograficos maneja GET /api/rrv/consultas/resultados-geograficos (Query 19).
// Parámetros opcionales: ?departamento=X&municipio=Y&provincia=Z
func (h *QueryHandler) HandleResultadosGeograficos(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	departamento := c.Query("departamento")
	municipio := c.Query("municipio")
	provincia := c.Query("provincia")

	results, err := h.referenciaRepo.GetResultadosPorGeografia(ctx, departamento, municipio, provincia)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Resultados electorales por región",
		Data: gin.H{
			"filtros": gin.H{
				"departamento": departamento,
				"municipio":    municipio,
				"provincia":    provincia,
			},
			"candidatos": gin.H{
				"P1": "Daenerys Targaryen",
				"P2": "Sansa Stark",
				"P3": "Robert Baratheon",
				"P4": "Tyrion Lannister",
			},
			"resultados": results,
		},
	})
}

// HandleErroresComunes maneja GET /api/rrv/consultas/errores-comunes (Query 20).
func (h *QueryHandler) HandleErroresComunes(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	conteos, err := h.inconsistenciaRepo.CountByTipo(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.APIResponse{Success: false, Message: err.Error()})
		return
	}

	type ErrorFrecuencia struct {
		TipoError  string `json:"tipo_error"`
		Frecuencia int64  `json:"frecuencia"`
	}
	var errores []ErrorFrecuencia
	for tipo, freq := range conteos {
		errores = append(errores, ErrorFrecuencia{TipoError: tipo, Frecuencia: freq})
	}
	// Ordenar por frecuencia descendente
	for i := 0; i < len(errores)-1; i++ {
		for j := i + 1; j < len(errores); j++ {
			if errores[j].Frecuencia > errores[i].Frecuencia {
				errores[i], errores[j] = errores[j], errores[i]
			}
		}
	}

	c.JSON(http.StatusOK, models.APIResponse{
		Success: true,
		Message: "Errores más comunes en verificación de actas",
		Data:    errores,
	})
}

// ─── helpers internos ───────────────────────────────────────────────

func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

func parseFloat(s string) (float64, error) {
	var f float64
	_, err := fmt.Sscanf(s, "%f", &f)
	return f, err
}

func formatFloat(f float64) string {
	return fmt.Sprintf("%.1f", f)
}

func safeInt64(m map[string]int64, key string) int64 {
	if m == nil {
		return 0
	}
	return m[key]
}
