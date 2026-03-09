-- 000001_create_ledger_schema.up.sql
-- Creates the core ledger tables: transactions, entries, and materialized balances.

CREATE TABLE IF NOT EXISTS ledger_transactions (
    tx_id           CHAR(36)     NOT NULL,
    idempotency_key VARCHAR(255) NOT NULL,
    reference       VARCHAR(255) NOT NULL DEFAULT '',
    status          VARCHAR(20)  NOT NULL DEFAULT 'posted',
    created_at      DATETIME(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),

    PRIMARY KEY (tx_id),
    UNIQUE KEY uq_idempotency_key (idempotency_key),
    KEY idx_transactions_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS ledger_entries (
    entry_id   BIGINT       NOT NULL AUTO_INCREMENT,
    tx_id      CHAR(36)     NOT NULL,
    account_id VARCHAR(255) NOT NULL,
    amount     BIGINT       NOT NULL,
    asset      VARCHAR(20)  NOT NULL,
    created_at DATETIME(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),

    PRIMARY KEY (entry_id),
    KEY idx_entries_account_asset_created (account_id, asset, created_at),
    CONSTRAINT fk_entries_tx_id
        FOREIGN KEY (tx_id) REFERENCES ledger_transactions (tx_id)
        ON DELETE RESTRICT ON UPDATE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS ledger_balances (
    account_id VARCHAR(255) NOT NULL,
    asset      VARCHAR(20)  NOT NULL,
    balance    BIGINT       NOT NULL DEFAULT 0,
    updated_at DATETIME(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),

    PRIMARY KEY (account_id, asset)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
