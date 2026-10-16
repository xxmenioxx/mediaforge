# Traducción de subtítulos

Esta guía describe la traducción explícita de subtítulos desde **Asset Info**
con el proveedor Ollama configurado en MVForge.

La traducción V1 es manual y opcional. No se ejecuta automáticamente desde
**Queue**, **Track Profiles**, **Profile Lab** ni **Workers**.

## Fuentes compatibles

La fuente siempre es el stream o sidecar exacto que el usuario selecciona.
MVForge no sustituye esa selección por otro subtítulo del mismo idioma.

Las fuentes de texto compatibles son:

- subtítulos de texto embebidos;
- sidecars externos SRT;
- sidecars externos ASS;
- streams SSA embebidos mediante la ruta canónica de texto compatible.

Las fuentes bitmap compatibles son:

- PGS;
- VobSub.

Para PGS y VobSub, MVForge reutiliza su flujo existente de OCR y corrección:

```text
PGS/VobSub
  → OCR y corrección canónicos de MVForge
  → cues de texto canónicos
  → traducción
```

La traducción no agrega otro motor de OCR ni requiere ejecutar OCR manualmente
antes.

## Idioma de origen

Las opciones disponibles dependen de las capacidades que anuncie el backend
para la fuente y el modelo configurado. Se puede seleccionar explícitamente
**Korean** (`ko`) o **French** (`fr`) y traducirlos a los destinos españoles
indicados abajo cuando Ollama tiene seleccionado un ID oficial de
TranslateGemma: `translategemma:4b`, `translategemma:12b`,
`translategemma:27b` o `translategemma:latest`. El catálogo oficial de Ollama
describe TranslateGemma como una colección de traducción para 55 idiomas e
incluye coreano, francés y español en su lista de idiomas.

Para otros modelos, la capacidad no anuncia `ko` ni `fr`. Si se pide uno de
esos idiomas explícitamente, o **Auto** lo resuelve a Korean/French, MVForge
rechaza la operación con un error de capacidad que indica que el modelo no
admite ese idioma de origen. Cambiar a otro idioma no debe usarse para eludir
ese rechazo.

**Auto** usa metadatos de idioma confiables cuando están disponibles. Si los
metadatos faltan, son contradictorios o no bastan para una resolución segura,
MVForge falla de forma cerrada. Para Korean también puede resolver evidencia
clara del alfabeto Hangul en los cues. Para French se requieren metadatos de
idioma inequívocos; no se infiere sólo por texto latino. La evidencia mixta o
inconclusa detiene la operación antes de contactar al proveedor.

English y Japanese siguen disponibles como idiomas de origen con los modelos
que anuncien esas capacidades.

## Idioma de destino

La V1 ofrece dos locales distintos:

| Opción | Locale |
| --- | --- |
| Spanish (Latin America) | `es-419` |
| Spanish (Spain) | `es-ES` |

`es-419` y `es-ES` producen identidades y artefactos de traducción distintos;
no se convierten en el locale genérico `es`.

## Configurar Ollama

Abre:

```text
Settings → General → Pipeline → Subtitle Translation
```

La configuración V1 contiene:

- **Provider**: `Ollama`;
- **Ollama endpoint**;
- **Ollama model**.

Ollama puede ejecutarse en el mismo NAS, en otro contenedor o en otro equipo
de la red. Por ejemplo:

```text
http://192.168.1.100:11434
```

Esta dirección es sólo un ejemplo: MVForge no la usa como valor por defecto ni
infiere automáticamente `localhost` o `127.0.0.1`. No se configuran
credenciales en este formulario.

El nombre del modelo debe corresponder a un modelo instalado en el servidor
Ollama configurado. Por ejemplo, `qwen2.5:7b` es un valor ilustrativo, no una
recomendación de modelo ni de rendimiento.

### Guardar la configuración

1. Selecciona **Ollama**.
2. Introduce el endpoint accesible desde el backend de MVForge.
3. Introduce el nombre del modelo.
4. Pulsa **Save translation configuration**.

Cambiar el proveedor, endpoint o modelo invalida la readiness anterior y
deshabilita la traducción hasta comprobar y habilitar de nuevo la configuración.
Este comportamiento evita usar una configuración que ya no fue validada.

