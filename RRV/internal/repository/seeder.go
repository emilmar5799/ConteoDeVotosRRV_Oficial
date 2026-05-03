package repository

import (
	"context"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Seeder inicializa colecciones de referencia vacías y migra datos legacy al arrancar.
type Seeder struct {
	db *mongo.Database
}

// NewSeeder crea un nuevo seeder para la base de datos dada.
func NewSeeder(db *mongo.Database) *Seeder {
	return &Seeder{db: db}
}

// SeedAll ejecuta todos los seeders necesarios (idempotente — solo actúa si la colección está vacía).
func (s *Seeder) SeedAll(ctx context.Context) {
	log.Println("[SEEDER] Inicializando colecciones de referencia...")
	s.seedPartidoPolitico(ctx)
	s.seedMesas(ctx)
	s.seedDetalleVotosPartido(ctx)
	s.migrateSMSRecibidos(ctx)
	log.Println("[SEEDER] Inicialización completada ✓")
}

// seedPartidoPolitico carga los 4 candidatos en Partido_Politico si está vacío.
func (s *Seeder) seedPartidoPolitico(ctx context.Context) {
	coll := s.db.Collection("Partido_Politico")
	count, _ := coll.CountDocuments(ctx, bson.M{})
	if count > 0 {
		log.Printf("[SEEDER] Partido_Politico: %d docs existentes, omitiendo", count)
		return
	}

	candidatos := []interface{}{
		bson.M{"codigo": "P1", "candidato": "Daenerys Targaryen", "partido": "MASISP",   "abreviacion": "MAS-ISP"},
		bson.M{"codigo": "P2", "candidato": "Sansa Stark",        "partido": "Partido2", "abreviacion": "P2"},
		bson.M{"codigo": "P3", "candidato": "Robert Baratheon",   "partido": "Partido3", "abreviacion": "P3"},
		bson.M{"codigo": "P4", "candidato": "Tyrion Lannister",   "partido": "Partido4", "abreviacion": "P4"},
	}

	if _, err := coll.InsertMany(ctx, candidatos); err != nil {
		log.Printf("[SEEDER] Partido_Politico: error — %v", err)
		return
	}
	log.Printf("[SEEDER] Partido_Politico: 4 candidatos insertados ✓")
}

// seedMesas copia ActasImpresas → Mesas con el esquema requerido por el documento.
func (s *Seeder) seedMesas(ctx context.Context) {
	mesasColl := s.db.Collection("Mesas")
	count, _ := mesasColl.CountDocuments(ctx, bson.M{})
	if count > 0 {
		log.Printf("[SEEDER] Mesas: %d docs existentes, omitiendo", count)
		return
	}

	srcColl := s.db.Collection("ActasImpresas")
	srcCount, _ := srcColl.CountDocuments(ctx, bson.M{})
	if srcCount == 0 {
		log.Println("[SEEDER] Mesas: ActasImpresas vacío, omitiendo")
		return
	}

	cursor, err := srcColl.Find(ctx, bson.M{}, options.Find().SetBatchSize(500))
	if err != nil {
		log.Printf("[SEEDER] Mesas: error leyendo ActasImpresas — %v", err)
		return
	}
	defer cursor.Close(ctx)

	var batch []interface{}
	total := 0

	for cursor.Next(ctx) {
		var ai struct {
			CodigoActa          int64 `bson:"CodigoActa"`
			CodigoRecinto       int64 `bson:"CodigoRecinto"`
			NroMesa             int   `bson:"NroMesa"`
			VotantesHabilitados int   `bson:"VotantesHabilitados"`
		}
		if err := cursor.Decode(&ai); err != nil {
			continue
		}
		batch = append(batch, bson.M{
			"CodigoMesa":             ai.CodigoActa,
			"NroMesa":                ai.NroMesa,
			"CantidadHabilitada":     ai.VotantesHabilitados,
			"CodigoRecintoElectoral": ai.CodigoRecinto,
		})
		if len(batch) >= 500 {
			if _, e := mesasColl.InsertMany(ctx, batch); e == nil {
				total += len(batch)
			}
			batch = batch[:0]
		}
	}
	if len(batch) > 0 {
		if _, e := mesasColl.InsertMany(ctx, batch); e == nil {
			total += len(batch)
		}
	}

	mesasColl.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "CodigoMesa", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	log.Printf("[SEEDER] Mesas: %d mesas insertadas ✓", total)
}

// seedDetalleVotosPartido agrega votos por candidato y departamento desde Transcripciones.
func (s *Seeder) seedDetalleVotosPartido(ctx context.Context) {
	detColl := s.db.Collection("Detalle_Votos_Partido")
	count, _ := detColl.CountDocuments(ctx, bson.M{})
	if count > 0 {
		log.Printf("[SEEDER] Detalle_Votos_Partido: %d docs existentes, omitiendo", count)
		return
	}

	transcCol := s.db.Collection("Transcripciones")
	transcCount, _ := transcCol.CountDocuments(ctx, bson.M{})
	if transcCount == 0 {
		log.Println("[SEEDER] Detalle_Votos_Partido: Transcripciones vacío, omitiendo")
		return
	}

	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$Departamento"},
			{Key: "P1", Value: bson.D{{Key: "$sum", Value: "$P1"}}},
			{Key: "P2", Value: bson.D{{Key: "$sum", Value: "$P2"}}},
			{Key: "P3", Value: bson.D{{Key: "$sum", Value: "$P3"}}},
			{Key: "P4", Value: bson.D{{Key: "$sum", Value: "$P4"}}},
			{Key: "total_validos", Value: bson.D{{Key: "$sum", Value: "$VotosValidos"}}},
			{Key: "total_nulos", Value: bson.D{{Key: "$sum", Value: "$VotosNulos"}}},
			{Key: "total_blancos", Value: bson.D{{Key: "$sum", Value: "$VotosBlancos"}}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
	}

	cursor, err := transcCol.Aggregate(ctx, pipeline)
	if err != nil {
		log.Printf("[SEEDER] Detalle_Votos_Partido: error agregando — %v", err)
		return
	}
	defer cursor.Close(ctx)

	now := time.Now()
	var docs []interface{}

	for cursor.Next(ctx) {
		var d struct {
			Dep     string `bson:"_id"`
			P1      int64  `bson:"P1"`
			P2      int64  `bson:"P2"`
			P3      int64  `bson:"P3"`
			P4      int64  `bson:"P4"`
			Validos int64  `bson:"total_validos"`
			Nulos   int64  `bson:"total_nulos"`
			Blancos int64  `bson:"total_blancos"`
		}
		if err := cursor.Decode(&d); err != nil || d.Dep == "" {
			continue
		}
		docs = append(docs, bson.M{
			"departamento":        d.Dep,
			"candidato_P1":        "Daenerys Targaryen",
			"candidato_P2":        "Sansa Stark",
			"candidato_P3":        "Robert Baratheon",
			"candidato_P4":        "Tyrion Lannister",
			"P1":                  d.P1,
			"P2":                  d.P2,
			"P3":                  d.P3,
			"P4":                  d.P4,
			"total_votos_validos": d.Validos,
			"total_votos_nulos":   d.Nulos,
			"total_votos_blancos": d.Blancos,
			"fecha_actualizacion": now,
		})
	}

	if len(docs) == 0 {
		return
	}
	if _, err := detColl.InsertMany(ctx, docs); err != nil {
		log.Printf("[SEEDER] Detalle_Votos_Partido: error insertando — %v", err)
		return
	}
	log.Printf("[SEEDER] Detalle_Votos_Partido: %d departamentos insertados ✓", len(docs))
}

// migrateSMSRecibidos copia los documentos de la colección legacy sms_rrv → SMS_Recibidos.
func (s *Seeder) migrateSMSRecibidos(ctx context.Context) {
	dest := s.db.Collection("SMS_Recibidos")
	destCount, _ := dest.CountDocuments(ctx, bson.M{})
	if destCount > 0 {
		log.Printf("[SEEDER] SMS_Recibidos: %d docs existentes, omitiendo migración", destCount)
		return
	}

	src := s.db.Collection("sms_rrv")
	srcCount, _ := src.CountDocuments(ctx, bson.M{})
	if srcCount == 0 {
		return
	}

	cursor, err := src.Find(ctx, bson.M{})
	if err != nil {
		log.Printf("[SEEDER] SMS migración: error leyendo sms_rrv — %v", err)
		return
	}
	defer cursor.Close(ctx)

	var docs []interface{}
	for cursor.Next(ctx) {
		var doc bson.M
		if cursor.Decode(&doc) == nil {
			delete(doc, "_id")
			docs = append(docs, doc)
		}
	}

	if len(docs) == 0 {
		return
	}
	if _, err := dest.InsertMany(ctx, docs); err != nil {
		log.Printf("[SEEDER] SMS migración: error — %v", err)
		return
	}
	log.Printf("[SEEDER] SMS: %d docs migrados sms_rrv → SMS_Recibidos ✓", len(docs))
}
