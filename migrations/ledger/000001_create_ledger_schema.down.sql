-- 000001_create_ledger_schema.down.sql
-- Drops all ledger tables in reverse dependency order.

DROP TABLE IF EXISTS ledger_balances;
DROP TABLE IF EXISTS ledger_entries;
DROP TABLE IF EXISTS ledger_transactions;
