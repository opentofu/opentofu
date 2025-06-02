import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
import path from "node:path";

export default defineConfig({
	plugins: [react(), tailwindcss()],
	resolve: {
		alias: {
			react: path.resolve("./node_modules/react"),
			"react-dom": path.resolve("./node_modules/react-dom"),
		},
	},
	build: {
		outDir: "dist",
		assetsDir: "assets",
		// Ensure paths work when served from Go embedded filesystem
		base: "./",
		// Generate manifest for asset tracking
		manifest: true,
		rollupOptions: {
			output: {
				// Ensure consistent asset naming for embedding
				assetFileNames: "assets/[name]-[hash].[ext]",
				chunkFileNames: "assets/[name]-[hash].js",
				entryFileNames: "assets/[name]-[hash].js",
			},
		},
	},
	server: {
		// Proxy API calls to Go server during development
		proxy: {
			"/api": {
				target: "http://127.0.0.1:8080",
				changeOrigin: true,
				// Fallback to different ports if 8080 is not available
				configure: (proxy, options) => {
					proxy.on("error", (err, req, res) => {
						console.log("Proxy error, trying different port...");
					});
				},
			},
		},
	},
	// Optimize deps for better dev experience
	optimizeDeps: {
		include: ["react", "react-dom", "@xyflow/react", "zod"],
	},
});
