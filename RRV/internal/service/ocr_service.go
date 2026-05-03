package service

import (
	"crypto/sha256"
	"fmt"
	"math/rand"
	"path/filepath"
	"strings"

	"rrv-backend/internal/models"
)

// OCRService simula la extracción de datos de imágenes y PDFs.
// Esta implementación es un MOCK determinista basado en el hash del archivo.
// Para reemplazar con OCR real (Google Vision, Tesseract), solo cambiar el método ProcesarArchivo.
type OCRService struct{}

// NewOCRService crea una nueva instancia del servicio OCR.
func NewOCRService() *OCRService {
	return &OCRService{}
}

// departamentos es una lista de departamentos de Bolivia para generar datos simulados.
var departamentos = []string{
	"Chuquisaca", "La Paz", "Cochabamba", "Oruro",
	"Potosí", "Tarija", "Santa Cruz", "Beni", "Pando",
}

var municipios = map[string][]string{
	"Chuquisaca":  {"Sucre", "Yotala", "Poroma", "Azurduy", "Tarvita"},
	"La Paz":      {"La Paz", "El Alto", "Viacha", "Achacachi", "Copacabana"},
	"Cochabamba":  {"Cochabamba", "Quillacollo", "Sacaba", "Tiquipaya", "Colcapirhua"},
	"Oruro":       {"Oruro", "Huanuni", "Caracollo", "Challapata", "Eucaliptus"},
	"Potosí":      {"Potosí", "Llallagua", "Villazón", "Tupiza", "Uyuni"},
	"Tarija":      {"Tarija", "Yacuiba", "Bermejo", "Villamontes", "Entre Ríos"},
	"Santa Cruz":  {"Santa Cruz", "Montero", "Warnes", "Cotoca", "Porongo"},
	"Beni":        {"Trinidad", "Riberalta", "Guayaramerín", "San Borja", "Reyes"},
	"Pando":       {"Cobija", "Porvenir", "Bolpebra", "Bella Flor", "Puerto Rico"},
}

var recintos = []string{
	"U.E. Santa Mónica", "U.E. Padresama", "U.E. Lacolaconi",
	"U.E. Genoveva Ríos", "U.E. 27 de Mayo", "Colegio Ayacucho",
	"Liceo Venezuela", "U.E. San Martín", "Colegio Don Bosco",
	"U.E. Juan XXIII", "Colegio Nacional Sucre", "U.E. Simón Bolívar",
}

// ProcesarArchivo simula el OCR sobre un archivo (imagen o PDF).
// Genera datos deterministas basados en el hash del contenido del archivo.
// fileHash es el SHA256 del contenido del archivo.
// fileName es el nombre original del archivo para determinar el tipo de entrada.
func (s *OCRService) ProcesarArchivo(fileHash string, fileName string) *models.ActaRRV {
	// Usar el hash como seed para generar datos deterministas
	seed := hashToSeed(fileHash)
	rng := rand.New(rand.NewSource(seed))

	// Determinar tipo de entrada por extensión
	ext := strings.ToLower(filepath.Ext(fileName))
	tipoEntrada := models.TipoImagen
	if ext == ".pdf" {
		tipoEntrada = models.TipoPDF
	}

	// Generar datos simulados deterministas
	depIdx := rng.Intn(len(departamentos))
	departamento := departamentos[depIdx]

	munis := municipios[departamento]
	municipio := munis[rng.Intn(len(munis))]

	recinto := recintos[rng.Intn(len(recintos))]
	mesaNum := rng.Intn(35000) + 1
	codigoMesa := fmt.Sprintf("%d", mesaNum)

	// Generar candidatos dinámicos (entre 3 y 7 candidatos)
	numCandidatos := rng.Intn(5) + 3
	candidatos := make([]models.Candidato, numCandidatos)
	sumaCandidatos := 0
	for i := 0; i < numCandidatos; i++ {
		votos := rng.Intn(200) + 10
		candidatos[i] = models.Candidato{
			CandidatoID: fmt.Sprintf("C%d", i+1),
			Votos:       votos,
		}
		sumaCandidatos += votos
	}

	votosBlancos := rng.Intn(20) + 1
	votosNulos := rng.Intn(15) + 1
	votosValidos := sumaCandidatos + votosBlancos
	totalVotos := votosValidos + votosNulos

	return &models.ActaRRV{
		ActaID:       fmt.Sprintf("MESA-%05d", mesaNum),
		CodigoMesa:   codigoMesa,
		Departamento: departamento,
		Municipio:    municipio,
		Recinto:      recinto,
		Mesa:         fmt.Sprintf("%d", mesaNum),
		Candidatos:   candidatos,
		VotosValidos: votosValidos,
		VotosNulos:   votosNulos,
		VotosBlancos: votosBlancos,
		TotalVotos:   totalVotos,
		Fuente:       models.FuenteRRV,
		TipoEntrada:  tipoEntrada,
		HashOrigen:   fileHash,
	}
}

// hashToSeed convierte un hash hexadecimal en un seed numérico para rand.
func hashToSeed(hash string) int64 {
	h := sha256.Sum256([]byte(hash))
	var seed int64
	for i := 0; i < 8 && i < len(h); i++ {
		seed = (seed << 8) | int64(h[i])
	}
	if seed < 0 {
		seed = -seed
	}
	return seed
}
