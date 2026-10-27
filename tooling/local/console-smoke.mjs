import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

// HTTP acceptance for the disposable console proxy, separate from rendered browser QA.
const origin = "http://127.0.0.1:3800";
const account = JSON.parse(readFileSync(".local/console-account.json", "utf8"));
async function request(
  path,
  { cookie, method = "GET", body, headers = {} } = {},
) {
  return fetch(`${origin}${path}`, {
    method,
    redirect: "manual",
    headers: {
      ...(cookie ? { Cookie: cookie } : {}),
      ...(body ? { "Content-Type": "application/json" } : {}),
      ...headers,
    },
    body: body ? JSON.stringify(body) : undefined,
    signal: AbortSignal.timeout(10000),
  });
}
async function signIn(email) {
  const response = await request("/v1/auth/login", {
    method: "POST",
    body: { email, password: account.password },
    headers: { Origin: origin },
  });
  assert.equal(response.status, 200, "fixture login through Next proxy");
  const setCookie = response.headers.get("set-cookie");
  assert.match(setCookie, /HttpOnly/i);
  assert.match(setCookie, /SameSite=Lax/i);
  const cookie = setCookie.split(";", 1)[0];
  return { cookie, session: await response.json() };
}
let ready = false;
for (const deadline = Date.now() + 30_000; Date.now() < deadline;) {
  try {
    ready = (await request("/v1/auth/me")).status === 401;
  } catch {
    /* A process can still be starting. */
  }
  if (ready) break;
  await new Promise((resolve) => setTimeout(resolve, 500));
}
assert.ok(ready, "start local:console:api and local:console before acceptance");
const anonymous = await request("/people");
assert.equal(anonymous.status, 307);
assert.equal(anonymous.headers.get("location"), "/sign-in");
const administrator = await signIn(account.email);
assert.equal(
  administrator.session.user.organizationId,
  "console-local",
  "refusing a non-fixture installation",
);
for (const path of [
  "/",
  "/projects",
  "/people",
  "/people/invite",
  "/clusters",
  "/settings/mcp",
  "/account",
]) {
  const response = await request(path, administrator);
  assert.equal(response.status, 200, path);
  assert.match(
    response.headers.get("cache-control"),
    /no-store/i,
    `${path} must not enter a shared cache`,
  );
  const text = await response.text();
  assert.ok(
    !text.includes('"digest":"'),
    `${path} returned a server-rendering failure`,
  );
}
const crossOrigin = await request("/v1/users", {
  cookie: administrator.cookie,
  method: "POST",
  body: {
    email: "blocked@example.test",
    name: "Blocked",
    role: "viewer",
    projects: [],
  },
  headers: {
    Origin: "https://other.example",
    "X-Keldock-CSRF": administrator.session.csrfToken,
  },
});
assert.equal(crossOrigin.status, 403);
const viewer = await signIn("viewer@keldock.test");
const projects = await request("/v1/projects", viewer);
assert.deepEqual(
  (await projects.json()).items.map((item) => item.projectId),
  ["sample"],
);
assert.equal((await request("/v1/users", viewer)).status, 403);
assert.equal(
  (await request("/v1/projects/empty-project/environments", viewer)).status,
  403,
);
const deniedPage = await request("/people", viewer);
if (deniedPage.status === 307) {
  assert.equal(deniedPage.headers.get("location"), "/");
} else {
  assert.equal(deniedPage.status, 200);
  const html = await deniedPage.text();
  assert.match(
    html,
    /NEXT_REDIRECT;replace;\/;307|http-equiv="refresh" content="0;url=\/"/,
  );
  assert.ok(
    !html.includes("admin@keldock.test"),
    "streamed redirect must not include administrator data",
  );
}
const mcpCookie = await request("/mcp", {
  cookie: administrator.cookie,
  method: "POST",
  body: { jsonrpc: "2.0", id: 1, method: "tools/list" },
  headers: { Accept: "application/json, text/event-stream" },
});
assert.equal(mcpCookie.status, 401);
assert.match(mcpCookie.headers.get("www-authenticate"), /resource_metadata=/);
const metadata = await request("/.well-known/oauth-protected-resource/mcp");
assert.equal(metadata.status, 200);
assert.equal((await metadata.json()).resource, `${origin}/mcp`);
for (const identity of [administrator, viewer]) {
  const response = await request("/v1/auth/logout", {
    cookie: identity.cookie,
    method: "POST",
    headers: { Origin: origin, "X-Keldock-CSRF": identity.session.csrfToken },
  });
  assert.equal(response.status, 200);
}
console.log(
  "Console HTTP acceptance passed: proxy cookies, authenticated SSR, no-store, CSRF, role/project denial and MCP discovery.",
);
