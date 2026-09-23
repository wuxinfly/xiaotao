#!/usr/bin/env python3
"""Build, check, and drill down XiaoTao's derived Project Memory Timeline."""

from __future__ import annotations

import argparse
import json
import re
import sys
from datetime import datetime
from pathlib import Path
from typing import Any

from activity_catalog import derive_index, resolve_reference_time

if hasattr(sys.stdout, "reconfigure"):
    try:
        sys.stdout.reconfigure(encoding="utf-8")
        sys.stderr.reconfigure(encoding="utf-8")
    except Exception:
        pass


TIMELINE_ROOT = Path(".xiaotao/memory/timeline")
YEARS_ROOT = TIMELINE_ROOT / "years"
DATE_PATTERN = re.compile(r"^(\d{4})-(\d{2})-(\d{2})$")


class TimelineError(ValueError):
    pass


def parse_scalar_val(val: str) -> Any:
    val = val.strip()
    if val.startswith('"') and val.endswith('"'):
        try:
            return json.loads(val)
        except Exception:
            return val[1:-1]
    if val.lower() == "true":
        return True
    if val.lower() == "false":
        return False
    return val


def dump_simple_yaml_event(event: dict[str, Any]) -> str:
    """Format a single timeline event into readable clean YAML."""
    lines = [
        f"  - event_id: {json.dumps(event['event_id'], ensure_ascii=False)}",
        f"    event_type: {event['event_type']}",
        f"    summary: {json.dumps(event.get('summary', event.get('title', '')), ensure_ascii=False)}",
        f"    occurred_at: {event['occurred_at']}",
    ]
    if "task_id" in event:
        lines.append(f"    task_id: {json.dumps(event['task_id'], ensure_ascii=False)}")
    if "decision_id" in event:
        lines.append(f"    decision_id: {json.dumps(event['decision_id'], ensure_ascii=False)}")
    if "details" in event and event["details"]:
        lines.append(f"    details: {json.dumps(event['details'], ensure_ascii=False)}")
    lines.append("    source_refs:")
    for ref in event.get("source_refs", []):
        lines.append(f"      - {json.dumps(ref, ensure_ascii=False)}")
    return "\n".join(lines)


def dump_date_file_content(date_str: str, events: list[dict[str, Any]]) -> str:
    header = f'date: "{date_str}"\nevents:\n'
    body = "\n".join(dump_simple_yaml_event(e) for e in events) + "\n"
    return header + body


def generate_month_summary(year: str, month: str, days: dict[str, list[dict[str, Any]]]) -> str:
    total_events = sum(len(evs) for evs in days.values())
    lines = [
        f"# 项目记忆月度摘要：{year}-{month}",
        "",
        f"本月共记录 **{total_events}** 个关键事件与任务成果，覆盖 {len(days)} 个活动日期。",
        "",
        "## 日程活动与交付",
        "",
    ]
    for day in sorted(days.keys()):
        day_events = days[day]
        lines.append(f"### {year}-{month}-{day}")
        for ev in day_events:
            summary = ev.get("summary") or ev.get("title") or "事件"
            lines.append(f"- **[{ev['event_type']}]** {summary} (`{ev['occurred_at']}`)")
        lines.append("")
    return "\n".join(lines)


def generate_year_summary(year: str, months: dict[str, dict[str, list[dict[str, Any]]]]) -> str:
    total_events = sum(
        sum(len(evs) for evs in days.values())
        for days in months.values()
    )
    lines = [
        f"# 项目记忆年度总览：{year}",
        "",
        f"{year} 年度共记录 **{total_events}** 项项目历程节点，跨越 {len(months)} 个月份。",
        "",
        "## 月度导航与重点",
        "",
    ]
    for month in sorted(months.keys()):
        m_events = sum(len(evs) for evs in months[month].values())
        lines.append(f"- **[{month} 月]({month}/summary.md)**：共沉淀 {m_events} 个事件节点")
    lines.append("")
    return "\n".join(lines)


