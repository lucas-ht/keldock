"use client";
import type { Session } from "./contracts";

export async function read<T>(path: string): Promise<T> {
  const response = await fetch(path, {
    credentials: "same-origin",
    cache: "no-store",
  });
  const body = await response.json().catch(() => null);
  if (!response.ok)
    throw new Error(
      body?.error?.message ?? "The control plane is unavailable. Try again.",
    );
  return body as T;
}
export async function mutate<T>(
  path: string,
  input?: unknown,
  method = "POST",
  headers: Record<string, string> = {},
): Promise<T> {
  const publicAction = [
    "/v1/auth/login",
    "/v1/auth/activate",
    "/v1/auth/invitations/info",
  ].includes(path);
  if (!publicAction) {
    const session = await read<Session>("/v1/auth/me");
    headers["X-Keldock-CSRF"] = session.csrfToken;
  }
  const response = await fetch(path, {
    method,
    credentials: "same-origin",
    cache: "no-store",
    headers: { "Content-Type": "application/json", ...headers },
    body: input === undefined ? undefined : JSON.stringify(input),
  });
  const body = await response.json().catch(() => null);
  if (!response.ok)
    throw new Error(
      body?.error?.message ??
        "The action could not be completed. Refresh and try again.",
    );
  return body as T;
}
