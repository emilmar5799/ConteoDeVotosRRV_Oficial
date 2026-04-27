# doc completo

**PRÁCTICA 4**
**Implementación de un Sistema Nacional de Cómputo Electoral (Bolivia)**

**Contexto**
[cite_start]El Órgano Electoral Plurinacional requiere diseñar e implementar un sistema distribuido capaz de procesar los resultados de elecciones nacionales en Bolivia[cite: 3].
[cite_start]El sistema debe garantizar rapidez en la difusión de resultados preliminares y rigurosidad en el cómputo oficial , considerando condiciones reales de infraestructura, conectividad limitada y alta demanda concurrente[cite: 4].

Se dispone de:
* [cite_start]5.368 recintos electorales [cite: 6]
* [cite_start]35.000 mesas electorales [cite: 7]
* [cite_start]No todos los recintos cuentan con conectividad estable, por lo que el sistema debe contemplar mecanismos alternativos de transmisión de datos, incluyendo mensajería SMS[cite: 8].

**Objetivo**
[cite_start]Diseñar e implementar una arquitectura de sistemas distribuidos basada en dos pipelines desacoplados , que permitan[cite: 10]:
* [cite_start]Procesar resultados preliminares en tiempo real ( Recuento Rápido de Votos - RRV ) [cite: 11]
* [cite_start]Generar resultados oficiales auditables ( Cómputo Oficial ) [cite: 12]
* [cite_start]Comparar ambos resultados mediante un sistema de visualización analítica (dashboard) [cite: 13]

**Descripción del Problema**
[cite_start]El sistema deberá implementar dos flujos independientes pero coherentes[cite: 15]:

**1. [cite_start]Sistema RRV (Recuento Rápido de Votos)** [cite: 16]
* Entrada: Fotografías de actas electorales enviadas desde recintos [cite: 17]
* [cite_start]Procesamiento [cite: 18]
    * [cite_start]Procesamiento de imágenes [cite: 19]
    * Extracción de datos mediante OCR [cite: 20]
    * [cite_start]Validaciones básicas (estructura, duplicados simples) [cite: 21]
    * [cite_start]Almacenamiento en un clúster de base de datos a elección (relacional o no relacional) justificando la elección del motor[cite: 22].
    * Este cluster debe siempre estar sincronizado. y en cuanto uno caiga debe levantarse automáticamente el/los otros [cite: 23]
* [cite_start]Características [cite: 24]
    * [cite_start]Baja latencia [cite: 25]
    * Consistencia eventual [cite: 26]
    * [cite_start]Procesamiento en tiempo real [cite: 27]
* [cite_start]Uso [cite: 28]
    * Publicación de resultados preliminares [cite: 29]
    * [cite_start]Visualización pública temprana [cite: 30]

**2. [cite_start]Sistema de Cómputo Oficial** [cite: 31]
* Entrada [cite: 32]
    * [cite_start]Se proporcionará un archivos CSV con la transcripción de todas las actas [cite: 33]
    * [cite_start]Se desarrollará una aplicación cliente (Consola/Windows Forms/Web) [cite: 34]
    * Integración mediante herramientas de automatización (ej: n8n) [cite: 35]
* [cite_start]Procesamiento [cite: 36]
    * [cite_start]Validación rigurosa de datos [cite: 37]
    * Control de consistencia [cite: 38]
    * [cite_start]Registro de auditoría [cite: 39]
    * [cite_start]Validación cruzada [cite: 40]
* Almacenamiento [cite: 41]
    * [cite_start]Base de datos (relacional / no relacional) [cite: 42]
    * [cite_start]Persistencia orientada a auditoría (eventos) [cite: 43]
* Características [cite: 44]
    * [cite_start]Alta exactitud [cite: 45]
    * [cite_start]Consistencia fuerte [cite: 46]
    * Trazabilidad completa [cite: 47]
* [cite_start]Uso generación de resultados oficiales vinculantes [cite: 48]

[cite_start]**Integración y Comparación** [cite: 49]
[cite_start]Ambos sistemas deben ser integrados en un Dashboard Analítico , el cual permita[cite: 50]:
* [cite_start]Comparar resultados entre RRV y cómputo oficial [cite: 51]
* Detectar inconsistencias [cite: 52]
* [cite_start]Visualizar métricas e indicadores clave [cite: 53]

