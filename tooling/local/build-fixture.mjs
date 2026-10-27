import { execFileSync } from "node:child_process";
import { copyFileSync, mkdirSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";

const root = resolve(import.meta.dirname, "../..");
process.chdir(root);
mkdirSync(".local/fixture-image", { recursive: true });
const run = (command, args, env = process.env) =>
  execFileSync(command, args, { cwd: root, env, stdio: "inherit" });
const architecture = execFileSync(
  "docker",
  ["info", "--format", "{{.Architecture}}"],
  {
    encoding: "utf8",
  },
).trim();
const target = {
  aarch64: "arm64",
  arm64: "arm64",
  x86_64: "amd64",
  amd64: "amd64",
}[architecture];
if (!target)
  throw new Error(`Unsupported local Docker architecture: ${architecture}`);
copyFileSync(
  "tooling/fixtures/workload/Dockerfile",
  ".local/fixture-image/Dockerfile",
);
async function build(revision) {
  run(
    "go",
    [
      "build",
      "-trimpath",
      "-ldflags",
      `-X main.buildRevision=${revision}`,
      "-o",
      ".local/fixture-image/fixture",
      "./tooling/fixtures/workload",
    ],
    { ...process.env, CGO_ENABLED: "0", GOOS: "linux", GOARCH: target },
  );
  const tag = `localhost:5010/keldock/fixture:m1-${revision}`;
  run("docker", ["build", "--tag", tag, ".local/fixture-image"]);
  run("docker", ["push", tag]);
  const response = await fetch(
    `http://127.0.0.1:5010/v2/keldock/fixture/manifests/m1-${revision}`,
    {
      method: "HEAD",
      headers: {
        Accept:
          "application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json",
      },
    },
  );
  const digest = response.headers.get("docker-content-digest");
  if (!response.ok || !/^sha256:[a-f0-9]{64}$/.test(digest ?? ""))
    throw new Error(
      "Fixture registry did not return an immutable image digest",
    );
  return `keldock-m1-registry:5000/keldock/fixture@${digest}`;
}
const image = await build("v1");
const updatedImage = await build("v2");
writeFileSync(
  ".local/fixture-image.json",
  JSON.stringify({ image, updatedImage }),
);
process.stdout.write(
  "Synthetic CI fixture image published by the local test harness.\n",
);
