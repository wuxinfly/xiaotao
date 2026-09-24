#!/usr/bin/env python3
"""Build, check, and search XiaoTao's derived Activity Timeline.

Activity is reconstructed from authoritative project records. Supported sources are completed
Tasks carrying ``completed_at``, Tasks promoted from Temporary carrying ``promoted_at``, immutable Decision Records under the Long-term Memory decisions
directory carrying ``decided_at``, and immutable Playbook Decision Records under the Playbook
decisions directory carrying ``decided_at``. No event journal is created, file mtimes are never
treated as event time, and records without a reliable timestamp are left out rather than guessed.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import sys
import tempfile
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Iterable

from memory_catalog import parse_simple_yaml
from validate import (
    Diagnostic,
    FileReferenceValidator,
    validate_activity_event,
    validate_activity_index,
    validate_checkpoint_observation,
    validate_decision_record,
    validate_worker_approval,
)


ACTIVITY_ROOT = Path(".xiaotao/activity")
INDEX_PATH = ACTIVITY_ROOT / "index.json"
TASKS_ROOT = Path(".xiaotao/tasks")
DECISIONS_ROOT = Path(".xiaotao/memory/long-term/decisions")
PLAYBOOK_DECISIONS_ROOT = Path(".xiaotao/playbooks/decisions")
IMPORTS_ROOT = Path(".xiaotao/memory/imports")
WORKER_APPROVALS_ROOT = Path(".xiaotao/workers/approvals")
TEMPORARY_ROOT = Path(".xiaotao/memory/temporary")
DECISION_SUFFIX = ".decision.json"
WORKER_APPROVAL_SUFFIX = ".approval.json"
EVENT_TYPES = {
    "task_completed",
    "temporary_promoted",
    "decision_approved",
    "decision_superseded",
    "playbook_approved",
    "playbook_superseded",
    "checkpoint_recovered",
    "worker_approved",
    "external_work",
}
TASK_EVENT_STATUSES = {"completed", "archive"}
# ``promoted_at`` is only written when the promotion transaction commits, so a ``preparing`` Task
# should never carry it. The status gate keeps a malformed record from projecting an event for a
# promotion that has not become logically visible yet.
PROMOTION_EVENT_STATUSES = {"active", "completed", "cancelled", "archive"}
MONTH_PATTERN = re.compile(r"^\d{4}-(0[1-9]|1[0-2])$")
YEAR_PATTERN = re.compile(r"^\d{4}$")
DATE_PATTERN = re.compile(r"^\d{4}-\d{2}-\d{2}$")


class CatalogError(ValueError):
    pass


def resolve_reference_time(arg_time: str | None = None) -> datetime | None:
    if arg_time is None:
        return None
    try:
        dt = datetime.fromisoformat(arg_time.replace("Z", "+00:00"))
    except ValueError as error:
        raise CatalogError(f"invalid --now timestamp '{arg_time}': {error}") from error
    if dt.tzinfo is None or dt.utcoffset() is None:
        raise CatalogError(f"--now timestamp must include timezone offset: '{arg_time}'")
    return dt


def utc_now(reference_time: datetime | None = None) -> str:
    current = reference_time if reference_time is not None else datetime.now(timezone.utc)
    return current.astimezone(timezone.utc).isoformat(timespec="seconds").replace("+00:00", "Z")


def normalize_utc(value: str) -> str:
    try:
        dt = datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError as error:
        raise CatalogError(f"invalid timestamp '{value}': {error}") from error
    if dt.tzinfo is None or dt.utcoffset() is None:
        raise CatalogError(f"timestamp must include timezone offset: '{value}'")
    return dt.astimezone(timezone.utc).isoformat(timespec="seconds").replace("+00:00", "Z")


def optional_timestamp(mapping: dict[str, Any], key: str, source: Path) -> str | None:
    """Return a normalized UTC timestamp from an optional Task metadata field.

    An absent field means the record predates the field and simply cannot produce a precise event;
    callers skip it rather than substituting ``created_at``, ``updated_at``, Git time or file mtime.
    A field that is present but empty or unparseable is a corrupt canonical record and fails the
    build instead of silently disappearing from the timeline.
    """
    value = mapping.get(key)
    if value is None:
        return None
    if not isinstance(value, str) or not value.strip():
        raise CatalogError(f"{source}: '{key}' must be a non-empty timestamp")
    return normalize_utc(value.strip())


def project_relative(project_root: Path, path: Path) -> str:
    try:
        return path.resolve(strict=True).relative_to(project_root.resolve(strict=True)).as_posix()
    except (OSError, ValueError) as error:
        raise CatalogError(f"Activity source escapes the project: {path}") from error


def task_source_files(project_root: Path) -> list[Path]:
    root = project_root / TASKS_ROOT
    if root.exists() and not root.is_dir():
        raise CatalogError(f"Task root must be a directory: {root}")
    if not root.is_dir():
        return []

    archive_root = root / "archive"
    if archive_root.exists() and not archive_root.is_dir():
        raise CatalogError(f"Task archive root must be a directory: {archive_root}")

    candidates = [path for path in root.glob("*/task.yaml") if path.parent.name != "archive"]
    if archive_root.is_dir():
        candidates.extend(archive_root.glob("*/task.yaml"))

    result: list[Path] = []
    for path in sorted(candidates):
        if path.is_symlink():
            raise CatalogError(f"Task metadata must not be a symlink: {path}")
        project_relative(project_root, path)
        result.append(path)
    return result


def decision_record_source_files(
    project_root: Path, root: Path, *, root_label: str, record_label: str
) -> list[Path]:
    directory = project_root / root
    if directory.exists() and not directory.is_dir():
        raise CatalogError(f"{root_label} root must be a directory: {directory}")
    if not directory.is_dir():
        return []
    result: list[Path] = []
    for path in sorted(directory.glob(f"*{DECISION_SUFFIX}")):
        if path.is_symlink():
            raise CatalogError(f"{record_label} must not be a symlink: {path}")
        project_relative(project_root, path)
        result.append(path)
    return result


def decision_source_files(project_root: Path) -> list[Path]:
    return decision_record_source_files(
        project_root,
        DECISIONS_ROOT,
        root_label="Decision",
        record_label="Decision Record",
    )


def playbook_decision_source_files(project_root: Path) -> list[Path]:
    return decision_record_source_files(
        project_root,
        PLAYBOOK_DECISIONS_ROOT,
        root_label="Playbook decision",
        record_label="Playbook Decision Record",
    )


def worker_approval_source_files(project_root: Path) -> list[Path]:
    directory = project_root / WORKER_APPROVALS_ROOT
    if directory.exists() and not directory.is_dir():
        raise CatalogError(f"Worker approval root must be a directory: {directory}")
    if not directory.is_dir():
        return []
    result: list[Path] = []
    for path in sorted(directory.glob(f"*{WORKER_APPROVAL_SUFFIX}")):
        if path.is_symlink():
            raise CatalogError(f"Worker approval must not be a symlink: {path}")
        project_relative(project_root, path)
        result.append(path)
    return result


def temporary_metadata_files(project_root: Path) -> list[Path]:
    result: list[Path] = []
    for lifecycle in ("active", "archive"):
        root = project_root / TEMPORARY_ROOT / lifecycle
        if root.exists() and not root.is_dir():
            raise CatalogError(f"Temporary {lifecycle} root must be a directory: {root}")
        if not root.is_dir():
            continue
        for path in sorted(root.glob("*/meta.yaml")):
            if path.is_symlink():
                raise CatalogError(f"Temporary metadata must not be a symlink: {path}")
            project_relative(project_root, path)
            result.append(path)
    return result


def checkpoint_target_roots(project_root: Path) -> list[tuple[str, str, Path]]:
    result: list[tuple[str, str, Path]] = []
    seen: dict[tuple[str, str], Path] = {}

    def add(kind: str, target_id: str, root: Path) -> None:
        checkpoint_root = root / "references" / "checkpoints"
        if checkpoint_root.exists() and not checkpoint_root.is_dir():
            raise CatalogError(f"Checkpoint record root must be a directory: {checkpoint_root}")
        if not checkpoint_root.is_dir():
            return
        key = (kind, target_id)
        previous = seen.get(key)
        if previous is not None:
            raise CatalogError(
                f"duplicate checkpoint target '{kind}/{target_id}' in {previous} and {root}"
            )
        seen[key] = root
        result.append((kind, target_id, root))

    for metadata in task_source_files(project_root):
        task = parse_simple_yaml(metadata)
        add("task", require_text(task, "id", metadata), metadata.parent)
    for metadata in temporary_metadata_files(project_root):
        # Temporary metadata is not an Activity authority for checkpoint recovery. Its presence
        # establishes a canonical target directory; the stable target id is the directory name.
        add("temporary", metadata.parent.name, metadata.parent)
    return result


def checkpoint_source_files(project_root: Path) -> list[Path]:
    result: list[Path] = []
    for _kind, _target_id, root in checkpoint_target_roots(project_root):
        directory = root / "references" / "checkpoints"
        if directory.exists() and not directory.is_dir():
            raise CatalogError(f"Checkpoint record root must be a directory: {directory}")
        if not directory.is_dir():
            continue
        for path in sorted(directory.glob("*.json")):
            if ".failed-" in path.name:
                continue
            if path.is_symlink():
                raise CatalogError(f"Checkpoint record must not be a symlink: {path}")
            project_relative(project_root, path)
            result.append(path)
    return result


def external_source_files(project_root: Path) -> list[Path]:
    root = project_root / IMPORTS_ROOT
    if root.exists() and not root.is_dir():
        raise CatalogError(f"Import root must be a directory: {root}")
    files = sorted(root.glob("*/manifest.json"))
    for path in files:
        if path.is_symlink():
            raise CatalogError(f"Import manifest must not be a symlink: {path}")
        project_relative(project_root, path)
    return files


def derive_external_events(project_root: Path, sources: list[Path]) -> list[dict[str, Any]]:
    events = []
    ids = set()
    for path in sources:
        record, _ = read_json_object(path, "External work import")
        if record.get("import_id") != path.parent.name or record.get("status") not in {"pending", "skipped", "confirmed"}:
            raise CatalogError(f"invalid import manifest {path}")
        for source in record.get("sources", []):
            ref = source["ref"]
            expected = (IMPORTS_ROOT / path.parent.name / "sources").as_posix() + "/"
            if not isinstance(ref, str) or not ref.startswith(expected):
                raise CatalogError(f"invalid managed source: {ref}")
            file = project_root / ref
            if file.is_symlink() or not file.is_file() or project_relative(project_root, file) != ref:
                raise CatalogError(f"missing managed source: {ref}")
            if hashlib.sha256(file.read_bytes()).hexdigest() != source["sha256"]:
                raise CatalogError(f"managed source hash mismatch: {ref}")
        if record["status"] != "confirmed":
            continue
        allowed = {item["ref"] for item in record["sources"]}
        for item in record["events"]:
            refs = item["source_refs"]
            stamp = normalize_utc(item["occurred_at"])
            if not refs or len(refs) != len(set(refs)) or not set(refs) <= allowed:
                raise CatalogError(f"invalid event source refs: {path}")
            stable = "external-" + hashlib.sha256((stamp + "\n" + item["title"] + "\n" + "\n".join(sorted(refs))).encode()).hexdigest()[:24]
            if item["event_id"] != stable or stable in ids:
                raise CatalogError(f"duplicate or invalid external event ID: {path}")
            ids.add(stable)
            event = {"event_id": make_event_id("external_work", stable, stamp),
                     "occurred_at": stamp, "event_type": "external_work", "title": item["title"],
                     "summary": item["summary"], "source_refs": refs, "status": "completed"}
            validate_event(project_root, event)
            events.append(event)
    return events


def activity_source_files(project_root: Path) -> list[Path]:
    return (
        task_source_files(project_root)
        + decision_source_files(project_root)
        + playbook_decision_source_files(project_root)
        + checkpoint_source_files(project_root)
        + worker_approval_source_files(project_root)
        + external_source_files(project_root)
        + [project_root / item["ref"] for manifest in external_source_files(project_root)
           for item in read_json_object(manifest, "External work import")[0].get("sources", [])]
    )


def require_text(mapping: dict[str, Any], key: str, source: Path) -> str:
    value = mapping.get(key)
    if not isinstance(value, str) or not value.strip():
        raise CatalogError(f"{source}: '{key}' must be a non-empty string")
    return value.strip()


def make_event_id(event_type: str, source_id: str, occurred_at: str) -> str:
    seed = "\n".join((event_type, source_id, occurred_at))
    digest = hashlib.sha256(seed.encode("utf-8")).hexdigest()[:12]
    return f"activity-{occurred_at[:10].replace('-', '')}-{digest}"


def validate_event(project_root: Path, event: dict[str, Any]) -> None:
    errors: list[Diagnostic] = []
    validate_activity_event(event, "$", errors, FileReferenceValidator(project_root))
    if errors:
        messages = "; ".join(f"{error.path}: {error.message}" for error in errors)
        raise CatalogError(f"invalid derived Activity event: {messages}")


def derive_task_events(project_root: Path, sources: list[Path]) -> list[dict[str, Any]]:
    events: list[dict[str, Any]] = []
    seen_task_ids: dict[str, Path] = {}
    for path in sources:
        task = parse_simple_yaml(path)
        task_id = require_text(task, "id", path)
        previous = seen_task_ids.get(task_id)
        if previous is not None:
            raise CatalogError(f"duplicate Task id '{task_id}' in {previous} and {path}")
        seen_task_ids[task_id] = path

        status = require_text(task, "status", path)
        source_ref = project_relative(project_root, path)

        promoted_at = optional_timestamp(task, "promoted_at", path)
        source_temporary = task.get("source_temporary")
        # A promoted Task that predates ``promoted_at`` stays valid but cannot produce a promotion
        # event. ``promotion_transaction`` records when the transaction was opened, not when the
        # promotion became logically visible, so it is never used as a substitute.
        if promoted_at is not None and status in PROMOTION_EVENT_STATUSES:
            if not isinstance(source_temporary, str) or not source_temporary.strip():
                raise CatalogError(
                    f"{path}: 'promoted_at' requires a non-empty 'source_temporary'"
                )
            # The Task metadata is the authoritative record of the promotion: the promotion time
            # and the source Temporary id both live there, so the Temporary directory is not a
            # second source and may already be archived or trashed without affecting the timeline.
            objective = require_text(task, "objective", path)
            promoted_temporary = source_temporary.strip()
            event = {
                "event_id": make_event_id("temporary_promoted", task_id, promoted_at),
                "occurred_at": promoted_at,
                "event_type": "temporary_promoted",
                "title": objective,
                "summary": f"由 Temporary {promoted_temporary} 晋升为 Task：{objective}",
                "source_refs": [source_ref],
                "status": "completed",
            }
            validate_event(project_root, event)
            events.append(event)

        if status not in TASK_EVENT_STATUSES:
            continue
        completed_at = optional_timestamp(task, "completed_at", path)
        if completed_at is None:
            # Old Tasks without a trustworthy lifecycle time remain valid, but cannot produce a
            # precise timeline event. In particular, updated_at and mtime are not substitutes.
            continue
        objective = require_text(task, "objective", path)
        event = {
            "event_id": make_event_id("task_completed", task_id, completed_at),
            "occurred_at": completed_at,
            "event_type": "task_completed",
            "title": objective,
            "summary": f"完成 Task：{objective}",
            "source_refs": [source_ref],
            "status": "completed",
        }
        validate_event(project_root, event)
        events.append(event)

    events.sort(key=lambda item: (item["occurred_at"], item["event_id"]))
    return events


def derive_decision_record_events(
    project_root: Path,
    sources: list[Path],
    *,
    label: str,
    approved_summary_prefix: str,
    superseded_summary_prefix: str,
    approved_type: str,
    superseded_type: str,
) -> list[dict[str, Any]]:
    events: list[dict[str, Any]] = []
    seen_ids: dict[str, Path] = {}
    for path in sources:
        try:
            record = json.loads(path.read_text(encoding="utf-8"))
        except (OSError, UnicodeError, json.JSONDecodeError) as error:
            raise CatalogError(f"cannot parse {label} {path}: {error}") from error
        errors: list[Diagnostic] = []
        # Evidence refs were verified when the immutable record was published. Historical local
        # evidence may later move or be absent on another clone, so rebuild only rechecks that the
        # stored paths are safe project-relative references.
        validate_decision_record(
            record,
            errors,
            FileReferenceValidator(project_root, require_existing=False),
        )
        if errors:
            messages = "; ".join(f"{error.path}: {error.message}" for error in errors)
            raise CatalogError(f"invalid {label} {path}: {messages}")

        decision_id = record["decision_id"]
        expected_name = f"{decision_id}{DECISION_SUFFIX}"
        if path.name != expected_name:
            raise CatalogError(
                f"{path}: filename must match decision_id as '{expected_name}'"
            )
        previous = seen_ids.get(decision_id)
        if previous is not None:
            raise CatalogError(
                f"duplicate {label} id '{decision_id}' in {previous} and {path}"
            )
        seen_ids[decision_id] = path

        outcome = record["outcome"]
        if outcome == "rejected" or record["importance"] != "milestone":
            continue
        decided_at = normalize_utc(record["decided_at"])
        event_type = approved_type if outcome == "approved" else superseded_type
        title = record["title"].strip()
        reason = record["reason"].strip()
        event = {
            "event_id": make_event_id(event_type, decision_id, decided_at),
            "occurred_at": decided_at,
            "event_type": event_type,
            "title": title,
            "summary": (
                f"{approved_summary_prefix}：{reason}"
                if outcome == "approved"
                else f"{superseded_summary_prefix}：{reason}"
            ),
            "source_refs": [project_relative(project_root, path)],
            "status": "completed",
        }
        validate_event(project_root, event)
        events.append(event)
    return events


def derive_decision_events(project_root: Path, sources: list[Path]) -> list[dict[str, Any]]:
    return derive_decision_record_events(
        project_root,
        sources,
        label="Decision Record",
        approved_summary_prefix="批准决策",
        superseded_summary_prefix="取代决策",
        approved_type="decision_approved",
        superseded_type="decision_superseded",
    )


def derive_playbook_events(project_root: Path, sources: list[Path]) -> list[dict[str, Any]]:
    return derive_decision_record_events(
        project_root,
        sources,
        label="Playbook Decision Record",
        approved_summary_prefix="批准 Playbook",
        superseded_summary_prefix="取代 Playbook",
        approved_type="playbook_approved",
        superseded_type="playbook_superseded",
    )


def derive_worker_approval_events(
    project_root: Path, sources: list[Path]
) -> list[dict[str, Any]]:
    events: list[dict[str, Any]] = []
    for path in sources:
        record, _ = read_json_object(path, "Worker approval")
        errors: list[Diagnostic] = []
        validate_worker_approval(record, errors)
        if errors:
            messages = "; ".join(f"{error.path}: {error.message}" for error in errors)
            raise CatalogError(f"invalid Worker approval {path}: {messages}")
        approval_id = record["approval_id"]
        expected_name = f"{approval_id}{WORKER_APPROVAL_SUFFIX}"
        if path.name != expected_name:
            raise CatalogError(f"{path}: filename must match approval_id as '{expected_name}'")
        approved_at = normalize_utc(record["approved_at"])
        worker_id = record["worker_id"]
        registry_id = record["registry_id"]
        revision = record["registry_revision"]
        event = {
            "event_id": make_event_id("worker_approved", approval_id, approved_at),
            "occurred_at": approved_at,
            "event_type": "worker_approved",
            "title": record["worker_name"].strip(),
            "summary": f"批准 Worker {worker_id} 进入 registry {registry_id} revision {revision}",
            "source_refs": [project_relative(project_root, path)],
            "status": "completed",
        }
        validate_event(project_root, event)
        events.append(event)
    return events


def read_json_object(path: Path, label: str) -> tuple[dict[str, Any], str]:
    try:
        content = path.read_text(encoding="utf-8")
        value = json.loads(content)
    except (OSError, UnicodeError, json.JSONDecodeError) as error:
        raise CatalogError(f"cannot parse {label} {path}: {error}") from error
    if not isinstance(value, dict):
        raise CatalogError(f"invalid {label} {path}: must be an object")
    return value, content


def require_checkpoint_hash(value: Any, key: str, source: Path) -> str:
    if not isinstance(value, str) or not re.fullmatch(r"[0-9a-f]{64}", value):
        raise CatalogError(f"{source}: '{key}' must be a lowercase SHA-256 hash")
    return value


def derive_checkpoint_events(project_root: Path) -> list[dict[str, Any]]:
    events: list[dict[str, Any]] = []
    legacy_keys = {"request_id", "record_hash", "proposal_hash", "revision"}
    versioned_keys = legacy_keys | {"completion", "committed_at"}
    for kind, target_id, root in checkpoint_target_roots(project_root):
        directory = root / "references" / "checkpoints"
        if not directory.is_dir():
            continue
        for observation_path in sorted(directory.glob("*.committed.json")):
            if observation_path.is_symlink():
                raise CatalogError(f"Checkpoint observation must not be a symlink: {observation_path}")
            observation, _ = read_json_object(observation_path, "Checkpoint observation")
            observation_errors: list[Diagnostic] = []
            validate_checkpoint_observation(observation, observation_errors)
            if observation_errors:
                messages = "; ".join(
                    f"{error.path}: {error.message}" for error in observation_errors
                )
                raise CatalogError(
                    f"invalid Checkpoint observation {observation_path}: {messages}"
                )
            keys = set(observation)
            if keys == legacy_keys:
                continue
            if keys != versioned_keys:
                raise CatalogError(f"invalid Checkpoint observation {observation_path}: invalid fields")
            completion = observation.get("completion")
            if completion not in {"save", "recovery"}:
                raise CatalogError(f"{observation_path}: 'completion' must be save or recovery")
            committed_at = observation.get("committed_at")
            if not isinstance(committed_at, str) or not committed_at.strip():
                raise CatalogError(f"{observation_path}: 'committed_at' must be a timestamp")
            normalized_time = normalize_utc(committed_at)
            if completion == "save":
                continue

            request_id = require_text(observation, "request_id", observation_path)
            expected_name = f"{request_id}.committed.json"
            if observation_path.name != expected_name:
                raise CatalogError(f"{observation_path}: filename must match request_id as '{expected_name}'")
            request_path = directory / f"{request_id}.json"
            if not request_path.is_file() or request_path.is_symlink():
                raise CatalogError(f"{observation_path}: recovery request record is missing or unsafe")
            request, request_content = read_json_object(request_path, "Checkpoint request")
            if request.get("request_id") != request_id or request.get("kind") != kind \
                    or request.get("target_id") != target_id:
                raise CatalogError(f"{request_path}: checkpoint request binding mismatch")
            base_revision = request.get("base_revision")
            revision = observation.get("revision")
            if not isinstance(base_revision, int) or isinstance(base_revision, bool) \
                    or not isinstance(revision, int) or isinstance(revision, bool) \
                    or revision != base_revision + 1:
                raise CatalogError(f"{observation_path}: checkpoint revision mismatch")
            record_hash = require_checkpoint_hash(observation.get("record_hash"), "record_hash", observation_path)
            proposal_hash = require_checkpoint_hash(observation.get("proposal_hash"), "proposal_hash", observation_path)
            if record_hash != hashlib.sha256(request_content.encode("utf-8")).hexdigest() \
                    or proposal_hash != request.get("proposal_hash"):
                raise CatalogError(f"{observation_path}: checkpoint observation hash mismatch")
            snapshot = request.get("snapshot")
            if not isinstance(snapshot, dict):
                raise CatalogError(f"{request_path}: 'snapshot' must be an object")
            objective = require_text(snapshot, "objective", request_path)
            source_ref = project_relative(project_root, observation_path)
            source_id = f"{kind}\n{target_id}\n{request_id}"
            event = {
                "event_id": make_event_id("checkpoint_recovered", source_id, normalized_time),
                "occurred_at": normalized_time,
                "event_type": "checkpoint_recovered",
                "title": f"恢复 checkpoint：{objective}",
                "summary": f"恢复 {kind.title()} {target_id} 到 revision {revision}",
                "source_refs": [source_ref],
                "status": "completed",
            }
            validate_event(project_root, event)
            events.append(event)
    return events


def source_digest(project_root: Path, source_files: Iterable[Path] | None = None) -> str:
    files = list(source_files) if source_files is not None else activity_source_files(project_root)
    digest = hashlib.sha256()
    for path in sorted(files, key=lambda item: project_relative(project_root, item)):
        relative = project_relative(project_root, path)
        digest.update(relative.encode("utf-8"))
        digest.update(b"\0")
        try:
            digest.update(path.read_bytes())
        except OSError as error:
            raise CatalogError(f"cannot hash Activity source {path}: {error}") from error
        digest.update(b"\0")
    return digest.hexdigest()


def derive_index(project_root: Path, *, now: datetime | None = None) -> dict[str, Any]:
    task_sources = task_source_files(project_root)
    decision_sources = decision_source_files(project_root)
    playbook_sources = playbook_decision_source_files(project_root)
    checkpoint_sources = checkpoint_source_files(project_root)
    worker_approval_sources = worker_approval_source_files(project_root)
    external_sources = external_source_files(project_root)
    sources = activity_source_files(project_root)
    digest_before = source_digest(project_root, sources)
    events = derive_task_events(project_root, task_sources)
    events.extend(derive_decision_events(project_root, decision_sources))
    events.extend(derive_playbook_events(project_root, playbook_sources))
    events.extend(derive_checkpoint_events(project_root))
    events.extend(derive_worker_approval_events(project_root, worker_approval_sources))
    events.extend(derive_external_events(project_root, external_sources))
    events.sort(key=lambda item: (item["occurred_at"], item["event_id"]))
    sources_after = activity_source_files(project_root)
    digest_after = source_digest(project_root, sources_after)
    if digest_before != digest_after:
        raise CatalogError("Activity sources changed while the index was being built; retry")
    index = {
        "schema_version": 1,
        "generated_at": utc_now(now),
        "source_digest": digest_after,
        "events": events,
    }
    errors: list[Diagnostic] = []
    validate_activity_index(index, errors, FileReferenceValidator(project_root))
    if errors:
        messages = "; ".join(f"{error.path}: {error.message}" for error in errors)
        raise CatalogError(f"generated Activity Index is invalid: {messages}")
    return index


def atomic_write(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary_name = tempfile.mkstemp(prefix=f".{path.name}.", dir=path.parent)
    temporary_path = Path(temporary_name)
    try:
        with os.fdopen(descriptor, "w", encoding="utf-8", newline="\n") as stream:
            stream.write(content)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary_path, path)
    finally:
        temporary_path.unlink(missing_ok=True)


def persist_index(project_root: Path, index: dict[str, Any]) -> None:
    atomic_write(
        project_root / INDEX_PATH,
        json.dumps(index, ensure_ascii=False, indent=2, sort_keys=True) + "\n",
    )


def load_index(project_root: Path) -> dict[str, Any] | None:
    path = project_root / INDEX_PATH
    if not path.is_file():
        return None
    try:
        value = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, UnicodeError, json.JSONDecodeError):
        return None
    if not isinstance(value, dict):
        return None
    errors: list[Diagnostic] = []
    validate_activity_index(value, errors, FileReferenceValidator(project_root))
    return None if errors else value


def index_is_current(project_root: Path, index: dict[str, Any]) -> bool:
    return index.get("source_digest") == source_digest(project_root)


def ensure_current_index(
    project_root: Path, *, refresh: bool, now: datetime | None = None
) -> tuple[dict[str, Any], bool]:
    index = load_index(project_root)
    if index is not None and index_is_current(project_root, index):
        return index, False
    if not refresh:
        raise CatalogError("Activity index is missing or stale; rerun without --no-refresh")
    index = derive_index(project_root, now=now)
    persist_index(project_root, index)
    return index, True


def range_boundary(value: str, *, end: bool) -> str:
    if DATE_PATTERN.fullmatch(value):
        return value + ("T23:59:59Z" if end else "T00:00:00Z")
    return normalize_utc(value)


def filter_events(
    events: list[dict[str, Any]],
    *,
    month: str | None,
    year: str | None,
    from_: str | None,
    to: str | None,
    event_type: str | None,
    limit: int,
) -> tuple[list[dict[str, Any]], int]:
    if month and not MONTH_PATTERN.fullmatch(month):
        raise CatalogError("--month must use YYYY-MM")
    if year and not YEAR_PATTERN.fullmatch(year):
        raise CatalogError("--year must use YYYY")
    start = range_boundary(from_, end=False) if from_ else None
    end = range_boundary(to, end=True) if to else None
    if start and end and start > end:
        raise CatalogError("--from must not be later than --to")

    result: list[dict[str, Any]] = []
    for event in events:
        occurred = event["occurred_at"]
        if month and not occurred.startswith(month + "-"):
            continue
        if year and not occurred.startswith(year + "-"):
            continue
        if start and occurred < start:
            continue
        if end and occurred > end:
            continue
        if event_type and event["event_type"] != event_type:
            continue
        result.append(event)
    return result[-limit:], len(result)


def parse_args(argv: list[str]) -> argparse.Namespace:
    common = argparse.ArgumentParser(add_help=False)
    common.add_argument("--project-root", type=Path, default=argparse.SUPPRESS)
    common.add_argument("--now", default=argparse.SUPPRESS)
    parser = argparse.ArgumentParser(
        description="Maintain XiaoTao's derived Activity Timeline.", parents=[common]
    )
    subparsers = parser.add_subparsers(dest="command", required=True)
    subparsers.add_parser("build", parents=[common])
    subparsers.add_parser("check", parents=[common])
    search = subparsers.add_parser("search", parents=[common])
    search.add_argument("--month")
    search.add_argument("--year")
    search.add_argument("--from", dest="from_")
    search.add_argument("--to")
    search.add_argument("--event-type", choices=sorted(EVENT_TYPES))
    search.add_argument("--limit", type=int, default=50)
    search.add_argument("--no-refresh", action="store_true")
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
            raise CatalogError(f"project root is not a directory: {project_root}")
        reference_time = resolve_reference_time(args.now)

        if args.command == "build":
            index = derive_index(project_root, now=reference_time)
            persist_index(project_root, index)
            print(json.dumps({"status": "built", "events": len(index["events"])}, sort_keys=True))
            return 0
        if args.command == "check":
            current = load_index(project_root)
            if current is None or not index_is_current(project_root, current):
                print("Activity index is missing or stale", file=sys.stderr)
                return 1
            print(json.dumps({"status": "current", "events": len(current["events"])}, sort_keys=True))
            return 0

        if args.limit < 1 or args.limit > 200:
            raise CatalogError("--limit must be between 1 and 200")
        index, refreshed = ensure_current_index(
            project_root, refresh=not args.no_refresh, now=reference_time
        )
        events, total = filter_events(
            index["events"],
            month=args.month,
            year=args.year,
            from_=args.from_,
            to=args.to,
            event_type=args.event_type,
            limit=args.limit,
        )
        print(
            json.dumps(
                {
                    "month": args.month,
                    "year": args.year,
                    "from": args.from_,
                    "to": args.to,
                    "event_type": args.event_type,
                    "catalog_refreshed": refreshed,
                    "total": total,
                    "events": events,
                },
                ensure_ascii=False,
                indent=2,
                sort_keys=True,
            )
        )
        return 0
    except (CatalogError, OSError, RuntimeError, ValueError) as error:
        print(f"activity catalog error: {error}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
