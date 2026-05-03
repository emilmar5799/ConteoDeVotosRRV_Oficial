package main

import (
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"gorm.io/gorm"
)

type LoginRequest struct {
	Nombre string `json:"nombre"`
}

type TranscripcionRequest struct {
	CodigoActa          int64  `json:"codigo_acta"`
	IdUsuario           int    `json:"id_usuario"`
	P1                  int    `json:"p1"`
	P2                  int    `json:"p2"`
	P3                  int    `json:"p3"`
	P4                  int    `json:"p4"`
	VotosValidos        int    `json:"votos_validos"`
	VotosBlancos        int    `json:"votos_blancos"`
	VotosNulos          int    `json:"votos_nulos"`
	PapeletasAnfora     int    `json:"papeletas_anfora"`
	PapeltasNoUtilizadas int    `json:"papeletas_no_utilizadas"`
	AperturaHora        int    `json:"apertura_hora"`
	AperturaMinutos     int    `json:"apertura_minutos"`
	CierreHora          int    `json:"cierre_hora"`
	CierreMinutos       int    `json:"cierre_minutos"`
	Observaciones       string `json:"observaciones"`
}

func main() {
	ConnectDB()

	app := fiber.New()

	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
	}))

	api := app.Group("/api")

	api.Post("/login", func(c *fiber.Ctx) error {
		var req LoginRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
		}

		db := GetDB()
		var user Usuario
		result := db.Where("nombre = ?", req.Nombre).First(&user)
		if result.Error != nil {
			return c.Status(404).JSON(fiber.Map{"error": "Usuario no encontrado"})
		}

		return c.JSON(fiber.Map{
			"id_usuario": user.IdUsuario,
			"nombre":     user.Nombre,
		})
	})

	api.Get("/actas", func(c *fiber.Ctx) error {
		codigoActa := c.Query("codigo_acta")
		recinto := c.Query("codigo_recinto")
		mesaNum := c.Query("nro_mesa")

		db := GetDB()
		var mesa Mesa
		var result *gorm.DB

		if codigoActa != "" {
			result = db.Preload("Recinto").Preload("Recinto.Territorio").Where("codigo_acta = ?", codigoActa).First(&mesa)
		} else if recinto != "" && mesaNum != "" {
			result = db.Preload("Recinto").Preload("Recinto.Territorio").Where("codigo_recinto = ? AND nro_mesa = ?", recinto, mesaNum).First(&mesa)
		} else {
			return c.Status(400).JSON(fiber.Map{"error": "Debe proporcionar codigo_acta o bien codigo_recinto y nro_mesa"})
		}

		if result.Error != nil {
			return c.Status(404).JSON(fiber.Map{"error": "Acta no encontrada"})
		}

		// Verificar si ya fue transcrita
		var count int64
		db.Model(&Papeleta{}).Where("codigo_acta = ?", mesa.CodigoActa).Count(&count)
		if count > 0 {
			return c.Status(400).JSON(fiber.Map{"error": "El acta ya fue transcrita"})
		}

		return c.JSON(fiber.Map{
			"codigo_acta":          mesa.CodigoActa,
			"recinto":              mesa.Recinto.RecintoNombre,
			"nro_mesa":             mesa.NroMesa,
			"votantes_habilitados": mesa.VotantesHabilitados,
		})
	})

	api.Post("/transcripciones", func(c *fiber.Ctx) error {
		var req TranscripcionRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "Invalid request format"})
		}

		db := GetDB()

		// Validación 1: El acta existe
		var mesa Mesa
		if err := db.Where("codigo_acta = ?", req.CodigoActa).First(&mesa).Error; err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "Acta no encontrada"})
		}

		// Validación 2: Votos Válidos = P1 + P2 + P3 + P4
		sumPartidos := req.P1 + req.P2 + req.P3 + req.P4
		if sumPartidos != req.VotosValidos {
			return c.Status(400).JSON(fiber.Map{"error": "La suma de votos por partido no coincide con los votos válidos"})
		}

		// Validación 3: Papeletas en Ánfora = Válidos + Blancos + Nulos
		sumAnfora := req.VotosValidos + req.VotosBlancos + req.VotosNulos
		if sumAnfora != req.PapeletasAnfora {
			return c.Status(400).JSON(fiber.Map{"error": "Los votos válidos, blancos y nulos no coinciden con las papeletas en ánfora"})
		}

		// Validación 4: Ánfora + No Utilizadas = Votantes Habilitados
		totalPapeletas := req.PapeletasAnfora + req.PapeltasNoUtilizadas
		if totalPapeletas != mesa.VotantesHabilitados {
			return c.Status(400).JSON(fiber.Map{"error": "Las papeletas en ánfora más las no utilizadas no coinciden con los votantes habilitados"})
		}

		// Transacción
		tx := db.Begin()

		papeleta := Papeleta{
			CodigoActa:           req.CodigoActa,
			VotosValidos:         req.VotosValidos,
			VotosBlancos:         req.VotosBlancos,
			VotosNulos:           req.VotosNulos,
			PapeletasAnfora:      req.PapeletasAnfora,
			PapeltasNoUtilizadas: req.PapeltasNoUtilizadas,
			AperturaHora:         req.AperturaHora,
			AperturaMinutos:      req.AperturaMinutos,
			CierreHora:           req.CierreHora,
			CierreMinutos:        req.CierreMinutos,
			Observaciones:        req.Observaciones,
			IdUsuario:            req.IdUsuario,
		}

		if err := tx.Create(&papeleta).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Error al guardar papeleta, posiblemente ya transcrita"})
		}

		detalles := []DetalleVotosPartido{
			{IdPapeleta: papeleta.IdPapeleta, IdPartido: 1, CantidadVotos: req.P1},
			{IdPapeleta: papeleta.IdPapeleta, IdPartido: 2, CantidadVotos: req.P2},
			{IdPapeleta: papeleta.IdPapeleta, IdPartido: 3, CantidadVotos: req.P3},
			{IdPapeleta: papeleta.IdPapeleta, IdPartido: 4, CantidadVotos: req.P4},
		}

		if err := tx.Create(&detalles).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Error al guardar detalle de votos"})
		}

		tx.Commit()
		return c.JSON(fiber.Map{"message": "Transcripción guardada exitosamente"})
	})

	// ─── ENDPOINT DEMO: truncar transcripciones para reset del demo visual ───
	// DELETE /api/transcripciones  →  borra todos los registros de papeletas y detalles
	api.Delete("/transcripciones", func(c *fiber.Ctx) error {
		db := GetDB()
		tx := db.Begin()

		// Borrar detalles primero (FK → papeletas)
		if err := tx.Where("1 = 1").Delete(&DetalleVotosPartido{}).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Error al limpiar detalle_votos_partido: " + err.Error()})
		}
		// Borrar papeletas
		if err := tx.Where("1 = 1").Delete(&Papeleta{}).Error; err != nil {
			tx.Rollback()
			return c.Status(500).JSON(fiber.Map{"error": "Error al limpiar papeletas: " + err.Error()})
		}

		tx.Commit()
		log.Println("🗑️  Transcripciones truncadas vía endpoint demo")
		return c.JSON(fiber.Map{"message": "Todas las transcripciones fueron eliminadas correctamente"})
	})

	log.Fatal(app.Listen(":8080"))
}
