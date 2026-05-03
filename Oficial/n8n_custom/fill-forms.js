// fill-forms.js
// Automatización de transcripciones vía API REST (sin Puppeteer)
// Lee el CSV de transcripciones y hace POST directo al backend
// Se ejecuta desde n8n con el nodo "Execute Command"

const fs = require('fs');
const https = require('https');
const http = require('http');
const { parse } = require('csv-parse/sync');

const CSV_FILE_PATH = '/data/_Recursos Practica 4 - Transcripciones.csv';
const BACKEND_URL = 'http://backend:8080';
const FUNCIONARIO_NOMBRE = 'Funcionario 1'; // Nombre del funcionario que hará la transcripción automática

// Helper para hacer requests HTTP
function request(method, url, body) {
  return new Promise((resolve, reject) => {
    const parsed = new URL(url);
    const options = {
      hostname: parsed.hostname,
      port: parsed.port || (parsed.protocol === 'https:' ? 443 : 80),
      path: parsed.pathname + parsed.search,
      method,
      headers: { 'Content-Type': 'application/json' }
    };

    const lib = parsed.protocol === 'https:' ? https : http;
    const req = lib.request(options, (res) => {
      let data = '';
      res.on('data', chunk => data += chunk);
      res.on('end', () => {
        try {
          resolve({ status: res.statusCode, body: JSON.parse(data) });
        } catch {
          resolve({ status: res.statusCode, body: data });
        }
      });
    });

    req.on('error', reject);
    if (body) req.write(JSON.stringify(body));
    req.end();
  });
}

// Espera N milisegundos
function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms));
}

async function run() {
  console.log('=== INICIO DE AUTOMATIZACIÓN DE TRANSCRIPCIONES ===');

  // 1. Leer CSV
  let rawCsv;
  try {
    rawCsv = fs.readFileSync(CSV_FILE_PATH, 'utf-8');
  } catch (err) {
    console.error(`ERROR: No se pudo leer el CSV: ${err.message}`);
    process.exit(1);
  }

  // El CSV tiene una columna sin nombre (columna 22 vacía). csv-parse la ignora o la deja vacía.
  const rows = parse(rawCsv, {
    columns: true,
    skip_empty_lines: true,
    relax_column_count: true,
    trim: true
  });

  console.log(`Registros cargados del CSV: ${rows.length}`);

  // 2. Obtener ID de usuario
  const loginRes = await request('POST', `${BACKEND_URL}/api/login`, { nombre: FUNCIONARIO_NOMBRE });
  if (loginRes.status !== 200) {
    console.error(`ERROR: No se pudo autenticar al funcionario "${FUNCIONARIO_NOMBRE}". ¿Está en la base de datos?`);
    process.exit(1);
  }
  const userId = loginRes.body.id_usuario;
  console.log(`Funcionario autenticado: ${FUNCIONARIO_NOMBRE} (ID: ${userId})`);

  // 3. Procesar cada fila
  let exitosos = 0;
  let omitidos = 0;
  let errores = 0;

  for (let i = 0; i < rows.length; i++) {
    const row = rows[i];
    const codigoActa = parseInt(row.CodigoActa);

    if (!codigoActa) {
      console.log(`[${i + 1}/${rows.length}] Fila sin CodigoActa, omitiendo.`);
      omitidos++;
      continue;
    }

    // Parsear campos numéricos
    const p1            = parseInt(row.P1) || 0;
    const p2            = parseInt(row.P2) || 0;
    const p3            = parseInt(row.P3) || 0;
    const p4            = parseInt(row.P4) || 0;
    const votosValidos  = parseInt(row.VotosValidos) || 0;
    const votosBlancos  = parseInt(row.VotosBlancos) || 0;
    const votosNulos    = parseInt(row.VotosNulos) || 0;
    const anfora        = parseInt(row.PapeletasAnfora) || 0;
    const noUtilizadas  = parseInt(row.PapeltasNoUtilizadas) || 0;
    const aperturaHora  = parseInt(row.AperturaHora) || 8;
    const aperturaMin   = parseInt(row.AperturaMinutos) || 0;
    const cierreHora    = parseInt(row.CierreHora) || 16;
    const cierreMin     = parseInt(row.CierreMinutos) || 0;
    const observaciones = (row.Observaciones || '').trim();

    // Verificar si el acta ya fue transcrita
    const actaCheck = await request('GET', `${BACKEND_URL}/api/actas?codigo_acta=${codigoActa}`);
    if (actaCheck.status === 400 && actaCheck.body.error && actaCheck.body.error.includes('ya fue transcrita')) {
      console.log(`[${i + 1}/${rows.length}] Acta ${codigoActa}: ya transcrita, omitiendo.`);
      omitidos++;
      continue;
    }
    if (actaCheck.status === 404) {
      console.log(`[${i + 1}/${rows.length}] Acta ${codigoActa}: no encontrada en mesas, omitiendo.`);
      omitidos++;
      continue;
    }

    // Enviar transcripción
    const payload = {
      codigo_acta:             codigoActa,
      id_usuario:              userId,
      p1, p2, p3, p4,
      votos_validos:           votosValidos,
      votos_blancos:           votosBlancos,
      votos_nulos:             votosNulos,
      papeletas_anfora:        anfora,
      papeletas_no_utilizadas: noUtilizadas,
      apertura_hora:           aperturaHora,
      apertura_minutos:        aperturaMin,
      cierre_hora:             cierreHora,
      cierre_minutos:          cierreMin,
      observaciones
    };

    const res = await request('POST', `${BACKEND_URL}/api/transcripciones`, payload);

    if (res.status === 200) {
      console.log(`[${i + 1}/${rows.length}] ✓ Acta ${codigoActa} transcrita.`);
      exitosos++;
    } else {
      console.error(`[${i + 1}/${rows.length}] ✗ Acta ${codigoActa}: ${res.body?.error || 'Error desconocido'}`);
      errores++;
    }

    // Pequeña pausa para no saturar la BD
    await sleep(10);
  }

  console.log('');
  console.log('=== RESUMEN ===');
  console.log(`  Total filas CSV : ${rows.length}`);
  console.log(`  Exitosos        : ${exitosos}`);
  console.log(`  Omitidos        : ${omitidos}`);
  console.log(`  Errores         : ${errores}`);
  console.log('=== FIN ===');
}

run().catch(err => {
  console.error('Error fatal:', err);
  process.exit(1);
});
