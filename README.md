# Sistema Nacional de Cómputo Electoral - Arquitectura Distribuida

[cite_start]Este repositorio contiene la arquitectura y el código para la implementación de un sistema distribuido capaz de procesar los resultados de elecciones nacionales en Bolivia[cite: 3]. [cite_start]El sistema garantiza rapidez en resultados preliminares y rigurosidad en el cómputo oficial[cite: 4].

## Arquitectura General

[cite_start]El proyecto se divide en dos pipelines desacoplados para cumplir con las características del proceso[cite: 10]:

1. [cite_start]**Recuento Rápido de Votos (RRV):** Enfocado en baja latencia, procesamiento en tiempo real y consistencia eventual[cite: 25, 26, 27].
2. [cite_start]**Cómputo Oficial:** Enfocado en alta exactitud, trazabilidad completa para auditoría y consistencia fuerte[cite: 45, 46, 47].

[cite_start]Ambos sistemas convergen en un Dashboard Analítico que permite comparar resultados, detectar inconsistencias y visualizar métricas clave (participación, votos nulos/blancos, etc.)[cite: 50, 51, 52].

## Patrones de Diseño Aplicados
* [cite_start]**CQRS (Command Query Responsibility Segregation):** Separación de operaciones de lectura (optimizadas para el dashboard) y escritura (basadas en eventos)[cite: 93, 94, 95].
* [cite_start]**Event Sourcing:** Persistencia de eventos inmutables para permitir la reconstrucción del estado del sistema en caso de desastres[cite: 96, 97, 98].
* [cite_start]**Tolerancia a Fallos:** Reintentos automáticos, procesamiento asíncrono mediante colas de mensajes y clúster de bases de datos con replicación[cite: 105, 106, 107].
* [cite_start]**Idempotencia:** Manejo seguro de eventos duplicados (ej. SMS reenviados por problemas de red) registrando inconsistencias en logs[cite: 99, 100].

## Estructura de Documentación

* [`docs/rrv_arquitectura.md`](./docs/rrv_arquitectura.md) - Detalles del pipeline de ingreso rápido, OCR y parser de SMS.
* [`docs/oficial_cluster_flujo.md`](./docs/oficial_cluster_flujo.md) - Detalles del esquema relacional, flujo de automatización (formularios + CSV) y configuración del clúster tolerante a fallos.