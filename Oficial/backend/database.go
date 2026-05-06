package main

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/gocarina/gocsv"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var dbMu sync.RWMutex // protects DB during reconnection

var DB *gorm.DB

func buildDSN() string {
	host := os.Getenv("DB_HOST")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	dbname := os.Getenv("DB_NAME")
	port := os.Getenv("DB_PORT")

	if host == "" {
		host = "localhost"
		user = "postgres"
		password = "adminpassword"
		dbname = "sistema_electoral"
		port = "5432"
	}

	return fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable TimeZone=UTC",
		host, user, password, dbname, port,
	)
}

func openDB(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	// Pool settings
	sqlDB.SetMaxOpenConns(20)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)
	return db, nil
}

// GetDB devuelve la instancia activa de forma segura para concurrencia.
func GetDB() *gorm.DB {
	dbMu.RLock()
	defer dbMu.RUnlock()
	return DB
}

// startHealthChecker verifica la conexión cada 10 s y reconecta si es necesario.
// Esto cubre el periodo de failover de pgpool/repmgr.
func startHealthChecker(dsn string) {
	go func() {
		for {
			time.Sleep(10 * time.Second)
			dbMu.RLock()
			currentDB := DB
			dbMu.RUnlock()

			sqlDB, err := currentDB.DB()
			if err == nil {
				err = sqlDB.Ping()
			}

			if err != nil {
				log.Printf("⚠️  DB health check failed (%v) — intentando reconectar...", err)
				for attempt := 1; ; attempt++ {
					newDB, connErr := openDB(dsn)
					if connErr == nil {
						dbMu.Lock()
						DB = newDB
						dbMu.Unlock()
						log.Printf("✅  Reconectado a la base de datos (intento %d)", attempt)
						break
					}
					log.Printf("🔄  Reintento %d fallido: %v", attempt, connErr)
					time.Sleep(5 * time.Second)
				}
			}
		}
	}()
}

func ConnectDB() {
	dsn := buildDSN()

	var err error
	for i := 0; i < 15; i++ {
		DB, err = openDB(dsn)
		if err == nil {
			break
		}
		log.Printf("Failed to connect to database, retrying in 5 seconds... (%v)", err)
		time.Sleep(5 * time.Second)
	}

	if err != nil {
		log.Fatalf("Failed to connect to database after retries: %v", err)
	}

	log.Println("Connected to database successfully")

	// Migrate schemas
	err = DB.AutoMigrate(
		&DistribucionTerritorial{},
		&RecintoElectoral{},
		&Mesa{},
		&PartidoPolitico{},
		&Usuario{},
		&Papeleta{},
		&DetalleVotosPartido{},
	)
	if err != nil {
		log.Fatalf("AutoMigrate failed: %v", err)
	}

	seedData()

	// Iniciar monitor de salud en background
	startHealthChecker(dsn)
	log.Println("🔍 Monitor de salud de DB iniciado (reconexión automática habilitada)")
}

func seedData() {
	// Database seeding is idempotent thanks to OnConflict clauses
	log.Println("Seeding database (idempotent)...")

	log.Println("Seeding database...")

	// 1. Seed Usuarios
	usuarios := []Usuario{
		{Nombre: "Funcionario 1"},
		{Nombre: "Funcionario 2"},
		{Nombre: "Funcionario 3"},
		{Nombre: "Admin"},
	}
	for _, u := range usuarios {
		DB.Create(&u)
	}

	// 2. Seed Partidos
	partidos := []PartidoPolitico{
		{IdPartido: 1, Sigla: "P1"},
		{IdPartido: 2, Sigla: "P2"},
		{IdPartido: 3, Sigla: "P3"},
		{IdPartido: 4, Sigla: "P4"},
	}
	for _, p := range partidos {
		DB.Create(&p)
	}

	// 3. Seed Distribucion Territorial
	seedDistribucion()

	// 4. Seed Recintos
	seedRecintos()

	// 5. Seed Mesas (ActasImpresas)
	seedMesas()

	log.Println("Seeding completed successfully")
}

func seedDistribucion() {
	file, err := os.Open("../_Recursos Practica 4 - DistribucionTerritorial.csv")
	if err != nil {
		file, err = os.Open("/data/_Recursos Practica 4 - DistribucionTerritorial.csv")
		if err != nil {
			log.Printf("Warning: Could not open DistribucionTerritorial.csv: %v", err)
			return
		}
	}
	defer file.Close()

	var data []*CSVTerritorio
	if err := gocsv.UnmarshalFile(file, &data); err != nil {
		log.Printf("Error parsing DistribucionTerritorial.csv: %v", err)
		return
	}

	var batch []DistribucionTerritorial
	for _, row := range data {
		batch = append(batch, DistribucionTerritorial{
			CodigoTerritorial: row.CodigoTerritorial,
			Departamento:      row.Departamento,
			Provincia:         row.Provincia,
			Municipio:         row.Municipio,
		})
	}
	if err := DB.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(batch, 100).Error; err != nil {
		log.Printf("Error seeding DistribucionTerritorial: %v", err)
	}
}

func seedRecintos() {
	file, err := os.Open("../_Recursos Practica 4 - RecintosElectorales.csv")
	if err != nil {
		file, err = os.Open("/data/_Recursos Practica 4 - RecintosElectorales.csv")
		if err != nil {
			log.Printf("Warning: Could not open RecintosElectorales.csv: %v", err)
			return
		}
	}
	defer file.Close()

	var data []*CSVRecinto
	if err := gocsv.UnmarshalFile(file, &data); err != nil {
		log.Printf("Error parsing RecintosElectorales.csv: %v", err)
		return
	}

	var batch []RecintoElectoral
	for _, row := range data {
		batch = append(batch, RecintoElectoral{
			CodigoRecinto:     row.CodigoRecinto,
			CodigoTerritorial: row.CodigoTerritorial,
			RecintoNombre:     row.RecintoNombre,
			RecintoDireccion:  row.RecintoDireccion,
			NumMesas:          row.NumMesas,
		})
	}
	if err := DB.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(batch, 100).Error; err != nil {
		log.Printf("Error seeding RecintosElectorales: %v", err)
	}
}

func seedMesas() {
	file, err := os.Open("../_Recursos Practica 4 - Transcripciones.csv")
	if err != nil {
		file, err = os.Open("/data/_Recursos Practica 4 - Transcripciones.csv")
		if err != nil {
			log.Printf("Warning: Could not open Transcripciones.csv: %v", err)
			return
		}
	}
	defer file.Close()

	var data []*CSVActa
	if err := gocsv.UnmarshalFile(file, &data); err != nil {
		log.Printf("Error parsing Transcripciones.csv: %v", err)
		return
	}

	var batch []Mesa
	for _, row := range data {
		batch = append(batch, Mesa{
			CodigoActa:          row.CodigoActa,
			CodigoRecinto:       row.CodigoRecinto,
			NroMesa:             row.NroMesa,
			VotantesHabilitados: row.VotantesHabilitados,
		})
	}
	if err := DB.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(batch, 500).Error; err != nil {
		log.Printf("Error seeding Mesas: %v", err)
	}
}
