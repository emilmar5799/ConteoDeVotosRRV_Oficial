package service

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// TwilioService maneja el envío de mensajes SMS a través de la API REST de Twilio.
// No usa SDK — se comunica directamente con la API HTTP de Twilio.
type TwilioService struct {
	accountSID  string
	authToken   string
	fromNumber  string
	httpClient  *http.Client
	apiBaseURL  string
}

// TwilioSendResult contiene el resultado del envío de un SMS.
type TwilioSendResult struct {
	MessageSID string `json:"sid"`
	Status     string `json:"status"`
	To         string `json:"to"`
	From       string `json:"from"`
	Body       string `json:"body"`
	ErrorCode  int    `json:"error_code,omitempty"`
	ErrorMsg   string `json:"error_message,omitempty"`
}

// NewTwilioService crea un nuevo servicio de Twilio.
func NewTwilioService(accountSID, authToken, fromNumber string) *TwilioService {
	return &TwilioService{
		accountSID: accountSID,
		authToken:  authToken,
		fromNumber: fromNumber,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		apiBaseURL: fmt.Sprintf("https://api.twilio.com/2010-04-01/Accounts/%s", accountSID),
	}
}

// IsConfigured verifica si el servicio Twilio está configurado correctamente.
func (s *TwilioService) IsConfigured() bool {
	return s.accountSID != "" && s.authToken != "" && s.fromNumber != ""
}

// EnviarSMS envía un mensaje SMS al número destino.
// Retorna el resultado del envío o un error.
func (s *TwilioService) EnviarSMS(destinatario, mensaje string) (*TwilioSendResult, error) {
	if !s.IsConfigured() {
		return nil, fmt.Errorf("twilio no configurado: faltan credenciales")
	}

	endpoint := fmt.Sprintf("%s/Messages.json", s.apiBaseURL)

	// Preparar form data
	data := url.Values{}
	data.Set("To", destinatario)
	data.Set("From", s.fromNumber)
	data.Set("Body", mensaje)

	// Crear request
	req, err := http.NewRequest("POST", endpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("error creando request Twilio: %w", err)
	}

	// Autenticación HTTP Basic con Account SID + Auth Token
	req.SetBasicAuth(s.accountSID, s.authToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	log.Printf("[TWILIO] Enviando SMS a %s: %s", destinatario, truncar(mensaje, 50))

	// Enviar request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error enviando SMS via Twilio: %w", err)
	}
	defer resp.Body.Close()

	// Leer respuesta
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error leyendo respuesta Twilio: %w", err)
	}

	// Parsear respuesta JSON
	var result TwilioSendResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("error parseando respuesta Twilio: %w (body: %s)", err, string(body))
	}

	// Verificar status HTTP
	if resp.StatusCode >= 400 {
		log.Printf("[TWILIO] Error HTTP %d: %s (code: %d)", resp.StatusCode, result.ErrorMsg, result.ErrorCode)
		return &result, fmt.Errorf("twilio API error %d: %s", result.ErrorCode, result.ErrorMsg)
	}

	log.Printf("[TWILIO] SMS enviado exitosamente: SID=%s, Status=%s", result.MessageSID, result.Status)
	return &result, nil
}

// EnviarConfirmacion envía un SMS de confirmación al notario electoral.
func (s *TwilioService) EnviarConfirmacion(destinatario, actaID, estado string) {
	if !s.IsConfigured() {
		log.Println("[TWILIO] No configurado — confirmación SMS omitida")
		return
	}

	var mensaje string
	switch estado {
	case "PROCESADA":
		mensaje = fmt.Sprintf("✅ Sistema RRV Bolivia: Acta %s recibida y procesada correctamente.", actaID)
	case "ERROR":
		mensaje = fmt.Sprintf("⚠️ Sistema RRV Bolivia: Acta %s recibida con errores de validación. Será revisada.", actaID)
	case "ANULADA":
		mensaje = fmt.Sprintf("❌ Sistema RRV Bolivia: Acta %s marcada como ANULADA. Contacte al tribunal.", actaID)
	default:
		mensaje = fmt.Sprintf("📋 Sistema RRV Bolivia: Acta %s recibida. Estado: %s", actaID, estado)
	}

	// Enviar en goroutine para no bloquear el flujo principal
	go func() {
		_, err := s.EnviarSMS(destinatario, mensaje)
		if err != nil {
			log.Printf("[TWILIO] Error enviando confirmación a %s: %v", destinatario, err)
		}
	}()
}

// EnviarErrorFormato envía un SMS indicando error de formato al notario.
func (s *TwilioService) EnviarErrorFormato(destinatario string, errores []string) {
	if !s.IsConfigured() {
		return
	}

	mensaje := fmt.Sprintf("❌ Sistema RRV Bolivia: Error en formato de SMS. %s. "+
		"Formato: ACTA:MESA-ID|DEP:Dept|MUN:Mun|MESA:N|VAL:N|NUL:N|BLA:N|C1:N|C2:N|PIN:1234",
		strings.Join(errores, "; "))

	// Truncar a 1600 caracteres (límite SMS largo de Twilio)
	if len(mensaje) > 1600 {
		mensaje = mensaje[:1597] + "..."
	}

	go func() {
		_, err := s.EnviarSMS(destinatario, mensaje)
		if err != nil {
			log.Printf("[TWILIO] Error enviando alerta de formato a %s: %v", destinatario, err)
		}
	}()
}

// truncar corta un string a maxLen caracteres, agregando "..." si se corta.
func truncar(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
