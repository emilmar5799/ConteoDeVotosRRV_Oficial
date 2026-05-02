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

// SMSRepository maneja las operaciones de registros SMS en MongoDB.
type SMSRepository struct {
	collection *mongo.Collection
}

// NewSMSRepository crea un nuevo repositorio de SMS con índices.
func NewSMSRepository(db *mongo.Database) *SMSRepository {
	coll := db.Collection("sms_rrv")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "hash_mensaje", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "telefono", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "estado", Value: 1}},
		},
	}

	_, err := coll.Indexes().CreateMany(ctx, indexes)
	if err != nil {
		fmt.Printf("[WARN] Error creando índices de sms_rrv: %v\n", err)
	}

	return &SMSRepository{collection: coll}
}

// InsertSMS registra un nuevo SMS. Retorna error si el hash ya existe (idempotencia).
func (r *SMSRepository) InsertSMS(ctx context.Context, sms *models.SMSRegistro) error {
	_, err := r.collection.InsertOne(ctx, sms)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return fmt.Errorf("SMS duplicado: hash=%s ya existe", sms.HashMensaje)
		}
		return fmt.Errorf("error insertando SMS: %w", err)
	}
	return nil
}

// FindByHash busca un SMS por su hash de mensaje.
func (r *SMSRepository) FindByHash(ctx context.Context, hash string) (*models.SMSRegistro, error) {
	var sms models.SMSRegistro
	err := r.collection.FindOne(ctx, bson.M{"hash_mensaje": hash}).Decode(&sms)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("error buscando SMS por hash: %w", err)
	}
	return &sms, nil
}

// FindAll retorna todos los SMS registrados, ordenados por fecha descendente.
func (r *SMSRepository) FindAll(ctx context.Context) ([]models.SMSRegistro, error) {
	opts := options.Find().SetSort(bson.D{{Key: "fecha_recepcion", Value: -1}}).SetLimit(500)

	cursor, err := r.collection.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, fmt.Errorf("error listando SMS: %w", err)
	}
	defer cursor.Close(ctx)

	var registros []models.SMSRegistro
	if err := cursor.All(ctx, &registros); err != nil {
		return nil, fmt.Errorf("error decodificando SMS: %w", err)
	}
	return registros, nil
}
