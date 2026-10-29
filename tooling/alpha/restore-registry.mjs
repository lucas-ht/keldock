import { execFileSync } from "node:child_process";
import { mkdirSync } from "node:fs";
import { resolve } from "node:path";

const run = (args) =>
  execFileSync("docker", args, {
    encoding: "utf8",
    stdio: ["ignore", "pipe", "pipe"],
  });
const name = "keldock-m6-recovered-registry";
let existing;
try {
  existing = JSON.parse(run(["inspect", name]))[0];
} catch {}
if (existing && existing.Config.Labels["keldock.alpha-fixture"] !== "true")
  throw new Error("Recovery registry ownership mismatch");
if (!existing) {
  const path = resolve(".local/m6/recovered-registry");
  mkdirSync(path, { recursive: true, mode: 0o700 });
  run([
    "run",
    "-d",
    "--name",
    name,
    "--label",
    "keldock.alpha-fixture=true",
    "--network",
    "keldock-m1",
    "--user",
    `${process.getuid()}:${process.getgid()}`,
    "-p",
    "127.0.0.1:5012:5000",
    "--mount",
    `type=bind,source=${path},target=/var/lib/registry`,
    "registry:3.0.0@sha256:6c5666b861f3505b116bb9aa9b25175e71210414bd010d92035ff64018f9457e",
  ]);
}
let ready = false;
for (let attempt = 0; attempt < 20; attempt++) {
  try {
    const response = await fetch("http://127.0.0.1:5012/v2/", {
      signal: AbortSignal.timeout(3000),
    });
    if (response.ok) {
      ready = true;
      break;
    }
  } catch {}
  await new Promise((done) => setTimeout(done, 250));
}
if (!ready) throw new Error("Isolated recovery registry is unavailable");
process.stdout.write(
  "Target: owned isolated recovery registry 127.0.0.1:5012; no source-registry changes or deletions.\n",
);
