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

// ReferenciaRepository consulta los datos de referencia cargados del CSV.
// Es de solo lectura — los datos ya están precargados en MongoDB.
type ReferenciaRepository struct {
	distribucionColl *mongo.Collection
	recintoColl      *mongo.Collection
	actaImpresaColl  *mongo.Collection
	transcripcionColl *mongo.Collection
}

// NewReferenciaRepository crea el repositorio y genera índices de consulta.
func NewReferenciaRepository(db *mongo.Database) *ReferenciaRepository {
	repo := &ReferenciaRepository{
		distribucionColl:  db.Collection("Distribution_Territorial"),
		recintoColl:       db.Collection("Recinto_Electoral"),
		actaImpresaColl:   db.Collection("ActasImpresas"),
		transcripcionColl: db.Collection("Transcripciones"),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Índices para búsquedas frecuentes
	repo.actaImpresaColl.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "CodigoActa", Value: 1}},
	})
	repo.recintoColl.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "CodigoRecinto", Value: 1}},
	})
	repo.distribucionColl.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "CodigoTerritorial", Value: 1}},
	})
	repo.transcripcionColl.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "CodigoActa", Value: 1}},
	})

	return repo
}

// FindActaImpresa busca un acta en ActasImpresas por CodigoActa.
func (r *ReferenciaRepository) FindActaImpresa(ctx context.Context, codigoActa int64) (*models.ActaImpresa, error) {
	var acta models.ActaImpresa
	err := r.actaImpresaColl.FindOne(ctx, bson.M{"CodigoActa": codigoActa}).Decode(&acta)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("error buscando acta impresa: %w", err)
	}
	return &acta, nil
}

// FindTranscripcion busca una transcripción por CodigoActa.
func (r *ReferenciaRepository) FindTranscripcion(ctx context.Context, codigoActa int64) (*models.Transcripcion, error) {
	var t models.Transcripcion
	err := r.transcripcionColl.FindOne(ctx, bson.M{"CodigoActa": codigoActa}).Decode(&t)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("error buscando transcripción: %w", err)
	}
	return &t, nil
}

// FindRecinto busca un recinto por CodigoRecinto.
func (r *ReferenciaRepository) FindRecinto(ctx context.Context, codigoRecinto int64) (*models.RecintoElectoral, error) {
	var rec models.RecintoElectoral
	err := r.recintoColl.FindOne(ctx, bson.M{"CodigoRecinto": codigoRecinto}).Decode(&rec)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("error buscando recinto: %w", err)
	}
	return &rec, nil
}

// FindDistribucion busca distribución territorial por código.
func (r *ReferenciaRepository) FindDistribucion(ctx context.Context, codigoTerritorial int) (*models.DistribucionTerritorial, error) {
	var dist models.DistribucionTerritorial
	err := r.distribucionColl.FindOne(ctx, bson.M{"CodigoTerritorial": codigoTerritorial}).Decode(&dist)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("error buscando distribución territorial: %w", err)
	}
	return &dist, nil
}

// CountTotalActas retorna el total de actas esperadas (del CSV).
func (r *ReferenciaRepository) CountTotalActas(ctx context.Context) (int64, error) {
	return r.actaImpresaColl.CountDocuments(ctx, bson.M{})
}

// CountTotalRecintos retorna el total de recintos electorales.
func (r *ReferenciaRepository) CountTotalRecintos(ctx context.Context) (int64, error) {
	return r.recintoColl.CountDocuments(ctx, bson.M{})
}

// SumTotalHabilitados calcula el total de votantes habilitados en todas las mesas.
func (r *ReferenciaRepository) SumTotalHabilitados(ctx context.Context) (int64, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: nil},
			{Key: "total", Value: bson.D{{Key: "$sum", Value: "$VotantesHabilitados"}}},
		}}},
	}
	cursor, err := r.actaImpresaColl.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, err
	}
	defer cursor.Close(ctx)

	if cursor.Next(ctx) {
		var doc struct {
			Total int64 `bson:"total"`
		}
		if e := cursor.Decode(&doc); e == nil {
			return doc.Total, nil
		}
	}
	return 0, nil
}

// GetDepartamentos retorna la lista única de departamentos.
func (r *ReferenciaRepository) GetDepartamentos(ctx context.Context) ([]string, error) {
	results, err := r.distribucionColl.Distinct(ctx, "Departamento", bson.M{})
	if err != nil {
		return nil, err
	}
	deps := make([]string, 0, len(results))
	for _, r := range results {
		if s, ok := r.(string); ok {
			deps = append(deps, s)
		}
	}
	return deps, nil
}

