import type { CloudIntent } from "@/lib/cloud-contracts";
import { Badge } from "@/components/ui/badge";

export function CloudState({ item, now }: { item: CloudIntent; now: number }) {
  const stale =
    item.observation && now - Date.parse(item.observation.observedAt) > 120_000;
  const state = stale ? "stale" : item.state;
  const tone =
    state === "ready"
      ? "success"
      : state === "failed"
        ? "failure"
        : ["pending", "reconciling", "suspended", "stale"].includes(state)
          ? "warning"
          : "neutral";
  const labels: Record<string, string> = {
    ready: "Ready",
    failed: "Failed",
    pending: "Queued",
    reconciling: "Reconciling",
    suspended: "Suspended",
    retained: "Retained",
    disconnected: "Disconnected",
    stale: "Evidence stale",
  };
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

export function cloudReason(item: CloudIntent) {
  const reason = item.observation?.reason;
  const labels: Record<string, string> = {
    ExactProviderIdentitiesVerified:
      "Provider identities verified; resource permissions are checked separately.",
    BucketIdentityAndSecurityVerified:
      "Bucket owner, region, privacy, encryption, and versioning verified.",
    ExactConsumerIdentityAndPolicyVerified:
      "Component identity and IAM policy verified. Check application access after rollout.",
    OwnedDNSAndCertificateVerified:
      "Owned DNS records and certificate are ready.",
    CertificatePending: "DNS is configured; waiting for the certificate.",
    BaseBackupAndWALReported:
      "Base backup completed and WAL archiving is healthy.",
    BaseBackupFailed_RetryRequired:
      "The base backup failed. Review database and archive access, then retry.",
    BackupObjectStoreAccessConverging:
      "Waiting for the database’s permission to use its selected archive.",
    IsolatedRestorePending: "The isolated restore is running.",
    ExternalStorageRetained: "Cloud storage and its data are retained.",
    ConnectionRevisionRequiresExplicitRefresh:
      "The connection changed. Refresh this resource’s authority before resuming.",
    DNSOwnershipOrObservationConflict:
      "DNS ownership could not be verified. Review the existing records before retrying.",
    ReconciliationSuspended_AWSAuthorityRequiresOperatorRevocation:
      "Cloud reconciliation is suspended. An operator must separately revoke AWS provisioning authority.",
    AdoptionReviewStaleOrWriterNotReleased:
      "The bucket inventory or writer release changed. Ask an operator for a fresh adoption review.",
  };
  if (!reason) return "Waiting for the cloud agent.";
  if (reason.startsWith("AWS_"))
    return `AWS returned ${reason.slice(4)}. Ask an operator to review the approved identity and resource permissions.`;
  return (
    labels[reason] ??
    reason.replaceAll("_", " ").replace(/([a-z])([A-Z])/g, "$1 $2")
  );
}
