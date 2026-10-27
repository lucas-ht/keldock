import { appendFile, readFile, stat } from "node:fs/promises";
import { deploy } from "./client.mjs";
import { ActionError } from "./protocol.mjs";
import { oidcProvider } from "./oidc.mjs";

const input = (name) =>
  process.env[`INPUT_${name.toUpperCase()}`]?.trim() || "";
let last, failure;
try {
  const path = input("release-file");
  const info = await stat(path);
  if (!info.isFile() || info.size > 1024 * 1024)
    throw new ActionError("A bounded signed release file is required");
  const body = await readFile(path, "utf8");
  let release;
  try {
    release = JSON.parse(body);
  } catch {
    throw new ActionError("Invalid signed release JSON");
  }
  const project = input("project");
  if (
    release.apiVersion !== "keldock.io/v1alpha1" ||
    release.projectId !== project ||
    !release.provenance?.signatures?.length
  )
    throw new ActionError(
      "A signed release for the requested project is required",
    );
  const timeout = Number(input("timeout") || "180");
  if (
    !Number.isInteger(timeout) ||
    timeout < 10 ||
    timeout > 900 ||
    !["", "true", "false"].includes(input("wait"))
  )
    throw new ActionError("Invalid wait or timeout input");
  const key =
    input("idempotency-key") ||
    `github:${process.env.GITHUB_REPOSITORY_ID}:${process.env.GITHUB_RUN_ID}:${process.env.GITHUB_RUN_ATTEMPT}:${project}`;
  await deploy({
    apiURL: input("api-url"),
    project,
    body,
    key,
    wait: input("wait") !== "false",
    timeout: timeout * 1000,
    token: oidcProvider(process.env),
    output: async (values) => {
      last = { ...values };
      if (!process.env.GITHUB_OUTPUT)
        throw new ActionError("GitHub Action output channel is missing");
      await appendFile(
        process.env.GITHUB_OUTPUT,
        Object.entries(last)
          .map(([key, value]) => `${key}=${value}\n`)
          .join(""),
      );
    },
  });
} catch (error) {
  failure = error;
}

// Preserve the durable receipt even when readiness fails, expires or is interrupted.
if (last) {
  try {
    if (!process.env.GITHUB_OUTPUT)
      throw new ActionError("GitHub Action output channel is missing");
    await appendFile(
      process.env.GITHUB_OUTPUT,
      Object.entries(last)
        .map(([key, value]) => `${key}=${value}\n`)
        .join(""),
    );
    if (process.env.GITHUB_STEP_SUMMARY)
      await appendFile(
        process.env.GITHUB_STEP_SUMMARY,
        `### Keldock deployment\n\nAccepted generation ${last.generation}. State: **${last.state}**.\n\n[Inspect environment](${last["console-url"]}) · Operation ${last["operation-id"]}\n\nClient wait timed out: ${last["wait-timed-out"]}. Accepted work is not canceled when this runner stops.\n`,
      );
  } catch (error) {
    failure ||= error;
  }
}
if (failure) {
  console.error(
    `::error::${failure instanceof ActionError ? failure.message : "Keldock action could not read its inputs or complete the request"}`,
  );
  process.exitCode = 1;
}
