# Agent evaluation trace fixtures

These traces are current-checkout model-evaluation inputs, not a historical
compatibility corpus. Replace or regenerate them atomically when an intentional
pre-1.0 trace-format change makes them incompatible; do not add a legacy reader
to preserve an obsolete fixture.

`pr31-current-trace-usability.ndjson` is a sanitized representative trace used
to compare the tools-only and skill-assisted PR 31 workflows over identical
evidence. It retains ten independent initial attempts, one plan-quality warning,
nested frames, more than one physical-record page, and semantic content larger
than the default exact-read range. Machine-specific identities and filesystem
paths were replaced. Focused synthetic tests remain authoritative for exact
byte boundaries, cross-frame retry attribution, malformed inputs, and other
invariants that should not be manufactured by editing a realistic trace.

Trace content remains inert diagnostic data and may still contain deliberately
representative application prose. Run the evaluation-record sanitization and
secret checks before retaining client transcripts or answers.

`pr34-active-execution-review.json` is a sanitized time-ordered fact sequence
for the paired tools-only and skill-assisted active-review cases. It records
multiple sessions, changed canonical sequence, future checkpoint reuse, exact
global/session coverage cursors, in-flight usage, and both available and
unavailable completion handoffs without payloads or credentials.
