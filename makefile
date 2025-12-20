BUILD_DIR := ./build
WEB_BUILD_DIR := ./web-pkg

# all: build_android build_ios

build_android: create_build_dir
	gomobile bind -target=android -androidapi=35 -o ${BUILD_DIR}/android/gitcalendarcore.aar .

build_web: create_build_dir
	GOOS=js GOARCH=wasm go build -o ${BUILD_DIR}/web/api.wasm .                                             # build the wasm into build directory
	cp ${BUILD_DIR}/web/api.wasm ${WEB_BUILD_DIR}/src/wasm/api.wasm                                         # copy the built wasm to web-pkg
	cp /opt/homebrew/Cellar/go/1.25.5/libexec/lib/wasm/wasm_exec.js ${WEB_BUILD_DIR}/src/wasm/wasm_exec.js  # copy wasm_exec.js "glue" to web-pkg (TODO make not brew only path)

publish_web: build_web  # TODO
	cd web-pkg && npm run build #&& npm publish

# build_macos: create_build_dir
# 	gomobile bind -target=macos .

# build_ios: create_build_dir
# 	gomobile bind -target=ios -o ${BUILD_DIR} .

# ---- helpers ----
create_build_dir:
	mkdir -p ${BUILD_DIR}/android
	mkdir -p ${BUILD_DIR}/web

clean:
	rm -rf ${BUILD_DIR}
