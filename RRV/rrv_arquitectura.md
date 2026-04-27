# Arquitectura del Sistema: Recuento Rápido de Votos (RRV)

[cite_start]El sistema RRV está diseñado para ingerir datos de manera masiva y concurrente, priorizando la disponibilidad y la baja latencia sobre la consistencia inmediata[cite: 25, 26].

## 1. Fuentes de Ingreso de Datos

[cite_start]El sistema debe soportar recintos con y sin conectividad estable, utilizando dos vías principales[cite: 8]:

### A. Canal de Fotografías (Actas) y OCR
* [cite_start]**Input:** Fotografías de actas electorales capturadas desde una aplicación móvil[cite: 17, 111].
* [cite_start]**Procesamiento:** * Se aplica procesamiento de imágenes para mejorar el contraste y alinear el documento[cite: 19].
  * [cite_start]Extracción de datos mediante un servicio de OCR[cite: 20].
  * [cite_start]Los datos extraídos pasan por validaciones básicas (estructurales y detección de duplicados simples)[cite: 21].

### B. Canal de Mensajería SMS
[cite_start]Debido a que algunos recintos reportarán resultados mediante SMS por limitaciones de red[cite: 84, 85]:
* **Receptor (SMS Gateway):** Un microservicio (idealmente en Go o Node.js por su manejo de concurrencia) conectado a un módem o API de Twilio/proveedor local.
* [cite_start]**Intérprete (Parser):** Analiza el texto plano del mensaje[cite: 88].
  * *Formato esperado propuesto:* `[ID_MESA]*[VALIDOS]*[BLANCOS]*[NULOS]*[VOTOS_P1]*[VOTOS_P2]...`
* [cite_start]**Seguridad y Calidad:** El microservicio valida que el número de teléfono emisor esté registrado y vinculado al notario de esa mesa para evitar suplantación[cite: 90].

## 2. Flujo de Procesamiento Asíncrono (Eventos)

Para evitar cuellos de botella y tolerar fallos:
1. **API Gateway / SMS Webhook:** Recibe el payload.
2. **Cola de Mensajes (Message Broker):** Se encola el evento crudo (ej. en RabbitMQ o Kafka) de forma inmediata para no bloquear al cliente.
3. **Workers de Procesamiento:** * Consumen los eventos, realizan las sumas aritméticas y verifican que `votos_nulos + votos_blancos + suma(votos_partidos) == total_emitidos`.
   * [cite_start]Si hay una inconsistencia aritmética, se marca el registro con un flag de error y se envía a una tabla de auditoría/logs[cite: 100, 101].
4. [cite_start]**Almacenamiento (NoSQL / Caché):** Los resultados procesados se guardan en un clúster sincronizado de alta disponibilidad[cite: 22, 23].

## 3. Base de Datos del RRV
* **Motor Recomendado:** MongoDB o un clúster de Redis. 
* [cite_start]**Justificación:** Se requiere ingesta veloz (documentos JSON planos con los resultados por mesa) y capacidad de levantar réplicas instantáneamente si un nodo cae[cite: 22, 23]. Los esquemas flexibles permiten guardar tanto el JSON extraído del OCR como los datos parseados del SMS en una misma colección.