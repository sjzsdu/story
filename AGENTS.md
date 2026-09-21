# AGENTS.md — 项目决策记录（所有 Agent 必须遵守）

本文件记录本项目（**story**）不可轻易违背的技术决策。任何会话中的 Agent 在写代码前必须先读完本文件；新的决策在获得用户确认后追加到文末。

## 1. 项目目标

- 内容定位：以**中国历史故事与传统文化典籍**为素材（二十四史、资治通鉴、世说新语、笔记小说等），由 AI 生成带旁白、带字幕的 MP4 短视频，发布到抖音（中视频）、快手、B站、小红书、视频号等平台。
- 组织方式：内容按**系列（Series）→ 多集（Episode）**管理，例如「鬼谷子」系列包含若干集。
- 生产方式：一条多步骤、可人工介入的流水线；默认由 Agent 自动驱动，人可以在任意步骤检查产物并决策。

## 2. 技术栈

- 语言：**Go**（当前 go 1.24），编译为单一 CLI 二进制 `story`。
- CLI 框架：`spf13/cobra`。
- AI 能力：阿里云百炼 CLI **`bl`**（通过 `os/exec` 调用，不直接依赖 HTTP SDK）。**唯一例外**：造声（声音设计 / 声音复刻）`bl` 无对应命令，bailian provider 直连百炼 HTTP `POST /api/v1/services/audio/tts/customization`（见 §16）。
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

1. `generate`   — AI **直接生成一篇定稿故事**（标题、朝代、出处、梗概、正文），生成即自动定稿（2026-09-20 起取消「3 个候选人工选择」，见 §14）；不满意可重跑覆盖。
2. ~~`pick`~~   — 历史步骤，状态位与 `Pick` 方法保留兼容旧数据；新流程在 generate 成功时自动置 done，用户无感知。
3. `storyboard` — AI 将故事拆为 6–12 个镜头（visual_prompt / narration / duration / camera）。
4. `produce`    — 按系列设置 `visual_mode` 生产画面并合成旁白；产物落盘，支持断点续跑（已存在的片段默认跳过）。两种模式（见 §13）：`comic`（默认，小人书：每镜一张 AI 插画 → 本地 ffmpeg Ken Burns 渲染片段，仅按图片计费）/ `video`（每镜 AI 视频生成）。
   - **只重试未完成的镜头（2026-09-21）**：`produce` 先按磁盘产物规划（`planScenes`），**只为「画面或旁白尚未产出」的镜头建任务**，已完成的镜头连任务都不建、零费用；因此直接重跑 `produce` 等价于「只重试失败镜头」。指定镜头用 `Engine.ProduceScenes`（CLI `story produce --scenes 13,15`、server `{"action":"produce","scenes":[13,15]}`、Web 单镜「重试本镜」）；序号不存在直接报错。
   - **画面与旁白互相独立**：同一镜内画面失败也照常合成旁白，各自登记错误（`Err` 前缀「画面:」/「旁白:」），避免下次重试把已付费的那一项再跑一遍。
   - **可手动停止**：Web 运行中「停止」→ `POST /api/episodes/{id}/cancel` → broker 取消该集 job 的 context（kill 正在跑的 bl/ffmpeg 子进程）；已完成的产物全部落盘保留，步骤标注「已手动停止」，之后再次执行即从断点续跑。
   - 任务成功但整集仍有未完成镜头时（如只点了单镜重试）：不返回错误，仅把步骤标记 `failed` 并列出剩余镜头，便于继续续跑。
5. `compose`    — ffmpeg 归一化 → 音视频合成 → 拼接 → 烧录硬字幕，产出最终 MP4。

步骤状态：`pending → running → review → approved → done`，异常分支 `failed`（可重试）。v1 默认自动通过 review 检查点，但状态位保留；人工可随时查看工作目录中的 `story.md`、`storyboard.json` 介入。

每一步必须有**自动验收标准**，验收不通过标记 `failed` 并写入错误信息：

| 步骤 | 验收条件 |
| --- | --- |
| generate | 定稿故事 1 篇，含非空标题、出处、正文（生成时自动完成 pick） |
| ~~pick~~ | 仅旧数据：选中序号存在且故事正文非空；新流程无需人工动作 |
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

