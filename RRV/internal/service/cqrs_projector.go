package service

import (
	"context"
	"log"
	"math"
	"time"

	"go.mongodb.org/mongo-driver/bson"

	"rrv-backend/internal/models"
	"rrv-backend/internal/repository"
)

// CQRSProjector mantiene la vista materializada Estadisticas_RRV.
// Implementa el patrón CQRS: las escrituras van a actas_rrv,
// y este proyector actualiza la vista de lectura optimizada.
type CQRSProjector struct {
	actaRepo           *repository.ActaRepository
	estadisticaRepo    *repository.EstadisticaRepository
	referenciaRepo     *repository.ReferenciaRepository
	inconsistenciaRepo *repository.InconsistenciaRepository
}

// NewCQRSProjector crea un nuevo proyector CQRS.
func NewCQRSProjector(
	actaRepo *repository.ActaRepository,
	estadisticaRepo *repository.EstadisticaRepository,
	referenciaRepo *repository.ReferenciaRepository,
	inconsistenciaRepo *repository.InconsistenciaRepository,
) *CQRSProjector {
	return &CQRSProjector{
		actaRepo:           actaRepo,
		estadisticaRepo:    estadisticaRepo,
		referenciaRepo:     referenciaRepo,
		inconsistenciaRepo: inconsistenciaRepo,
	}
}

// Proyectar recalcula la vista materializada completa.
// Se ejecuta de forma asíncrona después de cada acta guardada.
func (p *CQRSProjector) Proyectar(ctx context.Context) {
	go func() {
		projCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := p.recalcular(projCtx); err != nil {
			log.Printf("[CQRS] Error proyectando estadísticas: %v", err)
		} else {
			log.Println("[CQRS] Vista materializada actualizada ✓")
		}
	}()
}

// recalcular ejecuta la recalculación completa de la vista materializada.
func (p *CQRSProjector) recalcular(ctx context.Context) error {
	stats := &models.EstadisticaRRV{
		VotosPorCandidato:      make(map[string]int64),
		PorDepartamento:        make(map[string]int64),
		PorTipoEntrada:         make(map[string]int64),
		InconsistenciasPorTipo: make(map[string]int64),
	}

	// Total de actas esperadas (del CSV)
	totalEsperadas, err := p.referenciaRepo.CountTotalActas(ctx)
	if err == nil {
		stats.TotalActasEsperadas = totalEsperadas
	}

	// Total habilitados (del CSV)
	totalHab, err := p.referenciaRepo.SumTotalHabilitados(ctx)
	if err == nil {
		stats.TotalHabilitados = totalHab
	}

	// Actas procesadas por estado
	procesadas, _ := p.actaRepo.CountByFilter(ctx, bson.M{"estado": "PROCESADA"})
	errCount, _ := p.actaRepo.CountByFilter(ctx, bson.M{"estado": "ERROR"})
	anuladas, _ := p.actaRepo.CountByFilter(ctx, bson.M{"estado": "ANULADA"})
	observadas, _ := p.actaRepo.CountByFilter(ctx, bson.M{"estado": "OBSERVADA"})

	stats.TotalActasProcesadas = procesadas
	stats.TotalActasError = errCount
	stats.TotalActasAnuladas = anuladas
	stats.TotalActasObservadas = observadas

	// Votos agregados
	validos, nulos, blancos, err := p.actaRepo.AggregateVotos(ctx)
	if err == nil {
		stats.TotalVotosValidos = validos
		stats.TotalVotosNulos = nulos
		stats.TotalVotosBlancos = blancos
		stats.TotalVotantes = validos + nulos + blancos
	}

	// Votos por candidato
	porCandidato, err := p.actaRepo.AggregateVotosPorCandidato(ctx)
	if err == nil {
		stats.VotosPorCandidato = porCandidato
	}

	// Por departamento
	porDep, err := p.actaRepo.AggregateByField(ctx, "departamento")
	if err == nil {
		stats.PorDepartamento = porDep
	}

	// Por tipo de entrada
	porTipo, err := p.actaRepo.AggregateByField(ctx, "tipo_entrada")
	if err == nil {
		stats.PorTipoEntrada = porTipo
	}

	// KPIs
	totalProcesadasGeneral := procesadas + errCount + anuladas + observadas
	if totalEsperadas > 0 {
		stats.PorcentajeAvance = math.Round(float64(totalProcesadasGeneral)/float64(totalEsperadas)*10000) / 100
	}
	if totalHab > 0 {
		stats.TasaParticipacion = math.Round(float64(stats.TotalVotantes)/float64(totalHab)*10000) / 100
	}

	// Margen de victoria (diferencia % entre 1° y 2° candidato)
	stats.MargenVictoria = calcularMargenVictoria(porCandidato, stats.TotalVotosValidos)

	// Inconsistencias
	totalInc, _ := p.inconsistenciaRepo.CountTotal(ctx)
	stats.TotalInconsistencias = totalInc

	incPorTipo, _ := p.inconsistenciaRepo.CountByTipo(ctx)
	if incPorTipo != nil {
		stats.InconsistenciasPorTipo = incPorTipo
	}

	// Guardar vista materializada
	return p.estadisticaRepo.UpsertEstadistica(ctx, stats)
}

// calcularMargenVictoria calcula la diferencia porcentual entre el 1° y 2° candidato.
func calcularMargenVictoria(votosCandidato map[string]int64, totalValidos int64) float64 {
	if totalValidos == 0 || len(votosCandidato) < 2 {
		return 0
	}

	var primero, segundo int64
	for _, votos := range votosCandidato {
		if votos > primero {
			segundo = primero
			primero = votos
		} else if votos > segundo {
			segundo = votos
		}
	}

	margen := float64(primero-segundo) / float64(totalValidos) * 100
	return math.Round(margen*100) / 100
}
