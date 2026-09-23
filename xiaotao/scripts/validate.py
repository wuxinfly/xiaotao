#!/usr/bin/env python3
"""Validate persisted XiaoTao protocol artifacts without changing workflow state."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import sys
from dataclasses import asdict, dataclass
from datetime import datetime
from pathlib import Path, PurePosixPath
from typing import Any, Callable


Validator = Callable[[Any, str, list["Diagnostic"]], None]
CAPABILITY_PATTERN = re.compile(r"^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$")
WINDOWS_DRIVE_PATTERN = re.compile(r"^[A-Za-z]:")
STABLE_ID_PATTERN = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]*$")
# Storage IDs (Temporary/Task directory names) are validated with a Unicode allow-list rather than
# a regex because Python's stdlib `re` has no \p{...} property escapes. The evaluator mirrors the
# schema pattern ^(?!\.)(?!.*[.]$)[\p{L}\p{N}._-]+$ : letters and numbers (including CJK), plus
# "_", ".", "-", with a leading or trailing dot rejected so "." and ".." are unusable and the
# directory name can never normalize away to something that differs from meta.yaml.id.
RFC3339_DATE_TIME_PATTERN = re.compile(
    r"^\d{4}-\d{2}-\d{2}[Tt]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:[Zz]|[+-]\d{2}:\d{2})$"
)
PLAYBOOK_METADATA_PATTERN = re.compile(
    r"^([A-Za-z_][A-Za-z0-9_-]*):[ \t]*(.*?)[ \t]*$"
)
PLAYBOOK_METADATA_FIELDS = {"playbook_id", "file_path", "revision", "status"}
PLAYBOOK_EXTENSIONS = {".md", ".markdown", ".yaml", ".yml"}
PLAYBOOK_RESERVED_DIRECTORIES = {"candidates", "decisions"}
MEMORY_LAYERS = {"temporary", "task", "long-term"}
MEMORY_RECORD_TYPES = {
    "long-term-entry",
    "temporary",
    "task",
    "worker-state",
}
MEMORY_STATUSES = {"active", "disputed", "superseded", "rejected", "archived"}
MEMORY_KINDS = {"fact", "experience", "principle", "decision", "constraint", "other"}
SHA256_PATTERN = re.compile(r"^[0-9a-f]{64}$")
ACTIVITY_EVENT_TYPES = {
    "task_completed",
    "temporary_promoted",
    "decision_approved",
    "decision_superseded",
    "playbook_approved",
    "playbook_superseded",
    "checkpoint_recovered",
    "worker_approved",
}
ACTIVITY_EVENT_ID_PATTERN = re.compile(r"^activity-[A-Za-z0-9][A-Za-z0-9._-]*$")


@dataclass(frozen=True)
class Diagnostic:
    path: str
    message: str


class NonStandardJsonConstant(ValueError):
    def __init__(self, constant: str):
        super().__init__(constant)
        self.constant = constant


def reject_non_standard_json_constant(constant: str) -> None:
    raise NonStandardJsonConstant(constant)


def add_error(errors: list[Diagnostic], path: str, message: str) -> None:
    errors.append(Diagnostic(path, message))


def is_object(value: Any) -> bool:
    return isinstance(value, dict)


def is_array(value: Any) -> bool:
    return isinstance(value, list)


def is_boolean(value: Any) -> bool:
    return isinstance(value, bool)


def require_object(value: Any, path: str, errors: list[Diagnostic]) -> bool:
    if not is_object(value):
        add_error(errors, path, "must be an object")
        return False
    return True


def check_object_shape(
    value: dict[str, Any],
    path: str,
    errors: list[Diagnostic],
    *,
    required: set[str],
    allowed: set[str],
) -> None:
    for key in sorted(required - value.keys()):
        add_error(errors, f"{path}.{key}", "is required")
    for key in sorted(value.keys() - allowed):
        add_error(errors, f"{path}.{key}", "is not allowed")


def check_string(
    value: Any,
    path: str,
    errors: list[Diagnostic],
    *,
    min_length: int = 0,
) -> bool:
    if not isinstance(value, str):
        add_error(errors, path, "must be a string")
        return False
    if len(value) < min_length:
        add_error(errors, path, f"must contain at least {min_length} character(s)")
        return False
    return True


def check_boolean(value: Any, path: str, errors: list[Diagnostic]) -> bool:
    if not is_boolean(value):
        add_error(errors, path, "must be a boolean")
        return False
    return True


def check_enum(
    value: Any, path: str, errors: list[Diagnostic], choices: set[str]
) -> bool:
    if not isinstance(value, str) or value not in choices:
        add_error(errors, path, f"must be one of: {', '.join(sorted(choices))}")
        return False
    return True


def check_array(
    value: Any,
    path: str,
    errors: list[Diagnostic],
    item_validator: Validator,
    *,
    min_items: int = 0,
) -> bool:
    if not is_array(value):
        add_error(errors, path, "must be an array")
        return False
    if len(value) < min_items:
        add_error(errors, path, f"must contain at least {min_items} item(s)")
    for index, item in enumerate(value):
        item_validator(item, f"{path}[{index}]", errors)
    return True


def check_plain_object(value: Any, path: str, errors: list[Diagnostic]) -> None:
    require_object(value, path, errors)


def check_storage_id(value: Any, path: str, errors: list[Diagnostic]) -> bool:
    if not check_string(value, path, errors, min_length=1):
        return False
    assert isinstance(value, str)
    if (
        value[0] == "."
        or value[-1] == "."
        or not all(character.isalnum() or character in "._-" for character in value)
    ):
        add_error(
            errors,
            path,
            "must be a filesystem-safe ID matching the directory name: letters and "
            "numbers (CJK ok) plus '_', '.', '-', with no leading or trailing dot",
        )
        return False
    return True


def check_stable_id(value: Any, path: str, errors: list[Diagnostic]) -> bool:
    if not check_string(value, path, errors, min_length=1):
        return False
    assert isinstance(value, str)
    if not STABLE_ID_PATTERN.fullmatch(value):
        add_error(errors, path, "must be a stable ID using letters, digits, '.', '_' or '-'")
        return False
    return True


def check_date_time(value: Any, path: str, errors: list[Diagnostic]) -> bool:
    if not check_string(value, path, errors, min_length=1):
        return False
    assert isinstance(value, str)
    if not RFC3339_DATE_TIME_PATTERN.fullmatch(value):
        add_error(errors, path, "must be an RFC 3339 date-time")
        return False
    try:
        normalized = value[:-1] + "+00:00" if value[-1] in "Zz" else value
        parsed = datetime.fromisoformat(normalized)
    except ValueError:
        add_error(errors, path, "must be an RFC 3339 date-time")
        return False
    if parsed.tzinfo is None:
        add_error(errors, path, "must include a timezone offset")
        return False
    return True


def check_unique_strings(value: Any, path: str, errors: list[Diagnostic]) -> None:
    if not is_array(value):
        return
    seen: set[str] = set()
    for index, item in enumerate(value):
        if not isinstance(item, str):
            continue
        if item in seen:
            add_error(errors, f"{path}[{index}]", "must be unique")
        seen.add(item)


def make_string_validator(*, min_length: int = 0) -> Validator:
    def validate(value: Any, path: str, errors: list[Diagnostic]) -> None:
        check_string(value, path, errors, min_length=min_length)

    return validate


def check_canonical_path(value: Any, path: str, errors: list[Diagnostic]) -> bool:
    if not check_string(value, path, errors, min_length=1):
        return False
    if "\\" in value:
        add_error(errors, path, "must use '/' separators")
        return False
    if value.startswith("/") or WINDOWS_DRIVE_PATTERN.match(value):
        add_error(errors, path, "must be project-relative")
        return False
    if any(ord(character) < 32 for character in value):
        add_error(errors, path, "must not contain control characters")
        return False
    if any(part == ".." for part in value.split("/")):
        add_error(errors, path, "must not contain '..' segments")
        return False
    return True


class FileReferenceValidator:
    def __init__(self, project_root: Path, *, require_existing: bool = True):
        self.project_root = project_root.resolve(strict=True)
        self.require_existing = require_existing

    def __call__(self, value: Any, path: str, errors: list[Diagnostic]) -> None:
        if not check_string(value, path, errors, min_length=1):
            return

        assert isinstance(value, str)
        if "\\" in value:
            add_error(errors, path, "must use '/' separators")
            return
        if value.startswith("/") or WINDOWS_DRIVE_PATTERN.match(value):
            add_error(errors, path, "must be project-relative")
            return
        if any(ord(character) < 32 for character in value):
            add_error(errors, path, "must not contain control characters")
            return

        pure_path = PurePosixPath(value)
        if any(part in {"", ".", ".."} for part in value.split("/")):
            add_error(errors, path, "must not contain empty, '.' or '..' segments")
            return
        if pure_path.is_absolute():
            add_error(errors, path, "must be project-relative")
            return

        try:
            resolved = (self.project_root / Path(*pure_path.parts)).resolve(strict=False)
        except (OSError, RuntimeError, ValueError) as error:
            add_error(errors, path, f"cannot resolve file reference: {error}")
            return
        try:
            resolved.relative_to(self.project_root)
        except ValueError:
            add_error(errors, path, "resolves outside the project root")
            return
        if not self.require_existing:
            return
        try:
            is_file = resolved.is_file()
        except (OSError, ValueError) as error:
            add_error(errors, path, f"cannot inspect file reference: {error}")
            return
        if not is_file:
            add_error(errors, path, "must reference an existing file")


def check_canonical_playbook_path(
    value: Any, path: str, errors: list[Diagnostic]
) -> PurePosixPath | None:
    if not check_canonical_path(value, path, errors):
        return None
    assert isinstance(value, str)
    pure_path = PurePosixPath(value)
    parts = pure_path.parts
    if len(parts) < 3 or parts[:2] != (".xiaotao", "playbooks"):
        add_error(errors, path, "must be under '.xiaotao/playbooks/'")
        return None
    if parts[2] in PLAYBOOK_RESERVED_DIRECTORIES:
        add_error(
            errors,
            path,
            "must not be under reserved 'candidates/' or 'decisions/' directories",
        )
        return None
    if pure_path.suffix not in PLAYBOOK_EXTENSIONS:
        add_error(errors, path, "must reference a Markdown or YAML Playbook file")
        return None
    return pure_path


def parse_playbook_metadata_scalar(value: str) -> str:
    value = re.split(r"\s+#", value, maxsplit=1)[0].strip()
    if len(value) >= 2 and value[0] == value[-1] == '"':
        try:
            decoded = json.loads(value)
        except json.JSONDecodeError:
            return value
        return decoded if isinstance(decoded, str) else value
    if len(value) >= 2 and value[0] == value[-1] == "'":
        return value[1:-1].replace("''", "'")
    return value


def load_canonical_playbook_metadata(
    playbook_path: Path, diagnostic_path: str, errors: list[Diagnostic]
) -> dict[str, Any] | None:
    try:
        lines = playbook_path.read_text(encoding="utf-8").splitlines()
    except (OSError, RuntimeError, UnicodeError, ValueError) as error:
        add_error(errors, diagnostic_path, f"cannot read canonical Playbook: {error}")
        return None

    if playbook_path.suffix.lower() in {".md", ".markdown"}:
        if not lines or lines[0].strip() != "---":
            add_error(errors, diagnostic_path, "Markdown Playbook must contain YAML front matter")
            return None
        closing_index = next(
            (
                index
                for index, line in enumerate(lines[1:], start=1)
                if line.strip() in {"---", "..."}
            ),
            None,
        )
        if closing_index is None:
            add_error(errors, diagnostic_path, "Markdown Playbook front matter is not closed")
            return None
        metadata_lines = lines[1:closing_index]
    else:
        metadata_lines = lines

    metadata: dict[str, Any] = {}
    for line in metadata_lines:
        if not line or line[0].isspace() or line.lstrip().startswith("#"):
            continue
        match = PLAYBOOK_METADATA_PATTERN.fullmatch(line)
        if match is None or match.group(1) not in PLAYBOOK_METADATA_FIELDS:
            continue
        key, raw_value = match.groups()
        if key in metadata:
            add_error(errors, diagnostic_path, f"canonical Playbook metadata repeats '{key}'")
            return None
        scalar = parse_playbook_metadata_scalar(raw_value)
        if key == "revision" and re.fullmatch(r"-?\d+", scalar):
            metadata[key] = int(scalar)
        else:
            metadata[key] = scalar

    missing = PLAYBOOK_METADATA_FIELDS - metadata.keys()
    if missing:
        add_error(
            errors,
            diagnostic_path,
            f"canonical Playbook metadata is missing: {', '.join(sorted(missing))}",
        )
        return None
    return metadata


def validate_question(value: Any, path: str, errors: list[Diagnostic]) -> None:
    if not require_object(value, path, errors):
        return
    check_object_shape(
        value,
        path,
        errors,
        required={"question", "reason"},
        allowed={"question", "reason"},
    )
    for key in ("question", "reason"):
        if key in value:
            check_string(value[key], f"{path}.{key}", errors, min_length=1)


def validate_recommended_next(value: Any, path: str, errors: list[Diagnostic]) -> None:
    if not require_object(value, path, errors):
        return
    required = {"capabilities", "reason"}
    check_object_shape(value, path, errors, required=required, allowed=required)
    if "role" in value:
        add_error(
            errors,
            f"{path}.role",
            "legacy role field is no longer supported; use capabilities",
        )

    if "reason" in value:
        check_string(value["reason"], f"{path}.reason", errors, min_length=1)
    if "capabilities" in value:
        capabilities = value["capabilities"]
        if check_array(
            capabilities,
            f"{path}.capabilities",
            errors,
            make_string_validator(min_length=1),
            min_items=1,
        ):
            seen: set[str] = set()
            for index, capability in enumerate(capabilities):
                if not isinstance(capability, str):
                    continue
                if capability in seen:
                    add_error(
                        errors,
                        f"{path}.capabilities[{index}]",
                        "must be unique",
                    )
                seen.add(capability)
                if not CAPABILITY_PATTERN.fullmatch(capability):
                    add_error(
                        errors,
                        f"{path}.capabilities[{index}]",
                        "must use canonical kebab-case",
                    )


def validate_handoff(
    value: Any, errors: list[Diagnostic], file_reference: FileReferenceValidator
) -> None:
    path = "$"
    if not require_object(value, path, errors):
        return
    required = {
        "status",
        "summary",
        "result_path",
        "worker_state_path",
        "needs_user_input",
        "recommended_next",
    }
    allowed = required | {"questions"}
    check_object_shape(value, path, errors, required=required, allowed=allowed)
    if "role_state_path" in value:
        add_error(
            errors,
            "$.role_state_path",
            "legacy role_state_path field is no longer supported; use worker_state_path",
        )

    if "status" in value:
        check_enum(
            value["status"],
            "$.status",
            errors,
            {"completed", "blocked", "failed", "cancelled"},
        )
    if "summary" in value:
        check_string(value["summary"], "$.summary", errors, min_length=1)
    for key in ("result_path", "worker_state_path"):
        if key in value:
            file_reference(value[key], f"$.{key}", errors)
    if "needs_user_input" in value:
        check_boolean(value["needs_user_input"], "$.needs_user_input", errors)
    if "questions" in value:
        check_array(value["questions"], "$.questions", errors, validate_question)
    if "recommended_next" in value:
        check_array(
            value["recommended_next"],
            "$.recommended_next",
            errors,
            validate_recommended_next,
        )

    if value.get("needs_user_input") is True:
        if value.get("status") != "blocked":
            add_error(errors, "$.status", "must be 'blocked' when user input is needed")
        questions = value.get("questions")
        if not is_array(questions) or len(questions) == 0:
            add_error(errors, "$.questions", "must contain at least one question")
    elif is_array(value.get("questions")) and len(value["questions"]) > 0:
        add_error(errors, "$.questions", "must be empty when user input is not needed")


def validate_memory_request(
    value: Any, errors: list[Diagnostic], file_reference: FileReferenceValidator
) -> None:
    path = "$"
    if not require_object(value, path, errors):
        return
    required = {
        "operation",
        "source_files",
        "current_memory",
        "current_playbooks",
        "memory_hints",
    }
    allowed = required | {"task_context", "source_content"}
    check_object_shape(value, path, errors, required=required, allowed=allowed)

    if "operation" in value:
        check_enum(
            value["operation"],
            "$.operation",
            errors,
            {"worker-compress", "session-handoff", "task-bootstrap", "task-complete", "explicit-create"},
        )
    if "source_files" in value:
        if check_array(
            value["source_files"],
            "$.source_files",
            errors,
            file_reference,
            min_items=1,
        ):
            check_unique_strings(value["source_files"], "$.source_files", errors)
    if "current_memory" in value:
        current_memory = value["current_memory"]
        if require_object(current_memory, "$.current_memory", errors):
            scope = current_memory.get("scope", "full")
            if "scope" in current_memory:
                check_enum(scope, "$.current_memory.scope", errors, {"full", "bounded"})
            if scope == "bounded":
                if "query" not in current_memory:
                    add_error(errors, "$.current_memory.query", "is required for bounded input")
                else:
                    check_string(current_memory["query"], "$.current_memory.query", errors, min_length=1)
                if "limit" not in current_memory:
                    add_error(errors, "$.current_memory.limit", "is required for bounded input")
                elif (
                    not isinstance(current_memory["limit"], int)
                    or isinstance(current_memory["limit"], bool)
                    or current_memory["limit"] < 1
                    or current_memory["limit"] > 3
                ):
                    add_error(errors, "$.current_memory.limit", "must be an integer between 1 and 3")
            else:
                for key in ("query", "limit"):
                    if key in current_memory:
                        add_error(errors, f"$.current_memory.{key}", "is allowed only for bounded input")
            if "long_term_entries" not in current_memory:
                add_error(
                    errors,
                    "$.current_memory.long_term_entries",
                    "is required",
                )
            else:
                entries = current_memory["long_term_entries"]
                if check_array(
                    entries,
                    "$.current_memory.long_term_entries",
                    errors,
                    lambda item, item_path, item_errors: validate_long_term_entry(
                        item, item_path, item_errors, file_reference
                    ),
                ):
                    if scope == "bounded" and len(entries) > 3:
                        add_error(
                            errors,
                            "$.current_memory.long_term_entries",
                            "must contain at most 3 entries for bounded input",
                        )
                    if (
                        scope == "bounded"
                        and isinstance(current_memory.get("limit"), int)
                        and not isinstance(current_memory.get("limit"), bool)
                        and len(entries) > current_memory["limit"]
                    ):
                        add_error(
                            errors,
                            "$.current_memory.long_term_entries",
                            "must not contain more entries than limit",
                        )
                    seen_entry_ids: set[str] = set()
                    for index, entry in enumerate(entries):
                        if not is_object(entry):
                            continue
                        entry_id = entry.get("entry_id")
                        if not isinstance(entry_id, str):
                            continue
                        if entry_id in seen_entry_ids:
                            add_error(
                                errors,
                                f"$.current_memory.long_term_entries[{index}].entry_id",
                                "must be unique within current memory",
                            )
                        seen_entry_ids.add(entry_id)
    operation = value.get("operation")
    current_memory = value.get("current_memory")
    if is_object(current_memory):
        scope = current_memory.get("scope", "full")
        if operation == "explicit-create" and scope != "bounded":
            add_error(
                errors,
                "$.current_memory.scope",
                "must be 'bounded' for explicit-create",
            )
        elif operation != "explicit-create" and scope == "bounded":
            add_error(
                errors,
                "$.current_memory.scope",
                "bounded input is allowed only for explicit-create",
            )
    if "current_playbooks" in value:
        playbooks = value["current_playbooks"]
        if check_array(
            playbooks,
            "$.current_playbooks",
            errors,
            lambda item, item_path, item_errors: validate_playbook(
                item, item_path, item_errors, file_reference
            ),
        ):
            seen_playbook_ids: set[str] = set()
            for index, playbook in enumerate(playbooks):
                if not is_object(playbook):
                    continue
                playbook_id = playbook.get("playbook_id")
                if not isinstance(playbook_id, str):
                    continue
                if playbook_id in seen_playbook_ids:
                    add_error(
                        errors,
                        f"$.current_playbooks[{index}].playbook_id",
                        "must be unique within current playbooks",
                    )
                seen_playbook_ids.add(playbook_id)
    for key in ("memory_hints", "task_context", "source_content"):
        if key in value:
            check_plain_object(value[key], f"$.{key}", errors)


def validate_memory_source(
    value: Any,
    errors: list[Diagnostic],
    output_file: Path,
    project_root: Path,
) -> None:
    if not require_object(value, "$", errors):
        return
    required = {
        "schema_version", "source_id", "source_kind", "recorded_at",
        "recorded_by", "content", "content_sha256",
    }
    check_object_shape(value, "$", errors, required=required, allowed=required)
    if "schema_version" in value and value["schema_version"] != 1:
        add_error(errors, "$.schema_version", "must be 1")
    if "source_kind" in value and value["source_kind"] != "user-message":
        add_error(errors, "$.source_kind", "must be 'user-message'")
    if "recorded_at" in value:
        check_date_time(value["recorded_at"], "$.recorded_at", errors)
    if "recorded_by" in value:
        check_string(value["recorded_by"], "$.recorded_by", errors, min_length=1)
    content_valid = "content" in value and check_string(
        value["content"], "$.content", errors, min_length=1
    )
    digest = value.get("content_sha256")
    if not isinstance(digest, str) or not re.fullmatch(r"[0-9a-f]{64}", digest):
        add_error(errors, "$.content_sha256", "must be a lowercase SHA-256 digest")
        digest = None
    if content_valid and digest is not None:
        expected = hashlib.sha256(value["content"].encode("utf-8")).hexdigest()
        if digest != expected:
            add_error(errors, "$.content_sha256", "must match the UTF-8 content digest")
    source_id = value.get("source_id")
    if not isinstance(source_id, str) or not re.fullmatch(r"src-[0-9a-f]{16}", source_id):
        add_error(errors, "$.source_id", "must use 'src-' plus 16 lowercase hex characters")
    elif digest is not None and source_id != f"src-{digest[:16]}":
        add_error(errors, "$.source_id", "must be derived from content_sha256")
    if isinstance(source_id, str):
        expected_path = (
            project_root / ".xiaotao" / "memory" / "sources" / f"{source_id}.json"
        ).resolve()
        if output_file != expected_path:
            add_error(
                errors,
                "$",
                "must be stored at .xiaotao/memory/sources/<source_id>.json",
            )


def load_memory_request(
    request_path: Path,
    errors: list[Diagnostic],
    file_reference: FileReferenceValidator,
) -> tuple[set[str], set[str], str] | None:
    reference_path = "--request"
    try:
        request_value = json.loads(
            request_path.read_text(encoding="utf-8"),
            parse_constant=reject_non_standard_json_constant,
        )
    except json.JSONDecodeError as error:
        add_error(
            errors,
            reference_path,
            f"supplied request is invalid JSON at line {error.lineno}, column {error.colno}",
        )
        return None
    except NonStandardJsonConstant as error:
        add_error(
            errors,
            reference_path,
            f"supplied request contains non-standard constant '{error.constant}'",
        )
        return None
    except (OSError, RuntimeError, UnicodeError, ValueError) as error:
        add_error(errors, reference_path, f"cannot read supplied request: {error}")
        return None

    linked_errors: list[Diagnostic] = []
    validate_memory_request(request_value, linked_errors, file_reference)
    if linked_errors:
        for linked_error in linked_errors:
            add_error(
                errors,
                reference_path,
                f"supplied request {linked_error.path}: {linked_error.message}",
            )
        return None

    assert isinstance(request_value, dict)
    memory_entries = request_value["current_memory"]["long_term_entries"]
    playbooks = request_value["current_playbooks"]
    memory_ids = {
        entry["entry_id"]
        for entry in memory_entries
        if isinstance(entry, dict) and isinstance(entry.get("entry_id"), str)
    }
    playbook_ids = {
        playbook["playbook_id"]
        for playbook in playbooks
        if isinstance(playbook, dict) and isinstance(playbook.get("playbook_id"), str)
    }
    scope = request_value["current_memory"].get("scope", "full")
    return memory_ids, playbook_ids, scope


def validate_request_audit_reference(
    value: Any,
    request_path: Path,
    errors: list[Diagnostic],
    file_reference: FileReferenceValidator,
) -> None:
    reference_path = "$.request_file"
    previous_error_count = len(errors)
    file_reference(value, reference_path, errors)
    if len(errors) != previous_error_count or not isinstance(value, str):
        return
    try:
        audited_path = (
            file_reference.project_root / Path(*PurePosixPath(value).parts)
        ).resolve(strict=True)
    except (OSError, RuntimeError, ValueError) as error:
        add_error(errors, reference_path, f"cannot resolve audit request: {error}")
        return
    if audited_path != request_path:
        add_error(
            errors,
            reference_path,
            "must match the externally supplied --request file",
        )


def validate_decision_context(
    value: Any,
    path: str,
    errors: list[Diagnostic],
) -> None:
    if not require_object(value, path, errors):
        return
    required = {"reason"}
    allowed = required | {"rejected_alternatives"}
    check_object_shape(value, path, errors, required=required, allowed=allowed)
    if "reason" in value:
        check_string(value["reason"], f"{path}.reason", errors, min_length=1)
    if "rejected_alternatives" in value:
        check_array(
            value["rejected_alternatives"],
            f"{path}.rejected_alternatives",
            errors,
            validate_rejected_alternative,
        )


def validate_rejected_alternative(
    value: Any,
    path: str,
    errors: list[Diagnostic],
) -> None:
    if not require_object(value, path, errors):
        return
    required = {"alternative", "reason"}
    check_object_shape(value, path, errors, required=required, allowed=required)
    for key in required:
        if key in value:
            check_string(value[key], f"{path}.{key}", errors, min_length=1)


def validate_temporal_fields(
    value: dict[str, Any],
    path: str,
    errors: list[Diagnostic],
) -> None:
    parsed: dict[str, datetime] = {}
    for key in ("valid_from", "valid_until"):
        if key not in value:
            continue
        field_path = f"{path}.{key}"
        if check_date_time(value[key], field_path, errors):
            raw = value[key]
            normalized = raw[:-1] + "+00:00" if raw[-1] in "Zz" else raw
            parsed[key] = datetime.fromisoformat(normalized)
    if parsed and value.get("memory_kind") not in {"fact", "constraint"}:
        for key in parsed:
            add_error(
                errors,
                f"{path}.{key}",
                "is allowed only when memory_kind is 'fact' or 'constraint'",
            )
    if (
        "valid_from" in parsed
        and "valid_until" in parsed
        and parsed["valid_from"] >= parsed["valid_until"]
    ):
        add_error(errors, f"{path}.valid_until", "must be later than valid_from")


def validate_long_term_entry(
    value: Any,
    path: str,
    errors: list[Diagnostic],
    file_reference: FileReferenceValidator,
) -> None:
    if not require_object(value, path, errors):
        return
    required = {"entry_id", "title", "memory_kind", "content", "source_refs"}
    allowed = required | {
        "status", "decision_context", "tags", "aliases", "search_hints",
        "valid_from", "valid_until", "code_refs", "code_fingerprints",
    }
    check_object_shape(value, path, errors, required=required, allowed=allowed)
    if "entry_id" in value:
        check_stable_id(value["entry_id"], f"{path}.entry_id", errors)
    for key in ("title", "content"):
        if key in value:
            check_string(value[key], f"{path}.{key}", errors, min_length=1)
    if "memory_kind" in value:
        check_enum(
            value["memory_kind"],
            f"{path}.memory_kind",
            errors,
            {"fact", "experience", "principle", "decision", "constraint", "other"},
        )
    if "status" in value:
        check_enum(
            value["status"],
            f"{path}.status",
            errors,
            {"active", "disputed", "superseded", "rejected"},
        )
    if "decision_context" in value:
        validate_decision_context(
            value["decision_context"], f"{path}.decision_context", errors
        )
        if value.get("memory_kind") != "decision":
            add_error(
                errors,
                f"{path}.decision_context",
                "is allowed only when memory_kind is 'decision'",
            )
    if "source_refs" in value and check_array(
        value["source_refs"],
        f"{path}.source_refs",
        errors,
        file_reference,
        min_items=1,
    ):
        check_unique_strings(value["source_refs"], f"{path}.source_refs", errors)
    for key in ("tags", "aliases", "search_hints"):
        if key in value and check_array(
            value[key],
            f"{path}.{key}",
            errors,
            make_string_validator(min_length=1),
        ):
            check_unique_strings(value[key], f"{path}.{key}", errors)
    validate_temporal_fields(value, path, errors)
    if "code_refs" in value and check_array(
        value["code_refs"],
        f"{path}.code_refs",
        errors,
        file_reference,
    ):
        check_unique_strings(value["code_refs"], f"{path}.code_refs", errors)
    if "code_fingerprints" in value:
        if require_object(value["code_fingerprints"], f"{path}.code_fingerprints", errors):
            for fp_key, fp_val in value["code_fingerprints"].items():
                file_reference(fp_key, f"{path}.code_fingerprints.{fp_key}", errors)
                check_string(fp_val, f"{path}.code_fingerprints.{fp_key}", errors, min_length=1)


def validate_memory_index_entry(
    value: Any,
    path: str,
    errors: list[Diagnostic],
    file_reference: FileReferenceValidator,
) -> None:
    if not require_object(value, path, errors):
        return
    required = {
        "memory_id",
        "layer",
        "record_type",
        "title",
        "summary",
        "path",
        "locator",
        "status",
        "memory_kind",
        "tags",
        "aliases",
        "search_hints",
        "updated_at",
        "stale",
    }
    temporal = {"valid_from", "valid_until", "temporal_state"}
    check_object_shape(value, path, errors, required=required, allowed=required | temporal)
    if "memory_id" in value:
        check_storage_id(value["memory_id"], f"{path}.memory_id", errors)
    if "layer" in value:
        check_enum(value["layer"], f"{path}.layer", errors, MEMORY_LAYERS)
    if "record_type" in value:
        check_enum(
            value["record_type"],
            f"{path}.record_type",
            errors,
            MEMORY_RECORD_TYPES,
        )
    for key in ("title", "summary", "locator"):
        if key in value:
            check_string(value[key], f"{path}.{key}", errors, min_length=1)
    if "path" in value:
        file_reference(value["path"], f"{path}.path", errors)
    if "status" in value:
        check_enum(value["status"], f"{path}.status", errors, MEMORY_STATUSES)
    if "memory_kind" in value and value["memory_kind"] is not None:
        check_enum(value["memory_kind"], f"{path}.memory_kind", errors, MEMORY_KINDS)
    for key in ("tags", "aliases", "search_hints"):
        if key in value and check_array(
            value[key],
            f"{path}.{key}",
            errors,
            make_string_validator(min_length=1),
        ):
            check_unique_strings(value[key], f"{path}.{key}", errors)
    if "updated_at" in value and value["updated_at"] is not None:
        check_date_time(value["updated_at"], f"{path}.updated_at", errors)
    for key in ("valid_from", "valid_until"):
        if key in value and value[key] is not None:
            check_date_time(value[key], f"{path}.{key}", errors)
    if "temporal_state" in value:
        check_enum(
            value["temporal_state"], f"{path}.temporal_state", errors,
            {"timeless", "not-yet-valid", "current", "expired"},
        )

    layer = value.get("layer")
    record_type = value.get("record_type")
    memory_kind = value.get("memory_kind")
    stale = value.get("stale")
    if layer == "long-term":
        if record_type != "long-term-entry":
            add_error(
                errors,
                f"{path}.record_type",
                "must be 'long-term-entry' for Long-term Memory",
            )
        if memory_kind is None:
            add_error(errors, f"{path}.memory_kind", "is required for a Long-term entry")
        if stale is not None:
            add_error(errors, f"{path}.stale", "must be null outside the Temporary layer")
        for key in temporal:
            if key not in value:
                add_error(errors, f"{path}.{key}", "is required for a Long-term entry")
    elif layer == "temporary":
        for key in temporal:
            if key in value:
                add_error(errors, f"{path}.{key}", "is allowed only for Long-term Memory")
        if record_type != "temporary":
            add_error(errors, f"{path}.record_type", "must be 'temporary' for Temporary Memory")
        if memory_kind is not None:
            add_error(errors, f"{path}.memory_kind", "must be null outside the Long-term layer")
        if not is_boolean(stale):
            add_error(errors, f"{path}.stale", "must be a boolean for Temporary Memory")
    elif layer == "task":
        for key in temporal:
            if key in value:
                add_error(errors, f"{path}.{key}", "is allowed only for Long-term Memory")
        if record_type not in {"task", "worker-state"}:
            add_error(errors, f"{path}.record_type", "must be a Task current-state record type")
        if memory_kind is not None:
            add_error(errors, f"{path}.memory_kind", "must be null outside the Long-term layer")
        if stale is not None:
            add_error(errors, f"{path}.stale", "must be null outside the Temporary layer")


def validate_memory_followup(
    value: Any,
    errors: list[Diagnostic],
    file_reference: FileReferenceValidator,
) -> None:
    if not require_object(value, "$", errors):
        return

    status = value.get("status")
    if status == "pending":
        required = {"followup_id", "title", "status", "created_at", "source_refs"}
        allowed = {
            "followup_id",
            "title",
            "status",
            "created_at",
            "source_refs",
            "related_ids",
        }
    elif status == "resolved":
        required = {
            "followup_id",
            "title",
            "status",
            "created_at",
            "source_refs",
            "resolved_at",
            "resolution",
        }
        allowed = {
            "followup_id",
            "title",
            "status",
            "created_at",
            "source_refs",
            "related_ids",
            "resolved_at",
            "resolution",
            "resolution_refs",
        }
    else:
        required = {"followup_id", "title", "status", "created_at", "source_refs"}
        allowed = {
            "followup_id",
            "title",
            "status",
            "created_at",
            "source_refs",
            "related_ids",
            "resolved_at",
            "resolution",
            "resolution_refs",
        }

    check_object_shape(value, "$", errors, required=required, allowed=allowed)

    if "followup_id" in value:
        check_stable_id(value["followup_id"], "$.followup_id", errors)
    if "title" in value:
        check_string(value["title"], "$.title", errors, min_length=1)
    if "status" in value:
        check_enum(value["status"], "$.status", errors, {"pending", "resolved"})
    if "created_at" in value:
        check_date_time(value["created_at"], "$.created_at", errors)
    if "source_refs" in value and check_array(
        value["source_refs"],
        "$.source_refs",
        errors,
        file_reference,
        min_items=1,
    ):
        check_unique_strings(value["source_refs"], "$.source_refs", errors)
    if "related_ids" in value and check_array(
        value["related_ids"],
        "$.related_ids",
        errors,
        lambda item, item_path, item_errors: check_stable_id(item, item_path, item_errors),
    ):
        check_unique_strings(value["related_ids"], "$.related_ids", errors)

    if status == "resolved":
        if "resolved_at" in value:
            check_date_time(value["resolved_at"], "$.resolved_at", errors)
        if "resolution" in value:
            check_string(value["resolution"], "$.resolution", errors, min_length=1)
        if "resolution_refs" in value and check_array(
            value["resolution_refs"],
            "$.resolution_refs",
            errors,
            file_reference,
        ):
            check_unique_strings(value["resolution_refs"], "$.resolution_refs", errors)


def validate_pending_followup_entry(
    value: Any,
    path: str,
    errors: list[Diagnostic],
    file_reference: FileReferenceValidator,
) -> None:
    if not require_object(value, path, errors):
        return
    required = {
        "followup_id",
        "title",
        "status",
        "created_at",
        "source_refs",
        "related_ids",
        "path",
    }
    check_object_shape(value, path, errors, required=required, allowed=required)
    if "followup_id" in value:
        check_stable_id(value["followup_id"], f"{path}.followup_id", errors)
    if "title" in value:
        check_string(value["title"], f"{path}.title", errors, min_length=1)
    if "status" in value:
        check_enum(value["status"], f"{path}.status", errors, {"pending"})
    if "created_at" in value:
        check_date_time(value["created_at"], f"{path}.created_at", errors)
    if "source_refs" in value and check_array(
        value["source_refs"],
        f"{path}.source_refs",
        errors,
        file_reference,
        min_items=1,
    ):
        check_unique_strings(value["source_refs"], f"{path}.source_refs", errors)
    if "related_ids" in value and check_array(
        value["related_ids"],
        f"{path}.related_ids",
        errors,
        lambda item, item_path, item_errors: check_stable_id(item, item_path, item_errors),
    ):
        check_unique_strings(value["related_ids"], f"{path}.related_ids", errors)
    if "path" in value:
        file_reference(value["path"], f"{path}.path", errors)


def validate_memory_index(
    value: Any,
    errors: list[Diagnostic],
    file_reference: FileReferenceValidator,
) -> None:
    if not require_object(value, "$", errors):
        return
    required = {"schema_version", "generated_at", "source_digest", "entries", "pending_followups"}
    check_object_shape(value, "$", errors, required=required, allowed=required)
    if value.get("schema_version") != 1:
        add_error(errors, "$.schema_version", "must equal 1")
    if "generated_at" in value:
        check_date_time(value["generated_at"], "$.generated_at", errors)
    if "source_digest" in value:
        if check_string(value["source_digest"], "$.source_digest", errors, min_length=1):
            if not SHA256_PATTERN.fullmatch(value["source_digest"]):
                add_error(errors, "$.source_digest", "must be a lowercase SHA-256 digest")
    if "entries" in value and check_array(
        value["entries"],
        "$.entries",
        errors,
        lambda item, item_path, item_errors: validate_memory_index_entry(
            item, item_path, item_errors, file_reference
        ),
    ):
        seen_ids: set[str] = set()
        for index, entry in enumerate(value["entries"]):
            if not is_object(entry):
                continue
            memory_id = entry.get("memory_id")
            if not isinstance(memory_id, str):
                continue
            if memory_id in seen_ids:
                add_error(
                    errors,
                    f"$.entries[{index}].memory_id",
                    "must be unique within the Memory Index",
                )
            seen_ids.add(memory_id)
    if "pending_followups" in value and check_array(
        value["pending_followups"],
        "$.pending_followups",
        errors,
        lambda item, item_path, item_errors: validate_pending_followup_entry(
            item, item_path, item_errors, file_reference
        ),
    ):
        seen_followup_ids: set[str] = set()
        for index, followup in enumerate(value["pending_followups"]):
            if not is_object(followup):
                continue
            fid = followup.get("followup_id")
            if not isinstance(fid, str):
                continue
            if fid in seen_followup_ids:
                add_error(
                    errors,
                    f"$.pending_followups[{index}].followup_id",
                    "must be unique within pending follow-ups",
                )
            seen_followup_ids.add(fid)


def validate_activity_event(
    value: Any,
    path: str,
    errors: list[Diagnostic],
    file_reference: FileReferenceValidator,
) -> None:
    if not require_object(value, path, errors):
        return
    required = {
        "event_id",
        "occurred_at",
        "event_type",
        "title",
        "summary",
        "source_refs",
        "status",
    }
    check_object_shape(value, path, errors, required=required, allowed=required)
    if "event_id" in value:
        if check_string(value["event_id"], f"{path}.event_id", errors, min_length=1):
            if not ACTIVITY_EVENT_ID_PATTERN.fullmatch(value["event_id"]):
                add_error(errors, f"{path}.event_id", "must match 'activity-<stable-id>'")
    if "occurred_at" in value:
        check_date_time(value["occurred_at"], f"{path}.occurred_at", errors)
    if "event_type" in value:
        check_enum(value["event_type"], f"{path}.event_type", errors, ACTIVITY_EVENT_TYPES)
    for key in ("title", "summary"):
        if key in value:
            check_string(value[key], f"{path}.{key}", errors, min_length=1)
    if "source_refs" in value and check_array(
        value["source_refs"],
        f"{path}.source_refs",
        errors,
        lambda item, item_path, item_errors: file_reference(item, item_path, item_errors),
        min_items=1,
    ):
        check_unique_strings(value["source_refs"], f"{path}.source_refs", errors)
    if "status" in value and value["status"] != "completed":
        add_error(errors, f"{path}.status", "must equal 'completed'")


def validate_checkpoint_observation(value: Any, errors: list[Diagnostic]) -> None:
    if not require_object(value, "$", errors):
        return
    legacy = {"request_id", "record_hash", "proposal_hash", "revision"}
    versioned = legacy | {"completion", "committed_at"}
    keys = set(value)
    if keys not in (legacy, versioned):
        missing = sorted(legacy - keys)
        unexpected = sorted(keys - versioned)
        for key in missing:
            add_error(errors, f"$.{key}", "is required")
        for key in unexpected:
            add_error(errors, f"$.{key}", "is not allowed")
        if ("completion" in keys) != ("committed_at" in keys):
            add_error(errors, "$", "completion and committed_at must appear together")
    if "request_id" in value:
        if check_string(value["request_id"], "$.request_id", errors, min_length=1):
            if not re.fullmatch(r"[a-z0-9][a-z0-9_-]{0,63}", value["request_id"]):
                add_error(errors, "$.request_id", "must be a valid checkpoint request id")
    for key in ("record_hash", "proposal_hash"):
        if key in value:
            if check_string(value[key], f"$.{key}", errors, min_length=64):
                if not SHA256_PATTERN.fullmatch(value[key]):
                    add_error(errors, f"$.{key}", "must be a lowercase SHA-256 hash")
    if "revision" in value:
        if (
            not isinstance(value["revision"], int)
            or isinstance(value["revision"], bool)
            or value["revision"] < 1
        ):
            add_error(errors, "$.revision", "must be an integer greater than or equal to 1")
    if "completion" in value:
        check_enum(value["completion"], "$.completion", errors, {"save", "recovery"})
    if "committed_at" in value:
        check_date_time(value["committed_at"], "$.committed_at", errors)


def validate_worker_approval(value: Any, errors: list[Diagnostic]) -> None:
    if not require_object(value, "$", errors):
        return
    required = {
        "schema_version", "record_type", "approval_id", "worker_id", "worker_name",
        "registry_id", "registry_revision", "worker_digest", "approved_at", "approved_by",
    }
    check_object_shape(value, "$", errors, required=required, allowed=required)
    if value.get("schema_version") != 1:
        add_error(errors, "$.schema_version", "must equal 1")
    if value.get("record_type") != "worker-approval":
        add_error(errors, "$.record_type", "must equal 'worker-approval'")
    for key in ("approval_id", "worker_id", "registry_id"):
        if key in value:
            if check_string(value[key], f"$.{key}", errors, min_length=1):
                if not CAPABILITY_PATTERN.fullmatch(value[key]):
                    add_error(errors, f"$.{key}", "must be a lowercase kebab-case id")
    for key in ("worker_name", "approved_by"):
        if key in value:
            if check_string(value[key], f"$.{key}", errors, min_length=1):
                if not value[key].strip():
                    add_error(errors, f"$.{key}", "must contain a non-whitespace character")
    if "registry_revision" in value:
        revision = value["registry_revision"]
        if not isinstance(revision, int) or isinstance(revision, bool) or revision < 0:
            add_error(errors, "$.registry_revision", "must be an integer greater than or equal to 0")
    if "worker_digest" in value:
        if check_string(value["worker_digest"], "$.worker_digest", errors, min_length=64):
            if not SHA256_PATTERN.fullmatch(value["worker_digest"]):
                add_error(errors, "$.worker_digest", "must be a lowercase SHA-256 hash")
    if "approved_at" in value:
        check_date_time(value["approved_at"], "$.approved_at", errors)


def validate_decision_record(
    value: Any,
    errors: list[Diagnostic],
    file_reference: FileReferenceValidator,
) -> None:
    if not require_object(value, "$", errors):
        return
    required = {
        "schema_version",
        "record_type",
        "decision_id",
        "title",
        "outcome",
        "importance",
        "decided_at",
        "decided_by",
        "reason",
        "target_ids",
        "source_refs",
    }
    allowed = required | {"superseded_by"}
    check_object_shape(value, "$", errors, required=required, allowed=allowed)
    if value.get("schema_version") != 1:
        add_error(errors, "$.schema_version", "must equal 1")
    if value.get("record_type") != "decision":
        add_error(errors, "$.record_type", "must equal 'decision'")
    if "decision_id" in value:
        check_stable_id(value["decision_id"], "$.decision_id", errors)
    for key in ("title", "decided_by", "reason"):
        if key in value:
            check_string(value[key], f"$.{key}", errors, min_length=1)
    if "outcome" in value:
        check_enum(
            value["outcome"],
            "$.outcome",
            errors,
            {"approved", "superseded", "rejected"},
        )
    if "importance" in value:
        check_enum(
            value["importance"], "$.importance", errors, {"milestone", "routine"}
        )
    if "decided_at" in value:
        check_date_time(value["decided_at"], "$.decided_at", errors)
    for key in ("target_ids", "source_refs"):
        if key not in value:
            continue
        item_validator = (
            file_reference
            if key == "source_refs"
            else lambda item, path, item_errors: check_string(
                item, path, item_errors, min_length=1
            )
        )
        min_items = 0 if key == "target_ids" and value.get("outcome") == "rejected" else 1
        if check_array(
            value[key], f"$.{key}", errors, item_validator, min_items=min_items
        ):
            check_unique_strings(value[key], f"$.{key}", errors)
    outcome = value.get("outcome")
    if outcome == "superseded":
        if "superseded_by" not in value:
            add_error(
                errors,
                "$.superseded_by",
                "is required when outcome is 'superseded'",
            )
        else:
            check_string(value["superseded_by"], "$.superseded_by", errors, min_length=1)
    elif "superseded_by" in value:
        add_error(errors, "$.superseded_by", "is allowed only when outcome is 'superseded'")


def validate_activity_index(
    value: Any,
    errors: list[Diagnostic],
    file_reference: FileReferenceValidator,
) -> None:
    if not require_object(value, "$", errors):
        return
    required = {"schema_version", "generated_at", "source_digest", "events"}
    check_object_shape(value, "$", errors, required=required, allowed=required)
    if value.get("schema_version") != 1:
        add_error(errors, "$.schema_version", "must equal 1")
    if "generated_at" in value:
        check_date_time(value["generated_at"], "$.generated_at", errors)
    if "source_digest" in value:
        if check_string(value["source_digest"], "$.source_digest", errors, min_length=1):
            if not SHA256_PATTERN.fullmatch(value["source_digest"]):
                add_error(errors, "$.source_digest", "must be a lowercase SHA-256 digest")
    if "events" in value and check_array(
        value["events"],
        "$.events",
        errors,
        lambda item, item_path, item_errors: validate_activity_event(
            item, item_path, item_errors, file_reference
        ),
    ):
        seen_ids: set[str] = set()
        for index, event in enumerate(value["events"]):
            if not is_object(event):
                continue
            event_id = event.get("event_id")
            if not isinstance(event_id, str):
                continue
            if event_id in seen_ids:
                add_error(
                    errors,
                    f"$.events[{index}].event_id",
                    "must be unique within the Activity Index",
                )
            seen_ids.add(event_id)


def validate_playbook(
    value: Any,
    path: str,
    errors: list[Diagnostic],
    file_reference: FileReferenceValidator,
) -> None:
    if not require_object(value, path, errors):
        return
    required = {
        "playbook_id",
        "file_path",
        "title",
        "trigger",
        "steps",
        "checks",
        "status",
        "revision",
        "updated_at",
        "updated_by",
        "source_refs",
    }
    check_object_shape(value, path, errors, required=required, allowed=required)
    if "playbook_id" in value:
        check_stable_id(value["playbook_id"], f"{path}.playbook_id", errors)
    if "file_path" in value:
        canonical_path = check_canonical_playbook_path(
            value["file_path"], f"{path}.file_path", errors
        )
        file_reference(value["file_path"], f"{path}.file_path", errors)
        if canonical_path is not None:
            resolved_path = file_reference.project_root / Path(*canonical_path.parts)
            try:
                is_file = resolved_path.is_file()
            except (OSError, ValueError) as error:
                add_error(
                    errors,
                    f"{path}.file_path",
                    f"cannot inspect canonical Playbook: {error}",
                )
                is_file = False
            if is_file:
                metadata = load_canonical_playbook_metadata(
                    resolved_path, f"{path}.file_path", errors
                )
                if metadata is not None:
                    for key in sorted(PLAYBOOK_METADATA_FIELDS):
                        if key in value and metadata[key] != value[key]:
                            add_error(
                                errors,
                                f"{path}.{key}",
                                f"must match canonical Playbook metadata ({metadata[key]!r})",
                            )
    for key in ("title", "trigger", "updated_by"):
        if key in value:
            check_string(value[key], f"{path}.{key}", errors, min_length=1)
    for key, min_items in (("steps", 1), ("checks", 0)):
        if key in value and check_array(
            value[key],
            f"{path}.{key}",
            errors,
            lambda item, item_path, item_errors: check_string(
                item, item_path, item_errors, min_length=1
            ),
            min_items=min_items,
        ):
            check_unique_strings(value[key], f"{path}.{key}", errors)
    if "status" in value and value["status"] != "active":
        add_error(errors, f"{path}.status", "must equal 'active'")
    if "revision" in value and (
        not isinstance(value["revision"], int)
        or is_boolean(value["revision"])
        or value["revision"] < 0
    ):
        add_error(errors, f"{path}.revision", "must be a non-negative integer")
    if "updated_at" in value:
        check_date_time(value["updated_at"], f"{path}.updated_at", errors)
    if "source_refs" in value and check_array(
        value["source_refs"],
        f"{path}.source_refs",
        errors,
        file_reference,
        min_items=1,
    ):
        check_unique_strings(value["source_refs"], f"{path}.source_refs", errors)


def validate_memory_item(
    value: Any,
    path: str,
    errors: list[Diagnostic],
    file_reference: FileReferenceValidator,
    *,
    content_min_length: int,
) -> None:
    if not require_object(value, path, errors):
        return
    required = {"title", "content", "source_refs"}
    check_object_shape(value, path, errors, required=required, allowed=required)
    if "title" in value:
        check_string(value["title"], f"{path}.title", errors, min_length=1)
    if "content" in value:
        check_string(
            value["content"],
            f"{path}.content",
            errors,
            min_length=content_min_length,
        )
    if "source_refs" in value:
        if check_array(
            value["source_refs"],
            f"{path}.source_refs",
            errors,
            file_reference,
            min_items=1,
        ):
            check_unique_strings(value["source_refs"], f"{path}.source_refs", errors)


def validate_long_term_candidate(
    value: Any,
    path: str,
    errors: list[Diagnostic],
    file_reference: FileReferenceValidator,
) -> None:
    if not require_object(value, path, errors):
        return

    required = {
        "candidate_id",
        "title",
        "memory_kind",
        "action",
        "match",
        "conflict_status",
        "content",
        "rationale",
        "source",
        "source_refs",
    }
    allowed = required | {
        "decision_context", "tags", "aliases", "search_hints",
        "valid_from", "valid_until",
    }
    check_object_shape(value, path, errors, required=required, allowed=allowed)

    if "candidate_id" in value:
        check_stable_id(value["candidate_id"], f"{path}.candidate_id", errors)
    if "title" in value:
        check_string(value["title"], f"{path}.title", errors, min_length=1)
    if "memory_kind" in value:
        check_enum(
            value["memory_kind"],
            f"{path}.memory_kind",
            errors,
            {"fact", "experience", "principle", "decision", "constraint", "other"},
        )
    action_valid = False
    if "action" in value:
        action_valid = check_enum(
            value["action"],
            f"{path}.action",
            errors,
            {"UPDATE", "MERGE", "CREATE", "SKIP"},
        )
    if "conflict_status" in value:
        check_enum(
            value["conflict_status"],
            f"{path}.conflict_status",
            errors,
            {"none", "pending-confirmation", "confirmed"},
        )
    for key in ("content", "rationale"):
        if key in value:
            check_string(value[key], f"{path}.{key}", errors, min_length=1)
    if "decision_context" in value:
        validate_decision_context(
            value["decision_context"], f"{path}.decision_context", errors
        )
        if value.get("memory_kind") != "decision":
            add_error(
                errors,
                f"{path}.decision_context",
                "is allowed only when memory_kind is 'decision'",
            )
    for key in ("tags", "aliases", "search_hints"):
        if key in value and check_array(
            value[key],
            f"{path}.{key}",
            errors,
            make_string_validator(min_length=1),
        ):
            check_unique_strings(value[key], f"{path}.{key}", errors)
    validate_temporal_fields(value, path, errors)

    classification: str | None = None
    entry_ids: list[Any] | None = None
    if "match" in value and require_object(value["match"], f"{path}.match", errors):
        match = value["match"]
        check_object_shape(
            match,
            f"{path}.match",
            errors,
            required={"classification", "entry_ids"},
            allowed={"classification", "entry_ids"},
        )
        if "classification" in match and check_enum(
            match["classification"],
            f"{path}.match.classification",
            errors,
            {"novel", "duplicate", "overlap", "conflict", "low-value"},
        ):
            classification = match["classification"]
        if "entry_ids" in match and check_array(
            match["entry_ids"],
            f"{path}.match.entry_ids",
            errors,
            lambda item, item_path, item_errors: check_stable_id(
                item, item_path, item_errors
            ),
        ):
            entry_ids = match["entry_ids"]
            check_unique_strings(entry_ids, f"{path}.match.entry_ids", errors)

    if action_valid and classification is not None and entry_ids is not None:
        action = value["action"]
        if action == "CREATE" and (classification != "novel" or entry_ids):
            add_error(
                errors,
                f"{path}.match",
                "CREATE requires classification 'novel' and no entry IDs",
            )
        elif action == "UPDATE" and (
            classification not in {"overlap", "conflict"} or len(entry_ids) != 1
        ):
            add_error(
                errors,
                f"{path}.match",
                "UPDATE requires overlap or conflict with exactly one entry ID",
            )
        elif action == "MERGE" and (
            classification not in {"overlap", "conflict"} or len(entry_ids) < 2
        ):
            add_error(
                errors,
                f"{path}.match",
                "MERGE requires overlap or conflict with at least two entry IDs",
            )
        elif action == "SKIP":
            valid_skip = (
                classification == "duplicate" and len(entry_ids) >= 1
            ) or (classification == "low-value" and len(entry_ids) == 0)
            if not valid_skip:
                add_error(
                    errors,
                    f"{path}.match",
                    "SKIP requires a duplicate with targets or low-value with no targets",
                )

    if classification is not None and "conflict_status" in value:
        conflict_status = value["conflict_status"]
        if classification == "conflict" and conflict_status not in {
            "pending-confirmation",
            "confirmed",
        }:
            add_error(
                errors,
                f"{path}.conflict_status",
                "must be pending-confirmation or confirmed for a conflict",
            )
        elif classification != "conflict" and conflict_status != "none":
            add_error(
                errors,
                f"{path}.conflict_status",
                "must be none when the match is not a conflict",
            )

    if "source" in value and require_object(value["source"], f"{path}.source", errors):
        source = value["source"]
        check_object_shape(
            source,
            f"{path}.source",
            errors,
            required={"type", "id", "created_at"},
            allowed={"type", "id", "workspace_id", "created_at"},
        )
        if "type" in source:
            check_enum(
                source["type"],
                f"{path}.source.type",
                errors,
                {"temporary", "task", "user-message"},
            )
        if "id" in source:
            check_storage_id(source["id"], f"{path}.source.id", errors)
        if "workspace_id" in source:
            check_storage_id(
                source["workspace_id"], f"{path}.source.workspace_id", errors
            )
        if "created_at" in source:
            check_date_time(source["created_at"], f"{path}.source.created_at", errors)

    if "source_refs" in value and check_array(
        value["source_refs"],
        f"{path}.source_refs",
        errors,
        file_reference,
        min_items=1,
    ):
        check_unique_strings(value["source_refs"], f"{path}.source_refs", errors)


def validate_playbook_candidate(
    value: Any,
    path: str,
    errors: list[Diagnostic],
    file_reference: FileReferenceValidator,
) -> None:
    if not require_object(value, path, errors):
        return

    required = {
        "candidate_id",
        "title",
        "trigger",
        "steps",
        "checks",
        "action",
        "match",
        "rationale",
        "source",
        "source_refs",
        "evidence_refs",
        "status",
    }
    check_object_shape(value, path, errors, required=required, allowed=required)

    if "candidate_id" in value:
        check_stable_id(value["candidate_id"], f"{path}.candidate_id", errors)
    for key in ("title", "trigger", "rationale"):
        if key in value:
            check_string(value[key], f"{path}.{key}", errors, min_length=1)
    for key, min_items in (("steps", 1), ("checks", 0)):
        if key in value and check_array(
            value[key],
            f"{path}.{key}",
            errors,
            lambda item, item_path, item_errors: check_string(
                item, item_path, item_errors, min_length=1
            ),
            min_items=min_items,
        ):
            check_unique_strings(value[key], f"{path}.{key}", errors)

    action_valid = False
    if "action" in value:
        action_valid = check_enum(
            value["action"],
            f"{path}.action",
            errors,
            {"UPDATE", "MERGE", "CREATE", "SKIP"},
        )

    classification = None
    playbook_ids = None
    if "match" in value and require_object(value["match"], f"{path}.match", errors):
        match = value["match"]
        check_object_shape(
            match,
            f"{path}.match",
            errors,
            required={"classification", "playbook_ids"},
            allowed={"classification", "playbook_ids"},
        )
        if "classification" in match and check_enum(
            match["classification"],
            f"{path}.match.classification",
            errors,
            {"novel", "duplicate", "overlap", "conflict", "low-value"},
        ):
            classification = match["classification"]
        if "playbook_ids" in match and check_array(
            match["playbook_ids"],
            f"{path}.match.playbook_ids",
            errors,
            lambda item, item_path, item_errors: check_stable_id(
                item, item_path, item_errors
            ),
        ):
            playbook_ids = match["playbook_ids"]
            check_unique_strings(playbook_ids, f"{path}.match.playbook_ids", errors)

    if action_valid and classification is not None and playbook_ids is not None:
        action = value["action"]
        if action == "CREATE" and (classification != "novel" or playbook_ids):
            add_error(
                errors,
                f"{path}.match",
                "CREATE requires classification 'novel' and no Playbook IDs",
            )
        elif action == "UPDATE" and (
            classification not in {"overlap", "conflict"}
            or len(playbook_ids) != 1
        ):
            add_error(
                errors,
                f"{path}.match",
                "UPDATE requires overlap or conflict with exactly one Playbook ID",
            )
        elif action == "MERGE" and (
            classification not in {"overlap", "conflict"}
            or len(playbook_ids) < 2
        ):
            add_error(
                errors,
                f"{path}.match",
                "MERGE requires overlap or conflict with at least two Playbook IDs",
            )
        elif action == "SKIP":
            valid_skip = (
                classification == "duplicate" and len(playbook_ids) >= 1
            ) or (classification == "low-value" and len(playbook_ids) == 0)
            if not valid_skip:
                add_error(
                    errors,
                    f"{path}.match",
                    "SKIP requires a duplicate with targets or low-value with no targets",
                )

    if "source" in value and require_object(value["source"], f"{path}.source", errors):
        source = value["source"]
        check_object_shape(
            source,
            f"{path}.source",
            errors,
            required={"type", "id", "created_at"},
            allowed={"type", "id", "workspace_id", "created_at"},
        )
        if "type" in source:
            check_enum(
                source["type"],
                f"{path}.source.type",
                errors,
                {"temporary", "task", "user-message"},
            )
        if "id" in source:
            check_storage_id(source["id"], f"{path}.source.id", errors)
        if "workspace_id" in source:
            check_storage_id(
                source["workspace_id"], f"{path}.source.workspace_id", errors
            )
        if "created_at" in source:
            check_date_time(source["created_at"], f"{path}.source.created_at", errors)

    if "source_refs" in value and check_array(
        value["source_refs"],
        f"{path}.source_refs",
        errors,
        file_reference,
        min_items=1,
    ):
        check_unique_strings(value["source_refs"], f"{path}.source_refs", errors)

    if "evidence_refs" in value and check_array(
        value["evidence_refs"],
        f"{path}.evidence_refs",
        errors,
        file_reference,
    ):
        check_unique_strings(value["evidence_refs"], f"{path}.evidence_refs", errors)
        if action_valid and value["action"] != "SKIP" and not value["evidence_refs"]:
            add_error(
                errors,
                f"{path}.evidence_refs",
                "must contain execution evidence for CREATE, UPDATE, or MERGE",
            )

    if "status" in value and value["status"] != "candidate":
        add_error(errors, f"{path}.status", "must equal 'candidate'")


def validate_memory_response(
    value: Any,
    errors: list[Diagnostic],
    file_reference: FileReferenceValidator,
    request_path: Path,
) -> None:
    path = "$"
    if not require_object(value, path, errors):
        return
    required = {"status", "request_file", "current"}
    allowed = required | {
        "references",
        "discarded",
        "long_term_candidates",
        "playbook_candidates",
        "notes",
    }
    check_object_shape(value, path, errors, required=required, allowed=allowed)

    request_targets = load_memory_request(request_path, errors, file_reference)
    current_memory_ids, current_playbook_ids, current_memory_scope = (
        request_targets if request_targets is not None else (None, None, None)
    )
    if "request_file" in value:
        validate_request_audit_reference(
            value["request_file"], request_path, errors, file_reference
        )

    if "status" in value and value["status"] != "completed":
        add_error(errors, "$.status", "must equal 'completed'")
    if "current" in value:
        current = value["current"]
        if require_object(current, "$.current", errors):
            check_object_shape(
                current,
                "$.current",
                errors,
                required={"content"},
                allowed={"content"},
            )
            if "content" in current:
                check_plain_object(current["content"], "$.current.content", errors)
    if "references" in value:
        check_array(
            value["references"],
            "$.references",
            errors,
            lambda item, item_path, item_errors: validate_memory_item(
                item,
                item_path,
                item_errors,
                file_reference,
                content_min_length=0,
            ),
        )
    if "long_term_candidates" in value:
        candidates = value["long_term_candidates"]
        if check_array(
            candidates,
            "$.long_term_candidates",
            errors,
            lambda item, item_path, item_errors: validate_long_term_candidate(
                item,
                item_path,
                item_errors,
                file_reference,
            ),
        ):
            seen_candidate_ids: set[str] = set()
            for index, candidate in enumerate(candidates):
                if not is_object(candidate):
                    continue
                candidate_id = candidate.get("candidate_id")
                if not isinstance(candidate_id, str):
                    continue
                if candidate_id in seen_candidate_ids:
                    add_error(
                        errors,
                        f"$.long_term_candidates[{index}].candidate_id",
                        "must be unique within the response",
                    )
                seen_candidate_ids.add(candidate_id)
            if current_memory_ids is not None and current_memory_scope == "bounded":
                for index, candidate in enumerate(candidates):
                    if not is_object(candidate) or not is_object(candidate.get("match")):
                        continue
                    entry_ids = candidate["match"].get("entry_ids")
                    if not is_array(entry_ids):
                        continue
                    for entry_index, entry_id in enumerate(entry_ids):
                        if isinstance(entry_id, str) and entry_id not in current_memory_ids:
                            add_error(
                                errors,
                                f"$.long_term_candidates[{index}].match."
                                f"entry_ids[{entry_index}]",
                                "must reference Long-term Memory from the externally supplied request",
                            )
    if "playbook_candidates" in value:
        candidates = value["playbook_candidates"]
        if check_array(
            candidates,
            "$.playbook_candidates",
            errors,
            lambda item, item_path, item_errors: validate_playbook_candidate(
                item,
                item_path,
                item_errors,
                file_reference,
            ),
        ):
            seen_candidate_ids: set[str] = set()
            for index, candidate in enumerate(candidates):
                if not is_object(candidate):
                    continue
                candidate_id = candidate.get("candidate_id")
                if not isinstance(candidate_id, str):
                    continue
                if candidate_id in seen_candidate_ids:
                    add_error(
                        errors,
                        f"$.playbook_candidates[{index}].candidate_id",
                        "must be unique within the response",
                    )
                seen_candidate_ids.add(candidate_id)
            if current_playbook_ids is not None:
                for index, candidate in enumerate(candidates):
                    if not is_object(candidate) or not is_object(candidate.get("match")):
                        continue
                    playbook_ids = candidate["match"].get("playbook_ids")
                    if not is_array(playbook_ids):
                        continue
                    for playbook_index, playbook_id in enumerate(playbook_ids):
                        if (
                            isinstance(playbook_id, str)
                            and playbook_id not in current_playbook_ids
                        ):
                            add_error(
                                errors,
                                f"$.playbook_candidates[{index}].match."
                                f"playbook_ids[{playbook_index}]",
                                "must reference a Playbook from the externally supplied request",
                            )


def validate_provenance(
    value: Any,
    path: str,
    errors: list[Diagnostic],
    file_reference: FileReferenceValidator,
) -> None:
    if not require_object(value, path, errors):
        return
    required = {
        "author",
        "branch",
        "commit",
        "task_id",
        "memory_path",
        "claim",
        "source_refs",
        "created_at",
    }
    check_object_shape(value, path, errors, required=required, allowed=required)
    for key in ("author", "branch", "commit", "claim"):
        if key in value:
            check_string(value[key], f"{path}.{key}", errors, min_length=1)
    if "task_id" in value:
        check_storage_id(value["task_id"], f"{path}.task_id", errors)
    if "memory_path" in value:
        check_canonical_path(value["memory_path"], f"{path}.memory_path", errors)
    if "source_refs" in value and check_array(
        value["source_refs"],
        f"{path}.source_refs",
        errors,
        file_reference,
        min_items=1,
    ):
        check_unique_strings(value["source_refs"], f"{path}.source_refs", errors)
    if "created_at" in value:
        check_date_time(value["created_at"], f"{path}.created_at", errors)


def validate_conflict_record(
    value: Any,
    path: str,
    errors: list[Diagnostic],
    file_reference: FileReferenceValidator,
) -> None:
    if not require_object(value, path, errors):
        return
    required = {
        "conflict_id",
        "topic",
        "status",
        "reason",
        "ours",
        "theirs",
    }
    allowed = required | {"resolution"}
    check_object_shape(value, path, errors, required=required, allowed=allowed)
    if "conflict_id" in value:
        check_stable_id(value["conflict_id"], f"{path}.conflict_id", errors)
    for key in ("topic", "reason"):
        if key in value:
            check_string(value[key], f"{path}.{key}", errors, min_length=1)
    if "status" in value:
        check_enum(
            value["status"],
            f"{path}.status",
            errors,
            {"pending-confirmation", "resolved"},
        )
    if "ours" in value:
        validate_provenance(value["ours"], f"{path}.ours", errors, file_reference)
    if "theirs" in value:
        validate_provenance(value["theirs"], f"{path}.theirs", errors, file_reference)
    if "resolution" in value and require_object(
        value["resolution"], f"{path}.resolution", errors
    ):
        res = value["resolution"]
        res_req = {"strategy", "resolved_by", "resolved_at"}
        res_allowed = res_req | {"notes"}
        check_object_shape(
            res, f"{path}.resolution", errors, required=res_req, allowed=res_allowed
        )
        for res_key in ("strategy", "resolved_by"):
            if res_key in res:
                check_string(
                    res[res_key], f"{path}.resolution.{res_key}", errors, min_length=1
                )
        if "resolved_at" in res:
            check_date_time(
                res["resolved_at"], f"{path}.resolution.resolved_at", errors
            )
        if "notes" in res:
            check_string(res["notes"], f"{path}.resolution.notes", errors)


def validate_merged_long_term_entry(
    value: Any,
    path: str,
    errors: list[Diagnostic],
    file_reference: FileReferenceValidator,
) -> None:
    if not require_object(value, path, errors):
        return
    required = {
        "entry_id",
        "title",
        "memory_kind",
        "content",
        "source_refs",
        "action_taken",
    }
    allowed = required | {"status", "superseded_entry_ids", "decision_context"}
    check_object_shape(value, path, errors, required=required, allowed=allowed)
    if "entry_id" in value:
        check_stable_id(value["entry_id"], f"{path}.entry_id", errors)
    for key in ("title", "content"):
        if key in value:
            check_string(value[key], f"{path}.{key}", errors, min_length=1)
    if "memory_kind" in value:
        check_enum(
            value["memory_kind"],
            f"{path}.memory_kind",
            errors,
            {"fact", "experience", "principle", "decision", "constraint", "other"},
        )
    if "action_taken" in value:
        check_enum(
            value["action_taken"],
            f"{path}.action_taken",
            errors,
            {"kept_ours", "kept_theirs", "merged", "novel_ours", "novel_theirs"},
        )
    if "status" in value:
        check_enum(
            value["status"],
            f"{path}.status",
            errors,
            {"active", "superseded", "rejected"},
        )
    if "decision_context" in value:
        validate_decision_context(
            value["decision_context"], f"{path}.decision_context", errors
        )
        if value.get("memory_kind") != "decision":
            add_error(
                errors,
                f"{path}.decision_context",
                "is allowed only when memory_kind is 'decision'",
            )
    if "superseded_entry_ids" in value and check_array(
        value["superseded_entry_ids"],
        f"{path}.superseded_entry_ids",
        errors,
        lambda item, item_path, item_errors: check_stable_id(
            item, item_path, item_errors
        ),
    ):
        check_unique_strings(
            value["superseded_entry_ids"], f"{path}.superseded_entry_ids", errors
        )
    if "source_refs" in value and check_array(
        value["source_refs"],
        f"{path}.source_refs",
        errors,
        file_reference,
        min_items=1,
    ):
        check_unique_strings(value["source_refs"], f"{path}.source_refs", errors)


def validate_memory_merge_request(
    value: Any, errors: list[Diagnostic], file_reference: FileReferenceValidator
) -> None:
    path = "$"
    if not require_object(value, path, errors):
        return
    required = {"file_path", "base_entries", "ours_entries", "theirs_entries"}
    allowed = required | {"merge_hints"}
    check_object_shape(value, path, errors, required=required, allowed=allowed)

    if "file_path" in value:
        check_canonical_path(value["file_path"], "$.file_path", errors)

    for key in ("base_entries", "ours_entries", "theirs_entries"):
        if key in value:
            entries = value[key]
            if check_array(
                entries,
                f"$.{key}",
                errors,
                lambda item, item_path, item_errors: validate_long_term_entry(
                    item, item_path, item_errors, file_reference
                ),
            ):
                seen_ids: set[str] = set()
                for index, entry in enumerate(entries):
                    if not is_object(entry):
                        continue
                    entry_id = entry.get("entry_id")
                    if not isinstance(entry_id, str):
                        continue
                    if entry_id in seen_ids:
                        add_error(
                            errors,
                            f"$.{key}[{index}].entry_id",
                            f"must be unique within {key}",
                        )
                    seen_ids.add(entry_id)

    if "merge_hints" in value:
        check_plain_object(value["merge_hints"], "$.merge_hints", errors)


def validate_memory_merge_response(
    value: Any, errors: list[Diagnostic], file_reference: FileReferenceValidator
) -> None:
    path = "$"
    if not require_object(value, path, errors):
        return
    required = {
        "status",
        "merged_entries",
        "resolved",
        "unresolved_conflicts",
        "requires_human_review",
    }
    allowed = required | {"notes"}
    check_object_shape(value, path, errors, required=required, allowed=allowed)

    if "status" in value and value["status"] != "completed":
        add_error(errors, "$.status", "must equal 'completed'")

    if "merged_entries" in value:
        merged = value["merged_entries"]
        if check_array(
            merged,
            "$.merged_entries",
            errors,
            lambda item, item_path, item_errors: validate_merged_long_term_entry(
                item, item_path, item_errors, file_reference
            ),
        ):
            seen_entry_ids: set[str] = set()
            for index, entry in enumerate(merged):
                if not is_object(entry):
                    continue
                entry_id = entry.get("entry_id")
                if not isinstance(entry_id, str):
                    continue
                if entry_id in seen_entry_ids:
                    add_error(
                        errors,
                        f"$.merged_entries[{index}].entry_id",
                        "must be unique within merged_entries",
                    )
                seen_entry_ids.add(entry_id)

    if "resolved" in value:
        check_array(
            value["resolved"],
            "$.resolved",
            errors,
            lambda item, item_path, item_errors: check_string(
                item, item_path, item_errors, min_length=1
            ),
        )

    has_unresolved_conflicts = False
    if "unresolved_conflicts" in value:
        conflicts = value["unresolved_conflicts"]
        if check_array(
            conflicts,
            "$.unresolved_conflicts",
            errors,
            lambda item, item_path, item_errors: validate_conflict_record(
                item, item_path, item_errors, file_reference
            ),
        ):
            if len(conflicts) > 0:
                has_unresolved_conflicts = True
            seen_conflict_ids: set[str] = set()
            for index, conflict in enumerate(conflicts):
                if not is_object(conflict):
                    continue
                conflict_id = conflict.get("conflict_id")
                if not isinstance(conflict_id, str):
                    continue
                if conflict_id in seen_conflict_ids:
                    add_error(
                        errors,
                        f"$.unresolved_conflicts[{index}].conflict_id",
                        "must be unique within unresolved_conflicts",
                    )
                seen_conflict_ids.add(conflict_id)

    if "requires_human_review" in value:
        check_boolean(
            value["requires_human_review"], "$.requires_human_review", errors
        )
        if has_unresolved_conflicts and value["requires_human_review"] is not True:
            add_error(
                errors,
                "$.requires_human_review",
                "must be true when unresolved conflicts exist",
            )


def parse_args(argv: list[str]) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Validate a XiaoTao protocol artifact without changing workflow state."
    )
    parser.add_argument(
        "kind",
        choices=(
            "handoff",
            "memory-index",
            "memory-request",
            "memory-response",
            "memory-merge-request",
            "memory-merge-response",
            "memory-followup",
            "memory-source",
            "decision-record",
            "activity-event",
            "activity-index",
            "checkpoint-observation",
            "worker-approval",
        ),
    )
    parser.add_argument("file", type=Path)
    parser.add_argument("--project-root", type=Path, default=Path.cwd())
    parser.add_argument(
        "--request",
        type=Path,
        help="trusted Memory Worker request used to validate a memory-response",
    )
    parser.add_argument("--json", action="store_true", dest="json_output")
    args = parser.parse_args(argv)
    if args.kind == "memory-response" and args.request is None:
        parser.error("--request is required for memory-response")
    if args.kind != "memory-response" and args.request is not None:
        parser.error("--request is only valid for memory-response")
    return args


def emit_result(
    args: argparse.Namespace, errors: list[Diagnostic], *, output_file: Path
) -> None:
    if args.json_output:
        payload = {
            "valid": not errors,
            "kind": args.kind,
            "file": str(output_file),
            "errors": [asdict(error) for error in errors],
        }
        print(json.dumps(payload, ensure_ascii=False, sort_keys=True))
        return

    if not errors:
        print(f"valid: {args.kind} {output_file}")
        return
    for error in errors:
        print(f"{error.path}: {error.message}", file=sys.stderr)


def main(argv: list[str] | None = None) -> int:
    args = parse_args(sys.argv[1:] if argv is None else argv)
    try:
        project_root = args.project_root.resolve(strict=True)
        if not project_root.is_dir():
            raise NotADirectoryError(f"project root is not a directory: {project_root}")
        output_file = args.file.resolve(strict=True)
        raw = output_file.read_text(encoding="utf-8")
        request_file = args.request.resolve(strict=True) if args.request is not None else None
        if request_file is not None and not request_file.is_file():
            raise FileNotFoundError(f"request is not a file: {request_file}")
    except (OSError, RuntimeError, UnicodeError, ValueError) as error:
        print(f"validator error: {error}", file=sys.stderr)
        return 2

    try:
        value = json.loads(raw, parse_constant=reject_non_standard_json_constant)
    except json.JSONDecodeError as error:
        errors = [
            Diagnostic(
                "$",
                f"invalid JSON at line {error.lineno}, column {error.colno}: {error.msg}",
            )
        ]
        emit_result(args, errors, output_file=output_file)
        return 1
    except NonStandardJsonConstant as error:
        errors = [
            Diagnostic(
                "$",
                f"invalid JSON: non-standard constant '{error.constant}'",
            )
        ]
        emit_result(args, errors, output_file=output_file)
        return 1

    file_reference = FileReferenceValidator(project_root)
    errors: list[Diagnostic] = []
    if args.kind == "handoff":
        validate_handoff(value, errors, file_reference)
    elif args.kind == "memory-index":
        validate_memory_index(value, errors, file_reference)
    elif args.kind == "memory-request":
        validate_memory_request(value, errors, file_reference)
    elif args.kind == "memory-response":
        assert request_file is not None
        validate_memory_response(value, errors, file_reference, request_file)
    elif args.kind == "memory-merge-request":
        validate_memory_merge_request(value, errors, file_reference)
    elif args.kind == "memory-merge-response":
        validate_memory_merge_response(value, errors, file_reference)
    elif args.kind == "memory-followup":
        validate_memory_followup(value, errors, file_reference)
    elif args.kind == "memory-source":
        validate_memory_source(value, errors, output_file, project_root)
    elif args.kind == "decision-record":
        validate_decision_record(value, errors, file_reference)
    elif args.kind == "activity-event":
        validate_activity_event(value, "$", errors, file_reference)
    elif args.kind == "activity-index":
        validate_activity_index(value, errors, file_reference)
    elif args.kind == "checkpoint-observation":
        validate_checkpoint_observation(value, errors)
    elif args.kind == "worker-approval":
        validate_worker_approval(value, errors)

    emit_result(args, errors, output_file=output_file)
    return 1 if errors else 0


if __name__ == "__main__":
    raise SystemExit(main())
