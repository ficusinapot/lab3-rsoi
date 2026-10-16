#!/usr/bin/env python3
"""Render the repository's request-flow guide as vector PDF and Markdown.
Requires ReportLab; no network or running services are used.
"""
from __future__ import annotations

import datetime
import html
import math
from pathlib import Path

from reportlab.lib import colors
from reportlab.lib.enums import TA_CENTER, TA_LEFT
from reportlab.lib.pagesizes import A4, landscape
from reportlab.lib.styles import ParagraphStyle
from reportlab.pdfbase import pdfmetrics
from reportlab.pdfbase.ttfonts import TTFont
from reportlab.pdfgen import canvas
from reportlab.platypus import Paragraph, Table, TableStyle

ROOT = Path(__file__).resolve().parents[1]
OUT = ROOT / 'docs' / 'gateway-request-flow.pdf'
MARKDOWN = OUT.with_suffix('.md')
DATE = datetime.date.today().isoformat()
BLUE = '#2357A5'
TEAL = '#117C83'
PURPLE = '#7954A2'
ORANGE = '#B36B25'
INK = '#182A40'
MUTED = '#53657B'
LIGHT = '#EDF3F9'

PAGES = []

def page(title, subtitle, *, lanes=(), events=(), notes=(), stack=(), refs=(), table=None, kind='sequence'):
    PAGES.append(dict(title=title, subtitle=subtitle, lanes=lanes, events=events,
                      notes=notes, stack=stack, refs=refs, table=table, kind=kind))

page('Как проходит запрос', 'Hotels Booking System · Gateway · CQRS · Saga · Redis', kind='overview',
 notes=[
 'Разбор текущей реализации репозитория, а не абстрактной архитектуры. Семь внешних бизнес-ручек разобраны отдельно.',
 'Чтение: REST → Queries → HTTP-адаптеры → downstream → собственная БД → сборка ответа.',
 'Изменение: REST → Commands. Redis заранее хранит план восстановления; операции исполняются через Processor → Actions.',
 'R — UUID бронирования, P — UUID оплаты, H — UUID отеля. Kcreate / Kcancel — UUID задач, раздельные по виду операции. Это разные идентификаторы. Пользователь передаётся в X-User-Name.'
 ], refs=['services/gateway/cmd/gateway/run.go', 'services/gateway/internal/models/coreifc/bookings.go'])

page('Сначала — общий HTTP-путь', 'Что происходит до и после любого бизнес-сценария',
 lanes=['Клиент', 'Chi / middleware', 'Strict handler', 'Queries / Commands', 'HTTP-адаптер'],
 events=[
 (0,1,'HTTP-запрос на Gateway :8080'),
 (1,1,'Observability: маршрут, длительность, результат; request timeout 5s'),
 (1,2,'Выбор маршрута и декодирование UUID, дат, query и заголовков'),
 (2,2,'POST: Content-Type application/json; тело не более 4096 байт'),
 (2,3,'DTO → PageQuery / CreateBooking / CancelBooking / аргументы чтения'),
 (3,3,'Проверки сценария: пользователь, даты, пагинация, принадлежность'),
 (3,4,'Вызовы downstream; чтения проходят через Circuit Breaker'),
 (4,3,'Сущности или ошибка с сохранённой причиной'),
 (3,2,'Результат сценария; DTO формируется в rest/bookings/dto.go'),
 (2,0,'JSON + HTTP-статус; ошибки отображает serviceError()')],
 notes=[
 'Gateway разрешает в YAML только http.api_prefix=/api/v1. X-User-Name обязателен для пользовательских ручек; для списка отелей пользователь не нужен. Пустое имя, некорректный UTF-8 или длина > 80 символов отклоняются.',
 'Ошибка декодирования даёт 400 ещё до usecase. Ограничение request timeout может закончить запрос до выполнения всей цепочки.',
 'Queries и Commands собраны отдельно. Queries не имеет методов записи и доступа к очереди. Commands читает данные для расчёта и проверок.'
 ], stack=['cmd/gateway.Run', 'rest.Observability → middleware.Timeout', 'api.HandlerWithOptions → api.NewStrictHandlerWithOptions', 'handler.Get… / Post… / Delete…', 'BookingQueries / BookingCommands'],
 refs=['services/gateway/cmd/gateway/run.go','services/gateway/internal/rest/bookings/register.go','services/gateway/internal/rest/bookings/handlers.go'])

page('01 · Список отелей', 'GET /api/v1/hotels?page=1&size=10 · успешный ответ 200',
 lanes=['Клиент','Gateway / Queries','Reservation :8070','Ent → PostgreSQL'],
 events=[(0,1,'GET /api/v1/hotels?page=1&size=10'),
 (1,1,'Hotels(PageQuery): проверить page и size'),
 (1,2,'reservation.Repository.Hotels → GET /api/v1/hotels'),
 (2,2,'hotels.Service.List: применить YAML-пагинацию'),
 (2,3,'Read-only REPEATABLE READ: COUNT + список отелей'),
 (3,2,'Одна согласованная страница; ORDER BY id, OFFSET, LIMIT'),
 (2,1,'200: page, pageSize, totalElements, items'),
 (1,0,'200: pageResponse(result)')],
 notes=['Gateway принимает page ≥ 0; page=0 отправляет в Reservation как 1, а в ответе возвращает page=0. size в Gateway: 1..100.',
 'Если параметры отсутствуют, Reservation использует default_page=1 и default_size=10 из своего YAML. Лимит Reservation отдельно равен 1000.',
 'Отказ Reservation или открытый его breaker → 500 «Reservation Service unavailable». Payment, Loyalty и Redis не вызываются.'],
 stack=['handler.GetApiV1Hotels', 'Queries.Hotels', 'reservation.Repository.Hotels → httpclient.Read', 'contracts.GetHotelsWithResponse', 'Reservation: hotels.Service.List → hotels.Repository.List'],
 refs=['services/gateway/internal/core/usecases/bookings/read.go','services/gateway/internal/reservation/repos/bookings.go','services/reservation/internal/core/usecases/hotels/service.go','services/reservation/internal/postgres/repos/hotels/repository.go'])

page('02 · Статус лояльности', 'GET /api/v1/loyalty · X-User-Name: Анна · успешный ответ 200',
 lanes=['Клиент','Gateway / Queries','Loyalty :8050','Ent → PostgreSQL'],
 events=[(0,1,'GET /api/v1/loyalty; X-User-Name=Анна'),
 (1,1,'Loyalty(user): проверить имя пользователя'),
 (1,2,'loyalty.Repository.Loyalty → GET /api/v1/loyalty'),
 (2,2,'loyalties.Service.Get → Repository.Get'),
 (2,3,'ensure: INSERT нового пользователя ON CONFLICT IGNORE'),
 (2,3,'SELECT loyalty WHERE username = Анна'),
 (3,2,'reservationCount, status, discount'),
 (2,1,'200: статус пользователя'),
 (1,0,'200: loyaltyResponse(result)')],
 notes=['Новый пользователь получает reservationCount=0, BRONZE, discount=5. Даже GET может создать строку Loyalty при первом обращении — это существующее поведение сервиса.',
 'Для этой самостоятельной ручки fallback отсутствует: отказ Loyalty → 503, message «Loyalty Service unavailable».',
 'Reservation, Payment и Redis не вызываются. Нулевой счётчик здорового ответа сохраняется как 0, а не исчезает.'],
 stack=['handler.GetApiV1Loyalty', 'Queries.Loyalty → validateUser', 'loyalty.Repository.Loyalty → httpclient.Read', 'contracts.GetLoyaltyWithResponse', 'Loyalty: Service.Get → Repository.Get → ensure + SELECT'],
 refs=['services/gateway/internal/core/usecases/bookings/read.go','services/gateway/internal/loyalty/repos/loyalty.go','services/loyalty/internal/postgres/repos/loyalties.go'])

