FROM n8nio/n8n:1.0.0

USER root

# Instalar Node.js y csv-parse (no necesitamos Puppeteer ni Chromium)
RUN apk update && apk add --no-cache nodejs npm

# Instalar csv-parse para leer el CSV desde Node.js
RUN npm install -g csv-parse

WORKDIR /app
COPY fill-forms.js .

USER node
