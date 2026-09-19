# AGENTS.md — 项目决策记录（所有 Agent 必须遵守）

本文件记录本项目（**story**）不可轻易违背的技术决策。任何会话中的 Agent 在写代码前必须先读完本文件；新的决策在获得用户确认后追加到文末。

## 1. 项目目标

- 内容定位：以**中国历史故事与传统文化典籍**为素材（二十四史、资治通鉴、世说新语、笔记小说等），由 AI 生成带旁白、带字幕的 MP4 短视频，发布到抖音（中视频）、快手、B站、小红书、视频号等平台。
- 组织方式：内容按**系列（Series）→ 多集（Episode）**管理，例如「鬼谷子」系列包含若干集。
- 生产方式：一条多步骤、可人工介入的流水线；默认由 Agent 自动驱动，人可以在任意步骤检查产物并决策。

## 2. 技术栈

- 语言：**Go**（当前 go 1.24），编译为单一 CLI 二进制 `story`。
- CLI 框架：`spf13/cobra`。
- AI 能力：阿里云百炼 CLI **`bl`**（通过 `os/exec` 调用，不直接依赖 HTTP SDK）。
- 视频后处理：**ffmpeg / ffprobe**（通过 `os/exec` 调用）。
- 持久化：**SQLite**，驱动用纯 Go 的 `modernc.org/sqlite`（禁止 CGO 依赖）。
- 不引入数据库服务、消息队列；HTTP 仅用**标准库 net/http**（含 Go 1.22+ 方法路由），不引入第三方 Web 框架（2026-09-18 增补：Web UI 见 §11）。

### 外部命令版本注意

- `bl` 参数以**本机 `bl <cmd> --help` 的实际输出为准**，skill 参考文档可能滞后。
- 已知差异（bl 1.25.0）：`bl video generate` 默认模型为 `wan3.0-video`；异步参数是 `--async`（没有 `--no-wait`）；`--resolution` 取值为 `720P` / `1080P`；`--watermark` 默认 true。
- `bl text chat --output json` 兼容多种返回形态：不带 `--quiet` 时为 OpenAI 信封（正文在 `choices[0].message.content`）；带 `--quiet` 非 stream 时直接打印模型正文（无 choices 信封）；带 `--stream --quiet` 时打印 `{"content": ...}` 包装。解析侧 `parseChatContent` 三种都接受。多轮对话用可重复的 `--message role:内容`（见 `internal/provider/bailian/plan.go`）。
- **bl 内置请求超时（两层）**：① bl 自身请求超时可报 `code 5 Request timed out`，provider 在 `run()` 统一注入 `--timeout`（文本/图片 600s，视频生成 1800s）解决；② **bl 内部 undici 固定 300s headers 超时（`--timeout` 与 `bl config set timeout` 均无法绕过）**：非 TTY 下 `--stream` 默认关闭，长文本生成整包等响应头必报 `UND_ERR_HEADERS_TIMEOUT`。解法是 text chat 显式加 `--stream`（provider 的 `runJSON` 已统一处理），此时输出形态变为 `{"content": ...}` 包装，`parseChatContent` 三种形态均兼容。`speech synthesize` 不支持 `--timeout`。

## 3. 架构铁律：接口驱动（Ports & Adapters）

```
cmd/story (CLI)
      │
internal/engine  ── 调度、状态机、验收、并发/重试
      │ 只依赖
internal/port    ── 全部接口定义（StoryGenerator / StoryboardPlanner /
      │              VideoGenerator / SpeechSynthesizer / VideoComposer /
      │              TaskPoller / Repository）
      ▲
      │ 实现
internal/provider/bailian  （bl：故事/分镜/视频/语音/任务）
internal/provider/ffmpeg   （拼接/字幕/转码/多比例导出）
internal/store/sqlite      （Repository 的 SQLite 实现）
```

硬性规则：

1. `internal/engine`、`internal/domain`、`internal/port` **禁止 import** `provider/*`、`store/sqlite` 等任何具体实现；依赖装配只允许发生在 `internal/app` 与 `cmd/story`。
2. `internal/domain` 是纯数据层（struct + JSON tag + 少量纯函数），不依赖任何第三方包。
3. 替换 AI 供应商（如未来接入其他厂商）时，只新增一个 provider 实现，**engine 与 CLI 逻辑零改动**。
4. 所有外部调用接口方法的第一个参数都是 `context.Context`，并返回 error。

## 4. 流水线与状态机

每一集（Episode）独立走一遍流水线：

1. `generate`   — AI 生成 3–5 个候选故事（标题、朝代、出处、梗概、正文），**由用户/Agent 选择一个**。
2. `pick`       — 选定候选，序号写入状态。
3. `storyboard` — AI 将故事拆为 6–12 个镜头（visual_prompt / narration / duration / camera）。
4. `produce`    — 并发生成每个镜头的视频片段与旁白音频；产物落盘，支持断点续跑（已存在的片段默认跳过）。
5. `compose`    — ffmpeg 归一化 → 音视频合成 → 拼接 → 烧录硬字幕，产出最终 MP4。