### Test Connection

Pulsa **Test Connection** después de guardar la configuración. El backend:

- comprueba que el proveedor configurado sea accesible;
- valida la configuración;
- comprueba que el modelo configurado esté disponible en Ollama;
- registra un estado de readiness y un diagnóstico acotado.

Un resultado exitoso deja la configuración en estado **Ready**, pero no activa
la traducción automáticamente:

```text
Test Connection exitoso ≠ traducción habilitada
```

El timestamp de la última comprobación es informativo; no representa un
monitor continuo y MVForge no ejecuta un health check antes de cada traducción.

### Enable y Disable

Después de un **Test Connection** exitoso, pulsa **Enable translation**. Esta
acción vuelve a validar en el backend la configuración persistida actual y sólo
habilita la traducción si esa comprobación es exitosa.

Para detener el uso del proveedor, pulsa **Disable translation**. Deshabilitar
no requiere que Ollama esté disponible y no borra el endpoint ni el modelo;
la configuración queda disponible para reutilizarla más adelante.

### Estados de readiness

| Estado | Significado |
| --- | --- |
| **Not configured** | Falta una configuración completa de proveedor, endpoint o modelo. |
| **Untested** | La configuración cambió o todavía no pasó **Test Connection**. |
| **Ready** | La configuración persistida pasó la validación de readiness. |
| **Unavailable** | No se pudo utilizar el endpoint del proveedor. |
| **Model not found** | El modelo configurado no está disponible en Ollama. |
| **Invalid configuration** | La configuración no es válida estructuralmente. |
| **Disabled** | La traducción está deshabilitada de forma intencional. |

## Modelos Ollama y dimensionamiento

Esta sección separa los requisitos de MVForge de las recomendaciones para
elegir un modelo. Las recomendaciones son puntos de partida, no
certificaciones de compatibilidad ni garantías de calidad, velocidad o
consumo.

### Requisitos de MVForge

El modelo configurado debe existir en el servidor Ollama, poder usarse mediante
`/api/chat`, seguir la instrucción acotada de MVForge y devolver JSON
estructurado. Debe conservar exactamente los IDs de los cues, devolver un
texto por cada cue solicitado, traducir el idioma de origen anunciado al
locale español solicitado y completar la solicitud dentro de los límites de
request y timeout del proveedor. Ollama documenta TranslateGemma como modelo de
traducción para 55 idiomas, incluidos `ko` (Korean), `fr` (French) y `es`
(Spanish); esto no certifica la calidad de traducción ni el cumplimiento del
contrato estructurado de MVForge. Evalúa muestras antes de confiar en los
resultados.

MVForge envía una solicitud equivalente a:

```json
{
  "model": "...",
  "messages": [{
    "role": "user",
    "content": "<instrucción estructurada de traducción de MVForge>"
  }],
  "stream": false,
  "format": "json"
}
```

Que un modelo esté instalado o sea multilingüe no garantiza este contrato.
**Test Connection** comprueba alcance del proveedor y existencia del modelo;
no comprueba calidad, JSON correcto para todos los subtítulos ni rendimiento
aceptable en el NAS.

### Candidatos recomendados para empezar

La familia **Qwen 3.5** es un candidato actual razonable para evaluar por su
cobertura de instrucciones multilingües, que incluye inglés, japonés y
español. Debe probarse con el contrato JSON e IDs de cues de MVForge.

| Candidato | Tamaño aproximado del paquete Ollama | Uso sugerido | Estado |
| --- | ---: | --- | --- |
| `qwen3.5:4b` | ≈ 3.4 GB | Primer candidato para NAS compartido o limitado | Recomendación inicial |
| `qwen3.5:9b` | ≈ 6.6 GB | Host con más margen de memoria | Candidato recomendado para evaluar |
| `qwen3.5:27b` | ≈ 17 GB | Host dedicado o con mucha memoria | Opcional; no es la opción NAS predeterminada |
| `translategemma:4b` | ≈ 3.3 GB | Experimento especializado | No validado aún con el contrato estructurado de MVForge |

