"use client";
import { useId, useState } from "react";
import type { CloudEnvironment, CloudIntent } from "@/lib/cloud-contracts";
import { CloudForm } from "../request";
import { Choice } from "@/components/shared/choice";
import { Checkbox } from "@/components/ui/checkbox";
import { Switch } from "@/components/ui/switch";
import {
  Field,
  FieldLabel,
  FieldGroup,
  FieldDescription,
  FieldSet,
  FieldLegend,
} from "@/components/ui/field";

export function BindingForm({
  project,
  resource,
  environments,
}: {
  project: string;
  resource: CloudIntent;
  environments: CloudEnvironment[];
}) {
  const prefix = useId();
  const [environmentId, setEnvironment] = useState(environments[0]?.id ?? ""),
    [components, setComponents] = useState<string[]>([]),
    [write, setWrite] = useState(true);
  const environment =
    environments.find((item) => item.id === environmentId) ?? environments[0];
  return (
    <CloudForm
      project={project}
      title="Grant component access"
      trigger="Grant access"
      submitLabel="Grant selected components"
      disabled={!environment}
      submitDisabled={!components.length}
      description="Only selected components receive an AWS identity and their own object prefix. No static AWS credentials enter the application."
      command={() => ({
        kind: "binding",
        input: {
          resourceId: resource.id,
          environmentId: environment?.id,
          components,
          access: write ? "read-write" : "read",
          generation: environment?.generation,
        },
      })}
    >
      <FieldGroup>
        <Field>
          <FieldLabel htmlFor={`${prefix}-environment`}>Environment</FieldLabel>
          <Choice
            id={`${prefix}-environment`}
            value={environment?.id ?? ""}
            onChange={(value) => {
              setEnvironment(value);
              setComponents([]);
            }}
            options={environments.map((item) => ({
              value: item.id,
              label: item.label,
            }))}
          />
        </Field>
        <FieldSet>
          <FieldLegend>Components</FieldLegend>
          <FieldGroup>
            {environment?.components.map((component) => (
              <Field key={component.name} orientation="horizontal">
                <Checkbox
                  id={`${prefix}-${component.name}`}
                  checked={components.includes(component.name)}
                  onCheckedChange={(checked) =>
                    setComponents((values) =>
                      checked
                        ? [...values, component.name]
                        : values.filter((name) => name !== component.name),
                    )
                  }
                />
                <FieldLabel htmlFor={`${prefix}-${component.name}`}>
                  {component.name}
                </FieldLabel>
              </Field>
            ))}
          </FieldGroup>
        </FieldSet>
        <Field orientation="horizontal">
          <FieldLabel htmlFor={`${prefix}-write`}>
            Allow writes within each component’s prefix
          </FieldLabel>
          <Switch
            id={`${prefix}-write`}
            checked={write}
            onCheckedChange={setWrite}
          />
        </Field>
        <FieldDescription>
          This creates a new deployment revision after the consumer identities
          converge. Bucket data is retained when access is removed.
        </FieldDescription>
      </FieldGroup>
    </CloudForm>
  );
}
