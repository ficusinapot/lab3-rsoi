# Hotels Booking System

Gateway (`8080`) предоставляет внешний `/api/v1` API варианта v2.
Reservation (`8070`) хранит отели и бронирования; Payment (`8060`) — оплаты;
Loyalty (`8050`) — счётчик, уровень и скидку. Все межсервисные вызовы выполняет
Gateway по HTTP. Gateway не использует БД.

Каждый сервис — отдельный Go-модуль. Компоненты получают зависимости через
конструкторы; общая инфраструктура находится в `modules/platform`.

## Запуск и готовность

```sh
docker compose up --build -d
WAIT_PORTS=8080,8070,8060,8050 scripts/wait-script.sh
```

PostgreSQL 13 создаёт БД `reservations`, `payments`, `loyalties` на новом volume.
Проверка готовности использует TCP: временный сервер init-скриптов принимает
соединения только через Unix-сокет, поэтому приложения запускаются после
завершения инициализации и старта основного PostgreSQL.
Учебные credentials: `program` / `test`. Если volume уже существует, проверьте
список БД; init-скрипты PostgreSQL повторно не выполняются. Недостающие БД
создаются отдельно, без удаления volume:

```sh
docker compose exec reservation-db psql -U program -d postgres -c 'CREATE DATABASE payments OWNER program'
docker compose exec reservation-db psql -U program -d postgres -c 'CREATE DATABASE loyalties OWNER program'
```

Все приложения читают выбранный YAML через `--config PATH` или `--config=PATH`.
Docker entrypoints Reservation, Payment и Loyalty распознают обе формы в любом месте
аргументов до `--`: Atlas и приложение используют один выбранный YAML.
Gateway разрешает только `http.api_prefix: /api/v1`; другой префикс отклоняется
при проверке конфигурации. Адреса и таймауты
HTTP-клиентов Gateway находятся в секции `clients`. Настройки серверов,
документации, логирования и метрик также задаются YAML. Нет настройки через
переменные окружения. `config.docker.yaml` применяется в Docker.

Для Payment/Loyalty Docker entrypoint запускает внешний Atlas runner, который
читает URL БД из YAML через yq. Схема создаётся native SQL-миграциями с `atlas.sum`;
приложения не выполняют миграции. Generated Ent-клиенты хранятся в репозитории.

```sh
make -C services/payment generate
make -C services/loyalty generate
make -C services/gateway generate
make -C services/payment migrate
make -C services/loyalty migrate
make -C services/gateway run CONFIG=config.yaml
```

Каждый сервис предоставляет `/manage/health`, `/metrics`, `/api/v1/docs` и
`/api/v1/openapi.json`. Проверка готовности сервисов с БД проверяет подключение;
Gateway проверяет собственную доступность. Его Chi-маршруты и типы запросов
генерируются из OpenAPI-файла варианта `v2`. RED использует шаблоны маршрутов;
метки не содержат пользователей и UUID. Platform сериализует короткий участок
подготовки и записи autometrics v1.1.0, использующий общий небезопасный PRNG
и карту вызовов. Сами операции выполняются параллельно.

Swagger UI во всех сервисах формирует Huma. Gateway публикует исходную схему
oapi-codegen, подготовленную один раз при запуске; пути документации выбираются YAML.

## Внутренние HTTP-контракты

Reservation сохраняет контракт, описанный в `reservation.md`.

Payment:

- `POST /api/v1/payments`, `{ "price": 27000 }` → `201`,
  `{ "paymentUid": "UUID", "status": "PAID", "price": 27000 }`.
- `GET /api/v1/payments/{paymentUid}` → `200`, тот же объект.
- `DELETE /api/v1/payments/{paymentUid}` → `204`, статус `CANCELED`.
  Повторная отмена сохраняет статус; неизвестная оплата возвращает `404`.

Loyalty использует `X-User-Name`:

- `GET /api/v1/loyalty` → `200`,
  `{ "status": "BRONZE", "discount": 5, "reservationCount": 0 }`.
- `POST /api/v1/loyalty/reservations` увеличивает счётчик.
- `DELETE /api/v1/loyalty/reservations` уменьшает его, минимум — ноль.
- Оба изменения возвращают `200` с актуальным объектом Loyalty.

Первое обращение создаёт пользователя `BRONZE/0/5`. Изменения счётчика и уровня
выполняются транзакцией с блокировкой строки. При 10 бронированиях уровень
`SILVER` и скидка 7%, при 20 — `GOLD` и 10%; отмена пересчитывает уровень.

## Внешнее поведение

Контракт Gateway соответствует материалам `v2`, внутренний `paymentUid`
не выдаётся. Чтение бронирований включает объект отеля и состояние оплаты;
`/me` дополнительно включает Loyalty. Внешний POST возвращает `200`,
внутренний POST Reservation — `201`.

