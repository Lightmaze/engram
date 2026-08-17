import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { randomUUID } from "node:crypto";
import { mkdir, readFile } from "node:fs/promises";
import { join } from "node:path";
import test from "node:test";
import engramV0 from "../src/v0.ts";

interface ToolResult {
  details: unknown;
  isError?: boolean;
}

interface RegisteredTool {
  name: string;
  execute(
    toolCallID: string,
    params: Record<string, unknown>,
    signal: AbortSignal,
    onUpdate: () => void,
    context: ToolContext,
  ): Promise<ToolResult>;
}

interface ToolContext {
  cwd: string;
  sessionManager: {
    getSessionId(): string;
    getLeafId(): string;
  };
}

type EventHandler = (event: unknown, context: unknown) => unknown;

const binary = process.env.ENGRAM_V0_BINARY;
const dataParent = process.env.ENGRAM_V0_DATA_ROOT;
const realBinaryTest = binary && dataParent ? test : test.skip;

realBinaryTest("Pi v0 tools drive a real Engram binary across one multi-round accompaniment", { timeout: 30_000 }, async () => {
  assert.ok(binary);
  assert.ok(dataParent);
  const dataRoot = join(dataParent, `pi-v0-${randomUUID()}`);
  await mkdir(dataRoot, { recursive: true });

  await runJSON(binary, [
    "create", "--data", dataRoot, "--driver", "rule", "--allow-rule-driver",
  ], {
    id: "pi-v0-ancestor",
    name: "Pi v0 Ancestor",
    statement: "Protect explicit multi-round accompaniment.",
    messages: [
      { id: "m1", role: "user", content: "An Engram keeps its own original Agent-session context." },
      { id: "m2", role: "assistant", content: "A summon should remain present for several rounds, not collapse into one retrieval." },
    ],
  });

  const tools = new Map<string, RegisteredTool>();
  const fakePi = {
    registerTool(tool: RegisteredTool) {
      tools.set(tool.name, tool);
    },
    on() {
      // Explicit summon mode does not need Hook delivery for this test.
    },
  };
  const previousCommand = process.env.ENGRAM_COMMAND;
  const previousArgs = process.env.ENGRAM_RUNTIME_ARGS_JSON;
  process.env.ENGRAM_COMMAND = binary;
  process.env.ENGRAM_RUNTIME_ARGS_JSON = JSON.stringify([
    "--data", dataRoot, "--driver", "rule", "--allow-rule-driver",
  ]);

  try {
    engramV0(fakePi as never);
    assert.deepEqual([...tools.keys()].sort(), [
      "engram_fold_revert",
      "engram_fold_status",
      "engram_observe",
      "engram_outcome",
      "engram_release",
      "engram_summon",
      "engram_wake",
    ]);
    const context: ToolContext = {
      cwd: dataRoot,
      sessionManager: {
        getSessionId: () => "pi-v0-smoke-session",
        getLeafId: () => "pi-v0-smoke-turn",
      },
    };
    const signal = new AbortController().signal;

    const summoned = await requireTool(tools, "engram_summon").execute("call-1", {
      engram_id: "pi-v0-ancestor",
      reason: "Pi extension smoke test",
      scene: "Verify that this is multi-round accompaniment.",
    }, signal, () => undefined, context);
    assert.equal(summoned.isError, undefined);
    const summonDetails = record(summoned.details);
    const accompanimentID = stringField(record(summonDetails.accompaniment), "accompaniment_id");
    const summonWake = record(summonDetails.wake);
    assert.equal(stringField(summonWake, "state"), "active");
    assert.deepEqual(record(summonWake.attribution), {
      engram_id: "pi-v0-ancestor",
      name: "Pi v0 Ancestor",
      statement: "Protect explicit multi-round accompaniment.",
      accompaniment_id: accompanimentID,
    });

    const observed = await requireTool(tools, "engram_observe").execute("call-2", {
      accompaniment_id: accompanimentID,
      role: "assistant",
      content: "Pi received the first steering.",
    }, signal, () => undefined, context);
    assert.equal(stringField(record(observed.details), "accompaniment_id"), accompanimentID);

    const awakened = await requireTool(tools, "engram_wake").execute("call-3", {
      accompaniment_id: accompanimentID,
      scene: "Continue with the same accompanying Engram.",
    }, signal, () => undefined, context);
    const wakeDetails = record(awakened.details);
    assert.equal(stringField(record(wakeDetails.accompaniment), "accompaniment_id"), accompanimentID);
    assert.equal(stringField(wakeDetails, "state"), "active");
    assert.deepEqual(record(wakeDetails.attribution), {
      engram_id: "pi-v0-ancestor",
      name: "Pi v0 Ancestor",
      statement: "Protect explicit multi-round accompaniment.",
      accompaniment_id: accompanimentID,
    });

    const outcomeSource = await requireTool(tools, "engram_observe").execute("call-4", {
      accompaniment_id: accompanimentID,
      role: "toolResult",
      content: "The real Pi binary completed the second bounded wake.",
    }, signal, () => undefined, context);
    assert.equal(outcomeSource.isError, undefined);
    const sourceEventID = stringField(record(outcomeSource.details), "observation_event_id");
    const outcome = await requireTool(tools, "engram_outcome").execute("call-5", {
      accompaniment_id: accompanimentID,
      wake_event_id: stringField(wakeDetails, "wake_event_id"),
      source_kind: "tool_result",
      source_event_id: sourceEventID,
      request_id: "pi-v0-real-self-fold",
    }, signal, () => undefined, context);
    assert.equal(outcome.isError, undefined);
    const outcomeDetails = record(outcome.details);
    const foldID = stringField(outcomeDetails, "active_fold_event_id");
    const foldEvent = record(outcomeDetails.self_fold_event);
    assert.equal(stringField(foldEvent, "event_id"), foldID);
    const foldRecord = record(foldEvent.self_fold);
    assert.equal(foldRecord.actor, "engram:pi-v0-ancestor");
    assert.equal(foldRecord.authority, "posture/hypothesis");
    assert.equal(foldRecord.user_ratified, false);

    const status = await requireTool(tools, "engram_fold_status").execute("call-6", {
      engram_id: "pi-v0-ancestor",
    }, signal, () => undefined, context);
    assert.equal(stringField(record(status.details), "active_fold_event_id"), foldID);

    const released = await requireTool(tools, "engram_release").execute("call-7", {
      accompaniment_id: accompanimentID,
      reason: "Pi v0 smoke complete",
    }, signal, () => undefined, context);
    assert.equal(stringField(record(released.details), "status"), "sleeping");

    const resummoned = await requireTool(tools, "engram_summon").execute("call-8", {
      engram_id: "pi-v0-ancestor",
      reason: "verify the applied self-fold on a later accompaniment",
      scene: "This is a fresh provider request with the same complete Engram history.",
    }, signal, () => undefined, context);
    assert.equal(resummoned.isError, undefined);
    const resummonDetails = record(resummoned.details);
    const laterWake = record(resummonDetails.wake);
    assert.equal(stringField(laterWake, "active_fold_event_id"), foldID);
    const laterAccompanimentID = stringField(record(resummonDetails.accompaniment), "accompaniment_id");

    const reverted = await requireTool(tools, "engram_fold_revert").execute("call-9", {
      engram_id: "pi-v0-ancestor",
      fold_event_id: foldID,
      reason: "real-binary test restores the original posture after proving replay",
      request_id: "pi-v0-real-self-fold-revert",
    }, signal, () => undefined, context);
    assert.equal(reverted.isError, undefined);
    assert.equal(record(reverted.details).active_fold_event_id, undefined);
    await requireTool(tools, "engram_release").execute("call-10", {
      accompaniment_id: laterAccompanimentID,
      reason: "Pi self-fold smoke complete",
    }, signal, () => undefined, context);
  } finally {
    restoreEnvironment("ENGRAM_COMMAND", previousCommand);
    restoreEnvironment("ENGRAM_RUNTIME_ARGS_JSON", previousArgs);
  }
});