- **叙事定位**：内容是百家讲坛式（王立群读史记）**口播讲述稿**——故事为主、画面为辅；开头两句必须是钩子（反常细节/悬念/设问），结构按「钩子→铺垫→冲突加压→高潮（典籍原话慢写）→转折→点题余味」，讲述者声音在场（设问、卖关子、夹叙夹议不超过两成）。禁止生平流水账开头与现代腔。
- 故事必须标注典籍出处，严禁无依据地杜撰重大史实；关键对话/情节须出自所标典籍；白话讲述但保留古典韵味。
- `visual_prompt` 必须经朝代视觉锚定：服饰、建筑、器物、配色、席居方式等符合该朝代生产力水平（见 `internal/templates/dynasty.go`）。
- **全片画风统一**（2026-09-19 修订）：`visual_prompt` 只写画面内容，**严禁出现任何画风/质感/媒介词**（电影感、写实、真人、动漫、3D、卡通、照片、水彩、构图、质感等）。画风由 `internal/templates/visualstyle.go` 的风格包唯一供给：分镜 prompt 注入 Brief（约束 LLM）+ engine `buildVideoPrompt` 末尾固定追加 VideoAnchor（代码双保险，不信任模型）+ 定妆照 KeyframePrompt 同源。风格 key 存系列配置 `video_style`（`gongbi` 工笔重彩国风动画＝默认、`realistic` 写实真人、`ink` 水墨写意），空值与未知值回退 gongbi。
- 所有 LLM 输出一律要求**纯 JSON**，代码侧去除 ``` 代码围栏后解析，并做字段校验。
- 镜头时长除 prompt 约束（6-10 秒、duration ≥ 旁白字数÷4.5）外，engine `normalizeDurations` 按旁白字数代码侧兜底上调（只上调、封顶 10s），防止旁白长于画面导致成片定格拉伸。

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
- **成本红线（2026-09-19）**：开发与测试过程**禁止真实调用 bl**（按次计费、消耗快）。功能验证一律用 mock provider、单元测试与本地 ffmpeg；真实 bl 调用仅用于用户明确要求的生产执行（正式生成分集/成片），且发起前应告知预计调用量。
- 提交前必须 `GOTOOLCHAIN=local go build ./... && GOTOOLCHAIN=local go test ./... && GOTOOLCHAIN=local go vet ./...` 通过（本机 go 1.24.0，禁止自动下载新工具链）。

## 11. Web UI（2026-09-18 增补）

- 定位：流水线各阶段产物（候选/故事/分镜/片段/旁白/成片）的**预览 + 操作**界面，与 CLI 共用同一个 `app.App` 容器，不复制业务逻辑。
- 后端：`internal/server` 只依赖 `app`/`domain`（不碰具体 provider/store），标准库 `net/http` 提供 REST + SSE；后台动作经事件总线 `broker` 调度，同一集同时只允许一个动作（冲突返回 409），每秒推送状态快照；媒体接口只允许访问该集 WorkDir 内文件（防路径穿越，越权 403），用 `http.ServeFile` 原生支持 Range（视频拖进度）。
- 前端：`web/`，Vite 8 + React 19 + TypeScript 7 + Tailwind CSS v4（`@tailwindcss/vite`，无 config 文件）+ React Router 7 + TanStack Query 5；SSE 快照直接写入 Query 缓存，无轮询。
- 分发：Vite 构建到 `web/dist/`，由 `web/embed.go` 的 `go:embed` 打进单二进制；`story serve`（默认 127.0.0.1:7878）一条命令启动。`web/dist/` 不入库，但需保留占位 `index.html`（含 `story-web-placeholder` 标记）让无 Node 环境也能 go build；`web/node_modules/` 不入库。
- 开发：Go 侧 `story serve` 跑 API，另在 `web/` 执行 `npm run dev`（5173 代理 /api 到 7878）。
- 新增流水线动作必须同时补 CLI 命令与 server `buildAction` 映射（复用 engine 方法），禁止在 server 中直接写生产逻辑。
- 系列级动作（如定妆照）同样经 broker 调度，以系列 ID 为槽位键；`GET /api/series/{id}/events` 推送系列+集列表快照，`GET /api/series/{id}/media?path=` 只允许访问 `data/projects/<series-id>/` 内文件（防路径穿越）。

## 12. 角色形象一致性（2026-09-19 增补；2026-09-20 泛化见 §15）

- 问题：同一人物跨集、跨分镜形象漂移。方案为**两层**（用户确认）：
  - **第一层（文本）**：系列级「人物设定集」`Series.Characters []CharacterSetting`（name/identity/appearance/temperament/ref_image），SQLite 随 series/plan_sessions 以 `characters_json` 持久化（`ensureColumn` 幂等迁移）。策划会话顺带产出/维护设定集（`appearance` 一经确定不无故改动）；分镜 prompt 注入设定集，**要求设定集内人物出场必须用姓名指代并逐字复制 appearance 原文**（最高优先级规则）。
  - **第二层（视觉）**：系列级定妆照 `data/projects/<series>/refs/<人名>.png`（风格与全片统一画风同源，默认工笔重彩国风动画，由 `visualstyle.go` 风格包渲染 prompt），`bl image generate` 生成（新 port `ImageGenerator`；配置项 `image_model`，留空用 bl 默认）。produce 时按 visual_prompt 中出现的人名匹配定妆照 → `ClipRequest.RefImages` → 自动改走 `bl video ref`，prompt 前缀声明 Image N 对应人物。
- 入口：策划采纳（apply 随 drafts 提交 characters）、`PUT /api/series/{id}/characters` 人工编辑、`POST /api/series/{id}/keyframes`（body `{"force":bool}`，broker 系列槽位）与 CLI `story keyframes <series-id> [--force]`；Web 系列详情页有「人物定妆照」卡片与策划面板内的人物设定编辑区。
- 幂等：已存在且未 `force` 的定妆照跳过；模型未返回 characters 时不覆盖会话既有人物。

## 13. 画面模式：小人书 comic 与 AI 视频 video（2026-09-19 增补）

- 决策：口播为主、画面为辅的百家讲坛定位下，默认采用**小人书模式**（连环画形态），每镜一段旁白配一张 AI 插画，由本地 ffmpeg Ken Burns 运镜（推近/拉远/上下左右平移/定格，smoothstep 缓动）渲染成与视频模式同规格的 `clips/scene-XX.mp4`。理由：①图片单价比视频生成低一个数量级，失败不重试烧钱；②静态工笔画不存在跨帧漂移与动作畸变，全片画风最稳；③出图快、重跑便宜。
- 配置：系列配置 `visual_mode`（`comic`＝默认，空值/未知值回退；`video`＝AI 视频）。**创建时锁定、不可更改**（2026-09-20 修订，曾提供的 Web 选择器与 `PUT /api/series/{id}/config` 已移除）；入口：CLI `story series create --visual-mode`、Web 新建系列弹窗。
- 实现要点：`domain.NormalizeVisualMode` 归一；engine `Produce` 按模式分流，comic 路径为 `ImageGenerator`（插画落 `panels/scene-XX.png`，9:16 出图 1080×1920）→ `port.VideoComposer.RenderStill`（`internal/provider/ffmpeg/still.go`，zoompan：输入先 cover 到 2× 缓冲防抖，30fps 精确帧数输出 720×1280/1080×1920、无音轨）；后续 Compose（归一化/字幕/拼接/导出）两种模式完全复用。
- 运镜来源：分镜 `camera` 限定「推近/拉远/左移/右移/上移/下移/定格」七词（可带对象），engine `motionForScene` 关键词映射；缺省时按镜头序号在轮换表取值，避免全片运镜雷同。
- 人物一致性：comic v1 靠分镜 prompt 的 appearance 逐字复制（§12 第一层）；`bl image generate` 不支持参考图，后续如需更强一致性可验证 `bl image edit --image 定妆照`（真实验证一次即可，禁止批量烧钱）。
- 已用 video 模式产出的片段不受模式切换影响（续跑只看文件是否存在）；新模式只影响之后新生产的镜头。

## 14. 故事生成即定稿，取消候选选择（2026-09-20 增补）

- 决策：取消「generate 生成 3 个候选 → 用户 pick」的人工选择环节（用户反馈选择困难，且多候选白白消耗 token）。`generate` 只产出**一篇定稿口播稿**，成功后 engine 自动把 `pick` 步骤置 done、写入 Story 与 story.md 审阅副本，下一步直接 storyboard。不满意可重跑 generate 覆盖。
- 兼容：状态结构（Candidates/Selected/StepPick）与 `Engine.Pick`、CLI `story pick`、server `pick` 动作全部保留，用于历史上停在 generate 与 pick 之间的旧数据；前端 StepsBar 不再展示 pick 节点。`StoryRequest.Count` 废弃，provider 忽略。
- 模型契约：故事 prompt 输出从 `{"candidates":[...]}` 改为单对象 `{title,dynasty,source,summary,content}`（bailian `storyResponse`），provider 包装为单元素切片返回；验收下限 `minCandidates` 3→1。
- 一键流程：CLI `story run` 与 server `run` 动作在 Story 为空时直接调用 generate（含旧数据），不再需要 index 参数。

## 15. 视觉参考两级化：人物 + 场景（2026-09-20 增补）

- 背景：聊斋这类单元剧每集人物不同，系列级「人物定妆照」不适用；且一致性约束不只是人物——跨镜头重复出现的**场景/环境**（如兰若寺大殿）也需要固定。UI/文案统一改称**视觉参考**（系列视觉参考 / 本集视觉参考），不再叫定妆照。
- 数据模型：`domain.VisualRef{Kind: "character"|"scene", Name, Description, RefImage}`（`internal/domain/visualref.go`，空/未知 Kind 归一 character）。
  - **系列级**：仍沿用 `Series.Characters []CharacterSetting`（跨集复用的主角，策划会话产出）；engine 内部经 `mergeVisualRefs` 转成 character 类 VisualRef。
  - **集级**：`Episode.Refs []VisualRef`（SQLite `episodes.refs_json`，ensureColumn 幂等迁移），人物 + 出现 ≥2 次的场景；`Storyboard.Refs` 只作 storyboard.json 审阅快照，事实源是 `Episode.Refs`。
- **两级合并规则**（`engine.mergeVisualRefs`）：系列人物先入列，集级 refs 追加；同 kind+name **集级覆盖系列级**（单元剧可为本集重定义形象）。produce（video 模式）与分镜 prompt 均用合并结果。
- 产出时机（成本红线配套）：分镜只产出**零成本文字描述**（storyboard prompt 规则 8 输出 `refs.characters/scenes`，bailian `storyboardResponse.Refs` 解析）；**参考图手动点按钮才按张计费生成**。重新 storyboard 时 `preserveRefImages` 按 kind+name 延续旧 RefImage，已付费图片不丢关联；缺名/缺描述的条目丢弃。
- 参考图：系列人物图落 `data/projects/<series>/refs/`（`story keyframes`，3:4 人物立绘）；集级图落 `<episode>/refs/`（`story episode-refs <id> [--force]` / `POST /api/episodes/{id}/refs`，broker 集槽位）。场景图用 `VisualStylePack.SceneRefPrompt`（空镜、无人、按成片比例出图，9:16 默认），人物图沿用 `KeyframePrompt`；画风从句 `SceneClause` 与全片风格同源（gongbi/realistic/ink 三包已补）。
- 生效路径：文字约束在分镜阶段注入 prompt（visual_prompt 须用 name 指代并逐字复制 description）；**参考图仅 video 模式** produce 时经 `refImagesForScene`（人物/场景名 Contains 匹配）→ `bl video ref`，`refPromptPrefix` 区分「人物形象参考/场景环境参考」。comic 模式 `bl image generate` 不支持参考图，只吃文字约束。
- Web：集详情页新增折叠区「本集视觉参考（人物/场景）」（缩略图 + 描述 + 生成缺失/全部重生按钮，徽标显示条数与图片数）；系列页卡片改名「系列视觉参考 · 人物」，空态提示单元剧人物/场景在集页管理。旧分镜无 refs 时提示重跑 storyboard（不产生图片费用）。

## 16. 声音库顶层实体化（2026-09-20 增补）

- 背景：旧路径把旁白音色硬编码在 `templates.VoicePresets`（初版 6 个预设用名人风格化名 wangliqun/kaishu/yizhongtian/shuoshu/cangsang/zhixing；2026-09-20 因名不副实改为音色真实 ID：longtian/longze/longcheng/longfei/longhao/longxiaoxia），参数散落在 `SeriesConfig`（TTSVoice/TTSInstruction/TTSRate/TTSPitch）与 `VoiceProfile` 字段，用户既无法新建/编辑音色条目，也无法跨系列复用自定义参数。决策：把声音提升为与 Series 同级的顶层持久化实体，新建系列时选定一个 voice_id 即可，**创建后锁定不可改**（与 §13 画面模式同款锁定语义）。
- 数据模型：`domain.Voice struct{ID,Name,Provider,Voice,Model,Instruction,Rate,Pitch,StyleNote,IsBuiltin,CreatedAt,UpdatedAt}`（`internal/domain/voice.go`）；`Series.VoiceID string`（`series.voice_id` 列，ensureColumn 幂等迁移）；`Voice.ToProfile()` 转回 `VoiceProfile` 供旧 API 用。`VoiceProfile` 值对象保留作 engine 间 DTO 与旧字段兼容期的回退。`Voice.Model` 为**驱动模型**（造声音色必填，普通音色留空走全局 `tts_model`）。
- **供应商归属（2026-09-20 修订）**：一条声音条目只属于**一个** TTS 供应商——`Voice.Provider`（`bailian` 等，空值/未知值经 `NormalizeVoiceProvider` 归一 bailian），`Voice.Voice` 存该供应商体系内的音色 ID。修订原因：初版 `Voice` 字段语义上就是百炼音色 ID（seed 的 6 个内置条目全是 `longtian_v3` 等），虽编译不耦合，但换供应商后旧条目全部失效、系列锁着用不了的 voice_id，违背 §3 精神。本方案（用户拍板，未选多供应商绑定）把供应商显式化：接第二家 TTS 时，新增该 provider 的 VoiceLister + 声音条目，UI 按供应商分组取数；代价是旧系列的声音不能自动平移，需人工为本系列建新声音（voice_id 锁定语义不变）。供应商清单前端维护在 `VoicesPage` 的 `PROVIDER_OPTIONS`，后端 `NormalizeVoiceProvider` 白名单，两边同步。
- 持久化：`internal/store/sqlite/voices.go` 实现 `voices` 表 CRUD + 两个一次性启动辅助：
  - schema 含 `provider TEXT NOT NULL DEFAULT 'bailian'` 与 `model TEXT NOT NULL DEFAULT ''`；旧库经 ensureColumn 加 `voices.provider` / `voices.model` 列，旧行自动得 bailian / 空串（幂等）。
  - `SeedBuiltinVoices(ctx, store, presets)`：Bootstrap 在 `Open` 之后调一次，用 `INSERT OR IGNORE` 把 6 个预设写入 voices（id=key, is_builtin=1, provider='bailian'，幂等可重复跑）。
  - `MigrateSeriesVoiceIDs(ctx, store, fallbackVoice, fallbackInstr)`：平迁旧 series 写回 voice_id。规则按优先级：①voice_id 已有跳过；②VoiceProfile 对应内置条目存在则用 key 作 id；③TTSVoice 或 fallback 非空 → 先在 voices 表按 voice+rate+pitch+instruction 匹配既有条目，否则建 `custom-<nanos>` 用户条目并写回；④全空 → 写默认条目 ID（longtian）。
  - `RemapLegacyBuiltinVoices(ctx, store)`：名人化名 ID → 音色真实 ID 的一次性数据修正（seed 新条目 → UPDATE series.voice_id → DELETE 旧内置行，映射表见 `legacyBuiltinVoiceIDs`），幂等；Bootstrap 在 MigrateSeriesVoiceIDs 之后调。
  - 错误哨兵：`ErrVoiceBuiltin`（内置不可删）、`ErrVoiceInUse`（被系列引用拒绝删除）。
- **名实相符（2026-09-20 修订）**：内置声音条目的 ID/Name/StyleNote 必须与系统音色真实身份一致——龙天（longtian_v3）就是「龙天·磁性理智男」，不再用「王立群风格」这类实际调不出来的名人化名；Instruction/Rate/Pitch 不附加名人模拟参数，一律留空走音色默认，想要变体由用户复制条目自行调整。UI 不向用户展示条目内部 ID（折叠 badge/卡片均显示名字）。
- 接口扩展：`port.Repository` 新增 `CreateVoice/GetVoice/ListVoices/UpdateVoice/DeleteVoice/CountSeriesByVoiceID`；mock 同步实现 noop 内存版。另新增小端口 `port.VoiceLister`（`ListSystemVoices(ctx, model) []SystemVoice`），bailian provider 实现为执行 `bl speech synthesize --list-voices`（只取元数据、不合成、不计费）并解析固定列宽表格（strings.Fields 四字段：id/name/description/language，描述含空格时中间合并）。`app.App` 新增 `ListVoices/GetVoice/CreateVoice/CreateVoiceInput/UpdateVoice/DeleteVoice/GetSeriesVoice/ListSystemVoices`（App 持有 voiceLister，Bootstrap 注入 bl）；`CreateSeriesInput` 加 `VoiceID` 字段，`CreateSeries` 校验 voice_id 必填（空时按 resolveCreateSeriesVoiceID 平迁规则回退）；`ListVoiceProfiles` 改为读表返回 `[]*domain.Voice`；`MatchVoiceProfile(key)` 改为按 key 查表回退到 templates 兜底。
- **创建声音的四种方式**（2026-09-20 用户拍板，均在 VoicesPage）：
  1. **手动填写**：名称 + 供应商 + 音色 ID + 指令/语速/音高/风格说明。
  2. **从已有声音复制**：卡片「复制」→ 新建表单预填全部参数、名称加「副本」后缀，改几项即存。
  3. **浏览音色库挑选**：表单内「浏览音色库」展开内嵌面板（搜索 id/名称/特质，实时调 VoiceLister），点音色即填入 ID，名称为空时顺带带出音色中文名；不需要手记音色 ID。
  4. **先试音再保存**：表单内「先试听」按当前参数实时合成（previewVoice），音频内嵌播放，满意后点创建/保存，避免存了不能听的条目再返工。
- **百炼造声：声音设计 / 声音复刻（2026-09-20 增补，用户要求）**：`bl` CLI **没有造声命令**，故 provider 内**直连百炼 HTTP**（§2「AI 能力经 bl」的例外，仅此一处；记入 §2 例外说明）。
  - 接口：`POST {base_url}/api/v1/services/audio/tts/customization`，Header `Authorization: Bearer <api_key>`、`Content-Type: application/json`，请求体 `{"model":"voice-enrollment","input":{...}}`。**声音设计**用 `action=create_voice` + `voice_prompt`(≤500字) + `preview_text`(≤200字)，返回 `output.voice_id` 与 `output.preview_audio.data`(base64 试听音频)；**声音复刻**用 `action=create_voice` + `url`(公网/oss 音频) + `language_hints` + `max_prompt_audio_length`(3-30) + `enable_preprocess`，返回 `output.voice_id`。错误可能落在顶层 `code/message` 或 `output.code/message`，两处都检查。
  - **模型一致性铁律**：造声 `target_model` 必须与后续合成 `--model` **完全一致**，否则合成失败。故新增 `Voice.Model`（落 voices.model 列）与 `port.SpeechRequest.Model`；engine `resolveVoice` 现返回 `(voice, model, rate, pitch, instr)`，`Produce`/`PreviewVoice` 把 model 透传给合成。造声默认 `cosyvoice-v3-flash`（`domain.VoiceBuildModelDefault`）；支持复刻+设计+指令控制的模型：`cosyvoice-v3.5-plus`/`cosyvoice-v3.5-flash`/`cosyvoice-v3-flash`（`cosyvoice-v3-plus` 不支持指令控制）。
  - 音频上传：复刻的参考音频可传本地路径（provider 内部走 `bl file upload --file <path> --model <model>`，返回 `oss://<key>`，请求体带 `oss://` 时自动加 Header `X-DashScope-OssResourceResolve: enable` 让服务端解析），或直接给公网/`oss://`/`data:` URL。
  - 凭据：`config.BailianAPIKey`/`BailianBaseURL`（env `STORY_BAILIAN_API_KEY`/`STORY_BAILIAN_BASE_URL`），空则 provider 回退环境变量 `DASHSCOPE_API_KEY`、再读 `~/.bailian/config.json` 的 `api_key`；base_url 空用 `https://dashscope.aliyuncs.com`（国际站 `dashscope-intl`）。
  - 落库：造声成功后 `app.BuildVoice` **自动**创建一条声音条目（Voice=返回的 voice_id，Model=target_model，Provider=所选供应商、默认 bailian，StyleNote 默认「声音设计：<prompt前40字>」/「声音复刻（上传音频）」），用户无需手填 voice_id；试听音频落系统临时目录 `story-voice-preview/`，可经 `GET /api/voices/preview?path=` 回放。
  - 合规与成本：**禁止复刻他人真人声音**（声音权法律风险）；造声按新建音色个数计费（CosyVoice 未单列单价，试听音频另按合成字数计费），受 §10 成本红线约束——开发/测试禁用真实调用，一律 mock。
  - 新接口：`port.VoiceBuilder`（`BuildVoice(ctx, VoiceBuildRequest) (VoiceBuildResult, error)`，`Kind` 取 `domain.VoiceBuildDesign`/`VoiceBuildClone`）。
  - **供应商维度（2026-09-21 增补，用户要求）**：造声与供应商强绑定（模型/音色/参数体系各异），故 **provider 必须可传入**而非写死。装配层持**注册表** `App.voiceBuilders map[string]port.VoiceBuilder`（key = `domain.VoiceProviderXxx`；Bootstrap 当前只登记 `bailian: bl`，接第二家 TTS 时加一项即可，engine/CLI 逻辑零改动）；`app.BuildVoice` 开头经 `resolveVoiceBuilder(provider)` 选实现，空值默认 bailian，**显式给出未登记的供应商直接报错**（错误文案列出已支持项）——刻意**不与 `NormalizeVoiceProvider` 的「未知静默回退 bailian」同语义，避免把 `iflytek` 静默当成百炼去造声**；`BuildVoiceInput.Provider` 解析结果同时写入新建声音条目的 `Provider`。
  - provider 入口：CLI `story voice design|clone --provider`；server `POST /api/voices/design|clone` body 加 `provider`；Web `BuildVoiceModal` 加「TTS 供应商（造声能力按供应商分发）」下拉（复用 `PROVIDER_OPTIONS`）。注：`App.voiceLister`（浏览音色库）暂仍为单一字段，`ListSystemVoices(ctx, provider, model)` 内部 normalize + 白名单守卫（未知供应商报错、server 映射 404），未一并改注册表。
  - **参考音频来源三 Tab（2026-09-21 增补，用户要求）**：Web 复刻表单不再让用户手填服务器绝对路径，改为三个平级 Tab——**上传音频文件** / **现场录音** / **音频 URL**。
    - **浏览器录音格式必须服务端归一化**：`MediaRecorder` 在 Chrome/Firefox 输出 `audio/webm;codecs=opus`、Safari 输出 `audio/mp4`，供应商复刻接口均不接受；用户上传件也可能是 48kHz 立体声 m4a。故新增小端口 `port.AudioNormalizer`（`NormalizeAudio(ctx, src, dst) (float64, error)`，返回时长供长度校验），由 ffmpeg provider（`internal/provider/ffmpeg/audio.go`，与 `VideoComposer` 同一实例）实现为 `-vn -ac 1 -ar 16000 -c:a pcm_s16le` 的 16kHz 单声道 PCM wav。
    - **两段式提交（不破坏 clone 契约）**：新增 `POST /api/voices/audio`（multipart，字段 `file`，25MB 上限）→ `App.SaveVoiceSample` 落盘归一化 → 返回 `{path, duration_sec}`；前端再调既有 `POST /api/voices/clone`（JSON 带 `audio_path`）。好处是天然支持「先上传试听、再决定是否克隆」，**克隆那一步才计费**。`App` 新增 `audioNormalizer port.AudioNormalizer` 字段（Bootstrap 注入 composer），未装配时报错。
    - **落盘与留存**：原始件 + 归一化件均落系统临时目录 `story-voice-samples/`（`raw-<nanos>.ext` / `sample-<nanos>.wav`，与试听目录 `story-voice-preview/` 并列），**不主动删除**（与试听音频策略一致，便于排查）。文件名只经 `sanitizeAudioExt`（先 `filepath.Base` 再只放行字母数字、长度 ≤5）取扩展名，**绝不使用上传文件名拼路径**（路径穿越防护，单测覆盖）。
    - **时长校验**：归一化返回的时长 <3 秒直接报错（供应商要求 3-30 秒）；前端录音到 30 秒自动停止，并在提交前用返回时长提前拦一次过短。
    - **录音细节**：`getUserMedia` 仅 https/localhost 可用（`story serve` 默认 `127.0.0.1:7878` 属安全上下文）；mimeType 用 `MediaRecorder.isTypeSupported` 探测（webm/opus → webm → mp4 → ogg/opus）；停止时 `track.stop()` 释放麦克风，弹窗卸载时清理计时器 + 停流 + 释放 objectURL。页面给一段**可编辑的推荐朗读文本**（`CLONE_SAMPLE_TEXT`，含叙述句与设问句、约 25 秒）供用户照着念，以试出音色全貌；该文本仅为提示，不随请求提交。
    - **multipart 陷阱**：`web/src/api.ts` 的 `request` 默认写死 `Content-Type: application/json`，`uploadVoiceSample` 必须用 `headers: {}` 覆盖，否则 boundary 丢失、后端 `ParseMultipartForm` 失败。
    - **CLI 不变**：`story voice clone --audio <本地路径>` 保持现状（服务端本地执行，无浏览器场景）；归一化仅用于 Web 上传/录音路径。
