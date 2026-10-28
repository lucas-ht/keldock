import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { resolve } from "node:path";
import { parseAllDocuments } from "yaml";

const directory = mkdtempSync(resolve(tmpdir(), "keldock-chart-"));
const image = `example.invalid/keldock@sha256:${"a".repeat(64)}`;
function render(overrides = {}) {
  const path = resolve(directory, "values.json");
  writeFileSync(
    path,
    JSON.stringify({
      images: { runtime: image, console: image },
      installation: {
        existingSecret: "private-input",
        publicURL: "https://control.example.test",
      },
      storageClass: "tested-storage",
      ingress: {
        enabled: true,
        host: "control.example.test",
        tlsSecret: "tls-input",
      },
      ...overrides,
    }),
  );
  return parseAllDocuments(
    execFileSync(
      "helm",
      [
        "template",
        "review",
        "packaging/charts/keldock",
        "--namespace",
        "review",
        "--kube-version",
        "1.35.5",
        "--values",
        path,
      ],
      { encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] },
    ),
  )
    .map((document) => {
      assert.equal(document.errors.length, 0);
      return document.toJSON();
    })
    .filter(Boolean);
}
try {
  const core = render();
  const publicSA = core.find(
    (item) =>
      item.kind === "ServiceAccount" && item.metadata.name === "review-public",
  );
  assert.equal(publicSA.automountServiceAccountToken, false);
  for (const item of core.filter((item) => item.kind.endsWith("Binding")))
    assert.ok(
      !(item.subjects ?? []).some(
        (subject) => subject.name === "review-public",
      ),
      "Public processes must not receive Kubernetes authority",
    );
  assert.equal(
    core.filter((item) => item.kind === "PersistentVolumeClaim").length,
    4,
  );
  for (const claim of core.filter(
    (item) => item.kind === "PersistentVolumeClaim",
  ))
    assert.equal(claim.metadata.annotations["helm.sh/resource-policy"], "keep");
  for (const item of core.filter((item) => item.spec?.template?.spec))
    for (const container of [
      ...(item.spec.template.spec.initContainers ?? []),
      ...(item.spec.template.spec.containers ?? []),
    ]) {
      assert.match(container.image, /@sha256:[a-f0-9]{64}$/);
      assert.equal(container.securityContext.readOnlyRootFilesystem, true);
      assert.equal(container.securityContext.allowPrivilegeEscalation, false);
      assert.ok(container.resources?.limits?.memory);
    }
  const supplied = render({
    database: { enabled: false },
    registry: { enabled: false },
    registrar: { enabled: false },
  });
  assert.equal(
    supplied.filter((item) => item.kind === "StatefulSet").length,
    0,
  );
  assert.equal(
    supplied.filter((item) => item.kind === "PersistentVolumeClaim").length,
    1,
  );
  const data = render({ databases: { enabled: true } });
  const provisioner = data.find(
    (item) =>
      item.kind === "ClusterRole" && item.metadata.name.includes("database"),
  );
  assert.ok(provisioner);
  for (const rule of provisioner.rules)
    if (
      rule.resources.includes("persistentvolumeclaims") ||
      rule.resources.includes("clusters")
    )
      assert.ok(
        !rule.verbs.includes("delete"),
        "Database agent cannot delete retained clusters or PVCs",
      );
  const egress = [
    {
      to: [{ ipBlock: { cidr: "192.0.2.42/32" } }],
      ports: [{ protocol: "TCP", port: 5432 }],
    },
  ];
  const customized = render({
    networking: { controlEgress: egress, agentEgress: [] },
  });
  assert.ok(
    customized
      .find(
        (item) =>
          item.kind === "NetworkPolicy" &&
          item.metadata.name === "review-control",
      )
      .spec.egress.some(
        (rule) => rule.to?.[0]?.ipBlock?.cidr === "192.0.2.42/32",
      ),
  );
  assert.throws(
    () =>
      render({
        images: { runtime: "example.invalid/runtime:latest", console: image },
      }),
    "Mutable runtime tags must be rejected",
  );
  assert.throws(
    () => render({ storageClass: "" }),
    "Storage must be an explicit operator choice",
  );
  process.stdout.write(
    "Packaged core/supplied/database profiles, retained data, process isolation, immutable images and rejected unsafe inputs verified.\n",
  );
} finally {
  rmSync(directory, { recursive: true, force: true });
}
