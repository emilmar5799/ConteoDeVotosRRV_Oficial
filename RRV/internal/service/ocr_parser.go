package service

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"rrv-backend/internal/models"
)

// ActaParser parsea texto extraído de actas electorales bolivianas.
// Soporta dos fuentes: texto embebido (mutool) y OCR (Tesseract).
type ActaParser struct {
	// Candidatos conocidos en el orden que aparecen en el acta
	candidatosConocidos []string

	// Regex patterns compilados (usados solo para OCR/fallback)
	reCodigoMesa   *regexp.Regexp
	reNumeroMesa   *regexp.Regexp
	reDepartamento *regexp.Regexp
	reProvincia    *regexp.Regexp
	reMunicipio    *regexp.Regexp
	reLocalidad    *regexp.Regexp
	reRecinto      *regexp.Regexp
	reVotosValidos *regexp.Regexp
	reVotosBlancos *regexp.Regexp
	reVotosNulos   *regexp.Regexp
}

// NewActaParser crea un nuevo parser de actas.
func NewActaParser() *ActaParser {
	return &ActaParser{
		candidatosConocidos: []string{
			"Daenerys Targaryen",
			"Sansa Stark",
			"Robert Baratheon",
			"Tyrion Lannister",
		},

		// Código de mesa: número largo (13-16 dígitos)
		reCodigoMesa: regexp.MustCompile(`(?i)C[OÓ0]DIGO\s+DE\s+MESA[:\s]*(\d{10,16})`),

		// Número de mesa: número pequeño después de "NÚMERO DE MESA"
		reNumeroMesa: regexp.MustCompile(`(?i)N[UÚ]MERO\s+DE\s+MESA[:\s]*(\d{1,3})`),

		// Ubicación
		reDepartamento: regexp.MustCompile(`(?i)Departamento[:\s]+([A-ZÁÉÍÓÚÑa-záéíóúñ\s]+?)(?:\s*$|\s*Provincia|\s*\d)`),
		reProvincia:    regexp.MustCompile(`(?i)Provincia[:\s]+([A-ZÁÉÍÓÚÑa-záéíóúñ\s]+?)(?:\s*$|\s*Municipio|\s*\d)`),
		reMunicipio:    regexp.MustCompile(`(?i)Municipio[:\s]+([A-ZÁÉÍÓÚÑa-záéíóúñ\s]+?)(?:\s*$|\s*Localidad|\s*\d)`),
		reLocalidad:    regexp.MustCompile(`(?i)Localidad[:\s]+([A-ZÁÉÍÓÚÑa-záéíóúñ\s.]+?)(?:\s*$|\s*Recinto|\s*\d)`),
		reRecinto:      regexp.MustCompile(`(?i)Recinto[:\s]+([A-ZÁÉÍÓÚÑa-záéíóúñ\s.]+?)(?:\s*$|\s*\d)`),

		// Votos totales
		reVotosValidos: regexp.MustCompile(`(?i)VOTOS\s+V[ÁA]LIDOS[:\s]*(\d[\d\s]{0,8}\d)`),
		reVotosBlancos: regexp.MustCompile(`(?i)VOTOS\s+BLANCOS[:\s]*(\d[\d\s]{0,8}\d)`),
		reVotosNulos:   regexp.MustCompile(`(?i)VOTOS\s+NULOS[:\s]*(\d[\d\s]{0,8}\d)`),
	}
}

