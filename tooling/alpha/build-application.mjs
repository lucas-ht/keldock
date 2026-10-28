import { execFileSync } from "node:child_process";
import { mkdirSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
const root = resolve(import.meta.dirname, "../..");
const run = (command, args, capture = false, options = {}) =>
  execFileSync(command, args, {
    cwd: root,
    encoding: "utf8",
    stdio: capture ? ["ignore", "pipe", "inherit"] : "inherit",
    ...options,
  });
const architecture = run(
  "docker",
  ["info", "--format", "{{.Architecture}}"],
  true,
).trim();
const arch =
  architecture === "aarch64"
    ? "arm64"
    : architecture === "x86_64"
      ? "amd64"
      : architecture;
if (!["amd64", "arm64"].includes(arch))
  throw new Error("Unsupported Docker architecture");
const directory = resolve(root, ".local/m6/application");
mkdirSync(directory, { recursive: true, mode: 0o700 });
run(
  "go",
  [
    "build",
    "-buildvcs=false",
    "-trimpath",
    "-ldflags=-s -w",
    "-o",
    resolve(directory, "app"),
    ".",
  ],
  false,
  {
    cwd: resolve(root, "examples/postgresql/application"),
    env: { ...process.env, CGO_ENABLED: "0", GOOS: "linux", GOARCH: arch },
  },
);
writeFileSync(
  resolve(directory, "Dockerfile"),
  'FROM scratch\nCOPY app /app\nUSER 65532:65532\nENTRYPOINT ["/app"]\n',
);
const tag = "localhost:5010/keldock/m6-application:fixture";
run("docker", [
  "build",
  "--platform",
  `linux/${arch}`,
  "--tag",
  tag,
  directory,
]);
run("docker", ["push", tag]);
const digest = JSON.parse(
  run(
    "docker",
    ["image", "inspect", tag, "--format", "{{json .RepoDigests}}"],
    true,
  ),
)[0].split("@")[1];
writeFileSync(
  resolve(root, ".local/m6/application-image.json"),
  JSON.stringify({
    image: `keldock-m1-registry:5000/keldock/m6-application@${digest}`,
  }),
);
