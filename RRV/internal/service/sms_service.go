package service

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"rrv-backend/internal/models"
)

// SMSService maneja el parsing, validación y seguridad de mensajes SMS.
type SMSService struct {
	pinValido            string
	telefonosAutorizados map[string]bool
}

// NewSMSService crea una nueva instancia del servicio SMS.
func NewSMSService(pin string, telefonos []string) *SMSService {
	telMap := make(map[string]bool)
	for _, t := range telefonos {
		telMap[t] = true
	}
	return &SMSService{
		pinValido:            pin,
		telefonosAutorizados: telMap,
	}
}

// SMSParseResult contiene los datos extraídos del mensaje SMS.
type SMSParseResult struct {
	ActaID       string
	Departamento string
	Municipio    string
	Recinto      string
	Mesa         string
	VotosValidos int
	VotosNulos   int
	VotosBlancos int
	Candidatos   []models.Candidato
	PIN          string
}

// candidatoRegex detecta campos de candidato dinámicamente: C1, C2, C3, ... CN
var candidatoRegex = regexp.MustCompile(`^C(\d+)$`)

// ValidarTelefono verifica que el teléfono esté en la lista de autorizados.
func (s *SMSService) ValidarTelefono(telefono string) bool {
	return s.telefonosAutorizados[telefono]
}

// ValidarPIN verifica que el PIN del mensaje coincida con el configurado.
func (s *SMSService) ValidarPIN(pin string) bool {
	return pin == s.pinValido
}

// CalcularHashSMS genera un SHA256 del mensaje raw para detección de duplicados.
func (s *SMSService) CalcularHashSMS(mensaje string) string {
	h := sha256.Sum256([]byte(mensaje))
	return fmt.Sprintf("%x", h)
}

// ParseSMS interpreta el mensaje SMS y extrae los datos del acta.
// Formato esperado: ACTA:MESA-35000|DEP:Chuquisaca|MUN:Sucre|REC:U.E. Santa Monica|MESA:1|VAL:500|NUL:10|BLA:5|C1:200|C2:150|C3:100|PIN:1234
// Los campos C\d+ se detectan dinámicamente — puede haber cualquier cantidad de candidatos.
func (s *SMSService) ParseSMS(mensaje string) (*SMSParseResult, []string) {
	var errores []string
	result := &SMSParseResult{}

	// Dividir por separador |
	parts := strings.Split(mensaje, "|")
	fields := make(map[string]string)
	candidatos := make(map[int]int) // número de candidato -> votos

	for _, part := range parts {
		kv := strings.SplitN(part, ":", 2)
		if len(kv) != 2 {
			errores = append(errores, fmt.Sprintf("campo mal formado: '%s'", part))
			continue
		}

		key := strings.TrimSpace(strings.ToUpper(kv[0]))
		value := strings.TrimSpace(kv[1])

		// Detectar campos de candidato dinámicamente
		if matches := candidatoRegex.FindStringSubmatch(key); matches != nil {
			num, err := strconv.Atoi(matches[1])
			if err != nil {
				errores = append(errores, fmt.Sprintf("número de candidato inválido: %s", key))
				continue
			}
			votos, err := strconv.Atoi(value)
			if err != nil {
				errores = append(errores, fmt.Sprintf("votos inválidos para %s: '%s'", key, value))
				continue
			}
			candidatos[num] = votos
		} else {
			fields[key] = value
		}
	}

	// Extraer campos obligatorios
	if v, ok := fields["ACTA"]; ok {
		result.ActaID = v
	} else {
		errores = append(errores, "campo obligatorio faltante: ACTA")
	}

	if v, ok := fields["DEP"]; ok {
		result.Departamento = v
	} else {
		errores = append(errores, "campo obligatorio faltante: DEP (departamento)")
	}

	if v, ok := fields["MUN"]; ok {
		result.Municipio = v
	} else {
		errores = append(errores, "campo obligatorio faltante: MUN (municipio)")
	}

	if v, ok := fields["REC"]; ok {
		result.Recinto = v
	} else {
		result.Recinto = "" // Recinto es opcional en SMS
	}

	if v, ok := fields["MESA"]; ok {
		result.Mesa = v
	} else {
		errores = append(errores, "campo obligatorio faltante: MESA")
	}

	// Parsear campos numéricos
	if v, ok := fields["VAL"]; ok {
		val, err := strconv.Atoi(v)
		if err != nil {
			errores = append(errores, fmt.Sprintf("VAL (votos válidos) no es número: '%s'", v))
		} else {
			result.VotosValidos = val
		}
	} else {
		errores = append(errores, "campo obligatorio faltante: VAL (votos válidos)")
	}

	if v, ok := fields["NUL"]; ok {
		val, err := strconv.Atoi(v)
		if err != nil {
			errores = append(errores, fmt.Sprintf("NUL (votos nulos) no es número: '%s'", v))
		} else {
			result.VotosNulos = val
		}
	} else {
		errores = append(errores, "campo obligatorio faltante: NUL (votos nulos)")
	}

	if v, ok := fields["BLA"]; ok {
		val, err := strconv.Atoi(v)
		if err != nil {
			errores = append(errores, fmt.Sprintf("BLA (votos blancos) no es número: '%s'", v))
		} else {
			result.VotosBlancos = val
		}
	} else {
		errores = append(errores, "campo obligatorio faltante: BLA (votos blancos)")
	}

	if v, ok := fields["PIN"]; ok {
		result.PIN = v
	} else {
		errores = append(errores, "campo obligatorio faltante: PIN")
	}

	// Convertir mapa de candidatos a slice ordenado
	if len(candidatos) == 0 {
		errores = append(errores, "no se encontraron candidatos (C1, C2, C3...)")
	} else {
		// Encontrar el máximo número de candidato
		maxNum := 0
		for num := range candidatos {
			if num > maxNum {
				maxNum = num
			}
		}
		// Crear slice ordenado
		for i := 1; i <= maxNum; i++ {
			votos, exists := candidatos[i]
			if !exists {
				errores = append(errores, fmt.Sprintf("candidato C%d faltante en la secuencia", i))
				votos = 0
			}
			result.Candidatos = append(result.Candidatos, models.Candidato{
				CandidatoID: fmt.Sprintf("C%d", i),
				Votos:       votos,
			})
		}
	}

	return result, errores
}

// ConvertirAActa convierte el resultado del parsing SMS a un ActaRRV.
func (s *SMSService) ConvertirAActa(parsed *SMSParseResult, hashMensaje string) *models.ActaRRV {
	totalVotos := parsed.VotosValidos + parsed.VotosNulos

	return &models.ActaRRV{
		ActaID:       parsed.ActaID,
		CodigoMesa:   parsed.Mesa,
		Departamento: parsed.Departamento,
		Municipio:    parsed.Municipio,
		Recinto:      parsed.Recinto,
		Mesa:         parsed.Mesa,
		Candidatos:   parsed.Candidatos,
		VotosValidos: parsed.VotosValidos,
		VotosNulos:   parsed.VotosNulos,
		VotosBlancos: parsed.VotosBlancos,
		TotalVotos:   totalVotos,
		Fuente:       models.FuenteRRV,
		TipoEntrada:  models.TipoSMS,
		HashOrigen:   hashMensaje,
	}
}
