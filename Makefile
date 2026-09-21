# story Makefile
# 用法: make [target]
# 主要目标:
#   make           构建二进制（含前端嵌入）
#   make install   构建并安装到 $(PREFIX)/bin/story
#   make run       本地运行（开发用）
#   make serve     启动 Web UI 服务
#   make test      运行单元测试
#   make vet       go vet
#   make fmt       gofmt 检查
#   make clean     清理构建产物

# ---- 变量 ----
BINARY       := story
# 安装目录：显式指定 PREFIX 时用 $(PREFIX)/bin；
# 否则自动挑选 PATH 中当前用户可写的 bin 目录（避免 sudo）；都不可写时回退 /usr/local/bin。
PREFIX       ?=
ifneq ($(PREFIX),)
BINDIR       := $(PREFIX)/bin
else
BINDIR       := $(shell for d in /opt/homebrew/bin /usr/local/bin "$$HOME/.local/bin"; do [ -d "$$d" ] && [ -w "$$d" ] && echo "$$d" && break; done)
ifeq ($(BINDIR),)
BINDIR       := /usr/local/bin
endif
endif
GOTOOLCHAIN  := local
GO           := GOTOOLCHAIN=$(GOTOOLCHAIN) go
GOFLAGS      :=
VERSION      ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS      := -X main.version=$(VERSION)

WEB_DIR      := web
WEB_DIST     := $(WEB_DIR)/dist/index.html
WEB_DEPS     := $(WEB_DIR)/node_modules/.package-lock.json

# 前端源文件（任一改动即触发重建）
WEB_SRCS     := $(shell find $(WEB_DIR)/src $(WEB_DIR)/public $(WEB_DIR)/index.html $(WEB_DIR)/package.json $(WEB_DIR)/vite.config.ts 2>/dev/null)

# Go 源文件
GO_SRCS      := $(shell find . -name '*.go' -not -path './web/*' 2>/dev/null)

.PHONY: all build web install uninstall run serve web-dev test test-integration vet fmt fmt-fix clean clean-all help

# ---- 默认目标 ----
all: build

# ---- 前端 ----
$(WEB_DEPS): $(WEB_DIR)/package-lock.json
	cd $(WEB_DIR) && npm install
	@touch $@

$(WEB_DIST): $(WEB_DEPS) $(WEB_SRCS)
	cd $(WEB_DIR) && npm run build

# 前端构建：依赖 $(WEB_DIST) —— 源码/依赖任一更新即自动重建（见其规则）。
# 额外兜底：dist 缺失、assets 为空或仍是占位页时强制重建，
# 保证 make build / install 无需人工预构建前端，绝不把占位页嵌进二进制。
web: $(WEB_DIST)
	@if [ -z "$$(ls $(WEB_DIR)/dist/assets 2>/dev/null)" ] || grep -q story-web-placeholder $(WEB_DIST) 2>/dev/null; then \
		echo ">>> 前端产物缺失/不完整/为占位页，重新构建..."; \
		cd $(WEB_DIR) && npm run build; \
	fi

# ---- Go 构建 ----
build: web $(BINARY)

$(BINARY): $(WEB_DIST) $(GO_SRCS) go.mod go.sum .version-stamp
	$(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BINARY) ./cmd/story

# 版本戳记：VERSION 变化时更新文件 mtime 触发重链接，未变化时不动作。
.version-stamp: FORCE
	@[ "$$(cat $@ 2>/dev/null)" = "$(VERSION)" ] || echo "$(VERSION)" > $@

FORCE:

# ---- 安装 ----
install: build
	@mkdir -p $(BINDIR)
	@if [ ! -w "$(BINDIR)" ]; then \
		echo "错误: $(BINDIR) 不可写，请改用: sudo make install PREFIX=/usr/local"; exit 1; \
	fi
	install -m 0755 $(BINARY) $(BINDIR)/$(BINARY)
	@echo "已安装: $(BINDIR)/$(BINARY)"

uninstall:
	rm -f $(BINDIR)/$(BINARY)
	@echo "已卸载: $(BINDIR)/$(BINARY)"

# ---- 运行 ----
run: build
	./$(BINARY)

serve: build
	./$(BINARY) serve

# ---- 开发：前端热重载（需另开终端跑 Go serve） ----
web-dev:
	cd $(WEB_DIR) && npm run dev

# ---- 测试 / 质量 ----
test:
	$(GO) test ./...

test-integration:
	$(GO) test -tags=integration ./...

vet:
	$(GO) vet ./...

fmt:
	@files=$$(gofmt -l .); if [ -n "$$files" ]; then echo "未格式化:"; echo "$$files"; exit 1; else echo "gofmt clean"; fi

fmt-fix:
	gofmt -w .

# ---- 清理 ----
clean:
	rm -f $(BINARY)
	rm -f .version-stamp
	rm -rf bin/
	@echo "已清理 Go 构建产物（保留 web/dist 与 node_modules）"

clean-all: clean
	rm -rf $(WEB_DIR)/dist
	rm -rf $(WEB_DIR)/node_modules

# ---- 帮助 ----
help:
	@echo "story 构建工具"
	@echo ""
	@echo "目标:"
	@echo "  all / build     构建二进制 $(BINARY)（含前端嵌入）"
	@echo "  web             仅构建前端到 $(WEB_DIR)/dist"
	@echo "  install         完整安装：重建前端（源码变更/缺失/占位页）+ 编译 + 安装"
	@echo "  uninstall       卸载已安装的二进制"
	@echo "  run             构建并运行"
	@echo "  serve           构建并启动 Web UI 服务"
	@echo "  web-dev         前端开发服务器（Vite 热重载）"
	@echo "  test            运行单元测试"
	@echo "  test-integration 运行集成测试（需 ffmpeg）"
	@echo "  vet             go vet 静态检查"
	@echo "  fmt             gofmt 格式检查"
	@echo "  fmt-fix         gofmt 自动格式化"
	@echo "  clean           清理 Go 构建产物（保留 web/dist）"
	@echo "  clean-all       清理构建产物 + web/dist + node_modules"
	@echo ""
	@echo "变量:"
	@echo "  PREFIX          安装前缀（默认自动探测可写 bin；如 make install PREFIX=\$$HOME/.local）"
	@echo "  BINDIR          直接指定安装目录（优先级高于 PREFIX）"
	@echo "  VERSION         注入的版本号（默认 git describe）"
	@echo "  GOTOOLCHAIN     Go 工具链（默认 local）"
