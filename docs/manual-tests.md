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
- [x] En el chat privado, al escribir `/` aparecen `start`, `status`, `help`.
- [x] En el chat privado no aparecen `add` ni `list`.
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

- [ ] `/add` abre la selección de tarea o pide nombre nuevo si el catálogo está vacío; todo el flujo edita este único mensaje sin enviar mensajes intermedios.
- [ ] Al seleccionar o escribir el nombre, el mensaje se edita para pedir el asignado.
- [ ] Paso de recurrencia: elegir "Una vez" salta intervalo y anclaje directamente al calendario de fecha; tareas recurrentes piden tipo (Diario, Semanal, Mensual).
- [ ] Intervalo: botones 1 a 12 o botón "Otro" para escribir un número entero (1 a 99); valores inválidos muestran guía y mantienen el paso.
- [ ] Calendario de fecha: muestra el mes actual con botones Hoy, Mañana y Escribir fecha; días pasados son inertes; navegar mes (`<` / `>`) edita el calendario sin avanzar el flujo; escribir fecha acepta fecha u hora y edita el mensaje al siguiente paso.
- [ ] Hora y minuto: elegir hora (00-23) y minuto (00-55 en pasos de 5 u "Otro" para escribir 00-59).
- [ ] Si la hora elegida para hoy ya pasó: muestra aviso de hora pasada y se mantiene en el paso de hora conservando la fecha.
- [ ] Anclaje de recurrencia: elegir "Mantener horario" o "Desde completado".
- [ ] Resumen de confirmación: muestra título, asignado, fecha/hora, recurrencia y anclaje con botones Confirmar y Cancelar.
- [ ] Confirmar: el mensaje de resumen pierde sus botones y crea exactamente una ocurrencia (`scheduled_at == due_at`).
- [ ] Control de pertenencia: otro usuario presiona un botón del flujo de Alice y recibe aviso de que la interacción pertenece a otro miembro, sin alterar el flujo ni cerrar el teclado.
- [ ] Cancelar en cualquier paso: cierra el teclado del mensaje de flujo y confirma la cancelación.
- [ ] A la hora de vencimiento llega un DM con **Hecho / Posponer**.
- [ ] Otro miembro toca **Hecho** en esa tarea: es rechazado.
- [ ] **Posponer** → 30m: llega otra notificación después.
- [ ] **Hecho**: el topic Chores muestra la actualización.
- [ ] Tarea recurrente: al completarla se genera la siguiente según el intervalo y anclaje configurados.
- [ ] `/list` en privado muestra tus tareas abiertas (solo tareas).

## 6. Reminders

- [ ] `/add` en Reminders pide el título en un mensaje único.
- [ ] Al enviar el título, el mensaje se edita para seleccionar el destinatario.
- [ ] Mismo selector guiado: recurrencia ("Una vez" o periódica), intervalo, calendario de fecha, hora, minuto, anclaje y confirmación, editando siempre el mismo mensaje.
- [ ] Control de pertenencia: si otro miembro intenta presionar botones del recordatorio de otro, recibe aviso de no pertenencia sin efectos.
- [ ] Confirmar: el mensaje de confirmación pierde sus botones y se agenda el recordatorio.
- [ ] A la hora de vencimiento llega el DM al destinatario.
- [ ] Ignorarlo: se repite según `unresolved_reminder_interval`. Para probarlo rápido, pon `2m` en `config/app.yaml` y reinicia.
- [ ] **Posponer** detiene las repeticiones.
- [ ] **Hecho** actualiza el grupo.
- [ ] Pulsar **Hecho** dos veces no duplica nada.

## 7. Persistencia

- [ ] Dejar un flujo a medias, ejecutar `docker compose -f deployments/docker-compose.yml restart` y continuarlo.
- [ ] Crear un recordatorio, reiniciar antes de que venza y confirmar que llega una sola vez.

## 8. Localización

- [ ] Con `locale: es`, las respuestas del bot salen en español.
- [ ] Los mensajes al grupo salen en el `default_locale` de `config/app.yaml`.

## 9. Resistencia al fallo del menú

- [ ] Con un `chat_id` inválido y reinicio, el log muestra el error de registro del menú y el bot sigue funcionando en privado.
- [ ] Restaurar el `chat_id` correcto y reiniciar.

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
