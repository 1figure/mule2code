#!/usr/bin/env python3
"""Turns the CA1502 warnings of a `dotnet build -p:CollectMetrics=true` (on stdin) into a complexity table.

Used by generate.sh; usage: dotnet build ... 2>&1 | python3 reports/complexity.py <date>
"""
import re
import sys

PATTERN = re.compile(
    r"(?P<file>\S+?\.cs)\((?P<line>\d+),\d+\): warning CA1502: '(?P<name>[^']+)' has a cyclomatic complexity of '(?P<cc>\d+)'"
)

seen = {}
for line in sys.stdin:
    m = PATTERN.search(line)
    if not m or "/src/" not in m["file"]:
        continue
    rel = m["file"].split("/src/", 1)[1]
    seen[(rel, int(m["line"]), m["name"])] = int(m["cc"])

rows = sorted(seen.items(), key=lambda kv: (-kv[1], kv[0]))
date = sys.argv[1] if len(sys.argv) > 1 else ""
print(f"# Cyclomatic complexity per method, src/ only, highest first ({date}).")
print("# Source: Roslyn CA1502 with threshold 1; methods of complexity 1 (no branching) are not listed.")
for (rel, ln, name), cc in rows:
    print(f"{cc:3d}  {name:45s} src/{rel}:{ln}")
if rows:
    print(f"\nAverage over the {len(rows)} listed methods: {sum(seen.values()) / len(rows):.2f}; maximum {rows[0][1]}.")
else:
    print("\n(no CA1502 warnings found on stdin)")
