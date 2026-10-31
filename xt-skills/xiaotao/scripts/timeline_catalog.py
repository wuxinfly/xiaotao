#!/usr/bin/env python3
"""Save daily project facts and derive bounded year/month/day navigation."""
from __future__ import annotations

import argparse
import hashlib
import json
import re
import posixpath
from urllib.parse import quote
import sys
from datetime import date, datetime, timedelta
from pathlib import Path

from activity_catalog import derive_index, resolve_reference_time
from daily_memory import (DailyError, ROOT, MAX_BYTES, atomic_text, day_relative, list_notes,
                          load_note, memory_timezone, note_relative, read_text, resolve_work,
                          safe, save_note, reconcile_note, stamp, timeline_lock, valid_date, zone)

TIMELINE_ROOT = ROOT
GENERATED = '<!-- xiaotao:timeline-generated:v2 -->\n'
MANAGED = re.compile(r'^(?:summary\.md|years/\d{4}/summary\.md|years/\d{4}/\d{2}/summary\.md|years/\d{4}/\d{2}/\d{2}/(?:summary\.md|events\.yaml))$')
LEGACY_DAY = re.compile(r'^years/(\d{4})/(\d{2})/(\d{2})\.yaml$')

if hasattr(sys.stdout, 'reconfigure'):
    sys.stdout.reconfigure(encoding='utf-8')
    sys.stderr.reconfigure(encoding='utf-8')


class TimelineError(DailyError):
    pass


def short(value, maximum=180):
    return ' '.join(str(value).split())[:maximum].replace('[', '［').replace(']', '］')


def event_summary(event):
    return short(event.get('summary') or event.get('title') or '事件')


def derive_timeline(project_root, *, now=None, day=None):
    now = now or resolve_reference_time()
    name = memory_timezone(project_root, now)
    tz = zone(name, now)
    events = list(derive_index(project_root, now=now).get('events', []))
    issues = []
    for relative, record in list_notes(project_root, start=day, end=day, issues=issues):
        work_path = resolve_work(project_root, record['work_ref'])
        events.append({
            'event_id': record['record_id'], 'event_type': 'daily_note',
            'summary': record['summary'], 'title': record['title'],
            'occurred_at': record['occurred_at'], 'date': record['date'],
            'source_refs': [relative, *record['source_refs'], *([work_path] if work_path else [])],
            'detail_ref': relative, 'work_ref': record['work_ref'],
            'work_path': work_path, 'missing_work': bool(record['work_ref'] and not work_path),
        })
    grouped = {}
    identities = set()
    for event in sorted(events, key=lambda e: (e['occurred_at'], e['event_id'])):
        event_day = event.get('date') or stamp(event['occurred_at']).astimezone(tz).date().isoformat()
        if day is not None and event_day != day:
            continue
        if event['event_id'] in identities:
            raise TimelineError('duplicate timeline event ID')
        identities.add(event['event_id'])
        y, m, d = valid_date(event_day).split('-')
        grouped.setdefault(y, {}).setdefault(m, {}).setdefault(d, []).append(event)
    return {'event_count': len(identities), 'years': grouped, 'timezone': name, 'issues': issues}


def highlight_values(events, limit=3):
    return list(dict.fromkeys(event_summary(e) for e in reversed(events)))[:limit]


def navigation_summary(level, label, children):
    """Parents use only bounded highlights from direct child summaries."""
    values = list(dict.fromkeys(v for _, highlights_ in reversed(children) for v in highlights_))[:3]
    meta = '<!-- xiaotao:timeline-highlights:' + json.dumps(values, ensure_ascii=False) + ' -->\n'
    if level == 'root':
        lines = ['# 项目经历总览 (Project Memory Timeline)', '', '## 年度导航与重点', '']
        for key, hints in reversed(children):
            lines.append(f'- [{key} 年度总览](years/{key}/summary.md)：' + '；'.join(hints))
        if not children:
            lines.append('尚无已保存的日常记忆或项目事件。')
    elif level == 'year':
        lines = [f'# 项目记忆年度总览：{label}', '', '## 月度导航与重点', '']
        lines.extend(f'- [{key} 月]({key}/summary.md)：' + '；'.join(hints) for key, hints in children)
    else:
        lines = [f'# 项目记忆月度摘要：{label}', '', '## 每日讨论、决定与推进', '']
        lines.extend(f'- [{label}-{key}]({key}/summary.md)：' + '；'.join(hints[:2]) for key, hints in children)
    return GENERATED + meta + '\n'.join(lines) + '\n'


