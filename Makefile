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
PREFIX       ?= /usr/local
BINDIR       := $(PREFIX)/bin
GOTOOLCHAIN  := local
GO           := GOTOOLCHAIN=$(GOTOOLCHAIN) go
GOFLAGS      :=
VERSION      ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS      := -X main.version=$(VERSION)

WEB_DIR      := web
WEB_DIST     := $(WEB_DIR)/dist/index.html
WEB_DEPS     := $(WEB_DIR)/node_modules/.package-lock.json

# 前端源文件（任一改动即触发重建）
WEB_SRCS     := $(shell find $(WEB_DIR)/src $(WEB_DIR)/index.html $(WEB_DIR)/package.json $(WEB_DIR)/vite.config.ts 2>/dev/null)

# Go 源文件
GO_SRCS      := $(shell find . -name '*.go' -not -path './web/*' 2>/dev/null)

.PHONY: all build web install run serve test vet fmt clean help

# ---- 默认目标 ----
all: build

# ---- 前端 ----
$(WEB_DEPS): $(WEB_DIR)/package-lock.json
	cd $(WEB_DIR) && npm install
	@touch $@

$(WEB_DIST): $(WEB_DEPS) $(WEB_SRCS)
	cd $(WEB_DIR) && npm run build

web: $(WEB_DIST)

# ---- Go 构建 ----
build: $(BINARY)

$(BINARY): $(WEB_DIST) $(GO_SRCS) go.mod go.sum
	$(GO) build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BINARY) ./cmd/story

# ---- 安装 ----
install: build
	@mkdir -p $(BINDIR)
	install -m 0755 $(BINARY) $(BINDIR)/$(BINARY)
	@echo "已安装: $(BINDIR)/$(BINARY)"

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
	@echo "  install         构建并安装到 $(BINDIR)/$(BINARY)"
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
	@echo "  PREFIX          安装前缀（默认 /usr/local）"
	@echo "  GOTOOLCHAIN     Go 工具链（默认 local）"
