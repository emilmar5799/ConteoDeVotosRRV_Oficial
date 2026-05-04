FROM python:3.11-slim

WORKDIR /app

# Instalar dependencias
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Copiar código del dashboard
COPY . .

# Exponer puerto
EXPOSE 8050

# Variable para que Dash no recargue en producción
ENV DASH_DEBUG=false
ENV DASH_PORT=8050

CMD ["gunicorn", "app:server", "--bind", "0.0.0.0:8050", "--workers", "2", "--timeout", "120"]
