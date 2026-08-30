import path from "node:path"
import tailwindcss from "@tailwindcss/vite"
import react from "@vitejs/plugin-react"
import { defineConfig } from "vite"

// https://vite.dev/config/
export default defineConfig({
  // GitHub Pages project site (justinstimatze.github.io/lexicon/), not a
  // user/org root site — Vite's default base ("/") makes every emitted
  // asset URL root-absolute, which 404s under this subpath and leaves the
  // page blank (no JS ever loads to hydrate #root).
  base: "/lexicon/",
  plugins: [react(), tailwindcss()],
  resolve: {
    alias: {
      "@": path.resolve(import.meta.dirname, "./src"),
    },
  },
})
