# Pruebas manuales de home-ctl

Lista de verificación para probar el bot en Telegram. Marca cada caso con `[x]` al pasarlo y anota los fallos en la columna de notas o debajo del caso.

## Preparación

- [x] `deployments/.env` tiene `TELEGRAM_BOT_TOKEN` y `APP_TIMEZONE`.
- [x] `config/telegram.yaml` tiene el `chat_id` del grupo y los IDs de los topics (`shopping`, `chores`, `reminders`).
- [x] `config/users.yaml` tiene los IDs reales de cada persona, sin los IDs de ejemplo.
- [x] El bot está en el grupo como admin y con Group Privacy desactivado en @BotFather.
- [x] Cada persona envió `/start` al bot en privado.
- [x] El contenedor corre con la última imagen: `docker compose -f deployments/docker-compose.yml up -d --build`.
- [x] `docker logs homectl` no muestra `failed to register bot commands`.

Para ver el log en vivo: `docker logs -f homectl`.

## 1. Menú de comandos

- [x] En el grupo, al escribir `/` aparecen `add`, `list`, `status`, `help`.
- [x] En el grupo no aparece `start`.
- [ ] En el chat privado, al escribir `/` aparecen `start`, `list`, `status`, `help`.
- [ ] En el chat privado no aparece `add`.
- [ ] En el chat privado, la descripción de `list` es "Ver tus tareas y recordatorios"; en el grupo sigue siendo "Ver artículos, tareas o recordatorios".
- [x] Las descripciones salen en el idioma del cliente de Telegram (no depende del `locale` de `users.yaml`).

## 2. Acceso y onboarding

- [x] `/start` en privado confirma la conexión.
- [x] `/status` en el grupo lista a los miembros como conectados o "abre el bot y presiona Iniciar".
- [x] `/add` en privado responde con la ayuda.
- [x] Alguien que no está en `users.yaml` escribe `/add` en el grupo: no pasa nada.
- [x] Esa misma persona envía `/start` al bot: no queda conectada.

## 3. Topics

- [x] `/add` en Shopping abre el flujo de compras.
- [x] `/add` en Chores abre el flujo de tareas.
- [x] `/add` en Reminders abre el flujo de recordatorios.
- [x] `/add` y `/list` en el topic General responden el aviso de tema no configurado.

## 4. Shopping

- [x] `/add` con catálogo vacío pide directamente el nombre.
- [x] Nombre "Leche" y emoji 🥛: responde `✅ Creado 🥛 Leche`.
- [x] Repetir el nombre "Leche": rechaza el duplicado.
- [x] `/add` con catálogo: elegir "Leche" responde `✅ Agregado a la lista de compras: 🥛 Leche`.
- [x] Tras elegir un artículo, el menú pierde sus botones y conserva el texto.
- [x] Elegir un artículo que ya está pendiente muestra una alerta y el menú conserva sus botones.
- [x] Cancelar a la mitad del flujo no crea nada y el menú pierde sus botones.
- [x] `/add` y esperar más de 30 minutos, luego pulsar un botón: alerta de expirado y el menú pierde sus botones.
- [x] `/list` muestra los pendientes con botones.
- [x] Tocar un artículo de la lista lo marca comprado y publica `✅ 🥛 Leche — comprado por <nombre>` en el grupo.
- [x] La lista se refresca y los demás artículos siguen tocables.
- [x] Tocar dos veces el mismo botón: la segunda vez avisa que ya fue comprado.
- [x] Con la lista vacía, `/list` responde `✅ La lista de compras está vacía. ¡Todo comprado!`.

## 5. Chores

