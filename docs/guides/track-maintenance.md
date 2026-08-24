# Track maintenance

Assets muestra un único tab **Tracks** con el inventario real de video, audio,
subtítulos, attachments y data streams.

## Alcance seguro

- Raw/Unprocessed y Archive son sólo lectura. Sus decisiones de tracks se
  aplican durante una conversión y el original no se modifica.
- Las acciones de mantenimiento están disponibles para MKV activos de Library
  y Converted.
- Un Queue job o análisis activo bloquea temporalmente el mantenimiento.
- Cada operación comprueba el fingerprint antes de comenzar, escribe un MKV
  temporal, valida streams y capítulos, conserva un backup durante el commit y
  refresca el snapshot técnico al terminar.

## Acciones

- **Edit** cambia título, idioma y disposiciones default/forced mediante remux.
- **Delete** elimina la pista seleccionada después de confirmación explícita.
  MVForge nunca permite eliminar el último video reproducible.
- **Create AAC track** conserva la pista fuente y añade una copia AAC de 128,
  160, 192, 256 o 320 kbps. Puede producir stereo o conservar el número de
  canales, heredar/editar idioma y título, y convertirse en la pista default.

Los streams que no participan en la nueva pista AAC se copian sin recodificar.
La operación no crea un QueueJob ni cambia el estado del asset.

Fuentes: `handlers/track_maintenance.go` y
`components/TrackMaintenancePanel.tsx`.