Дата передаётся в формате `YYYY-MM-DD`; конечная дата должна быть позже
начальной. Стоимость считается целыми числами: ночи × цена × (100 − скидка) / 100,
дробная часть отбрасывается. Переполнение и результат вне диапазона PostgreSQL
INT отклоняются до создания оплаты. Применяется скидка до увеличения счётчика.
Пагинация Reservation начинается с 1; Gateway допускает page=0 из OpenAPI как
первую страницу и сохраняет 0 в ответе. Размер внешней страницы — от 1 до 100.

Создание выполняет Payment → Reservation → увеличение Loyalty после чтения
отеля и скидки. Отмена выполняет Payment → Reservation → уменьшение Loyalty
после проверки владельца. Чужое бронирование возвращает `404`.
Уже отменённое бронирование возвращает `204` без повторных изменений.

При ошибке создания известные Reservation и Payment компенсируются независимо.
Незавершённые компенсации и отмены повторяются через очередь Redis.
Место резервируется до побочных эффектов; заполненная очередь возвращает `503`.
Отказ Payment при отмене оставляет Reservation `PAID`; отказ Loyalty после
отмены Reservation возвращает `204` после постановки оставшегося шага в очередь.
Критичный Reservation возвращает `500`, недоступный Loyalty при самостоятельном
чтении или создании — `503`. Ошибки ввода остаются `400`, отсутствие — `404`.
При чтении недоступный Payment представлен как `payment={}`, а Loyalty в `/me`
как `loyalty={}`. Повтор POST Payment автоматически не выполняется.

### Исполнение операций

Сборка зависимостей находится в `services/gateway/cmd/gateway/run.go`.
Компонент `internal/core/services/opstream` владеет операцией от резервирования
места до завершения всех шагов. Bookings зависит от `operationifc.Groups`.
Исполнитель групп использует `opstreamifc.Stream`: резервирует место до побочных
эффектов, передаёт отмену или компенсацию в `Execute` и проверяет
`PendingOperation` при повторной отмене. При успешном создании или ошибке до
побочных эффектов он освобождает резерв через `ReleaseOperation`.
Перед отменой `Groups.Cancel(ctx, user, reservationUID)` проверяет активную операцию
создания/компенсации по `OperationKey(OperationCompensation, reservationUID)`.
Пока она активна, возвращается `ErrOperationBusy` (`503`) без побочных эффектов.
Только после этой проверки Groups читает актуальную Reservation и начинает отмену.
Это предотвращает отмену до увеличения Loyalty и использование устаревшего снимка
`PAID`. `Commands.Cancel` проверяет имя и передаёт UUID в Groups.
Создание сохраняет назначенные UUID и план компенсации до первого POST;
перед увеличением Loyalty добавляет шаг ResolveCreation. `CreationOperation` строит план по результатам Payment и Reservation.
Исполнитель `operations.Groups` резервирует очередь, сохраняет план и в `finish`
создаёт независимый контекст отката с таймаутом из YAML
и вызывает Stream. Stream управляет выдачей, фиксацией и фоновым исполнением.

Один Processor из `internal/core/usecases/operations` используется первой
попыткой и фоновым worker. Он получает режим ExecutionInitial или
ExecutionBackground и возвращает оставшийся план и решение:

| Результат | Действие очереди |
| --- | --- |
| OperationCompleted | Освободить место, сохранить завершение |
| OperationDeferred | Сохранить оставшиеся шаги и назначить повтор |
| OperationFailed | Удерживать место, ждать явного ручного повтора |
| OperationRejected | Сохранить отказ в истории, освободить место |

Первичный отказ отмены Payment возвращает OperationRejected и сохраняет
Reservation PAID. Постоянные ошибки 400/404 фонового исполнения дают
OperationFailed; panic остаётся повторяемой ошибкой. Эти правила находятся
в Processor, worker не анализирует вид операции, шаги или бизнес-ошибки.
Store сохраняет оба вида отказа как failed, но только OperationFailed остаётся
в активной очереди. `Finish` принимает итоговый результат без отдельного флага retry.

