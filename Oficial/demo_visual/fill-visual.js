// fill-visual.js
// ============================================================
//  DEMO VISUAL: Rellena el formulario automáticamente con
//  un navegador real que puedes ver en pantalla.
//
//  Uso:
//    node fill-visual.js             → TRUNCA y procesa TODOS los registros
//    node fill-visual.js --limit 20  → TRUNCA y procesa solo los primeros 20
// ============================================================

const puppeteer = require('puppeteer');
const fs = require('fs');
const { parse } = require('csv-parse/sync');

const CSV_PATH = '../_Recursos Practica 4 - Transcripciones.csv';
const FRONTEND = 'http://localhost:3000';
const API_BASE = 'http://localhost:8080/api';
const FUNCIONARIO = 'Funcionario 1';

// Velocidad visual (ms entre teclas). Sube para que se vea mejor.
const SLOWMO = 5;

// Pausa entre actas (ms)
const PAUSA_ENTRE_ACTAS = 500;

// ─── Argumentos ────────────────────────────────────────────
const args = process.argv.slice(2);
let limit = Infinity;
const limitIdx = args.indexOf('--limit');
if (limitIdx !== -1 && args[limitIdx + 1]) {
  limit = parseInt(args[limitIdx + 1]);
}

// ─── Helpers ───────────────────────────────────────────────
// Limpiar y escribir en un input de React de forma confiable.
async function setField(page, selector, value) {
  await page.$eval(selector, (el, val) => {
    const nativeInputSetter = Object.getOwnPropertyDescriptor(
      window.HTMLInputElement.prototype,
      'value'
    ).set;
    nativeInputSetter.call(el, val);
    el.dispatchEvent(new Event('input', { bubbles: true }));
    el.dispatchEvent(new Event('change', { bubbles: true }));
  }, String(value));
}

// Lo mismo para textarea
async function setTextarea(page, selector, value) {
  await page.$eval(selector, (el, val) => {
    const nativeSetter = Object.getOwnPropertyDescriptor(
      window.HTMLTextAreaElement.prototype,
      'value'
    ).set;
    nativeSetter.call(el, val);
    el.dispatchEvent(new Event('input', { bubbles: true }));
    el.dispatchEvent(new Event('change', { bubbles: true }));
  }, String(value));
}

function sleep(ms) {
  return new Promise(r => setTimeout(r, ms));
}

// ─── Truncar transcripciones existentes via API ────────────
async function truncarTranscripciones() {
  console.log('🗑️  Truncando transcripciones existentes...');
  const res = await fetch(`${API_BASE}/transcripciones`, { method: 'DELETE' });
  if (!res.ok) {
    const body = await res.text();
    throw new Error(`Error al truncar: HTTP ${res.status} — ${body}`);
  }
  const data = await res.json();
  console.log(`✅  ${data.message}\n`);
}

