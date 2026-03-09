-- 000001_create_account_schema.down.sql
-- Drops account tables in reverse FK order.

DROP TABLE IF EXISTS user_asset_accounts;
DROP TABLE IF EXISTS users;