`opstreamifc.Store` отделяет публикацию, выдачу и фиксацию результата от Processor.
Адаптер `internal/redis/repos` использует Redis Streams с consumer group `workers`.
У каждого направления свой stream: `stream:compensation`, `stream:cancellation`,
`stream:loyalty`. `XREADGROUP` выдаёт одну задачу работнику, а `XAUTOCLAIM`
передаёт зависшее неподтверждённое сообщение после истечения lease.
`Finish` атомарно выполняет `XACK`/`XDEL` и сохраняет оставшийся план.
Sorted sets `due:*` используются только для отложенной публикации после ошибки
и восстановления прерванной синхронной попытки. Проверка лимитов, публикация
в stream и фиксация результата реализованы в Go через `WATCH` и `MULTI/EXEC`,
без Lua. При конфликте транзакция повторяется в пределах `storage_timeout`
и контекста; ошибки связи автоматически не повторяются, поскольку результат
`EXEC` может быть неизвестен.
Публикация использует явный ID сообщения и наблюдает за stream: сообщение и
ссылка на него в записи задачи фиксируются одной транзакцией. Redis не инвалидирует
`WATCH` при изменении pending entries, поэтому каждое
изменение доставки (`XREADGROUP`, `XAUTOCLAIM`, `XCLAIM`, `XACK`) увеличивает
`stream:*:version` в той же транзакции. `Take` и `Finish` наблюдают за этим ключом,
чтобы смена consumer отменяла устаревший `EXEC`.
Fencing-токены выдаёт отдельный `INCR`; пропуски после конфликтов допустимы.
При обновлении нужно остановить все Gateway со старым Lua-адаптером: они не
обновляют ключ версии доставки. Redis volume сохраняется; формат текущих
записей и сообщений Streams не меняется.
PostgreSQL не используется как очередь. Redis хранит состояния reserved, running,
deferred, completed и failed. История завершённых операций хранится 24 часа.
Lease на 30 секунд и возрастающий fencing token запрещают устаревшему работнику
фиксировать результат после повторной выдачи задачи. Время берётся из Redis.

Три пула работников проверяют готовность каждые 100ms: два для компенсаций,
один для отмен и один для оставшегося изменения Loyalty. Повтор после ошибки
доступности откладывается минимум на 10s; попытка ограничена 5s. Общая ёмкость
256 разделена на 128 компенсаций и 128 отмен, включая их шаги Loyalty.
Постоянные ошибки 400/404 в фоне переводят операцию в failed: payload остаётся
в Redis и занимает место, автоматический бесконечный повтор прекращается.
Операции с паникой продолжают повторяться. Первичный отказ отмены Payment
возвращается клиенту и освобождает место без фоновой отмены Reservation.

Успешные шаги повторно не выполняются, UUID и ключ Loyalty сохраняются.
Синхронная компенсация использует отдельный контекст с таймаутом 5 секунд,
который не отменяется вместе с HTTP-запросом.

Очередь ограничена 256 задачами на общий Redis namespace и сохраняется после аварии Gateway. Идемпотентный
ключ Loyalty сохраняется вместе со счётчиком в одной транзакции; распределённой
атомарности и exactly-once между экземплярами Gateway нет.

Подход соответствует разделению `opstream` в BSDN: выдача операции (queue),
исполнение обработчиком (Processor), фиксация результата (queue.finish) и
управление жизненным циклом (Stream.Run и worker). Первая попытка и повторы
проходят через общий process, который вызывает обработчик, фиксирует результат
и пишет лог. В отличие от BSDN операции здесь остаются
в ограниченной очереди Redis, как допускает lab3. Processor измеряется
autometrics для синхронных и фоновых вызовов; фоновые логи содержат ключ операции,
тип и номер попытки. UUID не используются как метки метрик.
Circuit Breaker каждого downstream защищает чтения, а fallback формируется
в сценариях чтения; изменяющие запросы не проходят через breaker.

Gateway назначает UUID оплаты и бронирования и сохраняет компенсацию в Redis
до первого POST. Payment и Reservation принимают эти UUID во внутренних запросах:
повтор с тем же содержимым возвращает прежнюю запись, конфликт — 400. Отмена
с `Idempotency-Key`, равным UUID, сохраняет запрет позднего создания даже при
отсутствии записи. Обычная отмена неизвестного UUID без ключа сохраняет 404.
Loyalty связывает действия `:create` и `:rollback`: откат уменьшает счётчик
только при сохранённом увеличении и запрещает позднее увеличение после отката.
Ключи и изменения каждого сервиса фиксируются в локальной транзакции Ent.

## Проверки

```sh
make -C modules/platform check
make -C services/reservation check
make -C services/payment check
make -C services/loyalty check
make -C services/gateway check
make -C services/gateway e2e-test
```

Общие цели Make находятся в `scripts/go.mk`; локальные Makefile задают особенности
сервисов. `check` выполняется последовательно, включая вызов с `-j`; цель по
умолчанию — `build`. Для проверки файлов с тегами используйте, например,
`make -C services/gateway fmt-check lint TAGS=integration`.

