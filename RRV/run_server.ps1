# Script de Arranque - Sistema RRV Backend

Write-Host "Iniciando Sistema RRV - Backend" -ForegroundColor Cyan
Write-Host ""

# CONFIGURACION MONGODB ATLAS
$env:MONGO_URI = "mongodb+srv://luxxogc_db_user:Mongo@cluster0.wftdss1.mongodb.net/?appName=Cluster0"
$env:MONGO_DB = "electoral_rrv"

# CONFIGURACION DEL SERVIDOR
$env:PORT = "8080"

# CONFIGURACION SMS / SEGURIDAD
$env:PIN_VALIDO = "1234"
$env:TELEFONOS_AUTORIZADOS = "+59170797542,+59162658425,+59170000003"

# CONFIGURACION TWILIO (opcional)
$env:TWILIO_ACCOUNT_SID = "AC22220cb30118dcf33c12e870b12c5f67"
$env:TWILIO_AUTH_TOKEN = "da80df626969ea738e0e0d5bcf57e416"
$env:TWILIO_PHONE_NUMBER = "+19786482096"
$env:TWILIO_WEBHOOK_URL = ""

# MOSTRAR CONFIGURACION
Write-Host "=== Configuracion ===" -ForegroundColor Yellow
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

# COMPILAR (recompila siempre para aplicar cambios al codigo fuente)
Write-Host "Compilando servidor..." -ForegroundColor Yellow
$buildResult = go build -o server.exe ./cmd/server/ 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Host "ERROR de compilacion:" -ForegroundColor Red
    Write-Host $buildResult -ForegroundColor Red
    exit 1
}
Write-Host "  Compilacion exitosa" -ForegroundColor Green
Write-Host ""

# EJECUTAR SERVIDOR
Write-Host "Iniciando servidor..." -ForegroundColor Green
Write-Host "  URL: http://localhost:$env:PORT" -ForegroundColor White
Write-Host "  Health: http://localhost:$env:PORT/health" -ForegroundColor White
Write-Host ""
Write-Host "  Presiona Ctrl+C para detener" -ForegroundColor Gray
Write-Host ""

.\server.exe