page('03 · Все бронирования пользователя', 'GET /api/v1/reservations · запрос списка и параллельное обогащение',
 lanes=['Клиент','Gateway / Queries','Reservation :8070','Payment :8060'],
 events=[(0,1,'GET /api/v1/reservations; X-User-Name=Анна'),
 (1,2,'Reservations(user) → GET /api/v1/reservations'),
 (2,1,'200: строки бронирований пользователя; порядок по id'),
 (1,1,'errgroup: максимум 4 задачи обогащения одновременно'),
 (1,2,'Для строки: Hotel(H) → GET /api/v1/hotels/H'),
 (2,1,'200: описание отеля; один завершённый запрос на уникальный H'),
 (1,3,'После Hotel: Payment(P) → GET /api/v1/payments/P'),
 (3,1,'200: оплата; при ошибке доступности — fallback payment={}'),
 (1,1,'Сохранить результат в исходную позицию; дождаться группы'),
 (1,0,'200: массив Reservation, включая CANCELED')],
 notes=['Кэш отеля существует только внутри одного вызова List: sync.OnceValues разделяет результат и ошибку для одинакового H. Между HTTP-запросами кэша нет.',
 'Hotel и Payment внутри одной строки вызываются последовательно; разные строки выполняются параллельно. Дедупликации Payment нет. N строк и U уникальных отелей: до 1+U+N чтений.',
 'Ошибка Reservation/Hotel прерывает сборку списка; ошибка Payment доступности обнуляет только его блок. Ошибки 400/404 и отмена контекста не маскируются.'],
 stack=['handler.GetApiV1Reservations', 'Queries.List → reservations.Reservations', 'errgroup.SetLimit(enrichment_concurrency)', 'reservations.Hotel → reservationWithHotel → payments.Payment', 'reservationResponses(items)'],
 refs=['services/gateway/internal/core/usecases/bookings/list.go','services/gateway/internal/core/usecases/bookings/read.go','services/reservation/internal/postgres/repos/bookings/repository.go'])

page('04 · Одно бронирование', 'GET /api/v1/reservations/{reservationUid} · принадлежность проверяется в Reservation',
 lanes=['Клиент','Gateway / Queries','Reservation :8070','Payment :8060'],
 events=[(0,1,'GET /api/v1/reservations/R; X-User-Name=Анна'),
 (1,2,'Reservation(R, user) → GET /api/v1/reservations/R'),
 (2,2,'Ent: WHERE reservation_uid=R AND username=Анна; WithHotel'),
 (2,1,'200: внутреннее бронирование с HotelUID и PaymentUID'),
 (1,2,'Hotel(H) → GET /api/v1/hotels/H'),
 (2,1,'200: полное описание отеля'),
 (1,3,'Payment(P) → GET /api/v1/payments/P'),
 (3,1,'200: оплата; либо ошибка доступности'),
 (1,1,'reservationWithHotel: собрать HotelInfo и Payment'),
 (1,0,'200: Reservation; при недоступном Payment его блок = {}')],
 notes=['Три последовательных downstream-чтения. Внутренний ответ Reservation содержит ссылку на отель; Gateway дополнительно получает его полное описание.',
 'Чужое или отсутствующее бронирование → 404. Проверка пользователя выполняется до чтения Payment. Неверный UUID пути → 400 при декодировании.',
 'Loyalty и Redis не участвуют. Если Reservation недоступен, частичный успешный ответ невозможен → 500.'],
 stack=['handler.GetApiV1ReservationsReservationUid', 'Queries.Get → reservations.Reservation', 'reservations.Hotel', 'Queries.reservationWithHotel → payments.Payment', 'reservationResponse(result)'],
 refs=['services/gateway/internal/core/usecases/bookings/read.go','services/reservation/internal/postgres/repos/bookings/repository.go','services/gateway/internal/rest/bookings/dto.go'])

page('05 · Личный кабинет', 'GET /api/v1/me · сначала список, затем Loyalty',
 lanes=['Клиент','Gateway / Queries','Reservation :8070','Payment :8060','Loyalty :8050'],
 events=[(0,1,'GET /api/v1/me; X-User-Name=Анна'),
 (1,1,'Me(user) вызывает List(user)'),
 (1,2,'Список бронирований + отели, как на странице 5'),
 (1,3,'Обогащение Payment; до 4 строк параллельно'),
 (2,1,'Список успешно собран; ошибка Reservation остановила бы Me'),
 (1,4,'Только после List: Loyalty(user) → GET /api/v1/loyalty'),
 (4,1,'200: Loyalty либо ошибка доступности'),
 (1,1,'Недоступный Loyalty → пустая модель → loyalty={}'),
 (1,0,'200: {reservations:[…], loyalty:{…}}')],
 notes=['Это последовательная композиция двух usecase-чтений: List → Loyalty. Между ними нет отдельной общей транзакции или общего снимка данных.',
 'Payment может отсутствовать у отдельных броней. Loyalty может отсутствовать целиком. Если оба некритичных сервиса недоступны, ответ всё ещё возможен при здоровом Reservation.',
 'Fallback действует только для ошибок доступности. Реальные нулевые значения здоровых моделей выводятся, 400/404 и отмена запроса не превращаются в пустой блок.'],
 stack=['handler.GetApiV1Me', 'Queries.Me', 'Queries.List → Hotel / Payment', 'Queries.Loyalty', 'UserInfo → userInfoResponse'],
 refs=['services/gateway/internal/core/usecases/bookings/read.go','services/gateway/internal/core/usecases/bookings/list.go','services/gateway/internal/rest/bookings/dto.go'])

page('06 · Создание: успешный путь', 'POST /api/v1/reservations · синхронная Saga · внешний ответ 200',
 lanes=['Клиент','Gateway / Commands','Redis / Stream','Reservation','Payment','Loyalty'],
 events=[(0,1,'POST {hotelUid:H, startDate, endDate}; X-User-Name'),
 (1,1,'Проверить имя, H и даты; назначить R; key=OperationKey(Compensation,R)'),
 (1,2,'ReserveOperation: занять бюджет компенсаций до изменений'),
 (1,3,'GET /hotels/H → 200; цена отеля'),
 (1,5,'GET /loyalty → 200; текущая скидка'),
 (1,1,'Цена = nights × hotelPrice × (100−discount) / 100; назначить P'),
 (1,2,'Prepare: сохранить отмену R, отмену P, откат Loyalty'),
 (1,4,'POST /payments {paymentUid:P, price} → 201 PAID'),
 (1,3,'POST /reservations {R,H,P,dates,user} → 201 PAID'),
 (1,2,'Prepare: добавить ResolveCreation первым шагом'),
 (1,5,'POST /loyalty/reservations; key=R:create → 200, count+1'),
 (1,2,'defer: ReleaseOperation(Kcreate), очистить payload'),
 (1,0,'200: Created — бронь, прежняя скидка и Payment')],
 notes=['Даты — календарные значения API. endDate должна быть позже startDate; стоимость вычисляется целочисленно, с защитой от переполнения.',
 'Запись оплаты и бронирования PAID появляется до увеличения Loyalty. Скидка берётся до count+1. Количество свободных номеров не ограничивается.',
 'Это несколько локальных транзакций разных сервисов. Ошибка Release тоже может повлиять на ответ; потерянный HTTP-ответ сам по себе не доказывает откат.'],
 stack=['handler.PostApiV1Reservations', 'Commands.Create(CreateBooking)', 'Groups.Create → Creation.CreatePayment / CreateReservation / IncreaseLoyalty', 'Payment.CreatePayment → Reservation.CreateReservation', 'Loyalty.ChangeLoyalty(+1) → ReleaseOperation'],
 refs=['services/gateway/internal/core/usecases/bookings/create.go','services/gateway/internal/core/usecases/operations/groups.go','services/gateway/internal/core/usecases/bookings/cost.go'])