[cite_start]El Dashboard debe incluir como mínimo[cite: 54]:
[cite_start]1) Métricas (datos crudos) [cite: 55]
* [cite_start]Participación electoral [cite: 56]
* Total de votos (válidos, nulos, blancos) [cite: 57]
* [cite_start]Votos por candidato [cite: 58]
* [cite_start]Estado de actas (recibidas, procesadas, pendientes) [cite: 59]
[cite_start]2) Indicadores (KPIs) [cite: 60]
* [cite_start]Tasa de participación [cite: 61]
* Porcentaje por candidato [cite: 62]
* [cite_start]Margen de victoria [cite: 63]
* [cite_start]Velocidad de procesamiento [cite: 64]
* Diferencias entre conteo rápido y oficial [cite: 65]
3) Análisis geográfico [cite: 66]
* [cite_start]Resultados por región [cite: 67]
* [cite_start]Mapas de calor [cite: 68]
* Distribución territorial del voto [cite: 69]
4) Transparencia [cite: 70]
* [cite_start]% de actas publicadas [cite: 71]
* [cite_start]Trazabilidad del procesamiento [cite: 72]
* Acceso a actas digitalizadas [cite: 73]
5) Indicadores técnicos [cite: 74]
* [cite_start]Latencia [cite: 75]
* [cite_start]Throughput [cite: 76]
* Disponibilidad [cite: 77]
* [cite_start]Seguridad [cite: 78]
[cite_start]6) Analítica avanzada (opcional) [cite: 79]
* Detección de anomalías [cite: 80]
* [cite_start]Patrones atípicos [cite: 81]
* [cite_start]Análisis estadístico del voto [cite: 82]

[cite_start]**Obligatorio** [cite: 83]
[cite_start]Debido a limitaciones de conectividad[cite: 84]:
* Algunos recintos deberán reportar resultados mediante SMS [cite: 85]
El sistema debe[cite: 86]:
* [cite_start]Recibir mensajes SMS [cite: 87]
* [cite_start]Interpretar el contenido [cite: 88]
* Integrarlo al flujo de procesamiento para RRV [cite: 89]
* [cite_start]Implementar los mecanismos de seguridad para evitar suplantación y calidad de dato [cite: 90]

[cite_start]**Criterios de Diseño Arquitectónico** [cite: 91]
[cite_start]La solución debe justificar e implementar los siguientes patrones[cite: 92]:
* [cite_start]CQRS (Command Query Responsibility Segregation) [cite: 93]
    * Escritura basada en eventos [cite: 94]
    * [cite_start]Lectura optimizada para consultas [cite: 95]
* [cite_start]Event Sourcing [cite: 96]
    * Persistencia basada en eventos [cite: 97]
    * [cite_start]Capacidad de reconstrucción del sistema [cite: 98]
* [cite_start]Idempotencia [cite: 99]
    * Procesamiento seguro de eventos duplicados o inconsistencias. Se recomienda registrar en Logs [cite: 100]
    * [cite_start]Inconsistencia aritmética: Suma de votos por candidatos que no coincide con el total de "votos válidos" o "votos emitidos". [cite: 101]
    * [cite_start]Datos contradictorios: Discrepancia entre el número de ciudadanos que votaron (según la lista de índice) y el total de papeletas encontradas en el ánfora. [cite: 102]
    * Falta de datos de apertura o cierre: Omisión de la hora exacta de inicio o finalización de la jornada electoral en el acta. [cite: 103]
    * [cite_start]Acta recibida con anterioridad y no coinciden los datos [cite: 104]
* [cite_start]Tolerancia a fallos [cite: 105]
    * Reintentos automáticos [cite: 106]
    * [cite_start]Procesamiento asíncrono [cite: 107]
    * [cite_start]Manejo de caídas parciales del sistema [cite: 108]

[cite_start]**Entregables** [cite: 109]
[cite_start]Los estudiantes deberán presentar (la base de datos cargada)[cite: 110]:

| Puntos | Criterios |
|---|---|
| 50 | Implementación funcional RRV (OCR + almacenamiento) Cómputo oficial (CSV + validación) + Documentación técnica no más de 2 págs y capturas de pantallas de las partes más importantes de la implementación |
| 20 | Dashboard Visualización de métricas e indicadores y esquemas de bases de datos |
| 30 | Defensa Individual cada estudiante deberá explicar la parte que hizo en inglés y responder a preguntas técnicas |
| 15 | Automatización : n8n o cualquier mecanismo que nos permita transcripción de datos leyendo desde un archivo csv |
| 15 | Aplicación móvil: que permita la captura de la foto del acta y continúe con el flujo |

[cite_start]**Presentacion** [cite: 112]
[cite_start]Se evaluará[cite: 113]:
* Correcta aplicación de arquitectura distribuida [cite: 114]
* [cite_start]Separación de responsabilidades (RRV vs Oficial) [cite: 115]
* [cite_start]Escalabilidad y resiliencia [cite: 116]
* Visualización y análisis [cite: 117]
* [cite_start]Manejo de escenarios reales (SMS, fallos, duplicados) [cite: 118]
* [cite_start]Decisiones arquitectónicas [cite: 119]
* Problemas encontrados [cite: 120]
* [cite_start]Estrategias de escalabilidad [cite: 121]
* [cite_start]Capturas de pantalla de ambos flujos [cite: 122]

