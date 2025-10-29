# ====================================================================================
# Makefile for Caddy Custom Build
# ====================================================================================

# --- Variables ---

# 自动从 go.mod 文件获取模块路径，这是最关键的变量。
# 例如: github.com/your-username/caddy-dynamic-sd
MODULE_PATH := $(shell go list -m)

# 输出的二进制文件名
BINARY_NAME := caddy

# 构建产物的输出目录
OUTPUT_DIR := dist

# (可选) 指定要构建的 Caddy 版本，留空则使用最新版
CADDY_VERSION ?=

# Go 可执行文件
GO := go

# --- Targets ---

# 将 "help" 作为默认目标，当只输入 "make" 时会显示帮助信息
.DEFAULT_GOAL := help

# 使用 .PHONY 声明伪目标，防止和文件名冲突
.PHONY: all build linux windows darwin darwin-amd64 darwin-arm64 clean tools help

all: linux windows darwin
	@echo "✅ All targets built successfully in $(OUTPUT_DIR)/"

# 构建适用于当前操作系统和架构的版本
build: tools
	@echo "Building Caddy for current system ($(shell go env GOOS)/$(shell go env GOARCH))..."
	@xcaddy build $(CADDY_VERSION) --with $(MODULE_PATH) --output $(OUTPUT_DIR)/$(BINARY_NAME) # <-- FIX: Changed -o to --output

# --- Cross-compilation Targets ---

# 构建所有 Linux (amd64) 版本
linux: linux-amd64

linux-amd64: tools
	@echo "Cross-compiling Caddy for Linux (amd64)..."
	@GOOS=linux GOARCH=amd64 xcaddy build $(CADDY_VERSION) --with $(MODULE_PATH) --output $(OUTPUT_DIR)/$(BINARY_NAME)-linux-amd64 # <-- FIX: Changed -o to --output

# 构建所有 Windows (amd64) 版本
windows: windows-amd64

windows-amd64: tools
	@echo "Cross-compiling Caddy for Windows (amd64)..."
	@GOOS=windows GOARCH=amd64 xcaddy build $(CADDY_VERSION) --with $(MODULE_PATH) --output $(OUTPUT_DIR)/$(BINARY_NAME)-windows-amd64.exe # <-- FIX: Changed -o to --output

# 构建所有 Darwin (macOS) 版本
darwin: darwin-amd64 darwin-arm64

darwin-amd64: tools
	@echo "Cross-compiling Caddy for macOS (amd64)..."
	@GOOS=darwin GOARCH=amd64 xcaddy build $(CADDY_VERSION) --with $(MODULE_PATH) --output $(OUTPUT_DIR)/$(BINARY_NAME)-darwin-amd64 # <-- FIX: Changed -o to --output

darwin-arm64: tools
	@echo "Cross-compiling Caddy for macOS (arm64)..."
	@GOOS=darwin GOARCH=arm64 xcaddy build $(CADDY_VERSION) --with $(MODULE_PATH) --output $(OUTPUT_DIR)/$(BINARY_NAME)-darwin-arm64 # <-- FIX: Changed -o to --output

# --- Utility Targets ---

# 清理构建目录
clean:
	@echo "Cleaning up build artifacts..."
	@rm -rf $(OUTPUT_DIR)

# 安装必要的构建工具 (xcaddy)
tools:
ifeq (, $(shell which xcaddy))
	@echo "xcaddy not found. Installing..."
	@$(GO) install github.com/caddyserver/xcaddy/cmd/xcaddy@latest
else
	@echo "xcaddy is already installed."
endif

# 显示帮助信息 (通过解析 Makefile 注释实现)
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Available targets:"
	@awk 'BEGIN {FS = ":.*?## "}; /^[\.a-zA-Z0-9_-]+:.*?## / {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

# --- Target Comments for Help ---
## all:                 Builds for all major platforms (Linux, Windows, Darwin).
## build:               Builds Caddy for the current OS and architecture.
## linux:               Cross-compiles for Linux (amd64).
## windows:             Cross-compiles for Windows (amd64).
## darwin:              Cross-compiles for macOS (amd64 and arm64).
## clean:               Removes the build directory and all compiled binaries.
## tools:               Installs the required build tools (xcaddy).
## help:                Displays this help message.