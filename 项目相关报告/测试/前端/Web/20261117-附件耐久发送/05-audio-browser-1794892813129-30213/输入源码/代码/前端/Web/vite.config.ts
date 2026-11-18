import { defineConfig } from "vite";
import https from "node:https";
import fs from "node:fs";
export default defineConfig({
  publicDir: "Public",
  server: {
    host: "127.0.0.1",
    port: 5178,
    strictPort: true,
    proxy: {
      "/api": {
        target: process.env.JOBSIM_API ?? "https://127.0.0.1:8443",
        secure: true,
        agent: new https.Agent(
          process.env.JOBSIM_CA_FILE
            ? { ca: fs.readFileSync(process.env.JOBSIM_CA_FILE) }
            : {},
        ),
      },
    },
  },
  build: { outDir: "Dist", sourcemap: true },
});