// FindAllTranscripciones retorna todas las transcripciones (limitado para consultas).
func (r *ReferenciaRepository) FindAllTranscripciones(ctx context.Context, limit int64) ([]models.Transcripcion, error) {
	opts := options.Find().SetLimit(limit)
	cursor, err := r.transcripcionColl.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var transcripciones []models.Transcripcion
	if err := cursor.All(ctx, &transcripciones); err != nil {
		return nil, err
	}
	return transcripciones, nil
}

// ═══════════════════════════════════════════════════════════════════
// BANCO DE CONSULTAS — Tipos de respuesta y métodos para el dashboard
// ═══════════════════════════════════════════════════════════════════

// ResultMesasPorRecinto para Query 1.
type ResultMesasPorRecinto struct {
	Departamento  string `json:"departamento" bson:"departamento"`
	Recinto       string `json:"recinto"      bson:"recinto"`
	CantidadMesas int    `json:"cantidad_mesas" bson:"cantidad_mesas"`
}

// ResultVotosMunicipio para Query 2.
type ResultVotosMunicipio struct {
	Departamento string `json:"departamento" bson:"departamento"`
	Municipio    string `json:"municipio"    bson:"municipio"`
	TotalVotos   int64  `json:"total_votos"  bson:"total_votos"`
}

// ResultVotosDepartamento para Query 3.
type ResultVotosDepartamento struct {
	Departamento string `json:"departamento" bson:"departamento"`
	TotalVotos   int64  `json:"total_votos"  bson:"total_votos"`
}

// ResultTopRecinto para Query 4.
type ResultTopRecinto struct {
	Recinto  string `json:"recinto"  bson:"recinto"`
	Votos    int64  `json:"votos"    bson:"votos"`
	Candidato string `json:"candidato" bson:"candidato"`
}

// ResultNulosDepartamento para Query 5.
type ResultNulosDepartamento struct {
	Departamento    string  `json:"departamento"     bson:"departamento"`
	VotosNulos      int64   `json:"votos_nulos"      bson:"votos_nulos"`
	VotosValidos    int64   `json:"votos_validos"    bson:"votos_validos"`
	PorcentajeNulos float64 `json:"porcentaje_nulos" bson:"porcentaje_nulos"`
}

// ResultParticipacion para Query 16.
type ResultParticipacion struct {
	Departamento        string  `json:"departamento"          bson:"departamento"`
	VotantesHabilitados int64   `json:"votantes_habilitados"  bson:"votantes_habilitados"`
	VotosEmitidos       int64   `json:"votos_emitidos"        bson:"votos_emitidos"`
	ParticipacionPct    float64 `json:"participacion_pct"     bson:"participacion_pct"`
}

// ResultGeografico para Query 19.
type ResultGeografico struct {
	Geo        string `json:"geo"         bson:"geo"`
	P1         int64  `json:"P1"          bson:"P1"`
	P2         int64  `json:"P2"          bson:"P2"`
	P3         int64  `json:"P3"          bson:"P3"`
	P4         int64  `json:"P4"          bson:"P4"`
	TotalVotos int64  `json:"total_votos" bson:"total_votos"`
}

// ResultMesaAbstencion para Query 11.
type ResultMesaAbstencion struct {
	Departamento        string  `json:"departamento"          bson:"departamento"`
	Recinto             string  `json:"recinto"               bson:"recinto"`
	NroMesa             int     `json:"nro_mesa"              bson:"nro_mesa"`
	VotantesHabilitados int64   `json:"votantes_habilitados"  bson:"votantes_habilitados"`
	VotosEmitidos       int64   `json:"votos_emitidos"        bson:"votos_emitidos"`
	AbstencionPct       float64 `json:"abstencion_pct"        bson:"abstencion_pct"`
}

