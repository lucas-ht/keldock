import { api, requireAdmin } from "@/lib/server-api";
import type { GitHubStatus } from "@/lib/github";
import { GitHubConnection } from "@/components/github/connection";
import { GitHubRepositories } from "@/components/github/repositories";
export const metadata = { title: "GitHub delivery" };
export default async function GitHubPage() {
  await requireAdmin();
  const status = await api<GitHubStatus>("/v1/github");
  return (
    <>
      <div className="page-heading">
        <div>
          <h1>GitHub delivery</h1>
          <p>
            Connect repositories and follow deployment feedback on your
            infrastructure.
          </p>
        </div>
      </div>
      <div className="flex flex-col gap-8">
        <GitHubConnection status={status} />
        <GitHubRepositories status={status} />
      </div>
    </>
  );
}
