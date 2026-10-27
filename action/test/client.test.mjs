import assert from "node:assert/strict";
import test from "node:test";
import { deploy } from "../client.mjs";
import { oidcProvider } from "../oidc.mjs";
import { origin } from "../protocol.mjs";

const receipt = {
  releaseId: "rel-test",
  environmentId: "env-test",
  deploymentId: "dep-test",
  operationId: "op-test",
  generation: 1,
  statusUrl: "/v1/projects/sample/operations/op-test",
};
const response = (body, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
const base = {
  apiURL: "https://keldock.example",
  project: "sample",
  body: "{}",
  key: "github:42:1:1:sample",
  token: async () => "test-oidc-credential",
  sleep: async () => {},
};

test("uncertain submission retries identical bytes and returns a durable submit-only receipt", async () => {
  const requests = [];
  const result = await deploy({
    ...base,
    wait: false,
    fetcher: async (url, options) => {
      requests.push({ url, options });
      if (requests.length === 1) throw new Error("lost response");
      return response(receipt, 202);
    },
  });
  assert.equal(requests.length, 2);
  assert.equal(requests[0].options.body, requests[1].options.body);
  assert.equal(
    requests[0].options.headers["Idempotency-Key"],
    requests[1].options.headers["Idempotency-Key"],
  );
  assert.equal(result.accepted, "true");
  assert.equal(result.ready, "false");
  assert.equal(result.state, "accepted");
  assert.equal(requests[0].options.redirect, "error");
});

test("readiness requires fresh matching deployment evidence", async () => {
  let reads = 0;
  const result = await deploy({
    ...base,
    fetcher: async (_url, options) =>
      options.method === "POST"
        ? response(receipt, 202)
        : response({
            operation: { id: "op-test", generation: 1, state: "ready" },
            stale: ++reads === 1,
            observation: {
              generation: 1,
              deploymentId: "dep-test",
              state: "ready",
            },
          }),
  });
  assert.equal(reads, 2);
  assert.equal(result.ready, "true");
});

test("a client timeout preserves acceptance and never cancels the deployment", async () => {
  let time = 0,
    last;
  const methods = [];
  await assert.rejects(
    deploy({
      ...base,
      timeout: 10,
      now: () => time,
      sleep: async (ms) => {
        time += ms;
      },
      output: (value) => {
        last = { ...value };
      },
      fetcher: async (url, options) => {
        methods.push([url, options.method]);
        return options.method === "POST"
          ? response(receipt, 202)
          : response({
              operation: { id: "op-test", generation: 1, state: "reconciling" },
              stale: true,
            });
      },
    }),
    /continues in Keldock/,
  );
  assert.equal(last.accepted, "true");
  assert.equal(last.ready, "false");
  assert.equal(last["wait-timed-out"], "true");
  assert.equal(methods.filter(([, method]) => method === "POST").length, 1);
});

test("receipt and evidence cannot escape project or desired revision", async () => {
  for (const statusUrl of [
    "https://other.example/steal",
    "/v1/projects/other/operations/op-test",
  ]) {
    await assert.rejects(
      deploy({
        ...base,
        fetcher: async () => response({ ...receipt, statusUrl }, 202),
      }),
      /escaped the requested project/,
    );
  }
  await assert.rejects(
    deploy({
      ...base,
      fetcher: async (_url, options) =>
        options.method === "POST"
          ? response(receipt, 202)
          : response({
              operation: { id: "op-other", generation: 2, state: "ready" },
              stale: false,
            }),
    }),
    /did not match/,
  );
  for (const value of [
    "http://keldock.example",
    "https://user:password@keldock.example",
    "https://keldock.example/path",
    "https://keldock.example?secret=value",
  ]) {
    assert.throws(() => origin(value), /HTTPS/);
  }
});

test("OIDC is audience-bound, cached until refresh, and restricted to GitHub's endpoint", async () => {
  let time = 1000000000,
    calls = 0;
  const provider = oidcProvider(
    {
      ACTIONS_ID_TOKEN_REQUEST_URL:
        "https://vstoken.actions.githubusercontent.com/token",
      ACTIONS_ID_TOKEN_REQUEST_TOKEN: "runner-test-credential",
    },
    async (url, options) => {
      calls++;
      assert.equal(new URL(url).searchParams.get("audience"), base.apiURL);
      assert.equal(options.redirect, "error");
      return response({
        value: `header.${Buffer.from(JSON.stringify({ exp: Math.floor(time / 1000) + 300 })).toString("base64url")}.signature`,
      });
    },
    () => time,
  );
  await provider(base.apiURL);
  await provider(base.apiURL);
  assert.equal(calls, 1);
  time += 250000;
  await provider(base.apiURL);
  assert.equal(calls, 2);
  await assert.rejects(
    oidcProvider({
      ACTIONS_ID_TOKEN_REQUEST_URL: "https://other.example/token",
      ACTIONS_ID_TOKEN_REQUEST_TOKEN: "test",
    })(base.apiURL),
    /Unsupported/,
  );
});
