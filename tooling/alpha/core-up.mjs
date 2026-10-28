import { execFileSync } from "node:child_process";
import { mkdirSync, readFileSync, writeFileSync, chmodSync } from "node:fs";
import { resolve } from "node:path";
import { parse, stringify } from "yaml";
import { renderFlux } from "./flux.mjs";

const root = resolve(import.meta.dirname, "../..");
process.chdir(root);
const directory = resolve(".local/m6/core");
mkdirSync(directory, { recursive: true, mode: 0o700 });
const run = (command, args, capture = false) =>
  execFileSync(command, args, {
    encoding: "utf8",
    stdio: capture ? ["ignore", "pipe", "inherit"] : "inherit",
    maxBuffer: 16 * 1024 * 1024,
  });
const name = "keldock-m6-core";
process.stdout.write(
  `Target: new owned ${name}, 2 CPU / 4 GiB cgroup limit, namespace keldock-core; no CNPG, cert-manager, Barman or cloud installation.\n`,
);
const clusters = JSON.parse(
  run("k3d", ["cluster", "list", "-o", "json"], true),
);
if (!clusters.some((item) => item.name === name)) {
  const config = parse(readFileSync("tooling/local/k3d.yaml", "utf8"));
  config.metadata.name = name;
  config.kubeAPI.hostPort = "36443";
  config.ports[0].port = "127.0.0.1:38080:80";
  config.volumes[0].volume = `${resolve("tooling/local/kubelet.yaml")}:/var/lib/rancher/k3s/agent/etc/kubelet.conf.d/10-keldock-dev.conf:ro`;
  const path = resolve(directory, "k3d.yaml");
  writeFileSync(path, stringify(config));
  run("k3d", [
    "cluster",
    "create",
    "--config",
    path,
    "--runtime-label",
    "keldock.alpha-fixture=true",
  ]);
}
const node = JSON.parse(
  run("docker", ["inspect", `k3d-${name}-server-0`], true),
)[0];
if (node.Config.Labels["keldock.alpha-fixture"] !== "true")
  throw new Error("Core fixture ownership mismatch");
run("docker", [
  "update",
  "--cpus",
  "2",
  "--memory",
  "4g",
  "--memory-swap",
  "4g",
  `k3d-${name}-server-0`,
]);
const kubeconfig = resolve(directory, "kubeconfig");
writeFileSync(kubeconfig, run("k3d", ["kubeconfig", "get", name], true), {
  mode: 0o600,
});
chmodSync(kubeconfig, 0o600);
const kube = ["--kubeconfig", kubeconfig, "--context", "k3d-keldock-m6-core"];
run("kubectl", [
  ...kube,
  "wait",
  "node",
  "--all",
  "--for=condition=Ready",
  "--timeout=180s",
]);
const path = renderFlux(resolve(directory, "flux.yaml"));
run("kubectl", [...kube, "apply", "-f", path]);
for (const name of ["source-controller", "kustomize-controller"])
  run("kubectl", [
    ...kube,
    "-n",
    "keldock-system",
    "rollout",
    "status",
    `deployment/${name}`,
    "--timeout=180s",
  ]);
process.stdout.write(
  "Fresh constrained core cluster is ready with Flux and Traefik only.\n",
);
