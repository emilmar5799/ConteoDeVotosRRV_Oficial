package service

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"log"
	"net/url"
	"sort"
	"strings"
)

// TwilioValidator verifica la autenticidad de los webhooks de Twilio
// usando la firma HMAC-SHA1 del header X-Twilio-Signature.
// Esto previene que atacantes envíen requests falsos al webhook.
type TwilioValidator struct {
	authToken  string
	webhookURL string
}

// NewTwilioValidator crea un nuevo validador de firmas Twilio.
func NewTwilioValidator(authToken, webhookURL string) *TwilioValidator {
	return &TwilioValidator{
		authToken:  authToken,
		webhookURL: webhookURL,
	}
}

// ValidarFirma verifica que el header X-Twilio-Signature sea válido.
//
// Algoritmo de Twilio:
//  1. Tomar la URL completa del webhook
//  2. Ordenar los parámetros POST alfabéticamente por nombre
//  3. Concatenar nombre+valor de cada parámetro a la URL
//  4. Calcular HMAC-SHA1 con el Auth Token como clave
//  5. Codificar en Base64
//  6. Comparar con el header X-Twilio-Signature
//
// Referencia: https://www.twilio.com/docs/usage/security#validating-requests
func (v *TwilioValidator) ValidarFirma(firma string, params map[string]string) bool {
	if v.authToken == "" || v.webhookURL == "" {
		log.Println("[TWILIO-VALIDATOR] Auth token o webhook URL vacíos — firma no verificada (desarrollo)")
		return true // En modo desarrollo sin configurar, permitir
	}

	if firma == "" {
		log.Println("[TWILIO-VALIDATOR] Header X-Twilio-Signature ausente")
		return false
	}

	// 1. Empezar con la URL del webhook
	data := v.webhookURL

	// 2. Ordenar los nombres de parámetros alfabéticamente
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 3. Concatenar nombre + valor
	for _, k := range keys {
		data += k + params[k]
	}

	// 4. Calcular HMAC-SHA1
	mac := hmac.New(sha1.New, []byte(v.authToken))
	mac.Write([]byte(data))
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	// 5. Comparar de forma segura (timing-safe)
	isValid := hmac.Equal([]byte(expected), []byte(firma))

	if !isValid {
		log.Printf("[TWILIO-VALIDATOR] Firma inválida. Esperada: %s, Recibida: %s", expected, firma)
	} else {
		log.Println("[TWILIO-VALIDATOR] Firma verificada correctamente ✓")
	}

	return isValid
}

// ExtraerParamsForm extrae todos los parámetros de un form-urlencoded body
// y los devuelve como map[string]string para la validación de firma.
func ExtraerParamsForm(formValues url.Values) map[string]string {
	params := make(map[string]string)
	for key, values := range formValues {
		if len(values) > 0 {
			params[key] = values[0]
		}
	}
	return params
}

// GenerarFirma genera una firma Twilio para testing.
// Útil para pruebas unitarias y simulación.
func (v *TwilioValidator) GenerarFirma(params map[string]string) string {
	data := v.webhookURL

	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		data += k + params[k]
	}

	mac := hmac.New(sha1.New, []byte(v.authToken))
	mac.Write([]byte(data))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// FormatearTwiML genera una respuesta TwiML para Twilio.
// TwiML es el formato XML que Twilio espera como respuesta al webhook.
func FormatearTwiML(mensaje string) string {
	// Escapar caracteres XML especiales
	mensaje = strings.ReplaceAll(mensaje, "&", "&amp;")
	mensaje = strings.ReplaceAll(mensaje, "<", "&lt;")
	mensaje = strings.ReplaceAll(mensaje, ">", "&gt;")

	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Response>
  <Message>%s</Message>
</Response>`, mensaje)
}

// FormatearTwiMLVacio genera una respuesta TwiML vacía (sin mensaje de respuesta).
func FormatearTwiMLVacio() string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<Response></Response>`
}
