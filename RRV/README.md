# RRV Backend — Recuento Rápido de Votos

Backend del pipeline TREP/RRV para el Sistema Nacional de Cómputo Electoral de Bolivia.

## Tecnologías

- **Lenguaje:** Go 1.21+
- **Framework HTTP:** Gin-Gonic
- **Base de datos:** MongoDB
- **Arquitectura:** Clean Architecture (models → repository → service → handlers)

## Requisitos

1. **Go 1.21+** instalado (`go version`)
2. **MongoDB** accesible en `localhost:27017` (o configurar `MONGO_URI`)

## Setup Rápido

```bash
# Desde el directorio RRV/
go mod tidy
go run cmd/server/main.go
```

El servidor arranca en `http://localhost:8080`.

## Variables de Entorno

| Variable | Default | Descripción |
|---|---|---|
| `MONGO_URI` | `mongodb://localhost:27017` | URI de conexión MongoDB |
| `MONGO_DB` | `rrv_electoral` | Nombre de la base de datos |
| `PORT` | `8080` | Puerto del servidor HTTP |
| `PIN_VALIDO` | `1234` | PIN de verificación para SMS |
| `TELEFONOS_AUTORIZADOS` | `+59170000001,+59170000002,+59170000003` | Lista de teléfonos autorizados (CSV) |

## Endpoints

| Método | Ruta | Descripción |
|---|---|---|
| `POST` | `/api/rrv/actas/upload` | Subir imagen/PDF → OCR simulado → validar → guardar |
| `POST` | `/api/rrv/sms` | Recibir SMS → parsear → validar → guardar |
| `GET` | `/api/rrv/actas` | Listar actas (filtros: `departamento`, `estado`, `tipo_entrada`) |
| `GET` | `/api/rrv/actas/:acta_id` | Obtener acta por ID |
| `GET` | `/api/rrv/eventos` | Listar eventos del pipeline (Event Sourcing) |
| `GET` | `/api/rrv/sms` | Listar SMS recibidos |
| `GET` | `/api/rrv/stats` | Estadísticas agregadas |
| `GET` | `/health` | Health check |

## Formato SMS

Los candidatos se detectan dinámicamente con regex `C\d+`. Puede haber cualquier cantidad:

```
ACTA:MESA-35000|DEP:Chuquisaca|MUN:Sucre|REC:U.E. Santa Monica|MESA:1|VAL:500|NUL:10|BLA:5|C1:200|C2:150|C3:100|C4:50|PIN:1234
```

### Campos

| Campo | Descripción | Obligatorio |
|---|---|---|
| `ACTA` | ID único del acta (ej: MESA-35000) | ✅ |
| `DEP` | Departamento | ✅ |
| `MUN` | Municipio | ✅ |
| `REC` | Recinto electoral | ❌ |
| `MESA` | Número de mesa | ✅ |
| `VAL` | Votos válidos | ✅ |
| `NUL` | Votos nulos | ✅ |
| `BLA` | Votos en blanco | ✅ |
| `C1..CN` | Votos por candidato (dinámico) | ✅ (mínimo 1) |
| `PIN` | PIN de verificación | ✅ |

## Seguridad SMS

- **PIN:** Verificación contra variable de entorno
- **Teléfonos:** Solo números autorizados pueden enviar
- **Anti-duplicados:** SHA256 del mensaje completo → índice único en MongoDB
- **Anti-suplantación:** Validación de número de teléfono del notario

## Colecciones MongoDB

- `actas_rrv` — Actas procesadas (índices únicos: `acta_id`, `hash_origen`)
- `eventos_rrv` — Eventos del pipeline (Event Sourcing / auditoría)
- `sms_rrv` — Registros de SMS recibidos (índice único: `hash_mensaje`)

## Pruebas

Ver `tests/api_tests.http` para los 13 casos de prueba documentados.

## Arquitectura

```
RRV/
├── cmd/server/main.go          ← Punto de entrada
├── internal/
│   ├── config/config.go        ← Variables de entorno
│   ├── models/acta.go          ← Structs y DTOs
│   ├── repository/             ← CRUD MongoDB
│   ├── service/                ← Lógica de negocio
│   └── handlers/               ← Handlers HTTP
├── uploads/                    ← Archivos temporales
└── tests/api_tests.http        ← Pruebas de API
```
