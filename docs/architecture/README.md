# Arquitectura de MVForge para mantenedores y agentes

Estado: comportamiento implementado en el código local. Para funciones futuras,
consulta `docs/roadmap/` y los documentos V2.

Este índice permite seleccionar contexto sin cargar toda la documentación del
proyecto. Lee `overview.md` y después únicamente el tema relacionado con la
tarea.

| Tarea | Contexto mínimo |
|---|---|
| Entender componentes o una ruta HTTP | [Overview](overview.md) |
| Sync, estados de Assets, recuperación o borrado | [Asset lifecycle](asset-lifecycle.md) y [Storage paths](storage-paths.md) |
| Queue, workers, stages o reintentos | [Queue lifecycle](queue-lifecycle.md) |
| Generación/ejecución de FFmpeg | [Conversion pipeline](conversion-pipeline.md) y [FFmpeg policy](ffmpeg-policy.md) |
| Muestras reales para probar reproducción | [Test Encode lifecycle](test-encodes.md) y [Conversion pipeline](conversion-pipeline.md) |
| LAB, perfiles o overrides | [Profiles and overrides](profiles-and-overrides.md) |
| Presets adaptativos, QualityIntent o traductores de encoder | [Quality engine](quality-engine.md) |
| Snapshots, reportes, logs o provenance | [Snapshots and reports](snapshots-and-reports.md) |
| Extracción, OCR o sidecars | [Subtitle pipeline](subtitle-pipeline.md) |
| Traducción de subtítulos | [Traducción de subtítulos](subtitle-translation.md) |

## Fuente de verdad

En caso de discrepancia:

1. Tests y código ejecutable.
2. Documentos bajo `docs/architecture/` y `docs/guides/`.
3. Roadmap y documentos de visión.

Los documentos de arquitectura deben actualizarse cuando cambia un contrato,
flujo persistido o regla de seguridad. No deben convertirse en una copia línea
por línea del código.
