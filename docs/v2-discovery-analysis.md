# MediaForge — Contexto V2: Discovery & Analysis Pipeline

## 1. Contexto general del proyecto

MediaForge es una aplicación local/self-hosted para gestionar, analizar, convertir, restaurar y publicar assets de video/audio.

La aplicación corre en Docker, usa almacenamiento persistente en NAS y está pensada para usuarios técnicos y no técnicos que quieren convertir bibliotecas multimedia de forma controlada.

MediaForge no busca reemplazar Radarr, Sonarr, Jellyfin o Plex.

Su rol es complementar el flujo multimedia:

* Radarr/Sonarr: adquisición y organización.
* Jellyfin/Plex: reproducción.
* MediaForge: análisis, perfilado, conversión, restauración, validación, publicación y auditoría.

Stack actual:

* Backend: Go
* Frontend: React + TypeScript + MUI
* Conversión/análisis: FFmpeg / ffprobe
* Contenedores: Docker Compose
* Persistencia: base de datos local + paths montados en Docker
* UI local:

  * Frontend: `http://localhost:5173`
  * Backend/API/Swagger: `http://localhost:8080`

Flujo principal esperado:

```text
Assets → Discovery → Analysis → Queue → Workers → Validation → Publisher → History
```

---

## 2. Objetivo de esta fase

Esta fase busca separar claramente dos responsabilidades del pipeline:

1. **Discovery**

   * Detectar y registrar archivos multimedia.
   * Agrupar assets por path/carpeta.
   * Identificar si un archivo es elegible para análisis.
   * No modificar archivos.
   * No convertir archivos.
   * No publicar archivos.

2. **Analysis**

   * Inspeccionar técnicamente el asset usando `ffprobe`.
   * Generar un reporte AS-IS persistente.
   * Detectar codecs, streams, resolución, audio, subtítulos, duración, warnings y riesgos.
   * Calcular un score de confianza.
   * Sugerir perfiles de video/audio.
   * Decidir si el asset puede pasar a Queue, Needs Review o Lab.

La idea es que MediaForge sea seguro por defecto:

```text
Discovery = automático y no destructivo.
Analysis = automático y no destructivo.
Queue = manual por default, automático solo con score alto.
Lab = manual o sugerido para casos dudosos.
Conversion = automática cuando el job ya fue aprobado.
Validation = automática.
Publisher = manual al inicio, automático solo con mucha confianza.
Archive/Delete originals = nunca sin reglas explícitas.
```

---

## 3. Discovery

### 3.1 Definición

Discovery es la fase encargada de responder:

```text
¿Qué archivos tengo en /media/raw y qué tan listos están para ser analizados?
```

Discovery debe limitarse a revisar el filesystem, registrar assets y preparar metadata básica.

No debe ejecutar conversiones, filtros, previews ni mover archivos fuera de rutas controladas.

---

### 3.2 Responsabilidades de Discovery

Discovery debe:

* Escanear `/media/raw`.
* Detectar archivos nuevos.
* Esperar a que un archivo esté estable antes de registrarlo.
* Registrar assets en la base de datos.
* Agrupar assets por path/carpeta.
* Identificar extensiones soportadas.
* Ignorar extensiones no soportadas.
* Detectar archivos duplicados básicos por path, nombre, tamaño o hash futuro.
* Detectar si un archivo ya está registrado.
* Detectar si un archivo ya está en Queue.
* Marcar assets sospechosos como `NEEDS_REVIEW`.
* Marcar archivos incompletos como `DISCOVERY_PENDING` o `DISCOVERY_FAILED`.
* Mantener un historial de discovery events.

Discovery no debe:

* Convertir archivos.
* Modificar archivos.
* Borrar archivos.
* Mover archivos a librerías finales.
* Archivar originales.
* Aplicar filtros FFmpeg.
* Crear samples A/B.
* Publicar en Jellyfin/Plex.

---

### 3.3 Cuándo debe correr Discovery

Discovery debe correr en tres momentos:

#### A. Al iniciar MediaForge

Cuando MediaForge inicia, debe escanear `/media/raw` y comparar el filesystem contra la base de datos.

Objetivo:

