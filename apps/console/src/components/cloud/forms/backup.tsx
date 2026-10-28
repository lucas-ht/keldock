"use client";
import { useId, useState } from "react";
import type { CloudCatalog, CloudIntent } from "@/lib/cloud-contracts";
import { CloudForm } from "../request";
import { Choice } from "@/components/shared/choice";
import { Input } from "@/components/ui/input";
import {
  Field,
  FieldLabel,
  FieldGroup,
  FieldDescription,
} from "@/components/ui/field";

export function BackupForm({
  project,
  catalog,
  resource,
}: {
  project: string;
  catalog: CloudCatalog;
  resource: CloudIntent;
}) {
  const prefix = useId();
  const [database, setDatabase] = useState(catalog.databases[0] ?? ""),
    [schedule, setSchedule] = useState("0 */6 * * *");
  return (
    <CloudForm
      project={project}
      title="Configure database backups"
      trigger="Set up backups"
      submitLabel="Configure archive and schedule"
      disabled={!catalog.databases.length}
      description="Bind this retained store to an operator-approved database. The archive writer and isolated restore reader receive separate identities."
      command={() => ({
        kind: "backup",
        input: { resourceId: resource.id, databaseId: database, schedule },
      })}
    >
      <FieldGroup>
        <Field>
          <FieldLabel htmlFor={`${prefix}-database`}>
            Approved database
          </FieldLabel>
          <Choice
            id={`${prefix}-database`}
            value={database}
            onChange={setDatabase}
            options={catalog.databases.map((value) => ({
              value,
              label: value,
            }))}
          />
        </Field>
        <Field>
          <FieldLabel htmlFor={`${prefix}-schedule`}>
            Backup schedule (UTC)
          </FieldLabel>
          <Input
            id={`${prefix}-schedule`}
            value={schedule}
            onChange={(event) => setSchedule(event.target.value)}
            required
            maxLength={64}
          />
          <FieldDescription>
            Five-field cron expression. The default runs every six hours; WAL
            archiving continues between backups.
          </FieldDescription>
        </Field>
      </FieldGroup>
    </CloudForm>
  );
}
