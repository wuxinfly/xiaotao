#!/usr/bin/env python3
"""Authoritative daily notes; no transcript parsing or model invocation."""
from __future__ import annotations

import hashlib
import json
import os
import re
import tempfile
from contextlib import contextmanager
from datetime import date, datetime, timedelta, timezone
from pathlib import Path
from zoneinfo import ZoneInfo, ZoneInfoNotFoundError

ROOT = Path('.xiaotao/memory/timeline')
MAX_BYTES = 32 * 1024
MAX_NOTES = 2048
MAX_TOTAL_BYTES = 8 * 1024 * 1024
ID = re.compile(r'^[a-z0-9][a-z0-9_-]{0,79}$')
FENCE = re.compile(r'^```xiaotao-daily-record\n(.*?)^```$', re.M | re.S)
WORK = re.compile(r'^\.xiaotao/(?:tasks/(?:archive/)?(?P<task>[^/]+)/progress\.md|memory/temporary/(?:active|archive)/(?P<temporary>[^/]+)/current\.md)$')


class DailyError(ValueError):
    pass


def stamp(value):
    if not isinstance(value, str):
        raise DailyError('timestamp must be a string')
    result = datetime.fromisoformat(value.replace('Z', '+00:00'))
    if result.tzinfo is None or result.utcoffset() is None:
        raise DailyError('timestamp needs timezone offset')
    return result


def valid_date(value):
    if not isinstance(value, str) or not re.fullmatch(r'\d{4}-\d{2}-\d{2}', value):
        raise DailyError('Invalid date format; expected YYYY-MM-DD')
    date.fromisoformat(value)
    return value


def zone(name, when):
    if not isinstance(name, str):
        raise DailyError('memory_timezone must be an IANA timezone string')
    try:
        return ZoneInfo(name)
    except ZoneInfoNotFoundError as error:
        # Modern Shanghai has no DST. Windows Python often has no IANA database.
        if name == 'UTC':
            return timezone.utc
        if name == 'Asia/Shanghai' and when.year >= 1992:
            return timezone(timedelta(hours=8))
        raise DailyError('unknown/unavailable timezone; install Python tzdata: ' + name) from error


def safe(root, relative, *, parents=False):
    if not isinstance(relative, str) or '\\' in relative or Path(relative).is_absolute():
        raise DailyError('unsafe project-relative path')
    parts = relative.split('/')
    if any(p in ('', '.', '..') or ':' in p for p in parts):
        raise DailyError('unsafe project-relative path')
    current = root
    for i, part in enumerate(parts):
        current = current / part
        if current.is_symlink():
            raise DailyError('symlink is not a managed path: ' + relative)
        if i < len(parts) - 1:
            if parents:
                current.mkdir(exist_ok=True)
            if current.exists() and not current.is_dir():
                raise DailyError('non-directory parent: ' + relative)
    return current


def read_text(root, relative):
    p = safe(root, relative)
    if not p.is_file() or p.stat().st_size > MAX_BYTES:
        raise DailyError('missing/oversized record: ' + relative)
    raw = p.read_bytes()
    if len(raw) > MAX_BYTES:
        raise DailyError('record size exceeded')
    return raw.decode('utf-8')


def memory_timezone(root, now):
    config = safe(root, '.xiaotao/config.yaml')
    name = 'UTC'
    if config.is_file():
        text = read_text(root, '.xiaotao/config.yaml')
        matches = re.findall(r'^memory_timezone:\s*([^\n#]+)', text, re.M)
        if len(matches) > 1:
            raise DailyError('duplicate memory_timezone')
        if matches:
            name = matches[0].strip().strip('\"\'')
    zone(name, now)
    return name


def day_relative(day):
    valid_date(day)
    y, m, d = day.split('-')
    return (ROOT / 'years' / y / m / d).as_posix()


def note_relative(day, topic):
    if not isinstance(topic, str) or not ID.fullmatch(topic):
        raise DailyError('invalid daily topic ID')
    return day_relative(day) + '/details/' + topic + '.md'


def atomic_text(root, relative, text):
    target = safe(root, relative, parents=True)
    fd, temporary = tempfile.mkstemp(prefix='.daily-', dir=target.parent)
    try:
        with os.fdopen(fd, 'w', encoding='utf-8', newline='\n') as stream:
            stream.write(text)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, target)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


