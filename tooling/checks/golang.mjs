import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";

const files = execFileSync(
  "git",
  ["ls-files", "--cached", "--others", "--exclude-standard", "-z"],
  { encoding: "utf8" },
)
  .split("\0")
  .filter((file) => file.endsWith(".go"));
const unformatted = execFileSync("gofmt", ["-l", ...new Set(files)], {
  encoding: "utf8",
});
if (unformatted) throw new Error(`Run gofmt on:\n${unformatted}`);
const version = readFileSync("go.mod", "utf8").match(/^go (\S+)$/m)?.[1];
const running = execFileSync("go", ["env", "GOVERSION"], {
  encoding: "utf8",
}).trim();
if (running !== `go${version}`)
  throw new Error(`Use Go ${version}; running ${running}`);
if (!readFileSync(".mise.toml", "utf8").includes(`go = "${version}"`))
  throw new Error("Go module and mise pins must agree");
process.stdout.write(`Go ${version} and formatting verified.\n`);
for (const cwd of [
  "examples/postgresql/application",
  "tooling/fixtures/backup-store",
]) {
  for (const args of [
    ["vet", "./..."],
    ["test", "-race", "./..."],
  ])
    execFileSync("go", args, { cwd, stdio: "inherit" });
}
