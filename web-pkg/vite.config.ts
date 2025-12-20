import { defineConfig } from 'vite';
import { resolve } from 'path';
import dts from 'vite-plugin-dts';

export default defineConfig({
  build: {
    lib: {
      entry: resolve(__dirname, 'src/index.ts'),
      name: 'GitCalendarCore',
      fileName: 'index',
      formats: ['es']
    },
    // Force EVERYTHING under 100MB to be inlined as Base64/Data URLs
    assetsInlineLimit: 100000000,
  },
  plugins: [
    dts({
      rollupTypes: true,
      insertTypesEntry: true
    })
  ],
  // This tells Vite how to handle the worker bundling
  worker: {
    format: 'es',
  }
});
