"use client";
import type { Project } from "@/lib/contracts";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Field,
  FieldDescription,
  FieldLabel,
  FieldLegend,
  FieldSet,
} from "@/components/ui/field";

export function ProjectGrants({
  projects,
  selected,
  onChange,
  admin = false,
  readOnly = false,
}: {
  projects: Project[];
  selected: string[];
  onChange: (value: string[]) => void;
  admin?: boolean;
  readOnly?: boolean;
}) {
  return (
    <FieldSet>
      <FieldLegend>Project access</FieldLegend>
      <FieldDescription>
        {admin
          ? "Administrators can access all configured projects and manage the installation."
          : readOnly
            ? "Select the projects this connection can read. It cannot perform deployment or administration actions."
            : "Select the projects this person can read. Operators can also retry and cancel deployments in these projects."}
      </FieldDescription>
      {projects.map((project) => (
        <Field
          orientation="horizontal"
          key={project.projectId}
          data-disabled={admin}
        >
          <Checkbox
            id={`project-${project.projectId}`}
            checked={admin || selected.includes(project.projectId)}
            disabled={admin}
            onCheckedChange={(checked) =>
              onChange(
                checked
                  ? [...selected, project.projectId]
                  : selected.filter((id) => id !== project.projectId),
              )
            }
          />
          <FieldLabel htmlFor={`project-${project.projectId}`}>
            {project.name}
          </FieldLabel>
        </Field>
      ))}
    </FieldSet>
  );
}
