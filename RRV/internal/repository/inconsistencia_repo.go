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

// InconsistenciaRepository maneja la colección Logs_Inconsistencias.
type InconsistenciaRepository struct {
	collection *mongo.Collection
}

// NewInconsistenciaRepository crea el repositorio con índices.
func NewInconsistenciaRepository(db *mongo.Database) *InconsistenciaRepository {
	coll := db.Collection("Logs_Inconsistencias")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	indexes := []mongo.IndexModel{
		{Keys: bson.D{{Key: "acta_id", Value: 1}}},
		{Keys: bson.D{{Key: "tipo", Value: 1}}},
		{Keys: bson.D{{Key: "fecha", Value: -1}}},
		{Keys: bson.D{{Key: "severidad", Value: 1}}},
	}
	coll.Indexes().CreateMany(ctx, indexes)

	return &InconsistenciaRepository{collection: coll}
}

// InsertInconsistencia registra una nueva inconsistencia.
func (r *InconsistenciaRepository) InsertInconsistencia(ctx context.Context, log *models.LogInconsistencia) error {
	_, err := r.collection.InsertOne(ctx, log)
	if err != nil {
		return fmt.Errorf("error insertando inconsistencia: %w", err)
	}
	return nil
}

// FindAll retorna todas las inconsistencias, ordenadas por fecha desc.
func (r *InconsistenciaRepository) FindAll(ctx context.Context) ([]models.LogInconsistencia, error) {
	opts := options.Find().SetSort(bson.D{{Key: "fecha", Value: -1}}).SetLimit(500)
	cursor, err := r.collection.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, fmt.Errorf("error listando inconsistencias: %w", err)
	}
	defer cursor.Close(ctx)

	var logs []models.LogInconsistencia
	if err := cursor.All(ctx, &logs); err != nil {
		return nil, fmt.Errorf("error decodificando inconsistencias: %w", err)
	}
	return logs, nil
}

// FindByActaID retorna las inconsistencias de un acta específica.
func (r *InconsistenciaRepository) FindByActaID(ctx context.Context, actaID string) ([]models.LogInconsistencia, error) {
	opts := options.Find().SetSort(bson.D{{Key: "fecha", Value: 1}})
	cursor, err := r.collection.Find(ctx, bson.M{"acta_id": actaID}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var logs []models.LogInconsistencia
	if err := cursor.All(ctx, &logs); err != nil {
		return nil, err
	}
	return logs, nil
}

// CountByTipo agrupa y cuenta inconsistencias por tipo.
func (r *InconsistenciaRepository) CountByTipo(ctx context.Context) (map[string]int64, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$tipo"},
			{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
		}}},
	}
	cursor, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	result := make(map[string]int64)
	for cursor.Next(ctx) {
		var doc struct {
			ID    string `bson:"_id"`
			Count int64  `bson:"count"`
		}
		if err := cursor.Decode(&doc); err == nil && doc.ID != "" {
			result[doc.ID] = doc.Count
		}
	}
	return result, nil
}

// CountTotal retorna el total de inconsistencias.
func (r *InconsistenciaRepository) CountTotal(ctx context.Context) (int64, error) {
	return r.collection.CountDocuments(ctx, bson.M{})
}
