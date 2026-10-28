import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { parseAllDocuments, stringify } from "yaml";

// Render the exact supplied Flux profile; the caller separately reviews/applies it.
export function renderFlux(path) {
  const root = resolve(import.meta.dirname, "../..");
  const lock = JSON.parse(
    readFileSync(
      resolve(root, "packaging/dependencies/flux.lock.json"),
      "utf8",
    ),
  );
  const source = execFileSync(
    "mise",
    [
      "exec",
      "--",
      "flux",
      "install",
      "--export",
      "--components=source-controller,kustomize-controller",
      "--namespace=keldock-system",
    ],
    { cwd: root, encoding: "utf8", maxBuffer: 16 * 1024 * 1024 },
  );
  const docs = parseAllDocuments(source).map((doc) => {
    if (doc.errors.length) throw new Error("Invalid Flux manifest");
    return doc.toJSON();
  });
  for (const doc of docs)
    if (doc.kind === "Deployment") {
      const container = doc.spec.template.spec.containers[0];
      if (!lock.images[doc.metadata.name])
        throw new Error("Unexpected Flux controller");
      container.image = lock.images[doc.metadata.name];
      if (doc.metadata.name === "kustomize-controller")
        container.args.push(
          "--no-cross-namespace-refs=true",
          "--no-remote-bases=true",
          "--default-service-account=untrusted",
          "--requeue-dependency=2s",
        );
    }
  writeFileSync(path, docs.map((doc) => stringify(doc)).join("---\n"));
  return path;
}