// ParseActaText parsea el texto y retorna un ActaRRV poblado.
// Detecta automáticamente si el texto viene de mutool (embebido) o Tesseract (OCR).
func (p *ActaParser) ParseActaText(text string, fileName string) *models.ActaRRV {
	acta := &models.ActaRRV{
		FechaRecepcion: time.Now(),
		Fuente:         models.FuenteRRV,
		TipoEntrada:    models.TipoPDF,
	}

	var errores []string

	// Detectar acta ANULADA — variantes posibles
	upperText := strings.ToUpper(text)
	if strings.Contains(upperText, "ANULAD") ||
		strings.Contains(upperText, "NULA") ||
		strings.Contains(upperText, "NULO") {
		acta.Estado = models.EstadoAnulada
		acta.MotivoEstado = models.MotivoTextoAnulada
		errores = append(errores, "Acta marcada como ANULADA")
	}

	// ═══════════════════════════════════════════════════════
	// Determinar fuente y parsear según corresponda
	// ═══════════════════════════════════════════════════════
	if strings.Contains(text, "--- FUENTE: TEXTO_EMBEBIDO ---") {
		// Extraer la parte embebida (después del marcador)
		embeddedText := strings.SplitN(text, "--- FUENTE: TEXTO_EMBEBIDO ---", 2)[1]
		p.parsearTextoEmbebido(embeddedText, acta, &errores)
	} else {
		// Texto OCR clásico (fallback)
		p.parsearTextoOCR(text, acta, &errores)
	}

	// Fallback para código de mesa desde nombre de archivo
	if acta.CodigoMesa == "" {
		acta.CodigoMesa = p.extraerCodigoDeNombreArchivo(fileName)
	}

	// Derivar número de mesa del código si no se encontró
	if acta.Mesa == "" && acta.CodigoMesa != "" && len(acta.CodigoMesa) >= 3 {
		ultimosTres := acta.CodigoMesa[len(acta.CodigoMesa)-3:]
		n, err := strconv.Atoi(ultimosTres)
		if err == nil {
			acta.Mesa = fmt.Sprintf("%d", n)
		}
	}

	// ActaID
	if acta.CodigoMesa != "" {
		acta.ActaID = "MESA-" + acta.CodigoMesa
	} else if acta.Mesa != "" {
		acta.ActaID = "MESA-" + acta.Mesa
	} else {
		acta.ActaID = "MESA-DESCONOCIDA-" + fileName
		errores = append(errores, "no se pudo determinar ID del acta")
	}

	// Calcular total
	acta.TotalVotos = acta.VotosValidos + acta.VotosBlancos + acta.VotosNulos

	// Validación aritmética: votos_válidos = suma de candidatos
	sumaCandidatos := 0
	for _, c := range acta.Candidatos {
		sumaCandidatos += c.Votos
	}
	if acta.VotosValidos > 0 && len(acta.Candidatos) > 0 && sumaCandidatos != acta.VotosValidos {
		errores = append(errores, fmt.Sprintf(
			"inconsistencia aritmética: suma_candidatos(%d) != votos_válidos(%d)",
			sumaCandidatos, acta.VotosValidos,
		))
	}

	// Estado final
	acta.Errores = errores
	if acta.Estado != models.EstadoAnulada {
		if len(errores) > 0 {
			acta.Estado = models.EstadoError
		} else {
			acta.Estado = models.EstadoProcesada
		}
	}

	return acta
}

// ═══════════════════════════════════════════════════════════════════
// PARSER PARA TEXTO EMBEBIDO (mutool draw -F txt)
// Fuente 100% confiable — el texto está en el PDF digital
// ═══════════════════════════════════════════════════════════════════

