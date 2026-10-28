import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { mkdirSync, readFileSync, writeFileSync, existsSync } from "node:fs";
import { resolve } from "node:path";
import { parseAllDocuments, stringify } from "yaml";

// This acceptance installer is deliberately restricted to the separate local fixture.
// Supplied installations retain their operator; see docs/decisions/0007-optional-aws-profile.md.
const kubeconfig = resolve(
  process.env.KELDOCK_CLOUD_KUBECONFIG ?? ".local/m5/kubeconfig",
);
const context = "k3d-keldock-m5";
const cache = resolve(".local/m5/controllers");
const lock = JSON.parse(
  readFileSync(
    new URL(
      "../../internal/cloud/crossplane/controllers.lock.json",
      import.meta.url,
    ),
    "utf8",
  ),
);
function kubectl(args, input) {
  return execFileSync(
    "kubectl",
    ["--kubeconfig", kubeconfig, "--context", context, ...args],
    {
      input,
      encoding: "utf8",
      stdio: input
        ? ["pipe", "inherit", "inherit"]
        : ["ignore", "pipe", "inherit"],
    },
  );
}
const config = JSON.parse(
  kubectl(["config", "view", "--minify", "-o", "json"]),
);
if (config.clusters?.[0]?.cluster?.server !== "https://127.0.0.1:26443")
  throw new Error(
    "Refusing controller installation outside isolated keldock-m5 fixture",
  );
const nodes = JSON.parse(kubectl(["get", "nodes", "-o", "json"]));
if (
  nodes.items.length !== 1 ||
  nodes.items[0].metadata.name !== "k3d-keldock-m5-server-0"
)
  throw new Error("Unexpected fixture node ownership");
mkdirSync(cache, { recursive: true, mode: 0o700 });
for (const [name, artifact] of Object.entries(lock.artifacts)) {
  const file = resolve(cache, artifact.file);
  let bytes;
  if (existsSync(file)) bytes = readFileSync(file);
  else {
    const response = await fetch(artifact.url, {
      signal: AbortSignal.timeout(120_000),
    });
    if (!response.ok)
      throw new Error(`${name} download failed: ${response.status}`);
    bytes = Buffer.from(await response.arrayBuffer());
  }
  if (createHash("sha256").update(bytes).digest("hex") !== artifact.sha256)
    throw new Error(`${name} checksum mismatch`);
  writeFileSync(file, bytes, { mode: 0o600 });
  if (name === "crossplane") continue;
  let manifest = bytes.toString("utf8");
  for (const [tag, digest] of Object.entries(lock.images))
    manifest = manifest.replaceAll(tag, digest);
  const documents = parseAllDocuments(manifest);
  for (const document of documents) {
    const object = document.toJSON();
    if (object.kind === "Secret" && object.data?.SIDECAR_IMAGE) {
      const tag = Buffer.from(
        object.data.SIDECAR_IMAGE.replaceAll(/\s/g, ""),
        "base64",
      )
        .toString("utf8")
        .trim();
      if (!lock.images[tag])
        throw new Error("Barman sidecar image is not pinned");
      document.setIn(
        ["data", "SIDECAR_IMAGE"],
        Buffer.from(lock.images[tag]).toString("base64"),
      );
    }
    if (object.kind === "Deployment")
      document.setIn(
        [
          "spec",
          "template",
          "metadata",
          "annotations",
          "keldock.io/controller-lock",
        ],
        createHash("sha256").update(JSON.stringify(lock)).digest("hex"),
      );
  }
  manifest = documents.map((document) => document.toString()).join("---\n");
  const rendered = resolve(cache, `${name}-pinned.yaml`);
  writeFileSync(rendered, manifest, { mode: 0o600 });
}
// cert-manager comes first because the Barman plugin uses its TLS certificates.
kubectl([
  "apply",
  "--server-side",
  "--field-manager=keldock-cloud-bootstrap",
  "-f",
  resolve(cache, "cert-manager-pinned.yaml"),
]);
for (const deployment of [
  "cert-manager",
  "cert-manager-webhook",
  "cert-manager-cainjector",
])
  kubectl([
    "rollout",
    "status",
    `deployment/${deployment}`,
    "-n",
    "cert-manager",
    "--timeout=180s",
  ]);
const values = {
  image: {
    repository: lock.images["xpkg.crossplane.io/crossplane/crossplane:v2.4.3"],
    ignoreTag: true,
  },
  provider: { defaultActivations: [] },
  resourcesCrossplane: {
    requests: { cpu: "100m", memory: "128Mi" },
    limits: { cpu: "1", memory: "512Mi" },
  },
};
writeFileSync(resolve(cache, "crossplane-values.yaml"), stringify(values), {
  mode: 0o600,
});
execFileSync(
  "helm",
  [
    "upgrade",
    "--install",
    "crossplane",
    resolve(cache, lock.artifacts.crossplane.file),
    "--namespace",
    "keldock-cloud",
    "--create-namespace",
    "--kubeconfig",
    kubeconfig,
    "--kube-context",
    context,
    "--values",
    resolve(cache, "crossplane-values.yaml"),
    "--wait",
    "--timeout=5m",
  ],
  { stdio: "inherit" },
);
for (const name of ["cnpg", "barman"]) {
  kubectl([
    "apply",
    "--server-side",
    "--field-manager=keldock-cloud-bootstrap",
    "-f",
    resolve(cache, `${name}-pinned.yaml`),
  ]);
  kubectl(
    [
      "rollout",
      "status",
      "deployment/cnpg-controller-manager",
      "-n",
      "cnpg-system",
      "--timeout=180s",
    ].map((value) =>
      name === "barman" && value === "deployment/cnpg-controller-manager"
        ? "deployment/barman-cloud"
        : value,
    ),
  );
}
// Select only the namespace-scoped AWS APIs needed by this profile.
const kinds = [
  "buckets",
  "bucketversionings",
  "bucketpublicaccessblocks",
  "bucketownershipcontrols",
  "bucketserversideencryptionconfigurations",
];
const activate = [
  ...kinds.map((kind) => `${kind}.s3.aws.m.upbound.io`),
  "roles.iam.aws.m.upbound.io",
  "rolepolicies.iam.aws.m.upbound.io",
  "records.route53.aws.m.upbound.io",
];
kubectl(
  [
    "apply",
    "--server-side",
    "--field-manager=keldock-cloud-bootstrap",
    "-f",
    "-",
  ],
  stringify({
    apiVersion: "apiextensions.crossplane.io/v1alpha1",
    kind: "ManagedResourceActivationPolicy",
    metadata: { name: "keldock-aws" },
    spec: { activate },
  }),
);
process.stdout.write(
  "Pinned optional controllers installed; AWS provider roles/resources are configured separately.\n",
);