[cite_start]**Nota final** [cite: 123]
[cite_start]El enfoque de esta práctica no es solo programar, sino pensar como arquitectos de sistemas distribuidos , diseñando soluciones que operen bajo condiciones reales de un país; [cite: 124]
[cite_start]el project manager tendrá una evaluación de 50% y la defensa será en inglés. [cite: 125]

[cite_start]**Recursos** [cite: 126]
[cite_start]https://docs.google.com/spreadsheets/d/1aJk5lt17I0pSHJpq8cjJuLnDXONwPyFJySitKyDJnQA/edit?usp=sharing [cite: 127]

| CodigoDistribucionTerritorial | Departamento | Municipio | Provincia |
|---|---|---|---|
| 10101 | Chuquisaca | Oropeza | Sucre |
| 10102 | Chuquisaca | Oropeza | Yotala |
| 10103 | Chuquisaca | Oropeza | Poroma |
| 10201 | Chuquisaca | Azurduy | Azurduy |
| 10202 | Chuquisaca | Azurduy | Tarvita |

| CodigoRecintoElectoralD | CodigoDistribucionTerritorial | RECINTO ELECTORAL | DIRECCIÓN |
|---|---|---|---|
| 1 | 10101 | U. E. Santa Mónica | Calle Achanq’ara entre las calles Chariña y Qoyllur, OTB Ticti Norte |
| 2 | 10102 | U. E. Padresama | Se encuentra sobre la carretera cochabamba a Santa Cruz km 150, sindicato agrario Padresama |
| 3 | 10103 | U.E. Lacolaconi | Lacolaconi |
| 4 | 10201 | U.E. Genoveva Ríos | Calle Los Robles entre Av. Segunda Circunvalación y Calle Sófocles |
| 5 | 10202 | U.E. 27 de Mayo | René Barrientos Ortuño |

| CodigoMesa | Nro Mesa | CantidadHabilitada | CodigoRecintoElectoralD |
|---|---|---|---|
| 35000 | 1 | 589 | 10101 |
| 34999 | 2 | 538 | 10102 |
| 34998 | 3 | 259 | 10103 |
| 34997 | 4 | 524 | 10201 |
| 34996 | 5 | 992 | 10202 |
| 34995 | 6 | 808 | 10301 |

*[Imagen de Acta Electoral de Escrutinio y Conteo]*

---

[cite_start]**Banco de consultas elecciones** [cite: 131]

1. [cite_start]Cantidad de mesas por recinto y departamento [cite: 132]
| Departamento | Recinto | Cantidad de Mesas |
|---|---|---|
| La Paz | Colegio Ayacucho | 12 |
| La Paz | Liceo Venezuela | 8 |
| Santa Cruz | U.E. San Martin | 15 |
| Cochabamba | Colegio Alemán | 10 |

2. [cite_start]Registro de votos (los que están en ánfora) por municipio [cite: 135]
| Departamento | Municipio | Total Votos |
|---|---|---|
| La Paz | El Alto | 345,000 |
| Santa Cruz | Warnes | 120,000 |
| Tarija | Yacuiba | 95,000 |

3. [cite_start]Cantidad de votos por departamento [cite: 137]
| Departamento | Votos Totales |
|---|---|
| La Paz | 1,200,000 |
| Santa Cruz | 1,500,000 |
| Cochabamba | 980,000 |

4. [cite_start]Top 5 recintos con más votos para X partido [cite: 139]
| Partido | Recinto | Votos |
|---|---|---|
| P1 | Colegio Ayacucho | 5,400 |
| P1 | Liceo Venezuela | 5,300 |
| P1 | San Martín | 5,250 |
| P1 | Don Bosco | 5,100 |
| P1 | Juan XXIII | 5,000 |

5. [cite_start]Votos nulos por departamento y el % en relación al total de validos [cite: 141]
| Departamento | Votos Nulos | % |
|---|---|---|
| La Paz | 35,000 | 17.50 |
| Santa Cruz | 42,000 | 18.77 |
| Oruro | 12,000 | 44.10 |

6. [cite_start]Listado de boletas anuladas en el TREP [cite: 143]
| ID Boleta | Recinto | Mesa | Motivo de Anulación |
|---|---|---|---|
| 120054 | Colegio Ayacucho | 4 | Error en cantidad de boletas no usadas |
| 130902 | Don Bosco | 6 | Error en número de votos nulos |
| 145678 | Liceo Venezuela | 2 | Error en la legibilidad de datos |

7. [cite_start]Cantidad de votos totales TREP vs Oficial [cite: 145]
| Fuente | Votos Totales |
|---|---|
| TREP | 7,200,000 |
| Oficial | 7,180,000 |