def summary_highlights(text):
    match = re.search(r'^<!-- xiaotao:timeline-highlights:(.*?) -->$', text, re.M)
    if not text.startswith(GENERATED) or not match:
        raise TimelineError('navigation needs one full build before incremental refresh')
    values = json.loads(match[1])
    if not isinstance(values, list) or len(values) > 3 or any(not isinstance(v, str) for v in values):
        raise TimelineError('invalid navigation highlights')
    return values


def expected_timeline_files(derived):
    years = derived['years']
    files = {}
    for issue in derived.get('issues', []):
        parts = Path(issue['path']).parts
        y, m, d = parts[-5:-2]
        years.setdefault(y, {}).setdefault(m, {}).setdefault(d, [])
    root_children = []
    for y, months in sorted(years.items()):
        month_children = []
        for m, days in sorted(months.items()):
            day_children = []
            for d, events in sorted(days.items()):
                day = f'{y}-{m}-{d}'
                day_children.append((d, highlight_values(events)))
                daily = [f'# 项目记忆日详情：{day}', '', f'共 {len(events)} 项记录；下方最多展示 50 项。', '']
                for issue in derived.get('issues', []):
                    if issue['path'].startswith(day_relative(day) + '/'):
                        daily.append('- 待修复记录：' + issue['path'] + '；运行 reconcile 后重新 build。')
                for event in events[:50]:
                    detail = event.get('detail_ref')
                    if detail:
                        daily.append(f"- [{short(event['title'], 80)}](details/{Path(detail).name})：{event_summary(event)}")
                        if event.get('work_path'):
                            # Relative link from this day directory to the current canonical target.
                            link = posixpath.relpath(event['work_path'], day_relative(day))
                            daily.append(f"  - [{event['work_ref']['kind']} 详情]({quote(link, safe='/')})")
                        elif event.get('missing_work'):
                            daily.append('  - 原工作记录暂不可达；保留稳定 ID，不猜测替代任务。')
                    else:
                        daily.append(f"- **{event['event_type']}**：{event_summary(event)}")
                        for ref in event.get('source_refs', [])[:3]:
                            daily.append(f'- [来源]({quote(posixpath.relpath(ref, day_relative(day)), safe='/')})')
                if len(events) > 50:
                    daily.append(f'\n更多记录：events --date {day} --offset 50 --limit 50。')
                prefix = Path('years') / y / m / d
                files[prefix / 'summary.md'] = GENERATED + '<!-- xiaotao:timeline-highlights:' + json.dumps(highlight_values(events), ensure_ascii=False) + ' -->\n' + '\n'.join(daily) + '\n'
                # A bounded derived machine view, not an authoritative daily journal.
                rows = ['date: ' + json.dumps(day), 'events:']
                for e in events[:50]:
                    rows.append('  - event_id: ' + json.dumps(e['event_id']))
                    rows.append('    event_type: ' + e['event_type'])
                    rows.append('    summary: ' + json.dumps(event_summary(e), ensure_ascii=False))
                    rows.append('    occurred_at: ' + e['occurred_at'])
                    rows.append('    source_refs: ' + json.dumps(e.get('source_refs', []), ensure_ascii=False))
                files[prefix / 'events.yaml'] = GENERATED.replace('<!-- ', '# ').replace(' -->', '') + '\n'.join(rows) + '\n'
            month_text = navigation_summary('month', f'{y}-{m}', day_children)
            files[Path('years') / y / m / 'summary.md'] = month_text
            month_children.append((m, summary_highlights(month_text)))
        year_text = navigation_summary('year', y, month_children)
        files[Path('years') / y / 'summary.md'] = year_text
        root_children.append((y, summary_highlights(year_text)))
    files[Path('summary.md')] = navigation_summary('root', '', root_children)
    if any(len(t.encode('utf-8')) > MAX_BYTES for t in files.values()):
        raise TimelineError('navigation exceeds read budget; split large source summaries')
    return files