Все Go-тесты запускаются через gotestsum. E2E самостоятельно строит четыре
образа и запускает изолированную сеть с PostgreSQL и случайными портами хоста.
Внешние сценарии выполняются через Gateway. Конкурентные изменения Loyalty
проверяются интеграционными тестами репозитория владельца данных.

Для проверки неизменённой Postman-коллекции нужны Newman и работающий Compose:

```sh
scripts/prepare-classroom.sh
newman run --folder success -e v2/postman/environment.json v2/postman/collection.json
scripts/test-script.sh v2 loyalty 8050
```

Подготовка явно загружает отель и пользователя `Test Max` с `GOLD/25/10`,
не перезаписывая существующие записи. Fixtures не входят в миграции.
Postman-коллекцию и environment можно импортировать в Postman вручную.
Workflow выполняет сборки, проверки, unit/e2e-тесты, запускает Compose
и Postman-коллекцию v2 через Newman. Удалённый результат появляется после push на GitHub.

## Refactoring and concurrency limits

Loyalty, Payment and Gateway use domain status types; Ent enums remain inside
storage. Gateway injects separate Reservation, Payment and Loyalty interfaces.
External JSON fields and enum values are unchanged.

Gateway requires explicit YAML settings for each downstream client:

```yaml
enrichment_concurrency: 4
clients:
  reservation:
    url: http://localhost:8070/api/v1
    timeout: 2s
    max_in_flight: 16
    max_idle_connections: 32
    max_idle_connections_per_host: 16
    idle_connection_timeout: 60s
  payment:
    url: http://localhost:8060/api/v1
    timeout: 2s
    max_in_flight: 8
    max_idle_connections: 16
    max_idle_connections_per_host: 8
    idle_connection_timeout: 60s
  loyalty:
    url: http://localhost:8050/api/v1
    timeout: 2s
    max_in_flight: 8
    max_idle_connections: 16
    max_idle_connections_per_host: 8
    idle_connection_timeout: 60s
```

All limits and timeouts must be positive. Up to four workers enrich one reservation list, preserving its order.
Each request shares one Hotel read per unique UUID through `sync.OnceValues`;
completed values and errors are retained only within that request. Each worker
obtains Hotel before reading Payment. A critical error cancels sibling
work, waits for it to finish and returns an error without a partial list.
Payment availability failures preserve the reservation with an empty payment object.
Reservation, Payment and Loyalty own separate HTTP clients and connection pools.
Their in-flight limits are 16, 8 and 8 respectively, for a total budget of 32.
A busy downstream cannot consume another client's slots; waiting callers respect
request cancellation. Idle connections close after HTTP shutdown and operation drain.
Creation and cancellation remain sequential; `/me` also remains sequential.
Limits apply per Gateway instance, not across replicas.

A unique Payment UUID index enforces idempotent creation; a nonunique Reservation
username index accelerates lookup of user bookings.
New Atlas migrations add them without changing existing migration files.
Loyalty counter updates retain PostgreSQL row locks and a transaction, so
independent connection pools cannot lose increments or split tier fields.

## Verification and diagnostics

```sh
make -C services/payment integration-test
make -C services/loyalty integration-test
make -C services/reservation integration-test
make -C services/gateway e2e-test
```

Payment and Loyalty integration targets create isolated PostgreSQL 13 containers
and apply SQL through the external `atlas` executable on PATH. Docker and Atlas
are required. Reservation integration tests retain their documented test database
and YAML configuration. All integration targets use gotestsum and `-race`.
Unit, integration and e2e targets write JUnit reports into each module's `bin/`.
CI uploads them; failed container tests print logs, and acceptance CI also uploads
Compose logs. `scripts/check-generated.sh` checks that committed Ent clients and
all service mocks match their generators; it compares against a working-source snapshot.
Atlas is pinned to the same immutable image in CI and Docker entrypoints.

Gateway e2e covers stopped upstream services and the resulting partial states:
a Reservation cancellation failure leaves Payment canceled, Reservation paid and Loyalty unchanged;
a Payment cancellation failure leaves Reservation and Payment paid and Loyalty
unchanged. A failed payment creation does not create a Reservation or increase
Loyalty. Loyalty cancellation failures are retried after recovery, without a
second decrement when DELETE is repeated. Unit tests also cover independent
compensation, resuming only outstanding steps, stale queue results, attempt
timeouts and worker shutdown.
Concurrent cancellation across Gateway instances still has no distributed
exactly-once guarantee; this refactoring does not introduce one.

The list benchmark uses a fixed 1 ms hotel lookup delay with immediate payment
responses. A local run on linux/amd64 with Intel Core Ultra 5 125H measured:

| Reservations | Workers | Time per list | Bytes per list | Allocations | Peak hotel calls |
| --- | --- | --- | --- | --- | --- |
| 1 | 1 | 1.11 ms | 1030 | 17 | 1 |
| 1 | 4 | 1.10 ms | 1027 | 17 | 1 |
| 10 | 1 | 10.78 ms | 5659 | 71 | 1 |
| 10 | 4 | 3.30 ms | 6379 | 78 | 4 |
| 50 | 1 | 54.25 ms | 26496 | 312 | 1 |
| 50 | 4 | 14.33 ms | 27042 | 319 | 4 |

This synthetic comparison isolates enrichment scheduling; it is not a production
latency guarantee. There is no timing threshold in CI. Run the benchmark through
`gotestsum -- -run '^$' -bench BenchmarkList -benchtime=100ms ./internal/core/usecases/bookings`
from the Gateway module. Gotestsum v1.12.3 may list benchmark events as incomplete
cases with Go 1.27; the benchmark output and underlying Go process exit status
remain the source of the measurements.

## Организация тестов

Unit-тесты используют `testing` и сгенерированные gomock зависимости.
`make generate` каждого сервиса обновляет его моки; версия mockgen закреплена
директивой `tool` в Go-модуле. Каждый сценарий имеет имя, собственный controller
и явные ожидания аргументов. Порядок вызовов проверяется там, где он задаёт
бизнес-последовательность создания или отмены бронирования.

Конкурентные unit-тесты Gateway используют `testing/synctest` и управляемые
каналы: они проверяют достижение лимита работников, сохранение порядка
результатов, отмену и завершение всех работников после ошибки. Задержка в
benchmark остаётся частью синтетической нагрузки. Зависимости benchmark возвращают
фиксированные ответы без gomock; накладные расходы обработки ожиданий в измерения не входят.

Интеграционные тесты Payment и Loyalty запускаются через `make integration-test`.
`scripts/integration-test.sh` создаёт изолированный PostgreSQL 13 на случайном
локальном порту, ждёт готовности и применяет Atlas-миграции до запуска gotestsum.
Параметры пула берутся из `TEST_CONFIG`, параметры Atlas — из `MIGRATION_CONFIG`.
Скрипт передаёт тестам временный YAML через `TEST_DATABASE_CONFIG`; Go только
открывает соединения. При ошибке выводятся логи БД, контейнер и временные файлы
удаляются при завершении. Reservation использует выбранный YAML и внешний
runner через Makefile.

Gateway E2E запускает один стек на родительский тест и выполняет именованные
сценарии последовательно. Каждый сценарий создаёт своего пользователя и свои
бронирования; сценарии отказов восстанавливают остановленные сервисы через
`Cleanup`. HTTP-проверки декодируют независимые структуры JSON и явно проверяют
коды ответов, поля и отсутствие внутренних идентификаторов.

### Panic и остановка Gateway

Каждый изменяющий шаг операции защищён от panic. Подтверждённые шаги
сохраняются; компенсация оплаты выполняется независимо от ошибки компенсации
бронирования. Дополнительная защита исполнителя освобождает задачу для следующей
попытки. Ошибка сохраняет причину и stack trace в атрибутах `oops`, при передаче этой ошибки в HTTP
возвращается 500. Такие ошибки не учитываются Circuit Breaker как отказ доступности.

При остановке Gateway сначала завершает HTTP-запросы, затем останавливает
фоновые workers и пытается выполнить оставшиеся задачи в течение
`opstream.shutdown_timeout` (20s в конфигурациях). Интервал повторов и таймаут
попытки сохраняются. После этого закрываются HTTP-клиент, метрики и логирование.
Docker Compose даёт Gateway 40s на остановку. При истечении срока ошибка
содержит число оставшихся операций. Очередь хранится в Redis: невыполненные
задачи остаются для следующего запуска или другого экземпляра Gateway.

HTTP-обработчики Gateway находятся в `internal/rest/bookings/handlers.go`,
регистрация маршрутов — в `register.go`, транспортные преобразования — в `dto.go`.
`Create` содержит весь порядок создания и сохраняет назначенные UUID до побочных эффектов. `List` содержит ограниченную параллельную обработку списка;
сбор ответа с fallback Payment общий для `List` и `Get`.

### Redis и восстановление операций

`docker compose up -d --build` запускает `operation-redis`. Конфигурация находится
в `redis/redis.conf`: AOF, appendfsync always, noeviction; данные хранятся
в `operation-data`. Все Gateway используют одинаковый YAML `redis.prefix` с
Redis Cluster hash tag. Redis является отдельной точкой отказа; HA не настроена.
При недоступности хранилища новое изменение отклоняется с 503 до побочных эффектов.