// GetMesasPorRecinto retorna cantidad de mesas por recinto y departamento (Query 1).
func (r *ReferenciaRepository) GetMesasPorRecinto(ctx context.Context) ([]ResultMesasPorRecinto, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: bson.D{
				{Key: "departamento", Value: "$Departamento"},
				{Key: "recinto", Value: "$RecintoNombre"},
			}},
			{Key: "cantidad_mesas", Value: bson.D{{Key: "$sum", Value: 1}}},
		}}},
		{{Key: "$project", Value: bson.D{
			{Key: "_id", Value: 0},
			{Key: "departamento", Value: "$_id.departamento"},
			{Key: "recinto", Value: "$_id.recinto"},
			{Key: "cantidad_mesas", Value: 1},
		}}},
		{{Key: "$sort", Value: bson.D{
			{Key: "departamento", Value: 1},
			{Key: "cantidad_mesas", Value: -1},
		}}},
	}
	cursor, err := r.transcripcionColl.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("GetMesasPorRecinto: %w", err)
	}
	defer cursor.Close(ctx)
	var results []ResultMesasPorRecinto
	return results, cursor.All(ctx, &results)
}

// GetVotosPorMunicipio retorna votos totales (P1+P2+P3+P4) por municipio (Query 2).
func (r *ReferenciaRepository) GetVotosPorMunicipio(ctx context.Context) ([]ResultVotosMunicipio, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: bson.D{
				{Key: "departamento", Value: "$Departamento"},
				{Key: "municipio", Value: "$Municipio"},
			}},
			{Key: "total_votos", Value: bson.D{{Key: "$sum", Value: bson.D{
				{Key: "$add", Value: bson.A{"$P1", "$P2", "$P3", "$P4"}},
			}}}},
		}}},
		{{Key: "$project", Value: bson.D{
			{Key: "_id", Value: 0},
			{Key: "departamento", Value: "$_id.departamento"},
			{Key: "municipio", Value: "$_id.municipio"},
			{Key: "total_votos", Value: 1},
		}}},
		{{Key: "$sort", Value: bson.D{
			{Key: "departamento", Value: 1},
			{Key: "total_votos", Value: -1},
		}}},
	}
	cursor, err := r.transcripcionColl.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("GetVotosPorMunicipio: %w", err)
	}
	defer cursor.Close(ctx)
	var results []ResultVotosMunicipio
	return results, cursor.All(ctx, &results)
}

// GetVotosPorDepartamento retorna votos totales por departamento (Query 3).
func (r *ReferenciaRepository) GetVotosPorDepartamento(ctx context.Context) ([]ResultVotosDepartamento, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$Departamento"},
			{Key: "total_votos", Value: bson.D{{Key: "$sum", Value: bson.D{
				{Key: "$add", Value: bson.A{"$P1", "$P2", "$P3", "$P4"}},
			}}}},
		}}},
		{{Key: "$project", Value: bson.D{
			{Key: "_id", Value: 0},
			{Key: "departamento", Value: "$_id"},
			{Key: "total_votos", Value: 1},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "total_votos", Value: -1}}}},
	}
	cursor, err := r.transcripcionColl.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("GetVotosPorDepartamento: %w", err)
	}
	defer cursor.Close(ctx)
	var results []ResultVotosDepartamento
	return results, cursor.All(ctx, &results)
}

// GetTopRecintosPorCandidato retorna los top N recintos por votos de un candidato (Query 4).
// candidatoID debe ser "P1", "P2", "P3" o "P4".
func (r *ReferenciaRepository) GetTopRecintosPorCandidato(ctx context.Context, candidatoID string, limite int) ([]ResultTopRecinto, error) {
	if limite <= 0 {
		limite = 5
	}
	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$RecintoNombre"},
			{Key: "votos", Value: bson.D{{Key: "$sum", Value: "$" + candidatoID}}},
		}}},
		{{Key: "$project", Value: bson.D{
			{Key: "_id", Value: 0},
			{Key: "recinto", Value: "$_id"},
			{Key: "votos", Value: 1},
			{Key: "candidato", Value: bson.M{"$literal": candidatoID}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "votos", Value: -1}}}},
		{{Key: "$limit", Value: int64(limite)}},
	}
	cursor, err := r.transcripcionColl.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("GetTopRecintosPorCandidato: %w", err)
	}
	defer cursor.Close(ctx)
	var results []ResultTopRecinto
	return results, cursor.All(ctx, &results)
}

