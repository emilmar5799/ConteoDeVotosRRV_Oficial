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

		// Código de mesa: número largo (13-16 dígitos).
		// Acepta variantes OCR: "CODIGO", "C0DIGO", "CÓDIGO" (con/sin tilde),
		// y el valor puede estar en la misma línea o en la siguiente (\s* incluye \n).
		reCodigoMesa: regexp.MustCompile(`(?i)C[OÓ0]D[lI1]G[OÓ0]\s+DE\s+MESA[:\s]*(\d{10,16})`),

		// Número de mesa: número pequeño después de "NÚMERO DE MESA".
		// Variantes OCR: "NUMERO", "NÚMERO", "N MERO" (espacio por Ú perdida).
		reNumeroMesa: regexp.MustCompile(`(?i)N[UÚ]?M?E?R[OÓ0]\s+DE\s+MESA[:\s]*(\d{1,3})`),

		// Ubicación — toleran variantes OCR comunes en letras españolas:
		//   á/a/4, é/e, í/i/1, ó/o/0, ú/u, ñ/n, rn→m, etc.
		// También admiten que el valor esté en la siguiente línea
		// (el label termina en \s+ que incluye \n en Go regexp).
		reDepartamento: regexp.MustCompile(`(?i)Dep[a4]rt[a4]m[ei]n?t[o0][:\s]+([A-ZÁÉÍÓÚÑa-záéíóúñ\s]+?)(?:\s*(?:Prov|Mun|Rec|Local|\d))`),
		reProvincia:    regexp.MustCompile(`(?i)Prov[i1]nc[i1][a4][:\s]+([A-ZÁÉÍÓÚÑa-záéíóúñ\s]+?)(?:\s*(?:Mun|Dep|Rec|Local|\d))`),
		reMunicipio:    regexp.MustCompile(`(?i)Mun[i1]c[i1]p[i1][o0][:\s]+([A-ZÁÉÍÓÚÑa-záéíóúñ\s]+?)(?:\s*(?:Local|Rec|Prov|Dep|\d))`),
		reLocalidad:    regexp.MustCompile(`(?i)Local[i1]d[a4]d[:\s]+([A-ZÁÉÍÓÚÑa-záéíóúñ\s.]+?)(?:\s*(?:Rec|Mun|Dep|\d))`),
		reRecinto:      regexp.MustCompile(`(?i)Rec[i1]nt[o0][:\s]+([A-ZÁÉÍÓÚÑa-záéíóúñ\s.]+?)(?:\s*\d|\s*$)`),

		// Votos totales
		reVotosValidos: regexp.MustCompile(`(?i)VOTOS\s+V[ÁA]L[I1]D[OÓ0]S[:\s]*(\d[\d\s]{0,8}\d)`),
		reVotosBlancos: regexp.MustCompile(`(?i)VOTOS\s+BLANC[OÓ0]S[:\s]*(\d[\d\s]{0,8}\d)`),
		reVotosNulos:   regexp.MustCompile(`(?i)VOTOS\s+NUL[OÓ0]S[:\s]*(\d[\d\s]{0,8}\d)`),
	}
}

