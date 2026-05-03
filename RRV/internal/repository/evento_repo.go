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

// EventoRepository maneja las operaciones de eventos (Event Sourcing) en MongoDB.
type EventoRepository struct {
	collection *mongo.Collection
}

// NewEventoRepository crea un nuevo repositorio de eventos con índices.
func NewEventoRepository(db *mongo.Database) *EventoRepository {
	coll := db.Collection("eventos_rrv")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "acta_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "tipo_evento", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "fecha", Value: -1}},
		},
	}

	_, err := coll.Indexes().CreateMany(ctx, indexes)
	if err != nil {
		fmt.Printf("[WARN] Error creando índices de eventos_rrv: %v\n", err)
	}

	return &EventoRepository{collection: coll}
}

// InsertEvento registra un nuevo evento del pipeline.
func (r *EventoRepository) InsertEvento(ctx context.Context, evento *models.EventoRRV) error {
	_, err := r.collection.InsertOne(ctx, evento)
	if err != nil {
		return fmt.Errorf("error insertando evento: %w", err)
	}
	return nil
}

// FindAll retorna todos los eventos, ordenados por fecha descendente.
func (r *EventoRepository) FindAll(ctx context.Context) ([]models.EventoRRV, error) {
	opts := options.Find().SetSort(bson.D{{Key: "fecha", Value: -1}}).SetLimit(500)

	cursor, err := r.collection.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, fmt.Errorf("error listando eventos: %w", err)
	}
	defer cursor.Close(ctx)

	var eventos []models.EventoRRV
	if err := cursor.All(ctx, &eventos); err != nil {
		return nil, fmt.Errorf("error decodificando eventos: %w", err)
	}
	return eventos, nil
}

// FindByActaID retorna todos los eventos asociados a un acta.
func (r *EventoRepository) FindByActaID(ctx context.Context, actaID string) ([]models.EventoRRV, error) {
	opts := options.Find().SetSort(bson.D{{Key: "fecha", Value: 1}})

	cursor, err := r.collection.Find(ctx, bson.M{"acta_id": actaID}, opts)
	if err != nil {
		return nil, fmt.Errorf("error buscando eventos por acta: %w", err)
	}
	defer cursor.Close(ctx)

	var eventos []models.EventoRRV
	if err := cursor.All(ctx, &eventos); err != nil {
		return nil, fmt.Errorf("error decodificando eventos: %w", err)
	}
	return eventos, nil
}
