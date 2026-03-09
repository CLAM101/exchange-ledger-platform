-- Creates per-service databases and users.
-- Mounted into the MySQL container via docker-compose; runs once on first start.

-- The 'ledger' database is created automatically by MYSQL_DATABASE env var.
-- Create the account database and its dedicated user.
CREATE DATABASE IF NOT EXISTS account;

CREATE USER IF NOT EXISTS 'account_user'@'%' IDENTIFIED BY 'account_pass';
GRANT ALL PRIVILEGES ON account.* TO 'account_user'@'%';

FLUSH PRIVILEGES;
