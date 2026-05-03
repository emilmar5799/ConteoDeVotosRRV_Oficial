const puppeteer = require('puppeteer');
const fs = require('fs');
const csv = require('csv-parser');

const CSV_FILE_PATH = '/data/_Recursos Practica 4 - Transcripciones.csv';
const FRONTEND_URL = 'http://frontend:3000'; // Dentro de docker-compose

async function run() {
  const data = [];
  
  // Read CSV
  await new Promise((resolve, reject) => {
    fs.createReadStream(CSV_FILE_PATH)
      .pipe(csv())
      .on('data', (row) => {
        // Skip empty rows
        if (row.CodigoActa) {
          data.push(row);
        }
      })
      .on('end', resolve)
      .on('error', reject);
  });

  console.log(`Loaded ${data.length} records from CSV.`);

  const browser = await puppeteer.launch({
    executablePath: process.env.PUPPETEER_EXECUTABLE_PATH,
    args: ['--no-sandbox', '--disable-setuid-sandbox', '--disable-dev-shm-usage'],
    headless: true
  });

  const page = await browser.newPage();
  
  try {
    await page.goto(FRONTEND_URL, { waitUntil: 'networkidle0' });
    console.log('Navigated to frontend');

    // Login
    await page.waitForSelector('input[type="text"]');
    await page.type('input[type="text"]', 'Funcionario 1');
    await page.click('button[type="submit"]');
    
    // Wait for main screen
    await page.waitForSelector('input[placeholder="Código de Acta (Ej. 1010200001001)"]', { timeout: 10000 });
    console.log('Logged in successfully');

    for (let i = 0; i < data.length; i++) {
      const row = data[i];
      console.log(`Processing Acta: ${row.CodigoActa}`);

      // Enter CodigoActa
      const searchInput = await page.$('input[placeholder="Código de Acta (Ej. 1010200001001)"]');
      await searchInput.click({ clickCount: 3 });
      await searchInput.press('Backspace');
      await searchInput.type(row.CodigoActa);
      
      // Click Buscar
      await page.click('button[type="submit"]');

      // Wait for form to appear (wait for P1 input)
      try {
        await page.waitForSelector('input[name="p1"]', { timeout: 5000 });
      } catch (err) {
        console.log(`Acta ${row.CodigoActa} already transcribed or not found. Skipping.`);
        continue;
      }

      // Clear and type values
      const fields = [
        { name: 'p1', val: row.P1 },
        { name: 'p2', val: row.P2 },
        { name: 'p3', val: row.P3 },
        { name: 'p4', val: row.P4 },
        { name: 'blancos', val: row.VotosBlancos },
        { name: 'nulos', val: row.VotosNulos },
        { name: 'anfora', val: row.PapeletasAnfora },
        { name: 'noUtilizadas', val: row.PapeltasNoUtilizadas }
      ];

      for (const field of fields) {
        const input = await page.$(`input[name="${field.name}"]`);
        await input.click({ clickCount: 3 });
        await input.press('Backspace');
        await input.type((field.val || '0').toString());
      }

      // Observaciones
      if (row.Observaciones) {
        const obs = await page.$('textarea[name="observaciones"]');
        await obs.click({ clickCount: 3 });
        await obs.press('Backspace');
        await obs.type(row.Observaciones);
      }

      // Submit transcription
      const submitBtn = await page.$x("//button[contains(text(), 'Enviar Transcripción Oficial')]");
      if (submitBtn.length > 0) {
        // Check if button is disabled due to inconsistency
        const disabled = await page.evaluate(el => el.disabled, submitBtn[0]);
        if (disabled) {
          console.error(`Acta ${row.CodigoActa} has inconsistencies. Cannot submit.`);
        } else {
          await submitBtn[0].click();
          await page.waitForTimeout(1000); // Wait for submission
          console.log(`Acta ${row.CodigoActa} submitted successfully.`);
        }
      }
    }

  } catch (error) {
    console.error('Error during automation:', error);
  } finally {
    await browser.close();
  }
}

run();
