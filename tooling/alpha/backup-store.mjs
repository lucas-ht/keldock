import { execFileSync } from "node:child_process";
import { randomBytes } from "node:crypto";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";

const root = resolve(import.meta.dirname, "../..");
process.chdir(root);
const args = process.argv.slice(2),
  option = (name) => {
    const index = args.indexOf(name);
    return index < 0 ? undefined : args[index + 1];
  };
const allocation = option("--allocation"),
  organization = option("--organization"),
  project = option("--project"),
  uid = option("--uid");
for (const value of [allocation, organization, project])
  if (!/^[a-z][a-z0-9-]{0,39}$/.test(value ?? ""))
    throw new Error("Exact fixture allocation/scope required");
if (!uid || uid.length > 80) throw new Error("Exact source UID required");
const directory = resolve(".local/m6/backup-store"),
  tls = resolve(directory, "tls");
for (const path of [
  directory,
  tls,
  resolve(directory, "data"),
  resolve(directory, "identities"),
  resolve(directory, "destinations"),
])
  mkdirSync(path, { recursive: true, mode: 0o700 });
const run = (command, args, capture = false, options = {}) =>
  execFileSync(command, args, {
    cwd: root,
    encoding: "utf8",
    stdio: options.input
      ? ["pipe", capture ? "pipe" : "inherit", "inherit"]
      : capture
        ? ["ignore", "pipe", "inherit"]
        : "inherit",
    ...options,
  });
const kube = [
  "--kubeconfig",
  resolve(".local/m6/kubeconfig"),
  "--context",
  "k3d-keldock-m6",
];
const name = "keldock-m6-backups",
  rootFile = resolve(directory, "root.json"),
  envFile = resolve(directory, "root.env");
if (!existsSync(rootFile)) {
  const credentials = {
    access: randomBytes(16).toString("hex"),
    secret: randomBytes(32).toString("hex"),
  };
  writeFileSync(rootFile, JSON.stringify(credentials), {
    mode: 0o600,
    flag: "wx",
  });
  writeFileSync(
    envFile,
    `MINIO_ROOT_USER=${credentials.access}\nMINIO_ROOT_PASSWORD=${credentials.secret}\n`,
    { mode: 0o600, flag: "wx" },
  );
}
const containers = run("docker", ["ps", "-a", "--format", "{{.Names}}"], true)
  .trim()
  .split("\n");
let created = false;
if (!containers.includes(name)) {
  const image = run(
    "docker",
    [
      "image",
      "inspect",
      "localhost:5010/keldock/m6-storage:fixture",
      "--format",
      "{{.Id}}",
    ],
    true,
  ).trim();
  process.stdout.write(
    `Creating disposable external backup target: local Docker ${name}; private data directory ${directory}/data; no cloud account/backend.\n`,
  );
  run("docker", [
    "create",
    "--name",
    name,
    "--network",
    "keldock-m1",
    "--label",
    "keldock.alpha-fixture=true",
    "--user",
    `${process.getuid()}:${process.getgid()}`,
    "--env-file",
    envFile,
    "-p",
    "127.0.0.1:19443:443",
    "--mount",
    `type=bind,source=${directory}/data,target=/data`,
    "--mount",
    `type=bind,source=${tls},target=/certs,readonly`,
    image,
    "server",
    "/data",
    "--certs-dir",
    "/certs",
    "--address",
    ":443",
    "--console-address",
    ":9001",
  ]);
  run("docker", ["start", name]);
  created = true;
}
let [container] = JSON.parse(run("docker", ["inspect", name], true));
if (container.Config.Labels["keldock.alpha-fixture"] !== "true")
  throw new Error("Refusing unrelated backup container");
