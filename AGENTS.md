# AGENTS.md — 项目决策记录（所有 Agent 必须遵守）

本文件记录本项目（**story**）不可轻易违背的技术决策。任何会话中的 Agent 在写代码前必须先读完本文件；新的决策在获得用户确认后追加到文末。

## 1. 项目目标（2026-09-21 立项调整：通用 AI 视频流水线）

- 定位：**一套通用的「AI 视频制作流水线」工具箱**——给一个题材/主题，经「故事/脚本 → 分镜 → 画面 + 旁白 → 成片」四个阶段，产出带旁白、带硬字幕的 MP4 短视频，发布到抖音（中视频）、快手、B站、小红书、视频号等平台。**不局限于历史故事**：中国历史故事只是内置的第一套题材模板，不是本项目的边界。
- 组织方式：内容按**系列（Series）→ 多集（Episode）**管理（系列＝一个栏目/主题，如「鬼谷子」系列；集＝一期）；一集是**一棵版本树**（见 §4、§17），任一阶段都可换一版、旧分支完整保留。
- 生产方式：一条多阶段、可人工介入的流水线；默认由 Agent 自动驱动，人可在任意阶段检查产物并决策；CLI 与 Web 双入口共用同一个 `app.App`（§11）。
- **题材解耦铁律**：流水线骨架与题材无关。`engine`/`port`/`domain`/`store`/`subtitle`/`provider`/`server` 与 Web 各层**不得写死题材假设、不得按题材做条件分支**；题材相关的一切只允许出现在两处——
  1. 系列配置字段（`dynasty`、`video_style`、`visual_mode`、声音、创作控制参数等，取值语义由题材模板定义；新增风格维度的插件化通道见 §18）；
  2. `internal/templates`（prompt 模板 + 题材/画风视觉锚定包）。
  新增一个题材＝在 `templates` 新增模板 + 用系列配置取值，**engine 与 CLI 零改动**（与 §3「换供应商零改动」同款约束）。当前仓库里只有「中国历史故事」这一套模板（§7），它是该约束下的第一个实现，不是特例。

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
internal/engine  ── 版本树派生、调度、验收、并发/重试
      │ 只依赖
internal/port    ── 全部接口定义（StoryGenerator / StoryboardPlanner /
      │              SeriesPlanner / VideoGenerator / ImageGenerator /
      │              SpeechSynthesizer / VoiceLister / VoiceBuilder /
      │              AudioNormalizer / VideoComposer / TaskPoller / Repository）
      ▲
      │ 实现
