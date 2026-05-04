// fill-visual.js
// ============================================================
//  DEMO VISUAL: Rellena el formulario automáticamente con
//  un navegador real que puedes ver en pantalla.
//
//  Trabaja con 10 páginas en PARALELO, dividiendo el CSV
//  en 10 partes iguales y reutilizando los 4 usuarios
//  de forma circular entre las páginas.
//
//  Uso:
//    node fill-visual.js             → TRUNCA y procesa TODOS los registros
//    node fill-visual.js --limit 20  → TRUNCA y procesa solo los primeros 20
// ============================================================

const puppeteer = require('puppeteer');
const fs        = require('fs');
const { parse } = require('csv-parse/sync');

const CSV_PATH  = '../_Recursos Practica 4 - Transcripciones.csv';
const FRONTEND  = 'http://localhost:3000';
const API_BASE  = 'http://localhost:8080/api';

// 4 usuarios disponibles – se reutilizan cíclicamente entre las 10 páginas
const FUNCIONARIOS = [
  'Funcionario 1',
  'Funcionario 2',
  'Funcionario 3',
  'Admin',
];

// Número de páginas paralelas
const NUM_PAGINAS = 10;

// Velocidad visual (ms entre eventos). Sube para que se vea mejor.
const SLOWMO = 5;

// Pausa entre actas por página (ms)
const PAUSA_ENTRE_ACTAS = 300;

// ─── Argumentos ────────────────────────────────────────────
const args     = process.argv.slice(2);
let   limit    = Infinity;
const limitIdx = args.indexOf('--limit');
if (limitIdx !== -1 && args[limitIdx + 1]) {
  limit = parseInt(args[limitIdx + 1]);
}

// ─── Helpers ───────────────────────────────────────────────
function sleep(ms) {
  return new Promise(r => setTimeout(r, ms));
}

/** Reintenta una función async hasta N veces ante errores de protocolo */
async function retry(fn, intentos = 3, espera = 800) {
  for (let i = 0; i < intentos; i++) {
    try {
      return await fn();
    } catch (err) {
      const esProtocolo = err.message && (
        err.message.includes('timed out') ||
        err.message.includes('Target closed') ||
        err.message.includes('Session closed')
      );
      if (esProtocolo && i < intentos - 1) {
        console.warn(`  ⚡ Reintento ${i + 1}/${intentos - 1} por timeout de protocolo...`);
        await sleep(espera * (i + 1));
      } else {
        throw err;
      }
    }
  }
}

/** Divide un array en N chunks lo más iguales posible */
function chunkArray(arr, n) {
  const chunks = [];
  const size   = Math.ceil(arr.length / n);
  for (let i = 0; i < arr.length; i += size) {
    chunks.push(arr.slice(i, i + size));
  }
  // Si n > arr.length podemos tener menos chunks; rellenamos con arrays vacíos
  while (chunks.length < n) chunks.push([]);
  return chunks;
}

/** Escribe en un input de React de forma confiable */
async function setField(page, selector, value) {
  await page.$eval(selector, (el, val) => {
    const nativeInputSetter = Object.getOwnPropertyDescriptor(
      window.HTMLInputElement.prototype,
      'value'
    ).set;
    nativeInputSetter.call(el, val);
    el.dispatchEvent(new Event('input',  { bubbles: true }));
    el.dispatchEvent(new Event('change', { bubbles: true }));
  }, String(value));
}