Los tamaños son aproximaciones de paquetes/modelos Ollama, no RAM o VRAM de
ejecución. Qwen 3.5 no está certificado por MVForge y no se afirma que sea el
mejor, más rápido o perfecto.

Para un NAS con recursos limitados y uso principalmente CPU, `qwen3.5:4b` es
un candidato inicial conservador. `qwen3.5:9b` merece evaluación cuando el host
puede dedicar más memoria a Ollama sin competir demasiado con los servicios de
media; su tamaño no demuestra por sí solo mayor calidad. `qwen3.5:27b` no es la
recomendación predeterminada para un NAS compartido: su paquete ronda 17 GB y
la ejecución requiere memoria adicional.

Las instalaciones existentes de Qwen 3 también pueden evaluarse. Como
referencias, `qwen3:4b` ronda 2.5 GB y `qwen3:8b` ronda 5.2 GB. Son alternativas
para usuarios que ya ejecutan Qwen 3, no un catálogo completo.

**TranslateGemma** es una familia especializada de traducción multilingüe. Sus
variantes `translategemma:4b`, `:12b` y `:27b` rondan 3.3 GB, 8.1 GB y 17 GB.
Su contrato documentado suele orientarse a devolver texto traducido
directamente, mientras MVForge requiere lotes JSON con IDs exactos y resultado
estructurado. Es un candidato especializado prometedor, pero no el modelo
predeterminado y todavía no está validado con MVForge. Esto no afirma una
incompatibilidad absoluta; una prueba futura podría cambiar la recomendación.

Los nombres, tags y tamaños cambian entre versiones de Ollama. Verifica el
catálogo actual antes de instalar un modelo. Las cifras y capacidades citadas
se basan en el catálogo oficial de Ollama y documentación del editor, no en
benchmarks de la comunidad.

### Memoria y carga del NAS

El tamaño del paquete es sólo una parte de la memoria de ejecución. También
influyen la arquitectura, cuantización, context/KV cache, offload a CPU/GPU,
overhead de Ollama, solicitudes concurrentes, modelos cargados, MVForge,
Docker, servicios multimedia y la caché del sistema. Un paquete de 3.4 GB no
significa que requiera exactamente 4 GB de RAM.

Esta es orientación no vinculante:

- **Alrededor de 8 GB de RAM total**: no es un objetivo recomendado para
  compartir MVForge y Ollama; puede quedar poco margen para Jellyfin, bases de
  datos, Docker y otros servicios. No se clasifica como soportado o no
  soportado de forma absoluta.
- **Alrededor de 16 GB**: 4B es un punto de partida más sensato, pero no se
  garantiza que 4B siempre quepa ni se afirma que 9B no pueda funcionar.
- **Alrededor de 24–32 GB**: rango más práctico para evaluar 4B y posiblemente
  9B, según la carga restante. Observa el consumo antes de usar traducción
  durante otras cargas pesadas.
- **Hosts mayores o dedicados**: 27B se maneja mejor en un host Ollama
  dedicado o con margen sustancial; una cantidad concreta de RAM no garantiza
  que un modelo cargue.

Ollama puede ejecutarse en el NAS o en otro host de la LAN. Separarlo es una
estrategia válida cuando el NAS también ejecuta Jellyfin/Plex/Emby,
transcodificación, Immich, automatización de descargas u otros contenedores.
MVForge usa un endpoint HTTP configurable.

MVForge no requiere GPU. CPU-only está permitido, pero no significa que todo
modelo sea suficientemente rápido: la latencia depende de CPU, modelo,
cuantización, lote y carga del host. MVForge no selecciona GPU, administra VRAM,
controla el offload de Ollama ni programa traducciones según GPU.

MVForge usa lotes de cues acotados, no una solicitud por cue. Esto limita el
número de solicitudes, pero no elimina el coste de memoria del modelo. Empieza
con una sola carga de traducción, observa CPU y memoria y decide después si la
concurrencia es razonable. Una ventana de contexto enorme tampoco es un
requisito: MVForge envía lotes acotados.

### Evaluar un candidato