- [ ] `/add` abre la selección de tarea o pide nombre nuevo si el catálogo está vacío; los pasos con botones editan este único mensaje sin enviar mensajes intermedios.
- [ ] Al seleccionar una tarea existente, el mensaje se edita para pedir el asignado.
- [ ] Al escribir un nombre nuevo, el paso de asignado aparece como mensaje nuevo debajo de tu texto y el mensaje anterior desaparece; los botones que aún pulses en el mensaje anterior dan aviso de que la acción expiró.
- [ ] Cada paso después del título empieza con una cabecera con lo ya elegido, por ejemplo `🧹 Limpiar bebedero · Alice · octubre 14` (en inglés: `🧹 Clean kitchen · Alice · October 14`).
- [ ] Paso de recurrencia: elegir "Una vez" salta intervalo y anclaje directamente al calendario de fecha; tareas recurrentes piden tipo (Diario, Semanal, Mensual).
- [ ] Intervalo: botones 1 a 12 o botón "Otro" para escribir un número entero (1 a 99); valores inválidos muestran guía y mantienen el paso. Los textos de recurrencia usan singular y plural: "Cada semana" para 1, "Cada 2 semanas" para más de 1.
- [ ] Calendario de fecha: muestra el mes actual con botones Hoy, Mañana y Escribir fecha; días pasados son inertes; navegar mes (`<` / `>`) edita el calendario sin avanzar el flujo; escribir fecha acepta fecha u hora y edita el mensaje al siguiente paso.
- [ ] Hora y minuto: elegir hora (00-23) y minuto (00-55 en pasos de 5 u "Otro" para escribir 00-59).
- [ ] Si la fecha elegida es hoy: las horas anteriores a la hora actual aparecen inertes (`·`) y pulsarlas no cambia nada; en la hora actual, los minutos no posteriores al minuto actual aparecen inertes.
- [ ] Si la fecha elegida es hoy, el paso de hora tiene **Cambiar fecha**: vuelve al calendario conservando recurrencia e intervalo.
- [ ] Hora pasada (minuto escrito, o el tiempo avanzó antes de pulsar): el mensaje muestra `⚠️ Esa hora ya pasó.` y vuelve al paso de hora conservando la fecha. No se crea nada.
- [ ] Anclaje de recurrencia: elegir "Mantener horario" o "Desde completado".
- [ ] Resumen de confirmación: una línea por dato (`🧹` título, `👤` asignado, `📅` fecha y hora, `🔁` recurrencia); en tareas recurrentes, una línea `⏭️` con la siguiente ocurrencia en lenguaje claro; botones `✅ Confirmar` y `❌ Cancelar`. La palabra "anclaje" no aparece. En una tarea de una sola vez no hay línea `⏭️`.
- [ ] Confirmar: el mensaje de resumen pierde sus botones y crea exactamente una ocurrencia (`scheduled_at == due_at`).
- [ ] Control de pertenencia: otro usuario presiona un botón del flujo de Alice y recibe aviso de que la interacción pertenece a otro miembro, sin alterar el flujo ni cerrar el teclado.
- [ ] Cancelar en cualquier paso: cierra el teclado del mensaje de flujo y confirma la cancelación.
- [ ] A la hora de vencimiento llega un DM con **Hecho / Posponer**.
- [ ] Otro miembro toca **Hecho** en esa tarea: es rechazado.
- [ ] **Posponer** → 30m: llega otra notificación después.
- [ ] **Hecho**: el topic Chores muestra la actualización.
- [ ] Tarea recurrente: al completarla se genera la siguiente según el intervalo y anclaje configurados.
- [ ] `/list` en privado muestra tus tareas abiertas (sección 10 para el formato y el cambio de vista).

## 6. Reminders

