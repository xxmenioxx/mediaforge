# Recomendaciones de perfiles de MVForge

Esta guía documenta perfiles prácticos de video, audio y tracks para usar
MVForge en una biblioteca personal o HomeLab. Complementa la guía conceptual
[Perfiles de video, audio y tracks](profiles.md) con ejemplos operativos y
recomendaciones por tipo de contenido.

Los perfiles son contratos reproducibles. Cuando se crea un job, MVForge
guarda un snapshot de los perfiles seleccionados; cambiar el perfil original
después no debe modificar silenciosamente un job ya planificado.

## Cómo elegir perfiles

Empieza con el perfil menos destructivo que resuelva el problema real:

1. Analiza el asset primero.
2. Prefiere remux o conversión conservadora si la fuente ya es buena.
3. Prueba una muestra corta en Profile Lab antes de procesar una película o temporada completa.
4. Conserva audio original y subtítulos durante el piloto.
5. Ejecuta dry run y revisa el execution plan.
6. Convierte un archivo pequeño y descartable antes de lanzar un batch.

## Perfiles de video incluidos en v0.1.2

Una base limpia de MVForge v0.1.2 incluye perfiles conservadores MKV/x265
Main10. Son buenos puntos de partida para un NAS.

| Perfil | Recomendado para | Objetivo | Notas |
|---|---|---|---|
| `DVD Archive x265 Main10` | DVDs, películas SD, documentales antiguos | Reducir tamaño preservando calidad de archivo | Bueno para fuentes 480p/576p. Revisa desentrelazado y crop antes de batches. |
| `Anime DVD x265 Main10` | Anime SD, fansubs antiguos, animación cel | Preservar line art y evitar banding | Prueba escenas oscuras, gradientes y subtítulos estilizados. |
| `Series Balanced x265 Main10` | Series 720p/1080p, episodios generales | Balance entre tamaño, calidad y compatibilidad | Buen default para contenido episódico repetible después de validar una muestra. |

Estos perfiles apuntan a Matroska (`.mkv`) con HEVC/x265 Main10 y preservan
subtítulos y capítulos de forma conservadora.

## Ejemplos de perfiles de video

### Movies Archive x265 Main10

Úsalo para Blu-ray o fuentes de película de buena calidad cuando la eficiencia
de almacenamiento a largo plazo sea más importante que convertir rápido.

```text
Container: mkv
Codec family: hevc / x265
Pixel format: yuv420p10le
Quality mode: CRF
CRF range to test: 18-22
Preset: slow or medium
Audio policy: preserve original during pilot
Subtitles: preserve all
Chapters: preserve
Metadata: preserve
```

Uso recomendado:

- `movies/`
- `anime-movies/` si la fuente es de alta calidad o alto bitrate
- documentales con mucho detalle visual

Evítalo para:

- conversiones temporales rápidas
- clientes que no reproducen HEVC Main10
- fuentes que ya son pequeñas y visualmente aceptables

### Series Balanced x265 Main10

Úsalo para librerías de series donde importan la consistencia, el tamaño y la
calidad entre episodios.

```text
Container: mkv
Codec family: hevc / x265
Pixel format: yuv420p10le
Quality mode: CRF
CRF range to test: 20-24
Preset: medium
Audio policy: preserve original or convert secondary stereo copy
Subtitles: preserve forced and selected languages
Chapters: preserve when present
```

Uso recomendado:

- `series/`
- documentales episódicos
- temporadas largas donde el ahorro de almacenamiento se acumula

Evítalo para:

- fuentes raras sin revisión visual
- deportes o conciertos con mucho movimiento si no validaste muestras

### Anime Grain x265 Main10

Úsalo para anime con grano, gradientes, escenas oscuras o sensibilidad en líneas.

```text
Container: mkv
Codec family: hevc / x265
Pixel format: yuv420p10le
Quality mode: CRF
CRF range to test: 17-21
Preset: slow
Audio policy: preserve original language tracks
Subtitles: preserve all, especially forced/signs/songs
Chapters: preserve
```

Uso recomendado:

- `anime/`
- `anime-movies/`
- fuentes con riesgo de banding o subtítulos estilizados

Evítalo para:

- web rips muy comprimidos donde reconvertir agrega artefactos
- fuentes con subtítulos dudosos hasta validar un track profile

### Concerts Preserve First

Úsalo cuando la calidad de audio y la sincronía importan más que comprimir video.

```text
Container: mkv
Codec family: keep source video or conservative x265
Pixel format: preserve when possible
Quality mode: CRF only after sample review
Audio policy: preserve all original audio tracks
Subtitles: preserve all
Chapters: preserve
```

Uso recomendado:

- conciertos
- videos musicales con audio lossless o surround
- presentaciones donde el audio es el asset principal

Evítalo para:

- normalización automática sin comparación A/B
- denoise o restauración agresiva sin revisión manual

### Compatibility H.264

Úsalo cuando tus dispositivos no reproducen HEVC Main10 de forma confiable.

```text
Container: mp4 or mkv
Codec family: h264 / x264
Pixel format: yuv420p
Quality mode: CRF
CRF range to test: 18-23
Preset: medium
Audio policy: AAC stereo copy or preserve original in MKV
Subtitles: burn in only if the target device requires it
```

Uso recomendado:

- smart TVs antiguos
- copias móviles
- exports temporales de compatibilidad

Evítalo para:

- masters de archivo si HEVC está disponible
- anime sensible a banding sin validar muestras

## Perfiles de audio

Los perfiles de audio deben probarse con más cuidado que los de video. Una mala
normalización o reducción de ruido puede arruinar un archivo aunque las métricas
parezcan correctas.

