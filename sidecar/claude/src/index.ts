/**
 * Symphony Claude Code Sidecar
 *
 * This TypeScript process bridges the Symphony orchestrator (Go) with
 * the Claude Agent SDK. It communicates via JSON-RPC over stdin/stdout.
 *
 * Protocol methods:
 *   initialize          -> capability negotiation
 *   session/new         -> create Claude SDK session
 *   session/prompt      -> send prompt turn, stream updates
 *   session/cancel      -> cancel in-flight turn
 *   session/close       -> close session
 */

import * as readline from "readline";

// Types
interface Request {
  id: number;
  method: string;
  params?: Record<string, unknown>;
}

interface Response {
  id: number;
  result?: Record<string, unknown>;
  error?: { code: number; message: string };
}

interface Notification {
  method: string;
  params: Record<string, unknown>;
}

// State
const sessions = new Map<
  string,
  { cwd: string; title: string; messages: unknown[] }
>();
let nextSessionId = 1;

// Send JSON line to stdout
function send(msg: Response | Notification): void {
  process.stdout.write(JSON.stringify(msg) + "\n");
}

function sendResult(id: number, result: Record<string, unknown>): void {
  send({ id, result });
}

function sendError(id: number, code: number, message: string): void {
  send({ id, error: { code, message } });
}

function sendUpdate(sessionId: string, update: Record<string, unknown>): void {
  send({
    method: "session/update",
    params: { sessionId, update },
  });
}

// Handle incoming requests
async function handleRequest(req: Request): Promise<void> {
  switch (req.method) {
    case "initialize":
      sendResult(req.id, {
        protocolVersion: 1,
        provider: "claude_code",
        adapterInfo: {
          name: "symphony-claude-adapter",
          version: "1.0.0",
        },
        capabilities: {
          sessionReuse: true,
          permissionRequests: false,
          toolRequests: true,
          inputRequests: false,
          mcp: true,
          tokenUsage: true,
          rateLimits: false,
          images: false,
          subagents: false,
        },
      });
      break;

    case "session/new": {
      const sessionId = `sess_${nextSessionId++}`;
      const cwd = (req.params?.cwd as string) || process.cwd();
      const title = (req.params?.title as string) || "Symphony session";

      sessions.set(sessionId, { cwd, title, messages: [] });
      sendResult(req.id, { sessionId });
      break;
    }

    case "session/prompt": {
      const sessionId = req.params?.sessionId as string;
      const session = sessions.get(sessionId);

      if (!session) {
        sendError(req.id, -1, `session not found: ${sessionId}`);
        return;
      }

      const input = req.params?.input as Array<{ type: string; text: string }>;
      const promptText = input?.find((i) => i.type === "text")?.text || "";

      sendUpdate(sessionId, { kind: "progress", message: "Processing..." });

      // TODO: Replace with actual Claude Agent SDK call
      // For now, return a placeholder response
      sendUpdate(sessionId, {
        kind: "assistant_text",
        message: `Received prompt: ${promptText.slice(0, 100)}`,
      });

      sendResult(req.id, {
        stopReason: "completed",
        summary: "Placeholder response — Claude Agent SDK integration pending.",
      });
      break;
    }

    case "session/cancel": {
      sendResult(req.id, { cancelled: true });
      break;
    }

    case "session/close": {
      const sessionId = req.params?.sessionId as string;
      sessions.delete(sessionId);
      sendResult(req.id, { closed: true });
      break;
    }

    default:
      sendError(req.id, -32601, `unknown method: ${req.method}`);
  }
}

// Main: read JSON lines from stdin
const rl = readline.createInterface({ input: process.stdin });

rl.on("line", (line: string) => {
  try {
    const req = JSON.parse(line) as Request;
    handleRequest(req).catch((err) => {
      sendError(req.id, -1, `internal error: ${err}`);
    });
  } catch {
    process.stderr.write(`malformed JSON: ${line}\n`);
  }
});

rl.on("close", () => {
  process.exit(0);
});