@contextmanager
def timeline_lock(root):
    p = safe(root, '.xiaotao/locks/timeline.lock', parents=True)
    try:
        fd = os.open(p, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
    except FileExistsError as error:
        raise DailyError('timeline writer is busy; retry without removing its lock') from error
    try:
        yield
    finally:
        os.close(fd)
        p.unlink()


def render_note(record):
    return ('---\nrevision: ' + str(record['revision']) + '\nupdated_at: ' + record['updated_at']
            + '\nupdated_by: ' + record['updated_by'] + '\n---\n\n# ' + record['title']
            + '\n\n```xiaotao-daily-record\n' + json.dumps(record, ensure_ascii=False, sort_keys=True, indent=2)
            + '\n```\n\n' + record['summary'] + '\n\n' + record['details'] + '\n')


def required_string(record, key, maximum):
    value = record.get(key)
    if not isinstance(value, str) or not value.strip() or len(value) > maximum or '\x00' in value:
        raise DailyError('invalid ' + key)
    return value


def validate_record(record, relative):
    fields = {'schema_version', 'record_id', 'topic_id', 'date', 'timezone', 'title', 'summary', 'details', 'source_refs', 'work_ref', 'occurred_at', 'revision', 'updated_at', 'updated_by', 'request_id', 'request_hash'}
    if not isinstance(record, dict) or set(record) != fields or record.get('schema_version') != 1:
        raise DailyError('invalid daily record version')
    required_string(record, 'title', 160)
    required_string(record, 'summary', 1000)
    if not isinstance(record.get('details'), str) or len(record['details']) > 6000:
        raise DailyError('invalid details')
    if any('```xiaotao-daily-record' in record[k] for k in ('title', 'summary', 'details')):
        raise DailyError('reserved daily record fence in content')
    required_string(record, 'updated_by', 128)
    if '\n' in record['updated_by'] or '\r' in record['updated_by'] or '\n' in record['title']:
        raise DailyError('invalid metadata newline')
    if type(record.get('revision')) is not int or not 0 <= record['revision'] <= 9007199254740991:
        raise DailyError('invalid revision')
    occurred = stamp(record.get('occurred_at'))
    stamp(record.get('updated_at'))
    if occurred.astimezone(zone(record.get('timezone'), occurred)).date().isoformat() != record.get('date'):
        raise DailyError('record date disagrees with timezone')
    if note_relative(record['date'], record.get('topic_id')) != relative:
        raise DailyError('daily note identity does not match path')
    if record.get('record_id') != 'daily-' + record['date'] + '-' + record['topic_id']:
        raise DailyError('invalid stable daily record ID')
    refs = record.get('source_refs')
    if not isinstance(refs, list) or len(refs) > 16 or len(set(refs)) != len(refs):
        raise DailyError('invalid source_refs')
    for ref in refs:
        if not isinstance(ref, str) or len(ref) > 500 or '\\' in ref or Path(ref).is_absolute() or any(p in ('', '.', '..') or ':' in p for p in ref.split('/')):
            raise DailyError('unsafe source_refs')
    work = record.get('work_ref')
    if work is not None:
        if not isinstance(work, dict) or set(work) != {'kind', 'id', 'path'}:
            raise DailyError('invalid work_ref')
        match = WORK.fullmatch(work['path']) if isinstance(work['path'], str) else None
        if work.get('kind') not in {'task', 'temporary'} or not match or match.group(work['kind']) != work.get('id'):
            raise DailyError('invalid work identity')
        if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9._-]{0,159}', work['id']):
            raise DailyError('unsafe work ID')
    if not re.fullmatch(r'[0-9a-f]{64}', record.get('request_hash', '')):
        raise DailyError('invalid request hash')
    required_string(record, 'request_id', 128)


def load_note(root, relative):
    text = read_text(root, relative)
    blocks = list(FENCE.finditer(text))
    if len(blocks) != 1:
        raise DailyError('daily note must contain one xiaotao-daily-record block')
    record = json.loads(blocks[0].group(1))
    validate_record(record, relative)
    if render_note(record) != text:
        raise DailyError('daily note has unmanaged edits; reconcile them before replacing')
    return record


def resolve_work(root, work):
    if not work:
        return None
    if work['kind'] == 'task':
        candidates = [f".xiaotao/tasks/{work['id']}/progress.md", f".xiaotao/tasks/archive/{work['id']}/progress.md"]
    else:
        candidates = [f".xiaotao/memory/temporary/{state}/{work['id']}/current.md" for state in ('active', 'archive')]
    found = [p for p in candidates if safe(root, p).is_file()]
    if len(found) > 1:
        raise DailyError('duplicate active/archive work identity')
    return found[0] if found else None


def list_notes(root, start=None, end=None):
    years = safe(root, (ROOT / 'years').as_posix())
    total = count = 0
    if not years.exists():
        return
    # Walk only the managed hierarchy, reject symlinks rather than following them.
    for y in sorted(years.iterdir()):
        if not re.fullmatch(r'\d{4}', y.name):
            continue
        safe(root, y.relative_to(root).as_posix())
        if not y.is_dir():
            continue
        for m in sorted(y.iterdir()):
            if not re.fullmatch(r'\d{2}', m.name):
                continue
            safe(root, m.relative_to(root).as_posix())
            if not m.is_dir():
                continue
            for d in sorted(m.iterdir()):
                if not re.fullmatch(r'\d{2}', d.name):
                    continue
                day = valid_date(f'{y.name}-{m.name}-{d.name}')
                safe(root, d.relative_to(root).as_posix())
                if (start and day < start) or (end and day > end):
                    continue
                detail = safe(root, d.relative_to(root).as_posix() + '/details')
                if not detail.is_dir():
                    continue
                for p in sorted(detail.iterdir()):
                    if p.suffix != '.md':
                        continue
                    relative = p.relative_to(root).as_posix()
                    record = load_note(root, relative)
                    count += 1
                    total += p.stat().st_size
                    if count > MAX_NOTES or total > MAX_TOTAL_BYTES:
                        raise DailyError('daily scan budget exceeded; archive/split scope explicitly')
                    yield relative, record


