package service

import (
	"context"
	"fmt"
	"log"
	"time"
)

// RetryConfig configura el comportamiento de reintentos.
type RetryConfig struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
}

// DefaultRetryConfig retorna la configuración por defecto: 3 reintentos, backoff 100ms→2s.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: 3,
		BaseDelay:  100 * time.Millisecond,
		MaxDelay:   2 * time.Second,
	}
}

// WithRetry ejecuta una función con reintentos automáticos y backoff exponencial.
// Implementa el patrón de Tolerancia a Fallos requerido por el documento.
func WithRetry(ctx context.Context, cfg RetryConfig, operacion string, fn func() error) error {
	var lastErr error

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := cfg.BaseDelay * time.Duration(1<<uint(attempt-1))
			if delay > cfg.MaxDelay {
				delay = cfg.MaxDelay
			}
			log.Printf("[RETRY] Reintento %d/%d para '%s' en %v (error anterior: %v)",
				attempt, cfg.MaxRetries, operacion, delay, lastErr)

			select {
			case <-ctx.Done():
				return fmt.Errorf("contexto cancelado durante reintento de '%s': %w", operacion, ctx.Err())
			case <-time.After(delay):
			}
		}

		lastErr = fn()
		if lastErr == nil {
			if attempt > 0 {
				log.Printf("[RETRY] '%s' exitoso en intento %d", operacion, attempt+1)
			}
			return nil
		}
	}

	log.Printf("[RETRY] '%s' falló después de %d reintentos: %v", operacion, cfg.MaxRetries, lastErr)
	return fmt.Errorf("operación '%s' falló después de %d reintentos: %w", operacion, cfg.MaxRetries, lastErr)
}
