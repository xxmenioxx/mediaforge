# MVForge — Contexto para LAB de fidelidad visual y preservación de características

## Nombre del proyecto

MVForge (Media Vault Forge)

## Objetivo de esta fase

Rediseñar el **Profile LAB / Preview A-B** para que la comparación visual entre la fuente y una conversión sea técnicamente válida, reproducible y transparente.

Actualmente el navegador no siempre puede reproducir directamente el stream original. Por esa razón, MVForge genera una referencia H.264 compatible con navegador y la muestra como si fuera el “Original”. Esto puede introducir cambios de color, rango, gamma, matriz, pixel format, aspect ratio, chroma o desentrelazado antes de que el usuario compare contra la conversión evaluada.

El objetivo es evitar comparaciones engañosas y garantizar que ambas ramas del LAB partan de una misma interpretación canónica de la fuente.

---

# Problema actual

La UI muestra algo parecido a:

- Original · browser-compatible H.264 reference
- Converted · profile output

Pero la primera vista no siempre es el stream original. Puede ser una transcodificación intermedia generada por FFmpeg para que Chrome, Safari u otro navegador pueda reproducirla.

Esto implica que se están comparando dos pipelines independientes:

```text
Fuente real
  -> referencia H.264 compatible con navegador

Fuente real
  -> conversión del perfil evaluado
```

Si cada pipeline interpreta o transforma de forma distinta:

- color primaries
- color transfer
- color matrix
- color range
- pixel format
- chroma location
- field order
- deinterlace
- SAR/DAR
- frame rate

la diferencia observada puede haber sido introducida por la referencia del navegador y no por el encoder bajo prueba.

---

# Ejemplo real de fuente

Una fuente MPEG-2 de DVD puede reportar:

```text
codec_name=mpeg2video
profile=Main
width=720
height=480
sample_aspect_ratio=8:9
display_aspect_ratio=4:3
pix_fmt=yuv420p
color_range=tv
color_space=bt470bg
color_transfer=bt470m
color_primaries=bt470m
chroma_location=left
field_order=bb
```

Interpretación:

```text
Codec: MPEG-2 Main
Resolución almacenada: 720x480
SAR: 8:9
DAR: 4:3
Pixel format: yuv420p
Range: limited / tv
Matrix: BT.470BG
Transfer: BT.470M
Primaries: BT.470M
Chroma location: left
Field order: bottom field first
```

Esta metadata debe considerarse parte de las características de la fuente, aunque el LAB decida normalizar temporalmente la imagen para reproducirla en navegador.

---

# Principio principal

MVForge debe separar claramente tres conceptos:

1. **Características de la fuente**
2. **Normalización de preview**
3. **Características de la salida del perfil**

Nunca se debe presentar una referencia H.264 transcodificada como si fuera el archivo original sin aclararlo.

---

# Arquitectura propuesta

## Pipeline canónico compartido

Ambas ramas del LAB deben usar el mismo pipeline previo al encoder:

```text
Fuente
  -> decode
  -> interpretación explícita de metadata
  -> correcciones estructurales comunes
  -> canonical preview frames
      -> rama A: referencia compatible con navegador
      -> rama B: encoder/perfil evaluado
```

El filter graph previo a la división debe ser idéntico.

Ejemplo conceptual:

```text
decode
  -> field order handling
  -> deinterlace
  -> color conversion or preservation policy
  -> range handling
  -> pixel format normalization
  -> SAR/DAR handling
  -> progressive flag
  -> split
```

Después del `split`, solo debe variar el encoder y las opciones propias del perfil.

---

# Modos de manejo de color

MVForge debe incluir un campo explícito:

```text
Color handling
```

Opciones recomendadas:

## 1. Preserve source characteristics

Mantiene, cuando el encoder y el contenedor lo soporten:

- color primaries
- transfer
- matrix
- range
- chroma location
- SAR/DAR
- bit depth
- pixel format compatible

No debe cambiar etiquetas sin conversión real.

Este modo es ideal para conversión final y preservación de archivo.

## 2. Normalize preview to BT.709

Convierte matemáticamente fuentes legacy a un dominio común y compatible con navegador:

```text
Primaries: BT.709
Transfer: BT.709
Matrix: BT.709
Range: TV / limited
Pixel format: yuv420p
Scan: progressive
```

Debe usarse el filtro `colorspace`, `zscale` u otro mecanismo de conversión real.

