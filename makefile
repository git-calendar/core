BUILD_DIR := ./build

# -------------------------------------------------------------------------------------------
# regular builds
# -------------------------------------------------------------------------------------------

all: build_android build_ios build_web

build_android: create_build_dir
	gomobile bind -target=android -androidapi=35 -o ${BUILD_DIR}/android/core.aar ./core

# requires Xcode installed (mac only)
build_ios: create_build_dir
 	gomobile bind -target=ios ./core # TODO

build_web: create_build_dir
	GOOS=js GOARCH=wasm go build -o ${BUILD_DIR}/web/core.wasm .              # build the wasm
	cp $$(go env GOROOT)/lib/wasm/wasm_exec.js ${BUILD_DIR}/web/wasm_exec.js  # copy wasm_exec.js "glue"



# -------------------------------------------------------------------------------------------
# production builds (ldflags make the binary smaller)
# -------------------------------------------------------------------------------------------

prod: production_build_android production_build_web

production_build_android: create_build_dir
	gomobile bind -target=android -androidapi=35 -ldflags="-s -w" -o ${BUILD_DIR}/android/core.aar ./core

production_build_web: create_build_dir
	GOOS=js GOARCH=wasm go build -ldflags "-w -s" -o ${BUILD_DIR}/web/core.wasm .  # build the wasm
	cp $$(go env GOROOT)/lib/wasm/wasm_exec.js ${BUILD_DIR}/web/wasm_exec.js       # copy wasm_exec.js "glue"



# -------------------------------------------------------------------------------------------
# other
# -------------------------------------------------------------------------------------------

create_build_dir:
	mkdir -p ${BUILD_DIR}/android
	mkdir -p ${BUILD_DIR}/ios
	mkdir -p ${BUILD_DIR}/web

clean:
	rm -rf ${BUILD_DIR}
	gomobile clean
