/// <reference lib="webworker" />
// @ts-ignore
import wasmUrl from './wasm/api.wasm?url';
// @ts-ignore
import wasmExecCode from './wasm/wasm_exec.js?raw';

const ctx: Worker = self as any;

async function startGo() {
  try {
    // create a opfs handle
    await initOPFS();

    // inject Go Glue
    (0, eval)(wasmExecCode);
    const go = new (self as any).Go();

    // define the ready callback for Go
    (self as any).onWasmReady = () => {
      ctx.postMessage({ type: 'wasm_ready' });
    };

    // load and run Wasm
    const response = await fetch(wasmUrl);
    const buffer = await response.arrayBuffer();
    const { instance } = await WebAssembly.instantiate(buffer, go.importObject);

    // start Go (non-blocking)
    go.run(instance);
  } catch (err: any) {
    ctx.postMessage({ type: 'wasm_error', error: err.message });
  }
}

let opfsRootHandle: FileSystemDirectoryHandle | null = null;
async function initOPFS() {
  opfsRootHandle = await navigator.storage.getDirectory();
  // Expose for Go
  (self as any).opfsRootHandle = opfsRootHandle;
}

// Listen for function calls from the Proxy
ctx.onmessage = async (e: MessageEvent) => {
  const { id, method, args } = e.data;

  // Handle initialization
  if (method === undefined && e.data.type === undefined) {
    // This is a catch-all for the very first ping if needed
    return;
  }

  try {
    // self.api was set by Go's RegisterCallbacks
    const goFunc = (self as any).api[method];
    if (!goFunc) throw new Error(`Go method "${method}" not found`);

    // Call the Go function (which returns a Promise via wrapPromise)
    const result = await goFunc(...args);

    ctx.postMessage({ id, result });
  } catch (err: any) {
    ctx.postMessage({ id, error: err.message });
  }
};

// Auto-start when worker is created
startGo();
