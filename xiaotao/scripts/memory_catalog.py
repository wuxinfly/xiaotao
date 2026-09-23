#!/usr/bin/env python3
"""Build, check, search, selectively read, and explicitly migrate XiaoTao Memory."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import re
import shutil
import sys
import tempfile
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Iterable

from validate import (
    Diagnostic,
    FileReferenceValidator,
    validate_memory_followup,
    validate_memory_index,
)

if hasattr(sys.stdout, "reconfigure"):
    try:
        sys.stdout.reconfigure(encoding="utf-8")
        sys.stderr.reconfigure(encoding="utf-8")
    except Exception:
        pass


INDEX_PATH = Path(".xiaotao/memory/index.json")
MANIFEST_PATH = Path(".xiaotao/memory/manifest.md")
LONG_TERM_ROOT = Path(".xiaotao/memory/long-term")
LONG_TERM_PATH = LONG_TERM_ROOT / "current.md"
LONG_TERM_ENTRIES_PATH = LONG_TERM_ROOT / "entries"
LONG_TERM_HISTORY_PATH = LONG_TERM_ROOT / "history"
LONG_TERM_MIGRATIONS_PATH = LONG_TERM_ROOT / "migrations"
LONG_TERM_MIGRATION_LOCK = Path(".xiaotao/locks/memory-long-term-migration.lock")
FOLLOWUPS_ROOT = Path(".xiaotao/memory/followups")
FOLLOWUPS_PENDING_PATH = FOLLOWUPS_ROOT / "pending"
FOLLOWUPS_RESOLVED_PATH = FOLLOWUPS_ROOT / "resolved"
CONFIG_PATH = Path(".xiaotao/config.yaml")
DEFAULT_TEMPORARY_STALE_DAYS = 7
CURRENT_POINTER_BODY = """# Long-term Memory

