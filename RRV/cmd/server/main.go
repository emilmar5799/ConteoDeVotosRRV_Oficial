package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"rrv-backend/internal/config"
	"rrv-backend/internal/handlers"
	"rrv-backend/internal/repository"
	"rrv-backend/internal/service"
)

func main() {
	// Banner
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║        Sistema RRV - Recuento Rápido de Votos              ║")
	fmt.Println("║        Pipeline TREP Preliminar — Bolivia                  ║")
	fmt.Println("║        Responsable: Jesús Espejo                           ║")
	fmt.Println("║                                                            ║")
	fmt.Println("║  Patrones: CQRS | Event Sourcing | Idempotencia | Retry    ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// 1. Cargar configuración
	cfg := config.LoadConfig()
	log.Printf("[CONFIG] MongoDB URI: %s", cfg.MongoURI)
	log.Printf("[CONFIG] MongoDB DB: %s", cfg.MongoDB)
	log.Printf("[CONFIG] Puerto: %s", cfg.Port)
	log.Printf("[CONFIG] Teléfonos autorizados: %v", cfg.TelefonosAutorizados)

	// Log Twilio config
	if cfg.TwilioAccountSID != "" {
		log.Printf("[CONFIG] Twilio SID: %s...%s", cfg.TwilioAccountSID[:8], cfg.TwilioAccountSID[len(cfg.TwilioAccountSID)-4:])
		log.Printf("[CONFIG] Twilio Phone: %s", cfg.TwilioPhoneNumber)
		log.Printf("[CONFIG] Twilio Webhook URL: %s", cfg.TwilioWebhookURL)
	} else {
		log.Println("[CONFIG] ⚠️ Twilio NO configurado — SMS gateway deshabilitado")
	}

	// 2. Conectar a MongoDB
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	clientOpts := options.Client().ApplyURI(cfg.MongoURI)
	client, err := mongo.Connect(ctx, clientOpts)
	if err != nil {
		log.Fatalf("[ERROR] No se pudo conectar a MongoDB: %v", err)
	}

	// Verificar conexión
	if err := client.Ping(ctx, nil); err != nil {
		log.Fatalf("[ERROR] MongoDB no responde (ping falló): %v", err)
	}
	log.Println("[OK] Conectado a MongoDB Atlas (Replica Set) exitosamente")

	db := client.Database(cfg.MongoDB)

	// 3. Inicializar repositorios (crean índices automáticamente)
	actaRepo := repository.NewActaRepository(db)
	eventoRepo := repository.NewEventoRepository(db)
	smsRepo := repository.NewSMSRepository(db)
	referenciaRepo := repository.NewReferenciaRepository(db)
	inconsistenciaRepo := repository.NewInconsistenciaRepository(db)
	estadisticaRepo := repository.NewEstadisticaRepository(db)
	log.Println("[OK] Repositorios inicializados con índices (6 colecciones)")

	// Inicializar y ejecutar seeder (idempotente — solo actúa si las colecciones están vacías)
	seeder := repository.NewSeeder(db)
	seedCtx, seedCancel := context.WithTimeout(context.Background(), 60*time.Second)
	seeder.SeedAll(seedCtx)
	seedCancel()

	// Log de datos de referencia cargados
	refCtx, refCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer refCancel()
	totalActas, _ := referenciaRepo.CountTotalActas(refCtx)
	totalRecintos, _ := referenciaRepo.CountTotalRecintos(refCtx)
	totalHab, _ := referenciaRepo.SumTotalHabilitados(refCtx)
	log.Printf("[OK] Datos de referencia: %d actas esperadas, %d recintos, %d votantes habilitados", totalActas, totalRecintos, totalHab)

	// 4. Inicializar servicios
	actaService := service.NewActaService()
	smsService := service.NewSMSService(cfg.PinValido, cfg.TelefonosAutorizados)

	// Pipeline OCR real (Tesseract + mutool). Fallback al mock si no están instalados.
	var ocrProcessor service.ActaProcessor
	if pipeline, pipelineErr := service.NewOCRPipeline("", ""); pipelineErr != nil {
		log.Printf("[WARN] Tesseract/mutool no disponibles (%v) — usando OCR mock", pipelineErr)
		ocrProcessor = service.NewMockPipeline()
	} else {
		log.Println("[OK] Pipeline OCR real inicializado (Tesseract + mutool)")
		ocrProcessor = pipeline
	}

	// Twilio — validador de firma + servicio de envío
	twilioValidator := service.NewTwilioValidator(cfg.TwilioAuthToken, cfg.TwilioWebhookURL)
	twilioService := service.NewTwilioService(cfg.TwilioAccountSID, cfg.TwilioAuthToken, cfg.TwilioPhoneNumber)

	// Servicio de detección de inconsistencias (4 tipos del documento)
	inconsistenciaService := service.NewInconsistenciaService(inconsistenciaRepo, referenciaRepo, actaRepo)

	// Proyector CQRS — mantiene vista materializada Estadisticas_RRV
	cqrsProjector := service.NewCQRSProjector(actaRepo, estadisticaRepo, referenciaRepo, inconsistenciaRepo)

	if twilioService.IsConfigured() {
		log.Println("[OK] Servicio Twilio inicializado — SMS gateway activo ✓")
	} else {
		log.Println("[WARN] Servicio Twilio no configurado — solo modo JSON disponible")
	}

	log.Println("[OK] Servicios inicializados: OCR pipeline, SMS, Twilio, Inconsistencias, CQRS Projector")

	// 5. Directorio de uploads
	execDir, _ := os.Getwd()
	uploadDir := filepath.Join(execDir, "uploads")
	os.MkdirAll(uploadDir, 0755)

	// 6. Inicializar handlers
	uploadHandler := handlers.NewUploadHandler(actaRepo, eventoRepo, ocrProcessor, actaService, inconsistenciaService, cqrsProjector, uploadDir)
	smsHandler := handlers.NewSMSHandler(actaRepo, eventoRepo, smsRepo, smsService, actaService, twilioValidator, twilioService, inconsistenciaService, cqrsProjector)
	queryHandler := handlers.NewQueryHandler(actaRepo, eventoRepo, smsRepo, inconsistenciaRepo, estadisticaRepo, referenciaRepo)
	log.Println("[OK] Handlers HTTP inicializados")

	// 7. Configurar Gin
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Logger())
	router.Use(gin.Recovery())

	// CORS para permitir acceso desde el Dashboard
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// 8. Registrar rutas
	api := router.Group("/api/rrv")
	{
		// POST - Ingesta de datos (COMMAND en CQRS)
		api.POST("/actas/upload", uploadHandler.HandleUpload)
		api.POST("/sms", smsHandler.HandleSMS)                        // JSON directo (testing)
		api.POST("/sms/twilio", smsHandler.HandleTwilioWebhook)       // Webhook real de Twilio
		api.POST("/sms/enviar", smsHandler.HandleEnviarSMS)           // Enviar SMS manualmente

		// GET - Consultas (QUERY en CQRS)
		api.GET("/actas", queryHandler.HandleListActas)
		api.GET("/actas/:acta_id", queryHandler.HandleGetActa)
		api.GET("/eventos", queryHandler.HandleListEventos)
		api.GET("/sms", queryHandler.HandleListSMS)
		api.GET("/stats", queryHandler.HandleStats)

		// GET - Nuevos endpoints CQRS + Event Sourcing + Referencia
		api.GET("/stats/full", queryHandler.HandleStatsFull)                     // Vista materializada CQRS
		api.GET("/inconsistencias", queryHandler.HandleInconsistencias)           // Logs de inconsistencias
		api.GET("/eventos/replay/:acta_id", queryHandler.HandleEventReplay)      // Event Sourcing replay
		api.GET("/referencia/totales", queryHandler.HandleReferenciaTotales)      // Datos de referencia CSV

		// GET - Banco de consultas para el Dashboard Analítico
		consultas := api.Group("/consultas")
		{
			consultas.GET("/mesas-por-recinto",           queryHandler.HandleMesasPorRecinto)          // Q1
			consultas.GET("/votos-por-municipio",         queryHandler.HandleVotosPorMunicipio)         // Q2
			consultas.GET("/votos-por-departamento",      queryHandler.HandleVotosPorDepartamento)      // Q3
			consultas.GET("/top-recintos",                queryHandler.HandleTopRecintos)               // Q4 ?candidato=P1&limite=5
			consultas.GET("/nulos-por-departamento",      queryHandler.HandleNulosPorDepartamento)      // Q5
			consultas.GET("/boletas-anuladas",            queryHandler.HandleBoletasAnuladas)           // Q6
			consultas.GET("/trep-vs-oficial",             queryHandler.HandleTREPvsOficial)             // Q7-Q8
			consultas.GET("/mesas-abstencion",            queryHandler.HandleMesasAbstencion)           // Q11 ?umbral=20
			consultas.GET("/actas-por-hora",              queryHandler.HandleActasPorHora)              // Q12
			consultas.GET("/tiempo-actas-departamento",   queryHandler.HandleTiempoActas)               // Q14
			consultas.GET("/participacion-departamento",  queryHandler.HandleParticipacionPorDepartamento) // Q16
			consultas.GET("/inconsistencias-trep-oficial",queryHandler.HandleActasInconsistentes)       // Q17
			consultas.GET("/resultados-geograficos",      queryHandler.HandleResultadosGeograficos)     // Q19 ?departamento=X
			consultas.GET("/errores-comunes",             queryHandler.HandleErroresComunes)            // Q20
		}
	}

	// Health check con indicadores técnicos
	router.GET("/health", func(c *gin.Context) {
		twilioStatus := "deshabilitado"
		if twilioService.IsConfigured() {
			twilioStatus = "activo"
		}
		c.JSON(http.StatusOK, gin.H{
			"status":              "ok",
			"service":             "rrv-backend",
			"twilio":              twilioStatus,
			"time":                time.Now().Format(time.RFC3339),
			"patrones": gin.H{
				"cqrs":            "Estadisticas_RRV (vista materializada)",
				"event_sourcing":  "eventos_rrv (replay vía /eventos/replay/:id)",
				"idempotencia":    "SHA256 + índices únicos + deduplicación",
				"tolerancia_fallos": "Retry con backoff exponencial (3 intentos)",
			},
			"base_datos": gin.H{
				"motor":      "MongoDB Atlas",
				"tipo":       "Replica Set (3 nodos)",
				"consistencia": "Eventual (RRV)",
			},
		})
	})

	// 9. Imprimir rutas registradas
	fmt.Println()
	log.Println("═══ Endpoints Registrados ═══")
	log.Println("  ── COMMAND (Escritura) ──")
	log.Println("  POST /api/rrv/actas/upload        → Subir imagen/PDF para OCR")
	log.Println("  POST /api/rrv/sms                 → Recibir SMS (JSON directo)")
	log.Println("  POST /api/rrv/sms/twilio          → Webhook Twilio (SMS real)")
	log.Println("  POST /api/rrv/sms/enviar          → Enviar SMS manualmente")
	log.Println()
	log.Println("  ── QUERY (Lectura) ──")
	log.Println("  GET  /api/rrv/actas               → Listar actas")
	log.Println("  GET  /api/rrv/actas/:id            → Obtener acta por ID")
	log.Println("  GET  /api/rrv/eventos              → Listar eventos (Event Sourcing)")
	log.Println("  GET  /api/rrv/sms                  → Listar SMS recibidos")
	log.Println("  GET  /api/rrv/stats                → Estadísticas básicas")
	log.Println("  GET  /api/rrv/stats/full           → Estadísticas CQRS completas (KPIs)")
	log.Println("  GET  /api/rrv/inconsistencias      → Logs de inconsistencias")
	log.Println("  GET  /api/rrv/eventos/replay/:id   → Event Sourcing replay")
	log.Println("  GET  /api/rrv/referencia/totales   → Datos de referencia CSV")
	log.Println("  GET  /health                       → Health check + indicadores técnicos")
	log.Println()
	log.Println("  ── BANCO DE CONSULTAS (Dashboard) ──")
	log.Println("  GET  /api/rrv/consultas/mesas-por-recinto")
	log.Println("  GET  /api/rrv/consultas/votos-por-municipio")
	log.Println("  GET  /api/rrv/consultas/votos-por-departamento")
	log.Println("  GET  /api/rrv/consultas/top-recintos?candidato=P1&limite=5")
	log.Println("  GET  /api/rrv/consultas/nulos-por-departamento")
	log.Println("  GET  /api/rrv/consultas/boletas-anuladas")
	log.Println("  GET  /api/rrv/consultas/trep-vs-oficial")
	log.Println("  GET  /api/rrv/consultas/mesas-abstencion?umbral=20")
	log.Println("  GET  /api/rrv/consultas/actas-por-hora")
	log.Println("  GET  /api/rrv/consultas/tiempo-actas-departamento")
	log.Println("  GET  /api/rrv/consultas/participacion-departamento")
	log.Println("  GET  /api/rrv/consultas/inconsistencias-trep-oficial")
	log.Println("  GET  /api/rrv/consultas/resultados-geograficos?departamento=X&municipio=Y")
	log.Println("  GET  /api/rrv/consultas/errores-comunes")
	fmt.Println()

	// 10. Generar proyección CQRS inicial
	log.Println("[CQRS] Generando proyección inicial de estadísticas...")
	cqrsProjector.Proyectar(context.Background())

	// 11. Arrancar servidor con graceful shutdown
	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}

	go func() {
		log.Printf("[SERVER] Servidor RRV escuchando en http://localhost:%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[ERROR] Error iniciando servidor: %v", err)
		}
	}()

	// Esperar señal de cierre
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[SHUTDOWN] Apagando servidor...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("[ERROR] Error en shutdown: %v", err)
	}

	if err := client.Disconnect(shutdownCtx); err != nil {
		log.Printf("[ERROR] Error desconectando MongoDB: %v", err)
	}

	log.Println("[SHUTDOWN] Servidor detenido correctamente")
}
