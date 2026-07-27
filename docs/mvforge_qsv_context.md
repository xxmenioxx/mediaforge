# MVForge — Contexto técnico para Intel Quick Sync Video (QSV)

## 1. Propósito

Este documento define cómo debe integrar MVForge la aceleración por hardware Intel Quick Sync Video mediante FFmpeg, especialmente para codificación HEVC/H.265 con `hevc_qsv`.

El objetivo no es únicamente agregar una opción de hardware encode. MVForge debe proporcionar:

- Detección automática de capacidades reales del host.
- Perfiles QSV fáciles de usar.
- Opciones avanzadas para usuarios técnicos.
- Validación previa de parámetros.
- Fallbacks seguros cuando una opción no esté soportada.
- Comparaciones A/B mediante Profile Lab.
- Preservación del máster cuando el contenido sea importante.
- Reportes que expliquen exactamente qué ruta de procesamiento se utilizó.

QSV debe considerarse principalmente como una opción para generar derivados optimizados de reproducción y distribución local, no como sustituto automático del máster original.

---

## 2. Principios generales

### 2.1 QSV no utiliza CRF

`hevc_qsv` no utiliza CRF de la misma manera que `libx265`.

MVForge debe evitar presentar ICQ como una equivalencia exacta de CRF.

Referencia aproximada inicial:

| x265 software | QSV aproximado |
|---|---:|
| CRF 18 | ICQ 15–17 |
| CRF 20 | ICQ 17–19 |
| CRF 22 | ICQ 20–22 |
| CRF 24 | ICQ 23–25 |

Valor inicial recomendado para una calidad similar a x265 CRF 20:

```text
ICQ / global_quality = 18
```

Para contenido importante, difícil o con mucho grano:

```text
ICQ / global_quality = 16–17
```

La equivalencia real depende de:

- Resolución.
- Ruido y grano.
- Movimiento.
- Animación.
- Escenas oscuras.
- Fuente DVD o TV rip.
- Generación de la iGPU Intel.
- Driver instalado.
- Versión de FFmpeg y oneVPL.

---

## 3. Casos de uso recomendados

### 3.1 QSV sí es recomendado para

- Copias de reproducción para Jellyfin o Plex.
- Conversión rápida de bibliotecas grandes.
- Procesamiento en NAS con CPU limitada.
- Archivos donde velocidad y consumo importan.
- Transcodes operativos.
- Versiones compatibles con dispositivos.
- Previews y muestras en Profile Lab.

### 3.2 QSV no debe reemplazar automáticamente

- ISO del DVD.
- Estructura `VIDEO_TS`.
- MKV remux de MakeMKV.
- Máster MPEG-2 original.
- Archivos de preservación.
- Fuentes irreemplazables.

Para contenido importante, MVForge debe manejar esta relación:

```text
Máster original intacto
        ↓
Derivado QSV High Quality
        ↓
Copia optimizada para reproducción diaria
```

---

## 4. Rol del asset

Agregar un concepto explícito de rol del asset.

```ts
export type AssetRole =
  | 'master'
  | 'source'
  | 'derivative'
  | 'preview';
```

Ejemplo de política para máster:

```json
{
  "assetRole": "master",
  "immutable": true,
  "allowOverwrite": false,
  "allowAutomaticDeletion": false,
  "preserveIndefinitely": true
}
```

Reglas:

- Un asset `master` nunca debe eliminarse por una política automática de retención.
- Un asset `master` no debe sobreescribirse.
- Los derivados deben mantener referencia al máster de origen.
- El historial debe registrar qué perfil generó cada derivado.

---

## 5. Detección de capacidades QSV

MVForge no debe asumir que todas las opciones QSV funcionan.

La disponibilidad depende de:

- Generación de CPU/iGPU.
- Kernel y permisos de `/dev/dri`.
- Driver Intel.
- oneVPL o Media SDK.
- Build de FFmpeg.
- Contenedor utilizado.

### 5.1 Detección básica

Validar existencia del dispositivo:

```bash
ls -l /dev/dri
```

Dispositivo esperado normalmente:

```text
/dev/dri/renderD128
```

Verificar aceleradores disponibles:

```bash
ffmpeg -hide_banner -hwaccels
```

Verificar encoder:

```bash
ffmpeg -hide_banner -encoders | grep qsv
```

Leer opciones del encoder:

```bash
ffmpeg -hide_banner -h encoder=hevc_qsv
```

### 5.2 Prueba real obligatoria

Una opción puede aparecer en la ayuda de FFmpeg y aun así fallar con el driver o hardware actual.

MVForge debe ejecutar una prueba corta de 1–2 segundos.

Ejemplo:

```bash
ffmpeg \
  -hide_banner \
  -f lavfi \
  -i testsrc2=size=1280x720:rate=30 \
  -t 2 \
  -c:v hevc_qsv \
  -preset slow \
  -global_quality 18 \
  -f null -
```

Después debe probar funciones avanzadas por separado:

- Main 10.
- P010.
- Look Ahead.
- Extended BRC.
- Adaptive I.
- Adaptive B.
- Low Power.
- Hardware decode.
- Hardware filters.

### 5.3 Resultado del capability probe

Ejemplo de estructura persistida:

```json
{
  "encoder": "hevc_qsv",
  "device": "/dev/dri/renderD128",
  "supported": true,
  "profiles": ["main", "main10"],
  "pixelFormats": ["nv12", "p010le", "qsv"],
  "rateControls": ["icq", "la_icq", "qvbr", "vbr", "cbr", "cqp"],
  "features": {
    "lookAhead": true,
    "extendedBrc": true,
    "adaptiveI": true,
    "adaptiveB": true,
    "bFrameStrategy": true,
    "lowPower": true,
    "hardwareDecode": true,
    "hardwareFilters": true
  },
  "driver": "detected-driver-version",
  "ffmpegVersion": "detected-version",
  "testedAt": "2026-07-27T00:00:00Z"
}
```

Este resultado debe asociarse al worker.

---

## 6. Modelo de configuración QSV

### 6.1 Rate control

```ts
export type QsvRateControl =
  | 'icq'
  | 'la_icq'
  | 'qvbr'
  | 'vbr'
  | 'cbr'
  | 'cqp';
```

Presentación amigable en UI:

| UI | Implementación |
|---|---|
| Calidad constante | ICQ |
| Calidad constante mejorada | LA-ICQ |
| Calidad con límite de bitrate | QVBR |
| Bitrate variable | VBR |
| Bitrate fijo | CBR |
| Cuantización fija | CQP |

Recomendaciones:

- `ICQ`: opción predeterminada más segura.
- `LA_ICQ`: usar sólo si una prueba real confirma soporte.
- `QVBR`: útil cuando se requiere calidad objetivo con límite de bitrate.
- `VBR` y `CBR`: para streaming o requisitos de ancho de banda.
- `CQP`: sólo para Lab o usuarios avanzados.

### 6.2 Presets

```ts
export type QsvPreset =
  | 'veryfast'
  | 'faster'
  | 'fast'
  | 'medium'
  | 'slow'
  | 'slower'
  | 'veryslow';
```

Recomendaciones:

- `medium`: balance general.
- `slow`: máxima calidad práctica.
- `slower`: experimental.
- `veryslow`: sólo benchmark o Lab.

MVForge no debe asumir que `veryslow` ofrece una mejora proporcional comparable con x265 por software.

### 6.3 Configuración TypeScript sugerida

```ts
export type QsvLookAheadConfig = {
  enabled: boolean;
  depth: number;
};

export type QsvGopConfig = {
  mode: 'auto' | 'frames' | 'seconds';
  frames?: number;
  seconds?: number;
};

export type QsvFallbackConfig = {
  disableUnsupportedOptions: boolean;
  fallbackRateControl: QsvRateControl;
  fallbackPreset: QsvPreset;
  fallbackToSoftware: boolean;
};

export type QsvVideoProfile = {
  codec: 'hevc_qsv' | 'h264_qsv' | 'av1_qsv';
  hardwareDevice: 'auto' | string;
  rateControl: QsvRateControl;
  globalQuality?: number;
  bitrate?: string;
  maxBitrate?: string;
  bufferSize?: string;
  preset: QsvPreset;
  profile: 'main' | 'main10';
  pixelFormat: 'nv12' | 'p010le';
  lookAhead: QsvLookAheadConfig;
  extendedBrc: 'auto' | 'enabled' | 'disabled';
  adaptiveIFrames: boolean;
  adaptiveBFrames: boolean;
  bFrameStrategy: boolean;
  bFrames?: number;
  referenceFrames?: number;
  gop: QsvGopConfig;
  lowPower: 'auto' | 'enabled' | 'disabled';
  hardwareDecode: boolean;
  hardwareFilters: boolean;
  fallback: QsvFallbackConfig;
};
```

