# Running Tests

This document explains how to run the unit tests for must-gather collection scripts.

## Quick Start

```bash
# Run all tests
make test
```

That's it! The `make test` command will:
1. Install BATS (Bash Automated Testing System) if not already installed
2. Run all unit tests in the `tests/` directory

## Understanding Test Output

Tests use the TAP (Test Anything Protocol) format:

```
1..23
ok 1 get_operator_ns sets operator_ns when operator exists with single subscription
ok 2 get_operator_ns exits 0 with INFO message when operator not found
ok 3 get_operator_ns exits 1 with ERROR message when multiple subscriptions found
...
```

- `ok` = test passed
- `not ok` = test failed

## Running Specific Tests

To run tests for a specific script:

```bash
# Run only common.sh tests
./tmp/bin/bats/bin/bats tests/common.bats

# Run only monitoring tests
./tmp/bin/bats/bin/bats tests/monitoring_common.bats
```

## Test Coverage

Current test coverage includes:

| Script | Test File | Coverage |
|--------|-----------|----------|
| `common.sh` | `tests/common.bats` | `get_operator_ns()`, `get_log_collection_args()` |
| `monitoring_common.sh` | `tests/monitoring_common.bats` | `get_first_ready_prom_pod()`, `get_first_ready_alertmanager_pod()` |
| `gather_service_logs_util` | `tests/service_logs_util.bats` | `collect_service_logs()` |
| `gather_metallb` | `tests/gather_metallb.bats` | End-to-end script execution |

## Troubleshooting

### Tests fail with "command not found: bats"

Run `make bats` to install BATS, then retry.

### Tests fail with permission errors

Ensure the mock scripts are executable:

```bash
chmod +x tests/mocks/*
```