if (!container.State.Running) run("docker", ["start", name]);
[container] = JSON.parse(run("docker", ["inspect", name], true));
const ip = container.NetworkSettings.Networks["keldock-m1"]?.IPAddress;
if (!ip) throw new Error("Backup fixture network is unavailable");
if (created || !existsSync(resolve(tls, "ca.crt"))) {
  const spec = resolve(directory, "certificate.json");
  writeFileSync(
    spec,
    JSON.stringify({ directory: tls, ips: ["127.0.0.1", ip], dns: [name] }),
    { mode: 0o600 },
  );
  run("go", ["run", "./tooling/fixtures/certificates", "--config", spec]);
  process.stdout.write(
    `Restarting only newly prepared local TLS fixture ${name}.\n`,
  );
  run("docker", ["restart", name]);
}
const identityFile = resolve(directory, "identities", `${allocation}.json`),
  prefix = `keldock/${organization}/${project}/${allocation}/generation-1`;
if (!existsSync(identityFile)) {
  const admin = JSON.parse(readFileSync(rootFile, "utf8"));
  writeFileSync(
    identityFile,
    JSON.stringify({
      url: "https://127.0.0.1:19443",
      ca: resolve(tls, "ca.crt"),
      bucket: "keldock-m6",
      prefix,
      rootAccess: admin.access,
      rootSecret: admin.secret,
      writerAccess: randomBytes(16).toString("hex"),
      writerSecret: randomBytes(32).toString("hex"),
      readerAccess: randomBytes(16).toString("hex"),
      readerSecret: randomBytes(32).toString("hex"),
    }),
    { mode: 0o600, flag: "wx" },
  );
}
run("go", ["run", ".", "--config", identityFile], false, {
  cwd: resolve("tooling/fixtures/backup-store"),
});
const identity = JSON.parse(readFileSync(identityFile, "utf8")),
  namespace = "keldock-m6-backups";
const apply = (object) =>
  run("kubectl", [...kube, "apply", "-f", "-"], false, {
    input: JSON.stringify(object),
  });
apply({
  apiVersion: "v1",
  kind: "Namespace",
  metadata: {
    name: namespace,
    labels: {
      "keldock.io/fixture": "m6-backups",
      "pod-security.kubernetes.io/enforce": "restricted",
    },
  },
});
const suffix = allocation.slice(3, 15),
  writerName = `writer-${suffix}`,
  readerName = `reader-${suffix}`;
for (const [name, values] of [
  [
    writerName,
    {
      ACCESS_KEY_ID: identity.writerAccess,
      ACCESS_SECRET_KEY: identity.writerSecret,
    },
  ],
  [
    readerName,
    {
      ACCESS_KEY_ID: identity.readerAccess,
      ACCESS_SECRET_KEY: identity.readerSecret,
    },
  ],
  ["backup-ca", { "ca.crt": readFileSync(resolve(tls, "ca.crt"), "utf8") }],
]) {
  const data = Object.fromEntries(
    Object.entries(values).map(([key, value]) => [
      key,
      Buffer.from(value).toString("base64"),
    ]),
  );
  apply({
    apiVersion: "v1",
    kind: "Secret",
    metadata: {
      name,
      namespace,
      labels: { "keldock.io/fixture": "m6-backups" },
    },
    type: "Opaque",
    data,
  });
}
const reference = (name) => ({
  namespace,
  name,
  uid: run(
    "kubectl",
    [
      ...kube,
      "-n",
      namespace,
      "get",
      "secret",
      name,
      "-o",
      "jsonpath={.metadata.uid}",
    ],
    true,
  ).trim(),
});
const destination = {
  allocationId: allocation,
  sourceUid: uid,
  description: "Supplied external disposable S3 fixture",
  endpoint: `https://${ip}:443`,
  probeEndpoint: "https://127.0.0.1:19443",
  region: "us-east-1",
  bucket: "keldock-m6",
  writer: reference(writerName),
  reader: reference(readerName),
  ca: reference("backup-ca"),
  development: true,
};
writeFileSync(
  resolve(directory, "destinations", `${allocation}.json`),
  JSON.stringify(destination),
  { mode: 0o600 },
);
process.stdout.write(
  `Supplied fixture ready: ${destination.endpoint}/keldock-m6/${prefix}; exact identities remain in private local files and operator Secrets.\n`,
);
