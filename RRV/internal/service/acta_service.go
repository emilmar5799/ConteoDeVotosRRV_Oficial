package service

import (
	"fmt"
	"strings"

	"rrv-backend/internal/models"
)

// ActaService contiene la lógica de validación de actas RRV.
type ActaService struct{}

// NewActaService crea una nueva instancia del servicio de actas.
func NewActaService() *ActaService {
	return &ActaService{}
}

// ValidarActa ejecuta todas las validaciones sobre un acta y retorna los errores encontrados.
// Si hay errores, el acta se marca como ERROR pero aún se guarda para auditoría.
func (s *ActaService) ValidarActa(acta *models.ActaRRV) []string {
	var errores []string

	// 1. Campos obligatorios
	if acta.ActaID == "" {
		errores = append(errores, "campo obligatorio: acta_id vacío")
	}
	if acta.Departamento == "" {
		errores = append(errores, "campo obligatorio: departamento vacío")
	}
	if acta.Municipio == "" {
		errores = append(errores, "campo obligatorio: municipio vacío")
	}
	if acta.Mesa == "" {
		errores = append(errores, "campo obligatorio: mesa vacío")
	}

	// 2. Votos no negativos
	if acta.VotosValidos < 0 {
		errores = append(errores, fmt.Sprintf("votos_validos no puede ser negativo: %d", acta.VotosValidos))
	}
	if acta.VotosNulos < 0 {
		errores = append(errores, fmt.Sprintf("votos_nulos no puede ser negativo: %d", acta.VotosNulos))
	}
	if acta.VotosBlancos < 0 {
		errores = append(errores, fmt.Sprintf("votos_blancos no puede ser negativo: %d", acta.VotosBlancos))
	}

	// 3. Candidatos no vacíos y votos no negativos
	if len(acta.Candidatos) == 0 {
		errores = append(errores, "no se encontraron candidatos en el acta")
	}
	for _, c := range acta.Candidatos {
		if c.Votos < 0 {
			errores = append(errores, fmt.Sprintf("candidato %s tiene votos negativos: %d", c.CandidatoID, c.Votos))
		}
	}

	// 4. Inconsistencia aritmética: suma de votos por candidato debe coincidir con votos_validos
	sumaCandidatos := 0
	for _, c := range acta.Candidatos {
		sumaCandidatos += c.Votos
	}

	// En el formato boliviano: votos_válidos = suma de votos de todos los candidatos
	// (los votos en blanco se cuentan aparte)
	if acta.VotosValidos > 0 && len(acta.Candidatos) > 0 && sumaCandidatos != acta.VotosValidos {
		errores = append(errores, fmt.Sprintf(
			"inconsistencia aritmética: suma_candidatos(%d) != votos_validos(%d)",
			sumaCandidatos, acta.VotosValidos,
		))
	}

	// 5. Total de votos: validos + blancos + nulos debe ser coherente
	totalCalculado := acta.VotosValidos + acta.VotosBlancos + acta.VotosNulos
	if acta.TotalVotos > 0 && totalCalculado != acta.TotalVotos {
		errores = append(errores, fmt.Sprintf(
			"inconsistencia en total: validos(%d) + blancos(%d) + nulos(%d) = %d, pero total_votos = %d",
			acta.VotosValidos, acta.VotosBlancos, acta.VotosNulos, totalCalculado, acta.TotalVotos,
		))
	}

	// ═══════════════════════════════════════════════════════════
	// 6. Validación visual — anomalías detectadas en la imagen
	// ═══════════════════════════════════════════════════════════

	if acta.ValidacionVisual != nil {
		// ❌ NULIDAD: Uso de lápiz (grafito)
		if acta.ValidacionVisual.LapizDetectado {
			errores = append(errores, fmt.Sprintf(
				"NULIDAD — USO DE LÁPIZ: intensidad promedio %.1f, contraste %.3f",
				acta.ValidacionVisual.IntensidadPromedio, acta.ValidacionVisual.RatioContraste))
		}

		// ❌ NULIDAD: Corrector (liquid paper)
		if acta.ValidacionVisual.CorrectorDetectado {
			errores = append(errores, "NULIDAD — CORRECTOR (LIQUID PAPER) detectado")
		}

		// ❌ NULIDAD: Tachaduras (números superpuestos)
		if acta.ValidacionVisual.TachaduraDetectada {
			errores = append(errores, "NULIDAD — TACHADURA: números superpuestos detectados")
		}

		// ❌ NULIDAD: Texto "ANULADA" detectado visualmente (en PDFs escaneados)
		if acta.ValidacionVisual.TextoAnuladaVisual {
			errores = append(errores, "NULIDAD — TEXTO 'ANULADA' escrito sobre el acta")
		}

		// ❌ NULIDAD: Rotura crítica (falta pedazo de papel)
		if acta.ValidacionVisual.RoturaDetectada {
			errores = append(errores, "NULIDAD — ROTURA CRÍTICA: sección faltante del papel")
		}

		// ❌ NULIDAD: Falta de firmas/huellas (< 3 jurados)
		if !acta.ValidacionVisual.FirmasSuficientes {
			errores = append(errores, fmt.Sprintf(
				"NULIDAD — FIRMAS INSUFICIENTES: %d huellas detectadas (mínimo 3)",
				acta.ValidacionVisual.HuellasDetectadas))
		}

		// ⚠️ OBSERVACIÓN: Mancha sobre zona de datos
		if acta.ValidacionVisual.ManchaDetectada {
			errores = append(errores, fmt.Sprintf(
				"OBSERVACIÓN — MANCHA sobre datos: %.1f%% del área afectada",
				acta.ValidacionVisual.PorcentajeMancha))
		}
	}

	// ═══════════════════════════════════════════════════════════
	// 7. Inconsistencia Total: Votos > Votantes
	// ═══════════════════════════════════════════════════════════
	if acta.ElectoresHabilitados > 0 && acta.TotalVotos > acta.ElectoresHabilitados {
		errores = append(errores, fmt.Sprintf(
			"NULIDAD — VOTOS > VOTANTES: total_votos(%d) > electores_habilitados(%d)",
			acta.TotalVotos, acta.ElectoresHabilitados))
	}

	return errores
}

