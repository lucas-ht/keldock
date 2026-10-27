import { Clock3 } from "lucide-react";
import type { Environment } from "@/lib/contracts";
import { age, digest, componentState } from "@/lib/format";
import { Status } from "@/components/shared/status";

export function ComponentStrip({ environment }: { environment: Environment }) {
  const observation = environment.observation;
  return (
    <div className="panel">
      <div className="revision-notice">
        <Clock3 className="size-4 shrink-0" />
        <span>
          {observation
            ? environment.stale
              ? "Observation is stale. Current health is unknown."
              : `Observed ${age(environment.observationAgeSeconds)} ago for generation ${environment.generation}.`
            : "Waiting for evidence of this selected revision."}
        </span>
        <span className="ml-auto text-xs text-muted-foreground">
          Generation {environment.generation}
        </span>
      </div>
      <div className="revision-strip">
        {environment.components.map((component) => {
          const health = observation?.components.find(
            (item) => item.name === component.name,
          );
          return (
            <div className="revision-component" key={component.name}>
              <div className="flex items-center justify-between gap-2">
                <h3>{component.name}</h3>
                <Status state={componentState(health, environment.stale)} />
              </div>
              <p>
                {component.kind}
                {health ? ` · ${health.reason}` : " · Awaiting observation"}
              </p>
              <code title={component.image}>
                sha256:{digest(component.image)}
              </code>
              {(component.dependsOn?.length ?? 0) > 0 && (
                <p>After {component.dependsOn!.join(", ")}</p>
              )}
            </div>
          );
        })}
      </div>
    </div>
  );
}
