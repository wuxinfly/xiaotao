# 工作区文件管理与浏览器标签

状态：已实现并通过前端、Go 自动验证；WebView2 实机交互待人工验收。

## 需求

办公室右侧工作区标签栏需要一个「+」按钮。点击后可以选择「文件管理」或「浏览器」：

- 文件管理在面板内浏览当前项目目录，支持进入子目录、查看文本和图片、返回上级，并能跳到 Windows 资源管理器。
- 浏览器提供地址栏，输入网址后在应用内访问；打不开时可以转交系统浏览器。
- 两个工具标签与办公室、员工对话共用标签栏，可以切换和关闭；切换员工对话时工具视图自动让位。

## 实现

- `useWorkspaceTools` 按项目维护工具标签顺序和当前选中项，项目切换时清空。
- `WorkspaceAddTab` 使用现有 Portal `Popover` 展示「文件管理」「浏览器」，支持键盘方向键和 Escape。
- `WorkspaceFiles` 只读列目录和预览文件，面包屑可回退；复用 `/api/agent/project/open` 打开资源管理器。
- `WorkspaceBrowser` 把输入归一化为 http/https 地址，地址栏记录用户输入的历史，提供后退、前进、刷新和系统浏览器入口。页面在带沙箱的 iframe 中加载。
- 后端新增 `xt-client/internal/app/workspace_files.go`：
  - `GET /api/workspace/files`：目录优先排序，最多 2000 项。
  - `GET /api/workspace/file`：文本上限 256 KB，NUL 判为二进制，大于 8 MiB 标记过大的文件。
  - `GET /api/workspace/file/raw`：仅允许 png/jpg/jpeg/gif/webp/svg/bmp/ico，最大 20 MiB，`no-store`。
  - `POST /api/system/open-url`：仅接受无用户信息的 http/https 地址，交给系统默认浏览器。
- 所有路由沿用同源检查；文件路径先解析项目根和目标，再做 `filepath.Rel` 越界检查，符号链接不能逃出项目目录。

## 边界

- 文件管理是只读浏览和预览，不提供重命名、删除、上传或编辑。
- iframe 受网站 `X-Frame-Options` / CSP 限制，部分网站会白屏；界面明确提示改用系统浏览器。不实现代理剥离响应头或改写页面。
- 跨域页面无法读取 iframe 当前地址，页面内跳转后地址栏可能仍是用户最后输入的网址；同源页面会同步。
- 地址栏历史只记录用户输入的地址，不接管页面内部导航历史。

## 验证

- 前端：`npm run lint`、`npm run check:size`、`npm run build`；`node --test tests/workspace-tools.browser.test.mjs`。
- Go：`gofmt -l ./internal/app`、`go test ./...`、`go vet ./...`、`node tools/check-go-size.mjs`。
- 浏览器回归使用隔离 API，覆盖加号菜单、地址归一化、iframe 加载、非法协议、系统浏览器、目录导航、文本/图片预览、资源管理器入口和标签关闭。
- 真实网站嵌入、WebView2 内的 iframe 显示和系统浏览器弹窗仍需人工验收。

## 人工验收

1. 打开办公室工作区，点「+」确认菜单只有「文件管理」「浏览器」，键盘可操作，Escape 关闭。
2. 打开文件管理，进入子目录，点文本和图片文件查看预览，点面包屑和返回按钮回到上级。
3. 点「在资源管理器中打开」，确认定位到当前项目目录。
4. 打开浏览器，输入 `example.com`，确认自动补全 https 并加载；输入 `javascript:` 等协议被拒绝。
5. 打开一个禁止嵌入的网站，确认白屏时有说明，并能用系统浏览器打开。
6. 关闭工具标签后办公室恢复；在工具标签打开时点员工工位，确认自动切到员工对话。
7. 在深海和浅海主题、宽窄窗口下检查布局、焦点和减少动态效果。

## 验证结果

2026-11-25：工作区浏览器回归通过；Go 工作区接口测试通过（含路径越界、符号链接、截断文本、二进制、图片和链接校验）；前端 lint、尺寸检查、TypeScript 与生产构建通过；`workspace-tools`、`navigation-layout`、`home-office-collaboration`、`office`、`office-entry`、`office-member-execution`、`office-team-real` 浏览器回归通过；Go 全量测试、vet、Go 尺寸检查和 `./build.ps1 -Verify` 通过。真实网站、桌面壳和系统浏览器交互尚未人工验收。