8. [cite_start]Total de votos por candidato (TREP vs Oficial) [cite: 147]
| Candidato | TREP | Oficial |
|---|---|---|
| MASISP | 3,500,000 | 3,495,000 |
| Partido2 | 2,800,000 | 2,810,000 |
| Partido3 | 900,000 | 875,000 |

9. [cite_start]Votos nulos, blancos, Válidos pero no blancos y Total por departamento [cite: 149]
| Departamento | Votos Nulos | Votos Blancos | Por Candidatos | (Válidos - Blancos) |
|---|---|---|---|---|
| Beni | 10,000 | 7,000 | 100000 | 1001700 |
| Pando | 5,000 | 3,000 | 20000 | 20300 |

10. [cite_start]Centros de votación activos y cantidad de actas enviadas por cada recinto electoral [cite: 151]
| ID Centro | Nombre del Centro | Departamento | Actas |
|---|---|---|---|
| 10301 | U.E. San Martín | Santa Cruz | 8 |
| 10404 | Colegio Nacional Sucre | La Paz | 12 |

11. [cite_start]Mesas con >20% de abstención (Ciudadanos que no fueron a votar) [cite: 153]
| Departamento | Recinto | Mesa | Abstención (%) |
|---|---|---|---|
| Cochabamba | U.E. Ayacucho | 3 | 23.5% |
| La Paz | Don Bosco | 5 | 21.1% |

12. Actas TREP | [cite_start]OFICIAL recibidas en el centro de cómputo en un periodo de tiempo por hora ( o por minuto o por segundo ) [cite: 155]
| Hora | Actas Recibidas |
|---|---|
| 18:00-19:00 | 1,200 |
| 19:00-20:00 | 2,300 |
| 20:00-21:00 | 1,800 |

13. [cite_start]Porcentaje de actas recibidas anuladas en el trep vs Oficial por departamento [cite: 157]
| Departamento | Total actas anuladas trep | Total actas anuladas Oficial | % Anulación TREP | % Anulación Oficial |
|---|---|---|---|---|
| La Paz | 1,200,000 | 1,352,652 | 19.4% | 19.5% |
| Santa Cruz | 1,500,000 | 16,875,52 | 14.6% | 15.2% |
| Cochabamba | 980,000 | 1,200,141 | 16.1% | 18.3% |

14. [cite_start]Tiempo promedio entre cierre y recepción de la votación oficial entre la recepción de la primera y la última acta de cada departamento [cite: 159]
| Recinto | Primera Acta | Última Acta | Tiempo Promedio (min) |
|---|---|---|---|
| La Paz | 18/04/2024 10:18:16 | 18/04/2024 11:18:16 | 45 |
| Santa Cruz | 18/04/2024 9:18:16 | 18/04/2024 16:18:16 | 152 |

15. [cite_start]Haga una consulta que nos permita saber el % de confiabilidad del TREP vs el Oficial (Justifique que factores tomo ) [cite: 161]
| Fuente | % de confianza |
|---|---|
| TREP | 98.99 |
| OFICIAL | 99.16 |

16. [cite_start]Porcentaje de participación ciudadana por departamento [cite: 163]
| Departamento | Participación (%) |
|---|---|
| La Paz | 78.5% |
| Santa Cruz | 82.1% |
| Tarija | 75.3% |

17. [cite_start]Actas inconsistentes entre TREP y Oficial (Diff = Oficial - TREP) es la diferencia entre todos los campos que se leen tanto de las actas como del transcriptor [cite: 165]
| ID Acta | Fuente | Diff field1 | Dif field2 | Diff fieldN |
|---|---|---|---|---|
| 789010 | TREP | -1 | 2 | 11 |
| 789022 | OFICIAL | +5 | -1 | 2 |

18. [cite_start]Cantidad de MegaBytes utilizados por PDF recibidos por departamento y número de archivos (sin importar si fueron actas validas o invalidas) [cite: 167]
| Recinto | Archivos PDF | Tamaño PDF (MB) |
|---|---|---|
| Santa Cruz | 1523 | 3500 |
| La Paz | 1426 | 4202 |
| Cochabamba | 1254 | 30533 |

19. [cite_start]Resultados por departamento, municipio o provincia (input específico departamento, municipio o provincia) [cite: 169]
(Ejemplo: Municipio: El Alto ) [cite_start][cite: 170]
| Candidato | Total Votos |
|---|---|
| Provincia 1 | 150,000 |
| Provincia 2 | 120,000 |
| Provincia 3 | 30,000 |

20. [cite_start]Error más común en verificación de PDFs o transcripción de datos [cite: 172]
| Tipo de Error | Frecuencia |
|---|---|
| Error en Cantidad de ciudadanos habilitados | 1,200 |
| Error en Votos Válidos | 980 |
| Error Cantidad de boletas en ánfora | 540 |