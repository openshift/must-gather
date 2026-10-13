# Writing Tests for Collection Scripts

This guide explains how to write unit tests for must-gather collection scripts using BATS.

## Overview

We use [BATS](https://github.com/bats-core/bats-core) (Bash Automated Testing System) for unit testing shell scripts. Tests are located in the `tests/` directory.

## Directory Structure

```
tests/
├── test_helper.bash      # Common setup/teardown, loads BATS helpers
├── mocks/
│   ├── oc                 # Mock oc command
│   └── sync               # Mock sync command
├── fixtures/
│   └── oc_outputs/        # Sample oc command outputs
│       ├── subs_single.txt
│       ├── subs_multiple.txt
│       └── subs_empty.txt
├── common.bats            # Tests for common.sh
├── monitoring_common.bats # Tests for monitoring_common.sh
├── service_logs_util.bats # Tests for gather_service_logs_util
└── gather_metallb.bats    # Tests for gather_metallb
```

## Writing a Test

### Basic Structure

```bash
#!/usr/bin/env bats

load test_helper

@test "descriptive test name" {
    run bash -c "
        export PATH=\"$TEST_TMPDIR/mocks:$BATS_TEST_DIRNAME/mocks:\$PATH\"
        # ... test code ...
    "

    assert_success
    assert_output --partial "expected output"
}
```

### Key Patterns

#### 1. Always Use Subshells for Script Testing

Scripts may call `exit`, so wrap them in `bash -c`:

```bash
@test "script exits early when condition met" {
    create_mock_oc ""
    run bash -c "
        export PATH=\"$TEST_TMPDIR/mocks:$BATS_TEST_DIRNAME/mocks:\$PATH\"
        source \"$SCRIPT_DIR/common.sh\"
        get_operator_ns 'test-operator'
    "

    assert_success
}
```

#### 2. Use `run_with_mocks()` Helper for Scripts with Strict Mode

For scripts that use strict mode (`set -o nounset`, `errexit`, `pipefail`), use the `run_with_mocks()` helper which handles PATH setup and disables strict mode:

```bash
@test "function returns expected value" {
    create_mock_oc "prometheus-k8s-0"
    run_with_mocks "
        source \"$SCRIPT_DIR/monitoring_common.sh\"
        get_first_ready_prom_pod
    "

    assert_success
    assert_output --partial "prometheus-k8s-0"
}
```

#### 3. Use BATS Variables and Helpers

Available variables from `test_helper.bash`:

| Variable | Description |
|----------|-------------|
| `$SCRIPT_DIR` | Path to `collection-scripts/` |
| `$FIXTURES_DIR` | Path to `tests/fixtures/` |
| `$PROJECT_ROOT` | Path to repository root |
| `$TEST_TMPDIR` | Temporary directory (cleaned up after each test) |
| `$BASE_COLLECTION_PATH` | Mock must-gather output directory |

Available helper functions:

| Function | Description |
|----------|-------------|
| `run_with_mocks "cmd"` | Run command with mocks in PATH and strict mode disabled |
| `create_mock_oc "output" [exit_code]` | Create an inline mock `oc` returning the given output and exit code |
| `fixture "name"` | Get path to fixture file (e.g., `fixture subs_single.txt`) |
| `create_fixture "name" "content"` | Create a temporary fixture file dynamically |

## Using the Mock `oc` Command

There are two ways to mock `oc`:

### Inline Mocks (preferred for simple tests)

Use `create_mock_oc` for tests where `oc` is called once with fixed output (see patterns 1 and 2 above for full examples). The inline mock is placed at `$TEST_TMPDIR/mocks/oc` and takes precedence over the shared mock via PATH ordering.

```bash
create_mock_oc "openshift-metallb"              # exits 0, outputs "openshift-metallb"
create_mock_oc "$(printf 'ns-one\nns-two')"      # multi-line output
create_mock_oc "" 1                              # empty output, exits 1
```

### Shared Mock (for multi-call tests)

The shared mock at `tests/mocks/oc` dispatches on command arguments and is controlled via environment variables. Use this for tests where a script makes multiple distinct `oc` calls (e.g., `gather_metallb`).

| Variable | Purpose |
|----------|---------|
| `MOCK_OC_OUTPUT` | File path containing output to return |
| `MOCK_OC_EXIT_CODE` | Exit code to return (default: 0) |
| `MOCK_OC_PODS_OUTPUT` | Direct string output for pod queries |
| `MOCK_METALLB_EXISTS` | Exit code for `oc get metallb` (0=exists, 1=not) |
| `MOCK_OC_INSPECT_EXIT_CODE` | Exit code for `oc adm inspect` (default: 0) |
| `MOCK_OC_LOG_FILE` | File path to log all oc calls for verification |

### Verifying Mock Calls

To verify that specific arguments were passed to `oc`, use `MOCK_OC_LOG_FILE`:

```bash
@test "script calls oc with correct arguments" {
    run bash -c "
        export PATH=\"$TEST_TMPDIR/mocks:$BATS_TEST_DIRNAME/mocks:\$PATH\"
        export MOCK_OC_LOG_FILE=\"$TEST_TMPDIR/oc_calls.log\"
        # ... run your script ...
        cat \"$TEST_TMPDIR/oc_calls.log\"
    "

    assert_success
    # Verify specific arguments were passed
    assert_output --partial "adm inspect"
    assert_output --partial "-n my-namespace"
}
```

The log file contains timestamped entries of all oc calls in the format:
`timestamp|command args...`

### Adding New Mock Handlers

Edit `tests/mocks/oc` to add new cases:

```bash
case "$*" in
# Add new handler
"get pods -n my-namespace"*)
    echo "pod-name-0"
    exit 0
    ;;
# ... existing cases ...
esac
```

## Creating Fixtures

Place sample outputs in `tests/fixtures/oc_outputs/`:

```bash
# Create a fixture for oc get subs output
echo "my-namespace" > tests/fixtures/oc_outputs/my_fixture.txt
```

## Assertions

We use `bats-assert` for assertions:

```bash
# Check exit code
assert_success          # exit 0
assert_failure          # exit non-zero

# Check output
assert_output "exact match"
assert_output --partial "substring"
refute_output --partial "should not contain"

# Check specific line
assert_line "exact line"
assert_line --partial "substring in any line"
```

## Running Tests During Development

```bash
# Run all tests
make test

# Run single test file
./tmp/bin/bats tests/common.bats

# Run with verbose output
./tmp/bin/bats --verbose-run tests/common.bats

# Run specific test by name pattern
./tmp/bin/bats -f "get_operator_ns" tests/common.bats
```

## Checklist for New Tests

- [ ] Test file named `<script_name>.bats`
- [ ] `load test_helper` at the top
- [ ] Use `run_with_mocks` for scripts with strict mode, or `run bash -c` otherwise
- [ ] Use `create_mock_oc` for simple single-call tests; shared mock for multi-call tests
- [ ] Tests cover success, failure, and edge cases
- [ ] No shellcheck warnings (`shellcheck tests/*.bats`)
