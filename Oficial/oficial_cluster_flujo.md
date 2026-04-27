# Cómputo Oficial: Clúster Relacional y Automatización de Transcripción

[cite_start]El Cómputo Oficial requiere consistencia fuerte, alta exactitud y control de consistencia riguroso[cite: 38, 45, 46]. [cite_start]Aquí no se asumen datos; todo evento se registra de forma inmutable[cite: 43].

## 1. Esquema de Base de Datos Relacional

Se implementará en un motor SQL robusto (PostgreSQL recomendado) utilizando el siguiente esquema para asegurar la integridad referencial:

* **Usuarios:** `id_usuario (SERIAL)`, `nombre`, `rol`, credenciales.
* **Distribucion_Territorial:** Jerarquía geográfica (departamento, provincia, municipio) vinculada a los recintos.
* **Recinto_Electoral & Mesas:** Relaciona los 5.368 recintos y las 35.000 mesas habilitadas[cite: 6, 7].
* **Papeleta (Acta):** `id_mesa (FK Unique)`, `cantidad_anfora`, `papeletas_no_usadas`, `votos_validos`, `votos_nulos`, `votos_blancos`. 
* **Detalle_Votos_Partido:** Rompe la relación muchos a muchos, guardando la `cantidad_votos` por cada `id_partido` para una `id_papeleta` específica.

*(Todas las tablas incluyen campos de auditoría: `fecha_creacion`, `usuario_creacion`, etc.)*

## 2. Clúster Tolerante a Fallos (Implementación Docker)

Para garantizar la disponibilidad y la persistencia orientada a auditoría, el clúster relacional se orquestará mediante Docker:

* **Arquitectura del Clúster (PostgreSQL High Availability):**
  * **Nodo Master (Primary):** Recibe las operaciones de escritura/actualización.
  * **Nodos Réplica (Standby):** Dos contenedores sincronizados en tiempo real mediante *Streaming Replication*. Atienden consultas de lectura (patrón CQRS).
* **Gestor de Failover (Pgpool-II o Patroni):**
  * [cite_start]Si el Master se cae (Simulación: `docker stop pg_master`), el gestor detecta el timeout y promueve automáticamente a una de las réplicas como nuevo Master[cite: 23].
  * La aplicación backend no necesita reconfigurarse; apunta a la IP/Puerto del balanceador.
* [cite_start]**Resincronización Automática:** Al volver a encender el contenedor del nodo caído (`docker start pg_master`), este se reconecta al clúster como nodo réplica y sincroniza los bloques faltantes (WAL logs) para recuperar la consistencia con el master actual[cite: 23].

## 3. Flujo de Transcripción Oficial (Automatización)

[cite_start]A diferencia del RRV, el cómputo oficial requiere una entrada validada mediante una aplicación cliente y herramientas de automatización[cite: 34, 35].

### A. La Aplicación Web (Forms)
* Se desarrollará una interfaz web (ej. React/Next.js o un framework C# Web) que incluye un sistema de Login.
* Tras la autenticación, se expone un formulario complejo y estrictamente validado para el ingreso de actas.
* Validaciones en caliente: Impedir envío si hay datos contradictorios o faltan horas de apertura/cierre[cite: 102, 103].

### B. El Automatizador (n8n / Scripting)
El flujo exigido para procesar el archivo CSV proporcionado se realiza de la siguiente manera[cite: 33, 111]:
1. **Disparador:** Un proceso en *n8n* (o un script en Puppeteer/Playwright) lee secuencialmente el archivo `.csv` que contiene la transcripción de las actas[cite: 33, 111].
2. **Autenticación (Bot):** La herramienta de automatización ejecuta un flujo de login en la aplicación Web (Forms), obteniendo el token de sesión o cookie.
3. **Data Entry Automatizado:** * Para cada fila del CSV, la herramienta rellena los inputs correspondientes en el formulario web del frontend.
   * Dispara el evento `submit` interactuando directamente con el cliente.
4. **Respuesta y Trazabilidad:** El backend procesa el guardado en el Master DB del clúster y devuelve un código HTTP. Si el acta ya existía con datos diferentes, la transacción falla y la herramienta registra la discrepancia[cite: 104].