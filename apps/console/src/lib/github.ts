export type GitHubStatus = {
  app: {
    appId: number;
    clientId: string;
    slug: string;
    enabled: boolean;
    version: number;
    permissions: Record<string, string>;
    syncedAt: string | null;
    syncError: string;
  };
  installations: {
    id: number;
    login: string;
    active: boolean;
    blocked: boolean;
  }[];
  repositories: {
    id: number;
    installationId: number;
    fullName: string;
    projectId: string;
    active: boolean;
  }[];
  pendingEvents: number;
  failedEvents: number;
  pendingFeedback: number;
  failedFeedback: number;
  requiredPermissions: Record<string, string>;
  webhookUrl: string;
  setupUrl: string;
};
export type GitHubSetup = { url: string; manifest?: Record<string, unknown> };