// ParseActaText parsea el texto y retorna un ActaRRV poblado.
// Detecta automáticamente si el texto viene de mutool (embebido) o Tesseract (OCR).
func (p *ActaParser) ParseActaText(text string, fileName string) *models.ActaRRV {
	acta := &models.ActaRRV{
		FechaRecepcion: time.Now(),
		Fuente:         models.FuenteRRV,
		// TipoEntrada se establece en el pipeline según la extensión del archivo
	}

	var errores []string

	// Detectar acta ANULADA — solo "ANULAD" (prefijo de "ANULADA"/"ANULADO").
	// NO usar "NULO" ni "NULA" solos: ambas palabras aparecen en el formulario
	// impreso como "VOTOS NULOS" y dispararían un falso positivo en cada acta.
	upperText := strings.ToUpper(text)
	if strings.Contains(upperText, "ANULAD") {
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

	if len(cleanLines) < 3 {
		*errores = append(*errores, "texto embebido insuficiente")
		return
	}

	// Las primeras líneas son la ubicación geográfica
	acta.Departamento = cleanLines[0]
	acta.Provincia = cleanLines[1]
	acta.Municipio = cleanLines[2]
	if len(cleanLines) > 3 {
		acta.Recinto = cleanLines[3]
	}
	if len(cleanLines) > 4 {
		// Línea 4 suele ser la dirección/nombre del recinto (más descriptiva)
		if len(cleanLines[4]) > len(acta.Recinto) {
			acta.Recinto = cleanLines[4]
		}
	}

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
// PARSER PARA TEXTO OCR (Tesseract) — PDFs escaneados e imágenes de cámara
//
// Estrategia dual:
//   1. PRIMARIA: detección posicional por tripletes "d d d"
//      → No depende de nombres de candidatos; funciona con cualquier acta real.
//      → Los últimos 7 tripletes = [C1,C2,C3,C4, Válidos, Blancos, Nulos]
//   2. FALLBACK: regex + nombres conocidos (solo para datos de test)
// ═══════════════════════════════════════════════════════════════════

func (p *ActaParser) parsearTextoOCR(text string, acta *models.ActaRRV, errores *[]string) {
	text = limitarTextoAlCuerpoActa(text)

	// ── Campos de ubicación ───────────────────────────────────────
	acta.CodigoMesa = p.extraerCodigoMesa(text)
	acta.Mesa = p.extraerNumeroMesa(text)

	// Estrategia primaria: regex (funciona si label y valor están en la misma línea)
	acta.Departamento = p.extraerCampoUbicacion(text, p.reDepartamento)
	acta.Provincia = p.extraerCampoUbicacion(text, p.reProvincia)
	acta.Municipio = p.extraerCampoUbicacion(text, p.reMunicipio)
	acta.Recinto = p.extraerRecinto(text)

	// Estrategia secundaria: buscar valor en la línea siguiente al label
	// (Tesseract en PSM 3 suele separar label y valor en líneas distintas)
	if acta.Departamento == "" {
		acta.Departamento = p.extraerCampoSiguienteLinea(text, `(?i)Dep[a4]rt[a4]m`)
	}
	if acta.Provincia == "" {
		acta.Provincia = p.extraerCampoSiguienteLinea(text, `(?i)Prov[i1]nc`)
	}
	if acta.Municipio == "" {
		acta.Municipio = p.extraerCampoSiguienteLinea(text, `(?i)Mun[i1]c[i1]p`)
	}
	if acta.Recinto == "" {
		acta.Recinto = p.extraerCampoSiguienteLinea(text, `(?i)Rec[i1]nt`)
	}

	// Fallback final para departamento: buscar nombre boliviano conocido en el texto
	if acta.Departamento == "" {
		acta.Departamento = extraerDepartamentoDesdeTexto(text)
	}

	// ── MÉTODO 1: Tripletes posicionales (actas reales) ───────────
	if p.parsearPorTriplets(text, acta) {
		return
	}

	// ── MÉTODO 2: Regex explícita de totales ─────────────────────
	acta.VotosValidos = p.extraerVotos(text, p.reVotosValidos)
	acta.VotosBlancos = p.extraerVotos(text, p.reVotosBlancos)
	acta.VotosNulos = p.extraerVotos(text, p.reVotosNulos)

	// ── MÉTODO 3: Candidatos por nombre (datos de test) ───────────
	candidatos := p.extraerCandidatos(text)
	acta.Candidatos = candidatos
	if len(candidatos) == 0 {
		*errores = append(*errores, "no se encontraron candidatos en OCR")
	}
}

// parsearPorTriplets extrae votos usando la posición relativa de los tripletes "d d d".
//
// Las actas bolivianas tienen los valores numéricos como 3 dígitos separados por espacios:
//
//	"0 8 5" = 085 = 85 votos.
//
// Estructura fija de los últimos tripletes:
//
//	[-N..-5]: datos administrativos (hora inicio/fin, electores, papeletas)
//	[-4]:     candidato 1
//	[-3..pero hay 4 candidatos en el PDF real]
//	[...] :   candidatos (variable, típicamente 4-6)
//	[-3]:     votos válidos
//	[-2]:     votos blancos
//	[-1]:     votos nulos
//
// Si hay >= 7 tripletes, los últimos 7 siempre son [c1,c2,c3,c4, val,bla,nul].
func (p *ActaParser) parsearPorTriplets(text string, acta *models.ActaRRV) bool {
	// Patrón amplio: un dígito (o sustituto) seguido de 1-4 espacios, repetido 3 veces.
	// Acepta tanto "0 8 5" como "0  8  5".
	reTriplete := regexp.MustCompile(
		`(?:^|[ \t])([0-9OolIBbSsZzGg|])[ \t]{0,4}([0-9OolIBbSsZzGg|])[ \t]{0,4}([0-9OolIBbSsZzGg|])(?:[ \t]|$)`,
	)
	// Número de 3 dígitos al final de línea (para líneas casi-numéricas)
	reNumFin := regexp.MustCompile(`(?:^|\s)(\d{3})(?:\s*$)`)
	// Número de 3 dígitos al final de una línea que contiene texto (ej: "Daenerys Targaryen 215")
	// Solo captura si hay exactamente un grupo de 3+ dígitos al final y la línea no es larga
	reNumFinTexto := regexp.MustCompile(`\b(\d{3})\s*$`)

	lines := strings.Split(text, "\n")
	var triplets []int

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || len(line) > 120 {
			continue
		}

		// Intento 1: patrón espaciado "d d d"
		if m := reTriplete.FindStringSubmatch(line); len(m) == 4 {
			val := charToDigit(m[1])*100 + charToDigit(m[2])*10 + charToDigit(m[3])
			triplets = append(triplets, val)
			continue
		}

		alphaCount := 0
		for _, ch := range line {
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') {
				alphaCount++
			}
		}

		// Intento 2: número de 3 dígitos al final de una línea casi-solo-numérica.
		if alphaCount <= 2 {
			if m := reNumFin.FindStringSubmatch(line); len(m) == 2 {
				n, err := strconv.Atoi(m[1])
				if err == nil && n >= 0 && n <= 999 {
					triplets = append(triplets, n)
					continue
				}
			}
		}

		// Intento 3: número de 3 dígitos al final de línea con texto (candidato + votos).
		// Aplica solo a líneas cortas-medianas (< 60 chars) que terminan en exactamente 3 dígitos
		// y no son el código de mesa (que tiene 13+ dígitos).
		// Evita líneas que contienen más de un grupo numérico grande (ej: timestamps, códigos).
		if alphaCount >= 3 && len(line) <= 60 {
			if m := reNumFinTexto.FindStringSubmatch(line); len(m) == 2 {
				n, err := strconv.Atoi(m[1])
				if err == nil && n >= 0 && n <= 999 {
					triplets = append(triplets, n)
					continue
				}
			}
		}
	}

	if len(triplets) < 7 {
		return false
	}

	n := len(triplets)

	// Últimos 3: totales
	acta.VotosValidos = triplets[n-3]
	acta.VotosBlancos = triplets[n-2]
	acta.VotosNulos = triplets[n-1]

	// Ante los últimos 3: candidatos (los 4 anteriores a los totales)
	numCandidatos := 4
	if n-3 < numCandidatos {
		numCandidatos = n - 3
	}
	acta.Candidatos = make([]models.Candidato, numCandidatos)
	for i := 0; i < numCandidatos; i++ {
		acta.Candidatos[i] = models.Candidato{
			CandidatoID: fmt.Sprintf("C%d", i+1),
			Votos:       triplets[n-3-numCandidatos+i],
		}
	}

	// Datos adicionales si hay suficientes tripletes (electores, papeletas)
	if n >= 10 {
		acta.ElectoresHabilitados = triplets[n-10]
		acta.PapeletasAnfora = triplets[n-9]
		acta.PapeletasNoUtilizadas = triplets[n-8]
	}

	return true
}