def generated_files(root):
    directory = safe(root, ROOT.as_posix())
    result = set()
    if not directory.is_dir():
        return result
    for count, p in enumerate(directory.rglob('*'), 1):
        if count > 10000:
            raise TimelineError('timeline file scan budget exceeded')
        relative = p.relative_to(directory).as_posix()
        if MANAGED.fullmatch(relative) or LEGACY_DAY.fullmatch(relative):
            safe(root, (ROOT / relative).as_posix())
            if p.is_file():
                result.add(Path(relative))
        elif p.is_symlink():
            raise TimelineError('symlink inside timeline')
    return result


def generated_owner(relative, text):
    if text.startswith(GENERATED) or text.startswith('# xiaotao:timeline-generated:v2'):
        return True
    # Recognize only v1's exact path depth and heading; never claim a details/summary.md.
    if relative.name == 'summary.md' and len(relative.parts) in (1, 3, 4):
        return text.startswith(('# 项目经历总览', '# 项目记忆年度总览：', '# 项目记忆月度摘要：'))
    return False


def persist_timeline(root, derived):
    expected = expected_timeline_files(derived)
    existing = generated_files(root)
    for relative in existing:
        text = read_text(root, (ROOT / relative).as_posix())
        if not LEGACY_DAY.fullmatch(relative.as_posix()) and not generated_owner(relative, text):
            raise TimelineError('unmanaged navigation file; preserve/reconcile before build: ' + str(relative))
    # Preserve recognized v1 navigation before replacing it too: manual additions
    # cannot be distinguished from the old generator's output by heading alone.
    for relative in existing:
        original = read_text(root, (ROOT / relative).as_posix())
        if relative.name == 'summary.md' and not original.startswith(GENERATED):
            digest = hashlib.sha256(original.encode()).hexdigest()[:16]
            name = '-'.join(relative.with_suffix('').parts) + '-' + digest + '.md'
            snapshot = (ROOT / 'legacy-navigation' / name).as_posix()
            if not safe(root, snapshot).exists():
                atomic_text(root, snapshot, original)
            elif read_text(root, snapshot) != original:
                raise TimelineError('legacy navigation snapshot collision')
    # Preserve v1 day files as audit snapshots, including files no longer derivable from sources.
    for relative in existing:
        match = LEGACY_DAY.fullmatch(relative.as_posix())
        if match:
            original = read_text(root, (ROOT / relative).as_posix())
            day = valid_date('-'.join(match.groups()))
            digest = hashlib.sha256(original.encode()).hexdigest()[:16]
            snapshot = day_relative(day) + '/legacy-events-' + digest + '.yaml'
            p = safe(root, snapshot)
            if not p.exists():
                atomic_text(root, snapshot, original)
            elif read_text(root, snapshot) != original:
                raise TimelineError('legacy snapshot collision')
    for relative, content in expected.items():
        atomic_text(root, (ROOT / relative).as_posix(), content)
    for relative in existing - expected.keys():
        safe(root, (ROOT / relative).as_posix()).unlink()


def _refresh_day(root, day, *, now=None):
    """Refresh one day and its parents; never scan other days' detail records."""
    y, m, d = valid_date(day).split('-')
    with timeline_lock(root):
        derived = derive_timeline(root, now=now, day=day)
        expected = expected_timeline_files(derived)
        prefix = Path('years') / y / m / d
        updates = {p: t for p, t in expected.items() if p.parent == prefix}
        def children(parent, pattern):
            directory = safe(root, (ROOT / parent).as_posix())
            result = []
            if directory.is_dir():
                for child in sorted(directory.iterdir()):
                    if not re.fullmatch(pattern, child.name):
                        continue
                    relative = parent / child.name / 'summary.md'
                    text = updates.get(relative)
                    if text is None:
                        file = safe(root, (ROOT / relative).as_posix())
                        if not file.is_file():
                            raise TimelineError('missing child navigation; run build once')
                        text = read_text(root, (ROOT / relative).as_posix())
                    result.append((child.name, summary_highlights(text)))
                    if len(result) > 256:
                        raise TimelineError('navigation child budget exceeded')
            return result
        # Include the newly saved day even before its navigation exists on disk.
        def with_current(items, key, relative):
            values = dict(items)
            values[key] = summary_highlights(updates[relative])
            return sorted(values.items())
        month = Path('years') / y / m
        month_items = with_current(children(month, r'\d{2}'), d, prefix / 'summary.md')
        updates[month / 'summary.md'] = navigation_summary('month', f'{y}-{m}', month_items)
        year = Path('years') / y
        year_items = with_current(children(year, r'\d{2}'), m, month / 'summary.md')
        updates[year / 'summary.md'] = navigation_summary('year', y, year_items)
        root_items = with_current(children(Path('years'), r'\d{4}'), y, year / 'summary.md')
        updates[Path('summary.md')] = navigation_summary('root', '', root_items)
        if any(len(text.encode('utf-8')) > MAX_BYTES for text in updates.values()):
            raise TimelineError('navigation exceeds read budget; split large source summaries')
        for relative in updates:
            p = safe(root, (ROOT / relative).as_posix())
            if p.exists() and not (read_text(root, (ROOT / relative).as_posix()).startswith(GENERATED) or read_text(root, (ROOT / relative).as_posix()).startswith('# xiaotao:timeline-generated:v2')):
                raise TimelineError('unmanaged navigation; preserve it before build')
        for relative, text in updates.items():
            atomic_text(root, (ROOT / relative).as_posix(), text)
        return derived.get('issues', [])


