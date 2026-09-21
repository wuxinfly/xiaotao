# Protocol Validator Design

XiaoTao needs a deterministic guard at the point where an Agent is about to persist a formal
Handoff or Memory Worker payload. The guard is artifact-triggered: invoking it validates one
existing JSON file and returns a result. It never creates XiaoTao state, selects a role or Worker,
starts a Workflow, delegates work, or advances a phase.

The first version is a single zero-dependency Python CLI at `xiaotao/scripts/validate.py`. It
accepts one of `handoff`, `memory-request`, or `memory-response`, followed by the JSON file to
validate. A project root defaults to the current working directory and can be supplied explicitly
with `--project-root`. The implementation mirrors the three repository schemas directly with
small reusable type, object, array, and path helpers. This avoids pretending that the Python
standard library is a general JSON Schema Draft 2020-12 engine while keeping the checked protocol
equivalent to the current schemas.

Schema errors and unsafe or missing referenced paths are validation failures. References must use
portable project-relative paths: absolute paths, Windows drive or UNC forms, backslashes, empty
segments, `.` and `..` segments, and paths that resolve outside the project root are rejected.
Handoff result/state paths, Memory Worker request `source_files`, and Memory Worker response
`source_refs` must resolve to existing files. The CLI prints one diagnostic per line with a JSON
path, exits `0` for valid input, `1` for invalid input, and `2` for invocation or unexpected I/O
errors. `--json` returns a stable machine-readable summary.

The existing PowerShell contract suite will run valid and invalid fixtures for all three kinds,
including invalid JSON, missing fields, unsafe traversal, and missing references. Documentation
will require the validator immediately before persistence. Repair-once and preservation of invalid
raw output remain orchestration responsibilities, keeping the CLI a protocol guard rather than a
runtime.

Because model output is untrusted, parsing rejects Python's non-standard `NaN`, `Infinity`, and
`-Infinity` constants. Path validation rejects control characters before calling host path APIs and
converts expected path API failures into normal diagnostics. Structural fixtures run through both
AJV and the Python CLI with the same expected result so schema changes cannot silently drift away
from the zero-dependency implementation; path reachability remains an intentional Python-only
semantic guard.
