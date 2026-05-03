# ═══════════════════════════════════════════════════════════════
# test_all.ps1 — Prueba completa del Sistema RRV
# Ejecutar con: .\test_all.ps1
# Requiere que el servidor esté corriendo (.\run_server.ps1 en otra terminal)
# ═══════════════════════════════════════════════════════════════

$base = "http://localhost:8080"
$ok = 0
$fail = 0

function Test-Endpoint {
    param(
        [string]$Name,
        [string]$Method = "GET",
        [string]$Url,
        [hashtable]$Body = $null,
        [int]$ExpectedStatus = 200
    )
    try {
        $params = @{ Uri = $Url; Method = $Method; TimeoutSec = 10 }
        if ($Body) {
            $params.Body = ($Body | ConvertTo-Json)
            $params.ContentType = "application/json"
        }
        $resp = Invoke-WebRequest @params -ErrorAction Stop
        $status = $resp.StatusCode
    } catch {
        $status = $_.Exception.Response.StatusCode.value__
        if (-not $status) { $status = 0 }
    }

    $icon  = if ($status -eq $ExpectedStatus) { "[OK]" } else { "[FAIL]" }
    $color = if ($status -eq $ExpectedStatus) { "Green" } else { "Red" }
    Write-Host ("  {0,-6} {1,-55} {2} (esperado {3})" -f $icon, $Name, $status, $ExpectedStatus) -ForegroundColor $color

    if ($status -eq $ExpectedStatus) { $script:ok++ } else { $script:fail++ }
}

Write-Host ""
Write-Host "╔══════════════════════════════════════════════════════════╗" -ForegroundColor Cyan
Write-Host "║         Sistema RRV — Suite de Pruebas Completa         ║" -ForegroundColor Cyan
Write-Host "╚══════════════════════════════════════════════════════════╝" -ForegroundColor Cyan
Write-Host ""

# ─── 1. HEALTH ────────────────────────────────────────────────
Write-Host "[ Health ]" -ForegroundColor Yellow
Test-Endpoint "Health check" -Url "$base/health"

# ─── 2. COMMANDS — SMS ────────────────────────────────────────
Write-Host ""
Write-Host "[ SMS — Commands ]" -ForegroundColor Yellow

Test-Endpoint "SMS válido Chuquisaca" -Method POST -Url "$base/api/rrv/sms" -ExpectedStatus 201 -Body @{
    telefono = "+59170000001"
    mensaje  = "ACTA:MESA-TEST01|DEP:Chuquisaca|MUN:Sucre|REC:U.E. Santa Monica|MESA:1|VAL:515|NUL:10|BLA:5|C1:200|C2:180|C3:130|C4:5|PIN:1234"
}

Test-Endpoint "SMS válido La Paz" -Method POST -Url "$base/api/rrv/sms" -ExpectedStatus 201 -Body @{
    telefono = "+59170000001"
    mensaje  = "ACTA:MESA-TEST02|DEP:La Paz|MUN:El Alto|REC:Colegio Ayacucho|MESA:2|VAL:620|NUL:15|BLA:20|C1:150|C2:130|C3:120|C4:220|PIN:1234"
}

Test-Endpoint "SMS válido Santa Cruz" -Method POST -Url "$base/api/rrv/sms" -ExpectedStatus 201 -Body @{
    telefono = "+59170000001"
    mensaje  = "ACTA:MESA-TEST03|DEP:Santa Cruz|MUN:Warnes|REC:U.E. San Martin|MESA:3|VAL:800|NUL:20|BLA:10|C1:400|C2:250|C3:100|C4:50|PIN:1234"
}

Test-Endpoint "SMS — PIN inválido (espera 401)" -Method POST -Url "$base/api/rrv/sms" -ExpectedStatus 401 -Body @{
    telefono = "+59170000001"
    mensaje  = "ACTA:MESA-TEST99|DEP:Oruro|MUN:Oruro|MESA:99|VAL:100|NUL:5|BLA:3|C1:50|C2:47|PIN:0000"
}

Test-Endpoint "SMS — teléfono no autorizado (espera 403)" -Method POST -Url "$base/api/rrv/sms" -ExpectedStatus 403 -Body @{
    telefono = "+59199999999"
    mensaje  = "ACTA:MESA-TEST98|DEP:Beni|MUN:Trinidad|MESA:98|VAL:200|NUL:10|BLA:5|C1:100|C2:95|PIN:1234"
}

Test-Endpoint "SMS — duplicado (espera 409)" -Method POST -Url "$base/api/rrv/sms" -ExpectedStatus 409 -Body @{
    telefono = "+59170000001"
    mensaje  = "ACTA:MESA-TEST01|DEP:Chuquisaca|MUN:Sucre|REC:U.E. Santa Monica|MESA:1|VAL:515|NUL:10|BLA:5|C1:200|C2:180|C3:130|C4:5|PIN:1234"
}

# ─── 3. QUERY — LISTADOS ──────────────────────────────────────
Write-Host ""
Write-Host "[ Query — Listados básicos ]" -ForegroundColor Yellow

