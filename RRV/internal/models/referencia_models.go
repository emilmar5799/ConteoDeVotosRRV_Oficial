package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ═══════════════════════════════════════════════════════════════════
// Modelos de REFERENCIA — datos cargados del CSV proporcionado.
// Estas colecciones son de solo lectura para el pipeline RRV.
// ═══════════════════════════════════════════════════════════════════

// DistribucionTerritorial corresponde a la colección Distribution_Territorial (340 docs).
type DistribucionTerritorial struct {
	ID                primitive.ObjectID `json:"_id,omitempty" bson:"_id,omitempty"`
	CodigoTerritorial int                `json:"CodigoTerritorial" bson:"CodigoTerritorial"`
	Departamento      string             `json:"Departamento" bson:"Departamento"`
	Municipio         string             `json:"Municipio" bson:"Municipio"`
	Provincia         string             `json:"Provincia" bson:"Provincia"`
}

// RecintoElectoral corresponde a la colección Recinto_Electoral (537 docs).
type RecintoElectoral struct {
	ID                primitive.ObjectID `json:"_id,omitempty" bson:"_id,omitempty"`
	CodigoTerritorial int                `json:"CodigoTerritorial" bson:"CodigoTerritorial"`
	CodigoRecinto     int64              `json:"CodigoRecinto" bson:"CodigoRecinto"`
	RecintoNombre     string             `json:"RecintoNombre" bson:"RecintoNombre"`
	RecintoDireccion  string             `json:"RecintoDireccion" bson:"RecintoDireccion"`
	NumMesas          int                `json:"NumMesas" bson:"NumMesas"`
}

// ActaImpresa corresponde a la colección ActasImpresas (5,400 docs).
type ActaImpresa struct {
	ID                  primitive.ObjectID `json:"_id,omitempty" bson:"_id,omitempty"`
	CodigoRecinto       int64              `json:"CodigoRecinto" bson:"CodigoRecinto"`
	CodigoActa          int64              `json:"CodigoActa" bson:"CodigoActa"`
	NroMesa             int                `json:"NroMesa" bson:"NroMesa"`
	VotantesHabilitados int                `json:"VotantesHabilitados" bson:"VotantesHabilitados"`
}

// Transcripcion corresponde a la colección Transcripciones (5,400 docs).
// Contiene los datos de referencia completos con votos reales.
type Transcripcion struct {
	ID                    primitive.ObjectID `json:"_id,omitempty" bson:"_id,omitempty"`
	CodigoTerritorial     int                `json:"CodigoTerritorial" bson:"CodigoTerritorial"`
	Departamento          string             `json:"Departamento" bson:"Departamento"`
	Provincia             string             `json:"Provincia" bson:"Provincia"`
	Municipio             string             `json:"Municipio" bson:"Municipio"`
	CodigoRecinto         int64              `json:"CodigoRecinto" bson:"CodigoRecinto"`
	RecintoNombre         string             `json:"RecintoNombre" bson:"RecintoNombre"`
	NumMesas              int                `json:"NumMesas" bson:"NumMesas"`
	CodigoActa            int64              `json:"CodigoActa" bson:"CodigoActa"`
	NroMesa               int                `json:"NroMesa" bson:"NroMesa"`
	VotantesHabilitados   int                `json:"VotantesHabilitados" bson:"VotantesHabilitados"`
	PapeletasEnAnfora     int                `json:"PapeletasEnAnfora" bson:"PapeletasEnAnfora"`
	PapeletasNoUtilizadas int                `json:"PapeletasNoUtilizadas" bson:"PapeletasNoUtilizadas"`
	P1                    int                `json:"P1" bson:"P1"`
	P2                    int                `json:"P2" bson:"P2"`
	P3                    int                `json:"P3" bson:"P3"`
	P4                    int                `json:"P4" bson:"P4"`
	VotosValidos          int                `json:"VotosValidos" bson:"VotosValidos"`
	VotosBlancos          int                `json:"VotosBlancos" bson:"VotosBlancos"`
	VotosNulos            int                `json:"VotosNulos" bson:"VotosNulos"`
	AperturaHora          interface{}        `json:"AperturaHora" bson:"AperturaHora"`
	CierreHora            interface{}        `json:"CierreHora" bson:"CierreHora"`
	CierreMinutos         interface{}        `json:"CierreMinutos" bson:"CierreMinutos"`
}

// ═══════════════════════════════════════════════════════════════════
// LogInconsistencia — Colección Logs_Inconsistencias
// Registra cada inconsistencia detectada durante el procesamiento.
// ═══════════════════════════════════════════════════════════════════

type LogInconsistencia struct {
	ID          primitive.ObjectID `json:"_id,omitempty" bson:"_id,omitempty"`
	ActaID      string             `json:"acta_id" bson:"acta_id"`
	Tipo        string             `json:"tipo" bson:"tipo"`
	Descripcion string             `json:"descripcion" bson:"descripcion"`
	Severidad   string             `json:"severidad" bson:"severidad"`
	Metadata    map[string]any     `json:"metadata,omitempty" bson:"metadata,omitempty"`
	Fuente      string             `json:"fuente" bson:"fuente"`
	Fecha       time.Time          `json:"fecha" bson:"fecha"`
}