长期记忆的权威内容按 entry 存放在 `entries/`；已取代或拒绝的历史快照存放在 `history/`。
请通过 `memory/manifest.md` 和 `memory/index.json` 渐进检索，不要在此文件聚合全部条目。
"""
LONG_TERM_FENCE = re.compile(
    r"^```xiaotao-memory-entry[ \t]*\r?\n(.*?)^```[ \t]*$",
    re.MULTILINE | re.DOTALL,
)
HEADING = re.compile(r"^(#{1,6})[ \t]+(.+?)[ \t]*$", re.MULTILINE)
LATIN_TOKEN = re.compile(r"[a-z0-9][a-z0-9._-]*", re.IGNORECASE)
CJK_RUN = re.compile(r"[\u3400-\u4dbf\u4e00-\u9fff]+")
STABLE_ID = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]*$")
ACTIVE_STATUSES = {"active"}
CURRENT_TEMPORAL_STATES = {"current", "timeless"}
LONG_TERM_KINDS = {"fact", "experience", "principle", "decision", "constraint", "other"}
CURRENT_ENTRY_STATUSES = {"active", "disputed"}
HISTORY_ENTRY_STATUSES = {"superseded", "rejected"}


class CatalogError(ValueError):
    pass


def resolve_reference_time(arg_time: str | None = None) -> datetime:
    if arg_time:
        try:
            dt = datetime.fromisoformat(arg_time.replace("Z", "+00:00"))
        except ValueError as error:
            raise CatalogError(f"invalid --now timestamp '{arg_time}': {error}") from error
        if dt.tzinfo is None or dt.utcoffset() is None:
            raise CatalogError(
                f"--now timestamp must include timezone offset (e.g. 'Z' or '+00:00'): '{arg_time}'"
            )
        return dt
    env_time = os.environ.get("XIAOTAO_CURRENT_TIME")
    if env_time:
        try:
            dt = datetime.fromisoformat(env_time.replace("Z", "+00:00"))
        except ValueError as error:
            raise CatalogError(f"invalid XIAOTAO_CURRENT_TIME '{env_time}': {error}") from error
        if dt.tzinfo is None or dt.utcoffset() is None:
            raise CatalogError(
                f"XIAOTAO_CURRENT_TIME must include timezone offset (e.g. 'Z' or '+00:00'): '{env_time}'"
            )
        return dt
    return datetime.now(timezone.utc)


def utc_now(reference_time: datetime | None = None) -> str:
    current = reference_time if reference_time is not None else datetime.now(timezone.utc)
    return current.isoformat(timespec="seconds").replace("+00:00", "Z")


def project_relative(project_root: Path, path: Path) -> str:
    return path.resolve().relative_to(project_root.resolve()).as_posix()


def unique_strings(values: Iterable[Any]) -> list[str]:
    result: list[str] = []
    seen: set[str] = set()
    for value in values:
        if not isinstance(value, str):
            continue
        normalized = " ".join(value.split()).strip()
        if not normalized or normalized in seen:
            continue
        seen.add(normalized)
        result.append(normalized)
    return result


def compact_text(value: str, limit: int = 240) -> str:
    compact = " ".join(value.split()).strip()
    if len(compact) <= limit:
        return compact
    return compact[: limit - 1].rstrip() + "…"


def parse_scalar(raw: str) -> Any:
    value = raw.strip()
    if not value:
        return ""
    if value in {"null", "~"}:
        return None
    if value in {"true", "false"}:
        return value == "true"
    if re.fullmatch(r"-?\d+", value):
        return int(value)
    if value.startswith("[") or value.startswith("{") or value.startswith('"'):
        try:
            return json.loads(value)
        except json.JSONDecodeError:
            pass
    if len(value) >= 2 and value[0] == value[-1] == "'":
        return value[1:-1].replace("''", "'")
    return value


def parse_simple_yaml(path: Path) -> dict[str, Any]:
    try:
        lines = path.read_text(encoding="utf-8").splitlines()
    except (OSError, UnicodeError) as error:
        raise CatalogError(f"cannot read {path}: {error}") from error
    result: dict[str, Any] = {}
    current_list: str | None = None
    current_dict: str | None = None
    for line_number, line in enumerate(lines, start=1):
        if not line.strip() or line.lstrip().startswith("#"):
            continue
        list_match = re.fullmatch(r"\s+-\s+(.+?)\s*", line)
        if list_match and current_list is not None:
            if not isinstance(result[current_list], list):
                result[current_list] = []
            result[current_list].append(parse_scalar(list_match.group(1)))
            continue
        dict_match = re.fullmatch(r"\s{2,}([A-Za-z_][A-Za-z0-9_-]*):\s*(.*?)\s*", line)
        if dict_match and current_dict is not None:
            d_key, d_raw = dict_match.groups()
            if not isinstance(result[current_dict], dict):
                result[current_dict] = {}
            result[current_dict][d_key] = parse_scalar(d_raw)
            continue
        if line[0].isspace():
            continue
        match = re.fullmatch(r"([A-Za-z_][A-Za-z0-9_-]*):\s*(.*?)\s*", line)
        if match is None:
            raise CatalogError(f"{path}:{line_number}: unsupported top-level YAML")
        key, raw_value = match.groups()
        if not raw_value:
            result[key] = []
            current_list = key
            current_dict = key
        else:
            result[key] = parse_scalar(raw_value)
            current_list = None
            current_dict = None
    return result


def get_temporary_stale_days(project_root: Path) -> int:
    config_path = project_root / CONFIG_PATH
    if not config_path.is_file():
        return DEFAULT_TEMPORARY_STALE_DAYS
    try:
        data = parse_simple_yaml(config_path)
        val = data.get("temporary_stale_days")
        if isinstance(val, int) and val > 0:
            return val
        memory_section = data.get("memory")
        if isinstance(memory_section, dict):
            val = memory_section.get("temporary_stale_days")
            if isinstance(val, int) and val > 0:
                return val
    except Exception:
        pass
    return DEFAULT_TEMPORARY_STALE_DAYS


BUILTIN_SYNONYMS: list[set[str]] = [
    # 认证鉴权 / 权限 / 登录
    {"auth", "authentication", "authorization", "oauth", "jwt", "token", "login", "sso", "认证", "鉴权", "权限", "登录"},
    # 数据库 / SQL
    {"db", "database", "sql", "orm", "mysql", "postgres", "postgresql", "sqlite", "数据库"},
    # 缓存
    {"cache", "caching", "redis", "memcached", "缓存"},
    # 存储与持久化 / 落盘
    {"storage", "persist", "persistence", "存储", "持久化", "落盘"},
    # 性能与优化 / 延迟 / 耗时 / 瓶颈
    {"perf", "performance", "optimization", "latency", "throughput", "bottleneck", "lcp", "fcp", "性能", "优化", "延迟", "耗时", "吞吐", "瓶颈"},
    # 部署与发布 / 流水线
    {"deploy", "deployment", "release", "ci", "cd", "pipeline", "publish", "部署", "发布", "上线", "流水线"},
    # 构建与打包
    {"build", "bundle", "compile", "package", "构建", "打包", "编译"},
    # 搜索与检索 / 查询 / 索引 / 编目
    {"search", "query", "find", "retrieve", "retrieval", "index", "catalog", "搜索", "检索", "查询", "索引", "编目"},
    # 架构与设计 / 模式 / 结构 / 模块 / 组件
    {"arch", "architecture", "design", "pattern", "structure", "module", "component", "架构", "设计", "模式", "结构", "模块", "组件"},
    # 调度与派工 / 委派 / 执行者 / 代理 / 子代理
    {"dispatch", "schedule", "delegate", "delegation", "worker", "agent", "subagent", "调度", "派工", "委派", "执行者", "代理", "子代理"},
    # 测试与验证 / 校验 / 核对 / 断言
    {"test", "testing", "verify", "verification", "check", "assert", "assertion", "测试", "验证", "校验", "核对", "断言"},
    # 接口与协议 / 契约 / 规范
    {"api", "interface", "protocol", "contract", "schema", "spec", "specification", "接口", "协议", "契约", "规范"},
    # 错误与异常 / 故障 / 失败 / 排查 / 调试
    {"error", "exception", "bug", "fault", "failure", "debug", "troubleshoot", "错误", "异常", "故障", "失败", "排查", "调试"},
    # 生命周期与钩子 / 事件 / 触发 / 启动
    {"lifecycle", "hook", "event", "trigger", "startup", "shutdown", "生命周期", "钩子", "事件", "触发", "启动"},
    # 配置与环境 / 设置
    {"config", "configuration", "setting", "env", "environment", "profile", "配置", "设置", "环境"},
]


def load_synonym_groups(project_root: Path | None = None) -> list[set[str]]:
    groups = [set(g) for g in BUILTIN_SYNONYMS]
    if project_root:
        config_path = project_root / CONFIG_PATH
        if config_path.is_file():
            try:
                parsed = parse_simple_yaml(config_path)
                custom = parsed.get("synonyms")
                if isinstance(custom, list):
                    for item in custom:
                        if isinstance(item, str):
                            words = {w.strip().casefold() for w in item.split(",") if w.strip()}
                            if len(words) > 1:
                                groups.append(words)
                        elif isinstance(item, list):
                            words = {str(w).strip().casefold() for w in item if str(w).strip()}
                            if len(words) > 1:
                                groups.append(words)
            except Exception:
                pass
    return groups


def expand_synonyms(tokens: set[str], synonym_groups: list[set[str]]) -> set[str]:
    expanded: set[str] = set()
    for token in tokens:
        for group in synonym_groups:
            if token in group:
                expanded.update(group - {token})
    return expanded


def strip_front_matter(text: str) -> str:
    lines = text.splitlines()
    if not lines or lines[0].strip() != "---":
        return text
    for index, line in enumerate(lines[1:], start=1):
        if line.strip() in {"---", "..."}:
            return "\n".join(lines[index + 1 :])
    return text


def split_front_matter(path: Path, text: str) -> tuple[dict[str, Any], str]:
    lines = text.splitlines()
    if not lines or lines[0].strip() != "---":
        return {}, text
    closing = next(
        (index for index, line in enumerate(lines[1:], start=1) if line.strip() in {"---", "..."}),
        None,
    )
    if closing is None:
        raise CatalogError(f"{path}: unterminated YAML front matter")
    metadata: dict[str, Any] = {}
    for line_number, line in enumerate(lines[1:closing], start=2):
        if not line.strip() or line.lstrip().startswith("#"):
            continue
        match = re.fullmatch(r"([A-Za-z_][A-Za-z0-9_-]*):\s*(.*?)\s*", line)
        if match is None:
            raise CatalogError(f"{path}:{line_number}: unsupported front matter")
        key, raw_value = match.groups()
        if key in metadata:
            raise CatalogError(f"{path}:{line_number}: duplicate front matter field '{key}'")
        metadata[key] = parse_scalar(raw_value)
    return metadata, "\n".join(lines[closing + 1 :])


def long_term_metadata(path: Path, text: str, *, required: bool) -> dict[str, Any]:
    metadata, _ = split_front_matter(path, text)
    if not required:
        return metadata
    revision = metadata.get("revision")
    if isinstance(revision, bool) or not isinstance(revision, int) or revision < 0:
        raise CatalogError(f"{path}: 'revision' must be a non-negative integer")
    for key in ("updated_at", "updated_by"):
        require_string(metadata, key, path)
    return metadata


def markdown_sections(text: str) -> dict[str, str]:
    body = strip_front_matter(text)
    matches = list(HEADING.finditer(body))
    sections: dict[str, str] = {}
    for index, match in enumerate(matches):
        start = match.end()
        end = len(body)
        current_level = len(match.group(1))
        for later in matches[index + 1 :]:
            if len(later.group(1)) <= current_level:
                end = later.start()
                break
        name = match.group(2).strip().casefold()
        sections[name] = body[start:end].strip()
    return sections


def section_values(text: str, names: Iterable[str]) -> list[str]:
    sections = markdown_sections(text)
    result: list[str] = []
    for name in names:
        value = sections.get(name.casefold(), "")
        if value:
            result.append(compact_text(value, 400))
    return unique_strings(result)


def read_optional(path: Path) -> str:
    if not path.is_file():
        return ""
    try:
        return path.read_text(encoding="utf-8")
    except (OSError, UnicodeError) as error:
        raise CatalogError(f"cannot read {path}: {error}") from error


def require_string(mapping: dict[str, Any], key: str, source: Path) -> str:
    value = mapping.get(key)
    if not isinstance(value, str) or not value.strip():
        raise CatalogError(f"{source}: '{key}' must be a non-empty string")
    return value.strip()


def optional_string_list(mapping: dict[str, Any], key: str, source: Path) -> list[str]:
    value = mapping.get(key, [])
    if value is None:
        return []
    if not isinstance(value, list) or any(not isinstance(item, str) for item in value):
        raise CatalogError(f"{source}: '{key}' must be a string array")
    return unique_strings(value)


def require_string_list(mapping: dict[str, Any], key: str, source: Path) -> list[str]:
    values = optional_string_list(mapping, key, source)
    if not values:
        raise CatalogError(f"{source}: '{key}' must contain at least one string")
    return values


def validate_decision_context(entry: dict[str, Any], source: Path) -> None:
    context = entry.get("decision_context")
    if context is None:
        return
    if entry.get("memory_kind") != "decision":
        raise CatalogError(
            f"{source}: 'decision_context' is allowed only for memory_kind 'decision'"
        )
    if not isinstance(context, dict):
        raise CatalogError(f"{source}: 'decision_context' must be an object")
    unknown = set(context) - {"reason", "rejected_alternatives"}
    if unknown:
        raise CatalogError(
            f"{source}: 'decision_context' contains unknown fields: {sorted(unknown)}"
        )
    require_string(context, "reason", source)
    alternatives = context.get("rejected_alternatives", [])
    if not isinstance(alternatives, list):
        raise CatalogError(
            f"{source}: 'decision_context.rejected_alternatives' must be an array"
        )
    for alternative in alternatives:
        if not isinstance(alternative, dict):
            raise CatalogError(
                f"{source}: each rejected alternative must be an object"
            )
        if set(alternative) != {"alternative", "reason"}:
            raise CatalogError(
                f"{source}: each rejected alternative requires only 'alternative' and 'reason'"
            )
        require_string(alternative, "alternative", source)
        require_string(alternative, "reason", source)


def parse_entry_time(entry: dict[str, Any], key: str, source: Path) -> datetime | None:
    value = entry.get(key)
    if value is None:
        return None
    if not isinstance(value, str) or not value.strip():
        raise CatalogError(f"{source}: '{key}' must be a non-empty RFC 3339 timestamp")
    parsed = parse_iso_timestamp(value)
    if parsed is None:
        raise CatalogError(f"{source}: '{key}' must be an RFC 3339 timestamp with timezone")
    return parsed


def validate_temporal_fields(entry: dict[str, Any], source: Path) -> None:
    present = {key for key in ("valid_from", "valid_until") if key in entry}
    if present and entry.get("memory_kind") not in {"fact", "constraint"}:
        names = ", ".join(sorted(present))
        raise CatalogError(
            f"{source}: {names} are allowed only for memory_kind 'fact' or 'constraint'"
        )
    valid_from = parse_entry_time(entry, "valid_from", source)
    valid_until = parse_entry_time(entry, "valid_until", source)
    if valid_from is not None and valid_until is not None and valid_from >= valid_until:
        raise CatalogError(f"{source}: 'valid_from' must be earlier than 'valid_until'")


def temporal_state(entry: dict[str, Any], now: datetime) -> str:
    valid_from = parse_entry_time(entry, "valid_from", Path(entry.get("entry_id", "entry")))
    valid_until = parse_entry_time(entry, "valid_until", Path(entry.get("entry_id", "entry")))
    if valid_from is None and valid_until is None:
        return "timeless"
    current = now.astimezone(timezone.utc)
    if valid_from is not None and current < valid_from:
        return "not-yet-valid"
    if valid_until is not None and current >= valid_until:
        return "expired"
    return "current"


def is_current_knowledge(entry: dict[str, Any]) -> bool:
    return entry.get("temporal_state", "timeless") in CURRENT_TEMPORAL_STATES


def parse_long_term_entries(
    path: Path,
    *,
    allow_pointer: bool = False,
    require_single: bool = False,
) -> list[dict[str, Any]]:
    if not path.is_file():
        return []
    text = read_optional(path)
    blocks = list(LONG_TERM_FENCE.finditer(text))
    if not blocks:
        meaningful = strip_front_matter(text).strip()
        if allow_pointer and meaningful == CURRENT_POINTER_BODY.strip():
            return []
        if meaningful and meaningful not in {"# Long-term Memory", "# Long-term memory"}:
            raise CatalogError(
                f"{path}: Long-term entries must use fenced 'xiaotao-memory-entry' JSON blocks"
            )
        return []
    if require_single and len(blocks) != 1:
        raise CatalogError(f"{path}: an entry file must contain exactly one xiaotao-memory-entry block")
    entries: list[dict[str, Any]] = []
    seen: set[str] = set()
    for index, block in enumerate(blocks, start=1):
        try:
            entry = json.loads(block.group(1))
        except json.JSONDecodeError as error:
            raise CatalogError(
                f"{path}: entry block {index} contains invalid JSON: {error.msg}"
            ) from error
        if not isinstance(entry, dict):
            raise CatalogError(f"{path}: entry block {index} must contain a JSON object")
        entry_id = require_string(entry, "entry_id", path)
        if not STABLE_ID.fullmatch(entry_id):
            raise CatalogError(f"{path}: invalid entry_id '{entry_id}'")
        if entry_id in seen:
            raise CatalogError(f"{path}: duplicate entry_id '{entry_id}'")
        seen.add(entry_id)
        require_string(entry, "title", path)
        kind = require_string(entry, "memory_kind", path)
        if kind not in LONG_TERM_KINDS:
            raise CatalogError(f"{path}: invalid memory_kind '{kind}'")
        require_string(entry, "content", path)
        require_string_list(entry, "source_refs", path)
        status = entry.get("status", "active")
        if status not in {"active", "disputed", "superseded", "rejected"}:
            raise CatalogError(f"{path}: invalid status '{status}'")
        optional_string_list(entry, "tags", path)
        optional_string_list(entry, "aliases", path)
        optional_string_list(entry, "search_hints", path)
        validate_decision_context(entry, path)
        validate_temporal_fields(entry, path)
        entries.append(entry)
    return entries


def validate_entry_sources(project_root: Path, entry: dict[str, Any], source: Path) -> None:
    validator = FileReferenceValidator(project_root)
    errors: list[Diagnostic] = []
    for index, reference in enumerate(require_string_list(entry, "source_refs", source)):
        validator(reference, f"source_refs[{index}]", errors)
    if errors:
        details = "; ".join(f"{error.path}: {error.message}" for error in errors)
        raise CatalogError(f"{source}: invalid source_refs: {details}")


def split_entry_file(
    project_root: Path,
    path: Path,
    *,
    expected_statuses: set[str],
) -> tuple[dict[str, Any], str | None]:
    text = read_optional(path)
    metadata = long_term_metadata(path, text, required=True)
    entries = parse_long_term_entries(path, require_single=True)
    if len(entries) != 1:
        raise CatalogError(f"{path}: an entry file must contain exactly one xiaotao-memory-entry block")
    entry = entries[0]
    entry_id = require_string(entry, "entry_id", path)
    if path.name != f"{entry_id}.md":
        raise CatalogError(f"{path}: filename must match entry_id '{entry_id}.md'")
    status = entry.get("status", "active")
    if status not in expected_statuses:
        expected = ", ".join(sorted(expected_statuses))
        raise CatalogError(f"{path}: status '{status}' does not belong here; expected {expected}")
    validate_entry_sources(project_root, entry, path)
    updated_at = metadata.get("updated_at")
    return entry, updated_at if isinstance(updated_at, str) else None


def split_long_term_records(
    project_root: Path,
    source_files: set[Path],
) -> list[tuple[dict[str, Any], Path, str | None]]:
    records: list[tuple[dict[str, Any], Path, str | None]] = []
    for relative_root, expected_statuses in (
        (LONG_TERM_ENTRIES_PATH, CURRENT_ENTRY_STATUSES),
        (LONG_TERM_HISTORY_PATH, HISTORY_ENTRY_STATUSES),
    ):
        root = project_root / relative_root
        if root.exists() and not root.is_dir():
            raise CatalogError(f"{root}: Long-term entry path must be a directory")
        if not root.is_dir():
            continue
        for path in sorted(root.iterdir()):
            if path.name.startswith("."):
                continue
            if not path.is_file() or path.suffix != ".md":
                raise CatalogError(f"{path}: Long-term entry directories may contain only Markdown files")
            source_files.add(path)
            entry, updated_at = split_entry_file(
                project_root,
                path,
                expected_statuses=expected_statuses,
            )
            records.append((entry, path, updated_at))
    return records


def validate_unique_long_term_records(
    records: list[tuple[dict[str, Any], Path, str | None]],
) -> None:
    seen: dict[str, Path] = {}
    for entry, path, _ in records:
        entry_id = require_string(entry, "entry_id", path)
        previous = seen.get(entry_id)
        if previous is not None:
            raise CatalogError(
                f"duplicate Long-term entry_id '{entry_id}' in {previous} and {path}"
            )
        seen[entry_id] = path


def long_term_records(
    project_root: Path,
    source_files: set[Path],
) -> list[tuple[dict[str, Any], Path, str | None]]:
    records: list[tuple[dict[str, Any], Path, str | None]] = []
    legacy_path = project_root / LONG_TERM_PATH
    if legacy_path.is_file():
        source_files.add(legacy_path)
        text = read_optional(legacy_path)
        metadata = long_term_metadata(legacy_path, text, required=False)
        updated_at = metadata.get("updated_at")
        for entry in parse_long_term_entries(legacy_path, allow_pointer=True):
            validate_entry_sources(project_root, entry, legacy_path)
            records.append((entry, legacy_path, updated_at if isinstance(updated_at, str) else None))

    records.extend(split_long_term_records(project_root, source_files))
    validate_unique_long_term_records(records)
    return records


def long_term_index_entries(
    project_root: Path,
    source_files: set[Path],
    *,
    now: datetime | None = None,
) -> list[dict[str, Any]]:
    result: list[dict[str, Any]] = []
    reference_time = now if now is not None else datetime.now(timezone.utc)
    for entry, path, updated_at in long_term_records(project_root, source_files):
        content = require_string(entry, "content", path)
        result.append(
            {
                "memory_id": require_string(entry, "entry_id", path),
                "layer": "long-term",
                "record_type": "long-term-entry",
                "title": require_string(entry, "title", path),
                "summary": compact_text(content),
                "path": project_relative(project_root, path),
                "locator": require_string(entry, "entry_id", path),
                "status": entry.get("status", "active"),
                "memory_kind": require_string(entry, "memory_kind", path),
                "valid_from": entry.get("valid_from"),
                "valid_until": entry.get("valid_until"),
                "temporal_state": temporal_state(entry, reference_time),
                "tags": optional_string_list(entry, "tags", path),
                "aliases": optional_string_list(entry, "aliases", path),
                "search_hints": optional_string_list(entry, "search_hints", path),
                "updated_at": updated_at,
                "stale": None,
            }
        )
    return result


def temporary_index_entries(
    project_root: Path,
    source_files: set[Path],
    *,
    now: datetime | None = None,
) -> list[dict[str, Any]]:
    active_root = project_root / ".xiaotao/memory/temporary/active"
    if not active_root.is_dir():
        return []
    result: list[dict[str, Any]] = []
    stale_threshold = get_temporary_stale_days(project_root)
    now_dt = now if now is not None else datetime.now(timezone.utc)
    for directory in sorted(path for path in active_root.iterdir() if path.is_dir()):
        meta_path = directory / "meta.yaml"
        if not meta_path.is_file():
            raise CatalogError(f"{directory}: active Temporary is missing meta.yaml")
        meta = parse_simple_yaml(meta_path)
        temporary_id = require_string(meta, "id", meta_path)
        if temporary_id != directory.name:
            raise CatalogError(f"{meta_path}: id must match directory name '{directory.name}'")
        if require_string(meta, "status", meta_path) != "active":
            raise CatalogError(f"{meta_path}: active Temporary must have status 'active'")
        topic = require_string(meta, "topic", meta_path)
        aliases = optional_string_list(meta, "aliases", meta_path)
        current_path = directory / "current.md"
        current = read_optional(current_path)
        goals = section_values(current, ("Current goal", "Goal"))
        hints = section_values(current, ("Confirmed", "Open questions"))
        detail_path = current_path if current_path.is_file() else meta_path
        source_files.add(meta_path)
        if current_path.is_file():
            source_files.add(current_path)
        updated_at = meta.get("updated_at") if isinstance(meta.get("updated_at"), str) else None
        is_stale = False
        if updated_at:
            try:
                updated_dt = datetime.fromisoformat(updated_at.replace("Z", "+00:00"))
                is_stale = (now_dt - updated_dt).total_seconds() >= stale_threshold * 86400
            except ValueError:
                is_stale = False
        result.append(
            {
                "memory_id": temporary_id,
                "layer": "temporary",
                "record_type": "temporary",
                "title": topic,
                "summary": goals[0] if goals else topic,
                "path": project_relative(project_root, detail_path),
                "locator": "routing-context",
                "status": "active",
                "memory_kind": None,
                "tags": [],
                "aliases": aliases,
                "search_hints": hints,
                "updated_at": updated_at,
                "stale": is_stale,
            }
        )
    return result


def current_state_entries(
    project_root: Path,
    task_directory: Path,
    task_id: str,
    task_title: str,
    source_files: set[Path],
) -> list[dict[str, Any]]:
    result: list[dict[str, Any]] = []
    record_type = "worker-state"
    root = task_directory / "workers"
    if root.is_dir():
        for unit in sorted(path for path in root.iterdir() if path.is_dir()):
            state_path = unit / "current-state.md"
            if not state_path.is_file():
                continue
            state = read_optional(state_path)
            objective = section_values(state, ("Objective", "Current goal"))
            findings = section_values(
                state,
                ("Key findings", "Work done", "Open items", "Recommended next"),
            )
            summary = objective[0] if objective else (
                findings[0] if findings else f"{unit.name} current state"
            )
            source_files.add(state_path)
            result.append(
                {
                    "memory_id": f"{task_id}.{record_type}.{unit.name}",
                    "layer": "task",
                    "record_type": record_type,
                    "title": f"{task_title} / {unit.name}",
                    "summary": summary,
                    "path": project_relative(project_root, state_path),
                    "locator": "current-state",
                    "status": "active",
                    "memory_kind": None,
                    "tags": [],
                    "aliases": [],
                    "search_hints": findings,
                    "stale": None,
                    "updated_at": None,
                }
            )
    return result


def task_index_entries(project_root: Path, source_files: set[Path]) -> list[dict[str, Any]]:
    tasks_root = project_root / ".xiaotao/tasks"
    if not tasks_root.is_dir():
        return []
    result: list[dict[str, Any]] = []
    directories = sorted(
        path
        for path in tasks_root.iterdir()
        if path.is_dir() and path.name != "archive"
    )
    for directory in directories:
        task_path = directory / "task.yaml"
        if not task_path.is_file():
            continue
        task = parse_simple_yaml(task_path)
        if task.get("status") != "active":
            continue
        task_id = require_string(task, "id", task_path)
        if task_id != directory.name:
            raise CatalogError(f"{task_path}: id must match directory name '{directory.name}'")
        objective = require_string(task, "objective", task_path)
        context_path = directory / "context.md"
        context = read_optional(context_path)
        hints = section_values(
            context,
            ("Confirmed", "Open questions", "Open items", "Current state"),
        )
        detail_path = context_path if context_path.is_file() else task_path
        source_files.add(task_path)
        if context_path.is_file():
            source_files.add(context_path)
        updated_at = task.get("updated_at") if isinstance(task.get("updated_at"), str) else None
        result.append(
            {
                "memory_id": task_id,
                "layer": "task",
                "record_type": "task",
                "title": objective,
                "summary": objective,
                "path": project_relative(project_root, detail_path),
                "locator": "task-context",
                "status": "active",
                "memory_kind": None,
                "tags": [],
                "aliases": [],
                "search_hints": hints,
                "stale": None,
                "updated_at": updated_at,
            }
        )
        result.extend(
            current_state_entries(
                project_root,
                directory,
                task_id,
                objective,
                source_files,
            )
        )
    return result


def digest_sources(project_root: Path, source_files: Iterable[Path]) -> str:
    digest = hashlib.sha256()
    for path in sorted(set(source_files), key=lambda item: project_relative(project_root, item)):
        relative = project_relative(project_root, path)
        digest.update(relative.encode("utf-8"))
        digest.update(b"\0")
        try:
            digest.update(path.read_bytes())
        except OSError as error:
            raise CatalogError(f"cannot hash {path}: {error}") from error
        digest.update(b"\0")
    return digest.hexdigest()


def validate_index(index: dict[str, Any], project_root: Path) -> None:
    errors: list[Diagnostic] = []
    validate_memory_index(index, errors, FileReferenceValidator(project_root))
    if errors:
        details = "; ".join(f"{error.path}: {error.message}" for error in errors)
        raise CatalogError(f"generated Memory Index is invalid: {details}")


def followup_records(
    project_root: Path,
    source_files: set[Path],
) -> tuple[list[dict[str, Any]], dict[str, dict[str, Any]]]:
    followups_root = project_root / FOLLOWUPS_ROOT
    if followups_root.exists() and not followups_root.is_dir():
        raise CatalogError(f"{followups_root}: Follow-ups path must be a directory")
    pending_followups: list[dict[str, Any]] = []
    all_followups: dict[str, dict[str, Any]] = {}
    validator = FileReferenceValidator(project_root)

    for directory, expected_status in (
        (project_root / FOLLOWUPS_PENDING_PATH, "pending"),
        (project_root / FOLLOWUPS_RESOLVED_PATH, "resolved"),
    ):
        if directory.exists() and not directory.is_dir():
            raise CatalogError(f"{directory}: Follow-up {expected_status} path must be a directory")
        if not directory.is_dir():
            continue
        for path in sorted(directory.iterdir()):
            if path.name.startswith("."):
                continue
            if not path.is_file() or path.suffix not in {".yaml", ".yml"}:
                raise CatalogError(f"{path}: Follow-up directories may contain only YAML files")
            source_files.add(path)
            record = parse_simple_yaml(path)
            errors: list[Diagnostic] = []
            validate_memory_followup(record, errors, validator)
            if errors:
                details = "; ".join(f"{err.path}: {err.message}" for err in errors)
                raise CatalogError(f"{path}: invalid follow-up: {details}")

            fid = record["followup_id"]
            if path.name not in {f"{fid}.yaml", f"{fid}.yml"}:
                raise CatalogError(f"{path}: filename must match followup_id '{fid}.yaml'")
            status = record["status"]
            if status != expected_status:
                raise CatalogError(
                    f"{path}: status must be '{expected_status}' in {expected_status} directory"
                )

            if fid in all_followups:
                raise CatalogError(f"duplicate followup_id '{fid}' across follow-up records")

            record_copy = dict(record)
            record_copy["path"] = project_relative(project_root, path)
            record_copy["related_ids"] = record.get("related_ids", [])
            if status == "resolved":
                record_copy["resolution_refs"] = record.get("resolution_refs", [])
            all_followups[fid] = record_copy
            if status == "pending":
                pending_followups.append(
                    {
                        "followup_id": fid,
                        "title": record["title"],
                        "status": "pending",
                        "created_at": record["created_at"],
                        "source_refs": record["source_refs"],
                        "related_ids": record.get("related_ids", []),
                        "path": project_relative(project_root, path),
                    }
                )

    pending_followups.sort(key=lambda item: item["followup_id"])
    return pending_followups, all_followups


def derive_catalog(project_root: Path, *, now: datetime | None = None) -> dict[str, Any]:
    source_files: set[Path] = set()
    config_path = project_root / CONFIG_PATH
    if config_path.is_file():
        source_files.add(config_path)
    entries = []
    entries.extend(long_term_index_entries(project_root, source_files, now=now))
    entries.extend(temporary_index_entries(project_root, source_files, now=now))
    entries.extend(task_index_entries(project_root, source_files))
    entries.sort(key=lambda item: (item["layer"], item["record_type"], item["memory_id"]))
    seen: set[str] = set()
    for entry in entries:
        memory_id = entry["memory_id"]
        if memory_id in seen:
            raise CatalogError(f"duplicate memory_id across layers: '{memory_id}'")
        seen.add(memory_id)
    pending_followups, all_followups = followup_records(project_root, source_files)
    for fid in all_followups:
        if fid in seen:
            raise CatalogError(f"followup_id '{fid}' collides with memory_id")
    index = {
        "schema_version": 1,
        "generated_at": utc_now(now),
        "source_digest": digest_sources(project_root, source_files),
        "entries": entries,
        "pending_followups": pending_followups,
    }
    validate_index(index, project_root)
    return index


def manifest_text(index: dict[str, Any]) -> str:
    visible = [
        entry for entry in index["entries"]
        if entry["status"] == "active" and is_current_knowledge(entry)
    ]
    groups = (
        (
            "Active Temporary Memory",
            [entry for entry in visible if entry["record_type"] == "temporary"],
        ),
        ("Active Tasks", [entry for entry in visible if entry["record_type"] == "task"]),
        (
            "Long-term Memory",
            [entry for entry in visible if entry["record_type"] == "long-term-entry"],
        ),
    )
    lines = [
        "---",
        "schema_version: 1",
        f"generated_at: {index['generated_at']}",
        f"source_digest: {index['source_digest']}",
        "---",
        "",
        "# Memory Overview",
        "",
        "This file is generated. Formal Memory files remain authoritative.",
    ]
    for title, entries in groups:
        lines.extend(("", f"## {title}", ""))
        if not entries:
            lines.append("- None")
            continue
        for entry in entries:
            timing = ""
            if entry["record_type"] == "temporary":
                timing_parts: list[str] = []
                if entry.get("updated_at"):
                    date_prefix = str(entry["updated_at"])[:10]
                    timing_parts.append(f"updated {date_prefix}")
                if entry.get("stale"):
                    timing_parts.append("stale")
                if timing_parts:
                    timing = f" ({', '.join(timing_parts)})"
            lines.append(
                f"- **{entry['title']}** (`{entry['memory_id']}`){timing} — {entry['summary']}"
            )

    state_count = sum(entry["record_type"] == "worker-state" for entry in visible)
    lines.extend(
        (
            "",
            "## Indexed execution states",
            "",
            f"- {state_count} current Worker state(s)",
        )
    )

    pending_followups = index.get("pending_followups", [])
    lines.extend(("", "## Pending follow-ups", ""))
    if not pending_followups:
        lines.append("- None")
    else:
        for item in pending_followups:
            sources = ", ".join(item.get("source_refs", []))
            ref_info = f" — from {sources}" if sources else ""
            lines.append(f"- **{item['title']}** (`{item['followup_id']}`){ref_info}")
    lines.append("")
    return "\n".join(lines)


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
        if temporary_path.exists():
            temporary_path.unlink()


def persist_catalog(project_root: Path, index: dict[str, Any]) -> None:
    atomic_write(
        project_root / INDEX_PATH,
        json.dumps(index, ensure_ascii=False, indent=2, sort_keys=True) + "\n",
    )
    atomic_write(project_root / MANIFEST_PATH, manifest_text(index))


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
    try:
        validate_index(value, project_root)
    except CatalogError:
        return None
    return value


def entries_equal(left: dict[str, Any], right: dict[str, Any]) -> bool:
    return (
        left.get("source_digest") == right.get("source_digest")
        and left.get("entries") == right.get("entries")
        and left.get("pending_followups") == right.get("pending_followups")
    )


def manifest_is_current(project_root: Path, index: dict[str, Any]) -> bool:
    path = project_root / MANIFEST_PATH
    if not path.is_file():
        return False
    return read_optional(path) == manifest_text(index)


def ensure_current_index(
    project_root: Path,
    *,
    refresh: bool,
    now: datetime | None = None,
) -> tuple[dict[str, Any], bool]:
    expected = derive_catalog(project_root, now=now)
    current = load_index(project_root)
    stale = (
        current is None
        or not entries_equal(current, expected)
        or not manifest_is_current(project_root, current)
    )
    if stale:
        if not refresh:
            raise CatalogError("Memory catalog is missing or stale; run the build command")
        persist_catalog(project_root, expected)
        return expected, True
    return current, False


def search_tokens(value: str) -> set[str]:
    normalized = value.casefold()
    tokens = set(LATIN_TOKEN.findall(normalized))
    for run in CJK_RUN.findall(normalized):
        tokens.add(run)
        if len(run) > 1:
            tokens.update(run[index : index + 2] for index in range(len(run) - 1))
    return {token for token in tokens if token}


def field_score(
    query: str,
    query_tokens: set[str],
    synonym_tokens: set[str],
    values: Iterable[str],
    exact: int,
    token: int,
    synonym_weight: int = 0,
) -> tuple[int, bool, bool]:
    score = 0
    matched = False
    synonym_matched = False
    for value in values:
        normalized = value.casefold()
        if query and query in normalized:
            score += exact
            matched = True
        val_tokens = search_tokens(normalized)
        overlap = query_tokens & val_tokens
        if overlap:
            score += min(len(overlap), 5) * token
            matched = True
        elif synonym_tokens and synonym_weight > 0:
            syn_overlap = synonym_tokens & val_tokens
            if syn_overlap:
                score += min(len(syn_overlap), 3) * synonym_weight
                synonym_matched = True
    return score, matched, synonym_matched


def rank_entry(
    entry: dict[str, Any],
    query: str,
    contexts: list[str],
    binding: str | None,
    synonym_groups: list[set[str]] | None = None,
) -> tuple[int, list[str]]:
    normalized_query = " ".join((query, *contexts)).casefold().strip()
    tokens = search_tokens(normalized_query)
    syn_tokens = expand_synonyms(tokens, synonym_groups) if synonym_groups else set()
    score = 0
    reasons: list[str] = []
    if binding and entry["memory_id"] == binding:
        score += 100
        reasons.append("current binding")
    hint_label = "current state" if entry.get("record_type") in {"worker-state", "task"} else "search hint"
    for label, values, exact, token, syn_w in (
        ("title", [entry["title"]], 30, 8, 4),
        ("summary", [entry["summary"]], 20, 4, 2),
        ("tag", entry["tags"], 25, 7, 3),
        ("alias", entry["aliases"], 25, 7, 3),
        (hint_label, entry["search_hints"], 10, 2, 1),
    ):
        added, matched, syn_matched = field_score(
            normalized_query, tokens, syn_tokens, values, exact, token, syn_w
        )
        score += added
        if matched:
            reasons.append(label)
        elif syn_matched:
            reasons.append(f"{label} (synonym)")
    return score, reasons


def search_index(
    index: dict[str, Any],
    query: str,
    *,
    layer: str | None,
    memory_kind: str | None,
    contexts: list[str],
    binding: str | None,
    limit: int,
    include_inactive: bool = False,
    synonym_groups: list[set[str]] | None = None,
    project_root: Path | None = None,
) -> list[dict[str, Any]]:
    if synonym_groups is None:
        synonym_groups = load_synonym_groups(project_root)
    candidates: list[dict[str, Any]] = []
    for entry in index["entries"]:
        if not include_inactive and (
            entry["status"] not in ACTIVE_STATUSES or not is_current_knowledge(entry)
        ):
            continue
        if layer and entry["layer"] != layer:
            continue
        if memory_kind and entry["memory_kind"] != memory_kind:
            continue
        score, reasons = rank_entry(
            entry, query, contexts, binding, synonym_groups=synonym_groups
        )
        if score <= 0:
            continue
        candidate = dict(entry)
        candidate["score"] = score
        candidate["relevance_reason"] = ", ".join(reasons)
        candidate.pop("search_hints", None)
        candidates.append(candidate)
    candidates.sort(key=lambda item: (-item["score"], item["memory_id"]))
    return candidates[:limit]


def parse_iso_timestamp(raw: str | None) -> datetime | None:
    if not raw or not isinstance(raw, str):
        return None
    try:
        dt = datetime.fromisoformat(raw.replace("Z", "+00:00"))
    except (ValueError, TypeError, AttributeError):
        return None
    if dt.tzinfo is None or dt.utcoffset() is None:
        return None
    return dt.astimezone(timezone.utc)


def bound_recent_entry(entry: dict[str, Any]) -> dict[str, Any]:
    bounded: dict[str, Any] = {
        "memory_id": entry["memory_id"],
        "layer": entry["layer"],
        "record_type": entry["record_type"],
        "title": entry["title"],
        "summary": entry["summary"],
        "status": entry.get("status", "active"),
        "path": entry["path"],
        "locator": entry["locator"],
        "updated_at": entry.get("updated_at"),
    }
    if entry.get("memory_kind") is not None:
        bounded["memory_kind"] = entry["memory_kind"]
    for key in ("valid_from", "valid_until", "temporal_state"):
        if entry.get(key) is not None:
            bounded[key] = entry[key]
    if entry.get("stale") is not None:
        bounded["stale"] = entry["stale"]
    return bounded


def recent_entries(
    index: dict[str, Any],
    *,
    layer: str | None = None,
    include_inactive: bool = False,
    limit: int = 5,
) -> list[dict[str, Any]]:
    timed: list[dict[str, Any]] = []
    untimed: list[dict[str, Any]] = []
    for entry in index["entries"]:
        if not include_inactive and (
            entry.get("status") not in ACTIVE_STATUSES or not is_current_knowledge(entry)
        ):
            continue
        if layer and entry.get("layer") != layer:
            continue
        dt = parse_iso_timestamp(entry.get("updated_at"))
        if dt is not None:
            timed.append(entry)
        else:
            untimed.append(entry)

    # Sort timed: primary updated_at descending, tie-breaker memory_id ascending
    timed.sort(key=lambda item: item["memory_id"])
    timed.sort(key=lambda item: parse_iso_timestamp(item.get("updated_at")), reverse=True)

    # Sort untimed: degraded deterministic order, memory_id ascending
    untimed.sort(key=lambda item: item["memory_id"])

    ordered = timed + untimed
    return [bound_recent_entry(entry) for entry in ordered[:limit]]


def extract_checkpoint_receipt(text: str) -> dict[str, Any] | None:
    if "checkpoint_receipt" not in text:
        return None
    req_match = re.search(r"request_id:\s*['\"]?([a-z0-9][a-z0-9_-]*)['\"]?", text, re.IGNORECASE)
    rev_match = re.search(r"revision:\s*(\d+)", text)
    if req_match and rev_match:
        return {
            "request_id": req_match.group(1),
            "revision": int(rev_match.group(1)),
        }
    return None


def parse_checkpoint_file(path: Path) -> dict[str, Any] | None:
    try:
        data = json.loads(path.read_text(encoding="utf-8"))
        if isinstance(data, dict) and "request_id" in data:
            return data
    except Exception:
        pass
    return None


def find_recoverable_checkpoint(project_root: Path) -> dict[str, Any] | None:
    candidates: list[dict[str, Any]] = []

    def scan_checkpoints_for_target(
        scope: str,
        binding: str,
        chk_dir: Path,
        receipt: dict[str, Any] | None = None,
        receipt_mtime: float = 0.0,
    ) -> None:
        committed_map: dict[str, tuple[int, float]] = {}
        if receipt:
            committed_map[receipt["request_id"]] = (receipt["revision"], receipt_mtime)

        if chk_dir.is_dir():
            for f in chk_dir.iterdir():
                if not f.is_file():
                    continue
                if f.name.endswith(".committed.json"):
                    req_id = f.name[:-len(".committed.json")]
                    data = parse_checkpoint_file(f)
                    rev = data.get("revision") if data else None
                    mtime = f.stat().st_mtime
                    if req_id not in committed_map or mtime > committed_map[req_id][1]:
                        committed_map[req_id] = (rev if rev is not None else 1, mtime)

            for f in chk_dir.iterdir():
                if not f.is_file() or not f.name.endswith(".json"):
                    continue
                if ".committed." in f.name or ".failed-" in f.name:
                    continue
                rec = parse_checkpoint_file(f)
                if not rec:
                    continue
                req_id = rec.get("request_id", f.stem)
                target_binding = rec.get("target_id", binding)
                target_scope = rec.get("kind", scope)
                mtime = f.stat().st_mtime

                if req_id in committed_map:
                    rev, c_mtime = committed_map[req_id]
                    candidates.append({
                        "status": "committed",
                        "scope": target_scope,
                        "binding": target_binding,
                        "revision": rev,
                        "request_id": req_id,
                        "mtime": max(mtime, c_mtime),
                    })
                else:
                    prop_rev = rec.get("base_revision", 0) + 1
                    candidates.append({
                        "status": "pending",
                        "scope": target_scope,
                        "binding": target_binding,
                        "proposed_revision": prop_rev,
                        "revision": prop_rev,
                        "request_id": req_id,
                        "mtime": mtime,
                    })

        for req_id, (rev, c_mtime) in committed_map.items():
            if not any(c["request_id"] == req_id for c in candidates):
                candidates.append({
                    "status": "committed",
                    "scope": scope,
                    "binding": binding,
                    "revision": rev,
                    "request_id": req_id,
                    "mtime": c_mtime,
                })

    # 1. Tasks
    tasks_root = project_root / ".xiaotao/tasks"
    if tasks_root.is_dir():
        for task_dir in tasks_root.iterdir():
            if not task_dir.is_dir() or task_dir.name == "archive":
                continue
            progress_path = task_dir / "progress.md"
            receipt = None
            receipt_mtime = 0.0
            if progress_path.is_file():
                try:
                    receipt = extract_checkpoint_receipt(progress_path.read_text(encoding="utf-8"))
                    receipt_mtime = progress_path.stat().st_mtime
                except Exception:
                    pass
            chk_dir = task_dir / "references/checkpoints"
            scan_checkpoints_for_target("task", task_dir.name, chk_dir, receipt, receipt_mtime)

    # 2. Temporaries
    temp_root = project_root / ".xiaotao/memory/temporary/active"
    if temp_root.is_dir():
        for temp_dir in temp_root.iterdir():
            if not temp_dir.is_dir():
                continue
            current_path = temp_dir / "current.md"
            receipt = None
            receipt_mtime = 0.0
            if current_path.is_file():
                try:
                    receipt = extract_checkpoint_receipt(current_path.read_text(encoding="utf-8"))
                    receipt_mtime = current_path.stat().st_mtime
                except Exception:
                    pass
            chk_dir = temp_dir / "references/checkpoints"
            scan_checkpoints_for_target("temporary", temp_dir.name, chk_dir, receipt, receipt_mtime)

    # 3. Project/Session checkpoints
    proj_chk = project_root / ".xiaotao/checkpoints"
    scan_checkpoints_for_target("session", "session", proj_chk)

    if not candidates:
        return None

    # Deduplicate candidates with the same request_id, keeping highest status / newest mtime
    seen_req: set[str] = set()
    deduped: list[dict[str, Any]] = []
    # Sort first: committed first, mtime descending, revision descending
    candidates.sort(
        key=lambda c: (
            0 if c["status"] == "committed" else 1,
            -c["mtime"],
            -c.get("revision", 0),
            c["binding"],
            c["request_id"],
        )
    )
    for c in candidates:
        if c["request_id"] not in seen_req:
            seen_req.add(c["request_id"])
            deduped.append(c)

    if not deduped:
        return None

    top = deduped[0]
    return {
        "available": True,
        "status": top["status"],
        "scope": top["scope"],
        "binding": top["binding"],
        "revision": top.get("revision"),
        "proposed_revision": top.get("proposed_revision"),
        "request_id": top["request_id"],
    }


def catalog_overview_summary(
    project_root: Path,
    index: dict[str, Any],
    refreshed: bool,
    *,
    limit: int = 3,
    cached: bool = False,
) -> dict[str, Any]:
    visible = [
        entry for entry in index["entries"]
        if entry["status"] == "active" and is_current_knowledge(entry)
    ]
    all_temporaries = [
        {
            "memory_id": entry["memory_id"],
            "title": entry["title"],
            "summary": entry["summary"],
            "updated_at": entry.get("updated_at"),
            "stale": entry.get("stale"),
        }
        for entry in visible
        if entry["record_type"] == "temporary"
    ]
    all_tasks = [
        {
            "memory_id": entry["memory_id"],
            "title": entry["title"],
            "summary": entry["summary"],
            "updated_at": entry.get("updated_at"),
        }
        for entry in visible
        if entry["record_type"] == "task"
    ]
    long_term_entries = [entry for entry in visible if entry["record_type"] == "long-term-entry"]
    long_term_count = len(long_term_entries)
    long_term_samples = [
        {"memory_id": entry["memory_id"], "title": entry["title"]}
        for entry in long_term_entries[:limit]
    ]
    worker_state_count = sum(entry["record_type"] == "worker-state" for entry in visible)
    all_followups = [
        {
            "followup_id": item["followup_id"],
            "title": item["title"],
            "status": item["status"],
            "created_at": item.get("created_at"),
            "source_refs": item.get("source_refs", []),
        }
        for item in index.get("pending_followups", [])
    ]
    recoverable_checkpoint = find_recoverable_checkpoint(project_root)
    has_active_work = bool(all_temporaries or all_tasks or all_followups or recoverable_checkpoint)

    bounded_temporaries = all_temporaries[:limit]
    bounded_tasks = all_tasks[:limit]
    bounded_followups = all_followups[:limit]

    return {
        "catalog_refreshed": refreshed,
        "catalog_cached": cached,
        "has_active_work": has_active_work,
        "recoverable_checkpoint": recoverable_checkpoint,
        "active_temporary_count": len(all_temporaries),
        "active_temporaries": bounded_temporaries,
        "more_temporaries": max(0, len(all_temporaries) - limit),
        "active_task_count": len(all_tasks),
        "active_tasks": bounded_tasks,
        "more_tasks": max(0, len(all_tasks) - limit),
        "long_term_count": long_term_count,
        "long_term_samples": long_term_samples,
        "more_long_term": max(0, long_term_count - limit),
        "worker_state_count": worker_state_count,
        "pending_followup_count": len(all_followups),
        "pending_followups": bounded_followups,
        "more_followups": max(0, len(all_followups) - limit),
        "bounded_limit": limit,
    }


def bounded_runtime_context_text(summary: dict[str, Any]) -> str:
    lines = ["# Memory Overview (Runtime Context)", ""]
    if summary.get("catalog_cached"):
        lines.append("> 启动时使用现有 Catalog 快照；处理具体问题时请先用 search / show 按需确认最新状态。")
        lines.append("")
    if summary["has_active_work"]:
        lines.append("当前检测到项目存在活动工作：")
    else:
        lines.append("当前项目暂无活动任务或临时探索，系统处于就绪状态。")
    lines.append("")

    chk = summary.get("recoverable_checkpoint")
    if chk and chk.get("available"):
        lines.append("## Recoverable Checkpoint")
        lines.append("Recoverable checkpoint: yes")
        status = chk.get("status", "committed")
        lines.append(f"status: {status}")
        lines.append(f"scope: {chk.get('scope')}")
        lines.append(f"binding: {chk.get('binding')}")
        if status == "pending":
            lines.append(f"proposed_revision: {chk.get('proposed_revision') or chk.get('revision')}")
        else:
            lines.append(f"revision: {chk.get('revision')}")
        lines.append("")

    t_count = summary["active_task_count"]
    lines.append(f"## Active Tasks ({t_count})")
    if summary["active_tasks"]:
        for t in summary["active_tasks"]:
            lines.append(f"- `{t['memory_id']}`: {t['title']}")
        if summary.get("more_tasks", 0) > 0:
            lines.append(f"  *(另外 {summary['more_tasks']} 项活动任务已省略，详情请使用 recent / show)*")
    else:
        lines.append("- *(无活动任务)*")
    lines.append("")

    temp_count = summary["active_temporary_count"]
    lines.append(f"## Active Temporary Memory ({temp_count})")
    if summary["active_temporaries"]:
        for t in summary["active_temporaries"]:
            lines.append(f"- `{t['memory_id']}`: {t['title']}")
        if summary.get("more_temporaries", 0) > 0:
            lines.append(f"  *(另外 {summary['more_temporaries']} 项活动探索已省略，详情请使用 recent / show)*")
    else:
        lines.append("- *(无活动探索)*")
    lines.append("")

    f_count = summary["pending_followup_count"]
    if f_count > 0:
        lines.append(f"## Pending Follow-ups ({f_count})")
        for f in summary["pending_followups"]:
            lines.append(f"- `{f['followup_id']}`: {f['title']}")
        if summary.get("more_followups", 0) > 0:
            lines.append(f"  *(另外 {summary['more_followups']} 项待跟进已省略，详情请使用 show)*")
        lines.append("")

    lt_count = summary["long_term_count"]
    lines.append(f"## Long-term Memory ({lt_count} 项已索引)")
    if summary.get("long_term_samples"):
        for lt in summary["long_term_samples"]:
            lines.append(f"- `{lt['memory_id']}`: {lt['title']}")
        if summary.get("more_long_term", 0) > 0:
            lines.append(f"  *(另外 {summary['more_long_term']} 条长期记忆已省略，详情请使用 recent / search / show)*")
    else:
        lines.append("- *(暂无已索引长期记忆)*")
    lines.append("")

    w_count = summary["worker_state_count"]
    lines.append(f"## Worker States: {w_count} current Worker state(s)")
    lines.append("")
    lines.append("> 提示：启动时仅加载本有界总览；具体记忆正文严禁全量预加载，请按需使用 recent / search / show。")
    return "\n".join(lines).strip()


def detail_for_entry(project_root: Path, entry: dict[str, Any]) -> dict[str, Any]:
    path = project_root / Path(entry["path"])
    record_type = entry["record_type"]
    if record_type == "long-term-entry":
        match = next(
            (
                candidate
                for candidate in parse_long_term_entries(path)
                if candidate.get("entry_id") == entry["locator"]
            ),
            None,
        )
        if match is None:
            raise CatalogError(f"Long-term entry '{entry['memory_id']}' is no longer present")
        return match
    text = read_optional(path)
    if path.suffix in {".yaml", ".yml"}:
        return parse_simple_yaml(path)
    sections = markdown_sections(text)
    if record_type == "temporary":
        names = (
            "topic",
            "current goal",
            "confirmed",
            "rejected",
            "open questions",
            "history references",
        )
    elif record_type == "task":
        names = (
            "objective",
            "current goal",
            "confirmed",
            "open questions",
            "open items",
            "current state",
        )
    else:
        names = (
            "objective",
            "work done",
            "key findings",
            "important paths",
            "open items",
            "recommended next",
            "history refs",
        )
    selected = {name: sections[name] for name in names if name in sections and sections[name]}
    return selected or {"content": compact_text(strip_front_matter(text), 2000)}


def entry_file_text(
    entry: dict[str, Any],
    *,
    revision: int,
    updated_at: str,
    updated_by: str,
) -> str:
    payload = json.dumps(entry, ensure_ascii=False, sort_keys=True, separators=(",", ":"))
    return "\n".join(
        (
            "---",
            f"revision: {revision}",
            f"updated_at: {updated_at}",
            f"updated_by: {updated_by}",
            "---",
            "",
            "# Long-term Memory Entry",
            "",
            "```xiaotao-memory-entry",
            payload,
            "```",
            "",
        )
    )


def migration_preview(
    project_root: Path,
) -> tuple[
    list[dict[str, Any]],
    dict[str, Any],
    list[tuple[dict[str, Any], Path, str | None]],
]:
    current_path = project_root / LONG_TERM_PATH
    if not current_path.is_file():
        raise CatalogError("legacy Long-term current.md does not exist")
    text = read_optional(current_path)
    entries = parse_long_term_entries(current_path)
    if not entries:
        raise CatalogError("legacy Long-term current.md contains no entries to migrate")
    for entry in entries:
        validate_entry_sources(project_root, entry, current_path)
    existing_records = split_long_term_records(project_root, set())
    legacy_records = [(entry, current_path, None) for entry in entries]
    validate_unique_long_term_records([*legacy_records, *existing_records])
    metadata = long_term_metadata(current_path, text, required=False)
    return entries, metadata, existing_records


def migration_lock(project_root: Path, actor: str) -> tuple[int, Path]:
    path = project_root / LONG_TERM_MIGRATION_LOCK
    path.parent.mkdir(parents=True, exist_ok=True)
    try:
        descriptor = os.open(path, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600)
    except FileExistsError as error:
        raise CatalogError(f"migration lock already exists: {path}") from error
    try:
        payload = json.dumps({"actor": actor, "created_at": utc_now()}, sort_keys=True).encode("utf-8")
        os.write(descriptor, payload)
        os.fsync(descriptor)
    except Exception:
        os.close(descriptor)
        path.unlink(missing_ok=True)
        raise
    return descriptor, path


def migrate_long_term(project_root: Path, *, actor: str, apply: bool) -> dict[str, Any]:
    if not apply:
        entries, _, existing_records = migration_preview(project_root)
        active = sum(entry.get("status", "active") in CURRENT_ENTRY_STATUSES for entry in entries)
        return {
            "status": "ready",
            "entries": len(entries),
            "current_entries": active,
            "history_entries": len(entries) - active,
            "preserved_entries": len(existing_records),
            "entry_ids": sorted(
                require_string(entry, "entry_id", project_root / LONG_TERM_PATH)
                for entry in entries
            ),
        }

    actor = " ".join(actor.split()).strip()
    if not actor:
        raise CatalogError("--actor must be a non-empty string when --apply is used")
    descriptor, lock_path = migration_lock(project_root, actor)
    current_path = project_root / LONG_TERM_PATH
    original: str | None = None
    stage_root: Path | None = None
    published: list[Path] = []
    try:
        # Re-read and validate only after taking the global lock. The preview is
        # deliberately advisory and cannot become the source for an applied migration.
        entries, metadata, existing_records = migration_preview(project_root)
        active = sum(entry.get("status", "active") in CURRENT_ENTRY_STATUSES for entry in entries)
        preview = {
            "status": "migrated",
            "entries": len(entries),
            "current_entries": active,
            "history_entries": len(entries) - active,
            "preserved_entries": len(existing_records),
            "entry_ids": sorted(
                require_string(entry, "entry_id", current_path) for entry in entries
            ),
        }
        original = current_path.read_text(encoding="utf-8")
        original_hash = hashlib.sha256(original.encode("utf-8")).hexdigest()
        migration_time = utc_now()
        migration_id = (
            datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
            + "-"
            + original_hash[:8]
        )
        audit_root = project_root / LONG_TERM_MIGRATIONS_PATH / migration_id
        if audit_root.exists():
            raise CatalogError(f"migration audit already exists: {audit_root}")
        revision = metadata.get("revision", 0)
        if isinstance(revision, bool) or not isinstance(revision, int) or revision < 0:
            raise CatalogError(f"{current_path}: 'revision' must be a non-negative integer")
        updated_at = metadata.get("updated_at")
        if not isinstance(updated_at, str) or not updated_at.strip():
            updated_at = migration_time
        updated_by = metadata.get("updated_by")
        if not isinstance(updated_by, str) or not updated_by.strip():
            updated_by = actor

        stage_root = Path(
            tempfile.mkdtemp(prefix=".entry-migration-", dir=project_root / LONG_TERM_ROOT)
        )
        for entry in entries:
            status = entry.get("status", "active")
            folder = "entries" if status in CURRENT_ENTRY_STATUSES else "history"
            target = stage_root / folder / f"{require_string(entry, 'entry_id', current_path)}.md"
            atomic_write(
                target,
                entry_file_text(
                    entry,
                    revision=revision,
                    updated_at=updated_at,
                    updated_by=updated_by,
                ),
            )
            split_entry_file(
                project_root,
                target,
                expected_statuses=(CURRENT_ENTRY_STATUSES if folder == "entries" else HISTORY_ENTRY_STATUSES),
            )

        audit_root.mkdir(parents=True)
        atomic_write(audit_root / "current.md", original)
        atomic_write(
            audit_root / "intent.json",
            json.dumps(
                {
                    "schema_version": 1,
                    "migration_id": migration_id,
                    "actor": actor,
                    "created_at": migration_time,
                    "source_path": LONG_TERM_PATH.as_posix(),
                    "source_sha256": original_hash,
                    "entry_ids": preview["entry_ids"],
                    "preserved_entry_ids": sorted(
                        require_string(entry, "entry_id", path)
                        for entry, path, _ in existing_records
                    ),
                },
                ensure_ascii=False,
                indent=2,
                sort_keys=True,
            ) + "\n",
        )
        for entry in entries:
            status = entry.get("status", "active")
            folder = "entries" if status in CURRENT_ENTRY_STATUSES else "history"
            entry_id = require_string(entry, "entry_id", current_path)
            staged = stage_root / folder / f"{entry_id}.md"
            relative_root = LONG_TERM_ENTRIES_PATH if folder == "entries" else LONG_TERM_HISTORY_PATH
            target = project_root / relative_root / f"{entry_id}.md"
            target.parent.mkdir(parents=True, exist_ok=True)
            if target.exists():
                raise CatalogError(f"migration target appeared after preflight: {target}")
            os.replace(staged, target)
            published.append(target)
        atomic_write(current_path, CURRENT_POINTER_BODY)

        migrated_records = long_term_records(project_root, set())
        migrated = {entry["entry_id"]: entry for entry, _, _ in migrated_records}
        expected = {entry["entry_id"]: entry for entry, _, _ in existing_records}
        expected.update({entry["entry_id"]: entry for entry in entries})
        if migrated != expected:
            raise CatalogError("migration verification failed: Long-term entries changed")
        persist_catalog(project_root, derive_catalog(project_root))
        atomic_write(
            audit_root / "committed.json",
            json.dumps(
                {"schema_version": 1, "migration_id": migration_id, "committed_at": utc_now()},
                sort_keys=True,
            ) + "\n",
        )
        preview["migration_id"] = migration_id
        preview["legacy_snapshot"] = project_relative(project_root, audit_root / "current.md")
        return preview
    except Exception:
        if original is not None:
            atomic_write(current_path, original)
        for target in reversed(published):
            target.unlink(missing_ok=True)
        raise
    finally:
        os.close(descriptor)
        if lock_path.exists():
            lock_path.unlink()
        if stage_root is not None and stage_root.exists():
            shutil.rmtree(stage_root)


def promote_temporary(
    project_root: Path,
    temporary_id: str,
    *,
    task_id: str | None = None,
    actor: str = "xiaotao",
    now: datetime | None = None,
) -> dict[str, Any]:
    def validate_id(value: str, label: str) -> None:
        if not value or value in {".", ".."} or "/" in value or "\\" in value:
            raise CatalogError(f"Invalid {label}: {value!r}")

    validate_id(temporary_id, "temporary id")
    active_root = project_root / ".xiaotao/memory/temporary/active"
    archived_root = project_root / ".xiaotao/memory/temporary/archived"
    temp_dir = active_root / temporary_id
    target_archived_dir = archived_root / temporary_id
    if not temp_dir.is_dir():
        raise CatalogError(f"Active Temporary '{temporary_id}' does not exist")
    if target_archived_dir.exists():
        raise CatalogError(
            f"Archived Temporary already exists: {target_archived_dir}; "
            "existing archives are never overwritten"
        )

    meta_file = temp_dir / "meta.yaml"
    if not meta_file.is_file():
        raise CatalogError(f"{temp_dir}: missing meta.yaml")
    meta = parse_simple_yaml(meta_file)
    if meta.get("status") != "active":
        raise CatalogError(f"Temporary '{temporary_id}' is not active (status: {meta.get('status')})")

    current_file = temp_dir / "current.md"
    current_text = read_optional(current_file)

    topic = meta.get("topic") or temporary_id
    goals = section_values(current_text, ("Current goal", "Goal"))
    confirmed = section_values(current_text, ("Confirmed",))
    open_items = section_values(current_text, ("Open questions", "Pending", "Open items"))
    objective = goals[0] if goals else topic

    tasks_root = project_root / ".xiaotao/tasks"
    if not task_id:
        slug = temporary_id[5:] if temporary_id.startswith("temp-") else temporary_id
        candidate_id = f"task-{slug}"
        if (tasks_root / candidate_id).exists():
            candidate_id = f"task-{slug}-1"
        task_id = candidate_id
    validate_id(task_id, "task id")

    task_dir = tasks_root / task_id
    if task_dir.exists():
        raise CatalogError(f"Task directory already exists: {task_dir}")

    reference_time = now if now is not None else datetime.now(timezone.utc)
    now_str = utc_now(reference_time)
    tx_id = f"tx-promote-{temporary_id}-{int(reference_time.timestamp())}"

    xiaotao_root = project_root / ".xiaotao"
    tasks_root.mkdir(parents=True, exist_ok=True)
    archived_root.mkdir(parents=True, exist_ok=True)
    staging_root = Path(tempfile.mkdtemp(prefix=".promotion-", dir=xiaotao_root))
    task_stage = staging_root / "task"
    archive_stage = staging_root / "temporary"
    active_backup = staging_root / "active-temporary-backup"

    task_yaml_content = f"""id: {task_id}
