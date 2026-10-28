import { execFileSync } from "node:child_process";
import { writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { profile } from "./profiles.mjs";

if (profile.cluster !== "keldock-m6-core")
  throw new Error("Measure only the explicitly constrained core fixture");
const run = (args) => execFileSync("docker", args, { encoding: "utf8" });
const node = `k3d-${profile.cluster}-server-0`;
const config = JSON.parse(run(["inspect", node]))[0];
if (
  config.Config.Labels["keldock.alpha-fixture"] !== "true" ||
  config.HostConfig.Memory !== 4294967296 ||
  config.HostConfig.NanoCpus !== 2000000000
)
  throw new Error("Expected exact owned 2 CPU / 4 GiB fixture limit");
const samples = [];
for (let sample = 0; sample < 4; sample++) {
  const value = (name) =>
    run(["exec", node, "cat", `/sys/fs/cgroup/${name}`]).trim();
  samples.push({
    at: new Date().toISOString(),
    memoryBytes: Number(value("memory.current")),
    peakBytes: Number(value("memory.peak")),
    cpu: value("cpu.stat"),
  });
  if (sample < 3) await new Promise((done) => setTimeout(done, 10000));
}
const result = {
  target: profile.context,
  cpuLimit: 2,
  memoryLimitBytes: 4294967296,
  samples,
};
writeFileSync(
  resolve(profile.directory, "resource-measurements.json"),
  JSON.stringify(result, null, 2) + "\n",
  { mode: 0o600 },
);
process.stdout.write(
  `Measured the complete constrained core node: current maximum ${Math.ceil(Math.max(...samples.map((item) => item.memoryBytes)) / 1024 / 1024)} MiB, cgroup lifetime peak ${Math.ceil(Math.max(...samples.map((item) => item.peakBytes)) / 1024 / 1024)} MiB. Includes K3s, controllers, packaged services and active fixture workloads; not a production load test.\n`,
);