```text
Reconstruir estado si el usuario movió, agregó o eliminó archivos manualmente.
```

Flujo:

```text
MediaForge starts
  ↓
Scan /media/raw
  ↓
Compare filesystem vs database
  ↓
Register new assets
  ↓
Mark missing assets if files no longer exist
  ↓
Complete discovery sync
```

---

#### B. Cuando aparece un archivo nuevo

Debe existir un watcher opcional para detectar archivos nuevos.

Flujo:

```text
File watcher detects new file
  ↓
Wait until file is stable
  ↓
Run Discovery
  ↓
Register asset
```

Regla recomendada:

```text
Si el tamaño del archivo no cambia durante 60–120 segundos, se considera estable.
```

Esto evita analizar archivos incompletos mientras todavía se están copiando al NAS.

---

#### C. Manualmente desde UI

Debe existir una acción manual:

```text
Rescan Raw Library
```

Y opcionalmente:

```text
Rescan this folder
```

Esto es importante para NAS/self-hosted porque el watcher puede fallar, Docker puede reiniciarse o el usuario puede montar nuevos volúmenes.

---

### 3.4 Estados internos de Discovery

Estados sugeridos:

```text
DISCOVERY_PENDING
DISCOVERING
DISCOVERED
DISCOVERY_FAILED
IGNORED
NEEDS_REVIEW
```

Uso recomendado:

* `DISCOVERY_PENDING`: archivo detectado pero todavía no estable.
* `DISCOVERING`: Discovery en ejecución.
* `DISCOVERED`: archivo registrado correctamente.
* `DISCOVERY_FAILED`: Discovery falló.
* `IGNORED`: archivo no soportado o excluido por reglas.
* `NEEDS_REVIEW`: asset detectado, pero requiere revisión humana.

---

### 3.5 Metadata mínima de Discovery

Cada asset descubierto debe guardar:

```json
{
  "asset_id": "asset_123",
  "source_path": "/media/raw/anime/rayearth/ep01.mkv",
  "file_name": "ep01.mkv",
  "folder_path": "/media/raw/anime/rayearth",
  "extension": ".mkv",
  "file_size_bytes": 1234567890,
  "discovery_status": "DISCOVERED",
  "is_stable": true,
  "needs_review": false,
  "review_reason": null,
  "tags": [],
  "created_at": "2026-07-03T00:00:00Z",
  "updated_at": "2026-07-03T00:00:00Z"
}
```

---

## 4. Analysis

### 4.1 Definición

Analysis es la fase encargada de responder:

```text
¿Qué contiene técnicamente este asset y qué necesita antes de convertirse?
```

Analysis debe inspeccionar el archivo usando `ffprobe` y generar un reporte AS-IS persistente.

Esta fase tampoco debe modificar el archivo original.

---

### 4.2 Responsabilidades de Analysis

Analysis debe:

* Ejecutar `ffprobe` sobre el asset.
* Detectar container.
* Detectar duración.
* Detectar streams de video.
* Detectar streams de audio.
* Detectar streams de subtítulos.
* Detectar capítulos.
* Detectar resolución.
* Detectar FPS.
* Detectar codec de video.
* Detectar codec de audio.
* Detectar sample rate.
* Detectar cantidad de canales.
* Detectar idioma de audio/subtítulos si existe.
* Detectar pix_fmt y bit depth.
* Detectar si hay HDR/Dolby Vision cuando sea posible.
* Detectar posibles señales de interlacing.
* Detectar warnings.
* Calcular un score de confianza.
* Sugerir un video profile.
* Sugerir un audio profile.
* Decidir siguiente paso:

  * Ready to Queue
  * Needs Review
  * Send to Lab
  * Analysis Failed

---

### 4.3 Cuándo debe correr Analysis

Analysis debe correr automáticamente después de Discovery si:

* El archivo está estable.
* La extensión está soportada.
* El archivo existe.
* El archivo está dentro de paths controlados por MediaForge.
* El asset no está marcado manualmente como ignored.
* El asset no está ya analizado con el mismo file signature.

Flujo:

```text
Discovery completed
  ↓
Is file stable?
  ↓
Is file supported?
  ↓
Run Analysis
  ↓
Generate AS-IS report
  ↓
Calculate confidence score
  ↓
Suggest next step
```

