package service

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"rrv-backend/internal/models"
	"rrv-backend/internal/repository"
)

// InconsistenciaService detecta y registra las 4 inconsistencias del documento:
// 1. Inconsistencia aritmética (suma candidatos ≠ votos válidos)
// 2. Datos contradictorios (papeletas ánfora ≠ votantes)
// 3. Falta datos apertura/cierre
// 4. Acta recibida con anterioridad y datos distintos
type InconsistenciaService struct {
	inconsistenciaRepo *repository.InconsistenciaRepository
	referenciaRepo     *repository.ReferenciaRepository
	actaRepo           *repository.ActaRepository
}

// NewInconsistenciaService crea el servicio de detección de inconsistencias.
func NewInconsistenciaService(
	inconsistenciaRepo *repository.InconsistenciaRepository,
	referenciaRepo *repository.ReferenciaRepository,
	actaRepo *repository.ActaRepository,
) *InconsistenciaService {
	return &InconsistenciaService{
		inconsistenciaRepo: inconsistenciaRepo,
		referenciaRepo:     referenciaRepo,
		actaRepo:           actaRepo,
	}
}

// DetectarYRegistrar ejecuta todas las validaciones y registra inconsistencias encontradas.
// Retorna la lista de inconsistencias detectadas.
func (s *InconsistenciaService) DetectarYRegistrar(ctx context.Context, acta *models.ActaRRV, fuente string) []models.LogInconsistencia {
	var inconsistencias []models.LogInconsistencia

	// 1. Inconsistencia aritmética
	if logs := s.detectarAritmetica(acta, fuente); len(logs) > 0 {
		inconsistencias = append(inconsistencias, logs...)
	}

	// 2. Datos contradictorios (comparar con referencia)
	if logs := s.detectarDatosContradictorios(ctx, acta, fuente); len(logs) > 0 {
		inconsistencias = append(inconsistencias, logs...)
	}

	// 3. Falta datos apertura/cierre (verificar contra transcripción)
	if logs := s.detectarFaltaAperturaCierre(ctx, acta, fuente); len(logs) > 0 {
		inconsistencias = append(inconsistencias, logs...)
	}

	// 4. Duplicado con datos diferentes
	if logs := s.detectarDuplicadoDiferente(ctx, acta, fuente); len(logs) > 0 {
		inconsistencias = append(inconsistencias, logs...)
	}

	// Registrar todas en Logs_Inconsistencias
	for i := range inconsistencias {
		if err := s.inconsistenciaRepo.InsertInconsistencia(ctx, &inconsistencias[i]); err != nil {
			log.Printf("[INCONSISTENCIA] Error registrando: %v", err)
		}
	}

	if len(inconsistencias) > 0 {
		log.Printf("[INCONSISTENCIA] %d inconsistencias detectadas para acta %s", len(inconsistencias), acta.ActaID)
	}

	return inconsistencias
}

// detectarAritmetica verifica que la suma de votos por candidato coincida con votos válidos.
func (s *InconsistenciaService) detectarAritmetica(acta *models.ActaRRV, fuente string) []models.LogInconsistencia {
	var logs []models.LogInconsistencia

	sumaCandidatos := 0
	for _, c := range acta.Candidatos {
		sumaCandidatos += c.Votos
	}

	// Suma candidatos ≠ votos válidos
	if acta.VotosValidos > 0 && len(acta.Candidatos) > 0 && sumaCandidatos != acta.VotosValidos {
		logs = append(logs, models.LogInconsistencia{
			ActaID:      acta.ActaID,
			Tipo:        models.InconsistenciaAritmetica,
			Descripcion: fmt.Sprintf("Suma de votos por candidatos (%d) no coincide con votos válidos (%d)", sumaCandidatos, acta.VotosValidos),
			Severidad:   models.SeveridadCritica,
			Metadata: map[string]any{
				"suma_candidatos": sumaCandidatos,
				"votos_validos":   acta.VotosValidos,
				"diferencia":      sumaCandidatos - acta.VotosValidos,
			},
			Fuente: fuente,
			Fecha:  time.Now(),
		})
	}

	// Total votos inconsistente
	totalCalculado := acta.VotosValidos + acta.VotosBlancos + acta.VotosNulos
	if acta.TotalVotos > 0 && totalCalculado != acta.TotalVotos {
		logs = append(logs, models.LogInconsistencia{
			ActaID:      acta.ActaID,
			Tipo:        models.InconsistenciaAritmetica,
			Descripcion: fmt.Sprintf("Total votos (%d) ≠ válidos(%d)+blancos(%d)+nulos(%d)=%d", acta.TotalVotos, acta.VotosValidos, acta.VotosBlancos, acta.VotosNulos, totalCalculado),
			Severidad:   models.SeveridadAlta,
			Metadata: map[string]any{
				"total_reportado": acta.TotalVotos,
				"total_calculado": totalCalculado,
			},
			Fuente: fuente,
			Fecha:  time.Now(),
		})
	}

	return logs
}

