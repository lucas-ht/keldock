export type User = {
  id: string;
  organizationId: string;
  email: string;
  name: string;
  role: "admin" | "operator" | "viewer";
  status: "invited" | "active" | "disabled";
  projects: string[];
  version: number;
  createdAt: string;
  lastLoginAt?: string;
};
export type Session = {
  user: User;
  csrfToken: string;
  sessionExpiresAt: string;
};
export type Page<T> = { items: T[]; nextCursor: string };
export type Project = {
  projectId: string;
  organizationId: string;
  name: string;
  repositoryId: number;
  clusterId: string;
  environments: number;
  ready: number;
  attention: number;
};
export type Operation = {
  id: string;
  deploymentId: string;
  environmentId: string;
  generation: number;
  kind: string;
  state: string;
  attempt: number;
  error?: string;
  updatedAt: string;
};
export type ComponentHealth = {
  name: string;
  ready: boolean;
  reason: string;
  message?: string;
  image?: string;
  observedGeneration: number;
  appliedRevision?: string;
};
export type Source = {
  repositoryId: number;
  ownerId: number;
  repository: string;
  commit: string;
  headCommit?: string;
  pullRequest?: number;
  runId: number;
  runAttempt: number;
  ref: string;
  event: string;
  workflowRef: string;
};
export type Release = {
  id: string;
  application: string;
  source: Source;
  images: Record<string, string>;
  admittedAt: string;
};
export type Environment = {
  kind: "staging" | "preview" | "production";
  pullRequestHead?: string;
  pullRequestClosed: boolean;
  deletionReason?: string;
  url?: string;
  id: string;
  projectId: string;
  application: string;
  clusterId: string;
  pullRequest: number;
  generation: number;
  deleted: boolean;
  expiresAt?: string;
  operation: Operation;
  releaseId: string;
  hostname: string;
  source?: Source;
  images: Record<string, string>;
  components: {
    name: string;
    kind: string;
    image: string;
    dependsOn: string[] | null;
  }[];
  observation: {
    state: string;
    reason?: string;
    components: ComponentHealth[];
    observedAt: string;
  } | null;
  observationAgeSeconds: number;
  stale: boolean;
};
export type Cluster = {
  id: string;
  name: string;
  revision: number;
  state: string;
  agentId?: string;
  agentVersion?: string;
  expiresAt?: string;
  lastSeenAt?: string;
  revokedAt?: string;
};
export type MCPSettings = {
  enabled: boolean;
  version: number;
  endpoint: string;
};
export type MCPClient = {
  id: string;
  name: string;
  redirectUris: string[];
  createdAt: string;
  disabledAt?: string;
};
export type Connection = {
  id: string;
  scope: string;
  clientId: string;
  clientName: string;
  userId: string;
  userName: string;
  projects: string[];
  resource: string;
  createdAt: string;
  expiresAt: string;
  lastUsedAt?: string;
  revokedAt?: string;
};
export type Authorization = {
  id: string;
  clientId: string;
  clientName: string;
  redirectUri: string;
  resource: string;
  scope: string;
  expiresAt: string;
};
export type Event = {
  id: number;
  actor: string;
  event: string;
  generation: number;
  createdAt: string;
};
export type Diagnostics = {
  environmentId: string;
  generation: number;
  state: string;
  stale: boolean;
  observationAgeSeconds: number;
  publicationError?: string;
  components: ComponentHealth[];
};