También debe poder correr manualmente desde UI:

```text
Analyze
Re-analyze
Open AS-IS Report
```

---

### 4.4 Estados internos de Analysis

Estados sugeridos:

```text
ANALYSIS_PENDING
ANALYZING
ANALYZED
ANALYSIS_WARNING
ANALYSIS_FAILED
NEEDS_REVIEW
READY_TO_QUEUE
READY_FOR_LAB
```

Uso recomendado:

* `ANALYSIS_PENDING`: esperando análisis.
* `ANALYZING`: ffprobe en ejecución.
* `ANALYZED`: análisis exitoso.
* `ANALYSIS_WARNING`: análisis exitoso con warnings.
* `ANALYSIS_FAILED`: ffprobe falló o metadata inválida.
* `NEEDS_REVIEW`: requiere aprobación humana.
* `READY_TO_QUEUE`: puede entrar a Queue.
* `READY_FOR_LAB`: se recomienda probar en Lab.

---

## 5. Reporte AS-IS

Analysis debe generar un reporte AS-IS antes de cualquier conversión.

El reporte debe guardarse en storage persistente del host/NAS.

Path sugerido configurable:

```text
/media/reports/as-is/
```

Naming sugerido:

```text
<asset_id>-as-is-<asset-name>-<date>.json
```

Ejemplo:

```text
asset_123-as-is-rayearth-ep01-2026-07-03.json
```

---

### 5.1 Ejemplo de reporte AS-IS

```json
{
  "report_type": "AS_IS",
  "asset_id": "asset_123",
  "source_path": "/media/raw/anime/rayearth/ep01.mkv",
  "file_name": "ep01.mkv",
  "created_at": "2026-07-03T00:00:00Z",
  "container": {
    "format_name": "matroska,webm",
    "duration_seconds": 1526.4,
    "size_bytes": 1234567890,
    "bit_rate": 6400000
  },
  "video_streams": [
    {
      "index": 0,
      "codec": "h264",
      "profile": "High",
      "width": 640,
      "height": 480,
      "fps": "29.97",
      "pix_fmt": "yuv420p",
      "bit_depth": 8,
      "color_space": "unknown",
      "field_order": "unknown"
    }
  ],
  "audio_streams": [
    {
      "index": 1,
      "codec": "aac",
      "channels": 2,
      "channel_layout": "stereo",
      "sample_rate": 44100,
      "language": "und",
      "bit_rate": 192000
    }
  ],
  "subtitle_streams": [],
  "chapters": {
    "count": 0,
    "present": false
  },
  "warnings": [
    {
      "code": "UNKNOWN_AUDIO_LANGUAGE",
      "severity": "LOW",
      "message": "Audio language is undefined."
    },
    {
      "code": "LOW_RESOLUTION",
      "severity": "LOW",
      "message": "Video resolution is SD or lower."
    }
  ],
  "confidence_score": 82,
  "recommendation": {
    "next_step": "READY_TO_QUEUE",
    "suggested_video_profile": "Anime SD x265 10-bit",
    "suggested_audio_profile": "AAC Stereo Normalize",
    "requires_manual_review": false
  }
}
```

---

## 6. Confidence Score

Analysis debe calcular un score de confianza para determinar si el asset puede avanzar automáticamente.

Rango:

```text
0–100
```

Interpretación:

```text
90–100: Safe / Auto Queue permitido si setting está activo.
70–89: Revisión recomendada.
0–69: Needs Review obligatorio.
```

---

### 6.1 Reglas sugeridas de scoring

Puntos positivos:

```text
+20 ffprobe exitoso
+15 duración válida
+15 tiene video stream
+10 tiene audio stream
+10 codec de video conocido
+10 codec de audio conocido
+10 estructura simple: 1 video + 1 audio
+5 subtítulos detectados correctamente
+5 capítulos detectados o ausencia válida
+5 nombre/path claro
```

Penalizaciones:

