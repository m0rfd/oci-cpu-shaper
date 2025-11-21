# AGENTS

## Scope: `pkg/adapt/`
- Controller files are split by concern: `controller_ctor.go` owns the struct definition and constructor, `controller_loop.go` keeps the ticker/run orchestration, `controller_step.go` covers OCI metric fetch/decision logic, `controller_observation.go` handles estimator feedback, `controller_state.go` houses getters plus the shared helpers such as `clamp`, and `dutycycler_wrapper.go` contains the dry-run wrapper.
- Place public state/interfaces in `state.go`, configuration structs/helpers in `config.go`, suppression math in `suppression.go`, and keep the dry-run implementation in `noop_controller.go`.
- Mirror logic changes with unit tests in `controller_loop_test.go`, `controller_step_test.go`, `controller_observation_test.go`, `config_test.go`, `suppression_test.go`, and `noop_controller_test.go`; share reusable fakes via `testhelpers_test.go` and ensure `make lint`, `make test`, and `make coverage MIN_COVERAGE=99` stay green before sending patches.