// Tipos de inconsistencia (requeridos por el documento)
const (
	InconsistenciaAritmetica         = "INCONSISTENCIA_ARITMETICA"
	InconsistenciaDatos              = "DATOS_CONTRADICTORIOS"
	InconsistenciaAperturaCierre     = "FALTA_APERTURA_CIERRE"
	InconsistenciaDuplicadoDiferente = "DUPLICADO_DATOS_DIFERENTES"
)

// Severidades
const (
	SeveridadCritica = "CRITICA"
	SeveridadAlta    = "ALTA"
	SeveridadMedia   = "MEDIA"
	SeveridadBaja    = "BAJA"
)

// ═══════════════════════════════════════════════════════════════════
// EstadisticaRRV — Vista materializada CQRS (colección Estadisticas_RRV)
// Se recalcula tras cada acta guardada para lectura rápida.
// ═══════════════════════════════════════════════════════════════════

type EstadisticaRRV struct {
	ID                     string             `json:"_id,omitempty" bson:"_id,omitempty"`
	TotalActasEsperadas    int64              `json:"total_actas_esperadas" bson:"total_actas_esperadas"`
	TotalActasProcesadas   int64              `json:"total_actas_procesadas" bson:"total_actas_procesadas"`
	TotalActasError        int64              `json:"total_actas_error" bson:"total_actas_error"`
	TotalActasAnuladas     int64              `json:"total_actas_anuladas" bson:"total_actas_anuladas"`
	TotalActasObservadas   int64              `json:"total_actas_observadas" bson:"total_actas_observadas"`
	TotalVotosValidos      int64              `json:"total_votos_validos" bson:"total_votos_validos"`
	TotalVotosNulos        int64              `json:"total_votos_nulos" bson:"total_votos_nulos"`
	TotalVotosBlancos      int64              `json:"total_votos_blancos" bson:"total_votos_blancos"`
	TotalVotantes          int64              `json:"total_votantes" bson:"total_votantes"`
	TotalHabilitados       int64              `json:"total_habilitados" bson:"total_habilitados"`
	VotosPorCandidato      map[string]int64   `json:"votos_por_candidato" bson:"votos_por_candidato"`
	PorDepartamento        map[string]int64   `json:"por_departamento" bson:"por_departamento"`
	PorTipoEntrada         map[string]int64   `json:"por_tipo_entrada" bson:"por_tipo_entrada"`
	PorcentajeAvance       float64            `json:"porcentaje_avance" bson:"porcentaje_avance"`
	TasaParticipacion      float64            `json:"tasa_participacion" bson:"tasa_participacion"`
	MargenVictoria         float64            `json:"margen_victoria" bson:"margen_victoria"`
	TotalInconsistencias   int64              `json:"total_inconsistencias" bson:"total_inconsistencias"`
	InconsistenciasPorTipo map[string]int64   `json:"inconsistencias_por_tipo" bson:"inconsistencias_por_tipo"`
	UltimaActualizacion    time.Time          `json:"ultima_actualizacion" bson:"ultima_actualizacion"`
}

// StatsFullResponse es la respuesta completa de estadísticas para el Dashboard.
type StatsFullResponse struct {
	// Métricas base
	TotalActasEsperadas  int64            `json:"total_actas_esperadas"`
	TotalActasProcesadas int64            `json:"total_actas_procesadas"`
	TotalActasError      int64            `json:"total_actas_error"`
	TotalActasAnuladas   int64            `json:"total_actas_anuladas"`
	TotalActasPendientes int64            `json:"total_actas_pendientes"`
	TotalRecintos        int64            `json:"total_recintos"`
	// Votos
	TotalVotosValidos int64            `json:"total_votos_validos"`
	TotalVotosNulos   int64            `json:"total_votos_nulos"`
	TotalVotosBlancos int64            `json:"total_votos_blancos"`
	VotosPorCandidato map[string]int64 `json:"votos_por_candidato"`
	// Geográfico
	PorDepartamento map[string]int64 `json:"por_departamento"`
	PorTipoEntrada  map[string]int64 `json:"por_tipo_entrada"`
	// KPIs
	PorcentajeAvance  float64 `json:"porcentaje_avance"`
	TasaParticipacion float64 `json:"tasa_participacion"`
	MargenVictoria    float64 `json:"margen_victoria"`
	// Transparencia
	PorcentajePublicadas float64 `json:"porcentaje_publicadas"`
	// Inconsistencias
	TotalInconsistencias   int64            `json:"total_inconsistencias"`
	InconsistenciasPorTipo map[string]int64 `json:"inconsistencias_por_tipo"`
	// Meta
	UltimaActualizacion string `json:"ultima_actualizacion"`
}
