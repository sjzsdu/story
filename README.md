# story — 中国历史故事 AI 视频流水线

从二十四史、《资治通鉴》、《世说新语》、笔记小说等典籍取材，经 AI 生成故事候选 → 人工选择 → 朝代锚定分镜 → 百炼生成视频片段与 TTS 旁白 → ffmpeg 合成带硬字幕的 MP4，适配抖音 / 快手 / B站 / 小红书 / 视频号。

内容按 **系列 Series → 多集 Episode** 组织（如「鬼谷子」系列若干集），每集独立走一条可中断、可续跑、可人工介入的流水线。

## 技术架构

Go 单二进制 CLI，接口驱动（Ports & Adapters），AI 与 ffmpeg 均通过 `os/exec` 调用，不依赖 HTTP SDK、CGO 或后台服务：

| 层 | 目录 | 职责 |
| --- | --- | --- |
| CLI | `cmd/story/` | cobra 命令 |
| 调度 | `internal/engine/` | 状态机、并发限流（默认 3）、指数退避重试（2s/4s/8s）、每步自动验收 |
| 接口 | `internal/port/` | Story/Storyboard/Video/Speech/Composer/Task/Repository |
| 百炼适配 | `internal/provider/bailian/` | 封装 `bl`（故事、分镜、视频、TTS、异步任务轮询） |
| ffmpeg 适配 | `internal/provider/ffmpeg/` | 归一化、拼接、烧字幕、多比例导出 |
| 字幕渲染 | `internal/subtitle/` | 纯 Go（x/image）渲染中文透明 PNG，叠加烧录，**不要求 ffmpeg 带 libass** |
| Web 后端 | `internal/server/` | 标准库 net/http：REST API + SSE 实时进度 + 媒体服务（防路径穿越、支持 Range） |
| Web 前端 | `web/` | Vite 8 + React 19 + TypeScript + Tailwind v4 + React Router + TanStack Query，go:embed 打包 |
| 持久化 | `internal/store/sqlite/` | 纯 Go 驱动 `modernc.org/sqlite`，WAL |
| 朝代约束 | `internal/templates/` | 8 朝代视觉锚定（服饰/建筑/器物等）+ prompt 模板 |
| 装配 | `internal/app/` | 唯一允许依赖具体实现的包 |

## 环境要求

