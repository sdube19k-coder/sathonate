import fs from "node:fs";
import path from "node:path";
import { createRequire } from "node:module";
import { pathToFileURL } from "node:url";

const [directory, dependencies] = process.argv.slice(2);
if (!directory || !dependencies) throw new Error("Provide handbook and tool dependency directories");
const require = createRequire(path.resolve(dependencies, "package.json"));
const { JSDOM } = require("jsdom");
const dom = new JSDOM();
globalThis.window = dom.window;
globalThis.document = dom.window.document;
const entry = path.resolve(dependencies, "node_modules/mermaid/dist/mermaid.esm.mjs");
const { default: mermaid } = await import(pathToFileURL(entry).href);
mermaid.initialize({ startOnLoad: false, securityLevel: "strict" });
let total = 0;
for (const file of fs.readdirSync(directory).filter((name) => name.endsWith(".md"))) {
  const content = fs.readFileSync(path.join(directory, file), "utf8");
  for (const match of content.matchAll(/^`{3}mermaid\s*\r?\n([\s\S]*?)^\x60{3}\s*$/gm)) {
    try {
      await mermaid.parse(match[1]);
    } catch (error) {
      throw new Error(`${file} diagram ${total + 1}: ${error.message}`);
    }
    total++;
  }
}
if (total === 0) throw new Error("No Mermaid diagrams found");
console.log(`Parsed ${total} Mermaid diagrams.`);
