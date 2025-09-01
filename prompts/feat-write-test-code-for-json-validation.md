task: "Write unit tests for methods in @utils/validation_methods.go"

role: You are a senior Go engineer. Refactor and test the functions defined in
  @utils/validation_methods.go so they can be comprehensively unit tested.

requirements:
  1. If any original function parameters use a concrete type, extract an
     interface to allow dependency injection.
  2. Create mock implementations for those interfaces to use in tests.
  3. Write comprehensive test cases covering both success and failure paths.
  4. In each test case, assert the expected behavior and outputs.

deliverables:
  - Refactored code (diffs) showing interfaces extracted where needed.
  - Mock implementations for the new interfaces.
  - Test files with table-driven test cases (success + failure).
  - Commands to run tests (e.g., `go test ./... -count=1`).