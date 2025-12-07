#!/usr/bin/env python3

import json
import os
import pathlib
import re
import sys
from typing import Any, Dict, Iterable, List, Optional, Set


def parse_list(raw: Optional[str]) -> Set[str]:
    if not raw:
        return set()
    return {item.strip() for item in re.split(r"[\n,]", raw) if item.strip()}


def is_relative_to(path: pathlib.Path, base: pathlib.Path) -> bool:
    try:
        path.relative_to(base)
        return True
    except ValueError:
        return False


def load_rule_metadata(run: Dict[str, Any]) -> Dict[str, Dict[str, Any]]:
    rules: Dict[str, Dict[str, Any]] = {}
    for rule in run.get("tool", {}).get("driver", {}).get("rules", []) or []:
        rule_id = rule.get("id")
        if not rule_id:
            continue
        properties = rule.get("properties", {}) or {}
        rules[rule_id] = {
            "severity": properties.get("problem.severity")
            or properties.get("security-severity")
            or rule.get("defaultConfiguration", {}).get("level"),
            "name": rule.get("name")
            or rule.get("shortDescription", {}).get("text")
            or rule_id,
        }
    return rules


def format_region(region: Dict[str, Any]) -> str:
    start_line = region.get("startLine")
    end_line = region.get("endLine")
    start_col = region.get("startColumn")
    end_col = region.get("endColumn")

    if start_line is None:
        return ""

    line_part = str(start_line) if end_line in (None, start_line) else f"{start_line}-{end_line}"
    if start_col is None:
        return line_part

    col_part = str(start_col) if end_col in (None, start_col) else f"{start_col}-{end_col}"
    return f"{line_part}:{col_part}"


def gather_locations(
    result: Dict[str, Any],
    repo_root: pathlib.Path,
    ignore_paths: Iterable[str],
    gomodcache: Optional[pathlib.Path],
) -> List[str]:
    locations: List[str] = []
    normalized_ignores = [p for p in ignore_paths if p]

    for location in result.get("locations", []) or []:
        physical = location.get("physicalLocation") or {}
        artifact = physical.get("artifactLocation") or {}
        uri = artifact.get("uri")
        if not uri:
            continue

        candidate = pathlib.Path(uri)
        if not candidate.is_absolute():
            candidate = repo_root / candidate
        candidate = candidate.resolve()

        if gomodcache and is_relative_to(candidate, gomodcache):
            continue
        if not is_relative_to(candidate, repo_root):
            continue

        relative = candidate.relative_to(repo_root)
        relative_str = str(relative)
        if any(relative_str.startswith(prefix) for prefix in normalized_ignores):
            continue

        region = physical.get("region") or {}
        region_suffix = format_region(region)
        if region_suffix:
            locations.append(f"{relative_str}:{region_suffix}")
        else:
            locations.append(relative_str)

    return locations


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

    repo_root = pathlib.Path(os.environ.get("CODEQL_REPO_ROOT", sarif_path.parent)).resolve()
    gomodcache_env = os.environ.get("CODEQL_GOMODCACHE")
    gomodcache = pathlib.Path(gomodcache_env).resolve() if gomodcache_env else None
    ignore_rules = parse_list(os.environ.get("CODEQL_IGNORE_RULES"))
    ignore_paths = parse_list(os.environ.get("CODEQL_IGNORE_PATHS"))

    with sarif_path.open("r", encoding="utf-8") as sarif_fp:
        sarif = json.load(sarif_fp)

    issues: List[Dict[str, Any]] = []

    for run in sarif.get("runs", []) or []:
        rule_meta = load_rule_metadata(run)
        for result in run.get("results", []) or []:
            if result.get("suppressions"):
                continue

            rule_id = result.get("ruleId") or result.get("rule", {}).get("id") or "unknown-rule"
            if rule_id in ignore_rules:
                continue

            severity = result.get("level") or rule_meta.get(rule_id, {}).get("severity") or "warning"
            security_severity = (
                result.get("properties", {}).get("security-severity")
                or rule_meta.get(rule_id, {}).get("severity")
            )
            severity_label = severity
            if security_severity and str(security_severity).strip():
                severity_label = f"{severity}/{security_severity}"

            message = result.get("message", {}).get("text") or "(no message provided)"
            locations = gather_locations(result, repo_root, ignore_paths, gomodcache)
            if not locations:
                continue

            for location in locations:
                issues.append(
                    {
                        "rule": rule_id,
                        "severity": severity_label,
                        "message": message.strip(),
                        "location": location,
                    }
                )

    if issues:
        print(f"CodeQL found {len(issues)} issue(s) in {scope}:")
        for issue in issues:
            print(
                f"- [{issue['severity']}] {issue['rule']}: {issue['message']} ({issue['location']})"
            )
        return 1

    print(f"No CodeQL issues found in {scope}.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