- engine 解析：`internal/engine/voice.go` 重写 `resolveVoice(ctx, cfg, voiceID, fallbackVoice, fallbackInstr) (voice, model string, rate, pitch float64, instr string)` 三级回退：①优先按 voice_id 查表（`repo.GetVoice`，带出 Model）；②旧 `cfg.VoiceProfile` 非空 → `templates.MatchVoiceProfile`；③旧 `cfg.TTSVoice` 裸读 + fallback 兜底。`Produce` 调用点 `engine.go:315` 改为传 `series.VoiceID` 并把 model 透传给 `port.SpeechRequest.Model`。
- **锁定语义**：靠 SQL 不入 voice_id 列实现——`store.UpdateSeries` 的 SET 子句不含 `voice_id` 列，应用层不需要判断；`PUT /api/series/{id}/voice` 路由保留但 `voiceProfileLocked` handler 直接返回 409「声音创建后锁定，请到声音页编辑条目」。**声音条目本身可编辑**，改后影响所有引用它的系列（这正是把声音提升为顶层实体的目的——一次改全网生效）。
- 旧字段兼容期：`SeriesConfig.TTSVoice/TTSInstruction/TTSRate/TTSPitch/VoiceProfile` **不删**，保留给 resolveVoice 规则 ②③作回退路径；新建系列一律走 voice_id，旧字段在 SeriesConfig 中可空。删除旧字段需要等所有旧 series 完成迁移并确认无回退需求。
- 入口与 UI：
  - CLI：新增 `story voice` 子命令（list/show/add/edit/rm/preview，flags `--name/--provider/--voice/--model/--instruction/--rate/--pitch/--style-note/--text`）；造声子命令 `story voice design`（`--name/--provider/--prompt/--preview-text/--model/--language/--style-note`）与 `story voice clone`（`--name/--provider/--audio/--audio-url/--model/--language/--max-audio-length/--preprocess/--style-note`）；`story series create` 新增 `--voice <voice-id>`（推荐），旧 `--voice-profile` 兼容，旧 `--voice` 改名 `--voice-raw`。
  - server：新增 `POST/GET/PUT/DELETE /api/voices/{id}` 与 `/api/voices/preview`（POST，body `voice_id`/`profile`/`voice+rate+pitch+instruction` 三种入口）；造声新增 `POST /api/voices/design` 与 `POST /api/voices/clone`（body 见 §16 造声条目 + `provider`，返回 `{voice, preview_audio_path}`）；`createSeries` 请求体加 `voice_id` 字段。
  - Web：新增独立路由 `/voices` 与 `VoicesPage`（卡片网格 + VoiceFormModal 新建/编辑/复制复用，内置禁删、引用时弹错）；页头三个入口「新建声音 / 声音设计 / 声音复刻」，造声走 `BuildVoiceModal`（design 填描述+试听文本 / clone 用三 Tab 选参考音频来源：上传文件、现场录音、音频 URL，见 §16 三 Tab 条目，成功后内嵌播放试听并刷新列表）；表单含「驱动模型」字段、卡片展示 model；nav 加「声音」入口；新建系列 Modal 加声音下拉（默认 longtian，Field 标「声音（创建后不可更改）」）；系列详情页 Collapsible 标题从「旁白语音画像」改名「声音」，卡片改只读展示 name/voice/rate/pitch/instruction + 试听按钮（编辑入口移到 VoicesPage），各处只显示声音名字不露内部 ID。