// parsearTextoEmbebido extrae datos del texto embebido del PDF.
// El texto embebido de mutool sigue un formato estructurado y predecible:
//
// Línea 1: Departamento (ej: "Chuquisaca")
// Línea 2: Provincia (ej: "Oropeza")
// Línea 3: Municipio (ej: "Yotala")
// Línea 4: Localidad (ej: "U.E. Padresama")
// Línea 5: Recinto/Dirección
// (vacía)
// Código de mesa (13 dígitos, ej: "1010200001001")
// (vacía)
// Número de mesa (1 dígito)
// ... luego horas, electores, papeletas ...
// ... luego los votos por candidato como "d d d" (3 dígitos separados por espacio)
// ... los últimos 3 son: Válidos, Blancos, Nulos
func (p *ActaParser) parsearTextoEmbebido(text string, acta *models.ActaRRV, errores *[]string) {
	lines := strings.Split(strings.TrimSpace(text), "\n")

	// Limpiar líneas vacías y trim
	var cleanLines []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" && l != "\f" {
			cleanLines = append(cleanLines, l)
		}
	}

	if len(cleanLines) < 5 {
		*errores = append(*errores, "texto embebido insuficiente")
		return
	}

	// Las primeras líneas son la ubicación geográfica
	acta.Departamento = cleanLines[0]
	acta.Provincia = cleanLines[1]
	acta.Municipio = cleanLines[2]

	// Buscar el código de mesa (número de 13+ dígitos)
	reCodigoMesa := regexp.MustCompile(`^(\d{13,16})$`)
	for _, line := range cleanLines {
		m := reCodigoMesa.FindStringSubmatch(line)
		if len(m) > 1 {
			acta.CodigoMesa = m[1]
			break
		}
	}

	// Buscar el número de mesa (dígito solo en su línea, después del código)
	reMesa := regexp.MustCompile(`^(\d{1,2})$`)
	codigoFound := false
	for _, line := range cleanLines {
		if reCodigoMesa.MatchString(line) {
			codigoFound = true
			continue
		}
		if codigoFound {
			m := reMesa.FindStringSubmatch(line)
			if len(m) > 1 {
				acta.Mesa = m[1]
				break
			}
		}
	}

	// ═══ Extraer todos los números de 3 dígitos (formato "d d d") ═══
	// Estos son los valores escritos en las casillas del acta.
	// NOTA: Algunos PDFs usan caracteres confusos en lugar de dígitos:
	//   O→0, l→1, I→1, |→1, B→8, S→5, Z→2, G→6
	// El regex acepta todos estos caracteres como variantes de dígitos, y permite espacios opcionales (\s*).
	reDigitTriplet := regexp.MustCompile(`^([0-9OlIBSZG|])\s*([0-9OlIBSZG|])\s*([0-9OlIBSZG|])$`)
	var triplets []int

	for _, line := range cleanLines {
		m := reDigitTriplet.FindStringSubmatch(line)
		if len(m) == 4 {
			// Convertir "1 4 O" → 140 (reemplazar O→0)
			d1 := charToDigit(m[1])
			d2 := charToDigit(m[2])
			d3 := charToDigit(m[3])
			valor := d1*100 + d2*10 + d3
			triplets = append(triplets, valor)
		}
	}

	// Los tripletes en un acta estándar siguen este orden:
	// [hora_inicio1, hora_inicio2, hora_cierre1, hora_cierre2, electores, papeletas, no_utilizadas,
	//  candidato1, candidato2, candidato3, candidato4, votos_validos, votos_blancos, votos_nulos]
	//
	// Los ÚLTIMOS 7 tripletes siempre son:
	//   [-7]: Candidato 1 (Daenerys Targaryen)
	//   [-6]: Candidato 2 (Sansa Stark)
	//   [-5]: Candidato 3 (Robert Baratheon)
	//   [-4]: Candidato 4 (Tyrion Lannister)
	//   [-3]: Votos Válidos
	//   [-2]: Votos Blancos
	//   [-1]: Votos Nulos

	if len(triplets) >= 7 {
		n := len(triplets)

		// Candidatos (últimos 7, los primeros 4 son candidatos)
		acta.Candidatos = make([]models.Candidato, 4)
		for i := 0; i < 4; i++ {
			acta.Candidatos[i] = models.Candidato{
				CandidatoID: fmt.Sprintf("C%d", i+1),
				Votos:       triplets[n-7+i],
			}
		}

		// Totales
		acta.VotosValidos = triplets[n-3]
		acta.VotosBlancos = triplets[n-2]
		acta.VotosNulos = triplets[n-1]

		// Datos adicionales del acta (si hay suficientes tripletes):
		//   [-10]: Electores habilitados
		//   [-9]:  Papeletas en ánfora
		//   [-8]:  Papeletas no utilizadas
		if n >= 10 {
			acta.ElectoresHabilitados = triplets[n-10]
			acta.PapeletasAnfora = triplets[n-9]
			acta.PapeletasNoUtilizadas = triplets[n-8]
		}
	} else {
		*errores = append(*errores, fmt.Sprintf("solo se encontraron %d tripletes de votos (se necesitan mínimo 7)", len(triplets)))
	}
}