Prueba muestras representativas de cada idioma anunciado —incluidos Korean o
French si están disponibles— hacia `es-419` y `es-ES`, cues cortos y
multilínea, nombres propios, diálogo informal, subtítulos
forced/de signos y ASS con estructura protegida. Evalúa significado, español
natural y regional, nombres/terminología, ausencia de texto de origen o
comentarios añadidos y cumplimiento del JSON con IDs exactos. No se define una
puntuación numérica ficticia ni un tiempo esperado.

**Test Connection** sólo demuestra endpoint alcanzable, modelo existente y
readiness del proveedor. Ejecuta una traducción real para comprobar calidad,
formato y carga del host.

## Iniciar una traducción

1. Abre **Asset Info** para el asset.
2. Busca la entrada exacta del subtítulo embebido o el sidecar externo.
3. Pulsa **Translate...** en esa entrada.
4. Selecciona **Auto** o un idioma de origen ofrecido por el backend para esa
   fuente y proveedor. Puede incluir **Korean** o **French** cuando estén
   anunciados como capacidades disponibles.
5. Selecciona `es-419` o `es-ES`.
6. Elige el formato sólo si el backend ofrece más de una representación.
7. Inicia la traducción.

La entrada sobre la que pulsaste es la identidad de origen. MVForge conserva el
stream embebido o la ruta exacta del sidecar y no busca otro subtítulo por
idioma, nombre de archivo o similitud con un sidecar vecino.

### Representación de salida

Para fuentes de texto se conserva la representación compatible:

```text
SRT → SRT
ASS → ASS
SSA → SSA
```

Para fuentes bitmap, el backend puede autorizar:

```text
SRT
ASS
Both
```

La UI no inventa ni decide esa elegibilidad; el backend entrega el conjunto
permitido. **Both** realiza una traducción canónica y luego serializa los
mismos cues a SRT y ASS, no dos traducciones independientes.

### ASS y SSA

La traducción modifica el texto de diálogo elegible, mientras MVForge conserva
los elementos estructurales del subtítulo, incluidos:

- tiempos;
- estilos;
- posicionamiento;
- tags de override;
- layout y metadatos estructurales.

Si la respuesta del proveedor altera una estructura protegida, MVForge rechaza
la respuesta en vez de publicar un ASS/SSA inválido.

### Fuentes bitmap

El flujo de PGS y VobSub es:

```text
PGS/VobSub
  → OCR canónico
  → corrección y revisión de calidad
  → cues de texto canónicos
  → traducción
```

El manejo OCR de japonés, incluidos `jpn` y `jpn_vert`, ocurre antes de la
traducción. En fuentes bitmap, Korean se procesa con el idioma OCR `kor`; los
alias de French se normalizan a `fra`. El backend comprueba que el runtime
Tesseract tenga el traineddata necesario. Si falta, el error indica que se
instale el paquete traineddata correspondiente en el runtime OCR y se reintente.
La extracción también requiere las herramientas OCR del backend. No se afirma
que el runtime se haya ejercitado en todos los despliegues. Si OCR no puede
completar la extracción, MVForge detiene la traducción y no envía imágenes
bitmap al proveedor. El proveedor recibe cues de texto canónicos, no imágenes
ni una ruta OCR alternativa.

## Progreso de la operación

Asset Info puede mostrar estas fases, según el tipo de fuente:

```text
Preparing
OCR
Translating
Validating
Publishing
Cleanup
```

Una fuente SRT normalmente no muestra una fase OCR. La fase de traducción
puede mostrar lotes procesados; no se deben interpretar los porcentajes como
una duración estimada.

## Cancelación y reintento

Mientras la operación sea cancelable, Asset Info muestra **Cancel**. La
cancelación se propaga al trabajo activo de OCR, proveedor o validación y
limpia los temporales propios de la operación.

Una cancelación se muestra separada de un fallo del proveedor. El archivo de
media, el subtítulo de origen y cualquier sidecar traducido anterior se
conservan.

Una operación fallida o cancelada puede mostrar **Retry** si el backend la
marca como reintentable. El reintento:

