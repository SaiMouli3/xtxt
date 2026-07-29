#!/usr/bin/env python3
"""Compare the C implementation against the shared conformance fixtures."""
import json
import pathlib
import subprocess
import sys

HERE = pathlib.Path(__file__).resolve().parent
CASES = sorted((HERE / ".." / ".." / ".." / "conformance" / "cases").resolve().glob("*.xtxt"))
EMIT = HERE / ".." / "build" / "emit_canonical"

if not CASES:
    sys.exit("no conformance cases found")
if not EMIT.exists():
    sys.exit(f"{EMIT} not built; run make first")

failures = []
for case in CASES:
    expected = json.loads(case.with_suffix(".json").read_text())
    proc = subprocess.run([str(EMIT), str(case)], capture_output=True, text=True)
    if proc.returncode != 0:
        failures.append(f"{case.name}: exited {proc.returncode}: {proc.stderr.strip()}")
        continue
    got = json.loads(proc.stdout)
    if got["ast"] != expected["ast"]:
        failures.append(f"{case.name}: AST mismatch\n  got:  {got['ast']}\n  want: {expected['ast']}")
    if got["issues"] != expected["issues"]:
        failures.append(
            f"{case.name}: issues mismatch\n  got:  {got['issues']}\n  want: {expected['issues']}")

if failures:
    print("\n".join(failures), file=sys.stderr)
    sys.exit(f"\n{len(failures)} of {len(CASES)} cases failed")
print(f"all {len(CASES)} conformance cases passed")