def save_note(root, request, now):
    if not safe(root, '.xiaotao').is_dir():
        raise DailyError('project has no .xiaotao; do not silently initialize')
    if not isinstance(request, dict) or set(request) - {'topic_id', 'title', 'summary', 'details', 'source_refs', 'work_path', 'actor', 'expected_revision', 'request_id', 'occurred_at', 'timezone'}:
        raise DailyError('invalid save request fields')
    actor = required_string(request, 'actor', 128)
    request_id = required_string(request, 'request_id', 128)
    expected = request.get('expected_revision')
    if expected is not None and (type(expected) is not int or expected < 0):
        raise DailyError('expected_revision must be null (create) or a nonnegative integer')
    # Freeze date attribution in the request itself so an identical retry after
    # midnight or a config change cannot create a second note on another day.
    if 'occurred_at' not in request or 'timezone' not in request:
        raise DailyError('save requires occurred_at and timezone; obtain them with context and keep them on retry')
    occurred = stamp(request['occurred_at'])
    zone_name = request['timezone']
    day = occurred.astimezone(zone(zone_name, occurred)).date().isoformat()
    relative = note_relative(day, request.get('topic_id'))
    with timeline_lock(root):
        old = load_note(root, relative) if safe(root, relative).exists() else None
        work_path = request.get('work_path')
        work = None
        if work_path is not None:
            match = WORK.fullmatch(work_path) if isinstance(work_path, str) else None
            if not match:
                raise DailyError('work_path must be a Task progress.md or Temporary current.md')
            kind = 'task' if match.group('task') else 'temporary'
            work = {'kind': kind, 'id': match.group(kind), 'path': work_path}
            if resolve_work(root, work) != work_path:
                raise DailyError('work_path is missing or not its current canonical path')
        record = {
            'schema_version': 1, 'record_id': 'daily-' + day + '-' + request['topic_id'],
            'topic_id': request['topic_id'], 'date': day, 'timezone': zone_name,
            'title': request.get('title'), 'summary': request.get('summary'),
            'details': request.get('details', ''), 'source_refs': request.get('source_refs', []),
            'work_ref': work, 'occurred_at': occurred.isoformat(timespec='seconds').replace('+00:00', 'Z'),
            'revision': 0 if old is None else old['revision'] + 1,
            'updated_at': now.isoformat(timespec='seconds').replace('+00:00', 'Z'), 'updated_by': actor,
            'request_id': request_id,
        }
        # Exact retry is recognized before CAS; other stale writes fail closed.
        semantic = {k: v for k, v in record.items() if k not in {'revision', 'updated_at', 'request_id'}}
        digest = hashlib.sha256(json.dumps(semantic, ensure_ascii=False, sort_keys=True).encode()).hexdigest()
        record['request_hash'] = digest
        validate_record(record, relative)
        if old and old['request_id'] == request_id:
            if old['request_hash'] != digest:
                raise DailyError('request_id reused with different facts')
            return {'status': 'duplicate', 'path': relative, 'revision': old['revision']}
        if (old is None and expected is not None) or (old is not None and expected != old['revision']):
            raise DailyError('revision conflict; read current note before retrying')
        if old and old['work_ref'] and (not work or old['work_ref']['kind'] != work['kind'] or old['work_ref']['id'] != work['id']):
            raise DailyError('cannot rebind a daily topic to different work')
        if old:
            if record['timezone'] != old['timezone'] or record['occurred_at'] != old['occurred_at']:
                raise DailyError('existing note date/timezone/occurred_at is immutable; use its original values')
            record['occurred_at'] = old['occurred_at']
            # A new request with no changed facts does not invent a new revision.
            compare = ('title', 'summary', 'details', 'source_refs', 'work_ref')
            if all(old[k] == record[k] for k in compare):
                return {'status': 'unchanged', 'path': relative, 'revision': old['revision']}
        for ref in record['source_refs']:
            if not safe(root, ref).is_file():
                raise DailyError('source file is missing: ' + ref)
        text = render_note(record)
        if len(text.encode('utf-8')) > MAX_BYTES:
            raise DailyError('daily note exceeds 32 KiB; split the topic')
        atomic_text(root, relative, text)
        verified = load_note(root, relative)
        if verified != record:
            raise DailyError('daily readback mismatch')
        return {'status': 'saved', 'path': relative, 'revision': record['revision']}
