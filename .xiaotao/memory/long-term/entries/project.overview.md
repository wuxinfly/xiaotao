---
revision: 1
updated_at: 2026-09-23T12:00:00Z
updated_by: xiao-tao/maintainer
---

# Project Overview

```xiaotao-memory-entry
{
  "entry_id": "project.overview",
  "title": "XiaoTao 架构全景与模块组织概览",
  "memory_kind": "fact",
  "content": "XiaoTao 是面向结果的 AI 协作 Skill，按模块分为语义核心 xiaotao/、跨宿主安装与命令行 cli/、宿主适配器 adapters/（包括 deepseek-harness、antigravity、codex）以及自动化测试 test/。核心入口由 bin/xiaotao.js 与 xiaotao/SKILL.md 承载。",
  "tags": [
    "architecture",
    "overview",
    "modules",
    "structure",
    "guide"
  ],
  "aliases": [
    "项目结构",
    "架构概览",
    "模块划分",
    "代码地图",
    "project structure",
    "architecture overview"
  ],
  "search_hints": [
    "项目怎么组织的",
    "核心模块在哪",
    "入口文件是哪个",
    "adapters目录做什么",
    "cli如何运行",
    "如何查看项目全貌"
  ],
  "source_refs": [
    "package.json",
    "xiaotao/SKILL.md"
  ],
  "code_refs": [
    "package.json",
    "xiaotao/SKILL.md",
    "bin/xiaotao.js"
  ],
  "code_fingerprints": {
    "package.json": "27d5c7420be49bf6b04fbee8598e88c36815b9ad7e51165a41f833c230fae60d",
    "xiaotao/SKILL.md": "d37862069d09b37ba234693f7dc219109fb1970041bc86dc56fcc87bfa515a6d",
    "bin/xiaotao.js": "06bfa723cc9f7bbc10815dd99c96e01316c08e66ff7b5b95371794d378dfadbb"
  },
  "status": "active"
}
```

## 模块结构与职责

- **`xiaotao/`**：语义 Core 与运行时规范。包含 `SKILL.md`（行为准则与角色定义）、`references/`（内存、协调、存储与 Worker 规范）、`scripts/`（Catalog 派生与校验脚本）。
- **`cli/`**：跨宿主安装与诊断工具。提供 `init`, `update`, `doctor` 命令，支持向 Antigravity、Codex 和 DSH 部署 Skill。
- **`adapters/`**：多宿主运行时适配层。
  - `antigravity/`：Google Antigravity 插件与钩子适配；
  - `codex/`：Codex 多 Agent 会话与子代理扩展；
  - `deepseek-harness/`：DeepSeek Harness 协议适配与确定性状态协议扩展。
- **`bin/`**：可执行入口。`bin/xiaotao.js` 为统一命令行入口。
- **`test/`**：完备的单元测试与契约校验套件。
