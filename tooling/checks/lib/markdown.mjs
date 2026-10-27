import { readFileSync, statSync } from "node:fs";
import { dirname, relative, resolve, sep } from "node:path";
import GithubSlugger from "github-slugger";
import MarkdownIt from "markdown-it";

const parser = new MarkdownIt();

function parse(path) {
  const tokens = parser.parse(readFileSync(path, "utf8"), {});
  const slugger = new GithubSlugger();
  const anchors = new Set();
  for (let index = 0; index < tokens.length; index++) {
    if (tokens[index].type !== "heading_open") continue;
    const text = (tokens[index + 1].children ?? [])
      .filter((token) => ["text", "code_inline"].includes(token.type))
      .map((token) => token.content)
      .join("");
    anchors.add(slugger.slug(text));
  }
  return { tokens, anchors };
}

export function checkDocuments(root, files) {
  const failures = [];
  const documents = new Map();
  function document(path) {
    if (!documents.has(path)) documents.set(path, parse(path));
    return documents.get(path);
  }

  function check(source, href) {
    if (/^[a-z][a-z\d+.-]*:/i.test(href) || href.startsWith("//")) return;
    const label = `${relative(root, source)} -> ${href}`;
    try {
      const hash = href.indexOf("#");
      const target = hash < 0 ? href : href.slice(0, hash);
      const fragment = hash < 0 ? "" : decodeURIComponent(href.slice(hash + 1));
      const decoded = decodeURIComponent(target.split("?")[0]);
      const path = decoded ? resolve(dirname(source), decoded) : source;
      const location = relative(root, path);
      if (
        decoded.startsWith("/") ||
        location === ".." ||
        location.startsWith(`..${sep}`)
      ) {
        failures.push(
          `${label}: links must stay portable and inside the repository`,
        );
        return;
      }
      const info = statSync(path);
      if (fragment && info.isFile() && path.endsWith(".md")) {
        if (!document(path).anchors.has(fragment)) {
          failures.push(`${label}: Markdown heading does not exist`);
        }
      }
    } catch (error) {
      failures.push(`${label}: ${error.message}`);
    }
  }

  for (const file of new Set(files)) {
    const path = resolve(root, file);
    try {
      for (const token of document(path).tokens) {
        for (const child of token.children ?? []) {
          if (child.type === "link_open") check(path, child.attrGet("href"));
          if (child.type === "image") check(path, child.attrGet("src"));
        }
      }
    } catch (error) {
      failures.push(`${file}: ${error.message}`);
    }
  }
  return failures;
}
