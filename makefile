BUILD_DIR ?= build
ANDROID_API ?= 31
LDFLAGS ?=
TOOLS_DIR := $(abspath $(BUILD_DIR)/tools)

.PHONY: all prod_build tools build_android build_ios build_web clean

all: build_android build_ios build_web

prod_build:
	$(MAKE) all LDFLAGS="-s -w"

# gomobile needs gobind on PATH
tools:
	go build -o $(TOOLS_DIR)/gobind golang.org/x/mobile/cmd/gobind

# 16 KB page alignment for Android 15+ devices, older NDKs (< r28) default to 4 KB
build_android: tools
	mkdir -p $(BUILD_DIR)/android
	CGO_LDFLAGS="-Wl,-z,max-page-size=16384" PATH="$(TOOLS_DIR):$$PATH" go tool gomobile bind -target=android -androidapi=$(ANDROID_API) -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/android/core.aar ./pkg/api

build_ios: tools
	mkdir -p $(BUILD_DIR)/ios
	PATH="$(TOOLS_DIR):$$PATH" go tool gomobile bind -target=ios -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/ios/Core.xcframework ./pkg/api

build_web:
	mkdir -p $(BUILD_DIR)/web
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" $(BUILD_DIR)/web/
	GOOS=js GOARCH=wasm go build -ldflags="$(LDFLAGS)" -o $(BUILD_DIR)/web/core.wasm ./cmd/wasm

clean:
	rm -rf $(BUILD_DIR)
