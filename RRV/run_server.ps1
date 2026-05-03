# ═══════════════════════════════════════════════════════════
# Script de Arranque - Sistema RRV Backend
# ═══════════════════════════════════════════════════════════

Write-Host "╔══════════════════════════════════════════════════════════════╗" -ForegroundColor Cyan
Write-Host "║        Iniciando Sistema RRV - Backend                     ║" -ForegroundColor Cyan
Write-Host "╚══════════════════════════════════════════════════════════════╝" -ForegroundColor Cyan
Write-Host ""

# ═══ CONFIGURACIÓN MONGODB ATLAS ═══
$env:MONGO_URI = "mongodb+srv://luxxogc_db_user:Mongo@cluster0.wftdss1.mongodb.net/?appName=Cluster0"
$env:MONGO_DB = "electoral_rrv"

# ═══ CONFIGURACIÓN DEL SERVIDOR ═══
$env:PORT = "8080"

# ═══ CONFIGURACIÓN SMS / SEGURIDAD ═══
$env:PIN_VALIDO = "1234"
$env:TELEFONOS_AUTORIZADOS = "+59170797542,+59162658425,+59170000003"

# ═══ CONFIGURACIÓN TWILIO (opcional — dejar vacío si no se usa) ═══
$env:TWILIO_ACCOUNT_SID = ""
$env:TWILIO_AUTH_TOKEN = "da80df626969ea738e0e0d5bcf57e416"
$env:TWILIO_PHONE_NUMBER = "+19786482096"
$env:TWILIO_WEBHOOK_URL = ""  # Pon tu URL de ngrok aquí si usas webhook real

# ═══ MOSTRAR CONFIGURACIÓN ═══
Write-Host "═══ Configuración ═══" -ForegroundColor Yellow
Write-Host "  MongoDB Atlas: $($env:MONGO_URI.Substring(0, 30))..." -ForegroundColor Gray
Write-Host "  Base de datos: $env:MONGO_DB" -ForegroundColor Gray
Write-Host "  Puerto: $env:PORT" -ForegroundColor Gray
Write-Host "  PIN: $env:PIN_VALIDO" -ForegroundColor Gray
Write-Host "  Telefonos: $env:TELEFONOS_AUTORIZADOS" -ForegroundColor Gray
if ($env:TWILIO_ACCOUNT_SID) {
    Write-Host "  Twilio: Configurado" -ForegroundColor Green
} else {
    Write-Host "  Twilio: No configurado (solo modo JSON)" -ForegroundColor Yellow
}
Write-Host ""

# ═══ EJECUTAR SERVIDOR ═══
Write-Host "Iniciando servidor..." -ForegroundColor Green
Write-Host "  URL: http://localhost:$env:PORT" -ForegroundColor White
Write-Host "  Health: http://localhost:$env:PORT/health" -ForegroundColor White
Write-Host ""
Write-Host "  Presiona Ctrl+C para detener" -ForegroundColor Gray
Write-Host ""

.\server.exe