| Tipo de perfil | Recomendado para | Objetivo | Riesgo |
|---|---|---|---|
| Preserve Original | Películas, conciertos, releases de alta calidad | Mantener audio intacto | Archivos más grandes, compatibilidad variable |
| Gentle Normalize | Series, documentales, contenido con diálogo | Suavizar diferencias de volumen | Puede reducir dinámica si se exagera |
| Dialogue Clarity | Documentales, clases, series antiguas | Mejorar inteligibilidad | Puede adelgazar música o efectos |
| Old Source Cleanup | VHS/DVD/TV captures, documentales antiguos | Reducir ruido o hum levemente | Artefactos de denoise si es agresivo |
| Mono / Dual Mono Cleanup | Fuentes mono antiguas o dual mono defectuoso | Corregir canales y balance | Una mala suposición puede dañar la imagen estéreo |
| Experimental Mono To Stereo | Pruebas descartables | Crear presentación más amplia | No es archival; requiere revisión manual |

## Ejemplos de perfiles de audio

### Preserve Original Audio

```text
Codec: copy
Normalize: off
Denoise: off
EQ: off
Keep original track: yes
```

Úsalo para:

- conciertos
- películas con pistas 5.1/7.1
- fuentes lossless o de alta calidad
- primer piloto de cualquier tipo de fuente nueva

### AAC Stereo Compatibility

```text
Codec: aac
Channels: stereo
Bitrate: 160k-256k
Normalize: optional gentle loudness
Keep original track: yes during pilot
```

Úsalo para:

- copias de compatibilidad
- reproducción en teléfono o tablet
- archivos con codecs de audio problemáticos para tus clientes

No reemplaces la pista original hasta validar reproducción y sincronía.

### Gentle Dialogue Normalize

```text
Codec: aac or opus
Loudness target: conservative broadcast-style target
True peak: leave headroom
Dialogue clarity: light
Keep original track: yes
```

Úsalo para:

- documentales
- clases o conferencias
- series antiguas con diálogo bajo
- TV con volumen desigual entre episodios

Evítalo para:

- conciertos
- películas donde la dinámica sea intencional

### Old Source Cleanup

```text
Codec: aac or opus
Denoise: light
Hum reduction: only when audible
EQ: small corrections only
Keep original track: yes
```

Úsalo para:

- capturas VHS
- extras de DVD
- grabaciones antiguas de TV
- documentales de archivo

Siempre compara A/B usando diálogo, música, silencio y escenas ruidosas antes de
aplicarlo a un batch.

## Track profiles

Los track profiles deciden qué streams de video, audio y subtítulos sobreviven.
Son tan importantes como los perfiles de codec.

### Spanish + Original Audio

```text
Audio rule: languages
Audio languages: spa, eng, jpn
Subtitle rule: forced-or-languages
Subtitle languages: spa
Validation: review
```

Úsalo para:

- anime con japonés y español/inglés
- películas donde importan subtítulos españoles o forced
- librerías con idiomas mezclados

### Preserve Everything For Pilot

```text
Audio rule: all
Subtitle rule: all
Validation: warn
```

Úsalo para:

- primera prueba de un tipo de fuente
- releases desconocidos
- archivos con metadata de idioma pobre

### Strict Archive

```text
Audio rule: all
Subtitle rule: all
Validation: block
```

Úsalo para:

- contenido raro
- conciertos con múltiples mezclas de audio
- archivos donde perder un stream es inaceptable

## Recomendaciones por librería

| Librería | Video profile | Audio profile | Track profile |
|---|---|---|---|
| Movies | Movies Archive x265 Main10 | Preserve Original Audio | Spanish + Original Audio o Strict Archive |
| Anime Movies | Anime Grain x265 Main10 | Preserve Original Audio | Spanish + Original Audio |
| Series | Series Balanced x265 Main10 | Gentle Dialogue Normalize o Preserve Original | Spanish + Original Audio |
| Anime | Anime Grain x265 Main10 | Preserve Original Audio | Spanish + Original Audio |
| Documentaries | Series Balanced x265 Main10 | Gentle Dialogue Normalize | Spanish + Original Audio |
| Concerts | Concerts Preserve First | Preserve Original Audio | Strict Archive |
| Music Videos | Compatibility H.264 o Concerts Preserve First | Preserve Original Audio | Preserve Everything For Pilot |

## Puntos iniciales de CRF

Estos valores son puntos de partida, no reglas universales. Valida con muestras.

| Fuente | x265 Main10 CRF inicial | Notas |
|---|---:|---|
| Blu-ray movie | 19-21 | Baja CRF para grano o escenas oscuras |
| Serie 1080p | 21-23 | Prueba un episodio antes de una temporada |
| DVD / SD live action | 19-22 | Revisa desentrelazado y blockiness |
| Anime SD/DVD | 17-20 | Vigila banding y line shimmer |
| Conciertos | 18-21 | Prioriza sincronía y preservación de audio |
| Compatibility H.264 | 18-23 | Más grande que HEVC a calidad similar |

## Reglas de seguridad

- Mantén dry run activo hasta validar paths, tracks y perfiles.
- Nunca apliques restauración a una librería completa sin muestras A/B.
- Conserva audio original en pilotos y contenido importante.
- Usa validación `review` o `block` cuando subtítulos o idiomas importan.
- Duplica un perfil estable antes de ajustarlo para una fuente especial.
- No elimines perfiles viejos mientras existan jobs que los referencien.
- Mantén reportes y backups de SQLite en el directorio de configuración.