def refresh_day(root, day, *, now=None):
    try:
        return _refresh_day(root, day, now=now)
    except TimelineError as error:
        # Upgrade pre-highlight navigation once; routine saves take the local path.
        if not str(error).startswith(('navigation needs one full build', 'missing child navigation')):
            raise
        with timeline_lock(root):
            derived = derive_timeline(root, now=now)
            persist_timeline(root, derived)
            return derived.get('issues', [])


def timeline_is_current(root, derived):
    expected = expected_timeline_files(derived)
    if generated_files(root) != expected.keys():
        return False
    return all(read_text(root, (ROOT / p).as_posix()) == text for p, text in expected.items())


def read_drill_down_summary(root, year=None, month=None, day=None):
    if day:
        target = day_relative(day) + '/summary.md'
    else:
        relative = ROOT
        if year:
            if not re.fullmatch(r'\d{4}', year):
                raise TimelineError('invalid year')
            relative = relative / 'years' / year
        if month:
            if not year or not re.fullmatch(r'0[1-9]|1[0-2]', month):
                raise TimelineError('--month requires --year and a valid month')
            relative = relative / month
        target = (relative / 'summary.md').as_posix()
    return read_text(root, target)


def parse_args(argv):
    common = argparse.ArgumentParser(add_help=False)
    common.add_argument('--project-root', type=Path, default=argparse.SUPPRESS)
    common.add_argument('--now', default=argparse.SUPPRESS)
    parser = argparse.ArgumentParser(description=__doc__, parents=[common])
    commands = parser.add_subparsers(dest='command', required=True)
    for c in ('build', 'check', 'save', 'context'):
        commands.add_parser(c, parents=[common])
    reconcile = commands.add_parser('reconcile', parents=[common])
    reconcile.add_argument('path')
    summary = commands.add_parser('summary', parents=[common])
    summary.add_argument('--year')
    summary.add_argument('--month')
    summary.add_argument('--date')
    events = commands.add_parser('events', parents=[common])
    events.add_argument('--date', required=True)
    events.add_argument('--offset', type=int, default=0)
    events.add_argument('--limit', type=int, default=20)
    show = commands.add_parser('show', parents=[common])
    show.add_argument('--date', required=True)
    show.add_argument('--topic', required=True)
    search = commands.add_parser('search', parents=[common])
    search.add_argument('query')
    search.add_argument('--from', dest='start')
    search.add_argument('--to', dest='end')
    search.add_argument('--limit', type=int, default=5)
    args = parser.parse_args(argv)
    if not hasattr(args, 'project_root'):
        args.project_root = Path.cwd()
    if not hasattr(args, 'now'):
        args.now = None
    return args


