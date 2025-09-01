task: Harden JSON input handling across the Golang repo
role: You are a senior Go engineer. Work repository-wide to find all places where external JSON input over network connection is accepted without strict validation, infer the data types, and add robust validation.

goals:
  - Inventory all JSON decoding sites and classify risk.
  - Replace loose decoding with strict decoding and schema validation.
  - Produce diffs and exact commands to run.

signals_to_search:
  - 'json.Unmarshal('
  - 'json.NewDecoder('
  - 'Decode(&' within HTTP handlers or networked code paths
  - Decoding into 'map[string]any' / 'interface{}' with ad-hoc type assertions
  - Missing: DisallowUnknownFields, body size limits, required-field checks
  - Lenient error handling (ignoring type errors, masking decode errors)

search_plan:
  - Run ripgrep on likely roots (adjust paths as needed):
    - rg -n "json\\.NewDecoder\\(" -g "*.go"'
    - rg -n "json\\.Unmarshal\\(" -g "*.go"'
    - rg -n "map\\[string\\](any|interface\\{\\})" -g "*.go"'
    - rg -n "Decode\\(&" -g "*.go"
  - For each hit, capture ~15 lines of context and record:
    - input source (HTTP body, message bus, stdin)
    - target type (struct/map/any) and current checks
    - presence/absence of: size limit, DisallowUnknownFields, validation, content-type check

validation_strategy:
  - Wrap body with http.MaxBytesReader (e.g. 1 MB).
  - Use json.Decoder with DisallowUnknownFields().
  - Enforce Content-Type: application/json.
  - Use validator/v10 tags for struct fields; add Validate() if needed.
  - Infer numeric fields by name/usage, and if strings are used, check all runes with unicode.IsDigit.
  - Introduce DTOs instead of decoding into domain models.
  - Return consistent error JSON: { "error": { "code": "...", "details": {...} } } 

type_inference_and_checks:
    - Infer likely field types from field names (e.g., `id`, `count`, `age`) and usage.
    - If a field is identified as numeric (int, float, id, etc.), enforce validation:
      - Use `unicode.IsDigit` to ensure the entire string consists of digits if the field is provided as a string.
      - Reject inputs that contain non-digit characters.