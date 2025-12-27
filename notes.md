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

### Encryption
  - why:
    - to hide data when using a public GitHub/GitLab/Gitea/Codeberg instance as a git remote
  - values-only encryption might be best for this
    - json structure stays
      - git diffs work well (unlike whole file encryption)
    - something like this:
    ```json
    {
      "title": "Meeting", 
      "location": "https://zoom.us/my/abcd", 
      "from": "2011-10-05T14:48:00.000Z",
      ...
    }
              |         - something like AES (GCM mode) or XChaCha20 encryption
              v         - base64 representation
    {
      "title": "/sNrzDJP/K1mmAI6LkBOk3Rv4+JeQQ==", 
      "location": "29J6yCgbtHpeoPCr6pRB9Z8yrmNswW4n5xOFRK1IvGwduFtkljE=", 
      "from": "gZY/iXYQq3gU+sv38NsG4sh7sSw+kjqMttCEhnT8HQ9orN/XIGsg",
      ...
    }
    ```
  - Workflow
1. git-calendar-core will pull changes to disk (FileSystem) (should work all right with this approach)
2. decrypts data using a key stored somewhere to memory/a separate folder on disk
3. makes changes
4. encrypts repository
5. pushes to remote