---

## 7. Perfil QSV High Quality

Perfil predeterminado recomendado para derivados de alta calidad:

```json
{
  "name": "QSV High Quality",
  "codec": "hevc_qsv",
  "hardwareDevice": "auto",
  "rateControl": "la_icq",
  "globalQuality": 17,
  "preset": "slow",
  "profile": "main10",
  "pixelFormat": "p010le",
  "lookAhead": {
    "enabled": true,
    "depth": 40
  },
  "extendedBrc": "auto",
  "adaptiveIFrames": true,
  "adaptiveBFrames": true,
  "bFrameStrategy": true,
  "bFrames": 4,
  "referenceFrames": 4,
  "gop": {
    "mode": "seconds",
    "seconds": 8
  },
  "lowPower": "disabled",
  "hardwareDecode": true,
  "hardwareFilters": true,
  "fallback": {
    "disableUnsupportedOptions": true,
    "fallbackRateControl": "icq",
    "fallbackPreset": "medium",
    "fallbackToSoftware": false
  }
}
```

### 7.1 Lógica esperada

1. Intentar `LA_ICQ` si está soportado.
2. Intentar Look Ahead sólo si la prueba real fue exitosa.
3. Activar ExtBRC únicamente si es compatible con el modo seleccionado.
4. Si una opción falla, remover únicamente esa opción.
5. Si `LA_ICQ` falla, cambiar a `ICQ`.
6. No cambiar automáticamente a software salvo que el perfil lo permita.
7. Registrar todos los fallbacks aplicados.

---

## 8. Otros perfiles predeterminados

### 8.1 QSV Balanced

```json
{
  "name": "QSV Balanced",
  "codec": "hevc_qsv",
  "rateControl": "icq",
  "globalQuality": 18,
  "preset": "medium",
  "profile": "main10",
  "pixelFormat": "p010le",
  "lookAhead": {
    "enabled": false,
    "depth": 0
  },
  "extendedBrc": "auto",
  "adaptiveIFrames": true,
  "adaptiveBFrames": true,
  "lowPower": "disabled"
}
```

### 8.2 QSV Fast

```json
{
  "name": "QSV Fast",
  "codec": "hevc_qsv",
  "rateControl": "icq",
  "globalQuality": 20,
  "preset": "fast",
  "profile": "main",
  "pixelFormat": "nv12",
  "lookAhead": {
    "enabled": false,
    "depth": 0
  },
  "extendedBrc": "disabled",
  "adaptiveIFrames": true,
  "adaptiveBFrames": false,
  "lowPower": "auto"
}
```

### 8.3 DVD QSV High Quality

```json
{
  "name": "DVD QSV High Quality",
  "purpose": "playback-copy",
  "sourcePolicy": {
    "preserveMaster": true,
    "deleteMasterAfterPublish": false
  },
  "video": {
    "codec": "hevc_qsv",
    "rateControl": "icq",
    "globalQuality": 17,
    "preset": "slow",
    "profile": "main10",
    "pixelFormat": "p010le",
    "extendedBrc": "auto",
    "lookAheadDepth": 40,
    "lowPower": false
  },
  "analysis": {
    "detectInterlacing": true,
    "detectTelecine": true,
    "detectCadence": true,
    "detectAspectRatio": true,
    "requireReviewWhenUncertain": true
  },
  "audio": {
    "mode": "copy"
  },
  "subtitles": {
    "mode": "copy"
  },
  "chapters": {
    "preserve": true
  }
}
```

---

## 9. DVD: análisis previo obligatorio

Para fuentes DVD, el tratamiento de la cadencia es frecuentemente más importante que cambiar ICQ 18 a 17.

MVForge debe detectar:

- Contenido progresivo.
- Contenido entrelazado.
- Telecine NTSC.
- Progressive segmented frame.
- Cambios de cadencia.
- Combing.
- Field order.
- SAR y DAR.
- Resolución activa.
- Barras negras.
- Movimiento vertical u horizontal inherente a la fuente.

### 9.1 Clasificación sugerida

