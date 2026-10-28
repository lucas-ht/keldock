import { createHash } from "node:crypto";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { parseAllDocuments, stringify } from "yaml";

// This renderer never applies or adopts an existing controller. Its output is
// included in the reviewed release kit for an operator's separate bootstrap.
export async function renderDependencies(destination) {
  const root = resolve(import.meta.dirname, "../..");
  const lock = JSON.parse(
    readFileSync(
      resolve(root, "internal/cloud/crossplane/controllers.lock.json"),
    ),
  );
  mkdirSync(destination, { recursive: true, mode: 0o700 });
  const outputs = [];
  for (const name of ["cert-manager", "cnpg", "barman"]) {
    const pin = lock.artifacts[name];
    const cached = resolve(root, ".local/m5", pin.file);
    let data;
    if (existsSync(cached)) data = readFileSync(cached);
    else {
      const response = await fetch(pin.url, {
        signal: AbortSignal.timeout(120_000),
      });
      if (!response.ok)
        throw new Error(`Cannot download pinned ${name}: ${response.status}`);
      data = Buffer.from(await response.arrayBuffer());
    }
    if (createHash("sha256").update(data).digest("hex") !== pin.sha256)
      throw new Error(`Pinned ${name} checksum mismatch`);
    let source = data.toString("utf8");
    for (const [tag, digest] of Object.entries(lock.images))
      source = source.replaceAll(tag, digest);
    const documents = parseAllDocuments(source).map((document) => {
      if (document.errors.length)
        throw new Error(`Invalid upstream ${name} manifest`);
      const object = document.toJSON();
      // Upstream stores the injected sidecar image as base64 Secret data. A
      // textual image replacement cannot pin this otherwise hidden runtime image.
      if (object?.kind === "Secret" && object.data?.SIDECAR_IMAGE) {
        let image = Buffer.from(object.data.SIDECAR_IMAGE, "base64")
          .toString("utf8")
          .trim();
        image = lock.images[image] ?? image;
        if (!image.includes("@sha256:"))
          throw new Error("Unpinned backup sidecar");
        object.data.SIDECAR_IMAGE = Buffer.from(image).toString("base64");
      }
      if (name === "barman" && object?.kind === "Deployment") {
        object.spec.template.metadata.annotations ??= {};
        object.spec.template.metadata.annotations["keldock.io/sidecar-pin"] =
          lock.images[
            "ghcr.io/cloudnative-pg/plugin-barman-cloud-sidecar:v0.15.1"
          ].split("@")[1];
        object.spec.template.metadata.annotations["keldock.io/config-sha256"] =
          createHash("sha256")
            .update(
              lock.images[
                "ghcr.io/cloudnative-pg/plugin-barman-cloud-sidecar:v0.15.1"
              ],
            )
            .digest("hex");
      }
      return object;
    });
    const path = resolve(destination, `${name}.yaml`);
    writeFileSync(
      path,
      documents.map((value) => stringify(value)).join("---\n"),
      { mode: 0o600 },
    );
    outputs.push(path);
  }
  return outputs;
}
