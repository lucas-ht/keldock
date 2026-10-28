import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import http from "node:http";
import { profile } from "../profiles.mjs";

if (profile.cluster !== "keldock-m6-core")
  throw new Error("Lifecycle acceptance requires the exact --core fixture");
export const directory = profile.directory;
export const input = resolve(directory, "installation");
export const kube = [
  "--kubeconfig",
  resolve(directory, "kubeconfig"),
  "--context",
  profile.context,
];
export const read = (path) => JSON.parse(readFileSync(path, "utf8"));
export const save = (path, value) =>
  writeFileSync(path, JSON.stringify(value, null, 2) + "\n", { mode: 0o600 });
export const run = (name, args, data) => {
  try {
    return execFileSync(name, args, {
      input: data,
      encoding: "utf8",
      maxBuffer: 16 * 1024 * 1024,
      stdio: [data ? "pipe" : "ignore", "pipe", "pipe"],
    });
  } catch {
    // Input/output may contain a Secret. Fail without dumping command buffers.
    throw new Error(
      `Disposable lifecycle ${name} operation failed; inspect the exact owned target`,
    );
  }
};
export const kubectl = (args, data) =>
  run("kubectl", [...kube, "-n", profile.namespace, ...args], data);
export const apply = (items) =>
  kubectl(
    ["apply", "-f", "-"],
    JSON.stringify({ apiVersion: "v1", kind: "List", items }),
  );
export const helm = (args) =>
  run("helm", [
    ...args,
    "--kubeconfig",
    kube[1],
    "--kube-context",
    profile.context,
    "--namespace",
    profile.namespace,
  ]);
export const claims = () =>
  Object.fromEntries(
    JSON.parse(
      kubectl([
        "get",
        "pvc",
        "-l",
        "app.kubernetes.io/instance=core",
        "-o",
        "json",
      ]),
    ).items.map((item) => [item.metadata.name, item.metadata.uid]),
  );
export function assertClaims(expected) {
  const current = claims();
  for (const [name, uid] of Object.entries(expected))
    if (current[name] !== uid)
      throw new Error(`Retained PVC identity changed: ${name}`);
}
export async function application() {
  const evidence = read(resolve(directory, "application-evidence.json"));
  const result = await new Promise((accept, reject) => {
    const req = http.get(
      {
        hostname: "127.0.0.1",
        port: 38080,
        path: "/healthz",
        headers: { Host: evidence.placement.hostname },
      },
      (response) => {
        const chunks = [];
        response.on("data", (chunk) => chunks.push(chunk));
        response.on("end", () =>
          accept({
            status: response.statusCode,
            body: Buffer.concat(chunks).toString(),
          }),
        );
      },
    );
    req.setTimeout(10000, () =>
      req.destroy(new Error("Application route timed out")),
    );
    req.on("error", reject);
  });
  const body = JSON.parse(result.body);
  if (
    result.status !== 200 ||
    body.web !== true ||
    body.api?.artifact !== "v1" ||
    body.api?.version !== "v1" ||
    body.api?.migrations < 1 ||
    body.api?.workerTicks < 1 ||
    body.api?.scheduledTicks < 1
  )
    throw new Error(
      "Packaged application did not retain its approved healthy response",
    );
}
export function assertTarget() {
  const node = JSON.parse(
    run("docker", ["inspect", "k3d-keldock-m6-core-server-0"]),
  )[0];
  if (node.Config.Labels["keldock.alpha-fixture"] !== "true")
    throw new Error("Core ownership mismatch");
  const crds = JSON.parse(
    run("kubectl", [...kube, "get", "crds", "-o", "json"]),
  ).items;
  if (crds.some((item) => /cnpg|barman|crossplane/.test(item.metadata.name)))
    throw new Error("Core unexpectedly requires optional controllers");
  process.stdout.write(
    "Lifecycle target: owned k3d-keldock-m6-core / core-fixture / keldock-core / Helm core; exact retained PVC identities are recorded before uninstall. No cloud or other cluster is targeted.\n",
  );
}
