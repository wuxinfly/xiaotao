#!/usr/bin/env python3
"""One bounded, resumable stage save. No model calls or transcript parsing."""
from __future__ import annotations

import argparse
from contextlib import contextmanager
from datetime import datetime, timedelta, timezone
import hashlib
import json
import os
from pathlib import Path
import re
import sys

from daily_memory import (DailyError, MAX_BYTES, WORK, atomic_text, load_note,
                          memory_timezone, note_relative, read_text, render_note, resolve_work, required_string,
                          safe, save_note, stamp, validate_record, zone)
from timeline_catalog import refresh_day

JOURNAL_LIMIT = 128 * 1024


def digest(value):
    return hashlib.sha256(value.encode('utf-8')).hexdigest()


@contextmanager
def exclusive(root, relative, owner):
    """Same flat state key as DSH; never steal held or expired locks."""
    p = safe(root, relative, parents=True)
    try:
        fd = os.open(p, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
    except FileExistsError as error:
        raise DailyError('writer busy; keep the original request and retry later') from error
    try:
        now = datetime.now(timezone.utc)
        with os.fdopen(fd, 'w', encoding='utf-8') as stream:
            json.dump({'owner': owner, 'state': 'held', 'acquiredAt': now.isoformat(),
                       'expiresAt': (now + timedelta(minutes=5)).isoformat()}, stream)
        yield
    finally:
        p.unlink()


def normalize(request):
    allowed = {'request_id', 'topic_id', 'title', 'summary', 'details', 'validation',
               'next_steps', 'source_refs', 'work_path', 'actor'}
    if not isinstance(request, dict) or set(request) - allowed:
        raise DailyError('invalid save-progress fields')
    result = dict(request)
    result.setdefault('actor', 'xiao-tao/progress')
    for key, limit in [('request_id', 128), ('topic_id', 80), ('title', 160),
                       ('summary', 1000), ('actor', 128)]:
        required_string(result, key, limit)
    for key in ('details', 'validation', 'next_steps'):
        value = result.setdefault(key, '')
        if not isinstance(value, str) or len(value) > 4000 or '\x00' in value:
            raise DailyError('invalid ' + key)
    result.setdefault('source_refs', [])
    result.setdefault('work_path', None)
    return result


def facts(request):
    parts = [request['summary']]
    for key, heading in [('details', '说明'), ('validation', '验证'), ('next_steps', '下一步')]:
        if request[key].strip():
            parts.append('### ' + heading + '\n' + request[key])
    return '\n\n'.join(parts)


def daily_facts(request):
    # Linked work owns detailed progress; the day keeps only a concise summary.
    return facts({**request, 'details': ''}) if request['work_path'] else facts(request)


def work_snapshot(root, relative, request, now, before=None):
    if not isinstance(relative, str) or not WORK.fullmatch(relative):
        raise DailyError('work_path must name an existing Task progress.md or Temporary current.md')
    text = read_text(root, relative) if before is None else before
    front = re.match(r'^---\r?\n(.*?)\r?\n---(?:\r?\n|$)', text, re.S)
    if not front:
        raise DailyError('work progress needs revision front matter; do not invent missing state')
    metadata = front[1].replace('\r\n', '\n')
    revision = re.findall(r'^revision: (\d+)\s*$', metadata, re.M)
    if len(revision) != 1 or int(revision[0]) >= 9007199254740991:
        raise DailyError('invalid work revision')
    for key in ('updated_at', 'updated_by'):
        if len(re.findall('^' + key + r': .+$', metadata, re.M)) != 1:
            raise DailyError('invalid work metadata')
    revision = int(revision[0])
    updated = now.isoformat(timespec='seconds').replace('+00:00', 'Z')
    for key, value in [('revision', str(revision + 1)), ('updated_at', updated),
                       ('updated_by', json.dumps(request['actor'], ensure_ascii=False))]:
        metadata = re.sub('^' + key + r': .*$', lambda _: key + ': ' + value, metadata, flags=re.M)
    entry = '\n\n## ' + request['title'] + '\n\n' + facts(request) + '\n'
    content = '---\n' + metadata + '\n---\n' + text[front.end():].rstrip() + entry
    if len(content.encode()) > MAX_BYTES:
        raise DailyError('work progress exceeds 32 KiB; reconcile it explicitly before saving')
    return {'path': relative, 'before': text, 'before_hash': digest(text), 'content': content,
            'after_hash': digest(content), 'revision': revision + 1}


def prepare(root, request, now):
    name = memory_timezone(root, now)
    day = now.astimezone(zone(name, now)).date().isoformat()
    relative = note_relative(day, request['topic_id'])
    old = load_note(root, relative) if safe(root, relative).exists() else None
    work_path = request['work_path']
    if old and old['work_ref'] and work_path is None:
        raise DailyError('existing topic is linked to work; supply its work_path')
    body = daily_facts(request)
    refs = list(dict.fromkeys([*(old['source_refs'] if old else []), *request['source_refs']]))
    # Same stage facts can be submitted in a later turn without manufacturing writes.
    same = bool(old and old['title'] == request['title'] and old['summary'] == request['summary']
                and old['details'].endswith(body) and old['source_refs'] == refs
                and (old['work_ref']['path'] if old['work_ref'] else None) == work_path)
    details = body if not old else old['details'] + ('\n\n## ' + request['title'] + '\n\n' + body)
    if same:
        details = old['details']
    occurred = old['occurred_at'] if old else now.isoformat(timespec='seconds').replace('+00:00', 'Z')
    daily = {'topic_id': request['topic_id'], 'title': request['title'],
             'summary': request['summary'], 'details': details, 'source_refs': refs,
             'actor': request['actor'], 'request_id': request['request_id'],
             'occurred_at': occurred, 'timezone': old['timezone'] if old else name,
             'expected_revision': old['revision'] if old else None}
    match = WORK.fullmatch(work_path) if isinstance(work_path, str) else None
    work = None
    if work_path is not None:
        if not match:
            raise DailyError('invalid work_path')
        kind = 'task' if match.group('task') else 'temporary'
        work = {'kind': kind, 'id': match.group(kind), 'path': work_path}
        daily['work_path'] = work_path
    # Validate the entire daily payload before writing work progress.
    candidate = {'schema_version': 1, 'record_id': 'daily-' + day + '-' + request['topic_id'],
                 'date': day, 'work_ref': work, 'revision': 0 if old is None else old['revision'] + 1,
                 'updated_at': now.isoformat(), 'updated_by': request['actor'], 'request_hash': '0' * 64,
                 **{k: v for k, v in daily.items() if k not in {'work_path', 'actor', 'expected_revision'}}}
    validate_record(candidate, relative)
    if len(render_note(candidate).encode()) > MAX_BYTES:
        raise DailyError('daily note exceeds 32 KiB; split the topic')
    if work and resolve_work(root, work) != work_path:
        raise DailyError('work_path is not the current canonical path')
    for ref in request['source_refs']:
        if not safe(root, ref).is_file():
            raise DailyError('missing source file: ' + ref)
    if old and old['work_ref'] and (not work or (work['kind'], work['id']) != (old['work_ref']['kind'], old['work_ref']['id'])):
        raise DailyError('cannot rebind daily work')
    snapshot = work_snapshot(root, work_path, request, now) if work_path else None
    # An unchanged daily summary alone cannot prove linked progress is unchanged.
    if same and snapshot:
        current = read_text(root, work_path)
        same = current.rstrip().endswith('## ' + request['title'] + '\n\n' + facts(request))
        if not same:
            daily['details'] = old['details'] + '\n\n## ' + request['title'] + '\n\n' + body
            if len(daily['details']) > 6000:
                raise DailyError('daily details exceed 6000 characters; split the topic')
    return {'daily': daily, 'work': snapshot, 'day': day, 'unchanged': same}


def save_progress(root, raw, now=None):
    now = now or datetime.now(timezone.utc)
    request = normalize(raw)
    if not safe(root, '.xiaotao').is_dir():
        raise DailyError('project has no .xiaotao; do not silently initialize')
    key = digest(request['request_id'])
    journal_path = '.xiaotao/runtime/progress-saves/' + key + '.json'
    input_hash = digest(json.dumps(request, ensure_ascii=False, sort_keys=True))
    with exclusive(root, '.xiaotao/locks/progress-save-' + key + '.lock', request['actor']):
        p = safe(root, journal_path)
        if p.exists():
            if p.stat().st_size > JOURNAL_LIMIT:
                raise DailyError('save recovery record exceeds budget')
            journal = json.loads(p.read_text(encoding='utf-8'))
            if journal['input_hash'] != input_hash:
                raise DailyError('request_id reused with different facts')
        else:
            journal = {'input_hash': input_hash, 'prepared_at': now.isoformat(), **prepare(root, request, now)}
            encoded = json.dumps(journal, ensure_ascii=False)
            if len(encoded.encode()) > JOURNAL_LIMIT:
                raise DailyError('save recovery record exceeds budget; split the stage')
            atomic_text(root, journal_path, encoded)
        # A recovery journal is project data, never authority for a new write target.
        daily = journal['daily']
        if (daily['topic_id'] != request['topic_id'] or daily['title'] != request['title']
                or daily['summary'] != request['summary'] or daily['request_id'] != request['request_id']
                or daily.get('work_path') != request['work_path'] or daily['actor'] != request['actor']
                or not daily['details'].endswith(daily_facts(request))):
            raise DailyError('invalid save recovery identity')
        snapshot = journal['work']
        if bool(snapshot) != bool(request['work_path']):
            raise DailyError('invalid save recovery work')
        if snapshot:
            verified = work_snapshot(root, request['work_path'], request,
                                     stamp(journal['prepared_at']), before=snapshot['before'])
            if verified != snapshot:
                raise DailyError('invalid save recovery snapshot')
        result = {'request_id': request['request_id'], 'status': 'failed', 'work_status': 'pending' if journal['work'] else 'not_requested',
                  'daily_status': 'pending', 'navigation_status': 'pending', 'path': None}
        try:
            snapshot = journal['work']
            if snapshot:
                match = WORK.fullmatch(snapshot['path'])
                kind = 'task' if match.group('task') else 'temporary'
                relative = resolve_work(root, {'kind': kind, 'id': match.group(kind), 'path': snapshot['path']})
                if relative is None:
                    raise DailyError('linked work is missing; preserve the saved request')
                lock = '.xiaotao/locks/' + relative.removeprefix('.xiaotao/').replace('/', '-') + '.lock'
                with exclusive(root, lock, request['actor']):
                    current = read_text(root, relative)
                    if journal['unchanged']:
                        if digest(current) != snapshot['before_hash']:
                            raise DailyError('work changed after unchanged save; reconcile newer progress')
                        result['work_status'] = 'unchanged'
                    elif digest(current) == snapshot['after_hash']:
                        result['work_status'] = 'duplicate'
                    elif digest(current) != snapshot['before_hash']:
                        raise DailyError('work revision/hash conflict; preserve newer progress and reconcile explicitly')
                    else:
                        if relative != snapshot['path']:
                            raise DailyError('work moved before commit; reconcile explicitly')
                        atomic_text(root, relative, snapshot['content'])
                        if digest(read_text(root, relative)) != snapshot['after_hash']:
                            raise DailyError('work readback mismatch')
                        result['work_status'] = 'saved'
            saved = save_note(root, journal['daily'], now)
            result.update(status=saved['status'], daily_status=saved['status'],
                          path=saved['path'], revision=saved['revision'])
            current = load_note(root, saved['path'])
            if current['revision'] != saved['revision'] or any(current[k] != journal['daily'][k] for k in ('title', 'summary', 'details', 'source_refs')):
                raise DailyError('daily changed before final readback; reconcile newer progress')
            result['daily_sha256'] = hashlib.sha256(safe(root, saved['path']).read_bytes()).hexdigest()
            if snapshot:
                result['work_paths'] = [relative]
                expected_hash = snapshot['before_hash'] if journal['unchanged'] else snapshot['after_hash']
                if digest(read_text(root, relative)) != expected_hash:
                    raise DailyError('work changed before final readback; reconcile newer progress')
                result['work_sha256'] = expected_hash
            try:
                issues = refresh_day(root, journal['day'], now=now)
                result['navigation_status'] = 'partial' if issues else 'current'
                result['navigation_issues'] = issues
            except (DailyError, OSError, ValueError) as error:
                result['navigation_error'] = str(error)
                result['navigation_next_command'] = 'python .agents/skills/xiaotao/scripts/timeline_catalog.py build'
        except (DailyError, OSError, ValueError) as error:
            result['status'] = 'failed'
            result['error'] = str(error)
            # Existing commits survive a later phase's failure.
        return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--project-root', type=Path, default=Path.cwd())
    parser.add_argument('--now', help=argparse.SUPPRESS)
    args = parser.parse_args()
    try:
        raw = sys.stdin.buffer.read(MAX_BYTES + 1)
        if len(raw) > MAX_BYTES:
            raise DailyError('save-progress input exceeds 32 KiB')
        result = save_progress(args.project_root.resolve(strict=True), json.loads(raw),
                               stamp(args.now) if args.now else None)
        print(json.dumps(result, ensure_ascii=False))
        return 1 if result['status'] == 'failed' else 0
    except (DailyError, OSError, ValueError, KeyError, TypeError) as error:
        print(json.dumps({'status': 'failed', 'error': str(error)}, ensure_ascii=False))
        return 1


if __name__ == '__main__':
    if hasattr(sys.stdout, 'reconfigure'):
        sys.stdout.reconfigure(encoding='utf-8')
    sys.exit(main())