```text
-5 idioma unknown
-10 audio mono raro
-10 FPS raro
-10 interlaced probable
-15 múltiples audios sin idioma
-15 múltiples subtítulos sin idioma
-20 duración sospechosa
-20 HDR/Dolby Vision detectado
-20 archivo muy pequeño para su duración
-30 streams corruptos o metadata incompleta
-50 ffprobe falla parcialmente
-100 archivo ilegible
```

---

### 6.2 Severidad de warnings

Los warnings deben tener severidad:

```text
LOW
MEDIUM
HIGH
CRITICAL
```

Ejemplos:

```text
LOW:
- Unknown audio language
- Missing chapters
- Low resolution

MEDIUM:
- Multiple audio tracks without language
- Multiple subtitle tracks without language
- Mono audio
- Unusual FPS

HIGH:
- HDR/Dolby Vision
- Interlaced content
- Duration mismatch
- Audio/video stream mismatch

CRITICAL:
- ffprobe failed
- No video stream
- No audio stream
- File not readable
- File disappeared from filesystem
```

---

## 7. Decision Engine

Después de Analysis, MediaForge debe decidir el siguiente estado del asset.

Flujo:

```text
Analysis completed
  ↓
Calculate confidence score
  ↓
Evaluate warnings
  ↓
Suggest profile
  ↓
Choose next state
```

Estados de salida:

```text
READY_TO_QUEUE
NEEDS_REVIEW
READY_FOR_LAB
ANALYSIS_FAILED
IGNORED
```

---

### 7.1 Reglas de decisión

Asset puede ir a `READY_TO_QUEUE` si:

```text
- ffprobe fue exitoso
- score >= minimum_auto_queue_score
- no tiene warnings HIGH o CRITICAL
- existe video profile compatible
- existe audio profile compatible
- destination library está clara
- no está marcado manualmente como Needs Review
```

Asset debe ir a `NEEDS_REVIEW` si:

```text
- score < 70
- tiene warnings HIGH o CRITICAL
- tiene múltiples pistas de audio sin idioma
- tiene múltiples subtítulos sin idioma
- tiene HDR/Dolby Vision
- tiene interlacing probable
- tiene duración sospechosa
- no existe perfil compatible
- el usuario lo marcó como Needs Review
```

Asset debe ir a `READY_FOR_LAB` si:

```text
- score está entre 70 y 89
- el asset parece viejo, SD, TV rip, DVD rip o anime antiguo
- hay audio mono/stereo problemático
- hay necesidad probable de corrección de color
- hay necesidad probable de restauración de audio
- se detectan filtros sugeridos pero no confirmados
```

---

## 8. Auto Queue

Por seguridad, Auto Queue debe estar desactivado por default en V2.

Setting sugerido:

```text
Enable Auto Queue: false
Minimum score for Auto Queue: 90
```

Si está habilitado:

```text
Analysis completed
  ↓
Score >= 90
  ↓
No high/critical warnings
  ↓
Profile and destination resolved
  ↓
Create queue job automatically
```

Si no está habilitado:

```text
Analysis completed
  ↓
READY_TO_QUEUE
  ↓
User manually clicks "Add to Queue"
```

---

## 9. Lab como compuerta de calidad

Lab debe usarse cuando Analysis detecta que no conviene convertir a ciegas.

Ejemplos:

```text
Anime viejo
TV rip
DVD rip
Baja resolución
Color lavado
Interlacing
Audio mono/stereo raro
Audio con ruido
FPS sospechoso
```

Flujo:

```text
Analysis completed
  ↓
Risk detected
  ↓
Send to Lab suggested
  ↓
User selects start time and duration
  ↓
Generate Sample A original
  ↓
Generate Sample B processed
  ↓
User compares A/B
  ↓
User approves or edits profile
  ↓
Asset becomes READY_TO_QUEUE
```

---

## 10. UI recomendada

### 10.1 Assets page

Agregar filtros:

```text
All
Discovered
Analyzed
Needs Review
Ready to Queue
Ready for Lab
Queued
Failed
Ignored
```

Cada row debe mostrar información compacta:

```text
Asset name
Folder/path
Status
Confidence score
Suggested profile
Warnings count
Next step
```

Ejemplo:

