package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"rrv-backend/internal/config"
	"rrv-backend/internal/models"
	"rrv-backend/internal/repository"
	"rrv-backend/internal/service"
)

// ResultadoActa almacena el resultado del procesamiento de un PDF.
type ResultadoActa struct {
	Archivo      string
	CodigoMesa   string
	Mesa         string
	Departamento string
	Municipio    string
	Recinto      string
	Candidatos   map[string]int
	VotosValidos int
	VotosBlancos int
	VotosNulos   int
	TotalVotos   int
	Estado       string
	MotivoEstado string
	Errores      []string
	Anulada      bool
	TextoRaw     string
	// Validación visual
	LapizDetectado     bool
	ManchaDetectada    bool
	CorrectorDetectado bool
	HuellasDetectadas  int
	FirmasSuficientes  bool
}

func main() {
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║     OCR Processor — Actas Electorales RRV                  ║")
	fmt.Println("║     Tesseract OCR + mutool + Validación Visual             ║")
	fmt.Println("║     Sistema de Recuento Rápido de Votos — Bolivia          ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Determinar directorio de PDFs
	pdfDir := "pdf"
	if len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], "-") {
		pdfDir = os.Args[1]
	}

	// Flags opcionales
	verbose := false
	debugMode := false
	skipVisual := false
	maxFiles := 0 // 0 = todos
	for _, arg := range os.Args[1:] {
		if arg == "--verbose" || arg == "-v" {
			verbose = true
		}
		if arg == "--debug" || arg == "-d" {
			debugMode = true
		}
		if arg == "--skip-visual" {
			skipVisual = true
		}
		if strings.HasPrefix(arg, "--max=") {
			fmt.Sscanf(arg, "--max=%d", &maxFiles)
		}
	}

	// Verificar que el directorio existe
	if info, err := os.Stat(pdfDir); err != nil || !info.IsDir() {
		log.Fatalf("[ERROR] Directorio de PDFs no encontrado: %s\n"+
			"Uso: go run cmd/ocr_processor/main.go [directorio_pdfs] [--verbose] [--debug] [--max=N] [--skip-visual]", pdfDir)
	}

	// Inicializar servicio OCR
	fmt.Println("[INIT] Verificando dependencias externas...")
	ocrService, err := service.NewRealOCRService("", "", "spa")
	if err != nil {
		log.Fatalf("[ERROR] %v", err)
	}
	fmt.Println("[OK] Tesseract y mutool encontrados")

	// Inicializar validador visual
	var visualValidator *service.VisualValidator
	if !skipVisual {
		absDir, _ := filepath.Abs(".")
		mutoolPath := filepath.Join(absDir, "mutool.exe")
		if _, err := os.Stat(mutoolPath); os.IsNotExist(err) {
			mutoolPath = "mutool"
		}
		visualValidator = service.NewVisualValidator(mutoolPath)
		fmt.Println("[OK] Validador visual inicializado (detección de lápiz, manchas, tinta)")
	} else {
		fmt.Println("[INFO] Validación visual desactivada (--skip-visual)")
	}

	parser := service.NewActaParser()
	actaService := service.NewActaService()

	// ═══ Conexión a MongoDB ═══
	fmt.Println("[INIT] Conectando a MongoDB...")
	cfg := config.LoadConfig()

	// Mostrar a qué MongoDB se está conectando para detectar errores de configuración
	uriDisplay := cfg.MongoURI
	if len(uriDisplay) > 50 {
		uriDisplay = uriDisplay[:50] + "..."
	}
	fmt.Printf("[INFO] URI: %s\n", uriDisplay)
	fmt.Printf("[INFO] Base de datos: %s\n", cfg.MongoDB)
	if cfg.MongoURI == "mongodb://localhost:27017" {
		fmt.Println("[WARN] ⚠️  Usando MongoDB LOCAL (localhost). Si quieres Atlas, ejecuta con run_ocr.ps1")
	}

	mongoClient, err := mongo.Connect(context.Background(), options.Client().ApplyURI(cfg.MongoURI))
	if err != nil {
		log.Fatalf("[ERROR] No se pudo conectar a MongoDB: %v", err)
	}
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := mongoClient.Ping(pingCtx, nil); err != nil {
		pingCancel()
		log.Fatalf("[ERROR] MongoDB no responde: %v", err)
	}
	pingCancel()
	defer mongoClient.Disconnect(context.Background())
	db := mongoClient.Database(cfg.MongoDB)

	actaValidaRepo := repository.NewActaRepositoryForCollection(db, "Actas")
	actaAnuladaRepo := repository.NewActaRepositoryForCollection(db, "Actas_Anuladas")
	actaRepo := repository.NewActaRepository(db) // actas_rrv — usado solo por inconsistenciaService
	inconsistenciaRepo := repository.NewInconsistenciaRepository(db)
	referenciaRepo := repository.NewReferenciaRepository(db)
	inconsistenciaService := service.NewInconsistenciaService(inconsistenciaRepo, referenciaRepo, actaRepo)
	fmt.Printf("[OK] Conectado a MongoDB: %s\n", cfg.MongoDB)
	fmt.Println("[OK] Colecciones: válidas→Actas | anuladas→Actas_Anuladas")

	// Buscar todos los PDFs
	pdfs, err := filepath.Glob(filepath.Join(pdfDir, "*.pdf"))
	if err != nil {
		log.Fatalf("[ERROR] Error buscando PDFs: %v", err)
	}

	if len(pdfs) == 0 {
		log.Fatalf("[ERROR] No se encontraron archivos PDF en: %s", pdfDir)
	}

	if maxFiles > 0 && maxFiles < len(pdfs) {
		pdfs = pdfs[:maxFiles]
		fmt.Printf("[INFO] Limitado a %d PDFs (de %d encontrados)\n", maxFiles, len(pdfs))
	}

	fmt.Printf("[INFO] Encontrados %d PDFs en %s\n", len(pdfs), pdfDir)
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println("  PROCESANDO ACTAS...")
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println()

	// Procesar cada PDF
	startTime := time.Now()
	resultados := make([]ResultadoActa, 0, len(pdfs))
	var exitosas, conErrores, anuladas, observadas, fallidas int
	var anuladasLapiz, anuladasTexto, anuladasCorrector, anuladasFirmas, anuladasVotos int
	var observadasMancha int
	var inconsistenciasGuardadas, actasGuardadas int

	// Contadores globales de votos (solo de actas PROCESADAS)
	votosTotalesPorCandidato := map[string]int{
		"Daenerys Targaryen": 0,
		"Sansa Stark":        0,
		"Robert Baratheon":   0,
		"Tyrion Lannister":   0,
	}
	var totalValidos, totalBlancos, totalNulos int

	candidatoNames := []string{"Daenerys Targaryen", "Sansa Stark", "Robert Baratheon", "Tyrion Lannister"}

	for i, pdfPath := range pdfs {
		fileName := filepath.Base(pdfPath)
		fmt.Printf("[%3d/%d] %-40s ", i+1, len(pdfs), fileName)

		// ═══ PASO 1: Extracción de texto ═══
		rawText, err := ocrService.ProcesarPDF(pdfPath)
		if err != nil {
			fmt.Printf("❌ ERROR OCR: %v\n", err)
			fallidas++
			resultados = append(resultados, ResultadoActa{
				Archivo: fileName,
				Estado:  "FALLO_OCR",
				Errores: []string{err.Error()},
			})
			continue
		}

		// Debug: guardar texto raw
		if debugMode {
			debugDir := filepath.Join(pdfDir, "debug_ocr")
			os.MkdirAll(debugDir, 0755)
			debugFile := filepath.Join(debugDir, strings.TrimSuffix(fileName, ".pdf")+".txt")
			os.WriteFile(debugFile, []byte(rawText), 0644)
		}

		// ═══ PASO 2: Parsear texto a estructura ═══
		acta := parser.ParseActaText(rawText, fileName)

		// ═══ PASO 3: Validación visual (lápiz, manchas, tinta) ═══
		if visualValidator != nil {
			validacionVisual, errVisual := visualValidator.ValidarImagenActa(pdfPath)
			if errVisual != nil {
				if verbose {
					fmt.Printf("\n         ⚠️  Error análisis visual: %v\n         ", errVisual)
				}
			} else {
				acta.ValidacionVisual = validacionVisual
			}
		}

		// ═══ PASO 4: Validación de datos ═══
		erroresValidacion := actaService.ValidarActa(acta)

		// Combinar errores del parser + validación
		todosErrores := append(acta.Errores, erroresValidacion...)
		acta.Errores = todosErrores

		// ═══ PASO 5: Determinar estado final ═══
		acta.Estado = actaService.DeterminarEstado(acta, todosErrores)

		// ═══ PASO 6: Construir resultado ═══
		resultado := ResultadoActa{
			Archivo:      fileName,
			CodigoMesa:   acta.CodigoMesa,
			Mesa:         acta.Mesa,
			Departamento: acta.Departamento,
			Municipio:    acta.Municipio,
			Recinto:      acta.Recinto,
			Candidatos:   make(map[string]int),
			VotosValidos: acta.VotosValidos,
			VotosBlancos: acta.VotosBlancos,
			VotosNulos:   acta.VotosNulos,
			TotalVotos:   acta.TotalVotos,
			Estado:       acta.Estado,
			MotivoEstado: acta.MotivoEstado,
			Errores:      acta.Errores,
			Anulada:      acta.Estado == models.EstadoAnulada,
			TextoRaw:     rawText,
		}

		if acta.ValidacionVisual != nil {
			resultado.LapizDetectado = acta.ValidacionVisual.LapizDetectado
			resultado.ManchaDetectada = acta.ValidacionVisual.ManchaDetectada
			resultado.CorrectorDetectado = acta.ValidacionVisual.CorrectorDetectado
			resultado.HuellasDetectadas = acta.ValidacionVisual.HuellasDetectadas
			resultado.FirmasSuficientes = acta.ValidacionVisual.FirmasSuficientes
		}

		for j, c := range acta.Candidatos {
			if j < len(candidatoNames) {
				resultado.Candidatos[candidatoNames[j]] = c.Votos
			}
		}

		resultados = append(resultados, resultado)

		// ═══ PASO 7: Persistir en MongoDB ═══
		opCtx := context.Background()

		// Asegurar ActaID no vacío
		if acta.ActaID == "" {
			acta.ActaID = "MESA-" + strings.TrimSuffix(fileName, ".pdf")
		}
		// Calcular hash del archivo si no fue asignado por el parser
		if acta.HashOrigen == "" {
			if fileBytes, readErr := os.ReadFile(pdfPath); readErr == nil {
				h := sha256.Sum256(fileBytes)
				acta.HashOrigen = hex.EncodeToString(h[:])
			}
		}
		acta.Fuente = models.FuenteRRV
		acta.TipoEntrada = models.TipoPDF
		if acta.FechaRecepcion.IsZero() {
			acta.FechaRecepcion = time.Now()
		}

		// Poblar campo Observaciones para actas OBSERVADAS
		if acta.Estado == models.EstadoObservada {
			var obs []string
			if acta.ValidacionVisual != nil && len(acta.ValidacionVisual.Observaciones) > 0 {
				obs = append(obs, acta.ValidacionVisual.Observaciones...)
			}
			for _, e := range acta.Errores {
				obs = append(obs, e)
			}
			acta.Observaciones = obs
		}

		// Detectar y registrar inconsistencias en Logs_Inconsistencias
		incs := inconsistenciaService.DetectarYRegistrar(opCtx, acta, "OCR_PROCESSOR")
		inconsistenciasGuardadas += len(incs)

		// Enrutar a la colección correcta según estado
		if acta.Estado == models.EstadoAnulada {
			// ANULADA → Actas_Anuladas (no va a Actas)
			if saveErr := actaAnuladaRepo.InsertActa(opCtx, acta); saveErr != nil {
				if verbose {
					fmt.Printf("\n         [WARN] No guardada en Actas_Anuladas: %v\n         ", saveErr)
				}
			} else {
				actasGuardadas++
			}
		} else {
			// PROCESADA, OBSERVADA, ERROR → Actas
			if saveErr := actaValidaRepo.InsertActa(opCtx, acta); saveErr != nil {
				if verbose {
					fmt.Printf("\n         [WARN] No guardada en Actas: %v\n         ", saveErr)
				}
			} else {
				actasGuardadas++
			}
		}

		// ═══ PASO 8: Imprimir resultado ═══
		if verbose && len(incs) > 0 {
			fmt.Printf("\n         📋 %d inconsistencia(s) guardada(s) en Logs_Inconsistencias\n         ", len(incs))
		}
		switch acta.Estado {
		case models.EstadoAnulada:
			anuladas++
			switch acta.MotivoEstado {
			case models.MotivoLapiz:
				fmt.Printf("🖊️  ANULADA (LÁPIZ)")
				anuladasLapiz++
			case models.MotivoCorrector:
				fmt.Printf("❄️  ANULADA (CORRECTOR)")
				anuladasCorrector++
			case models.MotivoFaltaFirmas:
				fmt.Printf("✋ ANULADA (FIRMAS: %d huellas)", acta.ValidacionVisual.HuellasDetectadas)
				anuladasFirmas++
			case models.MotivoInconsistenciaTotal:
				fmt.Printf("🚨 ANULADA (VOTOS>VOTANTES)")
				anuladasVotos++
			case models.MotivoTextoAnulada:
				fmt.Printf("❌ ANULADA (TEXTO)")
				anuladasTexto++
			default:
				fmt.Printf("❌ ANULADA (%s)", acta.MotivoEstado)
				anuladasTexto++
			}

		case models.EstadoObservada:
			observadas++
			if acta.ValidacionVisual != nil && acta.ValidacionVisual.ManchaDetectada {
				fmt.Printf("☕ OBSERVADA (MANCHA %.1f%%)", acta.ValidacionVisual.PorcentajeMancha)
				observadasMancha++
			} else {
				fmt.Printf("⚠️  OBSERVADA")
			}

		case models.EstadoProcesada:
			fmt.Printf("✅ OK")
			exitosas++
			// Acumular votos solo de actas exitosas
			for j, c := range acta.Candidatos {
				if j < len(candidatoNames) {
					votosTotalesPorCandidato[candidatoNames[j]] += c.Votos
				}
			}
			totalValidos += acta.VotosValidos
			totalBlancos += acta.VotosBlancos
			totalNulos += acta.VotosNulos

		default: // ERROR
			fmt.Printf("⚠️  ERRORES(%d)", len(acta.Errores))
			conErrores++
		}

		// Mostrar info de tinta si verbose
		if verbose {
			fmt.Printf(" | Mesa:%s Dep:%s", acta.Mesa, acta.Departamento)
			if acta.ValidacionVisual != nil {
				fmt.Printf(" Int:%.0f Ctr:%.2f Huellas:%d",
					acta.ValidacionVisual.IntensidadPromedio,
					acta.ValidacionVisual.RatioContraste,
					acta.ValidacionVisual.HuellasDetectadas)
			}
			if acta.ElectoresHabilitados > 0 {
				fmt.Printf(" Elect:%d", acta.ElectoresHabilitados)
			}
			for j, c := range acta.Candidatos {
				if j < len(candidatoNames) {
					shortName := strings.Split(candidatoNames[j], " ")[1]
					fmt.Printf(" %s:%d", shortName, c.Votos)
				}
			}
		}
		fmt.Println()

		// En modo verbose, mostrar errores
		if verbose && len(acta.Errores) > 0 {
			for _, e := range acta.Errores {
				fmt.Printf("         ↳ %s\n", e)
			}
		}
	}

	elapsed := time.Since(startTime)

	// ═══════════════════════════════════════════
	// RESUMEN FINAL
	// ═══════════════════════════════════════════
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    RESUMEN DE PROCESAMIENTO                ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	fmt.Printf("  Total PDFs procesados:  %d\n", len(pdfs))
	fmt.Printf("  ✅ Procesadas:          %d (%.1f%%)\n", exitosas, pct(exitosas, len(pdfs)))
	fmt.Printf("  ❌ Anuladas:            %d (%.1f%%)\n", anuladas, pct(anuladas, len(pdfs)))
	if anuladasTexto > 0 {
		fmt.Printf("     ↳ Por texto ANULADA: %d\n", anuladasTexto)
	}
	if anuladasLapiz > 0 {
		fmt.Printf("     ↳ Por uso de lápiz:  %d\n", anuladasLapiz)
	}
	if anuladasCorrector > 0 {
		fmt.Printf("     ↳ Por corrector:     %d\n", anuladasCorrector)
	}
	if anuladasFirmas > 0 {
		fmt.Printf("     ↳ Por falta firmas:  %d\n", anuladasFirmas)
	}
	if anuladasVotos > 0 {
		fmt.Printf("     ↳ Votos > Votantes:  %d\n", anuladasVotos)
	}
	fmt.Printf("  ⚠️  Observadas:         %d (%.1f%%)\n", observadas, pct(observadas, len(pdfs)))
	if observadasMancha > 0 {
		fmt.Printf("     ↳ Por manchas:       %d\n", observadasMancha)
	}
	fmt.Printf("  ⚠️  Con errores:        %d (%.1f%%)\n", conErrores, pct(conErrores, len(pdfs)))
	fmt.Printf("  💀 Fallo OCR:           %d (%.1f%%)\n", fallidas, pct(fallidas, len(pdfs)))
	fmt.Printf("  Tiempo total:           %s\n", elapsed.Round(time.Second))
	if len(pdfs) > 0 {
		fmt.Printf("  Tiempo promedio/acta:   %s\n", (elapsed / time.Duration(len(pdfs))).Round(time.Millisecond))
	}
	fmt.Printf("  📦 Actas guardadas en MongoDB:       %d\n", actasGuardadas)
	fmt.Printf("  📋 Inconsistencias en Logs_Inconsistencias: %d\n", inconsistenciasGuardadas)
	fmt.Println()

	// Tabla de votos por candidato (solo actas procesadas)
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║     RESULTADOS ELECTORALES (solo actas PROCESADAS)         ║")
	fmt.Println("╠══════════════════════════════════════════════════════════════╣")
	fmt.Println("║  Candidato                  │  Votos    │  Porcentaje      ║")
	fmt.Println("╠═════════════════════════════╪═══════════╪══════════════════╣")

	for _, nombre := range candidatoNames {
		votos := votosTotalesPorCandidato[nombre]
		pctVal := float64(0)
		if totalValidos > 0 {
			pctVal = float64(votos) / float64(totalValidos) * 100
		}
		bar := generarBarra(pctVal, 10)
		fmt.Printf("║  %-26s │ %7d   │ %5.1f%% %s ║\n", nombre, votos, pctVal, bar)
	}
	fmt.Println("╠═════════════════════════════╪═══════════╪══════════════════╣")
	fmt.Printf("║  %-26s │ %7d   │                  ║\n", "Votos Válidos", totalValidos)
	fmt.Printf("║  %-26s │ %7d   │                  ║\n", "Votos Blancos", totalBlancos)
	fmt.Printf("║  %-26s │ %7d   │                  ║\n", "Votos Nulos", totalNulos)
	fmt.Printf("║  %-26s │ %7d   │                  ║\n", "TOTAL", totalValidos+totalNulos)
	fmt.Println("╚═════════════════════════════╧═══════════╧══════════════════╝")
	fmt.Println()

	// Tabla por departamento
	depVotos := map[string]int{}
	depActas := map[string]int{}
	for _, r := range resultados {
		if r.Departamento != "" && r.Estado == models.EstadoProcesada {
			depVotos[r.Departamento] += r.TotalVotos
			depActas[r.Departamento]++
		}
	}

	if len(depVotos) > 0 {
		fmt.Println("═══ Actas Procesadas por Departamento ═══")
		for dep, count := range depActas {
			fmt.Printf("  %-20s: %d actas, %d votos totales\n", dep, count, depVotos[dep])
		}
		fmt.Println()
	}

	// ═══ Tabla de actas anuladas ═══
	fmt.Println("═══ Detalle de Actas ANULADAS ═══")
	shownAnuladas := 0
	for _, r := range resultados {
		if r.Estado == models.EstadoAnulada {
			motivo := r.MotivoEstado
			if motivo == "" {
				motivo = "DESCONOCIDO"
			}
			icon := "❌"
			if r.LapizDetectado {
				icon = "🖊️"
			}
			fmt.Printf("  %s %s (Mesa: %s) — Motivo: %s\n", icon, r.Archivo, r.Mesa, motivo)
			shownAnuladas++
		}
	}
	if shownAnuladas == 0 {
		fmt.Println("  (ninguna)")
	}
	fmt.Println()

	// ═══ Tabla de actas observadas ═══
	fmt.Println("═══ Detalle de Actas OBSERVADAS ═══")
	shownObservadas := 0
	for _, r := range resultados {
		if r.Estado == models.EstadoObservada {
			icon := "⚠️"
			if r.ManchaDetectada {
				icon = "☕"
			}
			fmt.Printf("  %s %s (Mesa: %s) — Motivo: %s\n", icon, r.Archivo, r.Mesa, r.MotivoEstado)
			shownObservadas++
		}
	}
	if shownObservadas == 0 {
		fmt.Println("  (ninguna)")
	}
	fmt.Println()

	// Mostrar primeras actas con errores para debugging
	fmt.Println("═══ Detalle de Actas con Errores (primeras 10) ═══")
	shown := 0
	for _, r := range resultados {
		if shown >= 10 {
			break
		}
		if len(r.Errores) > 0 && r.Estado == models.EstadoError {
			fmt.Printf("  📄 %s (Estado: %s)\n", r.Archivo, r.Estado)
			for _, e := range r.Errores {
				fmt.Printf("     ↳ %s\n", e)
			}
			shown++
		}
	}
	if shown == 0 {
		fmt.Println("  (ninguna)")
	}

	fmt.Println()
	fmt.Println("═══ Procesamiento completado ═══")
}

// pct calcula porcentaje de forma segura.
func pct(part, total int) float64 {
	if total == 0 {
		return 0
	}
	return float64(part) / float64(total) * 100
}

// generarBarra genera una barra visual de progreso.
func generarBarra(porcentaje float64, maxLen int) string {
	filled := int(porcentaje / 100 * float64(maxLen))
	if filled > maxLen {
		filled = maxLen
	}
	if filled < 0 {
		filled = 0
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", maxLen-filled)
}
