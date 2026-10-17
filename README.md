## Hotels Booking System

Реализация сервиса: [Reservation Service](docs/reservation.md).

Вызовы Gateway и восстановление после отказов: [диаграммы последовательности](docs/sequences.md).

Система предоставляет пользователю сервис поиска и бронирования отелей на интересующие даты. В зависимости от количества
заказов система лояльности дает скидку пользователю на новые бронирования.

### Структура Базы Данных

#### Reservation Service

Сервис запускается на порту 8070.

```sql
CREATE TABLE reservation
(
    id              SERIAL PRIMARY KEY,
    reservation_uid uuid UNIQUE NOT NULL,
    username        VARCHAR(80) NOT NULL,
    payment_uid     uuid        NOT NULL,
    hotel_id        INT REFERENCES hotels (id),
    status          VARCHAR(20) NOT NULL
        CHECK (status IN ('PAID', 'CANCELED')),
    start_date      TIMESTAMP WITH TIME ZONE,
    end_data        TIMESTAMP WITH TIME ZONE
);

CREATE TABLE hotels
(
    id        SERIAL PRIMARY KEY,
    hotel_uid uuid         NOT NULL UNIQUE,
    name      VARCHAR(255) NOT NULL,
    country   VARCHAR(80)  NOT NULL,
    city      VARCHAR(80)  NOT NULL,
    address   VARCHAR(255) NOT NULL,
    stars     INT,
    price     INT          NOT NULL
);
```

#### Payment Service

Сервис запускается на порту 8060.

```sql
CREATE TABLE payment
(
    id          SERIAL PRIMARY KEY,
    payment_uid uuid        NOT NULL,
    status      VARCHAR(20) NOT NULL
        CHECK (status IN ('PAID', 'CANCELED')),
    price       INT         NOT NULL
);
```

#### Loyalty Service

Сервис запускается на порту 8050.

```sql
CREATE TABLE loyalty
(
    id                SERIAL PRIMARY KEY,
    username          VARCHAR(80) NOT NULL UNIQUE,
    reservation_count INT         NOT NULL DEFAULT 0,
    status            VARCHAR(80) NOT NULL DEFAULT 'BRONZE'
        CHECK (status IN ('BRONZE', 'SILVER', 'GOLD')),
    discount          INT         NOT NULL
);
```

### Описание API

#### Получить список отелей

```http request
GET {{baseUrl}}/api/v1/hotels&page={{page}}&size={{size}}
```

#### Получить полную информацию о пользователе

Возвращается информация о бронированиях и статусе в системе лояльности.

```http request
GET {{baseUrl}}/api/v1/me
X-User-Name: {{username}}
```

#### Информация по всем бронированиям пользователя

```http request
GET {{baseUrl}}/api/v1/reservations
X-User-Name: {{username}}
```

#### Информация по конкретному бронированию

При запросе требуется проверить, что бронирование принадлежит пользователю.

```http request
GET {{baseUrl}}/api/v1/reservations/{{reservationUid}}
X-User-Name: {{username}}
```

#### Забронировать отель

Пользователь вызывает метод `GET {{baseUrl}}/api/v1/hotels` и выбирает нужный отель и в запросе на бронирование
передает:

* `hotelUid` (UUID отеля) – берется из запроса `/hotels`;
* `startDate` и `endDate` (дата начала и конца бронирования) – задается пользователем.

Система проверяет, что отель с таким `hotelUid` существует. Считаем что в отеле бесконечное количество мест.

Считается количество ночей (`endDate` – `startDate`), вычисляется общая сумма бронирования, выполняется обращение в
Loyalty Service и получается скидка в зависимости от статуса клиента:

* BRONZE – 5%
* SILVER – 7%
* GOLD – 10%

После применения скидки выполняется запрос в Payment Service и создается новая запись об оплате. После этого выполняется
обращение в сервис Loyalty Service, увеличивается счетчик бронирований. По-умолчанию у клиента статус `BRONZE`,
статус `SILVER` присваивается после 10 бронирований, `GOLD` после 20.

```http request
POST {{baseUrl}}/api/v1/reservations
Content-Type: application/json
X-User-Name: {{username}}

{
  "hotelUid": "049161bb-badd-4fa8-9d90-87c9a82b0668",
  "startDate": "2021-10-08",
  "endDate": "2021-10-11"
}
```

#### Отменить бронирование

* Статус бронирования помечается как `CANCELED`.
* В Payment Service запись об оплате помечается отмененной (статус `CANCELED`).
* Loyalty Service уменьшается счетчик бронирований. Так же возможно понижение статуса лояльности, если счетчик стал ниже
  границы уровня.

```http request
DELETE {{baseUrl}}/api/v1/reservations/{{reservationUid}}
X-User-Name: {{username}}
```

#### Получить информацию о статусе в программе лояльности

```http request
GET {{baseUrl}}/api/v1/loyalty
X-User-Name: {{username}}
```

Описание в формате [OpenAPI](%5Binst%5D%5Bv2%5D%20Hotels%20Booking%20System.yml).

### Данные для тестов

Создать данные для тестов:

```yaml
hotels:
  – id: 1
    hotelUid: "049161bb-badd-4fa8-9d90-87c9a82b0668"
    name: "Ararat Park Hyatt Moscow"
    country: "Россия"
    city: "Москва"
    address: "Неглинная ул., 4"
    stars: 5,
    price: 10000

loyalty:
  - id: 1
    username: "Test Max"
    reservation_count: 25
    status: "GOLD"
    discount: 10
```

## Реализация и запуск

Запуск четырёх сервисов, внутренние HTTP-контракты и проверки описаны
в [docs/services.md](docs/services.md). Детали Reservation —
в [docs/reservation.md](docs/reservation.md).