No debe usarse `setparams` como sustituto de la conversión, porque `setparams` solo cambia metadata del frame.

## 3. Custom

Permite elegir manualmente:

- input primaries
- input transfer
- input matrix
- input range
- output primaries
- output transfer
- output matrix
- output range
- pixel format
- chroma location

---

# Diferencia entre etiquetar y convertir

## Etiquetar

Filtros u opciones como:

```text
setparams
-color_primaries
-color_trc
-colorspace
-color_range
```

pueden establecer metadata, pero no necesariamente cambian matemáticamente los valores de pixel.

## Convertir

Filtros como:

```text
colorspace
zscale
scale con matrices explícitas
```

sí pueden transformar la imagen entre matrices, transferencias, primarias y rangos.

MVForge debe registrar qué operación se realizó:

```json
{
  "operation": "convert",
  "from": {
    "matrix": "bt470bg",
    "primaries": "bt470m",
    "transfer": "bt470m",
    "range": "tv"
  },
  "to": {
    "matrix": "bt709",
    "primaries": "bt709",
    "transfer": "bt709",
    "range": "tv"
  }
}
```

---

# Estructura de datos propuesta

## Source characteristics

```ts
export type VideoColorCharacteristics = {
  pixelFormat?: string;
  bitDepth?: number;
  colorRange?: 'tv' | 'pc' | 'unknown';
  colorSpace?: string;
  colorTransfer?: string;
  colorPrimaries?: string;
  chromaLocation?: string;
};

export type VideoGeometryCharacteristics = {
  width: number;
  height: number;
  sampleAspectRatio?: string;
  displayAspectRatio?: string;
  frameRate?: string;
  fieldOrder?: string;
  detectedScanType?: 'progressive' | 'interlaced' | 'telecine' | 'mixed' | 'unknown';
};

export type SourceVideoCharacteristics = {
  codec: string;
  profile?: string;
  color: VideoColorCharacteristics;
  geometry: VideoGeometryCharacteristics;
};
```

## Preview normalization

```ts
export type PreviewNormalization = {
  enabled: boolean;
  mode: 'preserve' | 'normalize_bt709' | 'custom';
  deinterlaceApplied: boolean;
  deinterlaceFilter?: string;
  sourceFieldOrder?: string;
  outputFieldOrder?: 'progressive' | string;
  inputColor?: VideoColorCharacteristics;
  outputColor?: VideoColorCharacteristics;
  inputSar?: string;
  outputSar?: string;
  outputCodec: 'h264';
  outputContainer: 'mp4';
  reason: string;
};
```

## Conversion result characteristics

```ts
export type ConversionVideoCharacteristics = {
  encoder: string;
  codec: string;
  container: string;
  color: VideoColorCharacteristics;
  geometry: VideoGeometryCharacteristics;
  inputDomain: 'source' | 'canonical_preview';
};
```

## Fidelity comparison

```ts
export type FidelityFieldResult = {
  field: string;
  sourceValue?: string;
  referenceValue?: string;
  conversionValue?: string;
  status: 'equal' | 'normalized' | 'changed' | 'unknown';
  severity: 'info' | 'warning' | 'error';
  explanation?: string;
};

export type FidelityReport = {
  characteristics: FidelityFieldResult[];
  metadataMatch: boolean;
  canonicalDomainMatch: boolean;
  warnings: string[];
};
```

---

# Generación de referencia para navegador

La referencia para navegador debe:

- usar MP4
- usar H.264
- usar `yuv420p`
- usar `+faststart`
- declarar explícitamente el perfil de color de salida
- usar el mismo tramo temporal que la conversión
- usar el mismo pipeline de normalización previo al encoder
- conservar el mismo SAR/DAR deseado
- ser progresiva si se aplicó deinterlace

Nombre recomendado en UI:

```text
Referencia de la fuente
Normalizada para navegador
```

No usar solo:

```text
Original
```

Tooltip sugerido:

```text
Esta vista no reproduce directamente el stream original. MVForge decodificó la fuente y generó una referencia normalizada compatible con el navegador. Las características originales permanecen disponibles en el panel técnico.
```

---

# Comparación visual recomendada

## Modo video continuo

Mantener dos reproductores sincronizados:

- referencia H.264 canónica
- resultado del perfil

Mostrar claramente:

```text
Referencia normalizada
vs.
Resultado del perfil
```

## Modo Frame Fidelity

