import "server-only";
import { cache } from "react";
import { cookies } from "next/headers";
import { redirect } from "next/navigation";
import type { Session } from "./contracts";

const origin = process.env.KELDOCK_API_ORIGIN ?? "http://127.0.0.1:3801";
export class APIError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
  }
}
export async function api<T>(path: string): Promise<T> {
  if (!path.startsWith("/v1/") || path.startsWith("//"))
    throw new Error("Invalid API path");
  const jar = await cookies();
  const session =
    jar.get("__Host-keldock_session") ?? jar.get("keldock_session");
  const headers = new Headers();
  if (session) headers.set("Cookie", `${session.name}=${session.value}`);
  const response = await fetch(`${origin}${path}`, {
    headers,
    cache: "no-store",
    signal: AbortSignal.timeout(10_000),
  });
  if (!response.ok) {
    const body = await response.json().catch(() => null);
    throw new APIError(
      response.status,
      body?.error?.message ??
        "The control plane is unavailable. Try refreshing.",
    );
  }
  return response.json() as Promise<T>;
}
export const getSession = cache(async (): Promise<Session | null> => {
  try {
    return await api<Session>("/v1/auth/me");
  } catch (error) {
    if (error instanceof APIError && error.status === 401) return null;
    throw error;
  }
});
export async function requireSession() {
  const session = await getSession();
  if (!session) redirect("/sign-in");
  return session;
}
export async function requireAdmin() {
  const session = await requireSession();
  if (session.user.role !== "admin") redirect("/");
  return session;
}