// GetNulosPorDepartamento retorna votos nulos y su % vs votos válidos (Query 5).
func (r *ReferenciaRepository) GetNulosPorDepartamento(ctx context.Context) ([]ResultNulosDepartamento, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$Departamento"},
			{Key: "votos_nulos", Value: bson.D{{Key: "$sum", Value: "$VotosNulos"}}},
			{Key: "votos_validos", Value: bson.D{{Key: "$sum", Value: "$VotosValidos"}}},
		}}},
		{{Key: "$project", Value: bson.D{
			{Key: "_id", Value: 0},
			{Key: "departamento", Value: "$_id"},
			{Key: "votos_nulos", Value: 1},
			{Key: "votos_validos", Value: 1},
			{Key: "porcentaje_nulos", Value: bson.D{
				{Key: "$cond", Value: bson.D{
					{Key: "if", Value: bson.D{{Key: "$gt", Value: bson.A{"$votos_validos", 0}}}},
					{Key: "then", Value: bson.D{{Key: "$multiply", Value: bson.A{
						bson.D{{Key: "$divide", Value: bson.A{"$votos_nulos", "$votos_validos"}}},
						100,
					}}}},
					{Key: "else", Value: 0},
				}},
			}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "votos_nulos", Value: -1}}}},
	}
	cursor, err := r.transcripcionColl.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("GetNulosPorDepartamento: %w", err)
	}
	defer cursor.Close(ctx)
	var results []ResultNulosDepartamento
	return results, cursor.All(ctx, &results)
}

// GetParticipacionPorDepartamento retorna tasa de participación por departamento (Query 16).
func (r *ReferenciaRepository) GetParticipacionPorDepartamento(ctx context.Context) ([]ResultParticipacion, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$Departamento"},
			{Key: "votantes_habilitados", Value: bson.D{{Key: "$sum", Value: "$VotantesHabilitados"}}},
			{Key: "votos_emitidos", Value: bson.D{{Key: "$sum", Value: "$PapeletasEnAnfora"}}},
		}}},
		{{Key: "$project", Value: bson.D{
			{Key: "_id", Value: 0},
			{Key: "departamento", Value: "$_id"},
			{Key: "votantes_habilitados", Value: 1},
			{Key: "votos_emitidos", Value: 1},
			{Key: "participacion_pct", Value: bson.D{
				{Key: "$cond", Value: bson.D{
					{Key: "if", Value: bson.D{{Key: "$gt", Value: bson.A{"$votantes_habilitados", 0}}}},
					{Key: "then", Value: bson.D{{Key: "$multiply", Value: bson.A{
						bson.D{{Key: "$divide", Value: bson.A{"$votos_emitidos", "$votantes_habilitados"}}},
						100,
					}}}},
					{Key: "else", Value: 0},
				}},
			}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "participacion_pct", Value: -1}}}},
	}
	cursor, err := r.transcripcionColl.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("GetParticipacionPorDepartamento: %w", err)
	}
	defer cursor.Close(ctx)
	var results []ResultParticipacion
	return results, cursor.All(ctx, &results)
}

// GetResultadosPorGeografia retorna votos por candidato filtrado por geografía (Query 19).
// Acepta cualquier combinación de departamento, municipio, provincia como filtros.
func (r *ReferenciaRepository) GetResultadosPorGeografia(ctx context.Context, departamento, municipio, provincia string) ([]ResultGeografico, error) {
	matchFilter := bson.D{}
	groupField := "$Departamento"

	if provincia != "" {
		matchFilter = append(matchFilter, bson.E{Key: "Provincia", Value: provincia})
		groupField = "$Provincia"
	} else if municipio != "" {
		matchFilter = append(matchFilter, bson.E{Key: "Municipio", Value: municipio})
		groupField = "$RecintoNombre"
	} else if departamento != "" {
		matchFilter = append(matchFilter, bson.E{Key: "Departamento", Value: departamento})
		groupField = "$Municipio"
	}

	pipeline := mongo.Pipeline{}
	if len(matchFilter) > 0 {
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: matchFilter}})
	}
	pipeline = append(pipeline,
		bson.D{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: groupField},
			{Key: "P1", Value: bson.D{{Key: "$sum", Value: "$P1"}}},
			{Key: "P2", Value: bson.D{{Key: "$sum", Value: "$P2"}}},
			{Key: "P3", Value: bson.D{{Key: "$sum", Value: "$P3"}}},
			{Key: "P4", Value: bson.D{{Key: "$sum", Value: "$P4"}}},
			{Key: "total_votos", Value: bson.D{{Key: "$sum", Value: bson.D{
				{Key: "$add", Value: bson.A{"$P1", "$P2", "$P3", "$P4"}},
			}}}},
		}}},
		bson.D{{Key: "$project", Value: bson.D{
			{Key: "_id", Value: 0},
			{Key: "geo", Value: "$_id"},
			{Key: "P1", Value: 1},
			{Key: "P2", Value: 1},
			{Key: "P3", Value: 1},
			{Key: "P4", Value: 1},
			{Key: "total_votos", Value: 1},
		}}},
		bson.D{{Key: "$sort", Value: bson.D{{Key: "total_votos", Value: -1}}}},
	)

	cursor, err := r.transcripcionColl.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("GetResultadosPorGeografia: %w", err)
	}
	defer cursor.Close(ctx)
	var results []ResultGeografico
	return results, cursor.All(ctx, &results)
}

