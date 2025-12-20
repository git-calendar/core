declare interface Go {
  importObject: WebAssembly.Imports;
  run(instance: WebAssembly.Instance): Promise<void>;
}

declare global {
  interface Window {
    Go: new () => Go;
  }
  interface WorkerGlobalScope {
    Go: new () => Go;
  }
  // this allows self.Go to work in the worker context
  var Go: new () => Go;
}

export { };
