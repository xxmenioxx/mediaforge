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

Las opciones V1 son:

- **Auto**: usa metadatos confiables y evidencia acotada de los cues. No es
  detección genérica de cualquier idioma.
- **English**: trata la fuente seleccionada como inglés.
- **Japanese**: trata la fuente seleccionada como japonés.

Si **Auto** no puede distinguir inglés de japonés con suficiente confianza,
MVForge detiene la operación antes de contactar al proveedor. En ese caso,
selecciona explícitamente **English** o **Japanese**.

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

## Iniciar una traducción

1. Abre **Asset Info** para el asset.
2. Busca la entrada exacta del subtítulo embebido o el sidecar externo.
3. Pulsa **Translate...** en esa entrada.
4. Selecciona **Auto**, **English** o **Japanese**.
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
traducción. El proveedor recibe los cues canónicos resultantes, no imágenes ni
una ruta OCR alternativa.

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

## Troubleshooting

| Síntoma | Significado o acción |
| --- | --- |
| Not configured | Guarda proveedor, endpoint y modelo. |
| Untested | Ejecuta **Test Connection**. |
| Ready pero `Enabled: No` | Pulsa **Enable translation**. |
| Unavailable | Revisa el host y la red de Ollama; luego repite **Test Connection**. |
| Model not found | Confirma que el modelo configurado exista en Ollama. |
| Auto no determina el idioma | Selecciona **English** o **Japanese** explícitamente. |
| Stale source | Actualiza Asset Info y reintenta sobre la fuente actual. |
| Cancelled | Pulsa **Retry** si la operación lo ofrece. |

## Datos enviados y privacidad

El texto de los cues se envía al proveedor Ollama configurado. Cuando está
disponible, también puede enviarse contexto multimedia opcional y acotado para
ayudar a la traducción. El endpoint y el modelo del proveedor se persisten en
MVForge.

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

- idiomas de origen: English y Japanese, con selección **Auto**;
- destinos: `es-419` y `es-ES`;
- proveedor implementado: Ollama;
- traducción explícita desde Asset Info;
- no hay traducción automática en Queue, Profiles, Track Profiles, Profile Lab
  ni Workers;
- no hay traducción genérica de idiomas arbitrarios.

## Referencias de implementación

- `backend/internal/handlers/translation_asset_info.go`
- `backend/internal/handlers/translation_readiness.go`
- `backend/internal/handlers/translation_ollama.go`
- `backend/internal/handlers/translation_bitmap_execution.go`
- `backend/internal/handlers/translation_publication.go`
- `frontend/src/components/SubtitleTranslationDialog.tsx`
- `frontend/src/pages/SettingsPage.tsx`
