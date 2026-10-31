---
revision: 9
updated_at: 2026-10-31T02:52:52Z
updated_by: xiao-tao/maintainer
---

# Project Overview

```xiaotao-memory-entry
{
  "entry_id": "project.overview",
  "title": "XiaoTao 架构全景与模块组织概览",
  "memory_kind": "fact",
  "content": "XiaoTao 是面向软件协作的 AI 项目。现有 Agent Skill、CLI、多宿主适配器和测试位于 xt-skills/；xt-client/ 为客户端预留目录，website/ 是独立网站，docs/ 保存共享文档。Skill 主入口为 xt-skills/xiaotao/SKILL.md，CLI 入口为 xt-skills/bin/xiaotao.js。",
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
    "xt-skills/package.json",
    "xt-skills/xiaotao/SKILL.md"
  ],
  "code_refs": [
    "xt-skills/package.json",
    "xt-skills/xiaotao/SKILL.md",
    "xt-skills/bin/xiaotao.js"
  ],
  "code_fingerprints": {
    "xt-skills/package.json": "e95e20c47fb353a06ef9cbdfc19e977710a767ff9f18bb88f91e76e769147d00",
    "xt-skills/xiaotao/SKILL.md": "d7ad076039b4e2317e40bd4a7bd5425b5c891be3e8c2c6770eaea5ff46a8f758",
    "xt-skills/bin/xiaotao.js": "06bfa723cc9f7bbc10815dd99c96e01316c08e66ff7b5b95371794d378dfadbb"
  },
  "status": "active"
}
```

## 模块结构与职责

- **`xt-skills/xiaotao/`**：语义 Core 与运行时规范。包含 `SKILL.md`（行为准则与角色定义）、`references/`（内存、协调、存储与 Worker 规范）、`scripts/`（Catalog 派生与校验脚本）。
- **`xt-skills/cli/`**：跨宿主安装与诊断工具。提供 `init`, `update`, `doctor` 命令，支持向 Antigravity、Codex 和 DSH 部署 Skill。
- **`xt-skills/adapters/`**：多宿主运行时适配层。
  - `antigravity/`：Google Antigravity 插件与钩子适配；
  - `codex/`：Codex 多 Agent 会话与子代理扩展；
  - `deepseek-harness/`：DeepSeek Harness 协议适配与确定性状态协议扩展。
- **`xt-skills/bin/`**：可执行入口。`xt-skills/bin/xiaotao.js` 为统一命令行入口。
- **`xt-skills/test/`**：完备的单元测试与契约校验套件。
