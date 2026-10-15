-- SPDX-License-Identifier: Apache-2.0
-- Tyche wallet-login users have string IDs; OMS money and order paths use numeric IDs.

CREATE SEQUENCE IF NOT EXISTS tyche_user_seq START 1000000 NO CYCLE;

CREATE TABLE IF NOT EXISTS tyche_user_mappings (
    oms_user_id BIGINT PRIMARY KEY DEFAULT nextval('tyche_user_seq'),
    tyche_user_id TEXT NOT NULL UNIQUE,
    neco_address TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS tyche_user_mappings_neco_address_idx
    ON tyche_user_mappings (neco_address);
