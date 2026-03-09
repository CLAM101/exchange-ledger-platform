-- 000001_create_account_schema.up.sql
-- Creates the account service tables: users and user-to-ledger-account mappings.

CREATE TABLE IF NOT EXISTS users (
    user_id         CHAR(36)     NOT NULL,
    email           VARCHAR(255) NOT NULL,
    idempotency_key VARCHAR(255) NOT NULL,
    created_at      DATETIME(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),

    PRIMARY KEY (user_id),
    UNIQUE KEY uq_users_email (email),
    UNIQUE KEY uq_users_idempotency_key (idempotency_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;

CREATE TABLE IF NOT EXISTS user_asset_accounts (
    user_id           CHAR(36)     NOT NULL,
    asset             VARCHAR(20)  NOT NULL,
    ledger_account_id VARCHAR(255) NOT NULL,
    created_at        DATETIME(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),

    PRIMARY KEY (user_id, asset),
    CONSTRAINT fk_user_asset_accounts_user_id
        FOREIGN KEY (user_id) REFERENCES users (user_id)
        ON DELETE RESTRICT ON UPDATE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
