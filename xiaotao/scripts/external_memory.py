#!/usr/bin/env python3
"""Explicitly import external work documents and confirm historical project events."""
from __future__ import annotations
import argparse
import hashlib
import json
from pathlib import Path
import re
import sys
from datetime import datetime, timezone

from activity_catalog import normalize_utc, atomic_write, CatalogError

ROOT = Path('.xiaotao/memory/imports')
MAX_SIZE = 20 * 1024 * 1024


def save(path, value):
    atomic_write(path, json.dumps(value, ensure_ascii=False, indent=2, sort_keys=True) + '\n')


def safe_file(path, root):
    if path.is_symlink() or not path.is_file() or not path.resolve().is_relative_to(root.resolve()):
        raise CatalogError(f'unsafe or missing managed source: {path}')
    return path


def manifests(root):
    for path in sorted((root / ROOT).glob('*/manifest.json')):
        if path.is_symlink():
            raise CatalogError(f'unsafe import manifest: {path}')
        yield path, json.loads(path.read_text(encoding='utf-8'))


def import_files(root, sources):
    existing = {s['sha256']: (p.parent.name, s) for p, m in manifests(root) for s in m['sources']}
    results = []
    for source in sources:
        if source.is_symlink() or not source.is_file() or not source.stat().st_size or source.stat().st_size > MAX_SIZE:
            raise CatalogError(f'source must be a nonempty regular file up to {MAX_SIZE} bytes: {source}')
        data = source.read_bytes()
        digest = hashlib.sha256(data).hexdigest()
        if digest in existing:
            results.append({'status': 'duplicate', 'import_id': existing[digest][0], 'sha256': digest})
            continue
        import_id = 'import-' + digest[:20]
        directory = root / ROOT / import_id
        target = directory / 'sources' / source.name
        if directory.exists():
            raise CatalogError(f'import ID collision: {directory}')
        target.parent.mkdir(parents=True)
        target.write_bytes(data)
        ref = target.relative_to(root).as_posix()
        save(directory / 'manifest.json', {'schema_version': 1, 'import_id': import_id,
             'imported_at': datetime.now(timezone.utc).isoformat(timespec='seconds').replace('+00:00', 'Z'),
             'status': 'pending', 'sources': [{'name': source.name, 'sha256': digest, 'ref': ref}]})
        existing[digest] = (import_id, {})
        results.append({'status': 'pending', 'import_id': import_id, 'source_ref': ref})
    return results


def confirm(root, import_id, proposal_path):
    if not re.fullmatch(r'import-[0-9a-f]{20}', import_id):
        raise CatalogError('invalid import ID')
    directory = root / ROOT / import_id
    manifest_path = directory / 'manifest.json'
    manifest = json.loads(safe_file(manifest_path, root).read_text(encoding='utf-8'))
    if manifest['import_id'] != import_id:
        raise CatalogError('manifest ID mismatch')
    if manifest['status'] == 'confirmed':
        raise CatalogError('already confirmed; immutable record cannot be changed')
    proposal = json.loads(proposal_path.read_text(encoding='utf-8'))
    if not isinstance(proposal, dict) or not isinstance(proposal.get('events'), list):
        raise CatalogError('proposal must contain events array')
    allowed = set()
    for source in manifest['sources']:
        ref = source['ref']
        if not ref.startswith((ROOT / import_id / 'sources').as_posix() + '/'):
            raise CatalogError('source path is outside import')
        target = safe_file(root / ref, root)
        if target.resolve().relative_to(root.resolve()).as_posix() != ref:
            raise CatalogError(f'noncanonical managed source: {ref}')
        if hashlib.sha256(target.read_bytes()).hexdigest() != source['sha256']:
            raise CatalogError(f'source hash mismatch: {ref}')
        allowed.add(ref)
    records = []
    seen = set()
    for event in proposal['events']:
        if not isinstance(event, dict) or set(event) != {'occurred_at', 'title', 'summary', 'source_refs'}:
            raise CatalogError('event needs exactly occurred_at, title, summary, source_refs')
        stamp = normalize_utc(event['occurred_at'])
        if not all(isinstance(event[k], str) and event[k].strip() for k in ('title', 'summary')):
            raise CatalogError('title and summary must be nonempty')
        refs = event['source_refs']
        if not isinstance(refs, list) or not refs or len(refs) != len(set(refs)) or not set(refs) <= allowed:
            raise CatalogError('source_refs must name managed sources of this import')
        event_id = 'external-' + hashlib.sha256((stamp + '\n' + event['title'].strip() + '\n' + '\n'.join(sorted(refs))).encode()).hexdigest()[:24]
        if event_id in seen:
            raise CatalogError('duplicate event in proposal')
        seen.add(event_id)
        records.append({'event_id': event_id, 'occurred_at': stamp, 'title': event['title'].strip(),
                        'summary': event['summary'].strip(), 'source_refs': refs})
    if not records:
        raise CatalogError('empty proposal: use skip instead')
    for path, other in manifests(root):
        if other.get('status') == 'confirmed' and path != manifest_path:
            for record in other.get('events', []):
                if record['event_id'] in seen:
                    raise CatalogError(f'event already confirmed in {path}')
    manifest['events'] = records
    manifest['status'] = 'confirmed'
    manifest['confirmed_at'] = datetime.now(timezone.utc).isoformat(timespec='seconds').replace('+00:00', 'Z')
    save(manifest_path, manifest)
    return records


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--project-root', type=Path, default=Path.cwd())
    sub = parser.add_subparsers(dest='command', required=True)
    common = argparse.ArgumentParser(add_help=False)
    common.add_argument('--project-root', type=Path, default=argparse.SUPPRESS)
    sub.add_parser('import', parents=[common]).add_argument('files', type=Path, nargs='+')
    p = sub.add_parser('confirm', parents=[common]); p.add_argument('import_id'); p.add_argument('proposal', type=Path)
    p = sub.add_parser('skip', parents=[common]); p.add_argument('import_id'); p.add_argument('--reason', required=True)
    sub.add_parser('list', parents=[common])
    args = parser.parse_args()
    try:
        root = args.project_root.resolve(strict=True)
        if args.command == 'import': result = import_files(root, args.files)
        elif args.command == 'confirm': result = confirm(root, args.import_id, args.proposal)
        elif args.command == 'list': result = [m for _, m in manifests(root)]
        else:
            if not re.fullmatch(r'import-[0-9a-f]{20}', args.import_id): raise CatalogError('invalid import ID')
            path = root / ROOT / args.import_id / 'manifest.json'
            m = json.loads(safe_file(path, root).read_text(encoding='utf-8'))
            if m['status'] != 'pending': raise CatalogError('only pending imports can be skipped')
            m['status'] = 'skipped'; m['reason'] = args.reason; save(path, m); result = m
        print(json.dumps(result, ensure_ascii=False, indent=2))
        return 0
    except (CatalogError, OSError, ValueError, KeyError, TypeError) as error:
        print(f'external memory error: {error}', file=sys.stderr)
        return 2

if __name__ == '__main__': raise SystemExit(main())