Компенсационный payload сохраняется до создания Payment и Reservation. Пустые
неиспользованные резервы освобождаются после истечения lease; подготовленные
задачи восстанавливаются после перезапуска Gateway/Redis. Локальные таблицы
`payment_requests` и `booking_requests` защищают создание от гонки с отменой;
это записи идемпотентности, очередь остаётся в Redis. Перед увеличением Loyalty Gateway добавляет в начало плана действие `ResolveCreationOperation`.
При восстановлении вызывает внутренний POST
`/api/v1/loyalty/reservations/{reservationUid}/resolve`. Под блокировкой строки
пользователя Loyalty проверяет запись `<UUID>:create`: сохранённое увеличение
подтверждает создание, и Gateway завершает задачу без отмены оплаты и бронирования.
Если увеличения нет, Loyalty сохраняет `<UUID>:rollback`, запрещая позднее
увеличение. Только после этого Gateway отменяет оплату и бронирование.
Недоступность Loyalty оставляет решение незавершённым и откладывает задачу;
worker не начинает отмену до получения решения. Повтор resolve возвращает
то же решение. Запрос изменяет состояние и не проходит через Circuit Breaker.

Общей транзакции между сервисами и Redis нет. Потеря HTTP-ответа по-прежнему
может оставить клиенту неизвестный результат; подтверждённое создание при этом
сохраняется после аварии Gateway.

Для просмотра состояния и оставшихся шагов:
`docker compose exec gateway gateway inspect-operation --config /app/config.yaml --key '<operationUuid>'`.
Команда выводит JSON с состоянием, владельцем, номером попытки, сроком lease,
ошибкой и расшифрованной задачей. После успешного создания payload очищается.
UUID предназначены для диагностики и не используются в метках метрик.

После устранения причины failed можно повторно выдать явно:
`docker compose exec gateway gateway retry-operation --config /app/config.yaml --key '<operationUuid>'`.


### План операции и формат Redis

`Operation` содержит `Kind`, `Key uuid.UUID` и оставшийся план `Steps`.
`OperationKey(kind, reservationUID)` детерминированно получает UUIDv8: повторы
одного действия имеют один ключ, создание и отмена одной брони — разные ключи.
Redis хранит строковое представление UUID только на границе адаптера.
Каждый элемент плана — `OperationAction[T]{Payload T}`.
Разные конкретные типы хранятся вместе через закрытый интерфейс `Action`: `Steps []Action`. UUID брони и оплаты,
пользователь и ключ Loyalty находятся в конкретном Go DTO, реализующем
закрытый интерфейс `OperationPayload`. В модели нет `json.RawMessage`, `any`,
JSON-тегов и маршалинга; DTO передаются как значения, без указателей:

| Тип | Payload |
| --- | --- |
| CancelPayment | PaymentUID |
| CancelReservation | User, ReservationUID |
| ResolveCreation | User, ReservationUID |
| RollbackLoyalty | RollbackLoyaltyOperation: User, OperationID |
| DecreaseLoyalty | DecreaseLoyaltyOperation: User, OperationID |

Подход повторяет BSDN: общий конверт операции, отдельные типы payload,
выбор типизированного обработчика и вызов usecase. В BSDN это
`history.Operation`, `database/operations` и `opstream/service.AddRoute`;
здесь — `OperationAction[T]`, типы в `entities/operation_payload.go` и
`operations.Actions.Execute`. UUID сохраняются для восстановления после
падения процесса, как и в payload BSDN; общая запись очереди их не знает.

`operations.Actions.Execute` проверяет действие, выбирает downstream-вызов через
`type switch` по конкретному `OperationAction[DTO]` и перехватывает panic
на границе каждого шага. Поля `Payload` передаются в узкие downstream-порты
без десериализации и реестра маршрутов.
`operations.Processor` управляет порядком группы, прогрессом и политикой повтора,
используя только интерфейс `operationifc.ActionExecutor`. Конкретные реализации
собираются вручную в `cmd/gateway`.

Отмена выполняет CancelPayment → CancelReservation → DecreaseLoyalty и
останавливается на первой ошибке. Компенсация сначала уточняет ResolveCreation,
если он есть, затем независимо выполняет CancelReservation, CancelPayment и
RollbackLoyalty. Подтверждённое увеличение Loyalty завершает компенсацию без
отмены оплаты и брони; недоступность Loyalty сохраняет весь план для повтора.

Валидация запрещает неизвестные шаги, повторы, неверный порядок,
некорректный payload и шаги чужого типа группы. Processor заменяет срез шагов,
не изменяя входные payload. CLI `inspect-operation` показывает для каждого
оставшегося действия строковый тип и DTO. JSON формируется только
для вывода CLI, а не для передачи данных между слоями.