page('Создание: если шаг не завершился', 'План сохранён до побочных эффектов; откат работает вне отменённого HTTP-контекста',
 lanes=['operations.Groups','Stream / Processor','Actions','Reservation','Payment','Loyalty'],
 events=[(0,0,'Ошибка после сохранения плана; например потерян ответ Payment'),
 (0,1,'finish: Execute(Compensation); WithoutCancel + timeout 5s'),
 (1,1,'Start в Redis; получить fencing token и проверить план'),
 (1,2,'Если есть ResolveCreation: сначала уточнить исход в Loyalty'),
 (2,5,'POST /loyalty/reservations/R/resolve → committed'),
 (1,1,'true: завершить без отмены; false: продолжить; ошибка: отложить'),
 (1,2,'CancelReservationOperation{User,R}'),
 (2,3,'DELETE /reservations/R; Idempotency-Key=R → 204'),
 (1,2,'CancelPaymentOperation{P}'),
 (2,4,'DELETE /payments/P; Idempotency-Key=P → 204'),
 (2,5,'DELETE /loyalty/reservations; key=R:rollback → 200'),
 (1,1,'Finish: удалить успешные шаги; сохранить только остаток')],
 notes=['Для ошибки до увеличения Loyalty шага ResolveCreation ещё может не быть. CancelReservation, CancelPayment и RollbackLoyalty пробуются независимо: отказ первого не отменяет попытку второго.',
 'Откат R:rollback уменьшает счётчик только если R:create действительно применился. Без увеличения записывается защита от позднего +1, чужие бронирования не уменьшаются.',
 'Groups.Create преобразует panic в ErrOperationPanic: сохранённый план проходит через finish и компенсацию вне HTTP-контекста; ответ 500. Без плана освобождается резерв. POST Payment не повторяется автоматически.'],
 stack=['operations.Groups.Create → finish (defer)', 'Stream.Execute → worker.process(Initial)', 'Processor.compensate → operations.Actions.Execute', 'Actions.Execute → прямые вызовы HTTP-портов', 'Store.Finish: Completed / Deferred / Failed'],
 refs=['services/gateway/internal/core/usecases/operations/groups.go','services/gateway/internal/core/usecases/operations/compensate.go','services/gateway/internal/core/usecases/operations/actions.go'])

page('07 · Отмена: успешный путь', 'DELETE /api/v1/reservations/{reservationUid} · внешний ответ 204',
 lanes=['Клиент','Gateway / Commands','Redis / Stream','Reservation','Payment','Loyalty'],
 events=[(0,1,'DELETE /api/v1/reservations/R; X-User-Name=Анна'),
 (1,2,'Groups.Cancel: PendingOperation(Kcreate); активна? → 503'),
 (1,3,'После проверки создания: GET /reservations/R; владелец и статус'),
 (1,1,'Уже CANCELED? Сразу 204, без новой задачи'),
 (1,2,'PendingOperation(Kcancel); проверить существующий план'),
 (1,2,'ReserveOperation: занять бюджет обычных отмен'),
 (1,2,'Execute → Start: сохранить план и получить token'),
 (2,4,'Processor → Actions: DELETE /payments/P → 204'),
 (2,3,'Только после Payment: DELETE /reservations/R → 204'),
 (2,5,'DELETE /loyalty/reservations; key=R:cancel → 200, count−1'),
 (2,2,'Finish(Completed); освободить место очереди'),
 (1,0,'204 No Content')],
 notes=['На диаграмме Stream обозначает цепочку worker.process → Processor.cancel → Actions; HTTP выполняют downstream-адаптеры Gateway, а не Redis.',
 'Отмена последовательна: Payment → Reservation → Loyalty. Это другой порядок и другая политика ошибок, чем у компенсации создания.',
 'Активное создание/восстановление → 503 без изменений. Затем читается актуальная бронь: отмена не обгоняет +1 Loyalty. Повтор R:cancel не уменьшает счётчик дважды.'],
 stack=['handler.DeleteApiV1ReservationsReservationUid', 'Commands.Cancel: validateUser → Groups.Cancel(user,R)', 'Groups.Cancel: pending(Kcreate) → Reservation → pending(Kcancel)', 'Groups.execute → reserve → Stream.Execute → Processor.cancel', 'Payment → Reservation → Loyalty → Store.Finish'],
 refs=['services/gateway/internal/core/usecases/bookings/cancel.go','services/gateway/internal/core/usecases/operations/cancel.go'])

page('Отмена: все важные развилки', 'Почему одинаковый DELETE может вернуть 204, 400, 404, 500, 502, 503 или 504', kind='cancel_branches',
 notes=['Повторный DELETE сначала читает Reservation. Если статус уже CANCELED, возвращает 204; ранее поставленная задача Loyalty продолжает выполняться независимо.',
 'Если текущая задача ещё содержит CancelPayment или CancelReservation, повтор получает ошибку Reservation unavailable. Если остался только Loyalty, запрос принимается без повторного списания.',
 '204 после отказа Loyalty означает принятую отложенную работу, а не уже изменённый счётчик. Для критичных шагов успешный ответ не подделывается.'],
 stack=['Groups.Cancel → entities.HasAction[T](result.Task)', 'Processor.Execute: Initial vs Background', 'Initial + Payment не отменён → Rejected', 'Deferred → план остаётся в Redis', 'serviceError → внешний статус'],
 table=[['Ситуация','Данные / очередь','Внешний результат'],
 ['Неверный ввод / чужая или отсутствующая бронь','Изменений нет','400 / 404'],
 ['Reservation недоступен при чтении','Изменений нет','500'],
 ['Redis недоступен / очередь заполнена','Бизнес-изменения ещё не начаты','503'],
 ['Первичный отказ отмены Payment','Reservation PAID; слот освобождается','Ошибка, обычно 502 / 504'],
 ['Payment отменён, Reservation не отменён','Остаток плана повторяется','Ошибка, обычно 500'],
 ['P и R отменены, Loyalty недоступен','Остаётся DecreaseLoyalty; очередь принята','204'],
 ['Все шаги выполнены / R уже CANCELED','Повторного уменьшения нет','204']],
 refs=['services/gateway/internal/core/usecases/bookings/cancel.go','services/gateway/internal/core/usecases/operations/processor.go','services/gateway/internal/rest/bookings/errors.go'])

page('Как исполняется операция', 'У очереди свой UUID-ключ; бизнес-данные находятся в конкретных Go DTO',
 lanes=['Stream / worker','Redis Store','Processor','Actions','Downstream'],
 events=[(0,1,'ReadyKeys(lane) каждые 100ms; затем Take(key)'),
 (1,0,'Operation + token + attempt; lease 30s'),
 (0,2,'Execute(task, Background); attempt timeout 5s'),
 (2,2,'Проверить тип группы, порядок и все payload'),
 (2,3,'Execute(Action): OperationAction[T]{Payload T}'),
 (3,3,'Validate → type switch OperationAction[DTO]'),
 (3,4,'payment.Repository.CancelPayment → DELETE /payments/P'),
 (4,3,'204 или ошибка; паника перехватывается на границе действия'),
 (3,2,'Результат действия; убрать подтверждённый шаг'),
 (2,0,'OperationResult: остаток + Completed / Deferred / Failed'),
 (0,1,'Finish(token, result) вне отменённого attempt-контекста')],
 notes=['Operation = {Kind, Key, Steps}. OperationAction[T] = {Payload T}; действие задаёт Go-тип DTO; Steps — []Action. Key — UUID задачи. DTO — Go-значение; только Redis-адаптер кодирует его в Gob. Конверт и действия не содержат версий.',
 'Каждому действию соответствует свой DTO: CancelPayment, CancelReservation, ResolveCreation, RollbackLoyalty, DecreaseLoyalty. Processor зависит от интерфейса ActionExecutor; реализации связывает cmd/gateway.',
 'Успешные действия не возвращаются в план. Повтор возможен, если действие выполнено, а Finish потерян; downstream защищает бизнес-эффект идемпотентностью.'],
 stack=['worker.runLane → runReady → execute', 'Store.Take → worker.process', 'Processor.Execute → cancel / compensate', 'operations.Actions.Execute: type switch по Go-типу', 'operations.Actions → HTTP repo → Store.Finish'],
 refs=['services/gateway/internal/core/services/opstream/worker.go','services/gateway/internal/core/usecases/operations/actions.go','services/gateway/internal/models/entities/operation_payload.go'])