```ts
export type ScanType =
  | 'progressive'
  | 'interlaced-tff'
  | 'interlaced-bff'
  | 'telecine'
  | 'mixed'
  | 'unknown';
```

### 9.2 Reglas de procesamiento

#### DVD progresivo

```text
Sin desentrelazado → Encode QSV
```

#### Película NTSC con telecine

```text
fieldmatch → decimate → Encode QSV
```

#### Video realmente entrelazado

```text
bwdif → Encode QSV
```

#### Cadencia mixta o incierta

```text
Needs Review → Profile Lab → decisión manual
```

No aplicar `yadif` o `bwdif` automáticamente a todos los DVD.

---

## 10. Main vs Main 10

### 10.1 Main 10 recomendado cuando

- La fuente es 10-bit.
- El contenido es HDR.
- Anime con gradientes.
- Hay riesgo de banding.
- El dispositivo destino soporta HEVC Main 10.

### 10.2 Main recomendado cuando

- El dispositivo destino es antiguo.
- Se prioriza compatibilidad.
- El contenido es SDR 8-bit sin problemas visibles de gradientes.

### 10.3 Regla importante

Convertir una fuente SDR 8-bit a `p010le` no crea HDR ni detalle nuevo.

Puede ayudar a reducir banding durante el procesamiento, pero debe validarse en Profile Lab.

---

## 11. GOP automático

No usar siempre `-g 240`.

Calcular GOP por segundos:

```text
GOP frames = FPS × segundos entre keyframes
```

Valor recomendado:

```text
5–8 segundos
```

Ejemplo para 8 segundos:

| FPS | GOP aproximado |
|---:|---:|
| 23.976 | 192 |
| 24 | 192 |
| 25 | 200 |
| 29.97 | 240 |
| 30 | 240 |
| 50 | 400 |
| 59.94 | 480 |

La UI debe permitir:

- Automático.
- Segundos.
- Frames manuales.

---

## 12. Ruta de procesamiento por hardware

MVForge debe distinguir tres capacidades separadas:

```ts
hardwareDecode: boolean;
hardwareFilters: boolean;
hardwareEncode: boolean;
```

Rutas posibles:

```text
QSV decode → QSV/VPP filters → QSV encode
```

```text
QSV decode → CPU filters → QSV encode
```

```text
Software decode → CPU filters → QSV encode
```

La UI debe mostrar claramente:

```text
Hardware path: Full QSV
```

```text
Hardware path: QSV decode + CPU filter + QSV encode
```

```text
Hardware path: Software decode + QSV encode
```

No basta con mostrar únicamente “Hardware enabled”.

---

## 13. Inicialización QSV en FFmpeg

Ejemplo Linux:

```bash
ffmpeg \
  -init_hw_device qsv=hw:/dev/dri/renderD128 \
  -filter_hw_device hw \
  -i input.mkv \
  -c:v hevc_qsv \
  output.mkv
```

Ejemplo de decode por hardware:

```bash
ffmpeg \
  -init_hw_device qsv=hw:/dev/dri/renderD128 \
  -filter_hw_device hw \
  -hwaccel qsv \
  -hwaccel_output_format qsv \
  -i input.mkv \
  -c:v hevc_qsv \
  output.mkv
```

El generador de comandos debe evitar agregar opciones incompatibles con el tipo de filtro seleccionado.

---

## 14. Comando base QSV High Quality

Ejemplo orientativo:

```bash
ffmpeg \
  -init_hw_device qsv=hw:/dev/dri/renderD128 \
  -filter_hw_device hw \
  -i input.mkv \
  -map 0 \
  -c:v hevc_qsv \
  -profile:v main10 \
  -pix_fmt p010le \
  -preset slow \
  -global_quality 17 \
  -look_ahead 1 \
  -look_ahead_depth 40 \
  -extbrc 1 \
  -adaptive_i 1 \
  -adaptive_b 1 \
  -b_strategy 1 \
  -bf 4 \
  -refs 4 \
  -g 192 \
  -c:a copy \
  -c:s copy \
  output.mkv
```

Notas:

- El valor de `-g` debe calcularse según los FPS.
- No todas las opciones deben enviarse si no fueron validadas.
- `-look_ahead 1` y `-extbrc 1` deben depender del capability probe.
- `-map 0` requiere manejo especial de attachments y data streams.
- Los subtítulos incompatibles con el contenedor destino deben transformarse o excluirse de forma explícita.