// ═══════════════════════════════════════════════════════════════════
// Funciones auxiliares de extracción (para OCR)
// ═══════════════════════════════════════════════════════════════════

// limitarTextoAlCuerpoActa elimina ruido capturado fuera del acta (tabs del
// navegador, visor PDF, nombres de archivo y barras del sistema) antes de parsear.
func limitarTextoAlCuerpoActa(text string) string {
	lines := strings.Split(text, "\n")
	start := -1

	for i, line := range lines {
		norm := normalizarLineaOCR(line)
		if strings.Contains(norm, "ACTA ELECTORAL") ||
			strings.Contains(norm, "ESCRUTINIO Y CONTEO") ||
			strings.Contains(norm, "ELECCION DE AUTORIDADES") ||
			strings.Contains(norm, "UBICACION DE LA MESA") ||
			strings.Contains(norm, "CONTEO DE VOTOS") {
			start = maxIntParser(0, i-1)
			break
		}
	}

	if start < 0 {
		return text
	}

	end := len(lines)
	for i := start; i < len(lines); i++ {
		norm := normalizarLineaOCR(lines[i])
		if strings.Contains(norm, "DELEGADOS DE MESA") ||
			strings.Contains(norm, "FIRMA") ||
			strings.Contains(norm, "OBSERVACIONES") {
			end = i + 1
			break
		}
	}

	return strings.Join(lines[start:end], "\n")
}

func normalizarLineaOCR(s string) string {
	s = strings.ToUpper(s)
	replacer := strings.NewReplacer(
		"Á", "A", "É", "E", "Í", "I", "Ó", "O", "Ú", "U", "Ñ", "N",
		"Ã", "A", "Ã‰", "E", "Ã", "I", "Ã“", "O", "Ãš", "U", "Ã‘", "N",
	)
	return replacer.Replace(s)
}