- [ ] `/add` en Reminders pide el título en un mensaje único.
- [ ] Al enviar el título, el selector de destinatario aparece como mensaje nuevo debajo de tu texto y el mensaje del título desaparece.
- [ ] Mismo selector guiado: recurrencia ("Una vez" o periódica), intervalo, calendario de fecha, hora, minuto, anclaje y confirmación. Los pasos con botones editan el mismo mensaje; una respuesta escrita (fecha, intervalo, minuto) abre el siguiente paso como mensaje nuevo debajo de tu texto.
- [ ] Cada paso después del título muestra la cabecera con lo ya elegido (`⏰ Título · Persona · fecha`).
- [ ] Horas y minutos pasados para hoy son inertes; "Cambiar fecha" vuelve al calendario; una hora pasada se explica en el mensaje con `⚠️ Esa hora ya pasó.` y no crea nada.
- [ ] El resumen de confirmación usa las mismas líneas con emoji que en tareas; en un recordatorio de una sola vez muestra `🔁 Una vez` y no tiene línea `⏭️`.
- [ ] Control de pertenencia: si otro miembro intenta presionar botones del recordatorio de otro, recibe aviso de no pertenencia sin efectos.
- [ ] Confirmar: el mensaje de confirmación pierde sus botones y se agenda el recordatorio.
- [ ] A la hora de vencimiento llega el DM al destinatario.
- [ ] Ignorarlo: se repite según `unresolved_reminder_interval`. Para probarlo rápido, pon `2m` en `config/app.yaml` y reinicia.
- [ ] **Posponer** detiene las repeticiones.
- [ ] **Hecho** actualiza el grupo.
- [ ] Pulsar **Hecho** dos veces no duplica nada.
- [ ] `/list` en Reminders entrega tus recordatorios en privado con el mismo formato de la sección 10.

## 7. Persistencia

- [ ] Dejar un flujo a medias, ejecutar `docker compose -f deployments/docker-compose.yml restart` y continuarlo.
- [ ] Crear un recordatorio, reiniciar antes de que venza y confirmar que llega una sola vez.

## 8. Localización

- [ ] Con `locale: es`, las respuestas del bot salen en español.
- [ ] Los mensajes al grupo salen en el `default_locale` de `config/app.yaml`.

## 9. Resistencia al fallo del menú

- [ ] Con un `chat_id` inválido y reinicio, el log muestra el error de registro del menú y el bot sigue funcionando en privado.
- [ ] Restaurar el `chat_id` correcto y reiniciar.

## 10. Listas privadas

- [ ] `/list` en privado abre la vista de tareas (botón `• 🧹 Tareas` marcado) con el botón `⏰ Recordatorios`.
- [ ] Pulsar `⏰ Recordatorios` edita el mismo mensaje a la vista de recordatorios; pulsar `🧹 Tareas` vuelve a la de tareas.
- [ ] Una vista vacía (sin tareas o sin recordatorios) sigue mostrando los botones de cambio.
- [ ] Cada elemento tiene número, emoji, nombre y una línea con la fecha (`📅 hoy a las 23:05`, `📅 mañana a las 10:00`).
- [ ] Un elemento recurrente añade la recurrencia en la misma línea (`· 🔁 Cada 2 semanas`); uno de una sola vez no muestra `🔁`.
- [ ] Un elemento vencido muestra `⚠️ Venció: ayer a las 18:00` en lugar de la línea de fecha.
- [ ] Los botones son numerados (`1. 🚿 Lavar el baño`) y abren el detalle; Volver regresa a la misma lista.
- [ ] Con más de 20 elementos, la lista muestra 20 y la línea `+N más` al final.

## Diagnóstico si algo falla

Si el bot no responde a texto normal en el grupo, revisar el privacy mode:

```bash
set -a; . deployments/.env; set +a
T=https://api.telegram.org/bot$TELEGRAM_BOT_TOKEN
curl -s $T/getMe | python3 -m json.tool          # can_read_all_group_messages debe ser true
curl -s $T/getWebhookInfo | python3 -m json.tool  # url debe estar vacío
```

Para ver los updates crudos, detener primero el contenedor (dos consumidores de `getUpdates` chocan con error 409):

```bash
docker compose -f deployments/docker-compose.yml stop
curl -s "$T/getUpdates" | python3 -m json.tool
docker compose -f deployments/docker-compose.yml start
```