## Отказоустойчивость (lab3, v2)

Go module/import paths `lab2/*`, Ent/Atlas и ручная сборка зависимостей сохранены.
Gateway использует независимый breaker для Reservation, Payment и Loyalty
с циклическим массивом результатов завершённых чтений.
В YAML `breaker.failure_threshold: 5`, `breaker.reset_timeout: 1s`: первые пять
ошибок доступности достигают upstream, шестой запрос отклоняется локально;
после секунды допускается одна HalfOpen-проба. Успех возобновляет чтения.
400/404 и отмена входящего контекста не открывают breaker. Таймаут HTTP-клиента,
5xx, чтение и разбор невалидного ответа учитываются. Изменения обходят breaker.

Payment недоступен: GET бронирований и `/me` сохраняют данные с `payment={}`.
Loyalty недоступен: `/me` отдаёт `loyalty={}`, а GET `/loyalty` и POST
`/reservations` — 503 с `message=Loyalty Service unavailable`. Критичный
Reservation — 500. Здоровые ответы сохраняют `price=0` и `reservationCount=0`.

DELETE сначала отменяет оплату: при её отказе Reservation остаётся `PAID`.
После отмены Reservation временно неуспешное уменьшение Loyalty ставится
в очередь, ответ — 204. Повторные и конкурентные DELETE не создают второе
уменьшение. Ключ `reservationUUID:cancel` передаётся как `Idempotency-Key`;
Loyalty хранит его транзакционно со счётчиком. Повтор одинаковых пользователя
и delta успешен, конфликт — 400. Без ключа прямые вызовы работают как прежде.
При ошибке создания компенсируются известные UUID Payment и Reservation;
ошибка одного отката не препятствует попытке другого. Ошибки сохраняют причины.

HTTP timeout downstream задаётся YAML и составляет 2s; общий timeout запроса
Gateway — 5s. Это оставляет время для формирования fallback при зависшем сервисе.
Контекст компенсации создаёт usecase бронирования. Политику повтора возвращает
Processor; worker сохраняет результат и выполняет готовые задачи.

Конфигурация `opstream`: `capacity: 256`, `retry_interval: 10s`,
`attempt_timeout: 5s`, `rollback_timeout: 5s`. Все значения положительны.
Место резервируется до изменений; заполнение — 503 без побочных эффектов.
Worker повторяет только незавершённые шаги с независимым контекстом приложения.
Queue хранит и выдаёт задачи без обращения к сервисам; Processor исполняет
компенсацию и отмену как отдельные операции. Один Processor используется
синхронным сценарием и Worker. Результат попытки сохраняет оставшиеся шаги;
номер выдачи защищает новую задачу от устаревшего результата с тем же ключом.
Очередь хранится в Redis с AOF и отдельным Docker volume; подтверждённые задачи
сохраняются при перезапуске Gateway. Реляционная БД как очередь не используется.
UUID оплаты и бронирования назначаются и сохраняются до первого POST.
Идемпотентное создание и запрет позднего создания после отмены позволяют откатить
операцию при потере ответа. Loyalty откатывает только сохранённое увеличение.
Перед последним шагом сохраняется необходимость уточнить результат создания.
При восстановлении Loyalty атомарно подтверждает сохранённое увеличение либо
запрещает позднее увеличение. Подтверждённое создание сохраняется; иначе Gateway
выполняет компенсацию. Если Loyalty недоступен, решение откладывается.
Общей транзакции между сервисами и Redis нет; потеря HTTP-ответа может оставить
клиенту неизвестный результат.

Воспроизводимая проверка из корня проекта (Go, Atlas, yq, gotestsum,
golangci-lint, govulncheck, Docker и Newman должны быть установлены):

```sh
for module in modules/platform services/reservation services/payment services/loyalty services/gateway; do
  make -C "$module" check
done
for service in gateway loyalty reservation payment; do
  make -C "services/$service" integration-test
done
scripts/check-generated.sh
docker compose config --quiet
docker compose build
GATEWAY_E2E_PREBUILT=1 make -C services/gateway e2e-test
docker compose up -d
WAIT_PORTS=8080,8070,8060,8050 scripts/wait-script.sh
scripts/prepare-classroom.sh
newman run --folder success -e v2/postman/environment.json v2/postman/collection.json
scripts/test-script.sh v2 loyalty 8050
git diff --check
```

Failover-скрипт запускает оригинальные success/step1…step4, останавливает и
восстанавливает Loyalty на 8050, использует временный environment и перед step4
ждёт итогового счётчика до 30s. Assertions исходной коллекции не изменены.
Compose и изолированный e2e-стек используют образы `lab3-*`.

### CQRS в Gateway

Чтения выполняет `bookings.Queries` с интерфейсами Reader, создание и отмену —
`bookings.Commands` с командами `CreateBooking` и `CancelBooking`. REST получает
оба обработчика отдельно через конструкторы. Модели чтения собираются из HTTP
ответов сервисов; Redis хранит задачи Saga. Подробнее: [границы CQRS](docs/services.md#cqrs-в-gateway)
и [диаграмма](docs/sequences.md#6-разделение-команд-и-запросов).

### Иллюстрированный разбор запросов

[PDF: путь каждой ручки, вызовы сервисов, транзакции и восстановление](docs/gateway-request-flow.pdf)
— 23 страницы с диаграммами последовательности и стеком вызовов.
[Текстовая версия и Mermaid](docs/gateway-request-flow.md).
Пересборка: `python3 scripts/render-gateway-guide.py` (ReportLab, DejaVu Sans).
