import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { parseAllDocuments } from "yaml";
import {
  apply,
  directory,
  input,
  run,
  kubectl,
  helm,
  read,
  save,
} from "./fixture.mjs";

export function preserveDependencies(manifest) {
  const objects = parseAllDocuments(manifest)
    .map((item) => item.toJSON())
    .filter(Boolean);
  const secret = read(resolve(input, "installation-secret.json"));
  const name = "core-supplied-database";
  const dependencies = [
    {
      apiVersion: "v1",
      kind: "Secret",
      metadata: { name, namespace: "keldock-core" },
      data: Object.fromEntries(
        ["postgres-password", "postgres-admin-password"].map((key) => [
          key,
          secret.data[key],
        ]),
      ),
    },
  ];
  for (const object of objects) {
    if (!(
      (object.kind === "StatefulSet" ||
        object.kind === "Service" ||
        object.kind === "ConfigMap" ||
        object.kind === "NetworkPolicy") &&
      /^core-(database|registry)(-init)?$/.test(object.metadata.name)
    ))
      continue;
    delete object.metadata.annotations;
    object.metadata.labels = {
      "keldock.alpha-fixture": "true",
      "keldock.io/ownership": "supplied-fixture",
    };
    if (object.kind === "NetworkPolicy")
      object.metadata.name = `supplied-${object.metadata.name}`;
    if (object.kind === "StatefulSet")
      for (const container of object.spec.template.spec.containers)
        for (const variable of container.env ?? [])
          if (variable.valueFrom?.secretKeyRef)
            variable.valueFrom.secretKeyRef.name = name;
    dependencies.push(object);
  }
  if (dependencies.filter((item) => item.kind === "StatefulSet").length !== 2)
    throw new Error("Expected exactly the two retained supplied dependencies");
  save(resolve(directory, "supplied-dependencies.json"), {
    apiVersion: "v1",
    kind: "List",
    items: dependencies,
  });
}

export function installSupplied() {
  apply(read(resolve(directory, "supplied-dependencies.json")).items);
  for (const name of ["database", "registry"])
    kubectl([
      "rollout",
      "status",
      `statefulset/core-${name}`,
      "--timeout=180s",
    ]);
  const supplied = resolve(directory, "installation-supplied");
  const binary = resolve(".local/m6/keldock");
  if (!existsSync(supplied)) {
    const control = read(resolve(input, "control.json"));
    const dbfile = resolve(directory, "supplied-database-url");
    writeFileSync(dbfile, control.databaseUrl, { mode: 0o600 });
    run(binary, [
      "installation",
      "init",
      "--directory",
      supplied,
      "--release-file",
      resolve(directory, "release-fixture.json"),
      "--name",
      "core",
      "--namespace",
      "keldock-core",
      "--organization",
      "core-fixture",
      "--public-url",
      "https://keldock-core.localhost",
      "--domain",
      "alpha.keldock.test",
      "--storage-class",
      "local-path",
      "--database-url-file",
      dbfile,
      "--registry-push",
      control.registryPush,
      "--registry-pull",
      control.registryPull,
      "--registry-plain-http",
      "--development",
    ]);
    run(binary, [
      "installation",
      "project",
      "--directory",
      supplied,
      "--file",
      resolve(directory, "repository-policy.json"),
    ]);
  }
  const original = read(resolve(input, "values.json"));
  const values = read(resolve(supplied, "values.json"));
  values.hostAliases = original.hostAliases;
  values.extraTrustSecret = original.extraTrustSecret;
  values.images = original.images;
  save(resolve(supplied, "values.json"), values);
  run(binary, ["installation", "render-secret", "--directory", supplied]);
  kubectl(["apply", "-f", resolve(supplied, "installation-secret.json")]);
  run(binary, [
    "installation",
    "preflight",
    "--directory",
    supplied,
    ...[
      "--kubeconfig",
      resolve(directory, "kubeconfig"),
      "--context",
      "k3d-keldock-m6-core",
    ],
  ]);
  helm([
    "upgrade",
    "--install",
    "core",
    "packaging/charts/keldock",
    "--values",
    resolve(supplied, "values.json"),
    "--wait",
    "--timeout",
    "5m",
  ]);
  const manifest = helm(["get", "manifest", "core"]);
  if (
    parseAllDocuments(manifest).some(
      (item) => item.toJSON()?.kind === "StatefulSet",
    )
  )
    throw new Error(
      "Supplied profile unexpectedly owns database/registry StatefulSets",
    );
  process.stdout.write(
    "Supplied-dependency installation reused the exact retained PostgreSQL and registry PVCs; Helm owns neither dependency.\n",
  );
}
