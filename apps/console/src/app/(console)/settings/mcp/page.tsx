import Link from "next/link";
import { api, requireSession } from "@/lib/server-api";
import type { Connection, MCPClient, MCPSettings, Page } from "@/lib/contracts";
import { EndpointSettings } from "@/components/mcp/settings";
import { Connections } from "@/components/mcp/connections";
import { Clients } from "@/components/mcp/clients";
import { Button } from "@/components/ui/button";
export const metadata = { title: "MCP access" };
export default async function MCPAccess({
  searchParams,
}: {
  searchParams: Promise<{ connectionsAfter?: string; clientsAfter?: string }>;
}) {
  const session = await requireSession();
  const admin = session.user.role === "admin";
  const query = await searchParams;
  const [settings, connections, clients] = await Promise.all([
    api<MCPSettings>("/v1/mcp/settings"),
    api<Page<Connection>>(
      `/v1/mcp/connections?all=${admin}&after=${encodeURIComponent(query.connectionsAfter ?? "")}`,
    ),
    admin
      ? api<Page<MCPClient>>(
          `/v1/mcp/clients?after=${encodeURIComponent(query.clientsAfter ?? "")}`,
        )
      : Promise.resolve(null),
  ]);
  return (
    <>
      <div className="page-heading">
        <div>
          <h1>MCP access</h1>
          <p>
            Give agents bounded read access through the same project permissions
            as the console.
          </p>
        </div>
      </div>
      <div className="flex flex-col gap-8">
        <EndpointSettings settings={settings} admin={admin} />
        <section>
          <div className="section-heading">
            <div>
              <h2>{admin ? "Installation connections" : "Your connections"}</h2>
              <p>
                Every connection is bound to its owner, selected projects, and
                this installation.
              </p>
            </div>
          </div>
          <Connections items={connections.items} />
          {connections.nextCursor && (
            <Button
              variant="outline"
              className="mt-4"
              nativeButton={false}
              render={
                <Link
                  href={`/settings/mcp?connectionsAfter=${encodeURIComponent(connections.nextCursor)}`}
                />
              }
            >
              Next connections
            </Button>
          )}
        </section>
        {clients && (
          <section>
            <Clients items={clients.items} enabled={settings.enabled} />
            {clients.nextCursor && (
              <Button
                variant="outline"
                className="mt-4"
                nativeButton={false}
                render={
                  <Link
                    href={`/settings/mcp?clientsAfter=${encodeURIComponent(clients.nextCursor)}`}
                  />
                }
              >
                Next clients
              </Button>
            )}
          </section>
        )}
        {(query.clientsAfter || query.connectionsAfter) && (
          <Link href="/settings/mcp">Back to first page</Link>
        )}
      </div>
    </>
  );
}
