import { execFileSync } from "node:child_process";
import {
  mkdirSync,
  readFileSync,
  writeFileSync,
  cpSync,
  readdirSync,
  statSync,
} from "node:fs";
import { resolve, relative, dirname } from "node:path";
import { createHash } from "node:crypto";
import { gzipSync } from "node:zlib";
import { parse } from "yaml";
import { renderDependencies } from "./dependencies.mjs";
import { renderFlux } from "./flux.mjs";

const root = resolve(import.meta.dirname, "../..");
process.chdir(root);
const args = process.argv.slice(2);
const option = (name) => {
  const at = args.indexOf(name);
  return at < 0 ? "" : (args[at + 1] ?? "");
};
const runtime = option("--runtime-image"),
  consoleImage = option("--console-image");
for (const image of [runtime, consoleImage])
  if (!/^[a-z0-9][a-z0-9./:_-]*@sha256:[a-f0-9]{64}$/u.test(image))
    throw new Error(
      "Reviewed immutable runtime and console images are required",
    );
const run = (name, args, options = {}) =>
  execFileSync(name, args, {
    cwd: root,
    encoding: "utf8",
    maxBuffer: 32 * 1024 * 1024,
    ...options,
  });
if (run("git", ["status", "--porcelain"]).trim())
  throw new Error("Package a clean committed source tree");
const revision = run("git", ["rev-parse", "HEAD"]).trim();
const chart = parse(
  readFileSync("packaging/charts/keldock/Chart.yaml", "utf8"),
);
if (chart.version !== chart.appVersion)
  throw new Error("Chart and runtime release identity must agree");
const directory = resolve(
  option("--directory") ||
    `.local/m6/release-kit/${chart.version}-${revision.slice(0, 12)}`,
);
mkdirSync(dirname(directory), { recursive: true, mode: 0o700 });
mkdirSync(directory, { mode: 0o700 });
const write = (name, value) => writeFileSync(resolve(directory, name), value);
write(
  "release.json",
  JSON.stringify(
    { version: chart.version, revision, runtimeImage: runtime, consoleImage },
    null,
    2,
  ) + "\n",
);
const flags = `-s -w -X github.com/lucas-ht/keldock/internal/buildinfo.Version=${chart.version} -X github.com/lucas-ht/keldock/internal/buildinfo.Revision=${revision}`;
for (const os of ["darwin", "linux"])
  for (const arch of ["amd64", "arm64"]) {
    const name = `keldock-${os}-${arch}`;
    process.stdout.write(`Building ${name} from ${revision}\n`);
    run(
      "go",
      [
        "build",
        "-p",
        "2",
        "-trimpath",
        "-ldflags",
        flags,
        "-o",
        resolve(directory, name),
        "./cmd/keldock",
      ],
      { env: { ...process.env, CGO_ENABLED: "0", GOOS: os, GOARCH: arch } },
    );
  }
run("helm", [
  "package",
  "packaging/charts/keldock",
  "--destination",
  directory,
]);
const dependencies = resolve(directory, "dependencies");
mkdirSync(dependencies);
renderFlux(resolve(dependencies, "flux.yaml"));
await renderDependencies(dependencies);
cpSync(
  "packaging/dependencies/flux.lock.json",
  resolve(dependencies, "flux.lock.json"),
);
cpSync(
  "internal/cloud/crossplane/controllers.lock.json",
  resolve(dependencies, "controllers.lock.json"),
);
for (const path of ["LICENSE", "skills"]) {
  cpSync(path, resolve(directory, path), { recursive: true });
}
const source = execFileSync(
  "git",
  ["archive", "--format=tar", `--prefix=keldock-${chart.version}/`, revision],
  { cwd: root, maxBuffer: 64 * 1024 * 1024 },
);
write(`keldock-${chart.version}-source.tar.gz`, gzipSync(source));
const sourceDirectory = resolve(directory, "source");
mkdirSync(sourceDirectory);
run("tar", ["-xf", "-", "-C", sourceDirectory], { input: source });
const guide = `source/keldock-${chart.version}/docs/runtime/alpha`;
write(
  "README.md",
  `# Keldock ${chart.version} candidate\n\nSource revision: ${revision}. This kit is unpublished; its image addresses are the reviewed candidate's addresses, not public availability.\n\n- [Install and onboard](${guide}/installation.md)\n- [Supported profile and measured limits](${guide}/support.md)\n- [PostgreSQL and backups](${guide}/postgresql.md)\n- [Control-plane recovery and retention](${guide}/recovery.md)\n- [Independent human operator walkthrough](${guide}/walkthrough.md)\n\nVerify SHA256SUMS before use. The complete extracted source keeps guide links and the thin vendorable Action available offline. Native recovery requires PostgreSQL 18 clients on PATH; they are included in the runtime image. The bundled CLI targets are cross-builds; consult the tested architecture limits.\n`,
);
const members = [];
function walk(path) {
  for (const entry of readdirSync(path, { withFileTypes: true })) {
    const member = resolve(path, entry.name);
    if (entry.isDirectory()) walk(member);
    else if (entry.isFile()) members.push(member);
    else throw new Error("Unexpected release member");
  }
}
walk(directory);
write(
  "SHA256SUMS",
  members
    .sort()
    .map(
      (path) =>
        `${createHash("sha256").update(readFileSync(path)).digest("hex")}  ${relative(directory, path)}`,
    )
    .join("\n") + "\n",
);
const bytes = members.reduce((sum, path) => sum + statSync(path).size, 0);
process.stdout.write(
  `Prepared ${members.length} local release members (${Math.ceil(bytes / 1024 / 1024)} MiB) in ${directory}. Nothing was published.\n`,
);