Redis-адаптер кодирует каждый DTO в Gob
и восстанавливает конкретную структуру перед передачей usecase. Ни глобальная
регистрация Gob-типов, ни init не нужны.
Конверт и действия не содержат версий. Декодер проверяет тип действия и корректность Gob.
Старые форматы и строковые ключи не преобразуются. Store работает с UUID-ключами;
при открытии он создаёт consumer groups. Сообщения и pending entries сохраняются в AOF.

### Границы usecase и opstream

`operations.Groups` получает Stream, клиенты Reservation, Payment и Loyalty
и rollback timeout из `opstream.rollback_timeout` выбранного YAML через конструктор. Его внутренний метод `finish` создаёт контекст
`WithTimeout(WithoutCancel(requestCtx), timeout)` для компенсации.
`Groups.Create` перехватывает panic бизнес-шагов и преобразует её в
`ErrOperationPanic` (HTTP `500`), затем вызывает `finish`: сохранённый план
компенсируется с этим независимым таймаутом; пустая группа освобождается.
Stream передаёт контекст вызывающего кода исполнителю; собственные таймауты
Stream ограничивают работу с хранилищем и фоновые попытки.

Actions зависит от узких портов `reservationifc.Canceler`,
`paymentifc.Canceler` и `loyaltyifc.Updater`; Processor — от ActionExecutor. Создание оплаты использует
`entities.PaymentCreate` с обязательным PaymentUID и ценой. Совместимость
запросов без UUID сохраняется во внутреннем HTTP API Payment Service.

Правила Loyalty находятся в `internal/models/entities/change.go`:
`LoyaltyChange.Decide` вычисляет следующий статус и счётчик, проверяет повтор
и связанные действия; `CreationResolution.Decide` определяет результат создания.
Ent-репозиторий блокирует пользователя, загружает записи, вызывает доменное
решение и фиксирует изменения. Ключ и счётчик по-прежнему сохраняются одной
транзакцией. Запросы без ключа и произвольные ключи lab2 поддерживаются.

Downstream timeout равен 2s, общий HTTP timeout Gateway — 5s. Разница оставляет
время для формирования fallback после зависшего обращения. Таймаут попытки
и синхронного отката остаётся 5s.

### Регрессионные проверки восстановления

Gateway integration проверяет реальный Redis: отдельные бюджеты, повторные
ключи, истечение lease, fencing, удержание failed-задач,
ручной повтор, отдельную очередь Loyalty и восстановление AOF после перезапуска.
Unit-тесты проверяют запись и чтение текущего payload, отклонение неизвестных
типов действий и некорректного Gob и выбор политики повтора.
Loyalty integration проверяет гонку resolve с увеличением через два пула БД,
стабильность решения и откат без увеличения на фоне существующего счётчика.

Gateway E2E запускает изолированные сервисы и дополнительный процесс Gateway
с конфигурацией YAML. HTTP-прокси моделируют потерю ответа после COMMIT,
задержку до/после увеличения Loyalty и временную недоступность. Сценарии
проверяют отсутствие повтора POST Payment, сохранение подтверждённого
бронирования, запрет позднего увеличения и исполнение отложенной отмены
после SIGKILL и запуска нового Gateway. Workflow запускает эти проверки
через существующие цели integration-test и e2e-test.

### CQRS в Gateway

Пакет `core/usecases/bookings` содержит два независимых обработчика:

| Обработчик | Контракт REST → usecase | Зависимости | Результат |
| --- | --- | --- | --- |
| `Commands` | `coreifc.BookingCommands` | Reservation BookingSource, Payment Creator, Loyalty BookingSource, Stream | Создание / отмена |
| `Queries` | `coreifc.BookingQueries` | Только Reader каждого downstream | Page, Reservation, UserInfo, Loyalty |

`CreateBooking` содержит пользователя, отель и даты; `CancelBooking` — пользователя
и UUID бронирования. REST собирает команды из DTO и заголовка, затем напрямую
вызывает соответствующий обработчик. `Queries` собирает модели чтения для API,
проверяет принадлежность бронирования и применяет fallback. Его зависимости
не предоставляют методов изменения данных или доступа к очереди.

Обработчики собраны отдельно через `NewCommands` и `NewQueries` в `cmd/gateway`.
Они остаются внутри одного пакета предметной области и не вызывают друг друга.
Команды могут читать актуальные данные для проверки и расчёта стоимости;
разделение CQRS не запрещает такие чтения в сценарии изменения.

Ответы POST и DELETE остаются синхронными по контракту лабораторной. Saga
`operations` и очередь `opstream` восстанавливают незавершённые изменения.
Redis хранит операции, а модели чтения собираются из downstream по запросу;
отдельных проекций, Event Sourcing и отдельной БД чтения нет.