// detectarDatosContradictorios compara contra la referencia del CSV.
func (s *InconsistenciaService) detectarDatosContradictorios(ctx context.Context, acta *models.ActaRRV, fuente string) []models.LogInconsistencia {
	var logs []models.LogInconsistencia

	// Intentar obtener código numérico del acta_id
	codigoActa := extraerCodigoActaNumerico(acta.ActaID)
	if codigoActa == 0 {
		return logs
	}

	// Buscar en referencia
	ref, err := s.referenciaRepo.FindActaImpresa(ctx, codigoActa)
	if err != nil || ref == nil {
		return logs
	}

	// Comparar VotantesHabilitados
	if acta.ElectoresHabilitados > 0 && ref.VotantesHabilitados > 0 && acta.ElectoresHabilitados != ref.VotantesHabilitados {
		logs = append(logs, models.LogInconsistencia{
			ActaID:      acta.ActaID,
			Tipo:        models.InconsistenciaDatos,
			Descripcion: fmt.Sprintf("Votantes habilitados reportados (%d) ≠ registro oficial (%d)", acta.ElectoresHabilitados, ref.VotantesHabilitados),
			Severidad:   models.SeveridadAlta,
			Metadata: map[string]any{
				"reportado":      acta.ElectoresHabilitados,
				"oficial":        ref.VotantesHabilitados,
				"codigo_acta_ref": ref.CodigoActa,
			},
			Fuente: fuente,
			Fecha:  time.Now(),
		})
	}

	// Votos > Votantes habilitados
	if acta.TotalVotos > ref.VotantesHabilitados && ref.VotantesHabilitados > 0 {
		logs = append(logs, models.LogInconsistencia{
			ActaID:      acta.ActaID,
			Tipo:        models.InconsistenciaDatos,
			Descripcion: fmt.Sprintf("Total votos (%d) supera votantes habilitados (%d)", acta.TotalVotos, ref.VotantesHabilitados),
			Severidad:   models.SeveridadCritica,
			Metadata: map[string]any{
				"total_votos":          acta.TotalVotos,
				"votantes_habilitados": ref.VotantesHabilitados,
			},
			Fuente: fuente,
			Fecha:  time.Now(),
		})
	}

	// Papeletas en ánfora ≠ votantes (buscar transcripción)
	trans, err := s.referenciaRepo.FindTranscripcion(ctx, codigoActa)
	if err == nil && trans != nil && trans.PapeletasEnAnfora > 0 {
		if acta.PapeletasAnfora > 0 && acta.PapeletasAnfora != trans.PapeletasEnAnfora {
			logs = append(logs, models.LogInconsistencia{
				ActaID:      acta.ActaID,
				Tipo:        models.InconsistenciaDatos,
				Descripcion: fmt.Sprintf("Papeletas en ánfora reportadas (%d) ≠ transcripción oficial (%d)", acta.PapeletasAnfora, trans.PapeletasEnAnfora),
				Severidad:   models.SeveridadAlta,
				Metadata: map[string]any{
					"reportado": acta.PapeletasAnfora,
					"oficial":   trans.PapeletasEnAnfora,
				},
				Fuente: fuente,
				Fecha:  time.Now(),
			})
		}
	}

	return logs
}