def generate_project_summary(years: dict[str, dict[str, dict[str, list[dict[str, Any]]]]]) -> str:
    total_events = sum(
        sum(
            sum(len(evs) for evs in days.values())
            for days in months.values()
        )
        for months in years.values()
    )
    lines = [
        "# 项目经历总览 (Project Memory Timeline)",
        "",
        f"本项目自沉淀以来，已累计记录 **{total_events}** 项关键任务交付与里程碑决策，历经 {len(years)} 个活跃年度。",
        "",
        "## 历程总览与年度导航",
        "",
    ]
    for year in sorted(years.keys(), reverse=True):
        y_events = sum(
            sum(len(evs) for evs in days.values())
            for days in years[year].values()
        )
        lines.append(f"- **[{year} 年度总览](years/{year}/summary.md)**：包含 {y_events} 项关键事件")
    lines.append("")
    lines.append("> 提示：可通过按年、按月逐层下钻定位具体事件与原始 Task/Decision 证据。")
    lines.append("")
    return "\n".join(lines)


def derive_timeline(project_root: Path, *, now: datetime | None = None) -> dict[str, Any]:
    """Derive hierarchical timeline from authoritative sources."""
    activity_data = derive_index(project_root, now=now)
    events = activity_data.get("events", [])

    # Group events by year, month, day
    # hierarchy: years -> months -> days -> list[event]
    grouped: dict[str, dict[str, dict[str, list[dict[str, Any]]]]] = {}

    for ev in events:
        occurred_at = ev["occurred_at"]
        date_str = occurred_at[:10]
        match = DATE_PATTERN.match(date_str)
        if not match:
            continue
        year, month, day = match.group(1), match.group(2), match.group(3)
        if year not in grouped:
            grouped[year] = {}
        if month not in grouped[year]:
            grouped[year][month] = {}
        if day not in grouped[year][month]:
            grouped[year][month][day] = []
        grouped[year][month][day].append(ev)

    return {
        "event_count": len(events),
        "years": grouped,
    }


def expected_timeline_files(derived: dict[str, Any]) -> dict[Path, str]:
    """Return every generated file relative to the timeline directory."""
    grouped = derived["years"]
    files: dict[Path, str] = {Path("summary.md"): generate_project_summary(grouped)}
    for year, months in grouped.items():
        year_dir = Path("years") / year
        files[year_dir / "summary.md"] = generate_year_summary(year, months)
        for month, days in months.items():
            month_dir = year_dir / month
            files[month_dir / "summary.md"] = generate_month_summary(year, month, days)
            for day, day_events in days.items():
                date_str = f"{year}-{month}-{day}"
                files[month_dir / f"{day}.yaml"] = dump_date_file_content(date_str, day_events)
    return files


def generated_files(timeline_dir: Path) -> set[Path]:
    if not timeline_dir.is_dir():
        return set()
    return {
        path.relative_to(timeline_dir)
        for path in timeline_dir.rglob("*")
        if path.is_file() and (
            path.name == "summary.md" or
            (path.suffix == ".yaml" and path.stem.isdigit() and len(path.stem) == 2)
        )
    }


def persist_timeline(project_root: Path, derived: dict[str, Any]) -> None:
    timeline_dir = project_root / TIMELINE_ROOT
    expected = expected_timeline_files(derived)
    for relative_path, content in expected.items():
        target = timeline_dir / relative_path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(content, encoding="utf-8")
    for relative_path in generated_files(timeline_dir) - expected.keys():
        (timeline_dir / relative_path).unlink()
    years_dir = timeline_dir / "years"
    if years_dir.is_dir():
        for directory in sorted(years_dir.rglob("*"), key=lambda p: len(p.parts), reverse=True):
            if directory.is_dir() and not any(directory.iterdir()):
                directory.rmdir()


def timeline_is_current(project_root: Path, derived: dict[str, Any]) -> bool:
    timeline_dir = project_root / TIMELINE_ROOT
    expected = expected_timeline_files(derived)
    if generated_files(timeline_dir) != expected.keys():
        return False
    return all(
        (timeline_dir / relative_path).read_text(encoding="utf-8") == content
        for relative_path, content in expected.items()
    )


def read_drill_down_summary(
    project_root: Path,
    year: str | None = None,
    month: str | None = None,
) -> str:
    """Read bounded summary at the specified hierarchy level."""
    timeline_dir = project_root / TIMELINE_ROOT
    if year and month:
        target = timeline_dir / "years" / year / month / "summary.md"
        label = f"Month summary ({year}-{month})"
    elif year:
        target = timeline_dir / "years" / year / "summary.md"
        label = f"Year summary ({year})"
    else:
        target = timeline_dir / "summary.md"
        label = "Project timeline summary"

    if not target.is_file():
        raise TimelineError(f"{label} not found at {target}")
    return target.read_text(encoding="utf-8")


