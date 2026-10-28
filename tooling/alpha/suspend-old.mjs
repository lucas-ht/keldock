import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";

const kube = [
  "--kubeconfig",
  ".local/m6/kubeconfig",
  "--context",
  "k3d-keldock-m6",
  "--request-timeout=15s",
];
const run = (args) =>
  execFileSync("kubectl", [...kube, ...args], {
    encoding: "utf8",
    maxBuffer: 8 * 1024 * 1024,
  });
const option = process.argv.indexOf("--preserve-organization");
const org =
  option >= 0
    ? process.argv[option + 1]
    : readFileSync(".local/m6/postgresql-backup-runtime.log", "utf8").match(
        /organization=(m6-[^ ]+) staging=/u,
      )?.[1];
if (!/^m6-[a-z0-9]{20,32}$/u.test(org ?? ""))
  throw new Error("Exact preserved synthetic organization is required");
const inventory = JSON.parse(
  run([
    "get",
    "namespace",
    "-l",
    "app.kubernetes.io/managed-by=keldock",
    "-o",
    "json",
  ]),
).items;
const namespaces = inventory.filter(
  (item) =>
    item.metadata.labels["keldock.io/organization"]?.startsWith("m6-") &&
    item.metadata.labels["keldock.io/organization"] !== org,
);
for (const ns of [...namespaces]) {
  const environment = ns.metadata.labels["keldock.io/environment"];
  const application = inventory.find(
    (item) =>
      item.metadata.name === "ke-" + environment?.replace(/^env-/u, "") &&
      item.metadata.labels["keldock.io/environment"] === environment,
  );
  if (
    application &&
    !namespaces.some((item) => item.metadata.uid === application.metadata.uid)
  )
    namespaces.push(application);
}
namespaces.sort(
  (a, b) =>
    Number(b.metadata.name.startsWith("ke-")) -
    Number(a.metadata.name.startsWith("ke-")),
);
const plan = [];
for (const ns of namespaces) {
  const namespace = ns.metadata.name;
  for (const [kind, patch] of [
    ["kustomizations.kustomize.toolkit.fluxcd.io", { spec: { suspend: true } }],
    ["deployments.apps", { spec: { replicas: 0 } }],
    ["cronjobs.batch", { spec: { suspend: true } }],
    [
      "clusters.postgresql.cnpg.io",
      { metadata: { annotations: { "cnpg.io/hibernation": "on" } } },
    ],
  ]) {
    for (const object of JSON.parse(
      run(["-n", namespace, "get", kind, "-o", "json"]),
    ).items) {
      if (
        ((kind.startsWith("kustomizations.") || kind.startsWith("cronjobs.")) &&
          object.spec.suspend === true) ||
        (kind === "deployments.apps" && object.spec.replicas === 0) ||
        (kind === "clusters.postgresql.cnpg.io" &&
          object.metadata.annotations?.["cnpg.io/hibernation"] === "on")
      )
        continue;
      plan.push({
        kind,
        namespace,
        name: object.metadata.name,
        uid: object.metadata.uid,
        patch,
      });
    }
  }
}
writeFileSync(".local/m6/suspension-plan.json", JSON.stringify(plan, null, 2), {
  mode: 0o600,
});
process.stdout.write(
  `Target: only k3d-keldock-m6; preserving ${org}. Reversibly suspend ${plan.length} objects in older synthetic fixtures; PVCs, namespaces and database contents remain retained.\n`,
);
for (const item of plan) {
  const current = JSON.parse(
    run(["-n", item.namespace, "get", item.kind, item.name, "-o", "json"]),
  );
  if (current.metadata.uid !== item.uid)
    throw new Error("Fixture identity changed");
  process.stdout.write(`${item.kind} ${item.namespace}/${item.name}\n`);
  run([
    "-n",
    item.namespace,
    "patch",
    item.kind,
    item.name,
    "--type=merge",
    "--patch",
    JSON.stringify(item.patch),
  ]);
}