// DeterminarEstado decide el estado final del acta con esta jerarquía:
//
//	ANULADA > ERROR > OBSERVADA > PROCESADA
func (s *ActaService) DeterminarEstado(acta *models.ActaRRV, errores []string) string {
	// Ya marcada como ANULADA por el parser (texto "ANULADA")
	if acta.Estado == models.EstadoAnulada {
		acta.MotivoEstado = models.MotivoTextoAnulada
		return models.EstadoAnulada
	}

	// Nulidades visuales (orden de prioridad)
	if acta.ValidacionVisual != nil {
		// Texto "ANULADA" detectado visualmente (PDFs escaneados)
		if acta.ValidacionVisual.TextoAnuladaVisual {
			acta.MotivoEstado = models.MotivoTextoAnuladaVisual
			return models.EstadoAnulada
		}
		if acta.ValidacionVisual.LapizDetectado {
			acta.MotivoEstado = models.MotivoLapiz
			return models.EstadoAnulada
		}
		if acta.ValidacionVisual.CorrectorDetectado {
			acta.MotivoEstado = models.MotivoCorrector
			return models.EstadoAnulada
		}
		if acta.ValidacionVisual.TachaduraDetectada {
			acta.MotivoEstado = models.MotivoTachadura
			return models.EstadoAnulada
		}
		if acta.ValidacionVisual.RoturaDetectada {
			acta.MotivoEstado = models.MotivoRotura
			return models.EstadoAnulada
		}
		if !acta.ValidacionVisual.FirmasSuficientes {
			acta.MotivoEstado = models.MotivoFaltaFirmas
			return models.EstadoAnulada
		}
	}

	// Nulidad por votos > votantes
	if acta.ElectoresHabilitados > 0 && acta.TotalVotos > acta.ElectoresHabilitados {
		acta.MotivoEstado = models.MotivoInconsistenciaTotal
		return models.EstadoAnulada
	}

	// Errores de datos
	if len(errores) > 0 {
		soloObservaciones := true
		for _, e := range errores {
			if !strings.HasPrefix(e, "OBSERVACIÓN") {
				soloObservaciones = false
				break
			}
		}
		if soloObservaciones {
			if acta.ValidacionVisual != nil && acta.ValidacionVisual.ManchaDetectada {
				acta.MotivoEstado = models.MotivoMancha
			}
			return models.EstadoObservada
		}
		return models.EstadoError
	}

	return models.EstadoProcesada
}

