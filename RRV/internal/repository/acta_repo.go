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

// ActaRepository maneja las operaciones CRUD de actas en MongoDB.
type ActaRepository struct {
	collection *mongo.Collection
}

// NewActaRepository crea un nuevo repositorio y configura los índices únicos.
func NewActaRepository(db *mongo.Database) *ActaRepository {
	coll := db.Collection("actas_rrv")

	// Crear índices únicos para idempotencia
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "acta_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "hash_origen", Value: 1}},
			Options: options.Index().SetUnique(true).SetSparse(true),
		},
		{
			Keys: bson.D{{Key: "departamento", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "estado", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "tipo_entrada", Value: 1}},
		},
	}

	_, err := coll.Indexes().CreateMany(ctx, indexes)
	if err != nil {
		fmt.Printf("[WARN] Error creando índices de actas_rrv: %v\n", err)
	}

	return &ActaRepository{collection: coll}
}

// InsertActa inserta un acta nueva. Retorna error si el acta_id o hash ya existen (idempotencia).
func (r *ActaRepository) InsertActa(ctx context.Context, acta *models.ActaRRV) error {
	_, err := r.collection.InsertOne(ctx, acta)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return fmt.Errorf("acta duplicada: acta_id=%s o hash=%s ya existe", acta.ActaID, acta.HashOrigen)
		}
		return fmt.Errorf("error insertando acta: %w", err)
	}
	return nil
}

// FindByActaID busca un acta por su ID único.
func (r *ActaRepository) FindByActaID(ctx context.Context, actaID string) (*models.ActaRRV, error) {
	var acta models.ActaRRV
	err := r.collection.FindOne(ctx, bson.M{"acta_id": actaID}).Decode(&acta)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("error buscando acta: %w", err)
	}
	return &acta, nil
}

// FindByHash busca un acta por su hash de origen (detección de duplicados).
func (r *ActaRepository) FindByHash(ctx context.Context, hash string) (*models.ActaRRV, error) {
	var acta models.ActaRRV
	err := r.collection.FindOne(ctx, bson.M{"hash_origen": hash}).Decode(&acta)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("error buscando acta por hash: %w", err)
	}
	return &acta, nil
}

// FindAll retorna todas las actas con filtros opcionales.
func (r *ActaRepository) FindAll(ctx context.Context, filters map[string]string) ([]models.ActaRRV, error) {
	filter := bson.M{}

	if dep, ok := filters["departamento"]; ok && dep != "" {
		filter["departamento"] = dep
	}
	if estado, ok := filters["estado"]; ok && estado != "" {
		filter["estado"] = estado
	}
	if tipo, ok := filters["tipo_entrada"]; ok && tipo != "" {
		filter["tipo_entrada"] = tipo
	}

	opts := options.Find().SetSort(bson.D{{Key: "fecha_recepcion", Value: -1}})

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("error listando actas: %w", err)
	}
	defer cursor.Close(ctx)

	var actas []models.ActaRRV
	if err := cursor.All(ctx, &actas); err != nil {
		return nil, fmt.Errorf("error decodificando actas: %w", err)
	}
	return actas, nil
}

// CountByFilter cuenta documentos que coincidan con un filtro.
func (r *ActaRepository) CountByFilter(ctx context.Context, filter bson.M) (int64, error) {
	return r.collection.CountDocuments(ctx, filter)
}

// AggregateByField agrupa y cuenta actas por un campo específico.
func (r *ActaRepository) AggregateByField(ctx context.Context, field string) (map[string]int64, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$" + field},
			{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
		}}},
	}

	cursor, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("error en agregación por %s: %w", field, err)
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

// AggregateVotos calcula la suma total de votos válidos, nulos y blancos.
func (r *ActaRepository) AggregateVotos(ctx context.Context) (validos, nulos, blancos int64, err error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.D{{Key: "estado", Value: "PROCESADA"}}}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: nil},
			{Key: "total_validos", Value: bson.D{{Key: "$sum", Value: "$votos_validos"}}},
			{Key: "total_nulos", Value: bson.D{{Key: "$sum", Value: "$votos_nulos"}}},
			{Key: "total_blancos", Value: bson.D{{Key: "$sum", Value: "$votos_blancos"}}},
		}}},
	}

	cursor, err2 := r.collection.Aggregate(ctx, pipeline)
	if err2 != nil {
		return 0, 0, 0, fmt.Errorf("error en agregación de votos: %w", err2)
	}
	defer cursor.Close(ctx)

	if cursor.Next(ctx) {
		var doc struct {
			Validos int64 `bson:"total_validos"`
			Nulos   int64 `bson:"total_nulos"`
			Blancos int64 `bson:"total_blancos"`
		}
		if e := cursor.Decode(&doc); e == nil {
			return doc.Validos, doc.Nulos, doc.Blancos, nil
		}
	}
	return 0, 0, 0, nil
}

// AggregateVotosPorCandidato desanida los candidatos y suma votos por candidato_id.
func (r *ActaRepository) AggregateVotosPorCandidato(ctx context.Context) (map[string]int64, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.D{{Key: "estado", Value: "PROCESADA"}}}},
		{{Key: "$unwind", Value: "$candidatos"}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$candidatos.candidato_id"},
			{Key: "total", Value: bson.D{{Key: "$sum", Value: "$candidatos.votos"}}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "total", Value: -1}}}},
	}

	cursor, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("error en agregación por candidato: %w", err)
	}
	defer cursor.Close(ctx)

	result := make(map[string]int64)
	for cursor.Next(ctx) {
		var doc struct {
			ID    string `bson:"_id"`
			Total int64  `bson:"total"`
		}
		if err := cursor.Decode(&doc); err == nil {
			result[doc.ID] = doc.Total
		}
	}
	return result, nil
}
