const fs = require("fs");
const path = require("path");

const projectRoot = path.resolve(__dirname, "..");
const sourceDir = path.join(projectRoot, "src");
const outputDir = path.join(projectRoot, "dist");

fs.mkdirSync(outputDir, { recursive: true });
fs.copyFileSync(path.join(sourceDir, "index.js"), path.join(outputDir, "index.js"));

console.log("Build complete: dist/index.js");
