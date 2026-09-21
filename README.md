# story — AI 视频制作流水线

**一套通用的「AI 视频制作流水线」工具箱**：给一个题材 / 主题，经 **故事（脚本）→ 分镜 → 画面 + 旁白 → 成片 → 发布** 五个阶段，自动产出带旁白、带硬字幕的 MP4 短视频并发布到各平台，适配抖音（中视频）/ 快手 / B站 / 小红书 / 视频号。

题材**不限**：仓库内置的第一套题材模板是**中国历史故事**（二十四史、《资治通鉴》、《世说新语》、笔记小说等典籍取材），它只是这套流水线的第一个实现，不是项目的边界。换题材＝在 `internal/templates` 换一套模板 + 用系列配置取值，**engine 与 CLI 零改动**（见 [扩展新题材](#扩展新题材)）。

内容按 **系列 Series → 多集 Episode** 组织（系列＝一个栏目 / 主题，如「鬼谷子」系列；集＝一期）。每一集是**一棵版本树**：四个阶段依次派生，任一阶段都可「换一版」产出新分支，旧分支与旧产物完整保留（见 [版本树](#版本树换一版与回退)）。

## 流水线五阶段

| 阶段 | 产物 | 说明 |
| --- | --- | --- |
| `story` | 定稿故事 / 脚本 | 一次生成一篇，**生成即定稿**（不做多候选人工挑选），不满意可「换一版」 |
| `storyboard` | 分镜表 | 拆为 18–26 个镜头（visual_prompt / narration / duration / camera），并顺带产出零成本的集级视觉参考文字约束 |
| `media` | `panels/` `clips/` `audio/` | 按系列配置生产画面（小人书插画运镜 / AI 视频）并合成旁白；**只生产未完成的镜头**，断点续跑、可手动停止 |
| `final` | 成片 MP4 | ffmpeg 归一化 → 音视频合成 → 拼接 → 烧录硬字幕；多比例导出追加到该节点的 `Outputs` |
| `publish` | 平台发布 | 将成片发布到抖音/快手/B站/小红书/视频号等平台，支持多账号、定时发布 |

每个阶段都有自动验收（故事非空、镜头数合法、媒体可被 ffprobe 解析、成片时长 ≈ 各镜头之和），不通过则标记 `failed` 并写入错误原因。

## 技术架构

Go 单二进制 CLI + 内嵌 Web UI，接口驱动（Ports & Adapters）；AI、ffmpeg 与平台发布均通过 `os/exec` 调用，不依赖 HTTP SDK、CGO 或后台服务：

| 层 | 目录 | 职责 |
| --- | --- | --- |
| CLI | `cmd/story/` | cobra 命令 |
| 调度 | `internal/engine/` | 版本树派生（内容寻址）、并发限流（默认 3）、指数退避重试（2s/4s/8s）、每阶段自动验收 |
| 接口 | `internal/port/` | Story / Storyboard / SeriesPlanner / Video / Image / Speech / VoiceLister / VoiceBuilder / AudioNormalizer / Composer / Task / PlatformPublisher / Repository |
| 领域 | `internal/domain/` | 纯数据层（Series / Episode / VersionNode / VisualRef / Voice / PublishJob / PlatformAccount 等 + 纯函数），不依赖第三方包 |
| 百炼适配 | `internal/provider/bailian/` | 封装 `bl`（故事、分镜、策划、视频、图片、TTS、造声、异步任务轮询） |
| ffmpeg 适配 | `internal/provider/ffmpeg/` | 静帧运镜渲染、音频归一化、拼接、烧字幕、多比例导出、封面截帧 |
| 平台发布 | `internal/provider/publish/` | 封装 `sau`（social-auto-upload CLI），支持抖音/快手/B站/小红书/视频号等 |
| 字幕渲染 | `internal/subtitle/` | 纯 Go（x/image）渲染中文透明 PNG，叠加烧录，**不要求 ffmpeg 带 libass** |
| 题材模板 | `internal/templates/` | prompt 模板 + 题材/朝代视觉锚定 + 视觉风格包（工笔 / 写实 / 水墨） |
| Web 后端 | `internal/server/` | 标准库 net/http：REST API + SSE 实时进度 + 媒体服务（防路径穿越、支持 Range） |
| Web 前端 | `web/` | Vite 8 + React 19 + TypeScript + Tailwind v4 + React Router + TanStack Query，go:embed 打包 |
| 持久化 | `internal/store/sqlite/` | 纯 Go 驱动 `modernc.org/sqlite`，WAL |
| 装配 | `internal/app/` | 唯一允许依赖具体实现的包（CLI 与 Web 共用同一容器） |
| 依赖检查 | `internal/doctor/` | CLI 依赖健康检查（bl / ffmpeg / python3 / sau / Chrome） |

## 环境要求

- Go 1.24（**构建加 `GOTOOLCHAIN=local`**，依赖已固定，禁止自动下载新工具链）；Node 20+（仅在需要重建前端时）
- [百炼 CLI `bl`](https://help.aliyun.com/zh/model-studio/) 并已认证（`bl auth login` 或 `DASHSCOPE_API_KEY`）
- ffmpeg / ffprobe（仅需基础滤镜，libass 非必需）
- macOS 中文字体（PingFang / 冬青黑体 / STHeiti / 宋体任一，自动探测）
- **发布功能**（可选）：Python 3.10+ + `social-auto-upload`（sau）+ Chrome 浏览器（sau 浏览器自动化依赖）

## 构建与安装

```bash
make install     # 重建前端（需要时）+ 编译 + 安装到 PATH 中可写的 bin 目录
# 或只构建：make build  → ./story

./story init     # 创建 data/、SQLite，检查所有依赖并尝试自动安装缺失项
./story doctor   # 全面健康检查（bl / ffmpeg / python3 / pip / sau / Chrome）
```

用 `make help` 查看全部目标（`build` / `install` / `serve` / `test` / `test-integration` / `vet` / `fmt` / `clean`）。

## 配置

可选配置文件 `story.yaml`（缺省全部有默认值）：

```yaml
data_dir: data
bl_bin: bl
ffmpeg_bin: ffmpeg

text_model: ""             # 留空用 bl 默认
video_model: ""
image_model: ""            # 图片模型（插画 / 视觉参考图）
tts_model: cosyvoice-v3-flash
tts_voice: longtian_v3
tts_instruction: "请用沉稳厚重、富有历史讲述感的语调，语速从容不迫，像学者在讲历史故事，不要播报腔。"  # 旧路径回退值；新系列按声音库条目（voice_id）取用

# 造声（声音设计 / 声音复刻）直连百炼 HTTP 用；留空回退 DASHSCOPE_API_KEY 与 ~/.bailian/config.json
bailian_api_key: ""
bailian_base_url: ""

default_ratio: 9:16
default_resolution: 1080P  # 档位指长边：1080P → 1080×1920，720P → 720×1280
max_concurrency: 3
max_retries: 3
subtitle_font: ""          # 留空自动探测系统中文字体
```

环境变量覆盖：`STORY_DATA_DIR` / `STORY_BL_BIN` / `STORY_FFMPEG_BIN` / `STORY_TTS_VOICE` / `STORY_BAILIAN_API_KEY` / `STORY_BAILIAN_BASE_URL` / `STORY_MAX_CONCURRENCY`。

## 使用流程（CLI）

```bash
# 1. 创建系列（题材参数在此选定；画面模式与声音创建后锁定，不可更改）
./story series create --name 鬼谷子 --dynasty 战国 \
  --ratio 9:16 --resolution 1080P --visual-mode comic \
  --platforms douyin,kuaishou,bilibili
./story series list
./story series show <series-id>

# 2. 创建一集（--topic 可空，由 AI 自由命题）
./story episode create --series <series-id> --title "入秦" --topic "苏秦张仪出山之前"
./story episode list --series <series-id>

# 3. 四阶段流水线（每步产物落 SQLite + 工作目录，失败可重入续跑）
./story generate   <episode-id>     # ① 生成定稿故事（story.md 可人工审阅）
./story storyboard <episode-id>     # ② 拆分为分镜（storyboard.json 可人工审阅）
./story produce    <episode-id>     # ③ 生产画面 + 旁白（已完成的镜头自动跳过，零费用）
./story compose    <episode-id>     # ④ 归一化 + 拼接 + 烧硬字幕 → 成片
./story status     <episode-id>     # 随时查看版本树各节点状态 / 错误

# 一键跑到底（供 Agent 自动驱动；已有节点直接复用）
./story run <episode-id>

# 4. 导出其他比例（模糊背景填充，不重复生成画面）
./story export <episode-id> --ratio 16:9
```

### 只重试失败镜头

`produce` 默认**只为「画面或旁白尚未产出」的镜头建任务**，已成功的镜头不做任何调用，因此直接重跑本命令即等价于「只重试失败镜头」；也可指定镜头（序号从 1 起，越界报错）：

```bash
./story produce <episode-id> --scenes 13,15,17
```

画面与旁白在同一镜内互相独立（画面失败也照常合成旁白），Web 端还支持运行中「停止」——已完成的产物全部落盘保留，之后再执行即从断点续跑。

### 版本树：换一版与回退

```bash
./story nodes    <episode-id>                 # 打印版本树（节点 ID / 版本 / 状态 / 摘要）
./story activate <episode-id> <node-id>       # 把某节点设为活跃（切换成片 / 分镜详情）
./story node-rm  <episode-id> <node-id>       # 删除该节点及其全部后代与媒体文件（不可恢复）

# 通用派生 flag（挂在 generate / storyboard / produce / compose / run / export 上）
./story storyboard <episode-id> --reroll                  # 基于同一父节点另开一版（旧版原地保留）
./story storyboard <episode-id> --from <story-node-id>    # 从指定节点往下派生
```

节点 ID 是**内容寻址**的派生键（`sha1(schema版本|阶段|父节点|参数|尝试次数)` 取前 12 位），只哈希输入不哈希输出。因此同输入重跑会**复用同一节点**（零模型调用），而「换一版」得到新节点、新目录——上游一变目录名就变，旧片段绝不会被静默错配（这正是版本树要消除的隐患）。不同阶段产物物理隔离在各自版本目录，因此可以放心地「换分镜 → 重出画面」而不用清理旧文件。

### 视觉参考（人物 / 场景一致性）

两级结构：**系列级**跨集复用的主角（`story keyframes`）+ **集级**本集人物与重复出现的场景（`story episode-refs`）。文字约束由分镜阶段零成本产出；参考图按张计费，**手动执行命令才生成**：

```bash
./story keyframes   <series-id>  [--force]   # 系列级人物参考图 → data/projects/<series>/refs/
./story episode-refs <episode-id> [--force]  # 集级人物/场景参考图 → <episode>/refs/
```

参考图仅在 `video` 模式下作为生成参考喂给模型（小人书模式的 `bl image generate` 不支持参考图，只吃文字约束）。

### 声音库

声音是与系列同级的顶层实体（跨系列复用），新建系列时选定 `voice_id` 后**锁定不可改**；声音条目本身可编辑，改后对所有引用它的系列生效。

```bash
./story voice list                       # 列出声音条目（含内置 6 条）
./story voice show <voice-id>
./story voice add --name ... --voice longtian_v3 --provider bailian
./story voice edit <voice-id> --rate 1.05
./story voice preview <voice-id> --text "试听文本"
./story voice rm <voice-id>

# 造声：从描述设计音色 / 用参考音频复刻音色（成功后自动落库成条目）
./story voice design --name 说书老者 --prompt "…音色描述…" --preview-text "试听文本"
./story voice clone  --name 我的音色 --audio /path/to/ref.wav --provider bailian
```

**禁止复刻他人真人声音用于商业发布**（声音权法律风险），复刻能力不得作为默认路径。

## 产物布局

`data/` 不入库；`story.md` / `storyboard.json` 在各版本目录内落一份人工审阅副本，**事实源以数据库为准**。

```
data/
├── story.db
└── projects/<series-id>/
    ├── refs/                                  # 系列级视觉参考图（跨集共享）
    └── <episode-id>/
        ├── refs/                              # 集级视觉参考图（跨分镜版本共享，永不随版本搬动）
        └── versions/<节点 ID>/                # 每个版本节点一个目录（attempt>0 追加 -a<n>）
            ├── story.md                       # story 阶段
            ├── storyboard.json                # storyboard 阶段
            ├── panels/ clips/ audio/          # media 阶段：插画 / 片段 / 旁白
            └── tmp/ output/                   # final 阶段：中间件 + 成片与多比例导出
```

## 平台发布

成片合成后，可一键发布到多个平台。发布基于 [social-auto-upload](https://github.com/dreammis/social-auto-upload)（sau）——一个用 Playwright 浏览器自动化实现的视频上传工具，用户只需扫码登录一次，后续上传复用 Cookie。

### 支持平台

| 平台 | sau key | 说明 |
| --- | --- | --- |
| 抖音 | `douyin` | 视频上传 + 定时发布 |
| 快手 | `kuaishou` | 视频上传 + 图文上传 |
| B站 | `bilibili` | 视频上传（自动管理 biliup 依赖） |
| 小红书 | `xiaohongshu` | 视频 + 图文笔记 |
| 视频号 | `tencent` | 视频上传 |

### 安装 sau

```bash
# story init 会自动安装，也可手动：
uv pip install --system git+https://github.com/dreammis/social-auto-upload.git

# 或 clone 后本地开发安装：
git clone https://github.com/dreammis/social-auto-upload.git ~/.story/sau
cd ~/.story/sau && cp conf.example.py conf.py && uv sync
```

### 使用流程

```bash
# 1. 健康检查（确认 sau / Chrome 已就绪）
./story doctor

# 2. 登录各平台（首次需扫码，Cookie 自动持久化）
./story platform login douyin --account my_douyin
./story platform login bilibili --account my_bili
./story platform login kuaishou --account my_ks

# 3. 检查登录状态
./story platform check douyin --account my_douyin

# 4. 发布
./story publish run <episode-id> --platform douyin,kuaishou --account my_douyin
./story publish run <episode-id> --all                                    # 发到系列配置的所有平台
./story publish run <episode-id> --platform douyin --schedule "2026-09-22T10:00:00+08:00"  # 定时

# 5. 查看发布状态
./story publish status <episode-id>
```

### 多账号

sau 原生支持多账号——`--account <name>` 标识不同账号，Cookie 按账号名独立存储：

```bash
./story platform login douyin --account personal
./story platform login douyin --account company
./story publish run <episode-id> --platform douyin --account company
```

### 封面策略

- **自动截帧（默认，零成本）**：从成片中截取第一帧 + 高潮帧
- **AI 生成封面（可选，按张计费）**：复用 ImageGenerator，按平台比例出图

## 依赖健康检查

```bash
./story doctor    # 检查全部 7 项依赖
./story init      # 初始化 + 自动安装缺失依赖
```

检查项：bl（百炼 CLI）、ffmpeg、ffprobe、python3（≥ 3.10）、pip/uv、sau（social-auto-upload）、Chrome（≥ 144）。每项显示状态（✅/❌/⚠️）、版本号和修复建议；`story init` 会尝试自动安装缺失项（如从 GitHub clone sau 并配置）。

## Web 界面

除 CLI 外提供 Web UI：预览版本树、故事、分镜、画面片段、旁白、成片、视觉参考与声音，在线播放媒体，并可直接点按钮触发各流水线动作（SSE 实时刷新状态，无需轮询；同一集同时只跑一个动作，运行中可「停止」）。

```bash
# 方式一：单二进制（前端已 go:embed 打包）
make install && story serve --daemon     # http://127.0.0.1:7878
story serve status / stop / restart

# 方式二：前端开发热更新
story serve                              # 终端 1：只跑 API
cd web && npm install && npm run dev     # 终端 2：http://127.0.0.1:5173（自动代理 /api）
```

修改前端后 `cd web && npm run build` 重新生成 `web/dist/`，再 go build 即把新界面嵌进二进制。页面包括：

- **系列列表**：系列卡片、新建系列（题材/朝代、比例、分辨率、画面模式、声音）
- **系列详情**：系列配置、AI 分集策划面板、系列视觉参考（人物）、集列表与各集进度
- **集详情**（三段式）：**版本树**（纯 SVG 手绘，活跃路径金色高亮；选中卡片可「从此处继续 / 换一版 / 设为活跃 / 删除」）+ 工具栏 · 选中节点的阶段产物详情（故事 / 分镜 / 片段与旁白 / 成片与导出）· 未完成镜头横幅 + 本集视觉参考 + 发布面板
- **声音**：声音条目卡片网格，新建 / 编辑 / 复制 / 试听，以及「声音设计」「声音复刻」（复刻支持上传文件 / 现场录音 / 音频 URL）
- **平台账号**：各平台登录状态管理（登录/检查/删除），支持多账号

## 扩展新题材

题材解耦是硬约束：`engine` / `port` / `domain` / `store` / `subtitle` / `provider` / `server` 与 Web 各层**不得写死题材假设**。题材相关的一切只允许出现在两处——

1. **系列配置字段**：`dynasty`、`video_style`、`visual_mode`、声音等（取值语义由题材模板定义）；
2. **`internal/templates`**：prompt 模板（故事 / 分镜 / 策划）+ 题材与画风视觉锚定包（`dynasty.go` 朝代锚定、`visualstyle.go` 视觉风格包）。

新增题材＝在 `templates` 新增一套模板 + 用系列配置取值，**engine 与 CLI 零改动**（与「换 AI 供应商只加一个 provider 实现」同款约束）。当前仓库内的「中国历史故事」模板即该约束下的第一个实现——其内容规则（说书人讲述稿、三层加工、文言比例、朝代视觉锚定等）与通用约束（纯 JSON、画风统一、时长兜底）在 [AGENTS.md](./AGENTS.md) §7 中分层记录。

## 说明

- **画面模式**（创建系列时选定、锁定）：`comic` 小人书（默认，每镜一张 AI 插画 + 本地 ffmpeg Ken Burns 运镜，仅按图片计费）/ `video`（每镜 AI 视频生成）。两种模式后续合成管线完全复用。
- **画风统一**：`visual_prompt` 只写画面内容，严禁出现画风词；画风由风格包唯一供给（`gongbi` 工笔重彩国风动画＝默认 / `realistic` 写实真人 / `ink` 水墨写意）。
- **字幕**：纯 Go 渲染透明 PNG → ffmpeg `overlay` 按镜头时间区间叠加；同时保留 SRT 旁路。无 libass 的 ffmpeg 构建也能烧录。
- **水印**：保留 AI 生成水印（合规）。
- **成本**：真实 `bl` 调用按次计费，`compose` 之后的步骤（归一化 / 拼接 / 字幕 / 多比例导出）全部本地完成、零费用。

## 测试

```bash
make test                                        # 单元测试（mock provider，不触网、零费用）
GOTOOLCHAIN=local go test -tags=integration ./... # 含真实 ffmpeg 的集成测试
make vet && make fmt
```

开发 / 测试**禁止真实调用 `bl`**（成本红线）：功能验证一律用 mock provider、单元测试与本地 ffmpeg；真实调用仅用于明确要求的生产执行。

## 架构约束

详见 [AGENTS.md](./AGENTS.md)：`engine` / `domain` / `port` 禁止 import 任何具体 provider；依赖装配只发生在 `internal/app` 与 `cmd/story`。替换 AI 供应商、新增题材，都只需新增一个实现或一套模板并在装配层接上，engine 与 CLI 零改动。