- crea una identidad de operación nueva;
- deja sin cambios la operación terminal anterior;
- vuelve a resolver y comprobar la fuente exacta;
- usa la configuración actual persistida de proveedor y modelo.

Retry no reanuda la operación anterior en el mismo registro.

### Fuente modificada durante la operación

Si el subtítulo seleccionado o la fuente multimedia cambia mientras la
traducción está en curso, MVForge puede rechazar la publicación por **stale
source**. Esto evita publicar una traducción calculada a partir de una revisión
obsoleta. En ese caso no se confirma un artefacto nuevo; actualiza Asset Info y
reintenta sobre la fuente actual.

## Artefactos traducidos

La traducción crea un sidecar separado y nunca sobrescribe el subtítulo de
origen. Por ejemplo:

```text
Movie.eng.forced.ass
→ Movie.es-419.eng.forced.ass
```

`es-419` identifica el locale de destino. El resto del nombre conserva el
contexto de idioma de origen y la disposición existente. Con un rol SDH y una
disposición forced, el nombre puede ser:

```text
Movie.es-419.eng.forced.sdh.ass
```

El formato exacto depende de los metadatos de la fuente, pero nunca debe
repetir el locale de destino.

El artefacto traducido conserva, cuando aplica, metadata de origen como:

- default;
- forced;
- rol semántico;
- título.

Su idioma/locale de traducción es `es-419` o `es-ES`. Para **Both**, la
publicación es un conjunto atómico: si no se puede publicar completo, MVForge
evita dejar sólo uno de los nuevos miembros. El media original, el stream
embebido, el sidecar SRT/ASS/SSA de origen y las fuentes PGS/VobSub permanecen
sin cambios.

## Si Ollama deja de estar disponible

Si Ollama falla después de haber sido configurado y habilitado:

- MVForge no cambia silenciosamente de proveedor;
- MVForge no sustituye el endpoint por `localhost`;
- la operación informa un fallo del proveedor;
- MVForge no inicia un bucle de health checks antes de cada traducción.

Vuelve a **Settings** y ejecuta **Test Connection** para diagnosticar el
estado actual del proveedor.

La capacidad disponible depende del modelo y su identidad. KO/FR sólo se
anuncian para los IDs oficiales de Ollama TranslateGemma indicados arriba; en
otros modelos, un idioma explícito o resuelto por Auto se rechaza claramente
antes de traducir. Si el proveedor no cumple el contrato de respuesta, la
operación informa el fallo; MVForge no cambia de idioma, modelo ni proveedor
silenciosamente.

## Troubleshooting

| Síntoma | Significado o acción |
| --- | --- |
| Not configured | Guarda proveedor, endpoint y modelo. |
| Untested | Ejecuta **Test Connection**. |
| Ready pero `Enabled: No` | Pulsa **Enable translation**. |
| Unavailable | Revisa el host y la red de Ollama; luego repite **Test Connection**. |
| Model not found | Confirma que el modelo configurado exista en Ollama. |
| Auto no determina el idioma | Selecciona explícitamente un idioma ofrecido para esa fuente y proveedor. |
| KO/FR falla por capacidad | Usa un ID oficial de Ollama TranslateGemma (`translategemma:4b`, `:12b`, `:27b` o `:latest`) para esos idiomas de origen. |
| Auto no resuelve French | Asegura que el subtítulo tenga metadatos de idioma inequívocos; French no se infiere sólo por texto latino. |
| Korean bitmap OCR indica traineddata ausente | Instala el traineddata `kor` en el runtime Tesseract del backend y reintenta. |
| French bitmap OCR indica traineddata ausente | Instala el traineddata `fra` en el runtime Tesseract del backend y reintenta. |
| Stale source | Actualiza Asset Info y reintenta sobre la fuente actual. |
| Cancelled | Pulsa **Retry** si la operación lo ofrece. |

## Datos enviados y privacidad

El texto de los cues se envía al proveedor Ollama configurado.

En la V1 actual, **Asset Info** no añade contexto multimedia opcional a la
traducción: el backend reporta ese contexto como `unavailable`/`none`. El
endpoint y el modelo del proveedor se persisten en MVForge.

