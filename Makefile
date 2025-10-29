# ====================================================================================
# Makefile for Caddy Custom Build (FINAL & ROBUST)
# ====================================================================================

# --- Variables ---

# FIX: Hardcode the module path to avoid shell/environment issues.
# This is the most robust way to ensure the build command is correct.
MODULE_PATH := github.com/liuxd6825/caddy-plus

# 输出的二进制文件名
BINARY_NAME := caddy

# 构建产物的输出目录
OUTPUT_DIR := dist

# (可选) 指定要构建的 Caddy 版本
CADDY_VERSION ?=

# Go 可执行文件
GO := go

# --- Targets ---
.DEFAULT_GOAL := help
.PHONY: all build linux windows darwin darwin-amd64 darwin-arm64 clean tools help

all: linux windows darwin
	@echo "✅ All targets built successfully in $(OUTPUT_DIR)/"

build: tools
	@echo "Building Caddy for current system ($(shell go env GOOS)/$(shell go env GOARCH))..."
	@xcaddy build $(CADDY_VERSION) --with $(MODULE_PATH)=. --output $(OUTPUT_DIR)/$(BINARY_NAME)

linux: linux-amd64
linux-amd64: tools
	@echo "Cross-compiling Caddy for Linux (amd64)..."
	@GOOS=linux GOARCH=amd64 xcaddy build $(CADDY_VERSION) --with $(MODULE_PATH)=. --output $(OUTPUT_DIR)/$(BINARY_NAME)-linux-amd64

windows: windows-amd64
windows-amd64: tools
	@echo "Cross-compiling Caddy for Windows (amd64)..."
	@GOOS=windows GOARCH=amd64 xcaddy build $(CADDY_VERSION) --with $(MODULE_PATH)=. --output $(OUTPUT_DIR)/$(BINARY_NAME)-windows-amd64.exe

darwin: darwin-amd64 darwin-arm64
darwin-amd64: tools
	@echo "Cross-compiling Caddy for macOS (amd64)..."
	@GOOS=darwin GOARCH=amd64 xcaddy build $(CADDY_VERSION) --with $(MODULE_PATH)=. --output $(OUTPUT_DIR)/$(BINARY_NAME)-darwin-amd64

darwin-arm64: tools
	@echo "Cross-compiling Caddy for macOS (arm64)..."
	@GOOS=darwin GOARCH=arm64 xcaddy build $(CADDY_VERSION) --with $(MODULE_PATH)=. --output $(OUTPUT_DIR)/$(BINARY_NAME)-darwin-arm64

clean:
	@echo "Cleaning up build artifacts..."
	@rm -rf $(OUTPUT_DIR)

tools:
ifeq (, $(shell which xcaddy))
	@echo "xcaddy not found. Installing..."
	@$(GO) install github.com/caddyserver/xcaddy/cmd/xcaddy@latest
else
	@echo "xcaddy is already installed."
endif

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