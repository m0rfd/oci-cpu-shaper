#!/usr/bin/env python3
import pathlib
import re
import sys


def main() -> None:
    if len(sys.argv) < 3:
        sys.stderr.write("usage: validate_makefile_shellcheck.py <output> <makefiles...>\n")
        sys.exit(1)

    output_path = pathlib.Path(sys.argv[1])
    commands = ["# shellcheck shell=bash"]
    target_pattern = re.compile(r"^[^#\s].*:")

    for source in map(pathlib.Path, sys.argv[2:]):
        content = source.read_text().splitlines()
        in_target = False
        for line in content:
            if target_pattern.match(line):
                in_target = True
                continue
            if not line.strip():
                in_target = False
                continue
            if in_target and re.match(r"^[\s\t]+", line):
                cleaned = re.sub(r"^[\s\t@]+", "", line)
                commands.extend(cleaned.split("\\n"))

    output_path.write_text("\n".join(commands) + "\n")


if __name__ == "__main__":
    main()