// ─── Main ──────────────────────────────────────────────────
async function run() {
  console.log('╔══════════════════════════════════════════════════╗');
  console.log('║   DEMO VISUAL – Transcripción Automática de Actas ║');
  console.log('╚══════════════════════════════════════════════════╝\n');

  // PASO 1: Truncar datos previos via API (sin abrir navegador)
  await truncarTranscripciones();

  // PASO 2: Leer CSV
  const rawCsv = fs.readFileSync(CSV_PATH, 'utf-8');
  const rows = parse(rawCsv, {
    columns: true,
    skip_empty_lines: true,
    relax_column_count: true,
    trim: true
  }).filter(r => r.CodigoActa);

  const total = Math.min(rows.length, limit);
  console.log(`📄 Total de registros a procesar: ${total}${limit !== Infinity ? ` (limitado a ${limit})` : ' (todos)'}\n`);

  // PASO 3: Lanzar navegador VISIBLE
  const browser = await puppeteer.launch({
    headless: false,
    slowMo: SLOWMO,
    defaultViewport: null,
    args: ['--start-maximized', '--disable-infobars']
  });

  const page = await browser.newPage();
  await page.goto(FRONTEND, { waitUntil: 'networkidle0', timeout: 30000 });
  console.log('🌐 Abriendo formulario en', FRONTEND);

  // Esperar que cargue el formulario
  await page.waitForSelector('input[name="funcionario"]');

  // Llenar el nombre del funcionario UNA SOLA VEZ
  await setField(page, 'input[name="funcionario"]', FUNCIONARIO);
  console.log(`👤 Funcionario: ${FUNCIONARIO}\n`);
  await sleep(400);

  let exitosos = 0;
  let omitidos = 0;
  let errores = 0;

  for (let i = 0; i < total; i++) {
    const row = rows[i];
    const codigoActa = row.CodigoActa || '';
    const codigoRecinto = row.CodigoRecinto || '';
    const nroMesa = row.NroMesa || '';

    console.log(`[${i + 1}/${total}] 📋 Acta: ${codigoActa} | Recinto: ${codigoRecinto} | Mesa: ${nroMesa}`);

    // ── Datos del Acta ──────────────────────────────────
    await setField(page, 'input[name="codigoActa"]', codigoActa);
    await sleep(30);
    await setField(page, 'input[name="codigoRecinto"]', codigoRecinto);
    await sleep(30);
    await setField(page, 'input[name="nroMesa"]', nroMesa);
    await sleep(30);

    // ── Votos por Partido ───────────────────────────────
    await setField(page, 'input[name="p1"]', row.P1 || '0');
    await sleep(20);
    await setField(page, 'input[name="p2"]', row.P2 || '0');
    await sleep(20);
    await setField(page, 'input[name="p3"]', row.P3 || '0');
    await sleep(20);
    await setField(page, 'input[name="p4"]', row.P4 || '0');
    await sleep(20);
    await setField(page, 'input[name="votosValidos"]', row.VotosValidos || '0');
    await sleep(20);

    // ── Otros Votos ─────────────────────────────────────
    await setField(page, 'input[name="blancos"]', row.VotosBlancos || '0');
    await sleep(20);
    await setField(page, 'input[name="nulos"]', row.VotosNulos || '0');
    await sleep(20);
    await setField(page, 'input[name="anfora"]', row.PapeletasAnfora || '0');
    await sleep(20);
    await setField(page, 'input[name="noUtilizadas"]', row.PapeltasNoUtilizadas || '0');
    await sleep(20);

    // ── Horarios ────────────────────────────────────────
    await setField(page, 'input[name="aperturaHora"]', row.AperturaHora || '8');
    await sleep(20);
    await setField(page, 'input[name="aperturaMinutos"]', row.AperturaMinutos || '0');
    await sleep(20);
    await setField(page, 'input[name="cierreHora"]', row.CierreHora || '16');
    await sleep(20);
    await setField(page, 'input[name="cierreMinutos"]', row.CierreMinutos || '0');
    await sleep(20);

    // ── Observaciones ───────────────────────────────────
    const obs = (row.Observaciones || '').trim();
    await setTextarea(page, 'textarea[name="observaciones"]', obs);
    await sleep(50);

    // ── Enviar ──────────────────────────────────────────
    const submitBtn = await page.$('button[type="submit"]');
    if (!submitBtn) {
      console.log(`  ⚠️  No se encontró el botón de envío.`);
      errores++;
      continue;
    }

    const disabled = await page.evaluate(el => el.disabled, submitBtn);
    if (disabled) {
      console.log(`  ⚠️  Botón deshabilitado (inconsistencia matemática). Omitida.`);
      omitidos++;
    } else {
      await submitBtn.click();

      try {
        await page.waitForSelector('.alert-success, .alert-error', { timeout: 6000 });

        const isSuccess = await page.$('.alert-success');
        if (isSuccess) {
          console.log(`  ✅ Transcrita correctamente.`);
          exitosos++;
        } else {
          const msg = await page.$eval('.alert-error span', el => el.textContent)
            .catch(() => 'Error desconocido');

          if (msg.includes('ya fue transcrita')) {
            console.log(`  ⚠️  Ya estaba transcrita, omitida.`);
            omitidos++;
          } else {
            console.log(`  ❌ Error del servidor: ${msg}`);
            errores++;
          }
        }
      } catch {
        // Timeout esperando el alert
        const submitErrEl = await page.$('.alert-error span');
        if (submitErrEl) {
          const msg = await page.evaluate(el => el.textContent, submitErrEl);
          if (msg.includes('ya fue transcrita')) {
            console.log(`  ⚠️  Ya estaba transcrita, omitida.`);
            omitidos++;
          } else {
            console.log(`  ❌ Error: ${msg}`);
            errores++;
          }
        } else {
          console.log(`  ❓ Sin respuesta clara del formulario.`);
          errores++;
        }
      }
    }

    await sleep(PAUSA_ENTRE_ACTAS);
  }

  console.log('\n╔══════════════════════════════╗');
  console.log('║         RESUMEN FINAL        ║');
  console.log('╠══════════════════════════════╣');
  console.log(`║  ✅ Exitosas  : ${String(exitosos).padEnd(12)}║`);
  console.log(`║  ⚠️  Omitidas : ${String(omitidos).padEnd(12)}║`);
  console.log(`║  ❌ Errores   : ${String(errores).padEnd(12)}║`);
  console.log('╚══════════════════════════════╝');
  console.log('\n⌛ El navegador permanecerá abierto. Ciérralo manualmente.');
}

run().catch(err => {
  console.error('💥 Error fatal:', err.message);
  process.exit(1);
});
