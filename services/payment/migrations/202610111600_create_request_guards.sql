CREATE TABLE payment_requests (id SERIAL PRIMARY KEY, operation_id VARCHAR(36) NOT NULL UNIQUE, canceled BOOLEAN NOT NULL DEFAULT FALSE);
DROP INDEX payment_payment_uid;
CREATE UNIQUE INDEX payment_payment_uid ON payment (payment_uid);