page('Redis: состояния и планирование', 'Redis Streams выдаёт задачи; Go координирует записи через WATCH + MULTI/EXEC', kind='states',
 notes=['Общий бюджет — 256 активных задач: 128 компенсаций и 128 отмен. Failed для ручного повтора удерживает место; Rejected освобождает слот. В обоих случаях Redis state=failed; различается удержание задачи.',
 'Компенсации обслуживают 2 worker, отмены — 1, остаток только Loyalty — ещё 1. Общий параметр concurrency=4. Остаток отмены переводится в отдельную lane Loyalty.',
 'Redis AOF с appendfsync always и отдельным volume сохраняет очередь. В PostgreSQL хранятся бизнес-данные и записи идемпотентности, а не эмуляция очереди.'],
 stack=['Reserve → reserved (запись может ещё не иметь payload)', 'Prepare → сохранить план до первого POST', 'Start / Take → running с lease и token', 'Deferred → следующий срок через 10s', 'Failed → inspect-operation / retry-operation'],
 refs=['services/gateway/internal/redis/repos/transaction.go','services/gateway/internal/redis/repos/streams.go','redis/redis.conf','services/gateway/config.yaml'])

page('Redis Streams: публикация и выдача', 'Sorted sets задают задержку; consumer group workers отслеживает доставку',
 lanes=['Worker Gateway','Go Store','Redis due:*','Redis Streams'],
 events=[(0,1,'ReadyKeys(lane): получить готовые UUID по Redis TIME'),
 (1,2,'ZRANGE до текущего времени'),
 (1,1,'publish: WATCH записи задачи и stream; вычислить явный ID'),
 (1,3,'MULTI/EXEC: XADD ID + HSET message + ZREM due'),
 (1,3,'XAUTOCLAIM зависшего сообщения после lease; иначе XREADGROUP'),
 (1,3,'В той же транзакции: INCR stream:*:version'),
 (1,0,'UUID доставленной задачи'),
 (0,1,'Take: WATCH записи, stream и stream:*:version; проверить PEL'),
 (1,3,'MULTI/EXEC: XCLAIM + INCR version + запись owner/token/lease'),
 (0,1,'Finish: проверить владельца, token, lease и PEL consumer'),
 (1,3,'MULTI/EXEC: XACK + XDEL + INCR version; сохранить остаток')],
 notes=['Изменения Pending Entries List (PEL) сами не инвалидируют WATCH. Поэтому XREADGROUP, XAUTOCLAIM, XCLAIM и подтверждение увеличивают stream:*:version в той же транзакции; Take и Finish наблюдают за этой версией.',
 'Явный возрастающий ID позволяет атомарно записать сообщение и ссылку message в hash задачи. Finish при Deferred подтверждает старое сообщение и назначает новый срок в due:*.',
 'Store.update повторяет только redis.TxFailedErr, в пределах storage_timeout. Сетевые ошибки не повторяются автоматически: исход EXEC может быть неизвестен. Lua и EVAL не используются.'],
 stack=['ReadyKeys → publish → XAUTOCLAIM / XREADGROUP', 'Take → deliveryOwned → XCLAIM + HSET', 'Finish → acknowledge → schedule / remove'],
 refs=['services/gateway/internal/redis/repos/streams.go','services/gateway/internal/redis/repos/transaction.go','services/gateway/internal/redis/repos/attempts.go'])

page('Падение Gateway: lease и fencing', 'Новый экземпляр подхватывает сохранённую задачу; старый не может перезаписать её результат',
 lanes=['Gateway A','Redis','Gateway B','Downstream'],
 events=[(0,1,'Prepare(task); затем Start → token=41, lease 30s'),
 (0,3,'Выполнить часть действий; ответ или Finish может потеряться'),
 (0,0,'Процесс остановился / SIGKILL; в Redis остался план'),
 (2,1,'ReadyKeys / Take до истечения lease → выдачи нет'),
 (1,1,'Истёк lease; время берётся из Redis TIME'),
 (2,1,'Take(key) → новый owner, token=42, attempt+1'),
 (2,3,'Повтор оставшегося действия с тем же UUID / ключом'),
 (3,2,'Повтор распознаётся локальной транзакцией сервиса'),
 (0,1,'Поздний Finish(token=41) → отклонён'),
 (2,1,'Finish(token=42) → принят; сохранён новый остаток')],
 notes=['Fencing защищает фиксацию очереди, но не отменяет уже отправленный HTTP-запрос старого worker. Повторный бизнес-эффект ограничивают ключи и транзакции downstream.',
 'Если процесс умер после Reserve, но до Prepare, задача без payload после истечения lease освобождается: побочные эффекты ещё не должны были начаться.',
 'Гарантии общей распределённой атомарности нет. Клиент также может не знать итог при потерянном ответе, даже если данные уже зафиксированы.'],
 stack=['Store.update: WATCH записи → проверки → MULTI/EXEC', 'Take: проверить PEL consumer, назначить token и attempt+1', 'Finish: owner + lease + token + state=running', 'Payment/Reservation guards + Loyalty operation key', 'Gateway при shutdown останавливает worker и вызывает Drain'],
 refs=['services/gateway/internal/redis/repos/attempts.go','services/gateway/internal/core/services/opstream/worker.go','services/gateway/cmd/gateway/run.go'])

page('Неоднозначный исход создания', 'Что делать, если увеличение Loyalty закоммитилось, а ответ потерян',
 lanes=['Recovery / Actions','Loyalty usecase','Ent-транзакция','Loyalty DB'],
 events=[(0,1,'ResolveCreationOperation{User, ReservationUID:R}'),
 (1,2,'ResolveCreation: BEGIN; ensure пользователя'),
 (2,3,'SELECT loyalty FOR UPDATE — общий lock с Change'),
 (2,3,'Найти запись операции R:create'),
 (2,2,'Есть +1 нужного пользователя → committed=true'),
 (2,2,'Нет +1 → записать guard R:rollback без уменьшения счётчика'),
 (2,3,'COMMIT решения'),
 (1,0,'200 {committed:true/false}'),
 (0,0,'true: оставить P/R PAID; false: отменять; ошибка: ждать'),
 (1,2,'Поздний R:create после guard → отклонение, без +1')],
 notes=['ResolveCreation — изменяющий POST: он может записать запрет позднего создания. Он не защищается read-breaker и не является простым GET состояния.',
 'Общий lock пользователя сериализует гонку ResolveCreation и Change(+1). Решение определяется зафиксированной записью R:create, а не предположением по таймауту HTTP.',
 'Если Loyalty недоступен, компенсация с ResolveCreation не отменяет Payment и Reservation вслепую: весь план ждёт следующей попытки.'],
 stack=['Actions.Execute: ResolveCreationOperation', 'loyalty.Repository.ResolveCreation (HTTP)', 'Loyalty Service.ResolveCreation', 'postgres.Repository.ResolveCreation', 'CreationResolution.Decide под row lock'],
 refs=['services/gateway/internal/core/usecases/operations/compensate.go','services/loyalty/internal/postgres/repos/loyalties.go','services/loyalty/internal/models/entities/change.go'])