def read_date_events(project_root: Path, date_str: str) -> list[dict[str, Any]]:
    """Read daily events list for a given date YYYY-MM-DD."""
    match = DATE_PATTERN.match(date_str)
    if not match:
        raise TimelineError(f"Invalid date format '{date_str}', expected YYYY-MM-DD")
    year, month, day = match.group(1), match.group(2), match.group(3)
    target = project_root / TIMELINE_ROOT / "years" / year / month / f"{day}.yaml"
    if not target.is_file():
        return []
    lines = target.read_text(encoding="utf-8").splitlines()
    events: list[dict[str, Any]] = []
    current_event: dict[str, Any] | None = None
    current_list: list[str] | None = None

    for line in lines:
        if not line.strip() or line.strip().startswith("#"):
            continue
        m_item = re.match(r"^\s*-\s+([A-Za-z_][A-Za-z0-9_-]*):\s*(.*?)\s*$", line)
        if m_item:
            if current_event:
                events.append(current_event)
            key, val = m_item.groups()
            current_event = {key: parse_scalar_val(val)}
            current_list = None
            continue
        m_subref = re.match(r"^\s*-\s+(.+?)\s*$", line)
        if m_subref and current_list is not None and current_event:
            current_list.append(parse_scalar_val(m_subref.group(1)))
            continue
        m_field = re.match(r"^\s+([A-Za-z_][A-Za-z0-9_-]*):\s*(.*?)\s*$", line)
        if m_field and current_event:
            k, v = m_field.groups()
            if not v:
                current_event[k] = []
                current_list = current_event[k]
            else:
                current_event[k] = parse_scalar_val(v)
                current_list = None
    if current_event:
        events.append(current_event)
    return events


def parse_args(argv: list[str]) -> argparse.Namespace:
    common = argparse.ArgumentParser(add_help=False)
    common.add_argument("--project-root", type=Path, default=argparse.SUPPRESS)
    common.add_argument(
        "--now",
        help="Fixed ISO-8601 timestamp for reproducible builds or tests",
        default=argparse.SUPPRESS,
    )

    parser = argparse.ArgumentParser(
        description="Maintain and query XiaoTao's derived Project Memory Timeline.",
        parents=[common],
    )
    subparsers = parser.add_subparsers(dest="command", required=True)
    subparsers.add_parser("build", parents=[common])
    subparsers.add_parser("check", parents=[common])

    summary_parser = subparsers.add_parser("summary", parents=[common])
    summary_parser.add_argument("--year", help="Year YYYY for annual summary")
    summary_parser.add_argument("--month", help="Month MM for monthly summary (requires --year)")

    events_parser = subparsers.add_parser("events", parents=[common])
    events_parser.add_argument("--date", required=True, help="Date YYYY-MM-DD to fetch events")

    args = parser.parse_args(argv)
    if not hasattr(args, "project_root"):
        args.project_root = Path.cwd()
    if not hasattr(args, "now"):
        args.now = None
    return args


def main(argv: list[str] | None = None) -> int:
    args = parse_args(sys.argv[1:] if argv is None else argv)
    try:
        project_root = args.project_root.resolve(strict=True)
        if not project_root.is_dir():
            raise TimelineError(f"project root is not a directory: {project_root}")
        ref_time = resolve_reference_time(args.now)

        if args.command == "build":
            derived = derive_timeline(project_root, now=ref_time)
            persist_timeline(project_root, derived)
            print(
                json.dumps(
                    {
                        "status": "built",
                        "event_count": derived["event_count"],
                        "years": sorted(derived["years"].keys()),
                    },
                    ensure_ascii=False,
                    sort_keys=True,
                )
            )
            return 0

        if args.command == "check":
            if not timeline_is_current(project_root, derive_timeline(project_root, now=ref_time)):
                print("Timeline is missing or stale", file=sys.stderr)
                return 1
            print(json.dumps({"status": "current"}, sort_keys=True))
            return 0

        if args.command == "summary":
            if args.month and not args.year:
                raise TimelineError("--month requires --year to be specified")
            text = read_drill_down_summary(project_root, year=args.year, month=args.month)
            print(text)
            return 0

        if args.command == "events":
            evs = read_date_events(project_root, args.date)
            print(json.dumps({"date": args.date, "events": evs}, ensure_ascii=False, indent=2))
            return 0

    except (TimelineError, OSError, RuntimeError, ValueError) as error:
        print(f"timeline catalog error: {error}", file=sys.stderr)
        return 2

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
