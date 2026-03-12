# Protocol Buffer Definitions

This directory will contain the protobuf definitions for all gRPC services.

## Services

- `ledger/` - Ledger service definitions (T1.4)
- `account/` - Account service definitions (T2.2)
- `wallet/` - Wallet service definitions (T3.1, T3.2)
- `asset/` - Asset registry service definitions (T3.4)

## Generation

Run `make proto` to generate Go code from proto files.

