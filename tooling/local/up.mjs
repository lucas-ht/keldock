import { execFileSync } from "node:child_process";
import { createHash, randomBytes } from "node:crypto";
import {
  chmodSync,
  existsSync,
  mkdirSync,
  readFileSync,
  writeFileSync,
} from "node:fs";
import { resolve } from "node:path";
import { parseAllDocuments, stringify } from "yaml";

const root = resolve(import.meta.dirname, "../..");
process.chdir(root);
const clusterConfigHash = createHash("sha256")
  .update(readFileSync("tooling/local/k3d.yaml"))
  .update(readFileSync("tooling/local/kubelet.yaml"))
  .digest("hex");
const run = (command, args, capture = false) =>
  execFileSync(command, args, {
    cwd: root,
    stdio: capture ? ["ignore", "pipe", "inherit"] : "inherit",
    encoding: "utf8",
    maxBuffer: 8 * 1024 * 1024,
  });

mkdirSync(".local", { recursive: true, mode: 0o700 });
chmodSync(".local", 0o700);
if (!existsSync(".local/local.env")) {
  writeFileSync(
    ".local/local.env",
    `KELDOCK_LOCAL_POSTGRES_PASSWORD=${randomBytes(32).toString("hex")}\n`,
    { mode: 0o600, flag: "wx" },
  );
}

const networkExists = run("docker", ["network", "ls", "--format", "json"], true)
  .trim()
  .split("\n")
  .filter(Boolean)
  .map((line) => JSON.parse(line))
  .some((network) => network.Name === "keldock-m1");
if (networkExists) {
  const labels = JSON.parse(
    run("docker", ["network", "inspect", "keldock-m1"], true),
  )[0].Labels;
  if (labels?.["com.docker.compose.project"] !== "keldock-m1")
    throw new Error("Refusing to reuse a network not owned by this fixture");
}

process.stdout.write(
  "Local target: Docker project/network keldock-m1; cluster/context k3d-keldock-m1; no cloud account/backend.\n",
);
run("docker", [
  "compose",
  "--env-file",
  ".local/local.env",
  "-f",
  "tooling/local/compose.yaml",
  "up",
  "-d",
  "--wait",
]);

const clusters = JSON.parse(
  run("k3d", ["cluster", "list", "-o", "json"], true),
);
if (!clusters.some((cluster) => cluster.name === "keldock-m1")) {
  const clusterConfiguration = parseAllDocuments(
    readFileSync("tooling/local/k3d.yaml", "utf8"),
  )[0].toJSON();
  clusterConfiguration.volumes[0].volume = `${resolve("tooling/local/kubelet.yaml")}:/var/lib/rancher/k3s/agent/etc/kubelet.conf.d/10-keldock-dev.conf:ro`;
  writeFileSync(".local/k3d.yaml", stringify(clusterConfiguration));
  run("k3d", [
    "cluster",
    "create",
    "--config",
    ".local/k3d.yaml",
    "--runtime-label",
    "keldock.local-fixture=true",
    "--runtime-label",
    `keldock.fixture.config=${clusterConfigHash}`,
  ]);
} else {
  const labels = JSON.parse(
    run("docker", ["inspect", "k3d-keldock-m1-server-0"], true),
  )[0].Config.Labels;
  if (labels?.["keldock.local-fixture"] !== "true")
    throw new Error("Refusing to reuse a cluster not owned by this fixture");
  if (labels?.["keldock.fixture.config"] !== clusterConfigHash)
    throw new Error(
      "Local fixture configuration changed; run task local:reset to recreate its cluster",
    );
}

writeFileSync(
  ".local/kubeconfig",
  run("k3d", ["kubeconfig", "get", "keldock-m1"], true),
  { mode: 0o600 },
);
chmodSync(".local/kubeconfig", 0o600);
const manifests = parseAllDocuments(
  run(
    "flux",
    [
      "install",
      "--export",
      "--components=source-controller,kustomize-controller",
      "--namespace=keldock-system",
    ],
    true,
  ),
).map((document) => document.toJSON());
for (const manifest of manifests) {
  if (
    manifest?.kind === "Deployment" &&
    manifest.metadata.name === "kustomize-controller"
  ) {
    manifest.spec.template.spec.containers[0].args.push(
      "--no-cross-namespace-refs=true",
      "--no-remote-bases=true",
      "--default-service-account=untrusted",
      "--requeue-dependency=2s",
    );
  }
}
writeFileSync(
  ".local/flux.yaml",
  manifests.map((manifest) => stringify(manifest)).join("---\n"),
);
const kube = [
  "--kubeconfig",
  ".local/kubeconfig",
  "--context",
  "k3d-keldock-m1",
];
process.stdout.write(
  "Waiting for local node readiness and disk-pressure recovery.\n",
);
for (const condition of [
  "Ready",
  "DiskPressure=false",
  "MemoryPressure=false",
  "PIDPressure=false",
]) {
  try {
    run("kubectl", [
      ...kube,
      "wait",
      "node",
      "--all",
      `--for=condition=${condition}`,
      "--timeout=360s",
    ]);
  } catch (cause) {
    throw new Error(
      `Local node did not reach ${condition}. Check Docker's available disk and memory before rerunning task local:up; no automatic cleanup is performed.`,
      { cause },
    );
  }
}
run("kubectl", [...kube, "apply", "-f", ".local/flux.yaml"]);
for (const controller of ["source-controller", "kustomize-controller"]) {
  run("kubectl", [
    ...kube,
    "-n",
    "keldock-system",
    "rollout",
    "status",
    `deployment/${controller}`,
    "--timeout=180s",
  ]);
}
const password = readFileSync(".local/local.env", "utf8").trim().split("=")[1];
writeFileSync(
  ".local/runtime.json",
  JSON.stringify({
    databaseUrl: `postgres://keldock:${password}@127.0.0.1:55432/keldock?sslmode=disable`,
    kubeconfig: resolve(".local/kubeconfig"),
    context: "k3d-keldock-m1",
    registryPush: "localhost:5010/keldock/bundles",
    registryPull: "keldock-m1-registry:5000/keldock/bundles",
  }),
  { mode: 0o600 },
);
process.stdout.write(
  "Local cluster, Flux, registry and PostgreSQL are ready.\n",
);