## 变更记录

- 2026-09-18：初始决策（Go + cobra + SQLite；接口驱动；系列/集模型；并发上限 3、重试 3；百炼为首家 provider；ffmpeg 合成与硬字幕；默认 9:16）。
- 2026-09-18（实现修订）：因本机 ffmpeg 未编译 libass/freetype，硬字幕方案从 `subtitles` 滤镜改为 Go 渲染透明 PNG + 内置 `overlay` 叠加；新增 `internal/subtitle` 与配置项 `subtitle_font`。分辨率档位语义为「长边」：720P→720×1280，1080P→1080×1920。依赖固定 `modernc.org/sqlite v1.34.5` + `golang.org/x/image v0.20.0`，本机构建统一加 `GOTOOLCHAIN=local`（go 1.24.0，禁止自动下载新工具链）。
- 2026-09-18（Web UI）：新增 `internal/server`（标准库 net/http REST + SSE，复用 app 层）与 `web/`（Vite 8 / React 19 / TS / Tailwind v4），go:embed 单二进制，`story serve` 启动；§2「不引入 Web 框架」相应澄清为「不引入第三方 Web 框架」。
- 2026-09-19（AI 分集策划）：新增 `port.SeriesPlanner` 与策划会话（`plan_sessions` 表，1 系列 1 会话，模型每轮返回全量草案）；`bl text chat --output json --quiet` 直接打印正文无信封，`parseChatContent` 兼容两形态（记入 §2）。采纳只建集不生产，集数不设小上限（技术上限 100）。
- 2026-09-19（角色形象一致性）：两层方案落地（§12）：人物设定集 + 水墨工笔定妆照；新增 `port.ImageGenerator`、`ClipRequest.RefImages`（非空走 `bl video ref`）、series/plan_sessions `characters_json` 列（ensureColumn 幂等迁移）；Web 增系列级 SSE 与系列媒体端点；CLI 增 `story keyframes`。
- 2026-09-19（叙事与画风系统协调）：故事 prompt 改为百家讲坛式口播稿（钩子/起伏/讲述者声音，见 §7）；新增 `internal/templates/visualstyle.go` 统一视觉风格包（默认 gongbi 工笔重彩国风动画），visual_prompt 禁画风词、画风由 Brief+VideoAnchor+定妆照三处同源供给；engine 增 `normalizeDurations` 按旁白字数兜底校准镜头时长（6-10s）。
- 2026-09-19（成本红线）：开发/测试禁止真实调用 bl（§10），验证一律 mock；真实调用仅限用户明确要求的生产执行（当天百炼账户曾因频繁真实验证欠费，e04 produce 的 2 镜 TTS 因 Arrearage 失败）。
- 2026-09-19（小人书画面模式）：新增 `visual_mode`（comic 默认 / video 可选，见 §13）：comic 模式每镜一张 AI 插画 + 本地 ffmpeg Ken Burns 运镜（RenderStill/zoompan），后续合成管线零改动复用；新增 `panels/` 产物目录、camera 七词运镜约束与 motionForScene 映射轮换；CLI/Web/API 均有配置入口。
- 2026-09-20（故事生成即定稿）：取消多候选人工选择（见 §14）：故事 prompt 输出单篇定稿、generate 自动完成 pick、验收下限 3→1；CLI/run/server run 不再需要候选序号，Web 去除候选卡片，StepsBar 隐藏 pick；pick 链路保留兼容旧数据。同日把系列页画面模式升级为「系列设置」卡片（两个模式可点选），新建系列表单新增画面模式字段。
- 2026-09-20（画面模式锁定 + 系列页信息架构）：画面模式改为创建时锁定、系列创建后不可改（移除 `PUT /api/series/{id}/config`、`App.UpdateSeriesVisualMode` 与 Web 选择器，标题旁只读展示）；系列详情页重构：系列信息/AI 策划/定妆照改为 Collapsible 折叠（集列表始终展示），删除系列下沉页脚；新建一集、新建系列均改为 Modal 弹窗（ui.tsx 新增 Modal/Collapsible，Modal 支持 Esc/遮罩关闭与 wide 加宽）。
- 2026-09-20（视觉参考两级化，见 §15）：新增 `domain.VisualRef`（character/scene）与 `Episode.Refs`（episodes.refs_json 迁移）；分镜顺带零成本产出本集人物+重复场景文字约束，参考图改手动按张生成（集级 `POST /api/episodes/{id}/refs`、CLI `story episode-refs`，落集 refs/，场景空镜图走 SceneRefPrompt/SceneClause）；系列+集两级同名集级优先，重跑分镜保留已生成图；video 模式人物/场景参考图均喂 bl video ref；UI 全面改称「视觉参考」，集页新增本集视觉参考折叠区，系列页改名「系列视觉参考 · 人物」。
- 2026-09-20（声音库顶层实体化，见 §16）：声音从 `templates.VoicePresets` 硬编码提升为与 Series 同级的持久化实体（`domain.Voice` + `voices` 表）；`Series.VoiceID` 创建后锁定（store.UpdateSeries SQL 不含 voice_id 列）；Bootstrap 调 `SeedBuiltinVoices` 写 6 个内置条目 + `MigrateSeriesVoiceIDs` 平迁旧 series；engine `resolveVoice` 三级回退（voice_id 查表 → VoiceProfile 预设 → TTSVoice 裸读）；CLI 新增 `story voice` 子命令、`story series create --voice`；server 新增 `/api/voices/{id}` CRUD + `/api/voices/preview`；Web 新增 `/voices` 页与新建系列 Modal 声音下拉，系列详情页声音卡片改只读展示。
- 2026-09-20（声音供应商归属 + 多路径创建，见 §16 修订）：Voice/VoiceProfile 加 `Provider` 字段（`NormalizeVoiceProvider` 空/未知归一 bailian），voices 表加 provider 列（旧行默认 bailian）；新增 `port.VoiceLister`，bailian 解析 `bl speech synthesize --list-voices` 表格（26 音色，零费用）；server 加 `GET /api/voice-providers/{p}/voices`，CLI voice add/edit 加 `--provider`；VoicesPage 重写支持四种创建方式（手填 / 复制 / 浏览音色库挑选 / 试听后保存），新建系列声音下拉不变。
- 2026-09-20（内置声音名实相符，见 §16 修订）：用户反馈系统音色调不出名人味道，6 个内置条目去掉名人化名（王立群/凯叔/易中天/说书人风格等），ID/名称/特质全部回归音色真实身份（龙天·磁性理智男、龙泽·温暖元气男、龙橙·智慧青年男、龙飞·热血磁性男、龙浩·多情忧郁男、龙小夏·沉稳权威女），Instruction/Rate/Pitch 清空走默认；新增 `RemapLegacyBuiltinVoices` 一次性重映射 series.voice_id 并删除旧内置行（幂等）；UI 各处改显示名字、不露内部 ID，新建系列默认 longtian。
- 2026-09-20（百炼造声：声音设计 / 声音复刻，见 §16）：`bl` 无造声命令，bailian provider 直连百炼 HTTP customization 接口（§2 唯一例外）；新增 `port.VoiceBuilder`、`domain.VoiceBuildDesign/VoiceBuildClone`、`Voice.Model`（voices.model 列 + `port.SpeechRequest.Model`，造声音色必须用造声模型合成，engine resolveVoice 透传 model）；`app.BuildVoice` 造声后自动落库成声音条目；CLI 新增 `story voice design`/`clone`（voice add/edit 加 `--model`）；server 加 `POST /api/voices/design`、`POST /api/voices/clone`；Web VoicesPage 加「声音设计」「声音复刻」入口（BuildVoiceModal）与驱动模型字段；config 加 `bailian_api_key`/`bailian_base_url`（回退 DASHSCOPE_API_KEY 与 `~/.bailian/config.json`）；mock 加 `VoiceBuild`/`VoiceList`。造声按新建音色个数计费，受 §10 成本红线约束（开发/测试一律 mock）。
- 2026-09-21（造声供应商维度，见 §16 修订）：造声的 provider 从写死 bailian 改为可传入——`App.voiceBuilder` 单实例改为注册表 `voiceBuilders map[string]port.VoiceBuilder`（key=供应商，Bootstrap 登记 `bailian: bl`），`app.BuildVoice` 经新增 `resolveVoiceBuilder(provider)` 选实现（空值默认 bailian，**显式未知供应商报错**而非静默回退，`BuildVoiceInput.Provider` 同时写入新条目 `Provider`）；CLI `story voice design|clone --provider`、server 造声 body 加 `provider`、Web `BuildVoiceModal` 加供应商下拉。`voiceLister`（浏览音色库）保持单一字段未改注册表。
- 2026-09-21（参考音频来源三 Tab，见 §16）：Web 复刻表单去掉「服务器本地路径」输入，改为**上传音频文件 / 现场录音 / 音频 URL** 三个平级 Tab，录音页给一段可编辑的推荐朗读文本（`CLONE_SAMPLE_TEXT`）。因 `MediaRecorder` 输出 webm/opus（Safari 为 mp4）供应商不认，新增小端口 `port.AudioNormalizer`（ffmpeg 实现 `-vn -ac 1 -ar 16000 -c:a pcm_s16le`）与 `App.SaveVoiceSample`（落 `os.TempDir()/story-voice-samples/`，时长 <3s 报错，`sanitizeAudioExt` 防路径穿越），新增 `POST /api/voices/audio`（multipart，25MB 上限，返回 `{path, duration_sec}`）；前端两段式提交：先上传归一化拿 path，再调既有 `POST /api/voices/clone`（**克隆那一步才计费**）。CLI `story voice clone --audio` 保持不变。
- 2026-09-21（失败镜头续跑与手动停止，见 §4 修订）：hanshu-e01 曾出现 26 镜中 10 镜因 `bl image generate` 内部 300s headers 超时（`UND_ERR_HEADERS_TIMEOUT`）失败。新增：① `Engine.produce` 改为先规划后建任务（`planScenes`/`selectScenes`），**只为未完成镜头建任务**，已完成镜头零费用跳过，故重跑 produce 即「只重试失败镜头」；② `Engine.ProduceScenes(episodeID, sceneIDs)` 支持指定镜头（CLI `story produce --scenes`、server `actionReq.Scenes`、Web 单镜「重试本镜」），越界序号报错；③ 同镜画面/旁白解耦（`produceClip`/`produceAudio` 各写各的结果槽，画面失败也合成旁白）；④ broker 增加取消能力（`cancels map[string]context.CancelFunc`、`jobCanceled` 状态）与新端点 `POST /api/episodes/{id}/cancel`，取消时保留已落盘进度并标注「已手动停止」；⑤ Web 集页新增未完成镜头汇总横幅（列出每个未完成镜头的画面/旁白错误）、produce 按钮动态文案「③ 仅重试失败镜头（N 镜）」、单镜「重试本镜」、运行中「停止」按钮与 canceled 提示；⑥ 未加 `--retry-failed` 冗余 flag（默认 produce 语义已等价）。engine 新增 `TestProduceSkipsCompletedScenes`/`...ClipFailureStillSynthesizesAudio`/`...ScenesSubset`/`...CanceledKeepsProgress`，server 新增 `TestCancelAction`，成本红线内全程 mock。
