import Link from "next/link";
import type { CloudIntent } from "@/lib/cloud-contracts";
import { date } from "@/lib/format";

export function CloudFacts({
  item,
  project,
}: {
  item: CloudIntent;
  project: string;
}) {
  const disconnected = item.action === "disconnected";
  const details = item.observation?.details ?? {};
  return (
    <dl className="resource-facts">
      <div>
        <dt>Desired revision</dt>
        <dd>{item.generation}</dd>
      </div>
      <div>
        <dt>Last checked</dt>
        <dd>
          {item.observation
            ? date(item.observation.observedAt)
            : "Awaiting observation"}
        </dd>
      </div>
      {item.connection && (
        <>
          <div>
            <dt>Account / region</dt>
            <dd>
              {item.connection.account} / {item.connection.region}
            </dd>
          </div>
          <div>
            <dt>Approved profiles</dt>
            <dd>{item.connection.profiles.join(", ")}</dd>
          </div>
        </>
      )}
      {item.resource && (
        <>
          <div>
            <dt>External bucket</dt>
            <dd>
              <code>{item.resource.externalName}</code>
            </dd>
          </div>
          <div>
            <dt>Management</dt>
            <dd>
              {item.resource.owner === "crossplane"
                ? "Keldock manages selected settings"
                : "Connected; externally managed"}
            </dd>
          </div>
          <div>
            <dt>Retention</dt>
            <dd>Retained on environment and preview deletion</dd>
          </div>
          <div>
            <dt>Connection revision</dt>
            <dd>{item.resource.connectionGeneration}</dd>
          </div>
        </>
      )}
      {item.binding && (
        <>
          <div>
            <dt>Environment</dt>
            <dd>
              <Link
                href={`/projects/${project}/environments/${item.binding.environmentId}`}
              >
                {item.binding.environmentId}
              </Link>
            </dd>
          </div>
          <div>
            <dt>Consumers</dt>
            <dd>
              {item.binding.components.join(", ")} ·{" "}
              {item.binding.access === "read" ? "Read only" : "Read and write"}
            </dd>
          </div>
          {item.binding.consumers.map((consumer) => (
            <div key={consumer.component}>
              <dt>{consumer.component} object prefix</dt>
              <dd>
                <code>{consumer.prefix}</code>
              </dd>
            </div>
          ))}
          <div>
            <dt>Application access</dt>
            <dd>
              {disconnected
                ? "Grant removed"
                : "Verify in the running application after its identity rollout"}
            </dd>
          </div>
        </>
      )}
      {item.domain && (
        <>
          <div>
            <dt>Ownership marker</dt>
            <dd>
              {disconnected
                ? "Released"
                : item.domain.ownershipVerified
                  ? "Verified"
                  : "Pending"}
            </dd>
          </div>
          <div>
            <dt>DNS record</dt>
            <dd>
              {disconnected
                ? "Owned records removed"
                : item.domain.dnsReady
                  ? "Converged"
                  : "Pending"}
            </dd>
          </div>
          <div>
            <dt>Certificate</dt>
            <dd>
              {disconnected
                ? "No longer tracked"
                : item.domain.certificateReady
                  ? "Ready"
                  : "Pending"}
            </dd>
          </div>
        </>
      )}
      {item.backup && (
        <>
          <div>
            <dt>Database</dt>
            <dd>{item.backup.databaseId}</dd>
          </div>
          <div>
            <dt>Schedule (UTC)</dt>
            <dd>
              <code>{item.backup.schedule}</code> ·{" "}
              {disconnected
                ? "Stopped"
                : item.backup.schedulePaused
                  ? "Paused"
                  : "Enabled"}
            </dd>
          </div>
          <div>
            <dt>Archive generation</dt>
            <dd>{item.backup.archiveGeneration}</dd>
          </div>
          <div>
            <dt>Base backup / WAL</dt>
            <dd>
              {disconnected
                ? "Archiving stopped"
                : (details.baseBackup ?? "Pending")}{" "}
              /{" "}
              {disconnected
                ? "Archives retained"
                : details.walHealthy === "true"
                  ? "Healthy"
                  : "Pending"}
            </dd>
          </div>
          {item.backup.restoreGeneration ? (
            <div>
              <dt>Isolated restore</dt>
              <dd>
                {disconnected
                  ? "No longer tracked after disconnect"
                  : details.restoreReady === "true"
                    ? "Database ready; verify application data"
                    : "Pending"}
              </dd>
            </div>
          ) : null}
        </>
      )}
    </dl>
  );
}
