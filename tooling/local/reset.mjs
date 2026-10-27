import { execFileSync } from "node:child_process";
import { resolve } from "node:path";

process.chdir(resolve(import.meta.dirname, "../.."));
const clusters = JSON.parse(
  execFileSync("k3d", ["cluster", "list", "-o", "json"], { encoding: "utf8" }),
);
if (clusters.some((cluster) => cluster.name === "keldock-m1")) {
  const node = JSON.parse(
    execFileSync("docker", ["inspect", "k3d-keldock-m1-server-0"], {
      encoding: "utf8",
    }),
  )[0];
  if (node.Config.Labels?.["keldock.local-fixture"] !== "true")
    throw new Error("Refusing to reset a cluster not owned by this fixture");
  process.stdout.write(
    "Reset target: disposable cluster/context keldock-m1 / k3d-keldock-m1 on local Docker.\n",
  );
  execFileSync("k3d", ["cluster", "delete", "keldock-m1"], {
    stdio: "inherit",
  });
}
await import("./up.mjs");
