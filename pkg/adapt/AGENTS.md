# AGENTS

## Scope: `pkg/adapt/`
- `controller.go` holds the adaptive loop, state machine transitions, and shared helpers such as `clamp`—keep exported APIs stable.
- Place public state/interfaces in `state.go`, configuration structs/helpers in `config.go`, suppression math in `suppression.go`, and keep the dry-run implementation in `noop_controller.go`.
- Mirror logic changes with unit tests in `controller_test.go` and ensure `make lint`, `make test`, and `make coverage MIN_COVERAGE=95` stay green before sending patches.
