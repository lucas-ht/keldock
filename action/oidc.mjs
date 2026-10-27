import { ActionError, readBounded } from "./protocol.mjs";

export function oidcProvider(env, fetcher = fetch, now = Date.now) {
  let cached = "",
    expires = 0;
  return async (audience) => {
    if (cached && now() + 60000 < expires) return cached;
    let endpoint;
    try {
      endpoint = new URL(env.ACTIONS_ID_TOKEN_REQUEST_URL);
    } catch {
      throw new ActionError(
        "GitHub OIDC is unavailable; grant id-token: write to the deployment job",
      );
    }
    if (
      endpoint.protocol !== "https:" ||
      !endpoint.hostname.endsWith(".actions.githubusercontent.com") ||
      endpoint.username ||
      endpoint.password ||
      !env.ACTIONS_ID_TOKEN_REQUEST_TOKEN
    )
      throw new ActionError("Unsupported GitHub OIDC endpoint");
    endpoint.searchParams.set("audience", audience);
    let response;
    try {
      response = await fetcher(endpoint, {
        headers: {
          Authorization: `Bearer ${env.ACTIONS_ID_TOKEN_REQUEST_TOKEN}`,
        },
        redirect: "error",
        signal: AbortSignal.timeout(15000),
      });
    } catch {
      throw new ActionError("GitHub OIDC request unavailable");
    }
    if (!response.ok) throw new ActionError("GitHub OIDC request rejected");
    const text = await readBounded(response, 65536);
    if (text.length > 65536)
      throw new ActionError("GitHub OIDC response exceeded its bound");
    let value;
    try {
      value = JSON.parse(text).value;
    } catch {
      throw new ActionError("Invalid GitHub OIDC response");
    }
    if (
      typeof value !== "string" ||
      value.length > 32768 ||
      value.split(".").length !== 3
    )
      throw new ActionError("Invalid GitHub OIDC response");
    let expiration;
    try {
      expiration =
        JSON.parse(Buffer.from(value.split(".")[1], "base64url").toString())
          .exp * 1000;
    } catch {
      throw new ActionError("Invalid GitHub OIDC expiry");
    }
    if (!Number.isFinite(expiration) || expiration <= now())
      throw new ActionError("Expired GitHub OIDC credential");
    cached = value;
    expires = expiration;
    return cached;
  };
}
