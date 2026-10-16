CREATE TABLE payment (
 id SERIAL PRIMARY KEY,
 payment_uid UUID NOT NULL,
 status VARCHAR(20) NOT NULL CHECK (status IN ('PAID', 'CANCELED')),
 price INT NOT NULL
);
