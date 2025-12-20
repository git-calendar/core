import type { GoApi } from './types';
// @ts-ignore - Vite inlines the worker code
import MyWorker from './worker?worker&inline';

let worker: Worker | null = null;
const pending = new Map<string, { resolve: Function; reject: Function }>();

export const initWasm = (): Promise<void> => {
  return new Promise((resolve, reject) => {
    if (worker) return resolve();

    worker = new MyWorker();

    worker.onmessage = (e: MessageEvent) => {
      const { type, id, result, error } = e.data;

      if (type === 'wasm_ready') return resolve();
      if (type === 'wasm_error') return reject(error);

      // Handle the function response
      const promise = pending.get(id);
      if (promise) {
        if (error) promise.reject(new Error(error));
        else promise.resolve(result);
        pending.delete(id);
      }
    };
  });
};

// The "Magic" Proxy
export const api = new Proxy({} as GoApi, {
  get(_, method: string) {
    return (...args: any[]) => {
      return new Promise((resolve, reject) => {
        if (!worker) return reject("Call initWasm() first");

        const id = crypto.randomUUID();
        pending.set(id, { resolve, reject });

        // Tell the worker to call the Go function
        worker.postMessage({ id, method, args });
      });
    };
  }
});