page('Внутренние ручки Reservation', 'Эти ручки вызывает Gateway; префикс /api/v1; сервис работает со своей логической БД', kind='catalog',
 table=[['Метод / путь','Стек внутри Reservation','Работа с собственной БД'],
 ['GET /hotels','rest/hotels → Service.List → Repository.List','REPEATABLE READ: COUNT + ORDER BY id / OFFSET / LIMIT'],
 ['GET /hotels/{hotelUid}','rest/hotels → Service.Get → Repository.Get','Поиск hotels по hotel_uid; отсутствие → 404'],
 ['GET /reservations','rest/bookings → Service.List → Repository.List','username=user; WithHotel; ORDER BY id'],
 ['GET /reservations/{reservationUid}','rest/bookings → Service.Get → Repository.Get','reservation_uid + username; чужая бронь → 404'],
 ['POST /reservations','rest/bookings → Service.Create → Repository.Create','Транзакция guard + сверка повтора + отель + INSERT PAID; 201'],
 ['DELETE /reservations/{reservationUid}','rest/bookings → Service.CancelKnown / Cancel','guard canceled + UPDATE CANCELED; 204']],
 notes=['Create: claim BookingRequest(R,user), FOR UPDATE, проверить canceled/user, найти существующую бронь. Совпадающий повтор возвращает её; другой payload или статус → 400.',
 'Новая бронь: найти отель в той же логической БД, создать reservation PAID со ссылкой на hotels.id, зафиксировать транзакцию. Reservation сам не вызывает Payment и Loyalty.',
 'Gateway посылает DELETE с Idempotency-Key=R: CancelKnown сохраняет tombstone даже при ещё отсутствующей броне. Поздний POST с тем же R отклоняется. Без ключа сохранена legacy-семантика 404.'],
 stack=['Huma на Chi → REST DTO', 'coreifc.Bookings / Hotels → usecases', 'dbifc → Ent repositories', 'reservation + hotels + BookingRequest', 'COMMIT → сущность → REST DTO'],
 refs=['services/reservation/internal/rest/bookings','services/reservation/internal/rest/hotels','services/reservation/internal/postgres/repos/bookings/create.go','services/reservation/internal/postgres/repos/bookings/repository.go'])

page('Внутренние ручки Payment', 'GET — чтение; POST и DELETE используют request guard и локальную транзакцию',
 lanes=['Gateway','Payment REST','Service','Ent / Payment DB'],
 events=[(0,1,'POST /api/v1/payments {paymentUid:P, price}'),
 (1,2,'CreateWithID(P, price); без UUID — legacy Create'),
 (2,3,'BEGIN; claim PaymentRequest(P); FOR UPDATE'),
 (3,3,'canceled guard? → 400; существующая PAID с той же ценой? → повтор'),
 (3,3,'Иначе INSERT payment(P, PAID, price); COMMIT'),
 (1,0,'201: paymentUid, status, price'),
 (0,1,'GET /api/v1/payments/P → Service.Get → Repository.Get'),
 (1,0,'200: Payment; отсутствующая оплата → 404'),
 (0,1,'DELETE /api/v1/payments/P; Idempotency-Key=P'),
 (1,3,'CancelKnown: guard canceled + UPDATE payment CANCELED; COMMIT'),
 (1,0,'204; отсутствие записи не мешает установить tombstone')],
 notes=['Gateway заранее назначает P и проверяет совпадение UUID в ответе. Автоматического повтора POST Payment нет.',
 'DELETE с неправильным ключом → 400. DELETE без ключа вызывает Cancel: сначала проверяет существование, поэтому неизвестный P → 404.',
 'Это модель записи оплаты для лабораторной. Внешний платёжный провайдер в данном стеке не участвует. Payment не вызывает Reservation или Loyalty.'],
 stack=['rest/payments.Register (Huma)', 'payments.Service.CreateWithID / Get / CancelKnown', 'postgres.Repository.Create / Get / CancelKnown', 'PaymentRequest + payment, одна транзакция', 'Gateway contracts.ClientWithResponses проверяет статус'],
 refs=['services/payment/internal/rest/payments/register.go','services/payment/internal/postgres/repos/payments.go','services/gateway/internal/payment/repos/payments.go'])

page('Внутренние ручки Loyalty', 'Одна транзакция фиксирует ключ операции, счётчик, статус и скидку', kind='catalog',
 table=[['Метод / путь','Сценарий','Что фиксируется'],
 ['GET /loyalty','Service.Get → Repository.Get','ensure пользователя + SELECT; новый: 0 / BRONZE / 5'],
 ['POST /loyalty/reservations','Service.Change(delta=+1)','R:create: count+1, статус и скидка; 200'],
 ['DELETE /loyalty/reservations','Service.Change(delta=−1)','R:cancel или R:rollback; разные правила отката; 200'],
 ['POST /loyalty/reservations/{R}/resolve','Service.ResolveCreation','Зафиксировать исход создания, вернуть committed; 200']],
 notes=['Change: BEGIN → ensure → SELECT пользователя FOR UPDATE → прочитать связанный ключ → claim текущего ключа → LoyaltyChange.Decide → UPDATE → COMMIT.',
 'Повтор с тем же ключом и теми же user/delta возвращает текущее состояние без повторного изменения. Конфликт содержания ключа → 400. Запрос без ключа выполняется как legacy-изменение.',
 'R:rollback проверяет R:create: если увеличения не было, реальный delta=0; guard записывается. R:cancel уменьшает count на 1, но не ниже 0.',
 'Статус рассчитывается по новому счётчику: <10 BRONZE/5%; 10..19 SILVER/7%; ≥20 GOLD/10%. Правила находятся в entities, блокировки и фиксация — в Ent-репозитории.'],
 stack=['rest/loyalties.Register', 'Service.Change → NewLoyaltyChange', 'Repository.Change: транзакция + FOR UPDATE', 'LoyaltyChange.Decide(current, applied, related)', 'COMMIT key + count + status + discount'],
 refs=['services/loyalty/internal/rest/loyalties/register.go','services/loyalty/internal/models/entities/change.go','services/loyalty/internal/postgres/repos/loyalties.go'])

page('Circuit Breaker и fallback', 'У каждого downstream свой breaker; он охватывает законченные чтения', kind='breaker',
 notes=['Closed: пять последовательных ошибок доступности открывают breaker. Open: в течение 1s чтения сразу получают unavailable, без сетевого вызова. Half-open: допускается одна проба.',
 'Успешная проба закрывает breaker; новая ошибка доступности снова открывает. Исключены отмена клиентом и истечение его контекста; 400/404 не считаются ошибкой доступности.',
 'Успех определяется после чтения и разбора ответа, а не только получения HTTP-заголовков. Записи POST/DELETE, включая ResolveCreation, не проходят через breaker.',
 'Fallback — пустой JSON-блок, а не чтение старого значения из Redis. При неотменённом контексте он применяется только к availability-ошибкам.'],
 stack=['Repository.Payment / Loyalty / Hotel / Reservation / Hotels', 'httpclient.Read → breaker.Allow', 'Вызов HTTP + проверка статуса + декодирование', 'done(success / availability failure / excluded)', 'Queries решает: fallback или ошибка'],
 refs=['services/gateway/internal/httpclient/breaker.go','services/gateway/internal/models/entities/availability.go','services/gateway/internal/core/usecases/bookings/read.go'])