func maxIntParser(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (p *ActaParser) extraerCodigoMesa(text string) string {
	matches := p.reCodigoMesa.FindStringSubmatch(text)
	if len(matches) > 1 {
		return limpiarNumero(matches[1])
	}
	// Fallback: buscar número de 13–16 dígitos en cualquier línea del documento
	re13 := regexp.MustCompile(`\b(\d{13,16})\b`)
	for _, line := range strings.Split(text, "\n") {
		if m := re13.FindStringSubmatch(line); len(m) > 1 {
			return m[1]
		}
	}
	// Buscar en todo el texto como último recurso (por si OCR pega líneas)
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

// extraerCampoSiguienteLinea busca un label (por regex) y captura el texto
// en la misma línea después de ":" o en la siguiente línea no numérica.
// Maneja el caso OCR más común en actas de cámara: label y valor en líneas separadas.
func (p *ActaParser) extraerCampoSiguienteLinea(text, labelPattern string) string {
	reLbl := regexp.MustCompile(labelPattern)
	reJustNum := regexp.MustCompile(`^\d[\d\s]*$`)
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if !reLbl.MatchString(line) {
			continue
		}
		// Puede haber el valor después del ":" en la misma línea
		if idx := strings.Index(line, ":"); idx >= 0 {
			after := strings.TrimSpace(line[idx+1:])
			if len(after) > 2 && !reJustNum.MatchString(after) {
				return after
			}
		}
		// Buscar en las siguientes 2 líneas (a veces hay una línea vacía intermedia)
		for j := i + 1; j <= i+2 && j < len(lines); j++ {
			next := strings.TrimSpace(lines[j])
			if len(next) > 2 && !reJustNum.MatchString(next) && !reLbl.MatchString(next) {
				// Excluir líneas que parecen otros labels (contienen mucho texto con mayúscula)
				if len(next) < 60 {
					return next
				}
			}
		}
	}
	return ""
}

// extraerRecinto intenta extraer el recinto electoral con varias estrategias:
//  1. Regex en la misma línea: "Recinto: Nombre"
//  2. Valor en la línea siguiente al label "Recinto"
//  3. Variantes OCR del label ("Recinto|Reclnto|Reclnto" etc.)
func (p *ActaParser) extraerRecinto(text string) string {
	// Estrategia 1: mismo renglón
	if v := p.extraerCampoUbicacion(text, p.reRecinto); v != "" {
		return v
	}
	// Estrategia 2: valor en línea siguiente al label
	reLabel := regexp.MustCompile(`(?i)R[eE][cC][iIlL1][nN][tT][oO0]`)
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if reLabel.MatchString(line) {
			// Puede haber el valor en la misma línea después de ":"
			after := reLabel.ReplaceAllString(line, "")
			after = strings.TrimSpace(strings.TrimLeft(after, ":. "))
			if len(after) > 2 {
				return after
			}
			// O en la siguiente línea
			if i+1 < len(lines) {
				next := strings.TrimSpace(lines[i+1])
				// La siguiente línea no debe ser un número ni otro label
				reJustNum := regexp.MustCompile(`^\d+$`)
				if len(next) > 2 && !reJustNum.MatchString(next) {
					return next
				}
			}
		}
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

// departamentosBolivianos es la lista oficial de departamentos.
var departamentosBolivianos = []string{
	"Chuquisaca", "La Paz", "Cochabamba", "Oruro",
	"Potosí", "Tarija", "Santa Cruz", "Beni", "Pando",
}

// extraerDepartamentoDesdeTexto busca nombres de departamentos bolivianos directamente
// en el texto OCR cuando la etiqueta "Departamento:" no está presente.
func extraerDepartamentoDesdeTexto(text string) string {
	upper := strings.ToUpper(text)
	for _, dep := range departamentosBolivianos {
		if strings.Contains(upper, strings.ToUpper(dep)) {
			return dep
		}
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
	reJustNum := regexp.MustCompile(`^\d[\d\s]*$`)

	for i, line := range lines {
		lineLower := strings.ToLower(line)
		if !strings.Contains(lineLower, apellido) {
			if !p.containsFuzzy(lineLower, apellido) {
				continue
			}
		}
		// Intento 1: el número está al final de la misma línea que el nombre
		votos := p.extraerUltimoNumero(line)
		if votos > 0 {
			return votos
		}
		// Intento 2: el número está en la línea siguiente (OCR separa label de valor)
		// Busca en las 2 líneas siguientes no vacías
		for j := i + 1; j <= i+3 && j < len(lines); j++ {
			next := strings.TrimSpace(lines[j])
			if next == "" {
				continue
			}
			// La siguiente línea debe parecer numérica (pocos alfa)
			if reJustNum.MatchString(next) || len(next) <= 10 {
				votos = p.extraerUltimoNumero(next)
				if votos > 0 {
					return votos
				}
			}
			break // si la línea tiene texto largo, no es el número de este candidato
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
//
//	O,o → 0 | l,I → 1 | B → 8 | S → 5 | Z → 2 | G → 6
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