def main(argv=None):
    args = parse_args(sys.argv[1:] if argv is None else argv)
    try:
        root = args.project_root.resolve(strict=True)
        now = resolve_reference_time(args.now)
        if args.command == 'context':
            name = memory_timezone(root, now)
            print(json.dumps({'timezone': name, 'occurred_at': now.isoformat(timespec='seconds').replace('+00:00', 'Z'),
                              'date': now.astimezone(zone(name, now)).date().isoformat()}))
        elif args.command == 'reconcile':
            print(json.dumps(reconcile_note(root, args.path, now), ensure_ascii=False))
        elif args.command == 'save':
            raw = sys.stdin.buffer.read(MAX_BYTES + 1)
            if len(raw) > MAX_BYTES:
                raise TimelineError('save input exceeds 32 KiB')
            result = save_note(root, json.loads(raw), now)
            try:
                with timeline_lock(root):
                    derived = derive_timeline(root, now=now)
                    persist_timeline(root, derived)
                result['navigation_status'] = 'partial' if derived['issues'] else 'current'
                result['navigation_issues'] = derived['issues']
            except (DailyError, OSError, ValueError) as error:
                # The note has committed; retry the same request repairs only derived navigation.
                result['navigation_status'] = 'pending'
                result['navigation_error'] = str(error)
                result['navigation_next_command'] = 'python .agents/skills/xiaotao/scripts/timeline_catalog.py build'
            print(json.dumps(result, ensure_ascii=False))
        elif args.command == 'build':
            with timeline_lock(root):
                derived = derive_timeline(root, now=now)
                persist_timeline(root, derived)
            print(json.dumps({'status': 'partial' if derived['issues'] else 'built', 'issues': derived['issues'], 'event_count': derived['event_count'], 'years': sorted(derived['years']), 'timezone': derived['timezone']}))
        elif args.command == 'check':
            with timeline_lock(root):
                derived = derive_timeline(root, now=now)
                current = not derived['issues'] and timeline_is_current(root, derived)
            if not current:
                print('Timeline is missing or stale', file=sys.stderr)
                return 1
            print(json.dumps({'status': 'current'}))
        elif args.command == 'summary':
            if args.date and (args.year or args.month):
                raise TimelineError('--date cannot be combined with --year/--month')
            print(read_drill_down_summary(root, args.year, args.month, args.date))
        elif args.command == 'events':
            day = valid_date(args.date)
            if not 1 <= args.limit <= 50 or args.offset < 0:
                raise TimelineError('events limit must be 1..50 and offset nonnegative')
            y, m, d = day.split('-')
            derived = derive_timeline(root, now=now, day=day)
            all_events = derived['years'].get(y, {}).get(m, {}).get(d, [])
            print(json.dumps({'date': day, 'issues': derived['issues'], 'total': len(all_events), 'events': all_events[args.offset:args.offset + args.limit]}, ensure_ascii=False))
        elif args.command == 'show':
            relative = note_relative(args.date, args.topic)
            record = load_note(root, relative)
            work_path = resolve_work(root, record['work_ref'])
            print(json.dumps({'path': relative, 'record': record, 'work_path': work_path,
                              'missing_work': bool(record['work_ref'] and not work_path)}, ensure_ascii=False))
        elif args.command == 'search':
            if not 1 <= args.limit <= 20 or not args.query.strip():
                raise TimelineError('search requires a query and limit 1..20')
            today = now.astimezone(zone(memory_timezone(root, now), now)).date()
            end = valid_date(args.end) if args.end else today.isoformat()
            start = valid_date(args.start) if args.start else (date.fromisoformat(end) - timedelta(days=30)).isoformat()
            if start > end:
                raise TimelineError('--from is after --to')
            candidates = []
            for p, record in list_notes(root, start, end):
                text = (record['title'] + ' ' + record['summary']).casefold()
                if args.query.casefold() in text:
                    candidates.append({'record_id': record['record_id'], 'date': record['date'],
                                       'topic_id': record['topic_id'], 'title': record['title'],
                                       'summary': short(record['summary']), 'path': p, 'revision': record['revision']})
            candidates.sort(key=lambda x: (x['date'], x['record_id']), reverse=True)
            print(json.dumps({'from': start, 'to': end, 'total': len(candidates), 'candidates': candidates[:args.limit]}, ensure_ascii=False))
        return 0
    except (DailyError, OSError, RuntimeError, ValueError, TypeError, KeyError, IndexError) as error:
        print('timeline catalog error: ' + str(error), file=sys.stderr)
        return 2


if __name__ == '__main__':
    raise SystemExit(main())
