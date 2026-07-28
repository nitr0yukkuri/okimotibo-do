import { defineConfig } from "vite";
import { VitePWA } from "vite-plugin-pwa";

export default defineConfig({
  plugins: [
    VitePWA({
      registerType: "autoUpdate",
      includeAssets: ["favion.png", "okimochi_logo.png"],
      manifest: {
        name: "おきもちぼ〜ど",
        short_name: "おきもちぼ〜ど",
        description: "今の気持ちをリアルタイムで共有するアプリ",
        theme_color: "#6bdd67",
        background_color: "#ffffff",
        display: "standalone",
        orientation: "portrait",
        scope: "/",
        start_url: "/",
        icons: [
          {
            src: "favion.png",
            sizes: "512x512",
            type: "image/png",
            purpose: "any maskable",
          },
        ],
      },
    }),
  ],
});
