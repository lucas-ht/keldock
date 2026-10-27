export class ActionError extends Error {}

export function origin(value, allowLocal = false) {
  let url;
  try {
    url = new URL(value);
  } catch {
    throw new ActionError("A canonical HTTPS API origin is required");
  }
  const local =
    allowLocal &&
    url.protocol === "http:" &&
    ["127.0.0.1", "localhost"].includes(url.hostname);
  if (
    (url.protocol !== "https:" && !local) ||
    url.username ||
    url.password ||
    url.pathname !== "/" ||
    url.search ||
    url.hash
  )
    throw new ActionError("A canonical HTTPS API origin is required");
  return url.origin;
}

export async function readBounded(response, maximum = 1024 * 1024) {
  if (!response.body) return "";
  const reader = response.body.getReader();
  const chunks = [];
  let length = 0;
  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    length += value.byteLength;
    if (length > maximum) {
      await reader.cancel();
      throw new ActionError("HTTP response exceeded its bound");
    }
    chunks.push(value);
  }
  return Buffer.concat(chunks).toString("utf8");
}