page('Статусы и ограничения', 'Как ошибка превращается в ответ Gateway; параметры взяты из YAML', kind='catalog',
 table=[['Условие','Ответ / правило','Где применяется'],
 ['Неверный ввод / отсутствующий объект','400 / 404','Валидация REST и usecase; ownership в Reservation'],
 ['Reservation недоступен','500: Reservation Service unavailable','Все критичные чтения и сценарии изменений'],
 ['Loyalty недоступен','503: Loyalty Service unavailable','Самостоятельное GET и создание бронирования'],
 ['Payment недоступен при чтении','200 с payment={}','GET reservations, GET reservations/R, GET me'],
 ['Loyalty недоступен в /me','200 с loyalty={}','Только если список удалось собрать'],
 ['Redis недоступен / занят / полон','503','До побочных эффектов резервируется слот'],
 ['Прочий downstream отказ / timeout','Обычно 502 / 504','Если более приоритетная service-ошибка не определена'],
 ['HTTP Gateway / downstream','5s / 2s','2s на одно обращение, общий запрос ограничен 5s'],
 ['Параллелизм / in-flight','4 строки; Reservation 16, Payment 8, Loyalty 8','Ограничены worker обогащения и HTTP-клиенты']],
 notes=['Mapper проверяет errors.Is в определённом порядке. Поэтому критичный Reservation timeout обычно становится 500, а Loyalty timeout — 503, а не универсальным 504.',
 'Пять секунд — общий request timeout, а не обещание, что последовательность из нескольких двухсекундных вызовов всегда уложится. Detached-компенсация отдельно ограничена 5s.',
 'При недоступности Payment и Loyalty здоровые нулевые поля не исчезают. Статус неизвестного блока определяет преобразование DTO, а не omitempty по цене или счётчику.'],
 stack=['serviceError: panic → invalid → not found', '→ Loyalty → Reservation → очередь', '→ timeout → upstream unavailable', 'dto.paymentResponse / loyaltyResponse', 'YAML определяет лимиты и таймауты'],
 refs=['services/gateway/internal/rest/bookings/errors.go','services/gateway/internal/rest/bookings/dto.go','services/gateway/config.yaml'])

page('Служебные ручки', 'Health, метрики и документация не запускают бронирование или Saga', kind='catalog',
 table=[['Ручка Gateway','Что происходит после запроса','Что получает клиент'],
 ['GET /manage/health','Chi handler выставляет статус; downstream не опрашиваются','200, без тела'],
 ['GET /metrics','telemetry.Handler собирает Prometheus collectors','Метрики HTTP, Go/process и операций; без user/UUID labels'],
 ['GET /api/v1/docs','Huma публикует Swagger UI для сгенерированного API','HTML интерфейс документации'],
 ['GET /api/v1/openapi.json','JSON схемы из api.GetSpec, сериализованной при запуске','OpenAPI-контракт Gateway']],
 notes=['Текущие пути взяты из YAML. Документация и метрики могут быть переименованы конфигурацией. Здоровье Gateway не означает доступность Reservation, Payment или Loyalty.',
 'Каждый downstream также имеет GET /manage/health. Служебные endpoints downstream регистрируются общим infrastructure-кодом; конкретные docs/metrics пути задаются YAML сервиса.',
 'Docker entrypoints принимают --config PATH и --config=PATH в любом месте до --; Atlas и приложение используют один YAML. Store работает с UUID-ключами и Gob-конвертом без версий. Преобразования старых данных нет. При запуске собираются клиенты, три breaker, Actions, Processor, Redis Store, Stream, Commands и Queries. Worker стартует рядом с HTTP-сервером.',
 'При завершении HTTP-сервер прекращает приём и завершает запросы; затем останавливаются worker, вызывается Drain с лимитом 20s, закрываются Redis и остальные ресурсы.'],
 stack=['cmd/gateway.Run', 'system.Register → api.GetSpec / Huma / telemetry', 'router.Get("/manage/health", …)', 'rest.Serve → server.Shutdown', 'stopStream → stream.Run finished → Drain'],
 refs=['services/gateway/internal/rest/system/register.go','services/gateway/cmd/gateway/run.go','modules/platform/rest/server.go'])

page('Конкретный пример: Анна', 'Три ночи × 10 000 ₽; начальный счётчик 9; читаем, создаём и отменяем', kind='example',
 notes=['1. GET /loyalty: count=9, BRONZE, discount=5. GET /hotels возвращает отель Ararat Park Hyatt Moscow с ценой 10 000.',
 '2. POST с 2026-10-20 по 2026-10-23: nights=3; цена 30 000×95/100=28 500. Созданы Payment P и Reservation R со статусом PAID.',
 '3. R:create увеличивает счётчик до 10: теперь SILVER/7%. Ответ этого создания всё ещё содержит discount=5 — именно эта скидка применена к оплате.',
 '4. GET /me собирает R + Hotel + Payment(28 500), затем текущее Loyalty(10, SILVER, 7). Скидка в лояльности уже может отличаться от скидки прошлого бронирования.',
 '5. DELETE R: P→CANCELED, затем R→CANCELED, затем R:cancel: count 10→9, BRONZE/5. Повтор DELETE даёт 204 без второго уменьшения.',
 '6. Если Loyalty выключен только при отмене, DELETE даёт 204 после принятия остатка задачи. Счётчик изменится после восстановления сервиса и повтора worker.'
 ],
 stack=['POST /api/v1/reservations', 'X-User-Name: Анна', 'Content-Type: application/json', '{"hotelUid":"049161bb-badd-4fa8-9d90-87c9a82b0668",', ' "startDate":"2026-10-20", "endDate":"2026-10-23"}', 'Проверка результата: GET /api/v1/me'],
 refs=['services/gateway/internal/core/usecases/bookings/create.go','services/loyalty/internal/models/entities/loyalty.go','services/gateway/internal/core/usecases/bookings/cancel.go'])

page('Куда смотреть в коде', 'Навигатор по реализации и границы описания', kind='sources',
 table=[['Вопрос','Файл / каталог'],
 ['Как собраны зависимости и жизненный цикл','services/gateway/cmd/gateway/run.go'],
 ['HTTP → DTO → конкретный usecase','services/gateway/internal/rest/bookings/handlers.go'],
 ['Чтение списка, одной брони, /me, отелей и Loyalty','services/gateway/internal/core/usecases/bookings/{read,list}.go'],
 ['Создание и отмена','services/gateway/internal/core/usecases/bookings/{create,cancel}.go'],
 ['Фоновый запуск и фиксация','services/gateway/internal/core/services/opstream/{stream,worker}.go'],
 ['План, порядок и политика повторов','services/gateway/internal/core/usecases/operations'],
 ['Типы payload / маршрутизация','services/gateway/internal/models/entities/operation_payload.go; internal/core/usecases/operations/actions.go'],
 ['Выдача Streams, транзакции и fencing','services/gateway/internal/redis/repos/{streams,transaction,attempts}.go'],
 ['Транзакции, guards и счётчик','services/{reservation,payment,loyalty}/internal/postgres/repos'],
 ['Лимиты и контракты','services/gateway/config.yaml; internal/clients/contracts/openapi.yaml']],
 notes=['Схемы отражают исходный код на дату сборки документа. Они описывают возможные вызовы и ветвления, а не фактическую трассу конкретного запущенного запроса.',
 'Гарантии: отдельные локальные транзакции, сохранённый план, идемпотентные действия и fencing фиксации очереди. Общей транзакции Redis + три БД и гарантии exactly-once доставки HTTP нет.',
 'Изменения файлов приложения этим документом не требуются. Пересборка PDF и Markdown: python3 scripts/render-gateway-guide.py (нужен ReportLab и шрифт DejaVu Sans).'],
 refs=['docs/services.md','docs/sequences.md','scripts/render-gateway-guide.py'])