---

## 15. Low Power

`low_power` no debe activarse en el perfil de máxima calidad.

Política recomendada:

```text
QSV High Quality → low_power disabled
QSV Balanced → low_power disabled o auto
QSV Fast → low_power auto
```

La opción Low Power prioriza velocidad y eficiencia energética, no necesariamente la mejor calidad visual.

---

## 16. Fallbacks

El sistema debe aplicar fallbacks graduales.

Orden recomendado:

1. Remover `look_ahead` si falla.
2. Remover `extbrc` si falla.
3. Remover adaptive B si falla.
4. Cambiar `LA_ICQ` a `ICQ`.
5. Cambiar `main10/p010le` a `main/nv12` si no está soportado.
6. Cambiar preset `slow` a `medium` si el driver rechaza el preset.
7. Usar software sólo si el perfil lo permite explícitamente.

Ejemplo de reporte:

```json
{
  "requestedEncoder": "hevc_qsv",
  "requestedRateControl": "la_icq",
  "effectiveRateControl": "icq",
  "disabledOptions": [
    "look_ahead",
    "extended_brc"
  ],
  "fallbackReason": "Driver rejected look-ahead initialization",
  "fallbackToSoftware": false
}
```

---

## 17. Profile Lab para QSV

Profile Lab debe permitir comparar:

- Original vs QSV High Quality.
- QSV ICQ 16 vs 17 vs 18.
- QSV vs x265 software.
- Main vs Main 10.
- Look Ahead on/off.
- ExtBRC on/off.
- Hardware filters vs CPU filters.

Métricas sugeridas:

- Tamaño estimado.
- Bitrate promedio.
- FPS de encode.
- Tiempo estimado.
- VMAF.
- SSIM.
- PSNR.
- Diferencia visual ampliada.
- Frames problemáticos.
- Banding.
- Blurring.
- Blocking.
- Pérdida de grano.

Las métricas no deben reemplazar la revisión visual.

---

## 18. UI recomendada

### 18.1 Vista simple

Mostrar:

- Encoder: Intel Quick Sync.
- Modo: High Quality / Balanced / Fast.
- Calidad: selector amigable.
- Compatibilidad: Main o Main 10.
- Preservar máster.
- Procesamiento totalmente por hardware: sí/no.

### 18.2 Vista avanzada

Mostrar:

- Rate control.
- Global quality.
- Preset.
- Look Ahead.
- Look Ahead depth.
- ExtBRC.
- Adaptive I/B.
- B-frames.
- Reference frames.
- GOP.
- Low Power.
- Pixel format.
- Hardware decode.
- Hardware filters.
- Fallback policy.

### 18.3 Mensajes explicativos

Ejemplo:

```text
QSV High Quality crea una copia de reproducción de alta calidad utilizando la iGPU Intel. No reemplaza el máster original.
```

Ejemplo para Main 10:

```text
Main 10 puede reducir banding y conservar mejor gradientes, pero requiere dispositivos compatibles.
```

---

## 19. Validación posterior

Después de codificar, validar:

- Codec de salida.
- Profile HEVC.
- Pixel format.
- Resolución.
- FPS.
- Duración.
- SAR/DAR.
- Field order.
- Audio tracks.
- Subtitle tracks.
- Chapters.
- Attachments.
- HDR metadata.
- Tamaño.
- Bitrate.
- Errores de decodificación.
- Frames corruptos.
- Diferencias de duración.
- Cambios inesperados en número de pistas.

Para DVD:

- Confirmar que el telecine fue removido correctamente.
- Confirmar que no se introdujo combing.
- Confirmar que el aspect ratio se preservó.
- Confirmar que no hay frame duplication excesiva.

---

## 20. Reportes AS-IS y Result

### 20.1 AS-IS

Guardar:

- Hardware detectado.
- Driver.
- FFmpeg.
- Encoder solicitado.
- Capacidades disponibles.
- Fuente original.
- Streams.
- Scan type.
- Telecine confidence.
- Aspect ratio.
- Pixel format.
- HDR.
- Duración.
- Bitrate.

### 20.2 Result

Guardar:

- Comando efectivo.
- Opciones solicitadas.
- Opciones aplicadas.
- Fallbacks.
- Hardware path efectivo.
- FPS promedio de encode.
- Tiempo total.
- Tamaño final.
- Ahorro.
- Métricas visuales.
- Validation score.
- Warnings.
- Errores del driver.

