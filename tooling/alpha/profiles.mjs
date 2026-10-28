import { resolve } from "node:path";

// Only named disposable targets are accepted by the alpha harness.
export const profile = process.argv.includes("--core")
  ? {
      cluster: "keldock-m6-core",
      context: "k3d-keldock-m6-core",
      release: "core",
      namespace: "keldock-core",
      organization: "core-fixture",
      host: "keldock-core.localhost",
      port: 24443,
      directory: resolve(".local/m6/core"),
    }
  : {
      cluster: "keldock-m6",
      context: "k3d-keldock-m6",
      release: "alpha",
      namespace: "keldock-alpha",
      organization: "alpha-fixture",
      host: "keldock-m6.localhost",
      port: 443,
      directory: resolve(".local/m6"),
    };
profile.origin = `https://${profile.host}`;
