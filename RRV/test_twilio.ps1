# ═══════════════════════════════════════════════════════════
# Script de Prueba - Integración Twilio SMS con Sistema RRV
# ═══════════════════════════════════════════════════════════

Write-Host "╔══════════════════════════════════════════════════════════════╗" -ForegroundColor Cyan
Write-Host "║   Pruebas de Integración Twilio SMS - Sistema RRV          ║" -ForegroundColor Cyan
Write-Host "╚══════════════════════════════════════════════════════════════╝" -ForegroundColor Cyan
Write-Host ""

# ═══ CONFIGURACIÓN ═══
# Cambia estos valores según tu entorno

$env:TWILIO_ACCOUNT_SID = "AC22220cb30118dcf33c12e870b12"
$env:TWILIO_AUTH_TOKEN = "30df626969ea738e0e0d5bcf57e416"
$env:TWILIO_PHONE_NUMBER = "+19786482096"
$env:TELEFONOS_AUTORIZADOS = "+59170000001,+59170000002,+59170000003"
$env:PIN_VALIDO = "1234"
$env:MONGO_URI = "mongodb+srv://luxxogc_db_user:Mongo@cluster0.wftdss1.mongodb.net/?appName=Cluster0"  # Cambia por tu URI de MongoDB Atlas
$env:MONGO_DB = "electoral_rrv"

# URL base del servidor
$BASE_URL = "http://localhost:8080"

Write-Host "═══ Configuración ═══" -ForegroundColor Yellow
Write-Host "  Twilio SID: $($env:TWILIO_ACCOUNT_SID.Substring(0,8))..." -ForegroundColor Gray
Write-Host "  Twilio Phone: $env:TWILIO_PHONE_NUMBER" -ForegroundColor Gray
Write-Host "  MongoDB: $env:MONGO_DB" -ForegroundColor Gray
Write-Host ""

# ═══════════════════════════════════════════════
# PRUEBA 1: Health Check
# ═══════════════════════════════════════════════
Write-Host "═══ PRUEBA 1: Health Check ═══" -ForegroundColor Green
try {
    $response = Invoke-RestMethod -Uri "$BASE_URL/health" -Method GET
    Write-Host "  Status: $($response.status)" -ForegroundColor White
    Write-Host "  Twilio: $($response.twilio)" -ForegroundColor White
    Write-Host "  ✓ Health check OK" -ForegroundColor Green
} catch {
    Write-Host "  ✗ Servidor no disponible. Ejecuta primero: go run ./cmd/server" -ForegroundColor Red
    Write-Host "  Error: $_" -ForegroundColor Red
    exit 1
}
Write-Host ""

# ═══════════════════════════════════════════════
# PRUEBA 2: SMS via JSON (endpoint original)
# ═══════════════════════════════════════════════
Write-Host "═══ PRUEBA 2: SMS via JSON (teléfono autorizado) ═══" -ForegroundColor Green

$smsBody = @{
    telefono = "+59170000001"
    mensaje = "ACTA:MESA-35000|DEP:Chuquisaca|MUN:Sucre|REC:U.E. Santa Monica|MESA:1|VAL:500|NUL:10|BLA:5|C1:200|C2:150|C3:100|C4:50|PIN:1234"
} | ConvertTo-Json

try {
    $response = Invoke-RestMethod -Uri "$BASE_URL/api/rrv/sms" `
        -Method POST `
        -ContentType "application/json" `
        -Body $smsBody
    Write-Host "  Success: $($response.success)" -ForegroundColor White
    Write-Host "  Message: $($response.message)" -ForegroundColor White
    Write-Host "  ✓ SMS procesado correctamente" -ForegroundColor Green
} catch {
    $statusCode = $_.Exception.Response.StatusCode.value__
    Write-Host "  Status: $statusCode" -ForegroundColor Yellow
    Write-Host "  (puede ser duplicado si ya se ejecutó antes)" -ForegroundColor Gray
}
Write-Host ""

# ═══════════════════════════════════════════════
# PRUEBA 3: SMS via JSON (teléfono NO autorizado)
# ═══════════════════════════════════════════════
Write-Host "═══ PRUEBA 3: SMS via JSON (teléfono NO autorizado) ═══" -ForegroundColor Green

$smsBody2 = @{
    telefono = "+59199999999"
    mensaje = "ACTA:MESA-99999|DEP:La Paz|MUN:La Paz|MESA:99|VAL:100|NUL:5|BLA:3|C1:50|C2:50|PIN:1234"
} | ConvertTo-Json

try {
    $response = Invoke-RestMethod -Uri "$BASE_URL/api/rrv/sms" `
        -Method POST `
        -ContentType "application/json" `
        -Body $smsBody2
    Write-Host "  ✗ Debería haber sido rechazado" -ForegroundColor Red
} catch {
    $statusCode = $_.Exception.Response.StatusCode.value__
    if ($statusCode -eq 403) {
        Write-Host "  Status: 403 Forbidden" -ForegroundColor White
        Write-Host "  ✓ Teléfono no autorizado rechazado correctamente" -ForegroundColor Green
    } else {
        Write-Host "  Status: $statusCode" -ForegroundColor Yellow
    }
}
Write-Host ""

# ═══════════════════════════════════════════════
# PRUEBA 4: SMS con PIN inválido
# ═══════════════════════════════════════════════
Write-Host "═══ PRUEBA 4: SMS con PIN inválido ═══" -ForegroundColor Green

$smsBody3 = @{
    telefono = "+59170000001"
    mensaje = "ACTA:MESA-88888|DEP:Cochabamba|MUN:Cochabamba|MESA:88|VAL:300|NUL:8|BLA:2|C1:150|C2:150|PIN:9999"
} | ConvertTo-Json