В Gateway E2E временная недоступность downstream моделируется Docker pause/unpause:
сетевой адрес сохраняется, запросы ограничены штатным таймаутом. Это отделяет
проверку fallback и отмены от переподключения Docker-сети. SIGKILL Gateway и
перезапуск Redis с AOF в тестах восстановления остаются настоящими перезапусками.
Приёмочный Newman использует исходный scripts/test-script.sh со Stop/Start Loyalty.

### Операция создания и исполнитель групп

`Commands.Create` содержит проверки и порядок бизнес-вызовов.
`entities.CreationOperation` содержит пользователя, UUID брони и оплаты и стабильный
ключ группы. `Group` создаёт пустую группу для резервирования; `Compensation`
строит типизированные отмены; `ResolveCompensation` добавляет проверку результата
Loyalty перед откатом. Изменение UUID по ответам downstream не меняет ключ очереди.

`operations.Groups` управляет жизненным циклом через `Create` и `Cancel`:

- `Create` резервирует бюджет до вызова бизнес-шагов и завершает операцию через `defer`.
- `operationifc.Creation` предоставляет создание оплаты, бронирования и увеличение
  лояльности. Реализация сохраняет план перед первым POST, обновляет UUID по ответам
  и добавляет ResolveCreation перед изменением Loyalty.
- Ошибка первого сохранения оставляет группу пустой; ошибка после побочных эффектов
  сохраняет новый план для немедленной компенсации.
- Завершение освобождает резерв при успехе либо запускает компенсацию вне отмены
  HTTP-запроса с таймаутом; ошибки объединяются с исходной ошибкой.
- `Cancel` проверяет незавершённое создание/компенсацию, читает актуальную бронь,
  проверяет незавершённую отмену, строит план, резервирует место и выполняет шаги. Принятое в очередь уменьшение Loyalty позволяет вернуть успешный ответ.

`Commands` зависит от `operationifc.Groups`, не управляет резервом и планом
компенсации. В нём остаются проверки, чтения, расчёт цены и порядок бизнес-шагов.
Зависимости собираются вручную в `cmd/gateway`. Подход следует разделению BSDN
между данными операции и сопровождением группы; атомарной транзакции между
HTTP-сервисами нет.

`OperationAction[T]` хранит конкретный DTO в `Payload T`. Общий интерфейс `Action`
нужен для смешанного списка шагов: `OperationAction[CancelPaymentOperation]` и
`OperationAction[CancelReservationOperation]` — разные типы Go.
`OperationData[T]` позволяет извлечь конкретный DTO с проверкой специализации; Actions передаёт `Payload` прямо из выбранной ветки `type switch`. Redis-адаптер восстанавливает нужную специализацию по типу
шага. Формат хранения — Gob, прежние форматы не поддерживаются.

### Упрощение операций

Действие определяется конкретным Go-типом DTO: конструктор принимает только данные,
например `NewOperationAction(CancelPaymentOperation{PaymentUID: id})`.
В Go-обёртке нет отдельного поля Type, которое могло бы противоречить payload.
В Redis поле Type остаётся частью формата хранения и используется для выбора DTO.
Его числовые коды приватны для Redis-адаптера.

Каждый DTO проверяет собственные поля в `Validate()`. Generic-обёртка вызывает проверку DTO, группа проверяет допустимость и порядок шагов.
`Processor.Execute` явно выбирает итоговый статус после исполнения; в `defer`
остаётся измерение метрик. Правила Rejected/Failed/Deferred не изменены.

Redis-адаптер кодирует и декодирует Gob-конверт без версий. JSON-декодеров и преобразования старых представлений нет.

В unit- и интеграционных тестах планы строятся непосредственно из типизированных
действий. Плоские `operationFixture`, преобразования `operationData` и повторные
конструкторы по списку enum удалены; оставшиеся шаги выбираются из исходного плана.

### Определение действий по Go-типам

`OperationStep`, методы `Step()` и enum-маршрутизация удалены из модели.
`Actions.Execute` использует `type switch` по конкретным `OperationAction[DTO]`.
`HasAction[T](task)` проверяет наличие конкретного типа. Корректность формата хранения
проверяется Redis-адаптером.
Порядок и допустимость действий проверяются по их Go-типам и виду группы;
локальные позиции порядка не являются идентификаторами хранения.

Redis-адаптер сохраняет приватный `actionCode` с прежними значениями:
1 — ResolveCreation, 2 — CancelPayment, 3 — CancelReservation,
4 — RollbackLoyalty, 5 — DecreaseLoyalty. Он преобразует коды в конкретные DTO
и обратно внутри формата Gob.
Имя Go-типа не используется как идентификатор хранения. CLI inspect показывает
конкретный тип DTO, например `entities.CancelPaymentOperation`; публичное HTTP API
не изменено.
