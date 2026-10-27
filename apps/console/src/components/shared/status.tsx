import { Badge } from "@/components/ui/badge";

const labels: Record<string, string> = {
  pending: "Queued",
  blocked: "Blocked",
  configured: "Configured",
  publishing: "Publishing",
  reconciling: "Deploying",
  ready: "Ready",
  failed: "Failed",
  canceled: "Canceled",
  superseded: "Superseded",
  deleted: "Deleted",
  connected: "Connected",
  offline: "Offline",
  unenrolled: "Not enrolled",
  revoked: "Revoked",
  expired: "Expired",
  active: "Active",
  invited: "Invited",
  disabled: "Disabled",
};
export function Status({ state }: { state: string }) {
  const tone = ["ready", "connected", "active", "configured"].includes(state)
    ? "success"
    : ["failed", "revoked", "disabled", "expired"].includes(state)
      ? "failure"
      : [
            "pending",
            "publishing",
            "reconciling",
            "offline",
            "canceled",
            "blocked",
          ].includes(state)
        ? "warning"
        : "neutral";
  return (
    <Badge
      variant={tone === "neutral" ? "secondary" : tone}
      className="status"
      data-tone={tone}
    >
      <span className="status-dot" aria-hidden="true" />
      {labels[state] ?? state}
    </Badge>
  );
}