- Go 1.24（**构建加 `GOTOOLCHAIN=local`**，依赖已固定，禁止自动下载新工具链）
- [百炼 CLI `bl`](https://help.aliyun.com/zh/model-studio/) 并已认证（`bl auth login` 或 `DASHSCOPE_API_KEY`）
- ffmpeg / ffprobe（仅需基础滤镜，libass 非必需）
- macOS 中文字体（PingFang / 冬青黑体 / STHeiti / 宋体任一，自动探测）

## 构建与初始化

```bash
GOTOOLCHAIN=local go build -o story ./cmd/story

./story init            # 创建 data/、SQLite，并检查 bl/ffmpeg/ffprobe
```

可选配置文件 `story.yaml`（缺省全部有默认值；也可用环境变量 `STORY_DATA_DIR` / `STORY_BL_BIN` / `STORY_FFMPEG_BIN` / `STORY_TTS_VOICE` / `STORY_MAX_CONCURRENCY` 覆盖）：

```yaml
data_dir: data
bl_bin: bl
ffmpeg_bin: ffmpeg
subtitle_font: ""          # 留空自动探测系统中文字体
text_model: ""             # 留空用 bl 默认
video_model: ""
tts_model: cosyvoice-v3-flash
tts_voice: longtian_v3
tts_instruction: 请用沉稳厚重、富有历史讲述感的语调，语速从容不迫，像学者在讲历史故事，不要播报腔。
default_ratio: 9:16
default_resolution: 1080P  # 档位指长边：1080P → 1080×1920，720P → 720×1280
max_concurrency: 3
max_retries: 3
```

## 使用流程

```bash
# 1. 创建系列
./story series create --name 鬼谷子 --dynasty 战国 \
  --ratio 9:16 --resolution 1080P --platforms douyin,kuaishou,bilibili
./story series list
./story series show <series-id>

# 2. 创建一集（--topic 可空，由 AI 自由命题）
./story episode create --series <series-id> --title "入秦" --topic "苏秦张仪出山之前"

# 3. 六步流水线（每步产物落 SQLite + 工作目录，失败可重入续跑）
./story candidates <episode-id>           # ① AI 出 3-5 个候选故事
./story pick <episode-id> --index 2       # ② 选定一个（可先人工改 story.md 后再继续）
./story storyboard <episode-id>           # ③ 朝代锚定分镜（storyboard.json 可人工审阅）
./story produce <episode-id>              # ④ 并发出视频片段+旁白，已存在文件自动复用
./story compose <episode-id>              # ⑤ 归一化+拼接+烧硬字幕
./story status <episode-id>               # 随时查看各步骤状态/错误/重试次数

# ② 之后也可一键跑到底（供 Agent 自动驱动）：
./story run <episode-id> [--index N]

# 4. 导出其他比例（模糊背景填充，不重复生成视频）
./story export <episode-id> --ratio 16:9
```

产物布局（`data/` 不入库）：

```
data/
├── story.db
└── projects/<series-id>/<episode-id>/
    ├── story.md            # 人工审阅副本（事实源在 DB）
    ├── storyboard.json
    ├── clips/              # AI 视频片段
    ├── audio/              # TTS 旁白
    ├── tmp/                # 归一化中间件、subs.srt、concat 清单、字幕 PNG
    └── output/             # 成片与多比例导出
```

## Web 界面

除 CLI 外提供 Web UI：预览系列/集/候选故事/故事正文/分镜/视频片段/旁白/成片，在线播放媒体，并可直接点按钮触发各流水线动作（SSE 实时刷新状态，无需轮询；同一集同时只跑一个任务）。

```bash
# 方式一：使用已嵌入前端的单二进制（仓库 web/dist 已含构建产物）
GOTOOLCHAIN=local go build -o story ./cmd/story
./story serve                     # http://127.0.0.1:7878 （--addr 可改）

# 方式二：前端开发热更新
./story serve                     # 终端 1：只跑 API 也可
cd web && npm install && npm run dev   # 终端 2：http://127.0.0.1:5173 （自动代理 /api）
```

修改前端后执行 `cd web && npm run build` 重新生成 `web/dist/`，再 go build 即把新界面嵌入二进制。页面包括：

- **系列页**：系列卡片、新建系列（朝代/比例/分辨率/平台）
- **系列详情**：配置信息、集列表与各集流水线状态、新建集
- **集详情**：5 步流水线状态条、候选故事卡（展开正文/在线选定）、故事正文、分镜卡（画面提示/旁白/运镜/时长 + 竖屏视频播放器 + 旁白音频）、成片播放器与 16:9 / 1:1 / 3:4 一键导出、失败原因展示

## 说明

- 字幕：纯 Go 渲染透明 PNG → ffmpeg `overlay` 按镜头时间区间叠加；同时保留 SRT 旁路。无 libass 的 ffmpeg 构建也能烧录。
- 配音：默认系统音色 `longtian_v3` + 风格指令；**不做真人声音克隆**。
- 水印：保留 AI 生成水印（合规）。
- 真实 `bl` 调用会产生 API 费用；produce 阶段耗时较长，断点续跑可随时 Ctrl-C 后重试。

## 测试

```bash
GOTOOLCHAIN=local go test ./...                          # 单元测试（mock provider，不触网）
GOTOOLCHAIN=local go test -tags=integration ./...        # 含真实 ffmpeg 的集成测试
GOTOOLCHAIN=local go vet ./...
```

## 架构约束

详见 [AGENTS.md](./AGENTS.md)：engine/domain/port 禁止 import 任何具体 provider；替换 AI 供应商只需新增一个 port 实现并在 `internal/app` 装配，engine 与 CLI 零改动。