// detectarFaltaAperturaCierre verifica si la transcripción de referencia tiene datos de hora.
func (s *InconsistenciaService) detectarFaltaAperturaCierre(ctx context.Context, acta *models.ActaRRV, fuente string) []models.LogInconsistencia {
	var logs []models.LogInconsistencia

	codigoActa := extraerCodigoActaNumerico(acta.ActaID)
	if codigoActa == 0 {
		return logs
	}

	trans, err := s.referenciaRepo.FindTranscripcion(ctx, codigoActa)
	if err != nil || trans == nil {
		return logs
	}

	// Verificar si faltan datos de apertura o cierre en la transcripción
	aperturaVacia := trans.AperturaHora == nil || fmt.Sprintf("%v", trans.AperturaHora) == "" || fmt.Sprintf("%v", trans.AperturaHora) == "0"
	cierreVacia := trans.CierreHora == nil || fmt.Sprintf("%v", trans.CierreHora) == "" || fmt.Sprintf("%v", trans.CierreHora) == "0"

	if aperturaVacia {
		logs = append(logs, models.LogInconsistencia{
			ActaID:      acta.ActaID,
			Tipo:        models.InconsistenciaAperturaCierre,
			Descripcion: "Falta hora de apertura de la jornada electoral en el acta",
			Severidad:   models.SeveridadMedia,
			Metadata:    map[string]any{"campo": "AperturaHora"},
			Fuente:      fuente,
			Fecha:       time.Now(),
		})
	}

	if cierreVacia {
		logs = append(logs, models.LogInconsistencia{
			ActaID:      acta.ActaID,
			Tipo:        models.InconsistenciaAperturaCierre,
			Descripcion: "Falta hora de cierre de la jornada electoral en el acta",
			Severidad:   models.SeveridadMedia,
			Metadata:    map[string]any{"campo": "CierreHora"},
			Fuente:      fuente,
			Fecha:       time.Now(),
		})
	}

	return logs
}

// detectarDuplicadoDiferente verifica si ya existe un acta con el mismo ID pero datos distintos.
func (s *InconsistenciaService) detectarDuplicadoDiferente(ctx context.Context, acta *models.ActaRRV, fuente string) []models.LogInconsistencia {
	var logs []models.LogInconsistencia

	existente, err := s.actaRepo.FindByActaID(ctx, acta.ActaID)
	if err != nil || existente == nil {
		return logs
	}

	// Comparar datos clave
	var diferencias []string

	if existente.VotosValidos != acta.VotosValidos {
		diferencias = append(diferencias, fmt.Sprintf("votos_validos: anterior=%d, nuevo=%d", existente.VotosValidos, acta.VotosValidos))
	}
	if existente.VotosNulos != acta.VotosNulos {
		diferencias = append(diferencias, fmt.Sprintf("votos_nulos: anterior=%d, nuevo=%d", existente.VotosNulos, acta.VotosNulos))
	}
	if existente.VotosBlancos != acta.VotosBlancos {
		diferencias = append(diferencias, fmt.Sprintf("votos_blancos: anterior=%d, nuevo=%d", existente.VotosBlancos, acta.VotosBlancos))
	}
	if existente.Departamento != acta.Departamento && acta.Departamento != "" {
		diferencias = append(diferencias, fmt.Sprintf("departamento: anterior=%s, nuevo=%s", existente.Departamento, acta.Departamento))
	}

	if len(diferencias) > 0 {
		logs = append(logs, models.LogInconsistencia{
			ActaID:      acta.ActaID,
			Tipo:        models.InconsistenciaDuplicadoDiferente,
			Descripcion: fmt.Sprintf("Acta recibida con anterioridad y datos no coinciden: %s", strings.Join(diferencias, "; ")),
			Severidad:   models.SeveridadCritica,
			Metadata: map[string]any{
				"diferencias":    diferencias,
				"hash_anterior":  existente.HashOrigen,
				"hash_nuevo":     acta.HashOrigen,
			},
			Fuente: fuente,
			Fecha:  time.Now(),
		})
	}

	return logs
}

// extraerCodigoActaNumerico intenta obtener un código numérico del acta_id.
// Soporta formatos: "MESA-1010200001001", "1010200001001", "MESA-00035"
func extraerCodigoActaNumerico(actaID string) int64 {
	// Quitar prefijo "MESA-"
	id := strings.TrimPrefix(actaID, "MESA-")
	id = strings.TrimSpace(id)

	n, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return 0
	}
	return n
}
