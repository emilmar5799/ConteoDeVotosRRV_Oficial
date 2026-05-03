package repository

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"rrv-backend/internal/models"
)

// EstadisticaRepository maneja la vista materializada CQRS en Estadisticas_RRV.
// Esta colección contiene UN solo documento con las estadísticas precalculadas.
type EstadisticaRepository struct {
	collection *mongo.Collection
}

// NewEstadisticaRepository crea el repositorio de estadísticas CQRS.
func NewEstadisticaRepository(db *mongo.Database) *EstadisticaRepository {
	coll := db.Collection("Estadisticas_RRV")
	return &EstadisticaRepository{collection: coll}
}

// UpsertEstadistica actualiza la vista materializada (upsert — crea si no existe).
func (r *EstadisticaRepository) UpsertEstadistica(ctx context.Context, stats *models.EstadisticaRRV) error {
	stats.UltimaActualizacion = time.Now()

	filter := bson.M{"_id": "stats_rrv_global"}
	update := bson.M{"$set": bson.M{
		"total_actas_esperadas":    stats.TotalActasEsperadas,
		"total_actas_procesadas":   stats.TotalActasProcesadas,
		"total_actas_error":        stats.TotalActasError,
		"total_actas_anuladas":     stats.TotalActasAnuladas,
		"total_actas_observadas":   stats.TotalActasObservadas,
		"total_votos_validos":      stats.TotalVotosValidos,
		"total_votos_nulos":        stats.TotalVotosNulos,
		"total_votos_blancos":      stats.TotalVotosBlancos,
		"total_votantes":           stats.TotalVotantes,
		"total_habilitados":        stats.TotalHabilitados,
		"votos_por_candidato":      stats.VotosPorCandidato,
		"por_departamento":         stats.PorDepartamento,
		"por_tipo_entrada":         stats.PorTipoEntrada,
		"porcentaje_avance":        stats.PorcentajeAvance,
		"tasa_participacion":       stats.TasaParticipacion,
		"margen_victoria":          stats.MargenVictoria,
		"total_inconsistencias":    stats.TotalInconsistencias,
		"inconsistencias_por_tipo": stats.InconsistenciasPorTipo,
		"ultima_actualizacion":     stats.UltimaActualizacion,
	}}

	opts := options.Update().SetUpsert(true)
	_, err := r.collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("error actualizando estadísticas CQRS: %w", err)
	}
	return nil
}

// GetEstadistica retorna la vista materializada actual.
func (r *EstadisticaRepository) GetEstadistica(ctx context.Context) (*models.EstadisticaRRV, error) {
	var stats models.EstadisticaRRV
	err := r.collection.FindOne(ctx, bson.M{"_id": "stats_rrv_global"}).Decode(&stats)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("error obteniendo estadísticas CQRS: %w", err)
	}
	return &stats, nil
}