Agregar una comparación de frames estáticos decodificados por FFmpeg.

Proceso:

```text
Fuente
  -> pipeline canónico
  -> frame de referencia PNG

Conversión
  -> decode
  -> mismo espacio canónico
  -> frame convertido PNG
```

El navegador compara PNG contra PNG, evitando diferencias producidas por:

- decoder H.264 del navegador
- decoder HEVC del navegador
- interpretación del contenedor
- soporte parcial de metadata
- selección diferente de color management por codec

Funciones sugeridas:

- slider before/after
- side-by-side
- blink comparison
- zoom sincronizado
- selector de frame
- histogramas
- diferencia absoluta

---

# Validación automática

MVForge debe ejecutar `ffprobe` sobre:

1. fuente
2. referencia de navegador
3. salida del perfil

Campos mínimos:

```text
codec_name
profile
pix_fmt
width
height
sample_aspect_ratio
display_aspect_ratio
field_order
r_frame_rate
avg_frame_rate
color_range
color_space
color_transfer
color_primaries
chroma_location
```

Mostrar una tabla:

| Característica | Fuente | Referencia | Conversión | Resultado |
|---|---|---|---|---|
| Matrix | bt470bg | bt709 | bt709 | Normalizada |
| Primaries | bt470m | bt709 | bt709 | Normalizada |
| Transfer | bt470m | bt709 | bt709 | Normalizada |
| Range | tv | tv | tv | Igual |
| Pixel format | yuv420p | yuv420p | yuv420p | Igual |
| Field order | bb | progressive | progressive | Corregido |
| SAR | 8:9 | 8:9 | 8:9 | Igual |
| DAR | 4:3 | 4:3 | 4:3 | Igual |

Estados:

- Equal
- Preserved
- Normalized
- Changed intentionally
- Changed unexpectedly
- Unsupported by encoder
- Unknown

---

# Preservación de características en la conversión final

La conversión final no necesariamente debe usar el mismo dominio BT.709 del preview.

Se deben separar dos decisiones:

```text
Preview normalization policy
Final output color policy
```

Ejemplo:

```text
Preview:
BT.470 legacy -> BT.709 canonical

Final output:
Preserve BT.470 metadata
```

O bien:

```text
Preview:
BT.470 legacy -> BT.709 canonical

Final output:
Convert intentionally to BT.709
```

La UI debe mostrar esta diferencia para evitar que el usuario confunda la normalización temporal del LAB con la política final del archivo.

---

# Deinterlace y field order

Para fuentes con:

```text
field_order=bb
```

MVForge debe interpretar:

```text
bottom field first
```

y generar:

```text
bwdif parity=bff
```

La detección debe separar:

- declared field order
- detected scan type
- detected cadence
- confidence

No confiar únicamente en metadata.

Modelo sugerido:

```ts
export type ScanAnalysis = {
  declaredFieldOrder?: string;
  detectedScanType: 'progressive' | 'interlaced' | 'telecine' | 'mixed' | 'unknown';
  detectedParity?: 'tff' | 'bff' | 'unknown';
  cadence?: string;
  confidence: number;
};
```

---

# Compatibilidad de aliases FFmpeg

MVForge no debe copiar directamente valores textuales de `ffprobe` a opciones de FFmpeg.

Un valor aceptado por `ffprobe` puede no ser aceptado por:

- `setparams`
- `colorspace`
- `libx265`
- `hevc_videotoolbox`
- `h264_videotoolbox`
- QSV
- VAAPI

Debe existir una capa de capacidades por:

```text
FFmpeg build
filter
encoder
platform
```

Ejemplo:

```ts
export type EncoderColorCapability = {
  encoder: string;
  acceptedPrimaries: string[];
  acceptedTransfers: string[];
  acceptedMatrices: string[];
  acceptedRanges: string[];
  supportsFrameMetadataPropagation: boolean;
  supportsExplicitVui: boolean;
  requiresNumericEnums: boolean;
};
```

Antes de generar un comando, MVForge debe validar el valor mediante probing local:

```bash
ffmpeg -hide_banner -h filter=colorspace
ffmpeg -hide_banner -h filter=setparams
ffmpeg -hide_banner -h encoder=hevc_videotoolbox
ffmpeg -hide_banner -h encoder=libx265
```

Los aliases deben normalizarse internamente a un modelo canónico y traducirse al formato aceptado por cada componente.