// ═══════════════════════════════════════════════════════════════════
// PARSER PARA TEXTO OCR (Tesseract) — fallback para PDFs escaneados
// ═══════════════════════════════════════════════════════════════════

func (p *ActaParser) parsearTextoOCR(text string, acta *models.ActaRRV, errores *[]string) {
	// Código de mesa
	acta.CodigoMesa = p.extraerCodigoMesa(text)

	// Número de mesa
	acta.Mesa = p.extraerNumeroMesa(text)

	// Ubicación
	acta.Departamento = p.extraerCampoUbicacion(text, p.reDepartamento)
	acta.Provincia = p.extraerCampoUbicacion(text, p.reProvincia)
	acta.Municipio = p.extraerCampoUbicacion(text, p.reMunicipio)
	acta.Recinto = p.extraerCampoUbicacion(text, p.reRecinto)

	// Candidatos y votos
	candidatos := p.extraerCandidatos(text)
	acta.Candidatos = candidatos
	if len(candidatos) == 0 {
		*errores = append(*errores, "no se encontraron candidatos en OCR")
	}

	// Votos válidos, blancos, nulos
	acta.VotosValidos = p.extraerVotos(text, p.reVotosValidos)
	acta.VotosBlancos = p.extraerVotos(text, p.reVotosBlancos)
	acta.VotosNulos = p.extraerVotos(text, p.reVotosNulos)
}

// ═══════════════════════════════════════════════════════════════════
// Funciones auxiliares de extracción (para OCR)
// ═══════════════════════════════════════════════════════════════════

func (p *ActaParser) extraerCodigoMesa(text string) string {
	matches := p.reCodigoMesa.FindStringSubmatch(text)
	if len(matches) > 1 {
		return limpiarNumero(matches[1])
	}
	// Fallback: buscar número largo de 13+ dígitos
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if i > 15 {
			break
		}
		re := regexp.MustCompile(`\b(\d{13,16})\b`)
		m := re.FindStringSubmatch(line)
		if len(m) > 1 {
			return m[1]
		}
	}
	return ""
}

func (p *ActaParser) extraerNumeroMesa(text string) string {
	matches := p.reNumeroMesa.FindStringSubmatch(text)
	if len(matches) > 1 {
		return limpiarNumero(matches[1])
	}
	return ""
}

