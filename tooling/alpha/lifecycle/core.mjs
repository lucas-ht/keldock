import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { randomUUID } from "node:crypto";
import {
  application,
  assertClaims,
  assertTarget,
  claims,
  directory,
  helm,
  input,
  kubectl,
  read,
  run,
  save,
} from "./fixture.mjs";
import { installSupplied, preserveDependencies } from "./supplied.mjs";

assertTarget();
const path = resolve(directory, "lifecycle-evidence.json");
const state = existsSync(path)
  ? read(path)
  : { target: "k3d-keldock-m6-core", stages: [] };
const done = (name) => {
  state.stages.push(name);
  save(path, state);
};
const onboard = () => run("node", ["tooling/alpha/onboard.mjs", "--core"]);
if (!state.stages.includes("baseline")) {
  onboard();
  await application();
  state.claims = claims();
  state.images = read(resolve(input, "values.json")).images;
  state.application = read(
    resolve(directory, "application-evidence.json"),
  ).receipt;
  writeFileSync(
    resolve(directory, "before-uninstall.yaml"),
    helm(["get", "manifest", "core"]),
    { mode: 0o600 },
  );
  preserveDependencies(
    readFileSync(resolve(directory, "before-uninstall.yaml"), "utf8"),
  );
  done("baseline");
}
if (!state.stages.includes("revoked")) {
  const { request } = await import("../onboard.mjs");
  const pods = JSON.parse(
    kubectl([
      "get",
      "pods",
      "-l",
      "keldock.io/process=registrar",
      "-o",
      "json",
    ]),
  ).items;
  const pod = pods.find(
    (item) =>
      item.status.phase === "Running" &&
      item.status.containerStatuses?.some((container) => container.ready),
  );
  if (!pod) throw new Error("Expected a ready registrar");
  const credential = JSON.parse(
    kubectl([
      "exec",
      pod.metadata.name,
      "-c",
      "registrar",
      "--",
      "cat",
      "/state/private/identity.json",
    ]),
  );
  await request("/v1/clusters/core/revoke", "POST");
  let denied = false;
  try {
    await request("/v1/agent/desired", "GET", undefined, {
      Authorization: `Bearer ${credential.identity.token}`,
      "X-Keldock-Agent-Session": randomUUID(),
    });
  } catch (error) {
    if (error.message.includes(": 403")) denied = true;
    else throw error;
  }
  if (!denied) throw new Error("Revoked registrar retained authority");
  await application();
  done("revoked");
}
if (!state.stages.includes("uninstalled")) {
  process.stdout.write(
    `Uninstalling only Helm release core in owned k3d-keldock-m6-core/keldock-core; preserving ${Object.keys(state.claims).join(", ")}.\n`,
  );
  helm(["uninstall", "core", "--wait", "--timeout", "3m"]);
  assertClaims(state.claims);
  await application();
  done("uninstalled");
}
if (!state.stages.includes("supplied")) {
  installSupplied();
  done("supplied");
}
if (!state.stages.includes("complete")) {
  onboard();
  assertClaims(state.claims);
  await application();
  state.completedAt = new Date().toISOString();
  done("complete");
}
process.stdout.write(
  "Packaged registrar revocation, uninstall retention, supplied-dependency reinstall and explicit re-enrollment passed; approved application remained available.\n",
);
