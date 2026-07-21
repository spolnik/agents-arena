import { copyFile, mkdir } from "node:fs/promises";

await mkdir(new URL("../webui/static/vendor/", import.meta.url), { recursive: true });
await copyFile(
  new URL("../node_modules/htmx.org/dist/htmx.min.js", import.meta.url),
  new URL("../webui/static/vendor/htmx.min.js", import.meta.url),
);
