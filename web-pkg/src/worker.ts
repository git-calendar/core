/// <reference lib="webworker" />
// @ts-ignore
import wasmUrl from './wasm/api.wasm?url';
// @ts-ignore
import wasmExecCode from './wasm/wasm_exec.js?raw';

const ctx: Worker = self as any;

async function startGo() {
  try {
    // 1. Inject Go Glue
    (0, eval)(wasmExecCode);
    const go = new (self as any).Go();

    // 2. Define the ready callback for Go
    (self as any).onWasmReady = () => {
      ctx.postMessage({ type: 'wasm_ready' });
    };

    // 3. Load & Run WASM
    const response = await fetch(wasmUrl);
    const buffer = await response.arrayBuffer();
    const { instance } = await WebAssembly.instantiate(buffer, go.importObject);

    // Start Go (Non-blocking)
    go.run(instance);
  } catch (err: any) {
    ctx.postMessage({ type: 'wasm_error', error: err.message });
  }
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
