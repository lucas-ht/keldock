"use client";
import type { CloudIntent } from "@/lib/cloud-contracts";
import { CloudForm, useCloudRequest } from "./request";
import { Button } from "@/components/ui/button";
import { ErrorNotice } from "@/components/shared/feedback";

function Confirm({
  project,
  item,
  action,
  label,
  description,
}: {
  project: string;
  item: CloudIntent;
  action: string;
  label: string;
  description: string;
}) {
  return (
    <CloudForm
      project={project}
      title={label}
      trigger={label}
      submitLabel={label}
      description={description}
      command={() => ({
        kind: "lifecycle",
        id: item.id,
        input: {
          generation: item.generation,
          action,
          ...(action === "adopt"
            ? { reviewHash: item.observation?.details.adoptionReviewHash }
            : {}),
          ...(action === "restore"
            ? { archiveGeneration: item.backup?.archiveGeneration }
            : {}),
        },
      })}
    >
      <dl className="resource-facts">
        <div>
          <dt>Target</dt>
          <dd>
            {item.domain?.request.hostname ??
              item.resource?.externalName ??
              item.id}
          </dd>
        </div>
        <div>
          <dt>Revision</dt>
          <dd>{item.generation}</dd>
        </div>
        {action === "restore" && (
          <div>
            <dt>Restore sandbox</dt>
            <dd>
              {item.backup?.database.restoreNamespace} /{" "}
              {item.backup?.restoreName}
            </dd>
          </div>
        )}
      </dl>
    </CloudForm>
  );
}

export function CloudActions({
  project,
  item,
  admin,
  bound = false,
}: {
  project: string;
  item: CloudIntent;
  admin: boolean;
  bound?: boolean;
}) {
  const { send, busy, error } = useCloudRequest(project);
  const active = item.action === "reconcile";
  const quick = async (action: string) =>
    send({
      kind: "lifecycle",
      id: item.id,
      input: { generation: item.generation, action },
    });
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap gap-2">
        {active && item.kind !== "connection" && item.state === "failed" && (
          <Button
            variant="outline"
            disabled={busy}
            onClick={() => quick("retry")}
          >
            Retry
          </Button>
        )}
        {item.resource && item.state === "suspended" && (
          <Button
            variant="outline"
            disabled={busy}
            onClick={() => quick("refresh")}
          >
            Refresh authority
          </Button>
        )}
        {active &&
          item.resource &&
          ["pending", "reconciling", "failed"].includes(item.state) &&
          !bound && (
            <Button
              variant="outline"
              disabled={busy}
              onClick={() => quick("retain")}
            >
              Stop and retain
            </Button>
          )}
        {active && item.backup && (
          <Button
            variant="outline"
            disabled={busy}
            onClick={() =>
              quick(
                item.backup!.schedulePaused
                  ? "resume-schedule"
                  : "pause-schedule",
              )
            }
          >
            {item.backup.schedulePaused ? "Resume schedule" : "Pause schedule"}
          </Button>
        )}
        {active &&
          item.backup &&
          !item.backup.restoreGeneration &&
          item.state === "ready" && (
            <Confirm
              project={project}
              item={item}
              action="restore"
              label="Restore current archive"
              description="Create a database in the approved isolated sandbox. Its AWS identity can read this archive and cannot overwrite it. Verify recovered application data after it becomes ready."
            />
          )}
        {active && item.backup && (
          <Confirm
            project={project}
            item={item}
            action="next-archive"
            label="Start next archive generation"
            description="Switch future archive writes to a new prefix. Earlier generations remain retained and readable by the restore identity."
          />
        )}
        {admin && active && item.connection && (
          <Confirm
            project={project}
            item={item}
            action="suspend"
            label="Suspend connection"
            description="Stop new cloud operations and suspend dependent grants through the cloud agent. AWS provisioning authority requires separate operator revocation."
          />
        )}
        {admin &&
          active &&
          item.resource?.mode === "connect" &&
          item.observation?.details.adoptionReviewHash && (
            <Confirm
              project={project}
              item={item}
              action="adopt"
              label="Adopt management"
              description="The operator has recorded release of the previous writer. Keldock will manage the bucket’s selected security and versioning settings while preserving its data and unrelated settings."
            />
          )}
        {item.kind !== "connection" &&
          !["disconnect", "disconnected"].includes(item.action) &&
          !bound && (
            <Confirm
              project={project}
              item={item}
              action="disconnect"
              label="Disconnect"
              description={
                item.domain
                  ? "Remove this claim’s owned DNS records and application route."
                  : item.backup
                    ? "Stop archive writes and scheduled backups. Existing archive data is retained."
                    : item.binding
                      ? "Remove this component grant and its application identity. Storage and data remain retained."
                      : "Disconnect management of this resource. The external bucket and its data remain retained."
              }
            />
          )}
      </div>
      {bound && item.resource && (
        <p className="muted text-sm">
          Remove consumer and backup grants before disconnecting this resource.
        </p>
      )}
      <ErrorNotice message={error} />
    </div>
  );
}