La procedencia y la auditoría conservan metadata acotada de la operación, pero
no retienen intencionalmente prompts completos, respuestas completas del
proveedor ni credenciales. Esto no sustituye las políticas de privacidad o
seguridad del host Ollama configurado fuera de MVForge.

## Rendimiento

El contrato de MVForge admite Ollama ejecutándose sólo con CPU, pero los
recursos y el tiempo real dependen del modelo elegido y del host donde se
ejecute Ollama. Esta guía no prescribe modelos, tamaños ni requisitos de
hardware.

## Limitaciones V1

- Korean (`ko`) y French (`fr`) sólo se anuncian para Ollama
  TranslateGemma: `translategemma:4b`, `:12b`, `:27b` y `:latest`; explicitarlos
  o resolverlos por Auto con otro modelo falla por capacidad;
- English y Japanese siguen disponibles cuando el backend los anuncia para el
  modelo seleccionado;
- Auto exige metadatos inequívocos para French; Korean también puede resolverse
  desde evidencia clara de Hangul. Evidencia mixta o insuficiente falla de
  forma cerrada;
- Korean bitmap OCR usa `kor`; alias de French usan `fra`. El runtime valida
  traineddata y da una instrucción de instalación/reintento cuando falta;
- destinos: `es-419` y `es-ES`;
- proveedor implementado: Ollama;
- traducción explícita desde Asset Info;
- no hay traducción automática en Queue, Profiles, Track Profiles, Profile Lab
  ni Workers;
- no hay traducción genérica de idiomas arbitrarios.

## Referencias de implementación

- [Catálogo oficial de Ollama: TranslateGemma](https://ollama.com/library/translategemma)

- `backend/internal/handlers/translation_asset_info.go`
- `backend/internal/handlers/translation_readiness.go`
- `backend/internal/handlers/translation_ollama.go`
- `backend/internal/handlers/translation_bitmap_execution.go`
- `backend/internal/handlers/translation_publication.go`
- `frontend/src/components/SubtitleTranslationDialog.tsx`
- `frontend/src/pages/SettingsPage.tsx`


## Media context (planned enhancement)

The current V1 does not add movie/series metadata to translation prompts.
Asset Info therefore reports media context as unavailable/none.

Planned follow-up work (#107 and #108) treats media context as an **optional
quality enhancement**, not as a requirement to translate.

This distinction matters for RAW-first workflows. A RAW asset may not yet:

- exist in Jellyfin/Plex;
- have an adjacent NFO;
- have been identified by Radarr/Sonarr;
- have a confirmed TMDB/provider identity.

That is a normal state. Translation must still work:

```text
RAW asset
  → no media metadata
  → neighboring subtitle cues only
  → Translate remains available
```

Users who keep assets directly in a consumable Library may have richer metadata
earlier, but MVForge does not require that workflow.

### Local NFO

The planned canonical metadata resolver will treat local NFO as a first-class
source in addition to media-server/provider APIs.

Jellyfin-compatible metadata commonly uses:

```text
movie.nfo
tvshow.nfo
season.nfo
<episode filename>.nfo
```

These files may contain title/year/plot, show/season/episode identity, actors
and external provider IDs. If another application already created a reliable
NFO, MVForge can consume it without requiring that application to remain
available.

### When MVForge is not sure about the match

MVForge should not silently attach a similar-looking movie or episode merely
because the title matches.

When metadata identity is not strong enough, the UI should stay simple:

```text
Media context (optional)

3 possible matches
[ Choose ]   [ Translate without context ]
```

The candidate picker should show only decision-relevant fields such as title,
year, media type, series/season/episode and provider/source evidence.

The user can leave metadata unset and translate normally. A confirmed selection
is saved through the canonical metadata layer so it can be reused by Asset
Info, Advisor and future operations.

### TMDB setup

Direct TMDB lookup is optional. When the TMDB adapter is implemented, the user
must create/configure TMDB application credentials in MVForge Settings.
Supported authentication should follow the adapter design and TMDB's
application authentication model, using either a v3 API key or an API Read
Access Token/Bearer token.

TMDB credentials must never be written to reports/logs, embedded in profile
snapshots or sent to the translation model.