realBinaryTest("Pi guardian persists an initial branch slice, a cursor delta, and failed-observation replay", { timeout: 30_000 }, async () => {
  assert.ok(binary);
  assert.ok(dataParent);
  const dataRoot = join(dataParent, `pi-guardian-${randomUUID()}`);
  await mkdir(dataRoot, { recursive: true });

  await runJSON(binary, [
    "create", "--data", dataRoot, "--driver", "rule", "--allow-rule-driver",
  ], {
    id: "pi-guardian-ancestor",
    name: "Pi Guardian Ancestor",
    statement: "Keep the observed host history distinct from my own history.",
    messages: [
      { role: "user", content: "A guardian must see more than the latest prompt." },
      { role: "assistant", content: "Preserve ordered host messages and tool results." },
    ],
  });

  const tools = new Map<string, RegisteredTool>();
  const handlers = new Map<string, EventHandler>();
  const branch: Array<Record<string, unknown>> = [
    { type: "message", id: "old-user", message: { role: "user", content: "Inspect the command." } },
    {
      type: "message",
      id: "old-assistant",
      message: {
        role: "assistant",
        content: [
          { type: "thinking", thinking: "must stay private" },
          { type: "text", text: "I will run the command." },
          { type: "toolCall", id: "call-old", name: "bash", arguments: { command: "demo --help" } },
        ],
      },
    },
    {
      type: "message",
      id: "old-tool",
      message: { role: "toolResult", toolName: "bash", toolCallId: "call-old", isError: false, content: "demo usage" },
    },
  ];
  let leafID = "old-tool";
  let cursorSequence = 0;
  const fakePi = {
    registerTool(tool: RegisteredTool) {
      tools.set(tool.name, tool);
    },
    on(event: string, handler: EventHandler) {
      handlers.set(event, handler);
    },
    appendEntry(customType: string, data: unknown) {
      cursorSequence += 1;
      const entry = { type: "custom", id: `cursor-${cursorSequence}`, customType, data };
      branch.push(entry);
      leafID = entry.id;
    },
  };
  const warnings: string[] = [];
  const signal = new AbortController().signal;
  const context = {
    cwd: dataRoot,
    signal,
    sessionManager: {
      getSessionId: () => "pi-guardian-session",
      getLeafId: () => leafID,
      getBranch: () => branch,
    },
    ui: {
      notify: (message: string) => warnings.push(message),
    },
  };

  const previousCommand = process.env.ENGRAM_COMMAND;
  const previousArgs = process.env.ENGRAM_RUNTIME_ARGS_JSON;
  const previousGuardian = process.env.ENGRAM_ID;
  process.env.ENGRAM_COMMAND = binary;
  process.env.ENGRAM_RUNTIME_ARGS_JSON = JSON.stringify([
    "--data", dataRoot, "--driver", "rule", "--allow-rule-driver",
  ]);
  process.env.ENGRAM_ID = "pi-guardian-ancestor";

  try {
    engramV0(fakePi as never);
    const before = requireHandler(handlers, "before_agent_start");
    const messageEnd = requireHandler(handlers, "message_end");
    const settled = requireHandler(handlers, "agent_settled");
    const shutdown = requireHandler(handlers, "session_shutdown");

    const currentImage = { type: "image", mimeType: "image/png", data: "current-image-binary" };
    const first = record(await before({ prompt: "What did the tool prove?", images: [currentImage] }, context));
    const firstMessage = record(first.message);
    assert.equal(firstMessage.customType, "engram-accompaniment");
    const firstDetails = record(firstMessage.details);
    const firstAccompaniment = stringField(record(firstDetails.attribution), "accompaniment_id");
    await messageEnd({
      message: { role: "user", content: [{ type: "text", text: "What did the tool prove?" }, currentImage] },
    }, context);
    branch.push({
      type: "message",
      id: "current-user",
      message: { role: "user", content: [{ type: "text", text: "What did the tool prove?" }, currentImage] },
    });
    branch.push({ type: "custom_message", id: "engram-one", ...firstMessage });
    await messageEnd({ message: { role: "user", content: [{ type: "text", text: "Queued correction inside this run." }] } }, context);
    branch.push({
      type: "message",
      id: "queued-user",
      message: { role: "user", content: [{ type: "text", text: "Queued correction inside this run." }] },
    });
    branch.push({ type: "message", id: "answer-one", message: { role: "assistant", content: [{ type: "text", text: "The command proved the seam." }] } });
    leafID = "answer-one";
    await messageEnd({ message: { role: "assistant", content: [{ type: "text", text: "The command proved the seam." }] } }, context);
    await settled({}, context);
    assert.equal(branch.filter((entry) => entry.customType === "engram-thread-observed").length, 1);

    const second = record(await before({ prompt: "Now verify only the delta." }, context));
    const secondMessage = record(second.message);
    const secondDetails = record(secondMessage.details);
    assert.equal(stringField(record(secondDetails.attribution), "accompaniment_id"), firstAccompaniment);

    const journalRaw = await readFile(join(dataRoot, "engrams", "pi-guardian-ancestor", "journal.jsonl"), "utf8");
    const events = journalRaw.trim().split("\n").map((line) => JSON.parse(line) as Record<string, unknown>);
    const wakes = events.filter((event) => event.kind === "wake_result");
    assert.equal(wakes.length, 2);
    const firstScene = stringField(wakes[0]!, "scene");
    const secondScene = stringField(wakes[1]!, "scene");
    const firstMetadata = record(JSON.parse(firstScene.split("\n", 1)[0]!) as unknown);
    const secondMetadata = record(JSON.parse(secondScene.split("\n", 1)[0]!) as unknown);
    assert.equal(firstMetadata.mode, "initial_snapshot");
    assert.equal(firstMetadata.selected_prior_messages, 3);
    assert.match(firstScene, /demo usage/);
    assert.match(firstScene, /image payload omitted; mime_type=image\/png/);
    assert.doesNotMatch(firstScene, /must stay private/);
    assert.doesNotMatch(firstScene, /current-image-binary/);
    assert.equal(secondMetadata.mode, "delta");
    assert.equal(secondMetadata.selected_prior_messages, 0);
    assert.doesNotMatch(secondScene, /demo usage|The command proved the seam/);
    assert.match(secondScene, /Now verify only the delta/);
    const observedUsers = events.filter((event) => event.kind === "observation" && event.role === "user");
    assert.equal(observedUsers.length, 1);
    assert.equal(observedUsers[0]?.content, "Queued correction inside this run.");

    const release = await requireTool(tools, "engram_release").execute("release-early", {
      accompaniment_id: firstAccompaniment,
      reason: "force the following observation to fail",
    }, signal, () => undefined, context);
    assert.equal(release.isError, undefined);
    await messageEnd({ message: { role: "user", content: [{ type: "text", text: "Now verify only the delta." }] } }, context);
    branch.push({
      type: "message",
      id: "current-user-two",
      message: { role: "user", content: [{ type: "text", text: "Now verify only the delta." }] },
    });
    branch.push({ type: "custom_message", id: "engram-two", ...secondMessage });
    await messageEnd({ message: { role: "toolResult", toolName: "bash", toolCallId: "late", content: "not durable" } }, context);
    branch.push({
      type: "message",
      id: "failed-tool-observation",
      message: { role: "toolResult", toolName: "bash", toolCallId: "late", content: "not durable" },
    });
    leafID = "failed-tool-observation";
    await settled({}, context);
    assert.equal(branch.filter((entry) => entry.customType === "engram-thread-observed").length, 1);
    assert.deepEqual(warnings, []);
    await shutdown({}, context);

    // A new Pi process loads the same append-only branch and Engram Journal.
    // Because the failed observation never advanced the cursor, this wake must
    // open a new bounded accompaniment and replay the unacknowledged branch.
    engramV0(fakePi as never);
    const reopenedBefore = requireHandler(handlers, "before_agent_start");
    const reopenedMessageEnd = requireHandler(handlers, "message_end");
    const reopenedSettled = requireHandler(handlers, "agent_settled");
    const reopenedShutdown = requireHandler(handlers, "session_shutdown");
    const third = record(await reopenedBefore({ prompt: "Continue after reopening Pi." }, context));
    const thirdMessage = record(third.message);
    const thirdAccompaniment = stringField(record(record(thirdMessage.details).attribution), "accompaniment_id");
    assert.notEqual(thirdAccompaniment, firstAccompaniment);

    const reopenedJournalRaw = await readFile(join(dataRoot, "engrams", "pi-guardian-ancestor", "journal.jsonl"), "utf8");
    const reopenedEvents = reopenedJournalRaw.trim().split("\n").map((line) => JSON.parse(line) as Record<string, unknown>);
    const reopenedWakes = reopenedEvents.filter((event) => event.kind === "wake_result");
    assert.equal(reopenedWakes.length, 3);
    const thirdScene = stringField(reopenedWakes[2]!, "scene");
    const thirdMetadata = record(JSON.parse(thirdScene.split("\n", 1)[0]!) as unknown);
    assert.equal(thirdMetadata.mode, "delta");
    assert.equal(thirdMetadata.selected_prior_messages, 2);
    assert.match(thirdScene, /Now verify only the delta/);
    assert.match(thirdScene, /not durable/);
    assert.match(thirdScene, /Continue after reopening Pi/);

    await reopenedMessageEnd({ message: { role: "user", content: [{ type: "text", text: "Continue after reopening Pi." }] } }, context);
    branch.push({
      type: "message",
      id: "current-user-three",
      message: { role: "user", content: [{ type: "text", text: "Continue after reopening Pi." }] },
    });
    branch.push({ type: "custom_message", id: "engram-three", ...thirdMessage });
    await reopenedMessageEnd({ message: { role: "assistant", content: [{ type: "text", text: "Replayed and continued." }] } }, context);
    branch.push({
      type: "message",
      id: "answer-three",
      message: { role: "assistant", content: [{ type: "text", text: "Replayed and continued." }] },
    });
    leafID = "answer-three";
    await reopenedSettled({}, context);
    assert.equal(branch.filter((entry) => entry.customType === "engram-thread-observed").length, 2);
    await reopenedShutdown({}, context);
  } finally {
    restoreEnvironment("ENGRAM_COMMAND", previousCommand);
    restoreEnvironment("ENGRAM_RUNTIME_ARGS_JSON", previousArgs);
    restoreEnvironment("ENGRAM_ID", previousGuardian);
  }
});