步骤状态：`pending → running → review → approved → done`，异常分支 `failed`（可重试）。v1 默认自动通过 review 检查点，但状态位保留；人工可随时查看工作目录中的 `story.md`、`storyboard.json` 介入。

每一步必须有**自动验收标准**，验收不通过标记 `failed` 并写入错误信息：

| 步骤 | 验收条件 |
| --- | --- |
| generate | 候选 ≥3，每个含非空标题、出处、正文 |
| pick | 选中序号存在且故事正文非空 |
| storyboard | 镜头 ≥4，每个含非空 visual_prompt / narration，duration 合法 |
| produce | 每个镜头的视频与音频文件存在且非空、可被 ffprobe 解析 |
| compose | 最终文件存在、可播放、时长 ≈ 各镜头之和 |

## 5. 并发与重试

- 多镜头生产使用带缓冲信号量限流，最大并发数取自系列配置（默认 3）。
- 失败自动重试 **3 次**，指数退避（2s、4s、8s）；仍失败则该步骤 `failed`，等待 Agent/人决策。
- 实现集中在 `internal/engine/runner.go`，业务代码不得自行开 goroutine 池或写重试循环。

## 6. 持久化与目录约定

- 结构化状态全部存 SQLite，经由 `port.Repository` 访问；engine 不知道 SQLite 的存在。
- 媒体与中间产物是普通文件，根目录 `data/projects/<series-id>/<episode-id>/`：
  `clips/`（视频片段）、`audio/`（旁白）、`tmp/`（归一化中间件、SRT、concat 清单）、`output/`（成品）。
- `story.md`、`storyboard.json` 同时在集工作目录落一份副本，专供人工审阅；**事实源以数据库为准**。
- `data/` 不入库。

## 7. 历史准确性与 Prompt 约束

- 故事必须标注典籍出处，严禁无依据地杜撰重大史实；白话讲述但保留古典韵味，须有冲突与转折。
- `visual_prompt` 必须经朝代视觉锚定：服饰、建筑、器物、配色、席居方式等符合该朝代生产力水平（见 `internal/templates/dynasty.go`）。
- 所有 LLM 输出一律要求**纯 JSON**，代码侧去除 ``` 代码围栏后解析，并做字段校验。

## 8. 配音合规

- 默认使用系统音色 `longtian_v3`（磁性理智男）+ `--instruction` 调出「沉稳、有书卷气、节奏从容」的历史讲述风格，形成自有品牌声线。
- **实测限制（2026-09-19）**：`longtian_v3 + cosyvoice-v3-flash` 组合不支持 `--instruction` 风格指令（引擎报 428 InvalidParameter）。provider 的 `Synthesize` 已做自动降级：带指令失败后去掉指令重试一次。品牌声线靠音色本身的「磁性理智男」特质承载；后续如需风格控制可评估 `--rate/--pitch` 参数或 v2/clone 音色。
- **禁止**克隆王立群、易中天等真人声音用于商业发布（声音权法律风险）；声音克隆能力不得作为默认路径。

## 9. 字幕与平台适配

- 字幕默认**烧录硬字幕**：由纯 Go 包 `internal/subtitle`（`golang.org/x/image`）把旁白渲染为与画幅同尺寸的透明 PNG（自动探测 PingFang/冬青黑体/STHeiti/宋体等系统中文字体，白字黑边底部居中），ffmpeg 用内置 `overlay` 滤镜按镜头时间区间叠加。**不依赖 ffmpeg 的 libass/freetype 编译选项**（本机 Homebrew ffmpeg 未编入 `subtitles`/`drawtext`）。SRT 仍按各镜头旁白与实际时长生成，作为 `tmp/subs.srt` 旁路基保留。
- 字幕字体可用配置项 `subtitle_font` 指定；留空时自动探测。
- 默认比例 **9:16**（抖音/快手/视频号）；成片后可用 ffmpeg 模糊背景填充方式导出 16:9、1:1、3:4 版本，不重复调用视频生成。
- 保留 AI 生成水印（合规要求），不主动关闭 `--watermark`。

## 10. 测试约定

- mock provider 实现 port 接口，用于 engine / runner / validator 单元测试；SQLite 层用临时库测试。
- 真实调用 `bl` / ffmpeg 的集成测试放在 `//go:build integration` 文件中，默认不运行。
- 提交前必须 `GOTOOLCHAIN=local go build ./... && GOTOOLCHAIN=local go test ./... && GOTOOLCHAIN=local go vet ./...` 通过（本机 go 1.24.0，禁止自动下载新工具链）。

## 11. Web UI（2026-09-18 增补）

