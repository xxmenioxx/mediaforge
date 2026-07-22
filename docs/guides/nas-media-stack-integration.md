# Integración opcional de MVForge con nas-media-stack

El HomeLab tiene dos repositorios con responsabilidades distintas:

- **MVForge**: código, tests, Dockerfiles, imágenes GHCR y releases.
- **nas-media-stack**: fuente de verdad del despliegue real del NAS, servicios, red, puertos, variables y backups.

Esta integración es específica del HomeLab y no forma parte de los requisitos de instalación estándar. MVForge continúa siendo instalable de manera independiente en `/volume1/docker/mvforge`.

Si se elige este modo, `nas-media-stack` administra el servicio y no debe levantarse simultáneamente otra instancia desde `/volume1/docker/mvforge` usando el mismo puerto o las mismas carpetas.

## Layout oficial

```text
/volume1/docker/nas-media-stack/
  compose.yaml
  compose/
  config/
    mvforge/
      mvforge.db
      backups/
      reports/
  work/
    mvforge/
      raw/
      staging/

/volume2/media/
  movies/
  anime-movies/
  series/
  anime/
  music/
  downloads/
  mvforge/
    originals_archive/
```

```mermaid
flowchart TD
    REPO["MVForge repository"] --> GHCR["GHCR versioned images"]
    GHCR --> STACK["/volume1/docker/nas-media-stack"]
    STACK --> CONFIG["config/mvforge: DB, backups, reports"]
    STACK --> NETWORK["media_network"]
    STACK --> WORK["NVMe work: raw and staging"]
    STACK --> MEDIA["/volume2/media"]
    MEDIA --> LIBRARY["movies, series, anime, music"]
    MEDIA --> ARCHIVE["mvforge/originals_archive"]
```

## Distribución de datos

| Rol | Host path | Container path | Backup |
|---|---|---|---|
| SQLite/config | `${CONFIG_ROOT}/mvforge` | `/app/data` | Sí |
| Reports | `${CONFIG_ROOT}/mvforge/reports` | `/media/reports` | Sí |
| Raw activo | `${MVFORGE_WORK_ROOT}/raw` | `/media/raw` | No; área de trabajo NVMe |
| Movies | `${MEDIA_ROOT}/movies` | `/media/library/movies` | No en backup del stack |
| Anime movies | `${MEDIA_ROOT}/anime-movies` | `/media/library/anime-movies` | No en backup del stack |
| Series | `${MEDIA_ROOT}/series` | `/media/library/series` | No en backup del stack |
| Anime | `${MEDIA_ROOT}/anime` | `/media/library/anime` | No en backup del stack |
| Music | `${MEDIA_ROOT}/music` | `/media/library/music` | No en backup del stack |
| Staging | `${MVFORGE_WORK_ROOT}/staging` | `/media/staging` | No; área de trabajo NVMe |
| Originales archivados | `${MEDIA_ROOT}/mvforge/originals_archive` | `/media/originals_archive` | Según política de medios |

`CONFIG_ROOT` actualmente es `/volume1/docker/nas-media-stack/config`, `MVFORGE_WORK_ROOT` es `/volume1/docker/nas-media-stack/work/mvforge` y `MEDIA_ROOT` es `/volume2/media`.

## Módulo Compose propuesto

En `nas-media-stack`, MVForge debe vivir en `compose/mvforge.yaml`:

```yaml
services:
  mvforge-backend:
    image: ghcr.io/xxmenioxx/mvforge-backend:${MVFORGE_VERSION}
    container_name: mvforge-backend
    environment:
      MVFORGE_API_HOST: 0.0.0.0
      MVFORGE_API_PORT: 8080
      MVFORGE_DB_PATH: /app/data/mvforge.db
      TZ: ${TZ}
    volumes:
      - ${CONFIG_ROOT}/mvforge:/app/data
      - ${MVFORGE_WORK_ROOT}/raw:/media/raw
      - ${MEDIA_ROOT}/movies:/media/library/movies
      - ${MEDIA_ROOT}/anime-movies:/media/library/anime-movies
      - ${MEDIA_ROOT}/series:/media/library/series
      - ${MEDIA_ROOT}/anime:/media/library/anime
      - ${MEDIA_ROOT}/music:/media/library/music
      - ${MVFORGE_WORK_ROOT}/staging:/media/staging
      - ${MEDIA_ROOT}/mvforge/originals_archive:/media/originals_archive
      - ${CONFIG_ROOT}/mvforge/reports:/media/reports
    # Habilita este device sólo en hosts Intel con /dev/dri disponible.
    devices:
      - /dev/dri:/dev/dri
    networks:
      media_network:
        aliases:
          - backend
    restart: unless-stopped
    healthcheck:
      test: ["CMD", "wget", "-qO-", "http://127.0.0.1:8080/health"]
      interval: 30s
      timeout: 5s
      start_period: 15s
      retries: 3

  mvforge:
    image: ghcr.io/xxmenioxx/mvforge-web:${MVFORGE_VERSION}
    container_name: mvforge
    ports:
      - "${MVFORGE_PORT}:80"
    depends_on:
      mvforge-backend:
        condition: service_healthy
    networks:
      - media_network
    restart: unless-stopped
```

El bloque `devices` es necesario para Intel Quick Sync (`hevc_qsv`). Elimínalo
en hosts sin `/dev/dri`; MVForge marcará QSV como no usable y utilizará un
encoder permitido alternativo. Después de cambiar devices, recrea el backend y
ejecuta **Refresh host**.

El alias `backend` es necesario porque el Nginx incluido en la imagen web utiliza ese hostname para el proxy interno.

## Cambios en nas-media-stack

1. Copiar el módulo a `compose/mvforge.yaml`.
2. Añadirlo a los `include` de `compose.yaml`.
3. Añadir a `.env.example`:

```dotenv
MVFORGE_VERSION=0.1.0
MVFORGE_PORT=8090
MVFORGE_WORK_ROOT=/volume1/docker/nas-media-stack/work/mvforge
```

4. Crear carpetas:

```sh
mkdir -p /volume1/docker/nas-media-stack/config/mvforge/reports
mkdir -p /volume1/docker/nas-media-stack/work/mvforge/raw
mkdir -p /volume1/docker/nas-media-stack/work/mvforge/staging
mkdir -p /volume2/media/mvforge/originals_archive
```

5. Validar desde `/volume1/docker/nas-media-stack`:

```sh
docker compose --env-file .env config
docker compose --env-file .env pull
docker compose --env-file .env up -d
docker compose --env-file .env ps
```

## Backups

El script de backup de `nas-media-stack` ya protege `/volume1/docker/nas-media-stack`. Al guardar SQLite, backups y reports bajo `${CONFIG_ROOT}/mvforge`, pasan a formar parte del backup del stack.

Antes de actualizar MVForge, crea además un backup consistente con SQLite `.backup`; copiar el directorio mientras la base está escribiendo no sustituye ese paso.

El directorio `work/` del stack se excluye del backup porque contiene raw activo y staging reconstruible. Los archivos bajo `/volume2/media` también se excluyen deliberadamente y requieren su propia política de protección.

## Red y futuras integraciones

MVForge puede pertenecer a `media_network`, pero no necesita acceso directo a Radarr, Sonarr, Bazarr o Jellyfin para su flujo v1. Cualquier integración futura debe usar Docker DNS y credenciales dedicadas; no debe asumir acceso administrativo sólo por compartir red.

## Actualizaciones

La imagen se publica desde el repositorio MVForge. El despliegue se actualiza desde `nas-media-stack`:

```mermaid
flowchart LR
    TAG["MVForge tag vX.Y.Z"] --> IMAGE["GHCR image X.Y.Z"]
    IMAGE --> ENV["Update MVFORGE_VERSION in nas-media-stack"]
    ENV --> BACKUP["SQLite backup"]
    BACKUP --> UPDATE["scripts/update.sh or docker compose up -d"]
    UPDATE --> VALIDATE["Health + dry run"]
```

No uses `latest`; actualiza `MVFORGE_VERSION` mediante un cambio revisado en `nas-media-stack`.