objective: "{objective}"
status: active
created_at: "{now_str}"
updated_at: "{now_str}"
updated_by: "{actor}"
revision: 0
source_temporary: "{temporary_id}"
promotion_transaction: "{tx_id}"
promoted_at: "{now_str}"
"""
    findings_lines = "\n".join(f"- {item}" for item in confirmed) if confirmed else "- *(由临时探索整理转正)*"
    open_lines = "\n".join(f"- {item}" for item in open_items) if open_items else "- *(暂无遗留待确认项)*"
    progress_content = f"""---
revision: 0
updated_at: "{now_str}"
updated_by: "{actor}"
---

# Task Progress: {topic}

## Objective
{objective}

## Key findings
{findings_lines}

## Open items
{open_lines}
"""

    meta_rev = int(meta.get("revision", 0)) + 1
    aliases_yaml = ""
    if meta.get("aliases") and isinstance(meta["aliases"], list):
        aliases_yaml = "aliases:\n" + "\n".join(f"  - {alias}" for alias in meta["aliases"]) + "\n"
    created_at = meta.get("created_at", now_str)
    updated_meta = f"""id: {temporary_id}
topic: "{topic}"
status: archive
created_at: "{created_at}"
updated_at: "{now_str}"
updated_by: "{actor}"
revision: {meta_rev}
{aliases_yaml}"""

    index_path = project_root / INDEX_PATH
    manifest_path = project_root / MANIFEST_PATH
    catalog_snapshots = {
        path: path.read_bytes() if path.is_file() else None
        for path in (index_path, manifest_path)
    }
    published_archive = False
    published_task = False

    try:
        task_stage.mkdir()
        (task_stage / "task.yaml").write_text(task_yaml_content, encoding="utf-8")
        (task_stage / "progress.md").write_text(progress_content, encoding="utf-8")

        shutil.copytree(temp_dir, archive_stage, symlinks=True)
        (archive_stage / "meta.yaml").write_text(updated_meta.strip() + "\n", encoding="utf-8")
        staged_current = archive_stage / "current.md"
        if staged_current.is_file():
            promoted_note = (
                f"\n\n## Promoted to Task\n"
                f"- Promoted to Task {chr(96)}{task_id}{chr(96)} at {now_str} "
                f"via {chr(96)}{tx_id}{chr(96)}.\n"
            )
            staged_current.write_text(current_text.rstrip() + promoted_note, encoding="utf-8")

        # Keep the source intact while preparing both outputs. Move it to a private
        # rollback location only when the staged outputs are ready to publish.
        if target_archived_dir.exists():
            raise CatalogError(
                f"Archived Temporary already exists: {target_archived_dir}; "
                "existing archives are never overwritten"
            )
        if task_dir.exists():
            raise CatalogError(f"Task directory already exists: {task_dir}")
        os.rename(temp_dir, active_backup)
        try:
            os.rename(archive_stage, target_archived_dir)
            published_archive = True
            os.rename(task_stage, task_dir)
            published_task = True

            expected = derive_catalog(project_root, now=reference_time)
            persist_catalog(project_root, expected)
        except Exception as error:
            rollback_errors: list[str] = []
            if published_task and task_dir.exists():
                try:
                    shutil.rmtree(task_dir)
                except OSError as rollback_error:
                    rollback_errors.append(f"remove task: {rollback_error}")
            if published_archive and target_archived_dir.exists():
                try:
                    shutil.rmtree(target_archived_dir)
                except OSError as rollback_error:
                    rollback_errors.append(f"remove archive: {rollback_error}")
            if active_backup.exists():
                try:
                    os.rename(active_backup, temp_dir)
                except OSError as rollback_error:
                    rollback_errors.append(f"restore active temporary: {rollback_error}")
            for path, content in catalog_snapshots.items():
                try:
                    if content is None:
                        path.unlink(missing_ok=True)
                    else:
                        path.parent.mkdir(parents=True, exist_ok=True)
                        restore_path = path.with_name(f".{path.name}.rollback-{tx_id}")
                        restore_path.write_bytes(content)
                        os.replace(restore_path, path)
                except OSError as rollback_error:
                    rollback_errors.append(f"restore {path}: {rollback_error}")
            if rollback_errors:
                raise CatalogError(
                    f"Temporary promotion failed ({error}); rollback was incomplete: "
                    f"{'; '.join(rollback_errors)}. Recovery data remains at {staging_root}"
                ) from error
            raise

        # Promotion is committed. The backup is no longer needed; cleanup failures
        # leave only a hidden recovery copy and must not turn success into failure.
        shutil.rmtree(active_backup, ignore_errors=True)
        return {
            "status": "promoted",
            "temporary_id": temporary_id,
            "task_id": task_id,
            "task_path": f".xiaotao/tasks/{task_id}/task.yaml",
            "objective": objective,
            "promotion_transaction": tx_id,
            "promoted_at": now_str,
        }
    finally:
        if not active_backup.exists():
            shutil.rmtree(staging_root, ignore_errors=True)

def parse_args(argv: list[str]) -> argparse.Namespace:
    common = argparse.ArgumentParser(add_help=False)
    common.add_argument("--project-root", type=Path, default=argparse.SUPPRESS)
    common.add_argument("--now", help="Fixed ISO-8601 timestamp for reproducible builds or tests", default=argparse.SUPPRESS)

    parser = argparse.ArgumentParser(
        description="Maintain XiaoTao's derived Memory catalog.",
        parents=[common],
    )
    subparsers = parser.add_subparsers(dest="command", required=True)
    subparsers.add_parser("build", parents=[common])
    subparsers.add_parser("check", parents=[common])

    search = subparsers.add_parser("search", parents=[common])
    search.add_argument("query")
    search.add_argument("--layer", choices=("temporary", "task", "long-term"))
    search.add_argument("--memory-kind", choices=sorted(LONG_TERM_KINDS))
    search.add_argument("--context", action="append", default=[])
    search.add_argument("--binding")
    search.add_argument("--limit", type=int, default=5)
    search.add_argument("--no-refresh", action="store_true")
    search.add_argument("--include-inactive", action="store_true")

    overview = subparsers.add_parser("overview", parents=[common])
    overview.add_argument("--format", choices=("text", "json"), default="text")
    overview.add_argument("--limit", type=int, default=3, help="Max entries per section for bounded runtime context (1-5, default 3)")
    overview.add_argument("--full", action="store_true", help="Print full un-truncated manifest.md instead of bounded summary")
    overview_mode = overview.add_mutually_exclusive_group()
    overview_mode.add_argument("--no-refresh", action="store_true")
    overview_mode.add_argument(
        "--cached",
        action="store_true",
        help="Read and validate the existing index without scanning authoritative sources",
    )

    recent = subparsers.add_parser("recent", parents=[common])
    recent.add_argument("--limit", type=int, default=5)
    recent.add_argument("--layer", choices=("temporary", "task", "long-term"))
    recent.add_argument("--include-inactive", action="store_true")
    recent.add_argument("--no-refresh", action="store_true")

    show = subparsers.add_parser("show", parents=[common])
    show.add_argument("memory_id")
    show.add_argument("--include-inactive", action="store_true")
    show.add_argument("--no-refresh", action="store_true")

    migrate = subparsers.add_parser("migrate-long-term", parents=[common])
    migrate.add_argument("--apply", action="store_true")
    migrate.add_argument("--actor", default="")

    promote = subparsers.add_parser("promote-temporary", parents=[common])
    promote.add_argument("temporary_id")
    promote.add_argument("--task-id")
    promote.add_argument("--actor", default="xiaotao")

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
            index = derive_catalog(project_root, now=reference_time)
            persist_catalog(project_root, index)
            print(json.dumps({"status": "built", "entries": len(index["entries"])}, sort_keys=True))
            return 0
        if args.command == "check":
            expected = derive_catalog(project_root, now=reference_time)
            current = load_index(project_root)
            if (
                current is None
                or not entries_equal(current, expected)
                or not manifest_is_current(project_root, current)
            ):
                print("Memory catalog is missing or stale", file=sys.stderr)
                return 1
            print(
                json.dumps(
                    {"status": "current", "entries": len(current["entries"])},
                    sort_keys=True,
                )
            )
            return 0
        if args.command == "migrate-long-term":
            result = migrate_long_term(project_root, actor=args.actor, apply=args.apply)
            print(json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True))
            return 0
        if args.command == "promote-temporary":
            result = promote_temporary(
                project_root,
                args.temporary_id,
                task_id=args.task_id,
                actor=args.actor or "xiaotao",
                now=reference_time,
            )
            print(json.dumps(result, ensure_ascii=False, indent=2, sort_keys=True))
            return 0
        cached = args.command == "overview" and getattr(args, "cached", False)
        if cached:
            index = load_index(project_root)
            if index is None:
                raise CatalogError("Memory catalog cache is missing or invalid; refresh it on demand")
            refreshed = False
        else:
            index, refreshed = ensure_current_index(project_root, refresh=not args.no_refresh, now=reference_time)
        if args.command == "overview":
            if args.limit < 1 or args.limit > 5:
                raise CatalogError("--limit must be between 1 and 5")
            summary = catalog_overview_summary(
                project_root,
                index,
                refreshed,
                limit=args.limit,
                cached=cached,
            )
            if args.format == "json":
                print(json.dumps(summary, ensure_ascii=False, indent=2, sort_keys=True))
                return 0
            if getattr(args, "full", False):
                manifest_path = project_root / MANIFEST_PATH
                if manifest_path.is_file():
                    print(manifest_path.read_text(encoding="utf-8").strip())
                else:
                    print(manifest_text(index).strip())
                return 0
            print(bounded_runtime_context_text(summary))
            return 0
        if args.command == "recent":
            if args.limit < 1 or args.limit > 5:
                raise CatalogError("--limit must be between 1 and 5")
            entries = recent_entries(
                index,
                layer=args.layer,
                include_inactive=args.include_inactive,
                limit=args.limit,
            )
            payload: dict[str, Any] = {
                "catalog_refreshed": refreshed,
                "command": "recent",
                "entries": entries,
                "limit": args.limit,
            }
            if args.layer:
                payload["layer"] = args.layer
            print(
                json.dumps(
                    payload,
                    ensure_ascii=False,
                    indent=2,
                    sort_keys=True,
                )
            )
            return 0
        if args.command == "search":
            if args.limit < 1 or args.limit > 5:
                raise CatalogError("--limit must be between 1 and 5")
            candidates = search_index(
                index,
                args.query,
                layer=args.layer,
                memory_kind=args.memory_kind,
                contexts=args.context,
                binding=args.binding,
                limit=args.limit,
                include_inactive=args.include_inactive,
                project_root=project_root,
            )
            print(
                json.dumps(
                    {
                        "query": args.query,
                        "contexts": args.context,
                        "catalog_refreshed": refreshed,
                        "candidates": candidates,
                    },
                    ensure_ascii=False,
                    indent=2,
                    sort_keys=True,
                )
            )
            return 0
        entry = next(
            (
                item
                for item in index["entries"]
                if item["memory_id"] == args.memory_id
            ),
            None,
        )
        if entry is not None:
            if (
                entry["status"] != "active" or not is_current_knowledge(entry)
            ) and not args.include_inactive:
                raise CatalogError(f"Memory '{args.memory_id}' is unavailable")
            print(
                json.dumps(
                    {
                        "memory": entry,
                        "detail": detail_for_entry(project_root, entry),
                        "catalog_refreshed": refreshed,
                    },
                    ensure_ascii=False,
                    indent=2,
                    sort_keys=True,
                )
            )
            return 0

        _, all_followups = followup_records(project_root, set())
        followup = all_followups.get(args.memory_id)
        if followup is not None:
            if followup["status"] != "pending" and not args.include_inactive:
                raise CatalogError(f"Follow-up '{args.memory_id}' is unavailable")
            print(
                json.dumps(
                    {
                        "followup": followup,
                        "detail": followup,
                        "catalog_refreshed": refreshed,
                    },
                    ensure_ascii=False,
                    indent=2,
                    sort_keys=True,
                )
            )
            return 0

        raise CatalogError(f"Memory '{args.memory_id}' is unavailable")
    except (CatalogError, OSError, RuntimeError, ValueError) as error:
        print(f"memory catalog error: {error}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
