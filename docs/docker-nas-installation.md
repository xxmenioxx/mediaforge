# MVForge — instalación Docker en NAS

Esta es la ruta de instalación recomendada para ejecutar una versión publicada de MVForge sin clonar ni compilar el repositorio.

## Requisitos

- NAS Intel `linux/amd64` con Docker y Docker Compose v2.
- Carpetas dedicadas para MVForge con permisos de lectura y escritura para Docker.
- Acceso al puerto elegido desde la red local.

No expongas MVForge directamente a Internet. Para acceso remoto utiliza una VPN o un reverse proxy con autenticación y TLS.

## Instalación estándar

MVForge se instala de forma independiente bajo el directorio Docker habitual del NAS:

```text
/volume1/docker/mvforge
```

Todas las rutas se configuran mediante `.env`. No existe una dependencia con `nas-media-stack`, Portainer u otro proyecto. En el NAS objetivo, `volume1` es NVMe y se utiliza para raw activo y staging; `volume2` conserva las bibliotecas y el archivo de originales.

## 1. Preparar carpetas

```sh
mkdir -p /volume1/docker/mvforge/config
mkdir -p /volume1/docker/mvforge/data/raw
mkdir -p /volume1/docker/mvforge/data/staging
mkdir -p /volume1/docker/mvforge/reports
mkdir -p /volume2/media/mvforge/originals_archive
```

## 2. Descargar el instalador

Desde el release elegido descarga:

- `mvforge-compose.yml`
- `mvforge.env.example`
- `mvforge-backup.sh`

Guárdalos en `/volume1/docker/mvforge` y renombra la configuración:

```sh
mv mvforge-compose.yml compose.yml
mv mvforge.env.example .env
chmod +x mvforge-backup.sh
```

Edita `.env` y configura la versión, el puerto, la zona horaria y los paths absolutos del NAS. No uses `latest`; conserva una versión explícita como `0.1.0`.

Para levantar MVForge como parte del stack personal en lugar de una aplicación independiente, consulta [Integración opcional con nas-media-stack](guides/nas-media-stack-integration.md).

## 3. Instalar

Si las imágenes son públicas:

```sh
docker compose --env-file .env -f compose.yml pull
docker compose --env-file .env -f compose.yml up -d
docker compose --env-file .env -f compose.yml ps
```

Si todavía son privadas, crea un personal access token de GitHub con acceso de lectura a packages y autentica el NAS antes del `pull`:

```sh
printf '%s' "$GHCR_TOKEN" | docker login ghcr.io -u TU_USUARIO --password-stdin
```

Abre `http://IP-DEL-NAS:8090`, sustituyendo el puerto si cambiaste `MVFORGE_PORT`.

## 4. Primera prueba segura

1. Comprueba que ambos servicios aparecen como `healthy`.
2. Mantén `dryRunOnly` habilitado.
3. Mantén validación y publicación automáticas deshabilitadas.
4. Registra las librerías usando paths internos `/media/...`, no paths del host.
5. Procesa un único archivo pequeño y descartable.
6. Completa el checklist de [scheduler-v1-validation.md](scheduler-v1-validation.md).

Para inspeccionar problemas:

```sh
docker compose --env-file .env -f compose.yml logs -f --tail=200
```

MVForge utiliza SQLite con WAL, espera limitada ante locks y un único writer.
Ejecuta exactamente una réplica del servicio `backend` por archivo
`mvforge.db`. Dos contenedores apuntando al mismo archivo no constituyen una
configuración soportada y pueden producir `database table is locked`.

El log persistente se encuentra en `/media/reports/logs/backend.log`; en el host
corresponde al directorio `reports/logs` configurado en Compose. Consulta la
[guía de troubleshooting](guides/troubleshooting.md) antes de modificar la base
o reencolar jobs bloqueados.

## Backup de SQLite

Ejecuta el backup antes de cada actualización:

```sh
COMPOSE_FILE=compose.yml ./mvforge-backup.sh
```

El script utiliza la operación `.backup` de SQLite dentro del contenedor y escribe una copia consistente en:

```text
CONFIG_PATH/backups/mvforge-YYYYMMDDTHHMMSSZ.db
```

Conserva además una copia de `reports` y de `.env` fuera del volumen principal.

## Actualizar

1. Confirma que no haya jobs ejecutándose.
2. Ejecuta el backup.
3. Anota la versión actual de `.env`.
4. Cambia `MVFORGE_VERSION` a la nueva versión.
5. Descarga y recrea los servicios:

```sh
docker compose --env-file .env -f compose.yml pull
docker compose --env-file .env -f compose.yml up -d
docker compose --env-file .env -f compose.yml ps
```

Verifica UI, `/health`, configuración, librerías y un dry run antes de reactivar conversiones.

## Rollback

Si la aplicación no arranca, vuelve a colocar la versión anterior en `.env` y ejecuta:

```sh
docker compose --env-file .env -f compose.yml pull
docker compose --env-file .env -f compose.yml up -d
```

Si la versión nueva modificó la base de datos de forma incompatible, detén primero el stack y restaura también el backup previo. Nunca reemplaces la base mientras el backend está ejecutándose.

## Publicar una versión (mantenedores)

Los workflows se dividen en:

- `.github/workflows/ci.yml`: tests de Go, race detector, vet, lint/build del frontend y builds Docker en cada PR o push a `main`.
- `.github/workflows/release-images.yml`: publicación `linux/amd64` en GHCR y creación del release al enviar un tag semántico.

Para publicar la primera versión:

```sh
git tag v0.1.0
git push origin v0.1.0
```

El workflow publica:

```text
ghcr.io/xxmenioxx/mvforge-backend:0.1.0
ghcr.io/xxmenioxx/mvforge-web:0.1.0
```

En GitHub, comprueba que Actions tenga permiso para crear packages y que ambos packages estén conectados al repositorio. Para una instalación sin login, cambia la visibilidad de los packages a pública después del primer release.
