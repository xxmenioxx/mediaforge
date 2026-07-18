# Scheduler v1 — checklist de validación

Este documento valida el flujo completo del scheduler antes de considerar v1 lista para uso continuo. Las pruebas automatizadas cubren reglas y concurrencia; esta lista cubre además FFmpeg, filesystem, energía y reinicios reales.

## Preparación

1. Respalda la base de datos y conserva al menos un asset pequeño de prueba.
2. Inicia backend y frontend con los comandos habituales del proyecto.
3. En **Settings > Scheduler runtime**, pulsa **Refresh detection**.
4. Confirma que aparecen CPU, memoria, espacio, tipo de disco, encoders y estado de energía. Un dato no detectable debe mostrarse como `unknown`, nunca bloquear la pantalla.
5. Configura una librería raw, una work y una published en discos de prueba.

## Plan y ejecución

- [ ] Valida un asset y abre el review plan antes de ejecutarlo.
- [ ] Confirma input, output esperado, streams, encoder, argumentos y estimación de tamaño.
- [ ] Verifica que un track profile asignado al path sólo modifica los assets a los que aplica.
- [ ] Verifica que el perfil seleccionado tenga precedencia sobre codecs y encoders inferidos.
- [ ] Ejecuta un dry run: no debe crear output, reservar worker ni cambiar el asset a running.
- [ ] Activa auto ejecución y confirma que un plan listo pasa una sola vez a running.

## Recursos y concurrencia

- [ ] Con un solo slot de video, encola dos transcodes: sólo uno debe quedar running.
- [ ] Encola dos veces el mismo path: debe existir un solo lock activo para el asset.
- [ ] Reduce artificialmente el espacio libre requerido: el plan debe esperar por disco y reanudarse al liberar la condición.
- [ ] En un portátil, activa `pauseWhenOnBattery`: en batería debe indicar `WAITING_POWER`; al conectar corriente debe volver a ser elegible.
- [ ] En macOS, activa prevención de reposo y confirma que el proceso FFmpeg queda acompañado por `caffeinate` hasta terminar.
- [ ] En NAS/Linux sin sensores compatibles, confirma que los valores desconocidos producen una política conservadora y no un crash.

## Resultado y ciclo de vida

- [ ] Una pista AAC estéreo única y válida no debe generar otra pista AAC de compatibilidad.
- [ ] Ejecuta un caso VideoToolbox y confirma que usa bitrate compatible, no `-q:v`.
- [ ] Al finalizar, el output debe validarse antes de publicar.
- [ ] El publisher debe mover el archivo a published y registrar el path definitivo en DB.
- [ ] El original debe archivarse y desaparecer de raw cuando la política lo indique.
- [ ] El asset publicado no debe volver a aparecer como `Not published yet` ni `Run Again` por ausencia del workspace.
- [ ] El housekeeping preview debe listar sólo workspaces elegibles. La ejecución debe eliminar los publicados y los fallidos/cancelados vencidos, nunca queued/running ni completed sin publicar.

## Reinicio y recuperación

1. Inicia un transcode y detén el backend mientras FFmpeg sigue activo.
2. Reinicia el backend.
3. Confirma que reconciliación no crea una segunda ejecución ni pierde el lock.
4. Si el proceso ya terminó, confirma que el job avanza según el output real o queda fallido con causa recuperable.
5. Confirma que reservas huérfanas y workspaces vencidos aparecen en housekeeping, primero como dry run.

## Migración

- [ ] Arranca una copia de la base anterior con el binario nuevo.
- [ ] Confirma que perfiles, jobs y estados permanecen disponibles.
- [ ] Los jobs antiguos deben recibir snapshot de perfil, etapa, plan activo y reserva sin duplicarse.
- [ ] Repite el arranque: la migración debe ser idempotente.

## Criterio de aceptación

Scheduler v1 queda aceptado cuando la suite Go, el detector de carreras y el build del frontend pasan, y este checklist se completa en macOS y en el entorno NAS/Linux objetivo sin duplicados, pérdida de assets ni estados que contradigan el filesystem.
