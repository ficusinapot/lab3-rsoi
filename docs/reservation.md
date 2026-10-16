# Reservation Service

Сервис хранит отели и бронирования в собственной PostgreSQL БД, слушает
порт `8070`. Payment и Loyalty вызывает Gateway, а не Reservation.

## Запуск

```sh
docker compose up --build -d
```

Docker entrypoint применяет миграции отдельным процессом Atlas, затем запускает
сервис. Atlas и yq включены в образ. Приложение не запускает миграции и не
содержит migration-команд.

Локально нужны Atlas CLI, mikefarah/yq v4, Go и PostgreSQL:

```sh
make -C services/reservation migrate
make -C services/reservation run
```

Все параметры приложения задаются в `config.yaml`, для Docker используются
`config.docker.yaml`, для интеграционных тестов `config.test.yaml`.
Cobra принимает `--config`, Viper отклоняет неизвестные ключи.
Настройки Atlas находятся отдельно в `migrations.yaml`: каталог SQL,
таймаут выполнения и ожидания блокировки. Внешний runner читает URL БД
из выбранного конфигурационного файла приложения через yq.

## Структура

`services/reservation` и `modules/platform` являются отдельными Go-модулями.
Внутри слоёв сервиса сценарии и адаптеры разделены по `hotels` и `bookings`:

- `internal/models/entities`: модели и ошибки без HTTP и Ent-тегов.
- `internal/models/coreifc` и `internal/models/dbifc`: контракты сценариев и БД.
- `internal/core/usecases`: бизнес-правила.
- `internal/postgres/repos` и `internal/postgres/converters`: Ent-адаптеры и преобразования.
- `internal/rest`: типизированные Huma DTO и обработчики на Chi.
- `cmd/reservation`: ручная сборка зависимостей; `internal/config` хранит импортируемый YAML-контракт.
- `ent/schema`: схемы; клиент Ent сгенерирован и хранится в репозитории.
- `migrations`: standalone Atlas SQL и `atlas.sum`.

Общий модуль содержит YAML CLI, PostgreSQL, REST, логирование и метрики.
Компоненты получают зависимости через конструкторы без DI-контейнера.

## Миграции

```sh
make -C services/reservation migrate-status
make -C services/reservation migrate-hash
make -C services/reservation migrations-check
make -C services/reservation generate
```

Миграции имеют формат `YYYYMMDDHHMM_description.sql`. Atlas хранит историю
в `atlas_schema_revisions`; `down` и seed отсутствуют. Начальная миграция
создает только таблицы, поле `end_data` сохраняется из README.
Прежняя БД требует сверки схемы и явного принятия начальной версии:

```sh
atlas migrate apply --dir file://services/reservation/migrations --url '<database.url из YAML>' --baseline 202609160001
```

`allow-dirty` автоматически не включается.

## API и наблюдаемость

| Метод | Путь | Назначение |
| --- | --- | --- |
| GET | `/manage/health` | Готовность БД |
| GET | `/api/v1/hotels` | Пагинация отелей |
| GET | `/api/v1/hotels/{hotelUid}` | Отель с ценой |
| GET | `/api/v1/reservations` | Бронирования пользователя |
| GET | `/api/v1/reservations/{reservationUid}` | Бронирование пользователя |
| POST | `/api/v1/reservations` | Создание, `201` и `Location` |
| DELETE | `/api/v1/reservations/{reservationUid}` | Отмена, `204` |
| GET | `/metrics` | Prometheus |
| GET | `/api/v1/docs` | Swagger UI |
| GET | `/api/v1/openapi.json` | Схема Huma |

Пользователь определяется `X-User-Name`; чужое бронирование возвращает `404`.
Внутренний POST содержит `hotelUid`, `paymentUid`, `startDate`, `endDate`.
Оплату создает Gateway. Пагинация и пути HTTP, OpenAPI, метрик задаются YAML.

Ошибки оформляются через `samber/oops`: постоянные сообщения, структурированные
атрибуты и wrapping причин. Клиенту не раскрываются SQL, credentials или стек.
Метрики: HTTP RED по шаблонам маршрутов, Go/process, пул БД и autometrics
операций. HTTP метрики имеют префикс `reservation_rest_api_`, метрики пула
`reservation_db_connections_`. Метки не содержат UUID и пользователей.

## Проверка

```sh
make -C modules/platform check
make -C services/reservation check
make -C services/reservation integration-test
```

E2E-проверки сами поднимают PostgreSQL и Reservation Service через
testcontainers, собирая сервис из `Dockerfile`:

```sh
make -C services/reservation e2e-test
```

Все тесты запускаются через gotestsum. Интеграционные проверки используют
отдельную БД `reservations_atlas` на порту `55432`; данные создаются тестом.
`integration-test` сначала применяет миграции внешним Atlas runner через yq.
`migrations-check` проверяет SQL и контрольные суммы без подключения к БД.
Правила lab1 находятся в `.golangci.yml` каждого модуля: gofumpt, linters,
`nolintlint` и `lll` (120 символов), generated Ent исключен.
Доступны `fmt`, `fmt-check`, `lint`, `lint-fix`, `style-check`, `tidy`, `vuln`.
