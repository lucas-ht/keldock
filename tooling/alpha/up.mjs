import { execFileSync } from "node:child_process";
import { chmodSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { parse, stringify } from "yaml";
import { renderDependencies } from "./dependencies.mjs";
import { renderFlux } from "./flux.mjs";

const root = resolve(import.meta.dirname, "../..");
process.chdir(root);
const directory = resolve(".local/m6");
mkdirSync(directory, { recursive: true, mode: 0o700 });
const run = (command, args, capture = false) =>
  execFileSync(command, args, {
    encoding: "utf8",
    stdio: capture ? ["ignore", "pipe", "inherit"] : "inherit",
    maxBuffer: 16 * 1024 * 1024,
  });
process.stdout.write(
  "Alpha fixture target: local Docker network keldock-m1; new cluster keldock-m6 / context k3d-keldock-m6; no AWS account, backend or production resources.\n",
);
const clusters = JSON.parse(
  run("k3d", ["cluster", "list", "-o", "json"], true),
);
if (!clusters.some((item) => item.name === "keldock-m6")) {
  const configuration = parse(readFileSync("tooling/local/k3d.yaml", "utf8"));
  configuration.metadata.name = "keldock-m6";
  configuration.kubeAPI.hostPort = "26443";
  configuration.ports[0].port = "127.0.0.1:28080:80";
  configuration.volumes[0].volume = `${resolve("tooling/local/kubelet.yaml")}:/var/lib/rancher/k3s/agent/etc/kubelet.conf.d/10-keldock-dev.conf:ro`;
  const path = resolve(directory, "k3d.yaml");
  writeFileSync(path, stringify(configuration));
  run("k3d", [
    "cluster",
    "create",
    "--config",
    path,
    "--runtime-label",
    "keldock.alpha-fixture=true",
  ]);
} else {
  const [node] = JSON.parse(
    run("docker", ["inspect", "k3d-keldock-m6-server-0"], true),
  );
  if (node.Config.Labels["keldock.alpha-fixture"] !== "true")
    throw new Error("Alpha fixture ownership mismatch");
}
const kubeconfig = resolve(directory, "kubeconfig");
writeFileSync(
  kubeconfig,
  run("k3d", ["kubeconfig", "get", "keldock-m6"], true),
  { mode: 0o600 },
);
chmodSync(kubeconfig, 0o600);
const kube = ["--kubeconfig", kubeconfig, "--context", "k3d-keldock-m6"];
for (const condition of [
  "Ready",
  "DiskPressure=false",
  "MemoryPressure=false",
  "PIDPressure=false",
])
  run("kubectl", [
    ...kube,
    "wait",
    "node",
    "--all",
    `--for=condition=${condition}`,
    "--timeout=180s",
  ]);

const fluxPath = renderFlux(resolve(directory, "flux.yaml"));
run("kubectl", [...kube, "apply", "-f", fluxPath]);
for (const controller of ["source-controller", "kustomize-controller"])
  run("kubectl", [
    ...kube,
    "-n",
    "keldock-system",
    "rollout",
    "status",
    `deployment/${controller}`,
    "--timeout=180s",
  ]);

for (const path of await renderDependencies(
  resolve(directory, "dependencies"),
)) {
  run("kubectl", [
    ...kube,
    "apply",
    "--server-side",
    "--field-manager=keldock-alpha-bootstrap",
    "-f",
    path,
  ]);
  const name = path.split("/").at(-1);
  const deployments =
    name === "cert-manager.yaml"
      ? [
          ["cert-manager", "cert-manager"],
          ["cert-manager", "cert-manager-webhook"],
          ["cert-manager", "cert-manager-cainjector"],
        ]
      : [
          [
            "cnpg-system",
            name === "cnpg.yaml" ? "cnpg-controller-manager" : "barman-cloud",
          ],
        ];
  for (const [namespace, deployment] of deployments)
    run("kubectl", [
      ...kube,
      "-n",
      namespace,
      "rollout",
      "status",
      `deployment/${deployment}`,
      "--timeout=240s",
    ]);
}
process.stdout.write(
  "Dedicated alpha cluster and explicitly installed tested database dependencies are ready.\n",
);
