package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// --- Colección: actas_rrv ---

// Candidato representa los votos de un candidato individual. La cantidad de candidatos es dinámica.
type Candidato struct {
	CandidatoID string `json:"candidato_id" bson:"candidato_id"` // C1, C2, C3... dinámico
	Votos       int    `json:"votos"        bson:"votos"`
}

// ActaRRV es el documento principal del pipeline de Recuento Rápido de Votos.
type ActaRRV struct {
	ID                    primitive.ObjectID       `json:"_id,omitempty"     bson:"_id,omitempty"`
	ActaID                string                   `json:"acta_id"           bson:"acta_id"`
	CodigoMesa            string                   `json:"codigo_mesa"       bson:"codigo_mesa"`
	Departamento          string                   `json:"departamento"      bson:"departamento"`
	Provincia             string                   `json:"provincia,omitempty" bson:"provincia,omitempty"`
	Municipio             string                   `json:"municipio"         bson:"municipio"`
	Recinto               string                   `json:"recinto"           bson:"recinto"`
	Mesa                  string                   `json:"mesa"              bson:"mesa"`
	Candidatos            []Candidato              `json:"candidatos"        bson:"candidatos"`
	VotosValidos          int                      `json:"votos_validos"     bson:"votos_validos"`
	VotosNulos            int                      `json:"votos_nulos"       bson:"votos_nulos"`
	VotosBlancos          int                      `json:"votos_blancos"     bson:"votos_blancos"`
	TotalVotos            int                      `json:"total_votos"       bson:"total_votos"`
	ElectoresHabilitados  int                      `json:"electores_habilitados" bson:"electores_habilitados"`
	PapeletasAnfora       int                      `json:"papeletas_anfora"  bson:"papeletas_anfora"`
	PapeletasNoUtilizadas int                      `json:"papeletas_no_utilizadas" bson:"papeletas_no_utilizadas"`
	Estado                string                   `json:"estado"            bson:"estado"`
	MotivoEstado          string                   `json:"motivo_estado,omitempty" bson:"motivo_estado,omitempty"`
	Fuente                string                   `json:"fuente"            bson:"fuente"`
	TipoEntrada           string                   `json:"tipo_entrada"      bson:"tipo_entrada"`
	HashOrigen            string                   `json:"hash_origen"       bson:"hash_origen"`
	Errores               []string                 `json:"errores,omitempty"       bson:"errores,omitempty"`
	Observaciones         []string                 `json:"observaciones,omitempty" bson:"observaciones,omitempty"`
	ValidacionVisual      *ValidacionVisualResult   `json:"validacion_visual,omitempty" bson:"validacion_visual,omitempty"`
	FechaRecepcion        time.Time                `json:"fecha_recepcion"         bson:"fecha_recepcion"`
}

// ValidacionVisualResult contiene el resultado del análisis visual del acta.
type ValidacionVisualResult struct {
	// ── Detección clásica (Go nativo) ──────────────────────────────────
	LapizDetectado     bool    `json:"lapiz_detectado"     bson:"lapiz_detectado"`
	IntensidadPromedio float64 `json:"intensidad_promedio" bson:"intensidad_promedio"`
	RatioContraste     float64 `json:"ratio_contraste"     bson:"ratio_contraste"`
	ManchaDetectada    bool    `json:"mancha_detectada"    bson:"mancha_detectada"`
	PorcentajeMancha   float64 `json:"porcentaje_mancha"   bson:"porcentaje_mancha"`
	HuellasDetectadas  int     `json:"huellas_detectadas"  bson:"huellas_detectadas"`
	FirmasSuficientes  bool    `json:"firmas_suficientes"  bson:"firmas_suficientes"`
	CorrectorDetectado bool    `json:"corrector_detectado" bson:"corrector_detectado"`
	TachaduraDetectada bool    `json:"tachadura_detectada" bson:"tachadura_detectada"`
	RoturaDetectada    bool    `json:"rotura_detectada"    bson:"rotura_detectada"`
	TextoAnuladaVisual bool    `json:"texto_anulada_visual" bson:"texto_anulada_visual"`

	// ── Detección avanzada (microservicio Python/OpenCV) ───────────────
	NumerosSobreescritos  bool     `json:"numeros_sobreescritos"  bson:"numeros_sobreescritos"`
	CeldasSobreescritas   []string `json:"celdas_sobreescritas,omitempty" bson:"celdas_sobreescritas,omitempty"`
	ConfusionAlfanumerica bool     `json:"confusion_alfanumerica" bson:"confusion_alfanumerica"`
	CamposSospechosos     []string `json:"campos_sospechosos,omitempty" bson:"campos_sospechosos,omitempty"`
	HuellasZonaNumeros    bool     `json:"huellas_zona_numeros"   bson:"huellas_zona_numeros"`
	ArrugasDetectadas     bool     `json:"arrugas_detectadas"     bson:"arrugas_detectadas"`
	FlagRevisionManual    bool     `json:"flag_revision_manual"   bson:"flag_revision_manual"`
	PipelinePythonUsado   bool     `json:"pipeline_python_usado"  bson:"pipeline_python_usado"`

	Observaciones []string `json:"observaciones,omitempty" bson:"observaciones,omitempty"`
}

// Estados válidos del acta
const (
	EstadoProcesada = "PROCESADA"
	EstadoError     = "ERROR"
	EstadoAnulada   = "ANULADA"    // Nulidad automática — acta no se suma al cómputo
	EstadoObservada = "OBSERVADA"  // Requiere revisión del tribunal
)