---

# Comandos reproducibles

MVForge debe guardar para cada ejecución del LAB:

- comando de referencia
- comando de conversión
- versión de FFmpeg
- plataforma
- encoder
- filtros aplicados
- metadata de entrada
- metadata de salida
- warnings
- checksum del asset
- timestamps exactos del segmento

Ejemplo de reporte:

```json
{
  "labRunId": "lab-123",
  "sourceAssetId": "asset-456",
  "segment": {
    "start": "00:10:00",
    "durationSeconds": 10
  },
  "sourceCharacteristics": {},
  "previewNormalization": {},
  "referenceCommand": "ffmpeg ...",
  "profileCommand": "ffmpeg ...",
  "referenceCharacteristics": {},
  "conversionCharacteristics": {},
  "fidelityReport": {},
  "ffmpegVersion": "...",
  "platform": "darwin/amd64"
}
```

---

# Métricas futuras

Las métricas deben calcularse después de decodificar ambas ramas a un mismo dominio canónico.

Posibles métricas:

- PSNR
- SSIM
- VMAF
- diferencia media de luma
- diferencia media de chroma
- histogram distance
- RGB absolute difference
- Delta E aproximado

Estas métricas no deben reemplazar la inspección visual.

El resultado debe explicar:

```text
Metadata fidelity
Visual fidelity
Compression fidelity
Structural fidelity
```

No combinar todo en un único score opaco.

---

# UI propuesta

## Encabezado del LAB

Mostrar:

```text
Fuente: MPEG-2 720x480, 4:3, BFF, BT.470 legacy
Preview: H.264 MP4, progressive, BT.709 canonical
Perfil: HEVC VideoToolbox, progressive, BT.709
```

## Badges

- Source metadata preserved
- Preview normalized
- Color converted
- Deinterlace applied
- SAR preserved
- Browser reference generated
- Output differs from source

## Tabs

1. Visual Comparison
2. Frame Fidelity
3. Characteristics
4. FFmpeg Commands
5. Validation

## Warning importante

Si la referencia del navegador fue normalizada:

```text
La referencia fue convertida a un perfil canónico para permitir reproducción consistente en navegador. Consulta “Characteristics” para ver las propiedades originales de la fuente.
```

---

# Reglas funcionales

1. Nunca llamar “Original” a una transcodificación sin aclaración.
2. Ambas ramas deben usar exactamente el mismo segmento temporal.
3. Ambas ramas deben compartir el mismo pipeline previo al encoder cuando el objetivo sea comparar encoders.
4. No cambiar metadata sin registrar la operación.
5. No usar `setparams` para simular una conversión real.
6. La normalización del preview no debe cambiar automáticamente la política de salida final.
7. Preservar SAR/DAR explícitamente.
8. Registrar field order declarado y resultado progresivo.
9. Validar la metadata final con `ffprobe`.
10. Mostrar cambios intencionales y no intencionales por separado.
11. Generar frames PNG para una comparación de color más confiable.
12. Guardar comandos y reportes para reproducibilidad.

---

# Criterios de aceptación

La fase se considera completa cuando:

- La UI deja de presentar la referencia H.264 como original puro.
- Fuente, referencia y conversión tienen metadata visible por separado.
- El backend genera un canonical preview pipeline compartido.
- El LAB puede normalizar de forma real fuentes legacy a BT.709.
- La política de preview y la política de output final son independientes.
- Se compara automáticamente color, rango, pixel format, field order, SAR y DAR.
- Los cambios inesperados generan warning.
- Existe un modo de comparación PNG frame-by-frame.
- Cada LAB run guarda comandos, metadata y resultados.
- El sistema valida aliases y capacidades del FFmpeg local antes de generar argumentos.

---

# Prioridad de implementación

## Fase 1

- Renombrar “Original” a “Referencia de la fuente”.
- Mostrar que fue normalizada para navegador.
- Guardar metadata de fuente, referencia y conversión.
- Comparar campos principales con `ffprobe`.
- Separar preview policy de final output policy.

## Fase 2

- Implementar pipeline canónico compartido.
- Normalización real BT.470/BT.601 a BT.709.
- Preservación explícita de SAR/DAR y field order.
- Capability probing por encoder y filtro.

## Fase 3

- Frame Fidelity con PNG.
- Diferencia visual e histogramas.
- Métricas PSNR, SSIM y VMAF en dominio común.
- Reporte de fidelidad reproducible.