func (p *ActaParser) extraerCampoUbicacion(text string, re *regexp.Regexp) string {
	matches := re.FindStringSubmatch(text)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

func (p *ActaParser) extraerCandidatos(text string) []models.Candidato {
	var candidatos []models.Candidato
	lines := strings.Split(text, "\n")

	for idx, nombre := range p.candidatosConocidos {
		votos := p.buscarVotosCandidato(lines, nombre)
		candidatos = append(candidatos, models.Candidato{
			CandidatoID: fmt.Sprintf("C%d", idx+1),
			Votos:       votos,
		})
	}

	algunoEncontrado := false
	for _, c := range candidatos {
		if c.Votos >= 0 {
			algunoEncontrado = true
			break
		}
	}
	if !algunoEncontrado {
		return nil
	}
	return candidatos
}

func (p *ActaParser) buscarVotosCandidato(lines []string, nombre string) int {
	partes := strings.Fields(nombre)
	if len(partes) < 2 {
		return 0
	}
	apellido := strings.ToLower(partes[len(partes)-1])

	for _, line := range lines {
		lineLower := strings.ToLower(line)
		if !strings.Contains(lineLower, apellido) {
			if !p.containsFuzzy(lineLower, apellido) {
				continue
			}
		}
		votos := p.extraerUltimoNumero(line)
		if votos >= 0 {
			return votos
		}
	}
	return 0
}

func (p *ActaParser) containsFuzzy(text, pattern string) bool {
	replacements := map[string][]string{
		"targaryen": {"targarven", "targaryem", "tarqaryen", "targarycn"},
		"stark":     {"starh", "stork"},
		"baratheon": {"barathcon", "baratheom", "barathean"},
		"lannister": {"lamnister", "lannisler", "lamister", "lanmister"},
		"daenerys":  {"daenervs", "dacnerys"},
		"sansa":     {"samsa", "sanso"},
		"robert":    {"rohert", "roberl"},
		"tyrion":    {"tvrion", "tyrlon", "tyrian"},
	}
	if variants, ok := replacements[pattern]; ok {
		for _, v := range variants {
			if strings.Contains(text, v) {
				return true
			}
		}
	}
	return false
}

func (p *ActaParser) extraerUltimoNumero(line string) int {
	// Buscar número de 3 dígitos al final (con posibles espacios)
	re := regexp.MustCompile(`(\d[\s]*\d[\s]*\d)\s*$`)
	matches := re.FindStringSubmatch(line)
	if len(matches) > 1 {
		return parseNumeroOCR(matches[1])
	}

	// Fallback: cualquier grupo de 3 dígitos
	re2 := regexp.MustCompile(`(\d[\s]*\d[\s]*\d)`)
	allMatches := re2.FindAllStringSubmatch(line, -1)
	if len(allMatches) > 0 {
		last := allMatches[len(allMatches)-1]
		return parseNumeroOCR(last[1])
	}

	// 1-2 dígitos al final
	re3 := regexp.MustCompile(`(\d{1,3})\s*$`)
	matches = re3.FindStringSubmatch(line)
	if len(matches) > 1 {
		n, err := strconv.Atoi(strings.TrimSpace(matches[1]))
		if err == nil {
			return n
		}
	}
	return 0
}

func (p *ActaParser) extraerVotos(text string, re *regexp.Regexp) int {
	matches := re.FindStringSubmatch(text)
	if len(matches) > 1 {
		return parseNumeroOCR(matches[1])
	}
	return 0
}

func (p *ActaParser) extraerCodigoDeNombreArchivo(fileName string) string {
	re := regexp.MustCompile(`acta_(\d+)`)
	matches := re.FindStringSubmatch(fileName)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// === Funciones auxiliares ===

// charToDigit convierte un string de un carácter a su valor numérico.
// Maneja variantes de encoding de fuentes en PDFs:
//   O,o → 0 | l,I → 1 | B → 8 | S → 5 | Z → 2 | G → 6
func charToDigit(s string) int {
	switch s {
	case "O", "o":
		return 0
	case "l", "I", "|":
		return 1
	case "B":
		return 8
	case "S":
		return 5
	case "Z":
		return 2
	case "G":
		return 6
	default:
		n, err := strconv.Atoi(s)
		if err != nil {
			return 0
		}
		return n
	}
}

// parseNumeroOCR convierte un string OCR de número (posiblemente con espacios y errores) a int.
func parseNumeroOCR(s string) int {
	s = strings.ReplaceAll(s, " ", "")
	s = strings.TrimSpace(s)

	// Reemplazar errores comunes de OCR
	s = strings.ReplaceAll(s, "O", "0")
	s = strings.ReplaceAll(s, "o", "0")
	s = strings.ReplaceAll(s, "l", "1")
	s = strings.ReplaceAll(s, "I", "1")
	s = strings.ReplaceAll(s, "S", "5")
	s = strings.ReplaceAll(s, "B", "8")
	s = strings.ReplaceAll(s, "Z", "2")
	s = strings.ReplaceAll(s, "G", "6")

	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// limpiarNumero limpia un string numérico removiendo espacios y caracteres no numéricos.
func limpiarNumero(s string) string {
	s = strings.ReplaceAll(s, " ", "")
	s = strings.TrimSpace(s)
	var result strings.Builder
	for _, ch := range s {
		if ch >= '0' && ch <= '9' {
			result.WriteRune(ch)
		}
	}
	return result.String()
}
