import type { Cluster } from "./contracts";

export type BackupEvidence = {
  state: string;
  reason: string;
  baseBackup?: string;
  walObjects: boolean;
  walHealthy: boolean;
  lastArchivedAt?: string;
  lastBaseBackupAt?: string;
  restoreNamespace?: string;
  restoreName?: string;
  restoreUid?: string;
  restoreReady: boolean;
};
export type DatabaseAllocation = {
  id: string;
  environmentId: string;
  alias: string;
  clusterId: string;
  namespace: string;
  profile: string;
  configuration: { storageClass: string; storageSize: string };
  generation: number;
  sourceUid?: string;
  action: "reconcile" | "retain";
  components: string[];
  backup?: {
    destinationId: string;
    baseAttempt: number;
    restoreAttempt: number;
    paused: boolean;
  };
  observation?: {
    generation: number;
    state: string;
    reason: string;
    observedAt: string;
    backup?: BackupEvidence;
  };
};
export type DatabaseInventory = {
  items: DatabaseAllocation[];
  agent: Cluster;
  configuration: {
    limit: number;
    profiles: Record<string, { storageClass: string; storageSize: string }>;
    destinations: { id: string; allocationId: string; description: string }[];
  };
};