internal/provider/bailian  （bl：故事/分镜/策划/视频/图片/语音/造声/任务）
internal/provider/ffmpeg   （插画渲染/音频归一/拼接/字幕/转码/多比例导出）
internal/store/sqlite      （Repository 的 SQLite 实现）
```

硬性规则：

1. `internal/engine`、`internal/domain`、`internal/port` **禁止 import** `provider/*`、`store/sqlite` 等任何具体实现；依赖装配只允许发生在 `internal/app` 与 `cmd/story`。
2. `internal/domain` 是纯数据层（struct + JSON tag + 少量纯函数），不依赖任何第三方包。
3. 替换 AI 供应商（如未来接入其他厂商）时，只新增一个 provider 实现，**engine 与 CLI 逻辑零改动**。
4. 所有外部调用接口方法的第一个参数都是 `context.Context`，并返回 error。

## 4. 流水线与版本树

每一集（Episode）是**一棵版本树容器**（2026-09-21 改造，见 §17）：四个阶段依次派生，每个阶段可存在多份版本节点，节点间靠派生键（内容寻址）串成链；从任意节点都能继续往下派生出一条新的成片，旧分支完整保留。

阶段（`domain.Stage`）：

1. `story`      — AI **直接生成一篇定稿故事**（标题、朝代、出处、梗概、正文——这是当前历史题材模板的字段，见 §7；换题材即换模板），生成即定稿（2026-09-20 起取消「3 个候选人工选择」，见 §14）；不满意可「换一版」。
2. `storyboard` — AI 将故事拆为 18–26 个镜头（visual_prompt / narration / duration / camera；校验下限 4、上限 28），并顺带产出集级视觉参考文字约束（§15）。
3. `media`      — 按系列设置 `visual_mode` 生产画面并合成旁白；产物落盘，支持断点续跑（已存在的片段默认跳过）。两种模式（见 §13）：`comic`（默认，小人书：每镜一张 AI 插画 → 本地 ffmpeg Ken Burns 渲染片段，仅按图片计费）/ `video`（每镜 AI 视频生成）。
   - **只重试未完成的镜头（2026-09-21）**：`Produce` 先按磁盘产物规划（`planScenes`），**只为「画面或旁白尚未产出」的镜头建任务**，已完成的镜头连任务都不建、零费用；因此直接重跑 `produce` 等价于「只重试失败镜头」。指定镜头用 `DeriveOptions.Scenes`（CLI `story produce --scenes 13,15`、server `{"action":"produce","scenes":[13,15]}`、Web 单镜「重试本镜」）；序号不存在直接报错。
   - **画面与旁白互相独立**：同一镜内画面失败也照常合成旁白，各自登记错误（`Err` 前缀「画面:」/「旁白:」），避免下次重试把已付费的那一项再跑一遍。
   - **可手动停止**：Web 运行中「停止」→ `POST /api/episodes/{id}/cancel` → broker 取消该集 job 的 context（kill 正在跑的 bl/ffmpeg 子进程）；已完成的产物全部落盘保留，之后再次执行即从断点续跑。
   - 任务成功但整集仍有未完成镜头时（如只点了单镜重试）：不返回错误，仅把节点标记 `failed` 并列出剩余镜头，便于继续续跑。
4. `final`      — ffmpeg 归一化 → 音视频合成 → 拼接 → 烧录硬字幕，产出最终 MP4；多比例导出（`Export`）产物追加到该 final 节点的 `Outputs`。

节点状态：`pending → running → done`，异常分支 `failed`（可重试）；`Runs` 记录本节点被执行次数（原 `StepState.Attempts`）。人工可随时查看各版本目录中的 `story.md`、`storyboard.json` 介入。

每一步必须有**自动验收标准**，验收不通过标记 `failed` 并写入错误信息：

| 阶段 | 验收条件 |
| --- | --- |
| story | 定稿故事 1 篇，含非空标题、出处、正文 |
| storyboard | 镜头 ≥4，每个含非空 visual_prompt / narration，duration 合法 |
| media | 每个镜头的视频与音频文件存在且非空、可被 ffprobe 解析 |
| final | 最终文件存在、可播放、时长 ≈ 各镜头之和 |

## 5. 并发与重试

- 多镜头生产使用带缓冲信号量限流，最大并发数取自系列配置（默认 3）。
- 失败自动重试 **3 次**，指数退避（2s、4s、8s）；仍失败则该步骤 `failed`，等待 Agent/人决策。
- 实现集中在 `internal/engine/runner.go`，业务代码不得自行开 goroutine 池或写重试循环。

## 6. 持久化与目录约定

- 结构化状态全部存 SQLite，经由 `port.Repository` 访问；engine 不知道 SQLite 的存在。
- 媒体与中间产物是普通文件，根目录 `data/projects/<series-id>/<episode-id>/`：
  - `versions/<节点 ID>/`（2026-09-21 起，见 §17）：每个版本节点的产物目录，按阶段分别落 `story.md`、`storyboard.json`、`panels/`+`clips/`+`audio/`、`tmp/`+`output/`；`attempt > 0` 的版本目录追加 `-a<n>` 后缀。
  - `refs/`：集级视觉参考图，跨分镜版本共享，不随版本搬动。
- `story.md`、`storyboard.json` 同时在各版本目录内落一份副本，专供人工审阅；**事实源以数据库为准**。
- `data/` 不入库。

## 7. 题材模板与 Prompt 约束（当前模板：中国历史故事）

- **分层**：本节分两类规则——**通用约束**（任何题材模板都必须遵守）与**【历史模板】**（当前这套中国历史故事模板的内容规则）。换题材＝在 `internal/templates` 换后者，前者不动（§1 题材解耦铁律）。
- 通用 · 纯 JSON：所有 LLM 输出一律要求**纯 JSON**，代码侧去除 ``` 代码围栏后解析，并做字段校验。
- 通用 · 画风统一（2026-09-19 修订）：`visual_prompt` 只写画面内容，**严禁出现任何画风/质感/媒介词**（电影感、写实、真人、动漫、3D、卡通、照片、水彩、构图、质感等）。画风由 `internal/templates/visualstyle.go` 的风格包唯一供给：分镜 prompt 注入 Brief（约束 LLM）+ engine `buildVideoPrompt` 末尾固定追加 VideoAnchor（代码双保险，不信任模型）+ 视觉参考图 KeyframePrompt 同源。风格 key 存系列配置 `video_style`（`gongbi` 工笔重彩国风动画＝默认、`realistic` 写实真人、`ink` 水墨写意），空值与未知值回退 gongbi。
- 通用 · 时长兜底：镜头时长除 prompt 约束（6-12 秒、duration ≥ 旁白字数÷4.5）外，engine `normalizeDurations` 按旁白字数代码侧兜底上调（只上调、不下调，封顶 `maxSceneDur`=12s），防止旁白长于画面导致成片定格拉伸。
- 【历史模板】叙事定位：百家讲坛式**口播讲述稿**——故事为主、画面为辅；开头两句必须是钩子（反常细节/悬念/设问）；结构按「钩子 → 极简处境 → 冲突加压（至少三层）→ 高潮（关键动作慢写）→ 转折 → 结尾留白或留钩子」，**严禁说理/点题式收束**；讲述者声音在场，设问/卖关子/感慨合计**不超过一成**，且禁止「问题来了」「这故事最狠的地方不在……而在……」「真正……的是……」这类评论家口吻公式化短语。禁止生平流水账开头与现代腔。
- 【历史模板 · 加工铁律（2026-09-21）】典籍是原料，不是稿子：必须完成三层加工——**选人**（全片只跟一个人的处境走，不做群像平铺/事件清单）、**重排**（按「悬念—压力—代价」打散史料顺序，可后果前置、可跨年并置对撞）、**还原处境**（用能被画面拍到的动作与环境写他如何被一寸寸逼到岔路口）。严禁按史料顺序复述记载（那是翻译史料）。加工只改叙事结构，**不得新增史实**。
- 【历史模板 · 文言比例铁律（2026-09-21）】正文以白话为体：典籍原话引用全片**不超过两处**、每处只留一句短句（≤15 字）且先用白话把意思讲透；史书冷账句（如「徒多道亡」「自度比至皆亡之」）必须白话转述；**禁止连续文言对白**，人物对话一律白话转述，只在最有力处保留一两个短促原话。
- 【历史模板 · 情感落点（2026-09-21）】每集必须有一个让成年观众「心里一沉」的落点——一个怎么算都不划算却不得不做的选择、一句说出口就收不回的话、一个眼看要好转却拐进深渊的拐点；用具体动作或具体物什砸出来，砸完就停、不解释。触动来自「换成我也难」，这是评论互动的来源。
- 【历史模板】出处：必须标注具体典籍篇目，**严禁无依据地杜撰重大史实**，关键对话/情节须出自所标典籍；出处必须融入讲述本身（绝不单独成句、严禁「这话出自《XX》」等元讲述句）。
- 【历史模板 · 分镜忠实切分（2026-09-21）】分镜阶段的 `narration` 必须**逐字沿用讲述稿原文**（按口播节奏切句，只允许在镜头衔接处增删一两个顺承词）：严禁改写措辞、缩写、扩写、另起炉灶重写；严禁把讲述稿已白话化的对话改回文言、严禁自行新增文言引语。
- 【历史模板】`visual_prompt` 必须经朝代视觉锚定：服饰、建筑、器物、配色、席居方式等符合该朝代生产力水平（`internal/templates/dynasty.go`）。

## 8. 配音合规

- 声音与题材无关（§1）：声音是顶层实体（§16），按 `series.voice_id` 取用，**创建系列时选定后锁定**；新建系列默认内置条目 `longtian`（音色 `longtian_v3`，磁性理智男），内置条目的 Instruction/Rate/Pitch 一律留空、走音色默认。
- **实测限制（2026-09-19）**：`longtian_v3 + cosyvoice-v3-flash` 组合不支持 `--instruction` 风格指令（引擎报 428 InvalidParameter）。provider 的 `Synthesize` 已做自动降级：带指令失败后去掉指令重试一次。品牌声线靠音色本身的「磁性理智男」特质承载；后续如需风格控制可评估 `--rate/--pitch` 参数，或自建造声音色（§16）。
- **禁止**克隆他人真人声音用于商业发布（声音权法律风险）；声音复刻能力不得作为默认路径。

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

- 定位：流水线各阶段产物（版本树 / 故事 / 分镜 / 片段与旁白 / 成片 / 视觉参考 / 声音）的**预览 + 操作**界面，与 CLI 共用同一个 `app.App` 容器，不复制业务逻辑。
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
- 运镜幅度（2026-09-21 修订，用户反馈「动的幅度太小、看不出变化」）：推近/拉远 `stillZoom`=1.30（全程放大三成），平移 `panZoom`=1.24 使视窗横移约画面的四分之一宽度（旧值统一 1.12，行程仅约 12% 画宽）。静帧画面唯一的运动来源就是运镜，幅度须肉眼可辨。
- 人物一致性：comic v1 靠分镜 prompt 的 appearance 逐字复制（§12 第一层）；`bl image generate` 不支持参考图，后续如需更强一致性可验证 `bl image edit --image 视觉参考图`（真实验证一次即可，禁止批量烧钱）。
- 已用 video 模式产出的片段不受模式切换影响（续跑只看文件是否存在）；新模式只影响之后新生产的镜头。

## 14. 故事生成即定稿，取消候选选择（2026-09-20 增补）

- 决策：取消「generate 生成 3 个候选 → 用户 pick」的人工选择环节（用户反馈选择困难，且多候选白白消耗 token）。`generate` 只产出**一篇定稿口播稿**，写入 Story 与 story.md 审阅副本，下一步直接 storyboard。不满意可重跑（现为「换一版」，见 §17）。
- **2026-09-21 版本树改造后**：pick 链路已彻底移除——`StepPick`/`Candidates`/`Selected`/`Engine.Pick`/CLI `story pick`/server `pick` 动作全部删除；历史上停在 generate 与 pick 之间的旧数据由一次性迁移覆盖（取 `Candidates[Selected-1]` 作为 story 节点内容，见 §17）。`StoryRequest.Count` 废弃，provider 忽略。
- 模型契约：故事 prompt 输出从 `{"candidates":[...]}` 改为单对象 `{title,dynasty,source,summary,content}`（bailian `storyResponse`），provider 包装为单元素切片返回；验收下限 `minCandidates` 3→1。
- 一键流程：CLI `story run` 与 server `run` 动作从指定节点沿链往下补齐，不再需要 index 参数。

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

## 17. 集级版本树（2026-09-21 增补，用户拍板）

- **背景（要消除的隐患）**：旧模型下一集是一条线（`PipelineState` 只有单个 `Current` + 一张 `Steps` map，产物平铺在同一个 `state_json`），落盘产物按镜头序号命名（`clips/scene-01.mp4`），"是否已完成"只看文件是否存在（`reusable`）。于是**重跑上游不做失效**：重跑 `storyboard` 只覆盖 `Storyboard`，既不清理 `Clips`/`Audios`、磁盘旧片段也还在，再点生产时 `planScenes` 见到 `scene-01.mp4` 就判定已完成并复用——结果**新分镜的旁白文本配上旧旁白音频与旧画面**，`compose` 还会把它当正常成片合成出来，用户察觉不到。
- **模型**：Episode 成为**版本树容器**（`domain.Episode{ID,SeriesID,Number,Title,Topic,Refs,Nodes,ActiveNodeID,WorkDir,...}`，`internal/domain/episode.go`），下面挂 0..N 条「story→storyboard→media→final」版本链。`domain.State`/`state.go`（`StepName`/`StepStatus`/`StepState`/`PipelineState`/`AllSteps`/`NewPipelineState`）**已删除**。
- **派生键（内容寻址）**：`domain.VersionNode.ID` = `sha1(derivationSchemaVersion | stage | parentID | paramsJSON | attempt)` 取前 12 位，前缀 stage，形如 `storyboard-3f9a2c1b7d4e`（`internal/engine/derive.go` 的 `nodeKey`）。**只哈希输入不哈希输出**（bl 输出不确定，无法用产物内容定身份）。params 按阶段取：
  | 阶段 | 派生输入 |
  | --- | --- |
  | story | `series_id` + `topic` + `dynasty` |
  | storyboard | `parentKey` + `dynasty` + `ratio` + `resolution` + `video_style` + `refs` 摘要 |
  | media | `parentKey` + `visual_mode` + `video_style` + `ratio` + `resolution` + `voice_id` + 声音参数摘要 |
  | final | `parentKey` + `ratio` + `resolution` + `burn_subtitles`（**故意不含** `subtitle_font`，换字体只重烧字幕不必换 final 版本） |
- **`derivationSchemaVersion`（当前 =1）**：派生规则版本常量，prompt 模板/参数语义/产物布局发生不向后兼容变化时**递增**，一次性让全部旧派生键失效（旧产物不再被复用），等价于"一次性安全失效"。
- **`attempt` 语义**：同一组派生输入下的第 n 次尝试（0 起）。`DeriveOptions.Reroll=false` 且同键节点已存在 → **复用**（节点已 done 则零模型调用，未完成则续跑——现有"断点续跑/只重试失败镜头"语义天然保留）；`Reroll=true` → `attempt = Episode.MaxAttempt(parentID, stage) + 1`，得到新节点与新目录，**旧版本原地保留**。`Runs` 记录本节点被执行次数（失败重试与续跑累加）。
- **派生 API**：`DeriveOptions{From, Reroll, Scenes}`——`From` 为起始父节点 ID（空 = 用 `ActiveNodeID`）；父节点解析 `resolveParent(ep, from, want)`：`From` 非空时严格按它取（**阶段不符即报错**），为空时取活跃路径上对应阶段的节点（取不到则报错如「请先生成故事」）。方法签名：`GenerateStory/PlanStoryboard/Produce(ctx, epID, opts)`、`Compose(ctx, epID, opts) (string, error)`、`Export(ctx, epID, ratio, opts)`、`Run(ctx, epID, opts)`、`ActivateNode(ctx, epID, nodeID)`、`DeleteNode(ctx, epID, nodeID)`。`Produce` 额外返回 `*VersionNode`（便于 `Run` 串联）。`GenerateStory` 忽略 `From`（永远是根）。
- **目录布局**：`data/projects/<series-id>/<episode-id>/versions/<节点 ID>/`，`attempt > 0` 的目录追加 `-a<n>`；各阶段分别落 `story.md` / `storyboard.json` / `panels/`+`clips/`+`audio/` / `tmp/`+`output/`。`refs/` 仍留在集目录（**集级共享，永不随版本搬动**）。上游一变目录名就变，旧目录不会被误读——这是本次改造要消除的核心隐患。`planScenes`/`produceClip`/`produceAudio` 的复用判定逻辑**完全不变**，只是路径换成节点目录（§4 的断点续跑、画面/旁白解耦、只重试失败镜头、取消保留进度全部保留）。
- **持久化**：episodes 表经 `ensureColumn` 幂等加 `nodes_json TEXT NOT NULL DEFAULT '[]'` 与 `active_node_id TEXT NOT NULL DEFAULT ''`；`state_json` **保留但迁移后不再读写**（只作迁移前的只读历史快照，便于人工恢复）。`Episode.Nodes` 在 `NewEpisode` 中初始化为空切片（非 nil），保证 JSON 为 `[]` 而非 `null`。
- **旧集一次性迁移**（`internal/app/migrate_versions.go`，Bootstrap 中 `Open` 之后跑，幂等：`nodes_json IN ('','[]') AND active_node_id = ''` 直接跳过）：把旧 `state_json` 投影成一条线性链——`State.Story` 非空 → story 根节点；**停在 pick**（`Story` 空但 `Candidates`+`Selected` 有值）→ story 节点取 `Candidates[Selected-1]`；`Storyboard`/`Clips`+`Audios`/`Outputs` 非空依次挂 storyboard/media/final 节点；`ActiveNodeID` = 最深的已完成节点。链构造由 engine 的 `DeriveLegacyChain` 负责（**必须走同一套 `resolveVoice`/派生规则**，否则旧集续跑会全部重做、重复计费），app 只负责旧 JSON 解析与文件搬运。**先落库元数据 → 再 rename 文件 → 失败（跨设备等）回退该节点 `Dir` 指向旧路径并保留原文件 → 最终统一 rebase 登记路径并二次落库**（rebase 始终发生，故循环结束后必须无条件 `SaveEpisode`）。`refs/` 永不搬动。
- **节点删除语义**（`Engine.DeleteNode`）：级联删除该节点及其全部后代（`domain.RemoveSubtree`），逐个 `os.RemoveAll(node.Dir)` 删媒体文件，再写回 `ep.Nodes`；若删的是 `ActiveNodeID`，活跃指针改指其父节点。**不可恢复，Web 侧二次确认**。`ActivateNode` 只改 `ActiveNodeID` 并落库。
- **入口**：
  - CLI（`cmd/story/step.go`）：通用 flag `--from <node-id>`、`--reroll`（挂 generate/storyboard/produce/compose/run/export）；`story pick` **已删除**；新增 `story nodes <episode-id>`（打印版本树）、`story activate <episode-id> <node-id>`、`story node-rm <episode-id> <node-id>`。
  - server：`actionReq` 加 `From`/`Reroll`、**删 `Index`**，`buildAction` 动作名改 `story`（`pick` 移除）；新增两个**同步**端点（零费用、不经 broker）：`POST /api/episodes/{id}/nodes/{nodeID}/activate`、`DELETE /api/episodes/{id}/nodes/{nodeID}`（成功后 `broker.publish(id, evSnapshot, ep)`）。`GET /api/episodes/{id}`、SSE 快照、`serveMedia` 路径穿越防护**无需改动**（版本目录仍在 `WorkDir` 内）。
  - Web：`types.ts` 以 `Stage`/`NodeStatus`/`VersionNode` 取代 `PipelineState`/`StepState`/`StepName`/`StepStatus`，`Episode` 改 `nodes`/`active_node_id`；`api.ts` action body 加 `from`/`reroll` 并新增 `activateNode`/`deleteNode`；新增 `components/VersionTree.tsx`（**不引入图库**，纯 SVG + Tailwind 手绘：4 列 × 版本行、父→子贝塞尔连线、活跃路径金色高亮、选中卡片展开「从此处继续 / 换一版 / 设为活跃 / 删除」）；`EpisodePage` 改为「版本树 + 工具栏 / 选中节点产物详情（按 stage 四选一）/ 未完成镜头横幅 + 本集视觉参考」三段式；`StepsBar.tsx` **已删除**；`SeriesDetailPage` 集进度改用活跃路径。
- **工具条语义**：Web 工具栏一律**从活跃节点继续**（不传 `From`，由引擎按活跃路径解析各步骤所需父节点）；「换一版」由版本树卡片发起（`from = 该节点父节点`、`reroll = true`）；未完成镜头汇总只统计**活跃** media 节点（与 `produce` 默认作用对象一致）。
- **回归测试**（成本红线内全程 mock）：`TestRerollStoryboardIsolatesOldClips`（核心：换分镜后旧 clips 绝不被复用、旧目录文件仍在、视频调用 4→8 且无 Skipped）、`TestSameDerivationReusesNode`（同输入复用同节点、模型零调用）、`TestProduceResumeAfterReroll`、`TestActivateNode`、`TestDeleteNodeRemovesFiles`（级联 + 活跃指针回退父节点）、`TestMigrateLegacyEpisode`（含停在 pick 的中间态、文件搬入版本目录、登记路径改挂、幂等）、`TestLegacyChainReusesMigratedNodes`（迁移后派生键一致：续跑不新建节点、不重拆分镜、只补缺镜）、`TestActivateAndDeleteNodeHTTP`、`TestBuildActionWithFromAndReroll`。

## 18. 创作控制参数：插件化旋钮 + 预设（2026-09-21 增补，用户拍板）

- **背景**：项目从「历史故事」上调为通用视频流水线（§1）后，同一套流水线要服务完全不同的风格诉求（纪录片口吻 / 儿童向 / 悬疑倒叙 / 换画风 / 改运镜幅度 / 一次性自由要求）。若把「叙事风格」这类字段一路硬编码进 `SeriesConfig`、engine、CLI、前端，每加一个维度就要改四层——违背 §1 题材解耦铁律的精神。
- **决策：声明式注册表做唯一知识源**。全部参数与预设声明在 `internal/templates/creative.go`：
  - `Knob{Key,Label,Help,DefaultLabel,Type,Options,MaxLength,Get,Set}`——**关键在 `Get`/`Set func(domain.SeriesConfig)`**：app / engine / CLI / server 读写参数时只调这两个闭包，**不知道任何具体参数名**；新增一个参数＝在本文件加一个 Knob，其余各层零改动。`MaxLength` 也只是「文本型参数的长度上限」，由 catalog 下发给前端，避免前端复制 Go 常量（如 500）。
  - `KnobType` 两类：`KnobEnum`（值必须在 `Options` 里，未知值回落「未设置」）与 `KnobText`（自由文本，`ClipInstruction` 统一截断到 `MaxInstructionLen`）。
  - `Option.Story`/`Option.Board` 是**注入 prompt 的片段**（分阶段：故事 / 分镜各自一份，可空＝该阶段不受影响）。画风选项由 `styleOptions()` 从 `VisualStyles` 生成，避免两处维护。
  - 预设 `Preset{Key,Name,Desc,Values map[string]string}`：一键套用一组值，套完仍可逐项微调。**默认预设 `classic` 的 Values 必须为空**，且 `ApplyPreset` 对默认预设不落 `Creative.Preset` key——保证「一键套用默认 == 与历史行为逐字一致」。
- **当前 6 个参数**：`narrative`（纪录客观 / 当事人自述 / 悬疑倒叙）、`audience`（青少年 / 儿童）、`length`（短篇 800-1200 字·16-20 镜 / 长篇 1800-2400 字·24-28 镜）、`motion`（强 / 弱，小人书运镜幅度）、`video_style`（画风，选项由风格包生成）、`instruction`（自定义创作指令，文本）。4 个预设：`classic`（默认，空）、`documentary`、`kids`、`suspense`。
  - **`instruction` 也是 Knob**（而非 knob 外的独立字段）：文本输入走同一条插件通道，API/CLI/前端的语义与校验完全一致，且天然获得补丁语义。**集级「本集附加指令」不占 Knob**——它是 `Episode.Instruction`（每集一个值，非系列配置），在 `buildBrief` 中单独成行叠加在系列级之上。
- **brief 组装**：`StoryBrief(cfg, epInstruction)` / `BoardBrief(cfg, epInstruction)` 把「非空」的参数片段 + 系列级指令 + 本集附加指令拼成【创作要求】段，末尾固定附【冲突声明】——明确「只调整口味与体量，不得违反系统提示中的硬性规则（文言比例、不得编造史实、narration 逐字沿用、visual_prompt 禁画风词等），冲突时以硬性规则为准」。**全零值时返回空串**，调用方据此跳过注入。
- **三条硬约束（写在文件头注释里，改动前必读）**：
  1. **默认值一律空串**。这些值经 `StoryBrief`/`BoardBrief` 进 prompt，而 `brief` 字符串本身进派生键（§17）——engine `derive.go` 用 `json.Marshal(params)` 求 sha1，默认值一旦非空，**存量系列的派生键立刻全变**、下游被误判失效而重复调用付费模型。
  2. 全部零值时 brief 必须返回 `""`（见上）。
  3. brief 只能覆盖口味，不得推翻硬性规则（靠【冲突声明】兜住）。
- **派生键与持久化**：
  - `domain.CreativeStyle{Narrative,Audience,Length,Motion,Instruction,Preset}`（`internal/domain/creative.go`，带 `IsZero()`），以 `SeriesConfig.Creative` 落库。**Go 1.24 的 `omitempty` 对结构体无效**，故该字段用 `omitzero` + `IsZero()`——零值时 `config_json` 里不出现，旧数据与新建默认系列字节级一致。
  - 派生参数新增 `storyParams.Brief` / `storyboardParams.Brief` / `mediaParams.Motion`，**一律 `omitempty`**，默认空串 → 派生键不变（回归测试 `engine/creative_test.go` 与 `legacyNodeKey` 做字节级比对）。
  - `Episode.Instruction`（`episodes.instruction` 列，ensureColumn 幂等），只影响该集的故事与分镜 prompt。
- **取值链**：`templates.StoryBrief`/`BoardBrief` → `port.StoryRequest.Brief` / `port.StoryboardRequest.Brief`（bailian provider 透传进 `UserPrompt`）→ `templates.StoryboardSystemPrompt` 之外的【创作要求】段。运镜：`mediaParams.Motion` → `port.StillRequest.MotionStrength` → `ffmpeg/still.go` 的 `motionZooms`（标准 `stillZoom`=1.30/`panZoom`=1.24，strong=1.42/1.34，subtle=1.16/1.12）。
- **入口**：
  - 装配：`app.ExpandCreative(preset, knobs) (CreativeStyle, videoStyle, error)`（创建时展开）与 `app.UpdateSeriesCreative(seriesID, preset, knobs)`（**补丁语义**：未出现在 knobs 里的参数保持原值，显式空串＝清除该项回到内置默认；不动 voice_id / visual_mode，二者创建后锁定，§13/§16）。
  - server：`GET /api/creative-catalog`（返回 `Catalog()`，**前端据此渲染控件，不硬编码任何参数名/选项/长度上限**）与 `PUT /api/series/{id}/creative`；`POST /api/series` body 加 `preset` + `creative`（`{knobKey: value}`，**有意不做顶层 `video_style` 字段**——画风是其中一个 knob，统一走 creative map 避免双通道）；`POST /api/series/{id}/episodes` body 加 `instruction`。`decodeBody` 用 `DisallowUnknownFields`，故未知 knob key 在 handler 先经 `validateCreative` 拦成 400 并列出支持项。
  - CLI：`story series create` / `story series set <id>` 共用 4 个 flag——`--preset`、可重复的 `--creative key=value`（**天然插件化，新增参数不必加 flag**）、`--story-instruction`（`instruction` 的语法糖）、`--video-style`（`video_style` 的语法糖）；`story episode create --instruction`。`printCreative` 只打印已显式设置的项（全空打印「创作设置: 全部跟随内置默认」），并把溯源渲染成「创作设置（预设「悬疑倒叙」）」或「（预设「悬疑倒叙」基础上微调）」。
  - Web：见 §11 / 变更记录（`CreativeFields` 组件由 catalog 驱动渲染）。
- **回归测试**：`internal/templates/creative_test.go`（注册表不变式：每个 Knob 有 `DefaultLabel`、text 型可无 Options、默认预设 values 为空、`ApplyKnobs` 未知 key 报错）、`internal/engine/creative_test.go`（默认空 brief 时派生键与旧版逐字节相同）、`internal/server/server_test.go`（catalog 结构、创建时预设展开+微调覆盖、PUT 补丁语义、显式空串清除、未知 knob/预设 400、集级指令落库）。全程 mock，成本红线内。

## 19. 多平台发布系统（2026-09-21 增补，用户拍板）

- **定位**：流水线的第 5 阶段——成片（final）合成完毕后，将视频与配套素材发布到各短视频/中视频平台。**不替代平台的审核与推荐**，只负责「素材准备 → 上传 → 状态追踪」的自动化。
- **架构原则**：与现有 4 阶段同款 Ports & Adapters——`port.PlatformPublisher` 接口定义发布能力，各平台实现为独立 provider（`internal/provider/publish/<platform>/`），engine/app 不知道具体平台；发布任务（`PublishJob`）是新的顶层持久化实体，通过 `port.Repository` 读写。

### 19.1 支持平台与优先级

全部平台**一次性并行接入**（用户拍板）：

| 平台 | Provider Key | API 接入方式 | 认证方式 |
|------|-------------|-------------|---------|
| 抖音 | `douyin` | [开放平台](https://developer.open-douyin.com/) | OAuth2（Client Token + User Token） |
| 快手 | `kuaishou` | [开放平台](https://open.kuaishou.com/) | OAuth2 |
| B站 | `bilibili` | [投稿 API](https://member.bilibili.com/) | OAuth2 + SESSDATA Cookie |
| 小红书 | `xiaohongshu` | 暂无官方开放 API | 第三方服务 / 半自动（系统备素材 + 跳转上传页） |
| 视频号 | `weixin` | [微信开放平台](https://developers.weixin.qq.com/) | OAuth2 + 视频号助手 |

> **小红书特殊处理**：因无官方视频发布 API，采用「半自动」模式——系统准备好所有素材（封面、标题、标签、描述），Web 端一键跳转到小红书创作者页面并剪贴板填充标题/描述；后续如出现可用 API 或第三方服务可升级为全自动。

### 19.2 数据模型

#### domain 实体

```go
// internal/domain/publish.go

type Platform string  // "douyin" | "kuaishou" | "bilibili" | "xiaohongshu" | "weixin"

type PublishStatus string
const (
    PublishPending   PublishStatus = "pending"    // 待发布
    PublishUploading PublishStatus = "uploading"  // 上传中
    PublishUploaded  PublishStatus = "uploaded"   // 已上传，待确认/发布
    PublishPublished PublishStatus = "published"  // 已发布
    PublishFailed    PublishStatus = "failed"     // 发布失败（可重试）
    PublishRejected  PublishStatus = "rejected"   // 平台审核拒绝（不可重试，需改内容）
    PublishCanceled  PublishStatus = "canceled"   // 已取消
)

type PublishJob struct {
    ID          string        `json:"id"`
    EpisodeID   string        `json:"episode_id"`
    SeriesID    string        `json:"series_id"`
    Platform    Platform      `json:"platform"`
    NodeID      string        `json:"node_id"`         // 发布的 final 节点 ID
    Status      PublishStatus `json:"status"`
    // 素材
    VideoPath   string        `json:"video_path"`
    CoverPath   string        `json:"cover_path,omitempty"`
    Title       string        `json:"title"`
    Description string        `json:"description"`
    Tags        []string      `json:"tags,omitempty"`
    Category    string        `json:"category,omitempty"`
    // 平台返回
    PlatformVideoID string `json:"platform_video_id,omitempty"`
    PlatformURL     string `json:"platform_url,omitempty"`
    // 定时发布
    ScheduledAt *time.Time `json:"scheduled_at,omitempty"`
    // 重试
    Attempts   int    `json:"attempts"`
    MaxRetries int    `json:"max_retries"`
    Error      string `json:"error,omitempty"`
    // 时间
    CreatedAt  time.Time `json:"created_at"`
    UpdatedAt  time.Time `json:"updated_at"`
}
```

#### 平台账号凭证

```go
// internal/domain/platform_account.go

type PlatformAccount struct {
    ID           string    `json:"id"`
    Platform     Platform  `json:"platform"`
    AccountName  string    `json:"account_name"`   // 平台侧用户名/昵称
    AccountID    string    `json:"account_id"`     // 平台侧用户 ID
    AccessToken  string    `json:"access_token"`
    RefreshToken string    `json:"refresh_token,omitempty"`
    TokenExpiry  time.Time `json:"token_expiry"`
    Extra        string    `json:"extra,omitempty"` // 平台特有参数 JSON
    CreatedAt    time.Time `json:"created_at"`
    UpdatedAt    time.Time `json:"updated_at"`
}
```

### 19.3 Port 接口

```go
// internal/port/publish.go

// PlatformPublisher 平台发布能力（每个平台实现一个）。
type PlatformPublisher interface {
    // Platform 返回该发布器支持的平台标识。
    Platform() domain.Platform

    // Upload 上传视频（可能进入草稿箱，不一定直接发布）。
    Upload(ctx context.Context, req PublishRequest) (*PublishResult, error)

    // Publish 确认发布（上传与发布分两步的平台用此步）。
    Publish(ctx context.Context, videoID string) (*PublishResult, error)

    // Status 查询发布/审核状态。
    Status(ctx context.Context, videoID string) (PublishStatus, string, error)

    // Delete 删除已发布的视频。
    Delete(ctx context.Context, videoID string) error

    // UploadCover 上传/替换封面图。
    UploadCover(ctx context.Context, videoID, coverPath string) error
}

// PublishRequest 发布请求素材。
type PublishRequest struct {
    VideoPath   string
    CoverPath   string
    Title       string
    Description string
    Tags        []string
    Category    string
    ScheduledAt *time.Time
    Account     *domain.PlatformAccount  // 使用的账号凭证
    Extra       map[string]any           // 平台特有参数
}

// PublishResult 发布结果。
type PublishResult struct {
    VideoID string // 平台侧视频 ID
    URL     string // 发布后链接
    Status  domain.PublishStatus
    Error   string // 平台返回的错误信息
}
```

### 19.4 封面图策略

双模式（用户拍板）：

1. **自动截帧（默认，零成本）**：从成片中提取——
   - 第一帧：`ffmpeg -i video.mp4 -ss 0 -vframes 1 cover-first.jpg`
   - 高潮帧：按分镜中旁白最密集/时长最长的镜头时间点截帧，取 3 张供选
   - Web UI 提供「选封面」滑块，拖动时间戳实时预览 → 确认后截帧保存

2. **AI 生成封面（可选，按张计费）**：复用 `ImageGenerator`——
   - Prompt：基于 `Story.Title` + `Story.Summary` + 平台特征生成
   - 抖音/快手/视频号：9:16 竖屏封面，大字标题叠加
   - B站：16:9 横屏封面
   - 小红书：3:4 或 1:1 封面
   - 产物落 `<final-node>/covers/cover-ai.jpg`

3. **尺寸适配**（ffmpeg 自动裁剪缩放）：
   - 抖音/快手/视频号：1080×1920（9:16）
   - B站：1920×1080（16:9）
   - 小红书：1080×1440（3:4）

### 19.5 标题与描述自动生成

基于已有 `Story` 节点内容自动填充，按平台规则裁剪：

| 字段 | 生成规则 | 平台字数限制 |
|------|---------|-------------|
| 标题 | `story.Title` 截断 | 抖音 ≤55 / 快手 ≤20 / B站 ≤80 / 小红书 ≤20 / 视频号 ≤30 |
| 描述 | `story.Summary` + 换行 + 自动生成标签 | 各平台 ≤1000~2000 |
| 标签 | `#{dynasty} #{series_name} #{story.Title关键词}` | 按平台格式（#话题 或 话题） |

可扩展为 AI 生成平台专属标题（§18 creative knob 方式：新增 `title_style` knob 为每个平台生成不同风格标题）。

### 19.6 触发方式

#### CLI

```bash
# 发布到指定平台（单个或逗号分隔）
story publish <episode-id> --platform douyin,kuaishou,bilibili

# 发布到系列配置的所有目标平台
story publish <episode-id> --all

# 自定义标题/描述/标签
story publish <episode-id> --platform douyin \
  --title "鬼谷子第1集：那个改变战国格局的人" \
  --desc "百家讲坛式口播..." \
  --tags "历史,鬼谷子,战国"

# 使用 AI 生成封面
story publish <episode-id> --platform douyin --cover ai

# 指定封面文件
story publish <episode-id> --platform douyin --cover /path/to/cover.jpg

# 定时发布
story publish <episode-id> --platform douyin --schedule "2026-09-22T10:00:00+08:00"

# 重新发布（换一版成片后）
story publish <episode-id> --platform douyin --reroll

# 查看发布状态
story publish status [episode-id]

# 取消发布（上传中/待发布状态可取消）
story publish cancel <job-id>

# 删除已发布视频
story publish delete <job-id>
```

#### Web UI

```
集详情页 → 成片预览区 →「发布到平台」按钮
  → 发布面板（Modal）：
    ├── 平台勾选（多选 checkbox，已授权的平台才可勾选）
    ├── 标题输入（预填 story.Title，可编辑，字数实时计数）
    ├── 描述输入（预填自动生成，可编辑）
    ├── 封面选择（三个 Tab：自动截帧 / AI 生成 / 上传自定义）
    ├── 话题标签（预填推荐，可增删）
    ├── 分类选择（按平台拉取分类列表）
    ├── 定时发布开关 + 时间选择器
    └──「发布」按钮
  → 发布后 → 平台状态卡片列表（每平台一行：状态徽标 + 链接 + 错误信息 + 重试按钮）
```

#### 自动触发

`Engine.Compose`（final 阶段）成功后，若 `SeriesConfig.TargetPlatforms` 非空：
- 自动创建 `PublishDraft`（`status=pending`），素材自动填充
- **不自动上传**，需用户在 Web/CLI 确认后才真正发布（human-in-the-loop）
- Web 通过 SSE 推送 `publish_ready` 事件，前端自动刷新发布面板

#### 定时发布实现

- `PublishJob.ScheduledAt` 非空时，上传仍然立即执行（把视频传到平台草稿箱），但 `Publish` 动作延迟到指定时间
- engine 后台 goroutine 每分钟扫描 `status=uploaded AND scheduled_at <= now()` 的任务，自动执行 `Publish`
- 定时精度：±1 分钟（受扫描间隔影响）
- 用户可在定时触发前随时取消

### 19.7 发布失败处理

#### 失败分类

| 失败类型 | 错误示例 | 处理策略 |
|---------|---------|---------|
| 网络错误 | timeout, connection refused | 指数退避重试 3 次（2s/4s/8s） |
| 认证过期 | token expired, unauthorized | 自动 refresh token → 仍失败标记需重新授权 |
| 文件问题 | format not supported, file too large | 自动转码/压缩后重试（ffmpeg） |
| 审核拒绝 | content violation | 标记 `rejected` + 记录原因，不自动重试 |
| 频率限制 | rate limit exceeded | 按平台限流窗口延迟重试（如 60s 后） |
| 平台错误 | 500/502/503 | 指数退避重试 |
| 素材缺失 | cover required | 自动生成封面后重试 |
| 取消 | context canceled | 保留已上传进度，标记 `canceled` |

#### 重试机制

- 复用现有 `Runner` 的并发/重试模式（§5），每个平台发布任务独立重试
- `MaxRetries` 默认 3，可通过 `SeriesConfig.MaxRetries` 覆盖
- `Attempts` 累加，每次重试递增
- 认证过期不计入重试次数（属于外部状态变更，非瞬时错误）
- 审核拒绝（`rejected`）不可重试——需要修改内容后重新发布（新任务）

#### 断点续发

与现有 `produce` 同款——按磁盘产物判断状态：
- 已上传成功但未发布 → 只调 `Publish`
- 已发布但状态未同步 → 只调 `Status` 同步
- 用户手动重试 → 从上次失败点继续

### 19.8 目录布局

```
data/projects/<series-id>/<episode-id>/
  versions/<final-node-id>/
    output/<ep-id>-<ratio>.mp4   ← 成片（已有）
    covers/                       ← 封面截图目录（新增）
      cover-first.jpg             ← 自动截帧：第一帧
      cover高潮.jpg               ← 自动截帧：高潮帧
      cover-ai.jpg                ← AI 生成封面（可选）
    publish/                      ← 发布任务记录（新增）
      douyin-<job-id>.json
      bilibili-<job-id>.json
      ...
```

### 19.9 目录与持久化

- `publish_jobs` 表：`publish_jobs` + `platform_accounts` 表（SQLite，ensureColumn 幂等迁移）
- `publish_jobs` 含 `episode_id, platform, node_id, status, video_path, cover_path, title, description, tags, category, platform_video_id, platform_url, scheduled_at, attempts, max_retries, error, created_at, updated_at`
- `platform_accounts` 含 `platform, account_name, account_id, access_token, refresh_token, token_expiry, extra, created_at, updated_at`
- `port.Repository` 新增 `CreatePublishJob / GetPublishJob / ListPublishJobsByEpisode / UpdatePublishJob / DeletePublishJob / ListPlatformAccounts / GetPlatformAccount / SavePlatformAccount / DeletePlatformAccount`

### 19.10 目录约定与产物

```
data/projects/<series-id>/<episode-id>/
  versions/<final-node-id>/
    output/                       ← 成片（已有）
    covers/                       ← 封面图（新增）
      cover-<ts>.jpg
    publish/                      ← 发布任务记录（新增）
      <platform>-<job-id>.json
```

### 19.11 系列配置扩展

```go
// SeriesConfig 新增
type SeriesConfig struct {
    // ... 现有字段 ...
    // TargetPlatforms 保留作为默认发布目标（§19）
    // 发布默认配置
    PublishDefaults PublishConfig `json:"publish_defaults,omitempty"`
}

type PublishConfig struct {
    AutoTag     bool   `json:"auto_tag"`      // 自动生成话题标签
    TagTemplate string `json:"tag_template"`  // 标签模板：#{dynasty} #{series_name}
    TitleSuffix string `json:"title_suffix"`  // 标题后缀：如「#鬼谷子 #历史」
}
```

### 19.12 入口汇总

| 层 | 入口 |
|----|------|
| CLI | `story publish status/cancel/delete` + `story publish <ep> --platform` + `story platform auth/list/remove` |
| server | `POST /api/episodes/{id}/publish` + `GET /api/episodes/{id}/publish` + `POST /api/episodes/{id}/publish/{jobId}/cancel` + `DELETE /api/episodes/{id}/publish/{jobId}` + `GET/POST/DELETE /api/platforms/{platform}/account` + `GET /api/platforms` |
| Web | 集详情页「发布」面板 + 发布状态卡片 + 平台账号管理页（`/settings/platforms`） |
| engine | `Engine.Publish(ctx, epID, opts)` + `Engine.PublishStatus(ctx, jobID)` + `Engine.CancelPublish(ctx, jobID)` + `Engine.DeletePublished(ctx, jobID)` |

### 19.13 平台 Provider 实现要点

#### 抖音

- 上传：`POST /video/upload` → 返回 `video_id`
- 发布：`POST /video/publish` → `video_id` → 返回 `item_id`
- 封面：上传时传 `cover` 字段或上传后 `POST /video/update_cover`
- 定时：`publish_time` unix timestamp 参数
- 认证：OAuth2 refresh_token 有效期 ~15 天

#### 快手

- 上传：`POST /v1/source/upload` → 返回 `video_id`
- 发布：`POST /v1/source/publish`
- 封面：`cover_url` 字段
- 定时：`schedule_time` 参数
- 认证：OAuth2 refresh_token

#### B站

- 上传：分片上传 `POST /x/web-interface/view` → `bvid`
- 发布：`POST /x/web-interface/view/submit`
- 封面：`POST /x/web-interface/archive/cover`
- 定时：`delay` 参数（秒数）
- 认证：OAuth2 + SESSDATA Cookie，refresh_token 有效期 ~30 天
- 特殊：需要 `分区`（tid）+ `标签`

#### 视频号

- 上传：微信开放平台 `POST /cgi-bin/media/upload`
- 发布：`POST /cgi-bin/draft/add`
- 封面：`thumb_media_id`
- 定时：`pub_time` 参数
- 认证：微信 OAuth2，access_token 有效期 2 小时

### 19.14 回归测试（成本红线内全程 mock）

- `TestPublishJobLifecycle`（创建 → 上传中 → 已发布，状态机正确）
- `TestPublishRetryOnTransientError`（网络错误 → 重试 3 次 → 成功）
- `TestPublishNoRetryOnRejected`（审核拒绝 → 标记 rejected，不重试）
- `TestPublishAuthRefresh`（token 过期 → 自动刷新 → 继续上传）
- `TestPublishSchedule`（定时任务：uploaded 后等待 → 时间到 → published）
- `TestPublishCancel`（上传中取消 → 保留进度，状态 canceled）
- `TestPublishDraftFromCompose`（compose 成功 → 自动创建 pending 草稿）
- `TestPublishCoverAutoExtract`（自动截帧：第一帧 + 高潮帧）
- `TestPublishCoverAI`（AI 生成封面 → 落盘 covers/cover-ai.jpg）
- server 端：`TestPublishHTTP`（POST/GET/cancel/delete 端点 + 权限校验）

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
- 2026-09-21（项目定位调整为通用 AI 视频流水线，见 §1、§7、§8）：立项定位从「中国历史故事 AI 视频流水线」上调为**通用的「AI 视频制作流水线」工具箱**（给题材经 story→storyboard→media→final 四阶段产出带旁白与硬字幕的 MP4，题材不限）。新增 §1「题材解耦铁律」：engine/port/domain/store/subtitle/provider/server 与 Web 不得写死题材假设，题材只允许出现在①系列配置字段、②`internal/templates`（prompt 模板 + 题材/画风视觉锚定包）两处，**新增题材＝加模板 + 用系列配置取值，engine 与 CLI 零改动**。§7 标题与结构改为「题材模板与 Prompt 约束（当前模板：中国历史故事）」，拆出「通用约束」（纯 JSON、画风统一、时长兜底）与「【历史模板】」（叙事定位、加工铁律、文言比例铁律、情感落点、出处、分镜忠实切分、朝代视觉锚定）；§8 明确声音与题材无关（声音为顶层实体，按 series.voice_id 取用，默认内置条目 `longtian`）。中国历史故事成为该约束下的**第一套题材模板**，不是项目边界。同期修正 §4 的 storyboard 镜头数描述（旧写 6–12，实为 prompt 目标 18–26、校验下限 4 上限 28）。
- 2026-09-21（说书人讲述稿收紧 + 分镜忠实切分，见 §7）：用户反馈「内容只是照搬出处、没有深层加工」「文言太多像讲书、听众没波澜」。`StorySystemPrompt` 重写：核心原则新增【必须加工，严禁照搬】（三层加工——选人 / 重排 / 还原处境，只改叙事结构不新增史实）、【必须有一个"心里一沉"的落点】（不划算却不得不做的选择、收不回的话，用具体动作或物什砸出来，砸完就停）、【文言比例铁律】（典籍原话引用全片 ≤2 处、每处 ≤15 字且先白话讲透；史书冷账句必须白话转述；禁止连续文言对白；写完自查），并补「加工示例」「文言处理示例」两组对照示例；叙事结构改为「钩子 → 极简处境 → 冲突加压（≥三层）→ 高潮（关键动作慢写）→ 转折 → 结尾留白或留钩子」，**严禁说理/点题式收束**；`StoryboardSystemPrompt` 第 2 条改为【忠实切分，严禁重写】（`narration` 逐字沿用讲述稿、只允许在衔接处增删一两个顺承词，不得把白话改回文言、不得新增文言引语），`StoryboardUserPrompt` 末尾由「末镜点题」改为「末镜留白或留钩子」（修掉与系统提示自相矛盾处）。prompt 改动需重跑 generate/storyboard 才生效。
- 2026-09-21（小人书运镜幅度，见 §13）：用户反馈「每个分镜动的幅度太小，不仔细看根本看不出变化」。`internal/provider/ffmpeg/still.go` 把统一倍率 1.12 拆为两档——推近/拉远 `stillZoom`=1.30（全程放大三成），平移 `panZoom`=1.24（平移行程 = `iw - iw/zoom`，视窗横移约画面的四分之一宽度）。旧值下单是平滑缓动就让首尾几乎不动。静帧画面唯一的运动来源就是运镜，幅度须肉眼可辨；本地 ffmpeg 集成测试覆盖 8 种运镜。
- 2026-09-21（集级版本树改造，见 §17）：一集从「一条流水线」改为**版本树容器**，根治「重跑上游不做失效」隐患（新分镜旁白配旧画面/旧旁白被静默复用）。domain：新增 `version.go`（`Stage`/`NodeStatus`/`VersionNode` + `NodeByID`/`Children`/`ActivePath`/`ActiveNodeOfStage`/`MaxAttempt`/`AddNode`/`RemoveSubtree`），重写 `episode.go`（`Nodes`+`ActiveNodeID`），**删除 `state.go`**；store：episodes 表加 `nodes_json`/`active_node_id`（ensureColumn 幂等），`state_json` 转为只读历史快照；engine：新增 `derive.go`（内容寻址派生键 `nodeKey`、`derivationSchemaVersion=1`、`ensureNode`/`resolveParent`/`nodeDir`/`refsDigest`/`voiceDigest`）与 `legacy.go`（`DeriveLegacyChain`），各步骤方法统一收 `DeriveOptions{From,Reroll,Scenes}`（`GenerateCandidates`→`GenerateStory`、`ProduceScenes` 并入 `Produce`、**`Pick` 删除**），新增 `ActivateNode`/`DeleteNode`；app：新增 `migrate_versions.go` 一次性迁移旧集（先落库再 rename、失败回退 Dir、最终统一 rebase 并二次落库）；server：`actionReq` 加 `from`/`reroll` 删 `index`，新增 activate/delete 两个同步端点；CLI：`step.go` 改新签名 + 通用 `--from`/`--reroll`，新增 `story nodes`/`activate`/`node-rm`，删 `story pick`；Web：`types.ts` 换 `Stage`/`NodeStatus`/`VersionNode`，新增纯 SVG `VersionTree.tsx`（活跃路径金色高亮 + 选中展开「从此处继续/换一版/设为活跃/删除」），`EpisodePage` 改三段式，删 `StepsBar.tsx`，`SeriesDetailPage` 进度改活跃路径。核心回归 `TestRerollStoryboardIsolatesOldClips`（换分镜后旧片段绝不复用、旧目录保留）等 9 个新用例，全程 mock，`go build/vet/test` 与 `npm run build` 全绿。
- 2026-09-21（创作控制参数：插件化旋钮 + 预设，见新增 §18）：用户提出「定位从历史故事转为通用后，系列/剧集界面应能给不同用户产出不同风格的故事与视频，且要有默认值、灵活、插件化」。落地为**声明式注册表**：`internal/templates/creative.go` 声明 6 个 `Knob`（`narrative` 纪录客观/当事人自述/悬疑倒叙、`audience` 青少年/儿童、`length` 短篇/长篇、`motion` 强/弱、`video_style` 画风、`instruction` 自由指令）与 4 个 `Preset`（`classic` 默认 values 空、`documentary`、`kids`、`suspense`），**Knob 的 `Get`/`Set func(domain.SeriesConfig)` 使 app/engine/CLI/server 全程不知参数名**——新增一个风格维度只改本文件，其余各层零改动，前端由 `GET /api/creative-catalog`（`templates.Catalog()`）驱动渲染控件。三条硬约束：默认值一律空串、全零值时 `StoryBrief`/`BoardBrief` 返回 `""`、brief 只覆盖口味（末尾固定【冲突声明】点名硬性规则优先）。domain：新增 `CreativeStyle`（`IsZero()`）+ `SeriesConfig.Creative` 用 **`omitzero`**（Go 1.24 的 `omitempty` 对结构体无效）、`Episode.Instruction`（episodes.instruction 列）；engine：`storyParams.Brief`/`storyboardParams.Brief`/`mediaParams.Motion` 全部 `omitempty` 进派生键，**默认空串时派生键与旧版逐字节相同**（`engine/creative_test.go` 与 `legacyNodeKey` 比对）；取值链 `templates.StoryBrief`/`BoardBrief` → `port.StoryRequest/StoryboardRequest.Brief` → bailian `UserPrompt`，运镜 `port.StillRequest.MotionStrength` → ffmpeg `motionZooms`（标准 1.30/1.24、strong 1.42/1.34、subtle 1.16/1.12）；app：`ExpandCreative`（创建时展开预设+微调）与 `UpdateSeriesCreative`（**补丁语义**，未提到的参数保持原值、显式空串清除）；server：新增 `GET /api/creative-catalog` 与 `PUT /api/series/{id}/creative`，`POST /api/series` body 加 `preset`+`creative`（画风也是一个 knob，不做顶层双通道），`POST /api/series/{id}/episodes` body 加 `instruction`，未知 knob/预设 400 并列出支持项；CLI：`story series create`/`series set` 共用 `--preset`/可重复 `--creative key=value`/`--story-instruction`/`--video-style`，`story episode create --instruction`，`printCreative` 如实渲染预设溯源（「预设「悬疑倒叙」基础上微调」），全空打印「创作设置: 全部跟随内置默认」；Web：新建 `CreativeFields`（catalog 驱动）+ 系列详情「创作设置」折叠区 + 新建一集弹窗附加指令 + 集页只读展示。回归测试覆盖注册表不变式、派生键字节级兼容、catalog/补丁语义/未知 key 400/集级指令落库，全程 mock，未真实调用 `bl`。
- 2026-09-21（多平台发布系统，见新增 §19）：成片（final）合成完毕后的第 5 阶段——将视频与配套素材发布到各平台。全部平台（抖音/快手/B站/小红书/视频号）一次性并行接入。架构 Ports & Adapters：新增 `port.PlatformPublisher` 接口 + 各平台 provider（`internal/provider/publish/<platform>/`），engine/app 不知道具体平台；新增 `PublishJob`（发布任务）与 `PlatformAccount`（平台账号凭证）两个顶层持久化实体（SQLite `publish_jobs` + `platform_accounts` 表）。封面双模式：自动截帧（默认零成本，ffmpeg 截第一帧+高潮帧）+ AI 生成封面（可选按张计费，复用 ImageGenerator）。标题/描述/标签基于 Story 节点自动生成并按平台规则裁剪。触发：CLI `story publish` + Web 集详情页「发布」面板 + Compose 成功后自动创建 pending 草稿（human-in-the-loop 确认后发布）。定时发布：立即上传到草稿箱，`ScheduledAt` 时间到自动确认发布（后台 goroutine 每分钟扫描）。失败处理：网络错误/平台 500 指数退避重试 3 次、认证过期自动 refresh、审核拒绝标记 rejected 不重试、频率限制按窗口延迟。断点续发：按磁盘产物判断从失败点继续。小红书无官方 API 采用半自动模式（系统备素材 + 跳转上传页）。回归测试全程 mock。
