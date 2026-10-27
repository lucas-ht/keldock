import { readFileSync } from "node:fs";
import { parse } from "yaml";

const root = new URL("../../", import.meta.url);
const read = (path) => readFileSync(new URL(path, root), "utf8");
const manifest = JSON.parse(read("package.json"));
const mise = read(".mise.toml");
const workflow = parse(read(".github/workflows/ci.yaml"));
const task = parse(read("Taskfile.yaml"));
const lock = parse(read("pnpm-lock.yaml"));
const failures = [];
const expect = (condition, message) => {
  if (!condition) failures.push(message);
};

expect(
  manifest.name === "keldock-repository",
  "Package must use Keldock identity",
);
expect(
  manifest.private === true,
  "Repository tooling must not be an npm publication",
);
expect(
  manifest.license === "AGPL-3.0-only",
  "Preserve the agreed AGPLv3 license",
);
expect(
  manifest.repository?.url === "https://github.com/lucas-ht/keldock.git",
  "Package repository must identify Keldock",
);
expect(
  read("README.md").startsWith("# Keldock\n"),
  "README must identify Keldock",
);
for (const tool of ["node", "pnpm"]) {
  const version = manifest.engines?.[tool];
  expect(/^\d+\.\d+\.\d+$/.test(version), `${tool} must have an exact version`);
  expect(mise.includes(`${tool} = "${version}"`), `${tool} pins must agree`);
}
expect(
  manifest.packageManager === `pnpm@${manifest.engines.pnpm}`,
  "packageManager must match the pnpm engine pin",
);
expect(
  process.versions.node === manifest.engines.node,
  `Use Node ${manifest.engines.node}; running ${process.versions.node}`,
);
for (const group of ["dependencies", "devDependencies"]) {
  for (const [name, version] of Object.entries(manifest[group] ?? {})) {
    expect(
      /^\d+\.\d+\.\d+(?:-[\w.-]+)?$/.test(version),
      `${name} must be pinned`,
    );
    expect(
      lock.importers?.["."]?.[group]?.[name]?.specifier === version,
      `${name} lock specifier must match package.json`,
    );
  }
}
for (const name of ["bootstrap", "doctor", "format", "format:check", "test"]) {
  expect(
    task.tasks?.[name]?.cmds?.length > 0,
    `Task ${name} must execute real checks`,
  );
}
expect(
  JSON.stringify(workflow.permissions) === JSON.stringify({ contents: "read" }),
  "Validation CI must have only contents:read permission",
);
expect(
  !Object.hasOwn(workflow.on, "pull_request_target"),
  "Validation CI must not run through pull_request_target",
);
for (const job of Object.values(workflow.jobs)) {
  expect(!job.permissions, "Jobs must not override the read-only permissions");
  expect(job["timeout-minutes"] > 0, "CI jobs require a bounded timeout");
  for (const step of job.steps) {
    if (!step.uses) continue;
    expect(
      /^[\w./-]+@[a-f0-9]{40}$/.test(step.uses),
      "Actions require full commit pins",
    );
    if (step.uses.startsWith("actions/checkout@")) {
      expect(
        step.with?.["persist-credentials"] === false,
        "Checkout must not persist credentials",
      );
    }
    if (step.uses.startsWith("jdx/mise-action@")) {
      expect(
        /^\d+\.\d+\.\d+$/.test(step.with?.version),
        "CI mise must have an exact version",
      );
    }
  }
}

if (failures.length) {
  process.stderr.write(`${failures.join("\n")}\n`);
  process.exitCode = 1;
} else {
  process.stdout.write(
    "Repository identity, pins, lock and CI contracts verified.\n",
  );
}