---

# Resultado esperado

MVForge LAB debe permitir responder con confianza:

```text
¿La diferencia visual viene de la fuente, de la referencia del navegador, del pipeline de filtros, del encoder, del contenedor o de la reproducción?
```

El LAB no debe limitarse a mostrar dos videos. Debe convertirse en una herramienta de diagnóstico de fidelidad que explique qué características se preservaron, cuáles se normalizaron, cuáles se cambiaron deliberadamente y cuáles cambiaron de forma inesperada.

---

# Estado operativo y tareas pendientes

Actualizado: 2026-07-29.

## Estado operativo

El LAB ya puede utilizarse para pruebas de fidelidad A/B:

- La referencia de la fuente se identifica como una transcodificación temporal y no como el stream original.
- Fuente, referencia compatible y resultado del perfil se validan por separado con `ffprobe`.
- Ambas ramas pueden usar un dominio de preview BT.709 compartido mediante conversión matemática real.
- La normalización del preview es independiente del perfil de salida final.
- Se preserva y valida SAR/DAR.
- Frame Fidelity genera PNG mediante FFmpeg y permite comparación before/after, side-by-side y blink.
- PSNR y SSIM se calculan únicamente cuando la geometría de ambos frames es equivalente.
- Las diferencias de metadata inesperadas se muestran como warnings.

Estas capacidades permiten operar el LAB. Las tareas siguientes mejoran reproducibilidad, diagnóstico avanzado y cobertura, pero no bloquean las pruebas actuales.

## Backlog de fidelidad

### P1 — Reportes reproducibles

- [ ] Guardar cada ejecución del LAB con un identificador estable.
- [ ] Guardar fuente, checksum, inicio y duración exactos.
- [ ] Guardar los comandos reales ejecutados para referencia, perfil, frames y métricas.
- [ ] Guardar versión de FFmpeg, plataforma, encoder y filtros efectivos.
- [ ] Guardar metadata de fuente, referencia y resultado.
- [ ] Guardar política de normalización, warnings, SSIM y PSNR.
- [ ] Agregar historial de ejecuciones y opción para repetir una prueba.

### P1 — Política de color final

- [ ] Incorporar `Preserve source`, `Convert final output to BT.709` y `Custom` en perfiles.
- [ ] Mantener esta política separada de la normalización temporal del LAB.
- [ ] Validar la salida final con `ffprobe` y marcar cambios intencionales o inesperados.
- [ ] Permitir configuración custom de matrix, primaries, transfer, range, pixel format y chroma location.

### P1 — Pipeline canónico estructural

- [ ] Definir cuándo A y B deben compartir deinterlace, IVTC y corrección de field order.
- [ ] Definir un modo donde esos filtros sean deliberadamente la variable evaluada.
- [ ] Registrar field order declarado, detectado y efectivo.
- [ ] Evitar aplicar dos veces deinterlace o IVTC al combinar normalización y perfil.

### P2 — Capability probing

- [ ] Validar capacidades por build de FFmpeg, filtro, encoder, plataforma y worker.
- [ ] Validar los aliases aceptados por `colorspace`, `setparams`, QSV, VAAPI y VideoToolbox.
- [ ] Mostrar qué worker puede reproducir exactamente el comando del LAB.
- [ ] Registrar fallbacks sin ocultar que cambió el encoder o filtro solicitado.

### P2 — Comparación visual avanzada

- [ ] Agregar imagen de diferencia absoluta.
- [ ] Agregar histogramas de luma y chroma.
- [ ] Agregar zoom y desplazamiento sincronizados.
- [ ] Permitir seleccionar varios frames del segmento.
- [ ] Agregar distancia de histogramas y diferencias medias de luma/chroma.
- [ ] Agregar VMAF cuando el build de FFmpeg lo soporte.
- [ ] Mantener metadata, fidelidad visual, compresión y estructura como resultados separados; no crear un score único opaco.

### P2 — Validación integral

- [ ] Probar fixtures de DVD BT.470/BT.601.
- [ ] Probar fuentes anamórficas.
- [ ] Probar material interlaced TFF y BFF.
- [ ] Probar telecine y contenido mixed.
- [ ] Probar HDR y 10-bit.
- [ ] Probar QSV en el NAS y VideoToolbox en macOS.
- [ ] Añadir pruebas E2E del LAB con assets pequeños y redistribuibles.
