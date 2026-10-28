import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { profile } from "./profiles.mjs";
import { generateKeyPairSync } from "node:crypto";

const root = resolve(import.meta.dirname, "../..");
process.chdir(root);
const artifactRoot = resolve(".local/m6");
const directory = profile.directory;
const input = resolve(directory, "installation");
const kube = [
  "--kubeconfig",
  resolve(directory, "kubeconfig"),
  "--context",
  profile.context,
];
const run = (name, args, options = {}) =>
  execFileSync(name, args, {
    encoding: "utf8",
    maxBuffer: 16 * 1024 * 1024,
    stdio: options.input
      ? ["pipe", "pipe", "inherit"]
      : options.capture
        ? ["ignore", "pipe", "inherit"]
        : "inherit",
    ...options,
  });
const json = (path, value) =>
  writeFileSync(path, JSON.stringify(value, null, 2) + "\n", { mode: 0o600 });
const node = JSON.parse(
  run("docker", ["inspect", `k3d-${profile.cluster}-server-0`], {
    capture: true,
  }),
)[0];
if (node.Config.Labels["keldock.alpha-fixture"] !== "true")
  throw new Error("Disposable cluster ownership mismatch");
const nodeIP = node.NetworkSettings.Networks["keldock-m1"].IPAddress;
process.stdout.write(
  `Install target: ${profile.context} / namespace ${profile.namespace} / release ${profile.release}; no cloud resources.\n`,
);
const image = (name) => {
  const log = readFileSync(
    resolve(artifactRoot, `${name}-image-push.log`),
    "utf8",
  );
  const digest = log.match(/digest: (sha256:[a-f0-9]{64})/u)?.[1];
  if (!digest) throw new Error(`Push the built ${name} image first`);
  return `keldock-m1-registry:5000/keldock/${name}@${digest}`;
};
json(resolve(directory, "release-fixture.json"), {
  version: "0.6.0-alpha.1",
  revision: "local-candidate",
  runtimeImage: image("runtime"),
  consoleImage: image("console"),
});
if (!existsSync(input))
  run(resolve(artifactRoot, "keldock"), [
    "installation",
    "init",
    "--directory",
    input,
    "--release-file",
    resolve(directory, "release-fixture.json"),
    "--name",
    profile.release,
    "--namespace",
    profile.namespace,
    "--organization",
    profile.organization,
    "--public-url",
    profile.origin,
    "--domain",
    "alpha.keldock.test",
    "--storage-class",
    "local-path",
  ]);
if (process.argv.includes("--core")) {
  const control = JSON.parse(
    readFileSync(resolve(input, "control.json"), "utf8"),
  );
  if (!control.projects.length) {
    const policy = resolve(directory, "repository-policy.json");
    json(policy, {
      id: "sample",
      name: "Core acceptance",
      repositoryId: 42,
      ownerId: 24,
      workflowPath: ".github/workflows/build.yaml",
      pushRef: "refs/heads/main",
      provenanceKeys: {
        fixture: generateKeyPairSync("ed25519")
          .publicKey.export({ format: "der", type: "spki" })
          .subarray(-32)
          .toString("base64"),
      },
    });
    run(resolve(artifactRoot, "keldock"), [
      "installation",
      "project",
      "--directory",
      input,
      "--file",
      policy,
    ]);
  }
}
// Only this fixture adds split DNS and its private CA. No global DNS/trust changes.
const values = JSON.parse(readFileSync(resolve(input, "values.json"), "utf8"));
values.images = { runtime: image("runtime"), console: image("console") };
values.hostAliases = [{ ip: nodeIP, hostnames: [profile.host] }];
values.extraTrustSecret = "alpha-ca";
json(resolve(input, "values.json"), values);
if (values.registry.enabled) {
  const repository = `${profile.release}-registry.${profile.namespace}.svc.cluster.local:5000/keldock/bundles`;
  for (const filename of ["control.json", "agent.json"]) {
    const settings = JSON.parse(readFileSync(resolve(input, filename), "utf8"));
    settings.registryPull = repository;
    if (filename === "control.json") settings.registryPush = repository;
    json(resolve(input, filename), settings);
  }
  run(resolve(artifactRoot, "keldock"), [
    "installation",
    "render-secret",
    "--directory",
    input,
  ]);
}
const tls = resolve(directory, "installation-tls");
if (!existsSync(resolve(tls, "public.crt"))) {
  json(resolve(directory, "installation-tls.json"), {
    directory: tls,
    dns: [profile.host],
    ips: ["127.0.0.1"],
  });
  run("mise", [
    "exec",
    "--",
    "go",
    "run",
    "./tooling/fixtures/certificates",
    "--config",
    resolve(directory, "installation-tls.json"),
  ]);
}
const objects = [
  {
    apiVersion: "v1",
    kind: "Namespace",
    metadata: {
      name: profile.namespace,
      labels: {
        "keldock.alpha-fixture": "true",
        "pod-security.kubernetes.io/enforce": "restricted",
        "pod-security.kubernetes.io/enforce-version": "v1.35",
      },
    },
  },
  {
    apiVersion: "v1",
    kind: "Secret",
    metadata: { name: "keldock-tls", namespace: profile.namespace },
    type: "kubernetes.io/tls",
    data: {
      "tls.crt": readFileSync(resolve(tls, "public.crt")).toString("base64"),
      "tls.key": readFileSync(resolve(tls, "private.key")).toString("base64"),
    },
  },
  {
    apiVersion: "v1",
    kind: "Secret",
    metadata: { name: "alpha-ca", namespace: profile.namespace },
    data: { "ca.crt": readFileSync(resolve(tls, "ca.crt")).toString("base64") },
  },
];
run("kubectl", [...kube, "apply", "-f", "-"], {
  input: JSON.stringify({ apiVersion: "v1", kind: "List", items: objects }),
});
run("kubectl", [
  ...kube,
  "apply",
  "-f",
  resolve(input, "installation-secret.json"),
]);
let gateway;
try {
  gateway = JSON.parse(
    run("docker", ["inspect", `${profile.cluster}-gateway`], {
      capture: true,
      stdio: ["ignore", "pipe", "pipe"],
    }),
  )[0];
} catch {}
if (gateway && gateway.Config.Labels["keldock.alpha-fixture"] !== "true")
  throw new Error("Gateway ownership mismatch");
if (!gateway)
  run("docker", [
    "run",
    "-d",
    "--name",
    `${profile.cluster}-gateway`,
    "--label",
    "keldock.alpha-fixture=true",
    "--network",
    "keldock-m1",
    "-p",
    `127.0.0.1:${profile.port}:443`,
    "localhost:5010/keldock/m6-gateway:fixture",
    `${nodeIP}:443`,
  ]);
run(resolve(artifactRoot, "keldock"), [
  "installation",
  "preflight",
  "--directory",
  input,
  ...kube,
]);
run("mise", [
  "exec",
  "--",
  "helm",
  "upgrade",
  "--install",
  profile.release,
  "packaging/charts/keldock",
  "--kubeconfig",
  kube[1],
  "--kube-context",
  profile.context,
  "--namespace",
  profile.namespace,
  "--values",
  resolve(input, "values.json"),
  "--wait",
  "--timeout",
  "5m",
]);
process.stdout.write(
  "Packaged fixture installed. Canonical HTTPS uses only the isolated fixture CA. Enroll agents and activate the local administrator explicitly.\n",
);
