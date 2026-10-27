import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import test from "node:test";
import { checkDocuments } from "../lib/markdown.mjs";

function fixture(t, files) {
  const root = mkdtempSync(join(tmpdir(), "keldock-docs-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  for (const [file, content] of Object.entries(files)) {
    mkdirSync(dirname(join(root, file)), { recursive: true });
    writeFileSync(join(root, file), content);
  }
  return root;
}

test("relative, reference and encoded links resolve against their source document", (t) => {
  const root = fixture(t, {
    "README.md": "# Home\n[guide](docs/guide.md#runtime-api)\n",
    "docs/guide.md":
      "# Runtime `API`\n[asset][image]\n[home](../README.md#home)\n\n[image]: my%20image.svg\n",
    "docs/my image.svg": "<svg/>",
  });
  assert.deepEqual(checkDocuments(root, ["README.md", "docs/guide.md"]), []);
});

test("duplicate headings use GitHub slug suffixes and missing headings fail", (t) => {
  const root = fixture(t, {
    "README.md": "# Same\n# Same\n[ok](#same-1)\n[bad](#same-2)\n",
  });
  const failures = checkDocuments(root, ["README.md"]);
  assert.equal(failures.length, 1);
  assert.match(failures[0], /same-2: Markdown heading does not exist/);
});

test("missing files and image references are both checked", (t) => {
  const root = fixture(t, {
    "README.md": "[file](missing.md)\n![image](missing.svg)\n",
  });
  assert.equal(checkDocuments(root, ["README.md"]).length, 2);
});

test("absolute and encoded parent escapes cannot become machine-specific links", (t) => {
  const root = fixture(t, {
    "README.md": "[absolute](/tmp/outside.md)\n[escape](%2E%2E/outside.md)\n",
  });
  const failures = checkDocuments(root, ["README.md"]);
  assert.equal(failures.length, 2);
  assert.ok(
    failures.every((message) => message.includes("portable and inside")),
  );
});

test("external URLs are not interpreted as local files or fetched by CI", (t) => {
  const root = fixture(t, {
    "README.md":
      "[web](https://example.invalid/page#missing)\n[mail](mailto:example@example.invalid)\n",
  });
  assert.deepEqual(checkDocuments(root, ["README.md"]), []);
});

test("malformed encoding produces a diagnostic rather than aborting all checks", (t) => {
  const root = fixture(t, {
    "README.md": "[bad](bad%FF.md)\n[missing](missing.md)\n",
  });
  const failures = checkDocuments(root, ["README.md"]);
  assert.equal(failures.length, 2);
  assert.ok(failures.some((message) => message.includes("URI malformed")));
});
