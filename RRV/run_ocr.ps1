# ═══════════════════════════════════════════════════════════
# Script OCR Processor - Sistema RRV
# Uso: powershell -ExecutionPolicy Bypass -File ".\run_ocr.ps1"
# Opcional: .\run_ocr.ps1 -Dir "pdf" -Verbose -Debug -Max 10
# ═══════════════════════════════════════════════════════════

param(
    [string]$Dir     = "pdf",
    [switch]$Verbose,
    [switch]$Debug,
    [switch]$SkipVisual,
    [int]$Max        = 0
)

Write-Host "╔══════════════════════════════════════════════════════════════╗" -ForegroundColor Cyan
Write-Host "║        OCR Processor — Actas Electorales RRV               ║" -ForegroundColor Cyan
Write-Host "╚══════════════════════════════════════════════════════════════╝" -ForegroundColor Cyan
Write-Host ""

# ═══ CONFIGURACIÓN MONGODB ATLAS ═══
$env:MONGO_URI = "mongodb+srv://luxxogc_db_user:Mongo@cluster0.wftdss1.mongodb.net/?appName=Cluster0"
$env:MONGO_DB  = "electoral_rrv"

# ═══ CONFIGURACIÓN SMS / SEGURIDAD ═══
$env:PIN_VALIDO             = "1234"
$env:TELEFONOS_AUTORIZADOS  = "+59170797542,+59162658425,+59170000003"

Write-Host "  MongoDB Atlas: $($env:MONGO_URI.Substring(0,40))..." -ForegroundColor Gray
Write-Host "  Base de datos: $env:MONGO_DB"                        -ForegroundColor Gray
Write-Host "  Directorio PDF: $Dir"                                 -ForegroundColor Gray
Write-Host ""

# Construir argumentos
$args_list = @($Dir)
if ($Verbose)    { $args_list += "--verbose"     }
if ($Debug)      { $args_list += "--debug"       }
if ($SkipVisual) { $args_list += "--skip-visual" }
if ($Max -gt 0)  { $args_list += "--max=$Max"    }

Write-Host "Ejecutando OCR Processor..." -ForegroundColor Green
Write-Host ""

go run cmd/ocr_processor/main.go @args_list
