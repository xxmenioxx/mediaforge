# MediaForge — instalación Docker en NAS

Esta es la ruta de instalación recomendada para ejecutar una versión publicada de MediaForge sin clonar ni compilar el repositorio.

## Requisitos

- NAS `linux/amd64` o `linux/arm64` con Docker y Docker Compose v2.
- Carpetas dedicadas para MediaForge con permisos de lectura y escritura para Docker.
- Acceso al puerto elegido desde la red local.

No expongas MediaForge directamente a Internet. Para acceso remoto utiliza una VPN o un reverse proxy con autenticación y TLS.

## 1. Preparar carpetas

Adapta `/volume1` al path real del NAS:

```sh
mkdir -p /volume1/mediaforge/config
mkdir -p /volume1/mediaforge/raw
mkdir -p /volume1/mediaforge/library
mkdir -p /volume1/mediaforge/staging
mkdir -p /volume1/mediaforge/originals_archive
mkdir -p /volume1/mediaforge/reports
```

## 2. Descargar el instalador

Desde el release elegido descarga:

- `mediaforge-compose.yml`
- `mediaforge.env.example`
- `mediaforge-backup.sh`

Guárdalos en una carpeta dedicada, por ejemplo `/volume1/docker/mediaforge`, y renombra la configuración:

```sh
mv mediaforge-compose.yml compose.yml
mv mediaforge.env.example .env
chmod +x mediaforge-backup.sh
```

Edita `.env` y configura la versión, el puerto, la zona horaria y los paths absolutos del NAS. No uses `latest`; conserva una versión explícita como `0.1.0`.

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

Abre `http://IP-DEL-NAS:8090`, sustituyendo el puerto si cambiaste `MEDIAFORGE_PORT`.

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

## Backup de SQLite

Ejecuta el backup antes de cada actualización:

```sh
COMPOSE_FILE=compose.yml ./mediaforge-backup.sh
```

El script utiliza la operación `.backup` de SQLite dentro del contenedor y escribe una copia consistente en:

```text
CONFIG_PATH/backups/mediaforge-YYYYMMDDTHHMMSSZ.db
```

Conserva además una copia de `reports` y de `.env` fuera del volumen principal.

## Actualizar

1. Confirma que no haya jobs ejecutándose.
2. Ejecuta el backup.
3. Anota la versión actual de `.env`.
4. Cambia `MEDIAFORGE_VERSION` a la nueva versión.
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
- `.github/workflows/release-images.yml`: publicación multi-arquitectura en GHCR y creación del release al enviar un tag semántico.

Para publicar la primera versión:

```sh
git tag v0.1.0
git push origin v0.1.0
```

El workflow publica:

```text
ghcr.io/xxmenioxx/mediaforge-backend:0.1.0
ghcr.io/xxmenioxx/mediaforge-web:0.1.0
```

En GitHub, comprueba que Actions tenga permiso para crear packages y que ambos packages estén conectados al repositorio. Para una instalación sin login, cambia la visibilidad de los packages a pública después del primer release.
