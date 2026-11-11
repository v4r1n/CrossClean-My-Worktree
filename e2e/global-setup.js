// Builds the real cleanmywt binary once for the whole run.
import { execFileSync } from "node:child_process";
import { binaryPath, repoRoot } from "./support/app.js";

export default function globalSetup() {
  execFileSync("go", ["build", "-o", binaryPath, "./cmd/cleanmywt"], { cwd: repoRoot, stdio: "inherit" });
}