```text
RAYEARTH EP01.mkv
Status: ANALYZED
Confidence: 82%
Suggested video profile: Anime SD x265 10-bit
Suggested audio profile: AAC Stereo Normalize
Warnings: 2
Next step: Review or Send to Lab
```

Acciones recomendadas:

```text
Analyze
Re-analyze
Open AS-IS Report
Send to Lab
Approve for Queue
Mark Needs Review
Ignore
```

Detalles avanzados deben ir en modal o accordion, no en la row principal.

---

### 10.2 Discovery/Analysis dashboard cards

Agregar tarjetas al Dashboard:

```text
Discovered Assets
Analysis Pending
Needs Review
Ready to Queue
Ready for Lab
Analysis Failed
```

Cada tarjeta debe enlazar a Assets con el filtro correspondiente.

---

## 11. Settings recomendados

### 11.1 Discovery Settings

```text
Enable file watcher
Auto-discover new files
Scan raw path on startup
Wait until file is stable: 120 seconds
Ignore hidden files
Ignore temporary files
Ignore unsupported extensions
Auto-group assets by folder
Mark unknown files as Needs Review
```

---

### 11.2 Analysis Settings

```text
Auto-analyze after discovery
Generate AS-IS report
Persist ffprobe raw output
Enable confidence score
Auto-suggest video profile
Auto-suggest audio profile
Auto-tag warnings
Send risky assets to Needs Review
Send medium-risk assets to Lab
```

---

### 11.3 Automation Settings

```text
Enable Auto Queue: false
Minimum score for Auto Queue: 90
Minimum score for Auto Publish: 95
Require manual review for HDR
Require manual review for Dolby Vision
Require manual review for interlaced content
Require manual review for multiple audio tracks
Require manual review for unknown language
Require manual review for audio restoration candidates
Require manual review for color correction candidates
```

---

## 12. API sugerida

### Discovery

```text
POST /api/discovery/scan
POST /api/discovery/scan-folder
GET  /api/discovery/status
```

### Analysis

```text
POST /api/assets/{id}/analyze
POST /api/assets/{id}/reanalyze
GET  /api/assets/{id}/analysis
GET  /api/assets/{id}/as-is-report
```

### Decision

```text
POST /api/assets/{id}/mark-needs-review
POST /api/assets/{id}/approve-for-queue
POST /api/assets/{id}/send-to-lab
POST /api/assets/{id}/ignore
```

---

## 13. Backend implementation notes

### 13.1 Discovery service

Crear un servicio interno:

```go
type DiscoveryService interface {
    ScanRawLibrary(ctx context.Context) error
    ScanFolder(ctx context.Context, path string) error
    DiscoverAsset(ctx context.Context, path string) (*Asset, error)
}
```

Responsabilidades:

* Validar paths.
* Evitar salir de rutas controladas.
* Detectar estabilidad del archivo.
* Crear/actualizar asset.
* Emitir eventos internos.

---

### 13.2 Analysis service

Crear un servicio interno:

```go
type AnalysisService interface {
    AnalyzeAsset(ctx context.Context, assetID string) (*AnalysisResult, error)
    ReanalyzeAsset(ctx context.Context, assetID string) (*AnalysisResult, error)
}
```

Responsabilidades:

* Ejecutar `ffprobe`.
* Parsear JSON.
* Guardar raw ffprobe output opcional.
* Crear reporte AS-IS.
* Calcular warnings.
* Calcular score.
* Sugerir perfiles.
* Actualizar estado del asset.

---

### 13.3 Decision service

Crear un servicio interno:

```go
type DecisionService interface {
    EvaluateAsset(ctx context.Context, assetID string) (*DecisionResult, error)
}
```

Responsabilidades:

* Leer AnalysisResult.
* Leer Settings.
* Evaluar warnings.
* Evaluar score.
* Decidir siguiente estado.
* Sugerir perfil.
* Bloquear automatización si hay riesgo.

---

## 14. Reglas de seguridad

MediaForge debe ser estricto con filesystem safety.

Reglas:

