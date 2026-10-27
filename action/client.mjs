import { setTimeout as delay } from "node:timers/promises";
import { ActionError, origin, readBounded } from "./protocol.mjs";

function receipt(value, api, project) {
  for (const key of [
    "releaseId",
    "environmentId",
    "deploymentId",
    "operationId",
  ])
    if (
      typeof value[key] !== "string" ||
      !/^[a-z0-9-]{1,128}$/.test(value[key])
    )
      throw new ActionError("Invalid Keldock acceptance receipt");
  if (
    !Number.isSafeInteger(value.generation) ||
    value.generation < 1 ||
    value.statusUrl !==
      `/v1/projects/${project}/operations/${value.operationId}`
  )
    throw new ActionError("Acceptance receipt escaped the requested project");
  return {
    accepted: "true",
    ready: "false",
    "release-id": value.releaseId,
    "environment-id": value.environmentId,
    "deployment-id": value.deploymentId,
    "operation-id": value.operationId,
    generation: String(value.generation),
    "status-url": api + value.statusUrl,
    "console-url": `${api}/projects/${project}/environments/${value.environmentId}`,
    state: "accepted",
    "wait-timed-out": "false",
  };
}

// The Action neither signs inputs nor builds, publishes images, or talks to Kubernetes.
export async function deploy({
  apiURL,
  project,
  body,
  key,
  wait = true,
  timeout = 180000,
  token,
  fetcher = fetch,
  output = () => {},
  sleep = delay,
  now = Date.now,
  allowLocal = false,
}) {
  const api = origin(apiURL, allowLocal);
  if (
    !/^[a-z][a-z0-9-]{0,39}$/.test(project) ||
    !/^[a-zA-Z0-9._:-]{8,128}$/.test(key) ||
    body.length > 1024 * 1024
  )
    throw new ActionError("Invalid bounded deployment request");
  const request = async (
    method,
    url,
    payload,
    idempotent,
    deadline = Infinity,
  ) => {
    for (let attempt = 0; attempt < 3; attempt++) {
      if (now() >= deadline)
        throw new ActionError("Readiness deadline reached");
      const credential = await token(api);
      let response;
      try {
        response = await fetcher(url, {
          method,
          redirect: "error",
          signal: AbortSignal.timeout(
            Math.max(1, Math.min(15000, deadline - now())),
          ),
          headers: {
            Authorization: `Bearer ${credential}`,
            "Content-Type": "application/json",
            "ngrok-skip-browser-warning": "keldock-ci",
            ...(idempotent ? { "Idempotency-Key": key } : {}),
          },
          ...(payload ? { body: payload } : {}),
        });
      } catch {
        if (attempt === 2 || now() >= deadline)
          throw new ActionError(
            "Keldock request interrupted; retry with the same idempotency key",
          );
        await sleep(1000);
        continue;
      }
      if ([429, 502, 503, 504].includes(response.status) && attempt < 2) {
        await response.body?.cancel();
        const retry = Number(response.headers.get("Retry-After"));
        await sleep(
          Number.isFinite(retry) && retry > 0
            ? Math.min(retry * 1000, 15000)
            : 1000,
        );
        continue;
      }
      if (!response.ok) {
        await response.body?.cancel();
        throw new ActionError(
          `Keldock request rejected (HTTP ${response.status}); inspect the installation for details`,
        );
      }
      let text;
      try {
        text = await readBounded(response);
      } catch (error) {
        if (idempotent && attempt < 2 && !(error instanceof ActionError)) {
          await sleep(1000);
          continue;
        }
        throw error;
      }
      if (text.length > 1024 * 1024)
        throw new ActionError("Keldock response exceeded its bound");
      try {
        return JSON.parse(text);
      } catch {
        throw new ActionError("Invalid Keldock response");
      }
    }
  };
  const accepted = await request("POST", api + "/v1/releases", body, true);
  const outputs = receipt(accepted, api, project);
  await output(outputs);
  if (!wait) return outputs;
  const deadline = now() + timeout;
  while (now() < deadline) {
    let status;
    try {
      status = await request(
        "GET",
        outputs["status-url"],
        undefined,
        false,
        deadline,
      );
    } catch (error) {
      if (now() >= deadline) break;
      throw error;
    }
    if (
      status.operation?.id !== accepted.operationId ||
      status.operation?.generation !== accepted.generation
    )
      throw new ActionError(
        "Operation status did not match the accepted revision",
      );
    const state = status.operation.state;
    outputs.state = state;
    if (
      state === "ready" &&
      status.stale === false &&
      status.observation?.generation === accepted.generation &&
      status.observation?.deploymentId === accepted.deploymentId &&
      status.observation?.state === "ready"
    ) {
      outputs.ready = "true";
      await output(outputs);
      return outputs;
    }
    if (["failed", "canceled", "superseded", "deleted"].includes(state)) {
      await output(outputs);
      throw new ActionError(
        `Accepted deployment is ${state}; inspect ${outputs["console-url"]}`,
      );
    }
    await sleep(Math.min(5000, Math.max(0, deadline - now())));
  }
  outputs["wait-timed-out"] = "true";
  await output(outputs);
  throw new ActionError(
    `Client wait timed out; accepted operation ${accepted.operationId} continues in Keldock`,
  );
}