Test-Endpoint "Listar actas"                  -Url "$base/api/rrv/actas"
Test-Endpoint "Filtrar por departamento"      -Url "$base/api/rrv/actas?departamento=Chuquisaca"
Test-Endpoint "Filtrar por estado PROCESADA"  -Url "$base/api/rrv/actas?estado=PROCESADA"
Test-Endpoint "Filtrar por tipo SMS"          -Url "$base/api/rrv/actas?tipo_entrada=SMS"
Test-Endpoint "Obtener acta por ID"           -Url "$base/api/rrv/actas/MESA-TEST01"
Test-Endpoint "Acta inexistente (espera 404)" -Url "$base/api/rrv/actas/NO-EXISTE" -ExpectedStatus 404
Test-Endpoint "Listar eventos"                -Url "$base/api/rrv/eventos"
Test-Endpoint "Replay acta MESA-TEST01"       -Url "$base/api/rrv/eventos/replay/MESA-TEST01"
Test-Endpoint "Listar SMS recibidos"          -Url "$base/api/rrv/sms"
Test-Endpoint "Listar inconsistencias"        -Url "$base/api/rrv/inconsistencias"
Test-Endpoint "Stats básico"                  -Url "$base/api/rrv/stats"
Test-Endpoint "Stats CQRS completo"           -Url "$base/api/rrv/stats/full"
Test-Endpoint "Referencia totales"            -Url "$base/api/rrv/referencia/totales"

# ─── 4. BANCO DE CONSULTAS ────────────────────────────────────
Write-Host ""
Write-Host "[ Banco de Consultas — Dashboard ]" -ForegroundColor Yellow

Test-Endpoint "Q1  — Mesas por recinto"              -Url "$base/api/rrv/consultas/mesas-por-recinto"
Test-Endpoint "Q2  — Votos por municipio"            -Url "$base/api/rrv/consultas/votos-por-municipio"
Test-Endpoint "Q3  — Votos por departamento"         -Url "$base/api/rrv/consultas/votos-por-departamento"
Test-Endpoint "Q4  — Top 5 recintos P1"              -Url "$base/api/rrv/consultas/top-recintos?candidato=P1&limite=5"
Test-Endpoint "Q4  — Top 5 recintos P2"              -Url "$base/api/rrv/consultas/top-recintos?candidato=P2&limite=5"
Test-Endpoint "Q4  — Candidato inválido (espera 400)"-Url "$base/api/rrv/consultas/top-recintos?candidato=P9" -ExpectedStatus 400
Test-Endpoint "Q5  — Nulos por departamento"         -Url "$base/api/rrv/consultas/nulos-por-departamento"
Test-Endpoint "Q6  — Boletas anuladas TREP"          -Url "$base/api/rrv/consultas/boletas-anuladas"
Test-Endpoint "Q7-8 — TREP vs Oficial"               -Url "$base/api/rrv/consultas/trep-vs-oficial"
Test-Endpoint "Q11 — Mesas >20% abstención"          -Url "$base/api/rrv/consultas/mesas-abstencion?umbral=20"
Test-Endpoint "Q12 — Actas por hora"                 -Url "$base/api/rrv/consultas/actas-por-hora"
Test-Endpoint "Q14 — Tiempo actas por departamento"  -Url "$base/api/rrv/consultas/tiempo-actas-departamento"
Test-Endpoint "Q16 — Participación por departamento" -Url "$base/api/rrv/consultas/participacion-departamento"
Test-Endpoint "Q17 — Inconsistencias TREP-Oficial"   -Url "$base/api/rrv/consultas/inconsistencias-trep-oficial"
Test-Endpoint "Q19 — Resultados por departamento"    -Url "$base/api/rrv/consultas/resultados-geograficos?departamento=Chuquisaca"
Test-Endpoint "Q19 — Resultados por municipio"       -Url "$base/api/rrv/consultas/resultados-geograficos?municipio=Sucre"
Test-Endpoint "Q19 — Resultados globales"            -Url "$base/api/rrv/consultas/resultados-geograficos"
Test-Endpoint "Q20 — Errores más comunes"            -Url "$base/api/rrv/consultas/errores-comunes"

# ─── RESULTADO FINAL ──────────────────────────────────────────
Write-Host ""
Write-Host "══════════════════════════════════════════════════════════" -ForegroundColor Cyan
$total = $ok + $fail
Write-Host ("  Resultado: {0}/{1} pruebas pasaron" -f $ok, $total) -ForegroundColor $(if ($fail -eq 0) { "Green" } else { "Yellow" })
if ($fail -gt 0) {
    Write-Host ("  {0} prueba(s) fallaron — revisa los [FAIL] de arriba" -f $fail) -ForegroundColor Red
} else {
    Write-Host "  Todas las pruebas pasaron!" -ForegroundColor Green
}
Write-Host "══════════════════════════════════════════════════════════" -ForegroundColor Cyan
Write-Host ""
