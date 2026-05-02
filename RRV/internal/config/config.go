package config

import "os"

// Config contiene la configuración del servicio RRV.
type Config struct {
	MongoURI             string
	MongoDB              string
	Port                 string
	PinValido            string
	TelefonosAutorizados []string
}

// LoadConfig carga la configuración desde variables de entorno con defaults seguros.
func LoadConfig() *Config {
	cfg := &Config{
		MongoURI:  getEnv("MONGO_URI", "mongodb://localhost:27017"),
		MongoDB:   getEnv("MONGO_DB", "rrv_electoral"),
		Port:      getEnv("PORT", "8080"),
		PinValido: getEnv("PIN_VALIDO", "1234"),
	}

	// Teléfonos autorizados separados por coma
	telefonos := getEnv("TELEFONOS_AUTORIZADOS", "+59170000001,+59170000002,+59170000003")
	cfg.TelefonosAutorizados = splitCSV(telefonos)

	return cfg
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func splitCSV(s string) []string {
	var result []string
	current := ""
	for _, ch := range s {
		if ch == ',' {
			trimmed := trimSpace(current)
			if trimmed != "" {
				result = append(result, trimmed)
			}
			current = ""
		} else {
			current += string(ch)
		}
	}
	trimmed := trimSpace(current)
	if trimmed != "" {
		result = append(result, trimmed)
	}
	return result
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