/** Escribe en un textarea de React de forma confiable */
async function setTextarea(page, selector, value) {
  await page.$eval(selector, (el, val) => {
    const nativeSetter = Object.getOwnPropertyDescriptor(
      window.HTMLTextAreaElement.prototype,
      'value'
    ).set;
    nativeSetter.call(el, val);
    el.dispatchEvent(new Event('input',  { bubbles: true }));
    el.dispatchEvent(new Event('change', { bubbles: true }));
  }, String(value));
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

// ─── Validación previa de una fila del CSV ─────────────────
/**
 * Devuelve null si la fila es válida,
 * o un string con el motivo del rechazo si no lo es.
 *
 * Reglas:
 *  1. Ningún campo numérico puede ser negativo.
 *  2. P1+P2+P3+P4 debe ser igual a VotosValidos.
 *  3. VotosValidos+VotosBlancos+VotosNulos debe ser igual a PapeletasAnfora.
 */
function validarFila(row) {
  const n = (campo) => parseInt(row[campo] ?? '0', 10) || 0;

  const p1         = n('P1');
  const p2         = n('P2');
  const p3         = n('P3');
  const p4         = n('P4');
  const validos    = n('VotosValidos');
  const blancos    = n('VotosBlancos');
  const nulos      = n('VotosNulos');
  const anfora     = n('PapeletasAnfora');
  const noUtil     = n('PapeltasNoUtilizadas');

  // Regla 1 – negativos
  const campos = [
    ['P1', p1], ['P2', p2], ['P3', p3], ['P4', p4],
    ['VotosValidos', validos], ['VotosBlancos', blancos],
    ['VotosNulos', nulos], ['PapeletasAnfora', anfora],
    ['PapeltasNoUtilizadas', noUtil],
  ];
  for (const [nombre, valor] of campos) {
    if (valor < 0) {
      return `campo "${nombre}" es negativo (${valor})`;
    }
  }

  // Regla 2 – suma de partidos = votos válidos
  const sumaPartidos = p1 + p2 + p3 + p4;
  if (sumaPartidos !== validos) {
    return `P1+P2+P3+P4 (${sumaPartidos}) ≠ VotosValidos (${validos})`;
  }

  // Regla 3 – válidos + blancos + nulos = ánfora
  const sumaUsadas = validos + blancos + nulos;
  if (sumaUsadas !== anfora) {
    return `VotosValidos+Blancos+Nulos (${sumaUsadas}) ≠ PapeletasAnfora (${anfora})`;
  }

  return null; // OK
}

// Delay entre arranques para no saturar el protocolo CDP al inicio
const STAGGER_MS = 2000;

// ─── Worker: procesa un chunk de filas en una página ───────
/**
 * @param {import('fs').WriteStream} logStream  Stream compartido para errores.log
 */
async function procesarChunk(browser, pageIndex, rows, funcionario, logStream) {
  const tag = `[P${pageIndex + 1}|${funcionario}]`;

  /** Escribe una línea mala en consola Y en el .log */
  function logBad(numFila, codigoActa, motivo) {
    const linea = `[${new Date().toISOString()}] ${tag} fila=${numFila} acta=${codigoActa} | ${motivo}`;
    console.log(`${tag}   ❌ ${motivo}`);
    logStream.write(linea + '\n');
  }

  // Escalonar el arranque: cada página espera (pageIndex * STAGGER_MS)
  // para que no golpeen Chrome todas al mismo tiempo
  if (pageIndex > 0) {
    console.log(`${tag} ⏳ Esperando ${pageIndex * STAGGER_MS / 1000}s para arrancar escalonado...`);
    await sleep(pageIndex * STAGGER_MS);
  }

  // Abrir nueva página en el mismo navegador
  const page = await browser.newPage();
  await page.goto(FRONTEND, { waitUntil: 'networkidle0', timeout: 60000 });

  // Esperar formulario
  await page.waitForSelector('input[name="funcionario"]', { timeout: 30000 });

  // Asignar funcionario UNA SOLA VEZ
  await retry(() => setField(page, 'input[name="funcionario"]', funcionario));
  await sleep(300);

  let exitosos = 0;
  let omitidos = 0;
  let errores  = 0;

  for (let i = 0; i < rows.length; i++) {
    const row           = rows[i];
    const codigoActa    = row.CodigoActa    || '';
    const codigoRecinto = row.CodigoRecinto || '';
    const nroMesa       = row.NroMesa       || '';

    console.log(`${tag} [${i + 1}/${rows.length}] 📋 Acta: ${codigoActa} | Recinto: ${codigoRecinto} | Mesa: ${nroMesa}`);

    // ── Validación previa: saltar filas con datos inválidos ──
    const motivoRechazo = validarFila(row);
    if (motivoRechazo) {
      logBad(i + 1, codigoActa, `FILA INVÁLIDA – ${motivoRechazo}`);
      omitidos++;
      continue;
    }

    // ── Datos del Acta ──────────────────────────────────
    await retry(() => setField(page, 'input[name="codigoActa"]',    codigoActa));
    await sleep(20);
    await retry(() => setField(page, 'input[name="codigoRecinto"]', codigoRecinto));
    await sleep(20);
    await retry(() => setField(page, 'input[name="nroMesa"]',       nroMesa));
    await sleep(20);

    // ── Votos por Partido ───────────────────────────────
    await retry(() => setField(page, 'input[name="p1"]',           row.P1           || '0'));
    await sleep(15);
    await retry(() => setField(page, 'input[name="p2"]',           row.P2           || '0'));
    await sleep(15);
    await retry(() => setField(page, 'input[name="p3"]',           row.P3           || '0'));
    await sleep(15);
    await retry(() => setField(page, 'input[name="p4"]',           row.P4           || '0'));
    await sleep(15);
    await retry(() => setField(page, 'input[name="votosValidos"]',  row.VotosValidos || '0'));
    await sleep(15);

    // ── Otros Votos ─────────────────────────────────────
    await retry(() => setField(page, 'input[name="blancos"]',      row.VotosBlancos         || '0'));
    await sleep(15);
    await retry(() => setField(page, 'input[name="nulos"]',        row.VotosNulos           || '0'));
    await sleep(15);
    await retry(() => setField(page, 'input[name="anfora"]',       row.PapeletasAnfora      || '0'));
    await sleep(15);
    await retry(() => setField(page, 'input[name="noUtilizadas"]', row.PapeltasNoUtilizadas || '0'));
    await sleep(15);

    // ── Horarios ────────────────────────────────────────
    await retry(() => setField(page, 'input[name="aperturaHora"]',    row.AperturaHora    || '8'));
    await sleep(15);
    await retry(() => setField(page, 'input[name="aperturaMinutos"]', row.AperturaMinutos || '0'));
    await sleep(15);
    await retry(() => setField(page, 'input[name="cierreHora"]',      row.CierreHora      || '16'));
    await sleep(15);
    await retry(() => setField(page, 'input[name="cierreMinutos"]',   row.CierreMinutos   || '0'));
    await sleep(15);

    // ── Observaciones ───────────────────────────────────
    const obs = (row.Observaciones || '').trim();
    await retry(() => setTextarea(page, 'textarea[name="observaciones"]', obs));
    await sleep(40);

    // ── Enviar ──────────────────────────────────────────
    const submitBtn = await page.$('button[type="submit"]');
    if (!submitBtn) {
      logBad(i + 1, codigoActa, 'No se encontró el botón de envío');
      errores++;
      continue;
    }

    const disabled = await page.evaluate(el => el.disabled, submitBtn);
    if (disabled) {
      logBad(i + 1, codigoActa, 'Botón deshabilitado – inconsistencia matemática en el formulario');
      omitidos++;
    } else {
      await submitBtn.click();

      try {
        await page.waitForSelector('.alert-success, .alert-error', { timeout: 6000 });

        const isSuccess = await page.$('.alert-success');
        if (isSuccess) {
          console.log(`${tag}   ✅ Transcrita correctamente.`);
          exitosos++;
        } else {
          const msg = await page.$eval('.alert-error span', el => el.textContent)
            .catch(() => 'Error desconocido');

          if (msg.includes('ya fue transcrita')) {
            console.log(`${tag}   ⚠️  Ya estaba transcrita, omitida.`);
            omitidos++;
          } else {
            logBad(i + 1, codigoActa, `Error del servidor: ${msg}`);
            errores++;
          }
        }
      } catch {
        const submitErrEl = await page.$('.alert-error span');
        if (submitErrEl) {
          const msg = await page.evaluate(el => el.textContent, submitErrEl);
          if (msg.includes('ya fue transcrita')) {
            console.log(`${tag}   ⚠️  Ya estaba transcrita, omitida.`);
            omitidos++;
          } else {
            logBad(i + 1, codigoActa, `Error: ${msg}`);
            errores++;
          }
        } else {
          logBad(i + 1, codigoActa, 'Código de acta no existe en el sistema');
          errores++;
        }
      }
    }

    await sleep(PAUSA_ENTRE_ACTAS);
  }

  console.log(`\n${tag} ── Chunk terminado: ✅ ${exitosos}  ⚠️ ${omitidos}  ❌ ${errores}\n`);
  return { exitosos, omitidos, errores };
}

// ─── Main ──────────────────────────────────────────────────
async function run() {
  console.log('╔══════════════════════════════════════════════════╗');
  console.log('║   DEMO VISUAL – Transcripción Automática de Actas ║');
  console.log(`║   ${NUM_PAGINAS} páginas en paralelo · ${FUNCIONARIOS.length} usuarios cíclicos   ║`);
  console.log('╚══════════════════════════════════════════════════╝\n');

  // Abrir el archivo de log de errores (se sobreescribe en cada ejecución)
  const LOG_PATH  = `errores_${new Date().toISOString().replace(/[:.]/g, '-')}.log`;
  const logStream = fs.createWriteStream(LOG_PATH, { flags: 'w', encoding: 'utf-8' });
  logStream.write(`=== Errores y omisiones – ${new Date().toLocaleString()} ===\n\n`);
  console.log(`📝 Log de errores: ${LOG_PATH}\n`);

  // PASO 1: Truncar datos previos via API
  await truncarTranscripciones();

  // PASO 2: Leer CSV
  const rawCsv = fs.readFileSync(CSV_PATH, 'utf-8');
  const rows   = parse(rawCsv, {
    columns:             true,
    skip_empty_lines:    true,
    relax_column_count:  true,
    trim:                true,
  }).filter(r => r.CodigoActa);

  const total = Math.min(rows.length, limit);
  console.log(`📄 Total de registros a procesar: ${total}${limit !== Infinity ? ` (limitado a ${limit})` : ' (todos)'}`);

  // PASO 3: Dividir en chunks
  const chunks = chunkArray(rows.slice(0, total), NUM_PAGINAS);
  const paginas = chunks.filter(c => c.length > 0).length;
  console.log(`🔀 Dividiendo en ${paginas} páginas:`);
  chunks.forEach((c, i) => {
    if (c.length > 0) {
      const func = FUNCIONARIOS[i % FUNCIONARIOS.length];
      console.log(`   Página ${i + 1}: ${c.length} registros  →  ${func}`);
    }
  });
  console.log();

  // PASO 4: Lanzar UN navegador visible
  const browser = await puppeteer.launch({
    headless:        false,
    slowMo:          SLOWMO,
    defaultViewport: null,
    // Aumentar el timeout del protocolo CDP para que no falle
    // cuando 10 páginas compiten por el canal de comunicación
    protocolTimeout: 120000,
    args: [
      '--start-maximized',
      '--disable-infobars',
      '--no-sandbox',
      '--disable-setuid-sandbox',
      // Aumentar la cantidad de sockets HTTP del renderer
      '--max-connections-per-proxy-host=20',
    ],
  });

  // PASO 5: Lanzar todas las páginas en paralelo
  console.log(`🚀 Lanzando ${paginas} páginas en paralelo...\n`);

  const promises = chunks
    .map((chunk, i) => ({ chunk, i }))
    .filter(({ chunk }) => chunk.length > 0)
    .map(({ chunk, i }) => {
      const funcionario = FUNCIONARIOS[i % FUNCIONARIOS.length];
      return procesarChunk(browser, i, chunk, funcionario, logStream);
    });

  const resultados = await Promise.all(promises);

  // PASO 6: Resumen global
  const totalExitosos = resultados.reduce((s, r) => s + r.exitosos, 0);
  const totalOmitidos = resultados.reduce((s, r) => s + r.omitidos, 0);
  const totalErrores  = resultados.reduce((s, r) => s + r.errores,  0);

  // Cerrar el log
  logStream.end(`\n=== FIN – Exitosas: ${totalExitosos}  Omitidas: ${totalOmitidos}  Errores: ${totalErrores} ===\n`);

  console.log('╔══════════════════════════════╗');
  console.log('║         RESUMEN FINAL        ║');
  console.log('╠══════════════════════════════╣');
  console.log(`║  ✅ Exitosas  : ${String(totalExitosos).padEnd(12)}║`);
  console.log(`║  ⚠️  Omitidas : ${String(totalOmitidos).padEnd(12)}║`);
  console.log(`║  ❌ Errores   : ${String(totalErrores).padEnd(12)} ║`);
  console.log('╚══════════════════════════════╝');
  console.log(`\n📝 Revisa los errores en: ${LOG_PATH}`);
  console.log('\n⌛ El navegador permanecerá abierto. Ciérralo manualmente.');
}

run().catch(err => {
  console.error('💥 Error fatal:', err.message);
  process.exit(1);
});