// GetMesasConAltaAbstencion retorna mesas con abstención superior al umbral dado (Query 11).
func (r *ReferenciaRepository) GetMesasConAltaAbstencion(ctx context.Context, umbral float64) ([]ResultMesaAbstencion, error) {
	if umbral <= 0 {
		umbral = 20.0
	}
	pipeline := mongo.Pipeline{
		{{Key: "$project", Value: bson.D{
			{Key: "_id", Value: 0},
			{Key: "departamento", Value: "$Departamento"},
			{Key: "recinto", Value: "$RecintoNombre"},
			{Key: "nro_mesa", Value: "$NroMesa"},
			{Key: "votantes_habilitados", Value: "$VotantesHabilitados"},
			{Key: "votos_emitidos", Value: "$PapeletasEnAnfora"},
			{Key: "abstencion_pct", Value: bson.D{
				{Key: "$cond", Value: bson.D{
					{Key: "if", Value: bson.D{{Key: "$gt", Value: bson.A{"$VotantesHabilitados", 0}}}},
					{Key: "then", Value: bson.D{{Key: "$multiply", Value: bson.A{
						bson.D{{Key: "$divide", Value: bson.A{
							bson.D{{Key: "$subtract", Value: bson.A{"$VotantesHabilitados", "$PapeletasEnAnfora"}}},
							"$VotantesHabilitados",
						}}},
						100,
					}}}},
					{Key: "else", Value: 0},
				}},
			}},
		}}},
		{{Key: "$match", Value: bson.D{
			{Key: "abstencion_pct", Value: bson.D{{Key: "$gt", Value: umbral}}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "abstencion_pct", Value: -1}}}},
	}
	cursor, err := r.transcripcionColl.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("GetMesasConAltaAbstencion: %w", err)
	}
	defer cursor.Close(ctx)
	var results []ResultMesaAbstencion
	return results, cursor.All(ctx, &results)
}

// GetTotalesOficial retorna los totales consolidados del cómputo oficial (Transcripciones).
// Usado para comparar TREP vs Oficial (Queries 7 y 8).
func (r *ReferenciaRepository) GetTotalesOficial(ctx context.Context) (map[string]int64, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: nil},
			{Key: "P1", Value: bson.D{{Key: "$sum", Value: "$P1"}}},
			{Key: "P2", Value: bson.D{{Key: "$sum", Value: "$P2"}}},
			{Key: "P3", Value: bson.D{{Key: "$sum", Value: "$P3"}}},
			{Key: "P4", Value: bson.D{{Key: "$sum", Value: "$P4"}}},
			{Key: "votos_validos", Value: bson.D{{Key: "$sum", Value: "$VotosValidos"}}},
			{Key: "votos_nulos", Value: bson.D{{Key: "$sum", Value: "$VotosNulos"}}},
			{Key: "votos_blancos", Value: bson.D{{Key: "$sum", Value: "$VotosBlancos"}}},
		}}},
	}
	cursor, err := r.transcripcionColl.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("GetTotalesOficial: %w", err)
	}
	defer cursor.Close(ctx)

	if cursor.Next(ctx) {
		var d struct {
			P1      int64 `bson:"P1"`
			P2      int64 `bson:"P2"`
			P3      int64 `bson:"P3"`
			P4      int64 `bson:"P4"`
			Validos int64 `bson:"votos_validos"`
			Nulos   int64 `bson:"votos_nulos"`
			Blancos int64 `bson:"votos_blancos"`
		}
		if e := cursor.Decode(&d); e == nil {
			return map[string]int64{
				"P1":            d.P1,
				"P2":            d.P2,
				"P3":            d.P3,
				"P4":            d.P4,
				"votos_validos": d.Validos,
				"votos_nulos":   d.Nulos,
				"votos_blancos": d.Blancos,
				"total_votos":   d.Validos + d.Nulos + d.Blancos,
			}, nil
		}
	}
	return nil, nil
}
