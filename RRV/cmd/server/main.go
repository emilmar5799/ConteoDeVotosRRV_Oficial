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
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// 1. Cargar configuración
	cfg := config.LoadConfig()
	log.Printf("[CONFIG] MongoDB URI: %s", cfg.MongoURI)
	log.Printf("[CONFIG] MongoDB DB: %s", cfg.MongoDB)
	log.Printf("[CONFIG] Puerto: %s", cfg.Port)
	log.Printf("[CONFIG] Teléfonos autorizados: %v", cfg.TelefonosAutorizados)

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
	log.Println("[OK] Conectado a MongoDB exitosamente")

	db := client.Database(cfg.MongoDB)

	// 3. Inicializar repositorios (crean índices automáticamente)
	actaRepo := repository.NewActaRepository(db)
	eventoRepo := repository.NewEventoRepository(db)
	smsRepo := repository.NewSMSRepository(db)
	log.Println("[OK] Repositorios inicializados con índices")

	// 4. Inicializar servicios
	actaService := service.NewActaService()
	smsService := service.NewSMSService(cfg.PinValido, cfg.TelefonosAutorizados)

	// Pipeline OCR real (Tesseract + mutool). Si no están instalados, usa el mock.
	var processor service.ActaProcessor
	pipeline, pipelineErr := service.NewOCRPipeline("", "")
	if pipelineErr != nil {
		log.Printf("[WARN] Pipeline OCR real no disponible (%v) — usando mock", pipelineErr)
		processor = service.NewMockPipeline()
	} else {
		log.Println("[OK] Pipeline OCR real inicializado (Tesseract + mutool)")
		processor = pipeline
	}

	log.Println("[OK] Servicios de negocio inicializados")

	// 5. Directorio de uploads
	execDir, _ := os.Getwd()
	uploadDir := filepath.Join(execDir, "uploads")
	os.MkdirAll(uploadDir, 0755)

	// 6. Inicializar handlers
	uploadHandler := handlers.NewUploadHandler(actaRepo, eventoRepo, processor, actaService, uploadDir)
	smsHandler := handlers.NewSMSHandler(actaRepo, eventoRepo, smsRepo, smsService, actaService)
	queryHandler := handlers.NewQueryHandler(actaRepo, eventoRepo, smsRepo)
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
		// POST - Ingesta de datos
		api.POST("/actas/upload", uploadHandler.HandleUpload)
		api.POST("/sms", smsHandler.HandleSMS)

		// GET - Consultas
		api.GET("/actas", queryHandler.HandleListActas)
		api.GET("/actas/:acta_id", queryHandler.HandleGetActa)
		api.GET("/eventos", queryHandler.HandleListEventos)
		api.GET("/sms", queryHandler.HandleListSMS)
		api.GET("/stats", queryHandler.HandleStats)
	}

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":  "ok",
			"service": "rrv-backend",
			"time":    time.Now().Format(time.RFC3339),
		})
	})

	// 9. Imprimir rutas registradas
	fmt.Println()
	log.Println("═══ Endpoints Registrados ═══")
	log.Println("  POST /api/rrv/actas/upload  → Subir imagen/PDF para OCR")
	log.Println("  POST /api/rrv/sms           → Recibir SMS con datos de acta")
	log.Println("  GET  /api/rrv/actas         → Listar actas (filtros: departamento, estado, tipo_entrada)")
	log.Println("  GET  /api/rrv/actas/:id     → Obtener acta por ID")
	log.Println("  GET  /api/rrv/eventos       → Listar eventos del pipeline")
	log.Println("  GET  /api/rrv/sms           → Listar SMS recibidos")
	log.Println("  GET  /api/rrv/stats         → Estadísticas agregadas")
	log.Println("  GET  /health                → Health check")
	fmt.Println()

	// 10. Arrancar servidor con graceful shutdown
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
