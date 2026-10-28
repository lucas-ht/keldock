import { execFileSync } from "node:child_process";
import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import https from "node:https";
import { profile } from "./profiles.mjs";

const root = resolve(import.meta.dirname, "../..");
process.chdir(root);
const directory = resolve(profile.directory, "installation");
const kube = [
  "--kubeconfig",
  resolve(profile.directory, "kubeconfig"),
  "--context",
  profile.context,
  "-n",
  profile.namespace,
];
const run = (args, input) =>
  execFileSync("kubectl", [...kube, ...args], {
    input,
    encoding: "utf8",
    stdio: [input ? "pipe" : "ignore", "pipe", "pipe"],
    maxBuffer: 1024 * 1024,
  });
const save = (name, value) =>
  writeFileSync(
    resolve(directory, name),
    JSON.stringify(value, null, 2) + "\n",
    { mode: 0o600 },
  );
const ca = readFileSync(resolve(profile.directory, "installation-tls/ca.crt"));
let cookie = "";
let csrf = "";
export async function request(path, method = "GET", input, headers = {}) {
  return new Promise((accept, reject) => {
    const req = https.request(
      {
        hostname: profile.host,
        port: profile.port,
        path,
        method,
        ca,
        servername: profile.host,
        lookup: (_host, _options, done) =>
          done(null, [{ address: "127.0.0.1", family: 4 }]),
        headers: {
          Host: profile.host,
          Origin: profile.origin,
          "Content-Type": "application/json",
          ...(cookie ? { Cookie: cookie } : {}),
          ...(csrf ? { "X-Keldock-CSRF": csrf } : {}),
          ...headers,
        },
      },
      (response) => {
        const chunks = [];
        response.on("data", (data) => chunks.push(data));
        response.on("end", () => {
          let body;
          try {
            body = JSON.parse(Buffer.concat(chunks).toString());
          } catch {
            return reject(new Error("Expected product API JSON"));
          }
          if (response.statusCode >= 300)
            return reject(
              new Error(
                `API ${method} ${path}: ${response.statusCode} (${body.error?.code ?? "unavailable"})`,
              ),
            );
          if (response.headers["set-cookie"])
            cookie = response.headers["set-cookie"][0].split(";")[0];
          accept(body);
        });
      },
    );
    req.setTimeout(15000, () =>
      req.destroy(new Error("Fixture HTTPS request timed out")),
    );
    req.on("error", reject);
    req.end(input === undefined ? undefined : JSON.stringify(input));
  });
}

const password = "Keldock disposable alpha fixture 2026!";
if (!existsSync(resolve(directory, "bootstrap.json"))) {
  run([
    "exec",
    `deployment/${profile.release}-control`,
    "-c",
    "api",
    "--",
    "/usr/local/bin/keldock",
    "bootstrap",
    "--config",
    "/runtime/private/control.json",
    "--email",
    "operator@example.test",
    "--name",
    "Alpha operator",
    "--output",
    "/state/private/bootstrap.json",
  ]);
  const invitation = JSON.parse(
    run([
      "exec",
      `deployment/${profile.release}-control`,
      "-c",
      "api",
      "--",
      "cat",
      "/state/private/bootstrap.json",
    ]),
  );
  await request("/v1/auth/activate", "POST", {
    token: new URL(invitation.activationUrl).hash.slice(7),
    password,
  });
  save("bootstrap.json", { email: "operator@example.test", synthetic: true });
}
await request("/v1/auth/login", "POST", {
  email: "operator@example.test",
  password,
});
const session = await request("/v1/auth/me");
csrf = session.csrfToken;
save("session-fixture.json", { cookie, csrf });
let inventory = await request("/v1/clusters");
const cluster = inventory.items.find((item) => item.id === profile.release);
if (!cluster) throw new Error("Expected exact alpha cluster");
if (!cluster.agentId || cluster.state === "revoked") {
  const pods = JSON.parse(
    run([
      "get",
      "pods",
      "-l",
      `app.kubernetes.io/instance=${profile.release},keldock.io/process=registrar`,
      "-o",
      "json",
    ]),
  ).items;
  const pod = pods.find(
    (item) =>
      item.status.phase === "Running" &&
      item.status.containerStatuses?.some(
        (container) => container.name === "registrar" && container.ready,
      ),
  );
  if (!pod) throw new Error("Expected one ready registrar fixture pod");
  if (cluster.state === "revoked")
    run([
      "exec",
      pod.metadata.name,
      "-c",
      "registrar",
      "--",
      "sh",
      "-c",
      `if [ -f /state/private/identity.json ]; then mv /state/private/identity.json /state/private/revoked-${cluster.revision}.json; fi`,
    ]);
  const grant = await request(
    `/v1/clusters/${profile.release}/enrollment`,
    "POST",
  );
  if (
    typeof grant.enrollment?.token !== "string" ||
    grant.enrollment.token.length < 32
  )
    throw new Error("Missing enrollment credential in fixture response");
  run(
    [
      "exec",
      "-i",
      pod.metadata.name,
      "--",
      "sh",
      "-c",
      "umask 077; cat > /tmp/enrollment.token",
    ],
    grant.enrollment.token,
  );
  const bytes = Number(
    run([
      "exec",
      pod.metadata.name,
      "-c",
      "registrar",
      "--",
      "sh",
      "-c",
      "wc -c < /tmp/enrollment.token",
    ]).trim(),
  );
  if (bytes !== Buffer.byteLength(grant.enrollment.token))
    throw new Error(`Enrollment stdin transfer incomplete (${bytes} bytes)`);
  run([
    "exec",
    pod.metadata.name,
    "--",
    "/usr/local/bin/keldock",
    "enroll",
    "--config",
    "/runtime/private/agent.json",
    "--cluster",
    profile.release,
    "--token-file",
    "/tmp/enrollment.token",
  ]);
  if (cluster.state === "revoked")
    run([
      "patch",
      "deployment",
      `${profile.release}-registrar`,
      "--type=merge",
      "--patch",
      JSON.stringify({
        spec: {
          template: {
            metadata: {
              annotations: {
                "keldock.io/enrollment-refresh": new Date().toISOString(),
              },
            },
          },
        },
      }),
    ]);
}
for (let attempt = 0; attempt < 30; attempt++) {
  inventory = await request("/v1/clusters");
  if (
    inventory.items.some(
      (item) => item.id === profile.release && item.state === "connected",
    )
  )
    break;
  await new Promise((done) => setTimeout(done, 1000));
}
const setup = await request("/v1/installation/onboarding");
save("onboarding-evidence.json", setup);
if (
  !setup.steps.some(
    (item) => item.id === "registrar" && item.state === "connected",
  )
)
  throw new Error("Packaged registrar did not connect");
process.stdout.write(
  "Packaged HTTPS local account activation, session/CSRF, enrollment and connected setup inventory verified. Fixture credentials remain private.\n",
);
