#!/usr/bin/env bash
set -euo pipefail
docker compose exec -T reservation-db psql -v ON_ERROR_STOP=1 -U program -d reservations <<'SQL'
INSERT INTO hotels (hotel_uid, name, country, city, address, stars, price)
VALUES ('049161bb-badd-4fa8-9d90-87c9a82b0668', 'Ararat Park Hyatt Moscow',
        'Россия', 'Москва', 'Неглинная ул., 4', 5, 10000)
ON CONFLICT (hotel_uid) DO NOTHING;
SQL
docker compose exec -T reservation-db psql -v ON_ERROR_STOP=1 -U program -d loyalties <<'SQL'
INSERT INTO loyalty (username, reservation_count, status, discount)
VALUES ('Test Max', 25, 'GOLD', 10) ON CONFLICT (username) DO NOTHING;
SQL