# ---------- Layout: vector drawing, embedded Unicode fonts, predictable page boundaries. ----------
FONTDIR = Path('/usr/share/fonts/truetype/dejavu')
for name, filename in [('Body','DejaVuSans.ttf'), ('Bold','DejaVuSans-Bold.ttf')]:
    pdfmetrics.registerFont(TTFont(name, str(FONTDIR / filename)))

W, H = landscape(A4)
M = 34
CW = W - 2*M
C = None
OVERFLOWS = []


def para(text, x, top, width, size=9.1, leading=None, color=INK, bold=False, align=TA_LEFT):
    style = ParagraphStyle('guide', fontName='Bold' if bold else 'Body', fontSize=size,
                           leading=leading or size*1.42, textColor=colors.HexColor(color), alignment=align,
                           splitLongWords=True, spaceAfter=0)
    p = Paragraph(html.escape(str(text)).replace('\n','<br/>'), style)
    _, height = p.wrap(width, 2000)
    p.drawOn(C, x, H-top-height)
    if top < H-26 and top+height > H-26:
        OVERFLOWS.append((C.getPageNumber(), text[:60], round(top+height,1)))
    return top+height


def rounded(x, top, width, height, fill=LIGHT, stroke=None, radius=6):
    C.setFillColor(colors.HexColor(fill))
    C.setStrokeColor(colors.HexColor(stroke or fill))
    C.roundRect(x, H-top-height, width, height, radius, fill=1, stroke=bool(stroke))


def arrow(x1, top1, x2, top2, color=BLUE, dashed=False):
    C.saveState()
    C.setStrokeColor(colors.HexColor(color)); C.setFillColor(colors.HexColor(color)); C.setLineWidth(1)
    if dashed: C.setDash(3,3)
    y1,y2=H-top1,H-top2
    C.line(x1,y1,x2,y2); C.setDash()
    angle=math.atan2(y2-y1,x2-x1); length=5
    path=C.beginPath();path.moveTo(x2,y2)
    path.lineTo(x2-length*math.cos(angle-.5),y2-length*math.sin(angle-.5))
    path.lineTo(x2-length*math.cos(angle+.5),y2-length*math.sin(angle+.5));path.close()
    C.drawPath(path,fill=1,stroke=0);C.restoreState()


def box(text,x,top,width=150,height=44,fill=LIGHT,color=INK):
    rounded(x,top,width,height,fill)
    para(text,x+7,top+8,width-14,9.3,color=color,bold=True,align=TA_CENTER)


def header(index, data):
    C.setFillColor(colors.HexColor(BLUE));C.rect(0,H-7,W,7,fill=1,stroke=0)
    para('HOTELS BOOKING SYSTEM  /  РАЗБОР ЗАПРОСОВ',M,23,CW,7.8,color=MUTED,bold=True)
    para(data['title'],M,44,CW,21,bold=True)
    para(data['subtitle'],M,77,CW,9.8,color=MUTED)
    C.setStrokeColor(colors.HexColor('#D8E1EC'));C.line(M,25,W-M,25)
    para('Исходный код · '+DATE+'   |   R: бронь · P: оплата · H: отель',M,H-20,CW-50,7.2,color=MUTED)
    para(f'{index:02d} / {len(PAGES):02d}',W-M-48,H-20,48,7.6,color=MUTED,align=TA_CENTER)
    C.bookmarkPage(f'page-{index}')
    C.addOutlineEntry(data['title'],f'page-{index}',level=0)


def sequence(data, top=110):
    lanes=data['lanes'];events=data['events'];left=M+70; usable=CW-140
    positions=[left+i*usable/(len(lanes)-1) for i in range(len(lanes))]
    row=23.0
    end=top+32+row*len(events)
    for i,(name,x) in enumerate(zip(lanes,positions)):
        width=min(124,usable/max(1,len(lanes)-1)-8)
        width=max(99,width)
        rounded(x-width/2,top,width,27,fill=LIGHT)
        para(name,x-width/2+4,top+5,width-8,8.2,bold=True,align=TA_CENTER)
        C.saveState();C.setStrokeColor(colors.HexColor('#C0CCDB'));C.setDash(2,3)
        C.line(x,H-top-29,x,H-end+3);C.restoreState()
    for i,(a,b,label) in enumerate(events):
        y=top+34+i*row
        x1,x2=positions[a],positions[b]
        C.setFillColor(colors.HexColor(BLUE));C.circle(M+3,H-y-11,6.5,fill=1,stroke=0)
        para(str(i+1),M-3,y+6,12,6.7,color='#FFFFFF',bold=True,align=TA_CENTER)
        if a==b:
            x2=x1+23
            arrow(x1,y+18,x2,y+18,color=TEAL)
            arrow(x2,y+18,x1,y+22,color=TEAL)
        else:
            arrow(x1,y+19,x2,y+19,color=BLUE)
        label_width=min(420,CW-75)
        center=(positions[a]+positions[b])/2
        lx=max(M+18,min(center-label_width/2,W-M-label_width))
        # Opaque text band keeps lifelines from crossing letters.
        rounded(lx,y-1,label_width,18,fill='#FFFFFF',radius=2)
        para(label,lx+2,y-1,label_width-4,8.15,leading=9.2,align=TA_CENTER)
    return end+10


def notes_and_stack(data,top):
    rightw=250;gap=23;leftw=CW-rightw-gap
    ly=para('ПОРЯДОК И УСЛОВИЯ',M,top,leftw,8.0,bold=True,color=TEAL)+5
    ry=para('СТЕК ВЫЗОВОВ / ТОЧКИ КОДА',M+leftw+gap,top,rightw,8.0,bold=True,color=BLUE)+5
    for i,note in enumerate(data['notes']):
        ly=para(f'{i+1}. {note}',M,ly,leftw,8.35,leading=11.1)+5
    for i,item in enumerate(data['stack']):
        ry=para(f'{i+1:02d}  {item}',M+leftw+gap,ry,rightw,8.1,leading=11.0)+4
    bottom=max(ly,ry)+4
    if data['refs']:
        para('Код: '+' · '.join(data['refs']),M,bottom,CW,6.7,leading=8.6,color=MUTED)
    return bottom


def draw_table(data,top=113):
    rows=data['table']
    if data['kind']=='sources': widths=[CW*.35,CW*.65]
    elif len(rows[0])==3: widths=[CW*.255,CW*.33,CW*.415]
    else: widths=[CW*.38,CW*.62]
    out=[]
    for j,row in enumerate(rows):
        cells=[]
        for item in row:
            st=ParagraphStyle('cell',fontName='Bold' if j==0 else 'Body',fontSize=8.05,
                              leading=10.7,textColor=colors.HexColor('#FFFFFF' if j==0 else INK),splitLongWords=True)
            cells.append(Paragraph(html.escape(item),st))
        out.append(cells)
    table=Table(out,colWidths=widths,hAlign='LEFT')
    table.setStyle(TableStyle([
        ('BACKGROUND',(0,0),(-1,0),colors.HexColor(BLUE)),
        ('ROWBACKGROUNDS',(0,1),(-1,-1),[colors.HexColor('#F0F4F9'),colors.white]),
        ('VALIGN',(0,0),(-1,-1),'TOP'),('LEFTPADDING',(0,0),(-1,-1),8),
        ('RIGHTPADDING',(0,0),(-1,-1),8),('TOPPADDING',(0,0),(-1,-1),6),
        ('BOTTOMPADDING',(0,0),(-1,-1),6),('LINEBELOW',(0,0),(-1,0),.4,colors.HexColor(BLUE))]))
    _,height=table.wrap(CW,1000);table.drawOn(C,M,H-top-height)
    return top+height+13


