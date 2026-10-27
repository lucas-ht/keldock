import { execFileSync } from "node:child_process";
import { existsSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { checkDocuments } from "./lib/markdown.mjs";

const root = fileURLToPath(new URL("../../", import.meta.url));
const files = execFileSync(
  "git",
  ["ls-files", "--cached", "--others", "--exclude-standard", "-z"],
  { cwd: root, encoding: "utf8" },
)
  .split("\0")
  .filter((file) => file.endsWith(".md") && existsSync(resolve(root, file)));
const failures = checkDocuments(root, files);
if (failures.length) {
  process.stderr.write(`${failures.join("\n")}\n`);
  process.exitCode = 1;
} else {
  process.stdout.write(`Checked ${new Set(files).size} Markdown files.\n`);
}
