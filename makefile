GO ?= go
BUILD_DIR ?= ./build
ANDROID_API ?= 35

ANDROID_BUILD_DIR := $(BUILD_DIR)/android
IOS_BUILD_DIR := $(BUILD_DIR)/ios
WEB_BUILD_DIR := $(BUILD_DIR)/web
MOBILE_TOOLS_DIR := $(abspath $(BUILD_DIR)/tools)
GOROOT := $(shell $(GO) env GOROOT)

.PHONY: all mobile_tools build_android build_ios build_web prod_build prod_build_android prod_build_ios prod_build_web wasm_glue clean help

all: build_android build_ios build_web

mobile_tools:
	mkdir -p $(MOBILE_TOOLS_DIR)
	$(GO) build -o $(MOBILE_TOOLS_DIR)/gobind golang.org/x/mobile/cmd/gobind

build_android: mobile_tools
	mkdir -p $(ANDROID_BUILD_DIR)
	PATH="$(MOBILE_TOOLS_DIR):$$PATH" $(GO) tool gomobile bind -target=android -androidapi=$(ANDROID_API) -o $(ANDROID_BUILD_DIR)/core.aar ./pkg/api

build_ios: mobile_tools
	mkdir -p $(IOS_BUILD_DIR)
	PATH="$(MOBILE_TOOLS_DIR):$$PATH" $(GO) tool gomobile bind -target=ios -o $(IOS_BUILD_DIR)/Core.xcframework ./pkg/api

build_web: wasm_glue
	GOOS=js GOARCH=wasm $(GO) build -o $(WEB_BUILD_DIR)/core.wasm ./cmd/wasm

prod_build: prod_build_android prod_build_ios prod_build_web

prod_build_android: mobile_tools
	mkdir -p $(ANDROID_BUILD_DIR)
	PATH="$(MOBILE_TOOLS_DIR):$$PATH" $(GO) tool gomobile bind -target=android -androidapi=$(ANDROID_API) -ldflags="-s -w" -o $(ANDROID_BUILD_DIR)/core.aar ./pkg/api

prod_build_ios: mobile_tools
	mkdir -p $(IOS_BUILD_DIR)
	PATH="$(MOBILE_TOOLS_DIR):$$PATH" $(GO) tool gomobile bind -target=ios -ldflags="-s -w" -o $(IOS_BUILD_DIR)/Core.xcframework ./pkg/api

prod_build_web: wasm_glue
	GOOS=js GOARCH=wasm $(GO) build -ldflags="-s -w" -o $(WEB_BUILD_DIR)/core.wasm ./cmd/wasm

wasm_glue:
	mkdir -p $(WEB_BUILD_DIR)
	cp "$(GOROOT)/lib/wasm/wasm_exec.js" $(WEB_BUILD_DIR)/wasm_exec.js

clean:
	rm -rf $(BUILD_DIR)

help:
	@echo "Available targets:"
	@echo "  all                  - build Android, iOS, and Web"
	@echo "  build_android        - build the Android AAR"
	@echo "  build_ios            - build the iOS XCFramework (macOS only)"
	@echo "  build_web            - build the WASM module and glue"
	@echo "  prod_build           - build stripped Android, iOS, and Web artifacts"
	@echo "  prod_build_android   - build the stripped Android AAR"
	@echo "  prod_build_ios       - build the stripped iOS XCFramework (macOS only)"
	@echo "  prod_build_web       - build the stripped WASM module and glue"
	@echo "  clean                - remove build artifacts"
