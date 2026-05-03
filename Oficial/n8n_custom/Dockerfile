FROM n8nio/n8n:1.0.0

USER root

# Instalar dependencias para Puppeteer en Alpine
RUN apk update && apk add --no-cache \
    chromium \
    nss \
    freetype \
    harfbuzz \
    ca-certificates \
    ttf-freefont \
    nodejs \
    npm

ENV PUPPETEER_SKIP_CHROMIUM_DOWNLOAD=true \
    PUPPETEER_EXECUTABLE_PATH=/usr/bin/chromium-browser \
    NODE_PATH=/usr/local/lib/node_modules

# Instalar puppeteer y csv-parser globalmente
RUN npm install -g puppeteer csv-parser

WORKDIR /app
COPY fill-forms.js .

USER node

