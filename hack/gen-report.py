#!/usr/bin/env python3
"""
Generate a Markdown E2E test report from JUnit XML files produced by Ginkgo.
Usage: gen-report.py <reports_dir>
"""

import os
import sys
import xml.etree.ElementTree as ET
from datetime import datetime

REPORTS_DIR = sys.argv[1] if len(sys.argv) > 1 else "/tmp/reports"
SUITES = os.environ.get(
    "TEST_SUITES", "integration lifecycle consolidation scheduling storage"
).split()


def parse_xml(path):
    try:
        tree = ET.parse(path)
        root = tree.getroot()
        if root.tag == "testsuites":
            return list(root)
        elif root.tag == "testsuite":
            return [root]
        return []
    except Exception as e:
        print(f"[gen-report] WARN: failed to parse {path}: {e}", file=sys.stderr)
        return []


def fmt_duration(seconds):
    seconds = int(float(seconds))
    h = seconds // 3600
    m = (seconds % 3600) // 60
    s = seconds % 60
    if h > 0:
        return f"{h}h{m}m{s}s"
    if m > 0:
        return f"{m}m{s}s"
    return f"{s}s"


lines = []

# ── Header ────────────────────────────────────────────────────────────────────
now = datetime.now().strftime("%Y-%m-%d %H:%M:%S")

exit_code_file = os.path.join(REPORTS_DIR, "exit-code")
exit_code = open(exit_code_file).read().strip() if os.path.exists(exit_code_file) else "unknown"
status_emoji = "✅" if exit_code == "0" else "❌"

lines.append("# E2E Test Report\n")
lines.append(f"| | |")
lines.append(f"|---|---|")
lines.append(f"| **Time** | {now} |")
lines.append(f"| **Result** | {status_emoji} exit code `{exit_code}` |")
lines.append("")

# ── Summary table ─────────────────────────────────────────────────────────────
lines.append("## Summary\n")
lines.append("| Suite | Tests | ✅ Passed | ❌ Failed | ⏭ Skipped | ⏱ Duration |")
lines.append("|-------|------:|--------:|--------:|--------:|--------:|")

total_tests = total_passed = total_failed = total_skipped = 0
suite_failures = {}  # suite -> list of (name, message)

for suite in SUITES:
    xml_path = os.path.join(REPORTS_DIR, f"{suite}.xml")
    if not os.path.exists(xml_path):
        lines.append(f"| ⚪ {suite} | — | — | — | — | — |")
        continue

    ts_list = parse_xml(xml_path)
    if not ts_list:
        lines.append(f"| ⚠️ {suite} | parse error | — | — | — | — |")
        continue

    tests = failures = errors = skipped = 0
    duration = 0.0
    failed_cases = []

    for ts in ts_list:
        tests    += int(ts.get("tests",    0))
        failures += int(ts.get("failures", 0))
        errors   += int(ts.get("errors",   0))
        skipped  += int(ts.get("skipped",  0))
        try:
            duration += float(ts.get("time", 0))
        except (ValueError, TypeError):
            pass
        for tc in ts.findall("testcase"):
            f = tc.find("failure")
            if f is not None:
                tc_name = tc.get("name", "unknown")
                failed_cases.append((tc_name, (f.text or "").strip()))

    n_failed = failures + errors
    n_passed = tests - n_failed - skipped
    row_emoji = "✅" if n_failed == 0 else "❌"

    lines.append(
        f"| {row_emoji} **{suite}** | {tests} | {n_passed} | {n_failed} | {skipped} | {fmt_duration(duration)} |"
    )

    total_tests   += tests
    total_passed  += n_passed
    total_failed  += n_failed
    total_skipped += skipped
    suite_failures[suite] = failed_cases

lines.append(
    f"| **Total** | **{total_tests}** | **{total_passed}** | **{total_failed}** | **{total_skipped}** | — |"
)
lines.append("")

# ── Failed tests detail ────────────────────────────────────────────────────────
all_failures = [
    (suite, name, msg)
    for suite, cases in suite_failures.items()
    for name, msg in cases
]

if all_failures:
    lines.append("## Failed Tests\n")
    for suite, name, msg in all_failures:
        lines.append(f"### ❌ `[{suite}]` {name}\n")
        if msg:
            snippet = msg if len(msg) <= 3000 else msg[:3000] + "\n…(truncated)"
            lines.append("```")
            lines.append(snippet)
            lines.append("```")
            lines.append("")
else:
    lines.append("## 🎉 All Tests Passed\n")

# ── Per-suite collapsible details ─────────────────────────────────────────────
lines.append("## Suite Details\n")
for suite in SUITES:
    xml_path = os.path.join(REPORTS_DIR, f"{suite}.xml")
    if not os.path.exists(xml_path):
        continue
    ts_list = parse_xml(xml_path)
    if not ts_list:
        continue

    lines.append(f"<details>")
    lines.append(f"<summary><b>{suite}</b></summary>\n")
    lines.append("| Test | Status | Duration |")
    lines.append("|------|--------|----------|")

    for ts in ts_list:
        for tc in ts.findall("testcase"):
            tc_name = tc.get("name", "unknown")
            tc_time = fmt_duration(tc.get("time", 0))
            if tc.find("failure") is not None or tc.find("error") is not None:
                tc_status = "❌ FAIL"
            elif tc.find("skipped") is not None:
                tc_status = "⏭ SKIP"
            else:
                tc_status = "✅ PASS"
            # Escape pipe chars in test names
            tc_name_escaped = tc_name.replace("|", "\\|")
            lines.append(f"| {tc_name_escaped} | {tc_status} | {tc_time} |")

    lines.append("")
    lines.append("</details>\n")

print("\n".join(lines))
