This folder is a NPM package for git-calendar-core. 

It containes a wrapper layer around the built Wasm to make it easier to import in Javascript/Typescript projects.


It aims to be used as easy as:
```sh
npm install @firu11/git-calendar-core/web-pkg
```
```ts
import { onMounted } from 'vue';
import { api, initWasm } from '@firu11/git-calendar-core';

onMounted(async () => {
  await initWasm();
  try {
    await api.addEvent(JSON.stringify({ "id": 1, "from": 1, "to": 2, ... }))
  } catch (error) {
    console.log(error)
  }
})
```

I just vibe-coded the `worker.ts` and `index.ts` btw. What the skibidi even is that typescript bullshit...
