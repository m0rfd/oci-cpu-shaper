#!/usr/bin/env bash
set -euo pipefail

summary_file="${1:-}"
min_coverage="${2:-${MIN_COVERAGE:-0}}"

if [ -z "$summary_file" ]; then
        echo "Coverage summary path is required." >&2
        exit 1
fi

if [ ! -f "$summary_file" ]; then
        echo "Coverage summary '$summary_file' not found." >&2
        exit 1
fi

total=$(awk '/^total:/ {total=$NF} END {print total}' "$summary_file")
if [ -z "$total" ]; then
        echo "Coverage summary '$summary_file' is missing a total coverage entry." >&2
        exit 1
fi

if ! [[ "$total" =~ ^[0-9]+(\.[0-9]+)?%$ ]]; then
        echo "Coverage summary '$summary_file' has an unparsable total coverage value: '$total'." >&2
        exit 1
fi

coverage_value=${total%\%}
if ! awk -v cov="$coverage_value" -v min="$min_coverage" 'BEGIN { if (cov+0 >= min+0) exit 0; exit 1 }'; then
        echo "Coverage ${coverage_value}% is below required ${min_coverage}%." >&2
        exit 1
fi

echo "Total coverage: ${total}"
