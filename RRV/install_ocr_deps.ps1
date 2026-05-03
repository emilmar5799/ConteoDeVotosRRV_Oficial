# Instalación de Dependencias para OCR - Windows
# Ejecutar este script en PowerShell como Administrador

Write-Host "═══════════════════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  Instalador de Dependencias OCR para Actas RRV" -ForegroundColor Cyan
Write-Host "═══════════════════════════════════════════════════════" -ForegroundColor Cyan
Write-Host ""

$installDir = "C:\ocr_tools"

# ─── 1. TESSERACT ─────────────────────────────────────────────
Write-Host "[1/2] TESSERACT OCR" -ForegroundColor Yellow
Write-Host ""

$tesseractCheck = Get-Command tesseract -ErrorAction SilentlyContinue
if ($tesseractCheck) {
    Write-Host "  ✅ Tesseract ya está instalado:" -ForegroundColor Green
    & tesseract --version 2>&1 | Select-Object -First 1
} else {
    Write-Host "  ❌ Tesseract NO encontrado" -ForegroundColor Red
    Write-Host ""
    Write-Host "  INSTRUCCIONES DE INSTALACIÓN MANUAL:" -ForegroundColor White
    Write-Host "  ─────────────────────────────────────" -ForegroundColor White
    Write-Host "  1. Ve a: https://github.com/UB-Mannheim/tesseract/wiki" -ForegroundColor White
    Write-Host "  2. Descarga el instalador .exe (64-bit)" -ForegroundColor White
    Write-Host "  3. Ejecuta el instalador" -ForegroundColor White
    Write-Host "  4. IMPORTANTE: En 'Choose Components', asegúrate de marcar:" -ForegroundColor Yellow
    Write-Host "     ☑ Additional language data → Spanish" -ForegroundColor Yellow
    Write-Host "  5. Anota la ruta de instalación (por defecto: C:\Program Files\Tesseract-OCR)" -ForegroundColor White
    Write-Host "  6. Agrega esa ruta al PATH del sistema:" -ForegroundColor White
    Write-Host '     [System.Environment]::SetEnvironmentVariable("PATH", $env:PATH + ";C:\Program Files\Tesseract-OCR", "User")' -ForegroundColor Gray
    Write-Host ""
}

Write-Host ""

# ─── 2. POPPLER (pdftoppm) ────────────────────────────────────
Write-Host "[2/2] POPPLER (pdftoppm para convertir PDF a imagen)" -ForegroundColor Yellow
Write-Host ""

$pdftoppmCheck = Get-Command pdftoppm -ErrorAction SilentlyContinue
if ($pdftoppmCheck) {
    Write-Host "  ✅ pdftoppm ya está instalado" -ForegroundColor Green
} else {
    Write-Host "  ❌ pdftoppm NO encontrado" -ForegroundColor Red
    Write-Host ""
    Write-Host "  INSTRUCCIONES DE INSTALACIÓN MANUAL:" -ForegroundColor White
    Write-Host "  ─────────────────────────────────────" -ForegroundColor White
    Write-Host "  1. Ve a: https://github.com/oschwartz10612/poppler-windows/releases" -ForegroundColor White
    Write-Host "  2. Descarga el archivo .zip de la última release" -ForegroundColor White
    Write-Host "  3. Extrae el ZIP en C:\poppler (o donde prefieras)" -ForegroundColor White
    Write-Host "  4. Agrega la carpeta bin al PATH:" -ForegroundColor White
    Write-Host '     [System.Environment]::SetEnvironmentVariable("PATH", $env:PATH + ";C:\poppler\Library\bin", "User")' -ForegroundColor Gray
    Write-Host ""
    
    # Intentar descargar automáticamente
    Write-Host "  ¿Deseas intentar la descarga automática? (requiere internet)" -ForegroundColor Cyan
    $response = Read-Host "  Escribe 'si' para descargar"
    
    if ($response -eq "si") {
        Write-Host "  Descargando Poppler..." -ForegroundColor Yellow
        $popplerUrl = "https://github.com/oschwartz10612/poppler-windows/releases/download/v24.08.0-0/Release-24.08.0-0.zip"
        $zipPath = "$env:TEMP\poppler.zip"
        $extractPath = "C:\poppler"
        
        try {
            Invoke-WebRequest -Uri $popplerUrl -OutFile $zipPath -UseBasicParsing
            Write-Host "  Extrayendo..." -ForegroundColor Yellow
            Expand-Archive -Path $zipPath -DestinationPath $extractPath -Force
            
            # Buscar la carpeta bin
            $binPath = Get-ChildItem -Path $extractPath -Recurse -Directory -Filter "bin" | Select-Object -First 1
            if ($binPath) {
                $fullBinPath = $binPath.FullName
                [System.Environment]::SetEnvironmentVariable("PATH", $env:PATH + ";$fullBinPath", "User")
                $env:PATH += ";$fullBinPath"
                Write-Host "  ✅ Poppler instalado en: $fullBinPath" -ForegroundColor Green
                Write-Host "  ✅ Agregado al PATH" -ForegroundColor Green
            }
            
            Remove-Item $zipPath -Force
        } catch {
            Write-Host "  ❌ Error en descarga automática: $_" -ForegroundColor Red
            Write-Host "  Por favor descarga manualmente desde el link de arriba" -ForegroundColor Yellow
        }
    }
}

Write-Host ""
Write-Host "═══════════════════════════════════════════════════════" -ForegroundColor Cyan
Write-Host "  VERIFICACIÓN FINAL" -ForegroundColor Cyan
Write-Host "═══════════════════════════════════════════════════════" -ForegroundColor Cyan
Write-Host ""

# Verificar todo
$allGood = $true

$t = Get-Command tesseract -ErrorAction SilentlyContinue
if ($t) {
    Write-Host "  ✅ tesseract: $($t.Source)" -ForegroundColor Green
    
    # Verificar idioma español
    $langs = & tesseract --list-langs 2>&1
    if ($langs -match "spa") {
        Write-Host "  ✅ Idioma español (spa) disponible" -ForegroundColor Green
    } else {
        Write-Host "  ⚠️  Idioma español (spa) NO encontrado" -ForegroundColor Yellow
        Write-Host "     Reinstala Tesseract y marca 'Spanish' en los idiomas" -ForegroundColor Yellow
        $allGood = $false
    }
} else {
    Write-Host "  ❌ tesseract: NO ENCONTRADO" -ForegroundColor Red
    $allGood = $false
}

$p = Get-Command pdftoppm -ErrorAction SilentlyContinue
if ($p) {
    Write-Host "  ✅ pdftoppm: $($p.Source)" -ForegroundColor Green
} else {
    Write-Host "  ❌ pdftoppm: NO ENCONTRADO" -ForegroundColor Red
    $allGood = $false
}

Write-Host ""
if ($allGood) {
    Write-Host "  🎉 ¡Todo listo! Puedes ejecutar el procesador OCR:" -ForegroundColor Green
    Write-Host '  cd "c:\Users\Usuario\Documents\GAME JAM\ConteoDeVotosRRV_Oficial\RRV"' -ForegroundColor Gray
    Write-Host "  go run cmd/ocr_processor/main.go pdf --verbose --max=5" -ForegroundColor Gray
} else {
    Write-Host "  ⚠️  Faltan dependencias. Instálalas y vuelve a ejecutar este script." -ForegroundColor Yellow
    Write-Host "  IMPORTANTE: Reinicia la terminal después de instalar." -ForegroundColor Yellow
}
Write-Host ""
