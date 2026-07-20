# Diagnóstico y troubleshooting

Esta guía reúne procedimientos seguros para investigar MediaForge en una
instalación local o NAS. Conserva primero la evidencia: no borres SQLite,
workspaces, reports ni reservas mientras investigas.

## Fuentes de diagnóstico

Revisa en este orden:

1. **Queue > Details**: execution plan, waiting state y razones.
2. **Workers**: worker online, capacidad y claim activo.
3. **Dashboard > Runtime & Host**: RAM, discos y encoders.
4. **Logs**: `backend.log`, `scheduler.log`, `workers.log` y `job-N.log`.
5. **Scheduler Recovery**: reservas y ejecuciones interrumpidas.

En Docker:

```sh
docker logs --tail 200 mediaforge
docker logs -f mediaforge
```

El archivo persistente se guarda dentro del contenedor en:

```text
/media/reports/logs/backend.log
```

Cada request incluye `X-Request-ID`. Usa ese valor para buscar la petición, el
status HTTP y cualquier pánico o error relacionado en `backend.log`.

## `database table is locked`

Este mensaje indica contención de escritura en SQLite. El claim de MediaForge
serializa la comprobación de capacidad, la reserva y la transición del job. La
conexión de producción usa WAL, `busy_timeout=5000` y un único writer.

### Comprobaciones

1. Confirma que sólo exista un contenedor backend:

   ```sh
   docker ps --filter name=mediaforge
   ```

2. Confirma que ninguna instalación anterior continúe usando el mismo
   `mediaforge.db`.
3. Revisa si el contenedor fue reiniciado durante un claim.
4. Abre **Scheduler Recovery** y ejecuta la reconciliación antes de reencolar.
5. Conserva `backend.log`, `scheduler.log` y `workers.log` para el reporte.

No ejecutes dos réplicas backend contra la misma SQLite. El bloqueo del claim es
por proceso; SQLite no sustituye una coordinación distribuida entre
contenedores.

Si el error persiste con una sola instancia, detén el backend y crea un backup
consistente antes de inspeccionar SQLite. No elimines manualmente filas de
`scheduler_reservations` con el backend activo.

## `record not found` en settings opcionales

GORM puede imprimir `record not found` al consultar settings que todavía no
existen, por ejemplo `assetMetadataOverrides`. Si la aplicación continúa y la
petición no termina en `5xx`, es una ausencia opcional y no la causa del fallo.

Investígalo como error solamente cuando esté acompañado por:

- respuesta HTTP `500`;
- evento `error` en `backend.log`;
- job en `failed`;
- setting requerido ausente después de Seed/Migrate.

## Job queued que el worker no reclama

Un job `queued` no siempre es ejecutable. El worker sólo reclama planes en
estado `ready`. Revisa el waiting state:

| Estado | Diagnóstico principal |
|---|---|
| `WAITING_HDD_SPACE` | Espacio del destination path real de la librería |
| `WAITING_SSD_SPACE` | Workspace/staging y reserva mínima |
| `WAITING_ENCODER` | Encoder permitido frente al runtime snapshot |
| `WAITING_SCHEDULE_WINDOW` | Horario y zona horaria |
| `WAITING_WORKER` | Heartbeat, capacidad y encoder del worker |

El snapshot AS-IS aparece como `Unknown` hasta que el worker inicia la ejecución.
Si el plan todavía espera recursos, ese estado es esperado y no indica que
FFprobe haya fallado.

## Storage de library muestra el disco incorrecto

En Docker, `/media/library` puede ser sólo un directorio padre mientras cada
librería está montada individualmente, por ejemplo `/media/library/movies`.
MediaForge consulta los `DestinationPath` registrados y utiliza de forma
conservadora el menor espacio disponible.

Después de corregir mounts o librerías:

1. Pulsa **Refresh host**.
2. Confirma que library corresponda al volumen de destino.
3. Espera la reevaluación del planner.
4. Revisa `scheduler.log` si el plan conserva `WAITING_HDD_SPACE`.

El tipo físico puede aparecer como `unknown` dentro de un contenedor; la ruta y
la capacidad disponible son los valores que gobiernan la decisión de espacio.

## `Error creating a MFX session: -9` con `hevc_qsv`

FFmpeg puede listar `hevc_qsv` aunque el contenedor no tenga acceso a Intel
Quick Sync. MediaForge prueba una codificación HEVC Main10 real antes de marcar
el encoder como usable. Si la prueba falla, el runtime snapshot debe mostrar
QSV como no usable y el scheduler puede seleccionar `libx265` cuando el perfil
lo permita.

Para utilizar QSV en Linux, comprueba primero el host:

```sh
ls -l /dev/dri
```

Después expón el dispositivo al backend:

```yaml
services:
  backend:
    devices:
      - /dev/dri:/dev/dri
```

La imagen `linux/amd64` incluye el Intel Media Driver (`iHD`) y el runtime Intel
Media SDK de Alpine. Esos paquetes sólo se instalan en x86_64 para conservar el
build multi-arquitectura; ARM64 no anuncia QSV.

Recrea el contenedor, pulsa **Refresh host** y confirma que `hevc_qsv` aparezca
como usable. Si continúa fallando, utiliza temporalmente `libx265`; no fuerces
QSV sólo porque aparezca en `ffmpeg -encoders`.

La línea `ac3 -> aac` no indica este fallo: demuestra que la pista AAC empezó a
codificarse. `Nothing was written into output file` es la consecuencia de que
el encoder de video no produjo ningún frame.

## Información mínima para reportar un problema

Incluye:

- versión de MediaForge e imagen utilizada;
- arquitectura (`linux/amd64` o `linux/arm64`);
- timestamp y zona horaria;
- `requestId`, `jobId` y `planId`, cuando existan;
- waiting state o status HTTP;
- fragmentos relevantes de `backend.log`, `scheduler.log` y `workers.log`;
- mounts internos, ocultando paths privados si es necesario;
- pasos para reproducir.

No publiques tokens de GHCR, cookies, `.env` ni el contenido completo de la base
de datos.
