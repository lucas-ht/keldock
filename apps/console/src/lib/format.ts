import type { ComponentHealth } from "./contracts";
export function age(seconds: number) {
  if (seconds < 60) return `${Math.floor(seconds)}s`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h`;
  return `${Math.floor(seconds / 86400)}d`;
}
export function date(value?: string) {
  return value
    ? new Intl.DateTimeFormat("en", {
        dateStyle: "medium",
        timeStyle: "short",
        timeZone: "UTC",
      }).format(new Date(value)) + " UTC"
    : "Not yet";
}
export function digest(image?: string) {
  return image?.split("sha256:")[1]?.slice(0, 12) ?? "—";
}
export function environmentName(pr: number) {
  return pr ? `PR #${pr}` : "Staging";
}
export function canOperate(role: string) {
  return role === "admin" || role === "operator";
}

export function componentState(
  health: ComponentHealth | undefined,
  stale: boolean,
): string {
  if (stale) return "Unknown";
  if (!health) return "pending";
  if (health.reason === "ScheduleConfigured") return "configured";
  if (health.ready) return "ready";
  if (health.reason === "DependencyNotReady") return "blocked";
  return /Failed|Error|Unhealthy/.test(health.reason)
    ? "failed"
    : "reconciling";
}