function requireTool(tools: Map<string, RegisteredTool>, name: string): RegisteredTool {
  const tool = tools.get(name);
  assert.ok(tool, `Pi did not register ${name}`);
  return tool;
}

function requireHandler(handlers: Map<string, EventHandler>, name: string): EventHandler {
  const handler = handlers.get(name);
  assert.ok(handler, `Pi did not register ${name}`);
  return handler;
}

function runJSON(command: string, args: string[], input: unknown): Promise<unknown> {
  return new Promise((resolve, reject) => {
    const child = spawn(command, args, { shell: false, windowsHide: true, stdio: "pipe" });
    const stdout: Buffer[] = [];
    const stderr: Buffer[] = [];
    child.stdout.on("data", (chunk: Buffer) => stdout.push(chunk));
    child.stderr.on("data", (chunk: Buffer) => stderr.push(chunk));
    child.once("error", reject);
    child.once("close", (code) => {
      if (code !== 0) {
        reject(new Error(Buffer.concat(stderr).toString("utf8").trim() || `command exited with ${String(code)}`));
        return;
      }
      try {
        resolve(JSON.parse(Buffer.concat(stdout).toString("utf8")) as unknown);
      } catch (error) {
        reject(error);
      }
    });
    child.stdin.end(JSON.stringify(input));
  });
}

function record(value: unknown): Record<string, unknown> {
  assert.ok(typeof value === "object" && value !== null && !Array.isArray(value));
  return value as Record<string, unknown>;
}

function stringField(value: Record<string, unknown>, name: string): string {
  const field = value[name];
  if (typeof field !== "string") throw new TypeError(`${name} must be a string`);
  return field;
}

function restoreEnvironment(name: string, value: string | undefined): void {
  if (value === undefined) delete process.env[name];
  else process.env[name] = value;
}