try {
    $response = Invoke-RestMethod -Uri "$BASE_URL/api/rrv/sms" `
        -Method POST `
        -ContentType "application/json" `
        -Body $smsBody3
    Write-Host "  ✗ Debería haber sido rechazado" -ForegroundColor Red
} catch {
    $statusCode = $_.Exception.Response.StatusCode.value__
    if ($statusCode -eq 401) {
        Write-Host "  Status: 401 Unauthorized" -ForegroundColor White
        Write-Host "  ✓ PIN inválido rechazado correctamente" -ForegroundColor Green
    } else {
        Write-Host "  Status: $statusCode" -ForegroundColor Yellow
    }
}
Write-Host ""

# ═══════════════════════════════════════════════
# PRUEBA 5: Simulación de Webhook Twilio
# ═══════════════════════════════════════════════
Write-Host "═══ PRUEBA 5: Simulación de Webhook Twilio ═══" -ForegroundColor Green

$twilioParams = @{
    From = "+59170000002"
    Body = "ACTA:MESA-34999|DEP:Chuquisaca|MUN:Yotala|REC:U.E. Padresama|MESA:2|VAL:450|NUL:8|BLA:3|C1:180|C2:120|C3:90|C4:60|PIN:1234"
    MessageSid = "SM" + [guid]::NewGuid().ToString("N").Substring(0, 30)
    AccountSid = $env:TWILIO_ACCOUNT_SID
    NumMedia = "0"
    ToCountry = "BO"
    FromCountry = "BO"
}

# Construir form body
$formBody = ($twilioParams.GetEnumerator() | ForEach-Object { "$($_.Key)=$([System.Web.HttpUtility]::UrlEncode($_.Value))" }) -join "&"

try {
    # Sin firma válida (en desarrollo se acepta si TWILIO_WEBHOOK_URL está vacío)
    $response = Invoke-WebRequest -Uri "$BASE_URL/api/rrv/sms/twilio" `
        -Method POST `
        -ContentType "application/x-www-form-urlencoded" `
        -Body $formBody
    Write-Host "  Status: $($response.StatusCode)" -ForegroundColor White
    Write-Host "  Content-Type: $($response.Headers['Content-Type'])" -ForegroundColor White
    Write-Host "  Body: $($response.Content.Substring(0, [Math]::Min(200, $response.Content.Length)))" -ForegroundColor Gray
    Write-Host "  ✓ Webhook Twilio procesado" -ForegroundColor Green
} catch {
    $statusCode = $_.Exception.Response.StatusCode.value__
    Write-Host "  Status: $statusCode" -ForegroundColor Yellow
    Write-Host "  (puede ser duplicado o error de firma)" -ForegroundColor Gray
}
Write-Host ""

# ═══════════════════════════════════════════════
# PRUEBA 6: Listar SMS recibidos
# ═══════════════════════════════════════════════
Write-Host "═══ PRUEBA 6: Listar SMS recibidos ═══" -ForegroundColor Green

try {
    $response = Invoke-RestMethod -Uri "$BASE_URL/api/rrv/sms" -Method GET
    $smsCount = $response.data.Count
    Write-Host "  Total SMS registrados: $smsCount" -ForegroundColor White
    foreach ($sms in $response.data) {
        Write-Host "    [$($sms.estado)] $($sms.telefono) → $($sms.acta_id)" -ForegroundColor Gray
    }
    Write-Host "  ✓ Listado OK" -ForegroundColor Green
} catch {
    Write-Host "  ✗ Error listando SMS: $_" -ForegroundColor Red
}
Write-Host ""

# ═══════════════════════════════════════════════
# PRUEBA 7: Estadísticas
# ═══════════════════════════════════════════════
Write-Host "═══ PRUEBA 7: Estadísticas RRV ═══" -ForegroundColor Green

try {
    $response = Invoke-RestMethod -Uri "$BASE_URL/api/rrv/stats" -Method GET
    $stats = $response.data
    Write-Host "  Total actas: $($stats.total_actas)" -ForegroundColor White
    Write-Host "  Procesadas: $($stats.actas_procesadas)" -ForegroundColor White
    Write-Host "  Con error: $($stats.actas_error)" -ForegroundColor White
    Write-Host "  Votos válidos: $($stats.total_votos_validos)" -ForegroundColor White
    Write-Host "  ✓ Estadísticas OK" -ForegroundColor Green
} catch {
    Write-Host "  ✗ Error: $_" -ForegroundColor Red
}

Write-Host ""
Write-Host "═══════════════════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  Pruebas completadas" -ForegroundColor Cyan
Write-Host ""
Write-Host "  Para probar con SMS real:" -ForegroundColor Yellow
Write-Host "    1. Instala ngrok: choco install ngrok" -ForegroundColor Gray
Write-Host "    2. Ejecuta: ngrok http 8080" -ForegroundColor Gray
Write-Host "    3. Copia la URL https de ngrok" -ForegroundColor Gray
Write-Host "    4. En Twilio Console > Phone Numbers > tu número:" -ForegroundColor Gray
Write-Host "       Configura Webhook URL: https://xxxx.ngrok.io/api/rrv/sms/twilio" -ForegroundColor Gray
Write-Host "    5. Envía un SMS al +19786482096 con el formato:" -ForegroundColor Gray
Write-Host "       ACTA:MESA-35000|DEP:Chuquisaca|MUN:Sucre|MESA:1|VAL:500|NUL:10|BLA:5|C1:200|C2:150|C3:100|PIN:1234" -ForegroundColor Gray
Write-Host "═══════════════════════════════════════════════════════" -ForegroundColor Cyan