Ejemplo:

```json
{
  "hardwarePath": "qsv-decode-cpu-filter-qsv-encode",
  "encoder": "hevc_qsv",
  "rateControl": "icq",
  "globalQuality": 17,
  "preset": "slow",
  "profile": "main10",
  "pixelFormat": "p010le",
  "lookAheadRequested": true,
  "lookAheadApplied": false,
  "extendedBrcRequested": "auto",
  "extendedBrcApplied": false,
  "fallbacks": [
    "la_icq_to_icq",
    "disable_look_ahead"
  ]
}
```

---

## 21. Scheduler y workers

Cada worker debe publicar:

```ts
export type WorkerHardwareCapabilities = {
  qsvAvailable: boolean;
  qsvDevice?: string;
  supportedEncoders: string[];
  supportedProfiles: string[];
  maxHardwareEncodeJobs: number;
  fullHardwarePipelineSupported: boolean;
};
```

El scheduler debe:

- Enviar jobs QSV sólo a workers con QSV validado.
- Respetar `maxHardwareEncodeJobs`.
- Evitar saturar la iGPU.
- Separar límites de encode software y hardware.
- Considerar filtros CPU al calcular capacidad.
- Registrar qué worker ejecutó el job.

---

## 22. Reglas de decisión automáticas

### 22.1 Reglas sugeridas

```text
Fuente HDR/10-bit + QSV Main10 soportado
→ HEVC Main10 QSV
```

```text
DVD telecine con confianza alta
→ fieldmatch + decimate + QSV
```

```text
DVD entrelazado con confianza alta
→ bwdif + QSV
```

```text
DVD mixed/unknown
→ Needs Review
```

```text
Máster marcado como importante
→ preservar máster + generar derivado
```

```text
Dispositivo destino no soporta Main10
→ Main + nv12
```

```text
Filtros seleccionados no soportan hardware frames
→ hwdownload / CPU filters / hwupload
```

---

## 23. Criterios de éxito

La integración se considera completa cuando:

- MVForge detecta QSV automáticamente.
- Puede ejecutar un capability probe real.
- Crea perfiles High Quality, Balanced y Fast.
- Nunca trata ICQ como CRF exacto.
- Preserva másters importantes.
- Detecta DVD progresivo, entrelazado y telecine.
- Aplica fallbacks sin perder el job completo.
- Explica la ruta de hardware utilizada.
- Registra las opciones efectivas.
- Profile Lab permite comparar QSV contra software.
- Validation detecta problemas de cadencia, duración y streams.

---

## 24. Prioridad de implementación

### Fase 1

- Detectar `/dev/dri/renderD128`.
- Detectar `hevc_qsv`.
- Capability probe básico.
- Perfil QSV Balanced.
- ICQ.
- Main/Main10.
- Preset.
- Reporte de comando efectivo.

### Fase 2

- QSV High Quality.
- Look Ahead.
- ExtBRC.
- Adaptive I/B.
- Fallbacks granulares.
- Hardware path visible.

### Fase 3

- Detección de telecine e interlacing.
- Perfil DVD QSV High Quality.
- Profile Lab QSV vs x265.
- Métricas VMAF/SSIM/PSNR.

### Fase 4

- Reglas automáticas por dispositivo destino.
- Recomendaciones del Advisor.
- Aprendizaje a partir de reportes AS-IS/Result.
- Autoajuste de calidad según tipo de contenido.

---

## 25. Decisión final recomendada

La configuración predeterminada de alta calidad para Intel Quick Sync en MVForge debe ser:

```text
Encoder: HEVC QSV
Rate control: LA-ICQ si está soportado; ICQ como fallback
Global quality: 17 para contenido importante
Preset: slow
Profile: Main 10 cuando sea compatible
Pixel format: P010
Look Ahead: 40 si está soportado
ExtBRC: auto
Adaptive I/B: enabled si está soportado
Low Power: disabled
GOP: 8 segundos
Audio: copy
Subtitles: copy o transformación explícita
Chapters: preserve
Master: preserve siempre cuando esté marcado como importante
```

QSV High Quality debe considerarse una excelente opción para generar copias optimizadas de reproducción. La preservación real debe seguir dependiendo del máster original sin recodificar.
