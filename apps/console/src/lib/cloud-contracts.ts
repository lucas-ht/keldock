import type { Cluster } from "./contracts";

export type CloudProfile = {
  id: string;
  kind: "object-storage" | "backup-store" | "domain";
  account: string;
  capacity: number;
  existingBuckets: string[] | null;
  parentDomain?: string;
};
export type CloudCatalog = {
  accounts: string[];
  regions: string[];
  identities: string[];
  profiles: CloudProfile[];
  databases: string[];
};
export type CloudEvidence = {
  generation: number;
  connectionGeneration: number;
  state: string;
  reason: string;
  details: Record<string, string>;
  observedAt: string;
};
export type CloudIntent = {
  id: string;
  projectId: string;
  kind: "connection" | "resource" | "binding" | "domain" | "backup";
  generation: number;
  revision: number;
  action: string;
  state: string;
  updatedAt: string;
  observation?: CloudEvidence;
  connection?: {
    account: string;
    region: string;
    identityRef: string;
    profiles: string[];
    enabled: boolean;
    clusterId: string;
  };
  resource?: {
    kind: "object-storage" | "backup-store";
    profile: string;
    connectionId: string;
    connectionGeneration: number;
    mode: "create" | "connect" | "adopt";
    externalName: string;
    owner: string;
    retention: "retain";
  };
  binding?: {
    resourceId: string;
    environmentId: string;
    components: string[];
    access: "read" | "read-write";
    consumers: {
      component: string;
      namespace: string;
      serviceAccount: string;
      roleArn: string;
      prefix: string;
    }[];
  };
  domain?: {
    request: {
      connectionId: string;
      profile: string;
      hostname: string;
      environmentId: string;
      component: string;
    };
    ownershipVerified: boolean;
    dnsReady: boolean;
    certificateReady: boolean;
  };
  backup?: {
    resourceId: string;
    databaseId: string;
    database: { namespace: string; name: string; restoreNamespace: string };
    archiveGeneration: number;
    archivePrefix: string;
    schedule: string;
    schedulePaused: boolean;
    restoreName: string;
    restoreGeneration?: number;
    archives: { generation: number; prefix: string }[];
  };
};
export type CloudInventory = {
  items: CloudIntent[];
  configuration: CloudCatalog;
  agent: Cluster;
};
export type CloudEnvironment = {
  id: string;
  label: string;
  generation: number;
  components: { name: string; kind: string; routed?: boolean }[];
};
export type CloudCommand = {
  kind:
    "connection" | "resource" | "binding" | "domain" | "backup" | "lifecycle";
  id?: string;
  input: Record<string, unknown>;
};
export type CloudReceipt = {
  id: string;
  generation: number;
  statusUrl: string;
};

export const cloudKinds = {
  "object-storage": "Object storage",
  "backup-store": "Backup storage",
  domain: "Domain",
};