def overview():
    box('КЛИЕНТ\nHTTP / REST',M+7,113,134,47)
    box('GATEWAY :8080\nQueries + Commands',M+266,113,207,47,fill='#E3EDFC')
    arrow(M+145,136,M+262,136)
    box('REDIS\nплан восстановления',M+594,113,172,47,fill='#EFE7F7')
    arrow(M+475,136,M+590,136,color=PURPLE)
    for x,label,db in [(M+15,'Reservation :8070','reservations\nhotels + reservation + guards'),
                       (M+286,'Payment :8060','payments\npayment + guards'),
                       (M+557,'Loyalty :8050','loyalties\nloyalty + operation keys')]:
        box(label,x,205,202,39,fill='#E5F2F0')
        arrow(M+370,162,x+101,201)
        box(db,x,274,202,44,fill='#F2F4F7')
        arrow(x+101,245,x+101,271,color=TEAL)
    para('Между downstream-сервисами нет прямых вызовов. Gateway координирует сценарий; каждый сервис владеет своей логической БД.',M,332,CW,9.2,bold=True)
    para('КАК ЧИТАТЬ СХЕМЫ',M,364,350,8.2,bold=True,color=TEAL)
    para('Шаги нумеруются сверху вниз. Стрелка — вызов; «→ 200/201/204» в подписи объединяет вызов и ожидание ответа. Петля — локальная работа. Параллелизм указан явно.',M,381,350,9.0)
    para('НАВИГАЦИЯ ПО PDF',M+402,364,350,8.2,bold=True,color=BLUE)
    para('С. 2 — HTTP-путь\nС. 3–8 — чтения и создание\nС. 9–11 — компенсация и отмена\nС. 12–16 — операции, Redis, recovery\nС. 17–19 — внутренние ручки сервисов\nС. 20–24 — breaker, ошибки, служебные ручки, пример, код',M+402,381,350,8.7,leading=13.5)
    para('Числа и порядок вызовов сверены с исходным кодом и YAML. Примеры пользователя Анна иллюстративны; health не проверяет downstream.',M,496,370,8.3,color=MUTED)


def states(top=117):
    for text,x,y,fill in [('reserved',M+12,top,LIGHT),('running',M+297,top,'#E3EDFC'),
                          ('completed',M+582,top,'#E5F2F0'),('deferred',M+297,top+92,'#FFF1DC'),
                          ('failed / manual',M+12,top+184,'#F8E8E8'),('failed / Rejected',M+582,top+184,'#F8E8E8')]:
        box(text,x,y,166,40,fill)
    arrow(M+180,top+20,M+294,top+20)
    para('Start / Take',M+186,top+2,102,7.4,align=TA_CENTER)
    arrow(M+466,top+20,M+579,top+20,color=TEAL)
    para('Finish: всё выполнено',M+467,top-4,112,7.1,align=TA_CENTER)
    arrow(M+380,top+41,M+380,top+89,color=ORANGE)
    para('временный отказ',M+391,top+58,155,7.8,color=ORANGE)
    arrow(M+463,top+112,M+493,top+112,color=ORANGE)
    arrow(M+493,top+112,M+493,top+54,color=ORANGE)
    arrow(M+493,top+54,M+446,top+40,color=ORANGE)
    para('через 10s\nновая Take',M+505,top+80,90,7.4,color=ORANGE)
    arrow(M+297,top+36,M+177,top+184,color='#AF4545')
    para('400/404 в фоне:\nудержать до ручного retry',M+20,top+132,164,7.3,color='#914141')
    arrow(M+461,top+36,M+582,top+184,color='#AF4545')
    para('Initial: Payment не отменён\nосвободить слот',M+584,top+132,170,7.3,color='#914141')
    return top+238


def breaker(top=120):
    box('CLOSED\nчтения разрешены',M+12,top,196,53,fill='#E5F2F0')
    box('OPEN\nсразу unavailable',M+286,top,196,53,fill='#F8E8E8')
    box('HALF-OPEN\nодна проба',M+560,top,196,53,fill='#FFF1DC')
    arrow(M+210,top+26,M+282,top+26,color='#AF4545')
    para('5 ошибок',M+209,top+4,74,7.5,align=TA_CENTER)
    arrow(M+484,top+26,M+556,top+26,color=ORANGE)
    para('через 1s',M+483,top+4,74,7.5,align=TA_CENTER)
    arrow(M+658,top+55,M+658,top+99,color=TEAL)
    arrow(M+658,top+99,M+110,top+99,color=TEAL)
    arrow(M+110,top+99,M+110,top+55,color=TEAL)
    para('успех пробы → CLOSED; availability-ошибка пробы → снова OPEN',M+178,top+113,445,9.2,bold=True,align=TA_CENTER)
    return top+153


def example():
    # A compact numerical timeline; all numbers are derived in the accompanying prose.
    xpositions=[M+5,M+199,M+393,M+587]
    labels=[('До создания','9 / BRONZE / 5%'),('Создание R','28 500 ₽; PAID'),('После +1','10 / SILVER / 7%'),('После отмены','9 / BRONZE / 5%')]
    for i,(title,value) in enumerate(labels):
        box(title+'\n'+value,xpositions[i],114,180,57,fill=LIGHT if i%2==0 else '#E5F2F0')
        if i<3: arrow(xpositions[i]+181,143,xpositions[i+1]-3,143)
    return 187


def write_markdown():
    lines=['# Как работает Hotels Booking System', '', f'Дата сборки: {DATE}. PDF: [gateway-request-flow.pdf](gateway-request-flow.pdf).', '',
           'Исходник и генератор: `scripts/render-gateway-guide.py`. Диаграммы в PDF векторные, текст выделяется.', '']
    for index,d in enumerate(PAGES,1):
        lines += [f'## {index:02d}. {d["title"]}', '', d['subtitle'], '']
        if d['events']:
            lines += ['```mermaid','sequenceDiagram']
            for i,lane in enumerate(d['lanes']): lines.append(f'    participant P{i} as {lane}')
            for a,b,label in d['events']: lines.append(f'    P{a}->>P{b}: {label.replace(chr(59),chr(44))}')
            lines += ['```','']
        if d['table']:
            rows=d['table']; lines.append('| '+' | '.join(rows[0])+' |')
            lines.append('| '+' | '.join(['---']*len(rows[0]))+' |')
            for row in rows[1:]: lines.append('| '+' | '.join(row)+' |')
            lines.append('')
        for note in d['notes']: lines+=['- '+note]
        if d['stack']: lines+=['','Стек вызовов:', '', '```text']+list(d['stack'])+['```']
        if d['refs']: lines+=['','Код: '+', '.join('`'+r+'`' for r in d['refs'])+'.']
        lines+=['']
    MARKDOWN.write_text('\n'.join(lines))


def main():
    global C
    OUT.parent.mkdir(parents=True,exist_ok=True)
    C=canvas.Canvas(str(OUT),pagesize=(W,H),pageCompression=1)
    C.setTitle('Hotels Booking System — путь каждого запроса')
    C.setAuthor('Hotels Booking System · документация репозитория')
    C.setSubject('Gateway, HTTP, CQRS, Saga, Redis, порядок вызовов и восстановление')
    for index,d in enumerate(PAGES,1):
        header(index,d)
        if d['kind']=='overview': overview()
        else:
            if d['kind']=='states': bottom=states()
            elif d['kind']=='breaker': bottom=breaker()
            elif d['kind']=='example': bottom=example()
            elif d['table']: bottom=draw_table(d)
            else: bottom=sequence(d)
            notes_and_stack(d,bottom+2)
        C.showPage()
    C.save();write_markdown()
    if OVERFLOWS:
        raise SystemExit('Layout overflow: '+repr(OVERFLOWS))
    print(f'{OUT}: {len(PAGES)} pages; {OUT.stat().st_size:,} bytes')
    print(MARKDOWN)

if __name__=='__main__': main()
