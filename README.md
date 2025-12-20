### Notes
- when importing to some frontend js framework, the `vite.config.ts` will probably need:
  ```js
  export default defineConfig({
    server: {
      headers: {
        "Cross-Origin-Opener-Policy": "same-origin",
        "Cross-Origin-Embedder-Policy": "require-corp",
      },
    },
  });
  ```