```text
- Nunca borrar archivos fuera de paths controlados.
- Nunca mover archivos fuera de paths configurados.
- Nunca publicar si Validation no pasó.
- Nunca archivar originales si Publisher falló.
- Nunca hacer Auto Queue si el asset está marcado como Needs Review.
- Nunca hacer Auto Publish si hay warnings HIGH o CRITICAL.
- Nunca procesar archivos todavía inestables.
- Nunca asumir destination library si no está configurada.
```

---

## 15. Relación con futuras fases de IA

Discovery y Analysis deben preparar datos para futuros agentes IA.

Por eso cada asset debe guardar:

```text
- AS-IS report
- Result report
- ffmpeg command used
- profile used
- warnings before conversion
- validation result
- user decision
- manual tags
- review reason
- final status
```

Esto permitirá futuros agentes:

```text
Video Correction Agent
Audio Restoration Agent
Subtitle/Translation Agent
Profile Recommendation Agent
Failure Analysis Agent
```

La IA futura no debe tomar decisiones destructivas al inicio.

Primera etapa recomendada:

```text
AI suggests.
User approves.
MediaForge executes.
```

---

## 16. Implementación por fases

### Fase 1 — Discovery sólido

Objetivo:

```text
Detectar y registrar assets de forma confiable.
```

Entregables:

```text
- Scan on startup
- Manual rescan
- File stability check
- Asset registration
- Discovery status
- Ignore unsupported files
- Basic duplicate detection
```

---

### Fase 2 — Analysis automático

Objetivo:

```text
Ejecutar ffprobe y generar AS-IS report.
```

Entregables:

```text
- ffprobe integration
- Parse streams
- Store container metadata
- Store video/audio/subtitle metadata
- Persist AS-IS report
- Analysis status
- Re-analyze action
```

---

### Fase 3 — Warnings y confidence score

Objetivo:

```text
Convertir metadata técnica en decisiones útiles.
```

Entregables:

```text
- Warning engine
- Severity levels
- Confidence score
- Explanation of score
- UI badges
- Filter by warnings
```

---

### Fase 4 — Profile suggestion

Objetivo:

```text
Sugerir perfiles de video/audio según el asset.
```

Entregables:

```text
- Suggested video profile
- Suggested audio profile
- Default profile rules
- Anime/SD/DVD/TV rip profile mapping
- Manual override
```

---

### Fase 5 — Decision Engine

Objetivo:

```text
Decidir si el asset pasa a Queue, Needs Review o Lab.
```

Entregables:

```text
- READY_TO_QUEUE
- NEEDS_REVIEW
- READY_FOR_LAB
- ANALYSIS_FAILED
- Automation eligibility
- Manual approval flow
```

---

### Fase 6 — Auto Queue opcional

Objetivo:

```text
Permitir automatización segura para assets confiables.
```

Entregables:

```text
- Enable/disable Auto Queue
- Minimum score setting
- Block Auto Queue on warnings
- Audit event when job is auto-created
```

---

## 17. Resultado esperado

Al terminar esta fase, MediaForge debe operar así:

```text
1. Usuario copia archivos a /media/raw.
2. MediaForge detecta archivos nuevos.
3. Discovery registra assets.
4. Analysis inspecciona assets con ffprobe.
5. Se genera reporte AS-IS.
6. Se calculan warnings y confidence score.
7. MediaForge sugiere perfil y destino.
8. Assets seguros quedan Ready to Queue.
9. Assets dudosos van a Needs Review o Lab.
10. Usuario mantiene control antes de convertir o publicar.
```

La experiencia ideal para el usuario:

```text
MediaForge me dice qué tengo, qué tan confiable es, qué perfil usar, qué necesita revisión y cuál es el siguiente paso.
```

---

## 18. Principio de diseño para Codex

Al implementar esta fase, priorizar:

```text
Seguridad > Automatización
Claridad > Complejidad
Acciones explícitas > Magia invisible
Reports persistentes > Estado temporal
UI limpia > Detalles técnicos saturados
Automatización configurable > Automatización forzada
```

MediaForge debe sentirse como un Lightroom de multimedia/video:

```text
Importar → Analizar → Comparar → Ajustar → Procesar → Validar → Publicar
```

Pero para V2, Discovery y Analysis deben ser la base confiable antes de crecer hacia restauración avanzada, IA, comunidad de perfiles y automatización más agresiva.
