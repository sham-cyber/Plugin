import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { fileURLToPath } from "node:url";
import { Router, type IRouter } from "express";
import { GetFraudGuardDemoResponse } from "@workspace/api-zod";

const router: IRouter = Router();
const execFileAsync = promisify(execFile);
const fraudGuardModuleDir = fileURLToPath(
  new URL("../../../plugins/fraudguard/", import.meta.url),
);
const cacheMilliseconds = 5_000;

let cachedReport: { report: unknown; expiresAt: number } | undefined;
let inFlightReport: Promise<unknown> | undefined;

async function runFraudGuardDemo(): Promise<unknown> {
  if (cachedReport && cachedReport.expiresAt > Date.now()) {
    return cachedReport.report;
  }
  if (inFlightReport) {
    return inFlightReport;
  }

  inFlightReport = (async () => {
    const { stdout } = await execFileAsync(
      "go",
      ["run", "./cmd/demo", "-json"],
      {
        cwd: fraudGuardModuleDir,
        env: {
          ...process.env,
          GOSUMDB:
            process.env.GOSUMDB && process.env.GOSUMDB !== "off"
              ? process.env.GOSUMDB
              : "sum.golang.org",
        },
        maxBuffer: 256 * 1024,
        timeout: 30_000,
      },
    );
    const report = GetFraudGuardDemoResponse.parse(JSON.parse(stdout));
    cachedReport = { report, expiresAt: Date.now() + cacheMilliseconds };
    return report;
  })();

  try {
    return await inFlightReport;
  } finally {
    inFlightReport = undefined;
  }
}

router.get("/fraudguard/demo", async (req, res): Promise<void> => {
  try {
    const report = await runFraudGuardDemo();
    res.setHeader("Cache-Control", "no-store");
    res.json(report);
  } catch (error) {
    req.log.error({ err: error }, "Could not run the local FraudGuard Go demo");
    res.status(503).json({
      error: "The local Go demo engine is unavailable. Check the API server and Go toolchain.",
    });
  }
});

export default router;