// Motivos de nulidad automática (❌ acta pierde valor legal)
const (
	MotivoTextoAnulada        = "TEXTO_ANULADA"            // Jurados escribieron ANULADA
	MotivoTextoAnuladaVisual  = "TEXTO_ANULADA_VISUAL"     // ANULADA detectada por imagen
	MotivoLapiz               = "USO_DE_LAPIZ"             // Datos escritos a lápiz
	MotivoCorrector           = "USO_DE_CORRECTOR"         // Liquid paper detectado
	MotivoTachadura           = "TACHADURA_NUMEROS"        // Números superpuestos
	MotivoFaltaFirmas         = "FALTA_FIRMAS_HUELLAS"     // < 3 jurados firmaron
	MotivoRotura              = "ROTURA_CRITICA"           // Falta pedazo de papel
	MotivoInconsistenciaTotal = "VOTOS_MAYOR_QUE_VOTANTES" // Más votos que personas registradas
)

// Motivos de observación (⚠️ van a revisión, no anulan de inmediato)
const (
	MotivoMancha              = "MANCHA_SOBRE_DATOS"      // Mancha tapa números o QR
	MotivoSobreescritura      = "NUMERO_SOBREESCRITO"     // Dígito escrito encima de otro
	MotivoConfusionAlfa       = "CONFUSION_ALFANUMERICA"  // Letras en campos numéricos
	MotivoHuellaZonaNumeros   = "HUELLA_ZONA_NUMEROS"     // Huella dactilar sobre dígitos
	MotivoActaArrugada        = "ACTA_ARRUGADA"           // Papel muy arrugado o dañado
)

// Fuentes de datos
const (
	FuenteRRV = "RRV"
)

// Tipos de entrada
const (
	TipoImagen = "IMAGEN"
	TipoPDF    = "PDF"
	TipoSMS    = "SMS"
)

// --- Colección: eventos_rrv (Event Sourcing) ---

// EventoRRV registra cada evento del pipeline para trazabilidad y auditoría.
type EventoRRV struct {
	ID          primitive.ObjectID `json:"_id,omitempty" bson:"_id,omitempty"`
	ActaID      string             `json:"acta_id"       bson:"acta_id"`
	TipoEvento  string             `json:"tipo_evento"   bson:"tipo_evento"`
	Descripcion string             `json:"descripcion"   bson:"descripcion"`
	Metadata    map[string]any     `json:"metadata,omitempty" bson:"metadata,omitempty"`
	Fecha       time.Time          `json:"fecha"         bson:"fecha"`
}

// Tipos de evento del pipeline
const (
	EventoActaRecibida      = "ACTA_RECIBIDA"
	EventoOCRProcesado      = "OCR_PROCESADO"
	EventoValidacionOK      = "VALIDACION_OK"
	EventoValidacionError   = "VALIDACION_ERROR"
	EventoActaGuardada      = "ACTA_GUARDADA"
	EventoDuplicadoDetectado = "DUPLICADO_DETECTADO"
	EventoSMSRecibido       = "SMS_RECIBIDO"
	EventoSMSPinInvalido    = "SMS_PIN_INVALIDO"
	EventoSMSTelNoAutorizado = "SMS_TELEFONO_NO_AUTORIZADO"
	EventoSMSDuplicado      = "SMS_DUPLICADO"
)

// --- Colección: sms_rrv ---

// SMSRegistro almacena cada SMS recibido para auditoría.
type SMSRegistro struct {
	ID             primitive.ObjectID `json:"_id,omitempty"     bson:"_id,omitempty"`
	Telefono       string             `json:"telefono"          bson:"telefono"`
	MensajeRaw     string             `json:"mensaje_raw"       bson:"mensaje_raw"`
	HashMensaje    string             `json:"hash_mensaje"      bson:"hash_mensaje"`
	ActaID         string             `json:"acta_id"           bson:"acta_id"`
	Estado         string             `json:"estado"            bson:"estado"`
	Errores        []string           `json:"errores,omitempty" bson:"errores,omitempty"`
	FechaRecepcion time.Time          `json:"fecha_recepcion"   bson:"fecha_recepcion"`
}

// Estados del SMS
const (
	SMSProcesado = "PROCESADO"
	SMSError     = "ERROR"
	SMSDuplicado = "DUPLICADO"
	SMSRechazado = "RECHAZADO"
)

// --- DTOs (Request/Response) ---

// SMSRequest es el JSON que recibe el endpoint /api/rrv/sms.
type SMSRequest struct {
	Telefono string `json:"telefono" binding:"required"`
	Mensaje  string `json:"mensaje"  binding:"required"`
}

// APIResponse es la respuesta estándar de la API.
type APIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
	Errors  []string `json:"errors,omitempty"`
}

// StatsResponse contiene estadísticas agregadas del RRV.
type StatsResponse struct {
	TotalActas       int64                    `json:"total_actas"`
	ActasProcesadas  int64                    `json:"actas_procesadas"`
	ActasError       int64                    `json:"actas_error"`
	PorDepartamento  map[string]int64         `json:"por_departamento"`
	PorTipoEntrada   map[string]int64         `json:"por_tipo_entrada"`
	VotosPorCandidato map[string]int64        `json:"votos_por_candidato"`
	TotalVotosValidos int64                   `json:"total_votos_validos"`
	TotalVotosNulos   int64                   `json:"total_votos_nulos"`
	TotalVotosBlancos int64                   `json:"total_votos_blancos"`
}
