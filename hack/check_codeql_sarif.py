#!/usr/bin/env python3

import json
import os
import pathlib
import sys


def main() -> int:
    sarif_file = os.environ.get("SARIF_FILE")
    scope = os.environ.get("CODEQL_SCOPE", "analysis")

    if not sarif_file:
        print("SARIF_FILE environment variable is required.")
        return 1

    sarif_path = pathlib.Path(sarif_file)

    if not sarif_path.exists():
        print(f"SARIF report not found: {sarif_path}")
        return 1

    with sarif_path.open("r", encoding="utf-8") as sarif_fp:
        sarif = json.load(sarif_fp)

    issues = [
        result
        for run in sarif.get("runs", [])
        for result in run.get("results", [])
        if not result.get("suppressions")
    ]

    if issues:
        print(f"CodeQL found {len(issues)} issue(s) in {scope}.")
        return 1

    print(f"No CodeQL issues found in {scope}.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