- 定位：流水线各阶段产物（候选/故事/分镜/片段/旁白/成片）的**预览 + 操作**界面，与 CLI 共用同一个 `app.App` 容器，不复制业务逻辑。
- 后端：`internal/server` 只依赖 `app`/`domain`（不碰具体 provider/store），标准库 `net/http` 提供 REST + SSE；后台动作经事件总线 `broker` 调度，同一集同时只允许一个动作（冲突返回 409），每秒推送状态快照；媒体接口只允许访问该集 WorkDir 内文件（防路径穿越，越权 403），用 `http.ServeFile` 原生支持 Range（视频拖进度）。
- 前端：`web/`，Vite 8 + React 19 + TypeScript 7 + Tailwind CSS v4（`@tailwindcss/vite`，无 config 文件）+ React Router 7 + TanStack Query 5；SSE 快照直接写入 Query 缓存，无轮询。
- 分发：Vite 构建到 `web/dist/`，由 `web/embed.go` 的 `go:embed` 打进单二进制；`story serve`（默认 127.0.0.1:7878）一条命令启动。`web/dist/` 不入库，但需保留占位 `index.html`（含 `story-web-placeholder` 标记）让无 Node 环境也能 go build；`web/node_modules/` 不入库。
- 开发：Go 侧 `story serve` 跑 API，另在 `web/` 执行 `npm run dev`（5173 代理 /api 到 7878）。
- 新增流水线动作必须同时补 CLI 命令与 server `buildAction` 映射（复用 engine 方法），禁止在 server 中直接写生产逻辑。
- 系列级动作（如定妆照）同样经 broker 调度，以系列 ID 为槽位键；`GET /api/series/{id}/events` 推送系列+集列表快照，`GET /api/series/{id}/media?path=` 只允许访问 `data/projects/<series-id>/` 内文件（防路径穿越）。

## 12. 角色形象一致性（2026-09-19 增补）

- 问题：同一人物跨集、跨分镜形象漂移。方案为**两层**（用户确认）：
  - **第一层（文本）**：系列级「人物设定集」`Series.Characters []CharacterSetting`（name/identity/appearance/temperament/ref_image），SQLite 随 series/plan_sessions 以 `characters_json` 持久化（`ensureColumn` 幂等迁移）。策划会话顺带产出/维护设定集（`appearance` 一经确定不无故改动）；分镜 prompt 注入设定集，**要求设定集内人物出场必须用姓名指代并逐字复制 appearance 原文**（最高优先级规则）。
  - **第二层（视觉）**：系列级定妆照 `data/projects/<series>/refs/<人名>.png`（水墨工笔全身立绘风格，用户确认），`bl image generate` 生成（新 port `ImageGenerator`；配置项 `image_model`，留空用 bl 默认）。produce 时按 visual_prompt 中出现的人名匹配定妆照 → `ClipRequest.RefImages` → 自动改走 `bl video ref`，prompt 前缀声明 Image N 对应人物。
- 入口：策划采纳（apply 随 drafts 提交 characters）、`PUT /api/series/{id}/characters` 人工编辑、`POST /api/series/{id}/keyframes`（body `{"force":bool}`，broker 系列槽位）与 CLI `story keyframes <series-id> [--force]`；Web 系列详情页有「人物定妆照」卡片与策划面板内的人物设定编辑区。
- 幂等：已存在且未 `force` 的定妆照跳过；模型未返回 characters 时不覆盖会话既有人物。

## 变更记录

- 2026-09-18：初始决策（Go + cobra + SQLite；接口驱动；系列/集模型；并发上限 3、重试 3；百炼为首家 provider；ffmpeg 合成与硬字幕；默认 9:16）。
- 2026-09-18（实现修订）：因本机 ffmpeg 未编译 libass/freetype，硬字幕方案从 `subtitles` 滤镜改为 Go 渲染透明 PNG + 内置 `overlay` 叠加；新增 `internal/subtitle` 与配置项 `subtitle_font`。分辨率档位语义为「长边」：720P→720×1280，1080P→1080×1920。依赖固定 `modernc.org/sqlite v1.34.5` + `golang.org/x/image v0.20.0`，本机构建统一加 `GOTOOLCHAIN=local`（go 1.24.0，禁止自动下载新工具链）。
- 2026-09-18（Web UI）：新增 `internal/server`（标准库 net/http REST + SSE，复用 app 层）与 `web/`（Vite 8 / React 19 / TS / Tailwind v4），go:embed 单二进制，`story serve` 启动；§2「不引入 Web 框架」相应澄清为「不引入第三方 Web 框架」。
- 2026-09-19（AI 分集策划）：新增 `port.SeriesPlanner` 与策划会话（`plan_sessions` 表，1 系列 1 会话，模型每轮返回全量草案）；`bl text chat --output json --quiet` 直接打印正文无信封，`parseChatContent` 兼容两形态（记入 §2）。采纳只建集不生产，集数不设小上限（技术上限 100）。
- 2026-09-19（角色形象一致性）：两层方案落地（§12）：人物设定集 + 水墨工笔定妆照；新增 `port.ImageGenerator`、`ClipRequest.RefImages`（非空走 `bl video ref`）、series/plan_sessions `characters_json` 列（ensureColumn 幂等迁移）；Web 增系列级 SSE 与系列媒体端点；CLI 增 `story keyframes`。
