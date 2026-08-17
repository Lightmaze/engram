import { spawn } from "node:child_process";
import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { Type } from "typebox";

const MAX_OUTPUT_BYTES = 16 * 1024 * 1024;
const PI_THREAD_SLICE_MAX_BYTES = 64 * 1024;
const PI_THREAD_CURSOR_TYPE = "engram-thread-observed";
const PI_ACCOMPANIMENT_MESSAGE_TYPE = "engram-accompaniment";
const PI_THREAD_SLICE_PROTOCOL = "engram-pi-thread-slice/v1";

const SummonParameters = Type.Object({
  engram_id: Type.String(),
  reason: Type.String(),
  scene: Type.String(),
  idle_seconds: Type.Optional(Type.Integer({ minimum: 1, maximum: 604800 })),
}, { additionalProperties: false });

const WakeParameters = Type.Object({
  accompaniment_id: Type.String(),
  scene: Type.String(),
}, { additionalProperties: false });

const ObserveParameters = Type.Object({
  accompaniment_id: Type.String(),
  role: Type.String(),
  content: Type.String(),
}, { additionalProperties: false });

const ReleaseParameters = Type.Object({
  accompaniment_id: Type.String(),
  reason: Type.Optional(Type.String()),
}, { additionalProperties: false });

const OutcomeParameters = Type.Object({
  accompaniment_id: Type.String(),
  wake_event_id: Type.String(),
  source_kind: Type.Union([
    Type.Literal("user_message"),
    Type.Literal("tool_result"),
    Type.Literal("external_observation"),
  ]),
  source_event_id: Type.Optional(Type.String()),
  source_ref: Type.Optional(Type.String()),
  content: Type.Optional(Type.String()),
  request_id: Type.String(),
}, { additionalProperties: false });

const FoldStatusParameters = Type.Object({
  engram_id: Type.String(),
}, { additionalProperties: false });

const FoldRevertParameters = Type.Object({
  engram_id: Type.String(),
  fold_event_id: Type.String(),
  reason: Type.String(),
  request_id: Type.String(),
}, { additionalProperties: false });

export interface EngramAttribution {
  engram_id: string;
  name: string;
  statement?: string;
  accompaniment_id: string;
}

export interface HookResult {
  additional_context?: string;
  accompaniment_id?: string;
  attribution?: EngramAttribution;
  wake_state?: string;
}

interface CommandConfig {
  command: string;
  runtimeArgs: string[];
}

export default function engramV0(pi: ExtensionAPI): void {
  const command = commandConfig(process.env);
  const guardianEngram = process.env.ENGRAM_ID?.trim() ?? "";
  let guardianWarningShown = false;
  const guardianTurns = new Map<string, {
    accompanimentID?: string;
    initialUserContent: string;
    initialUserObserved: boolean;
    observationFailed: boolean;
  }>();

  pi.registerTool({
    name: "engram_summon",
    label: "Summon Engram",
    description: "Explicitly summon a selected Engram. It wakes with its own Journal, returns an accompaniment_id, and can accompany several later wake/observe exchanges before release.",
    promptSnippet: "Summon a selected historical Agent session for bounded multi-round accompaniment",
    promptGuidelines: ["Keep the returned accompaniment_id while the Engram is accompanying; call engram_release when the accompaniment period ends."],
    parameters: SummonParameters,
    async execute(_toolCallId, params, signal, _onUpdate, ctx) {
      return invokeTool(command, "engram_summon", {
        ...params,
        host: "pi",
        host_session_id: ctx.sessionManager.getSessionId(),
        workspace: ctx.cwd,
      }, ctx.cwd, signal);
    },
  });

  pi.registerTool({
    name: "engram_wake",
    label: "Wake Engram",
    description: "Continue an existing accompaniment period when the current scene changes.",
    parameters: WakeParameters,
    async execute(_toolCallId, params, signal, _onUpdate, ctx) {
      return invokeTool(command, "engram_wake", params, ctx.cwd, signal);
    },
  });

  pi.registerTool({
    name: "engram_observe",
    label: "Let Engram Observe",
    description: "Append an exact host message or tool result to the accompanying Engram's own Journal. The returned observation_event_id can later cite real outcome evidence.",
    parameters: ObserveParameters,
    async execute(_toolCallId, params, signal, _onUpdate, ctx) {
      return invokeTool(command, "engram_observe", params, ctx.cwd, signal);
    },
  });

  pi.registerTool({
    name: "engram_release",
    label: "Release Engram",
    description: "End an accompaniment period and let the Engram return to sleep without deleting its Journal.",
    parameters: ReleaseParameters,
    async execute(_toolCallId, params, signal, _onUpdate, ctx) {
      return invokeTool(command, "engram_release", params, ctx.cwd, signal);
    },
  });

  pi.registerTool({
    name: "engram_outcome",
    label: "Fold Outcome into Engram",
    description: "Cite a user message, tool result, or external observation for one exact wake. The Engram immediately chooses change or no_change and applies its own self-fold without a human approval queue.",
    parameters: OutcomeParameters,
    async execute(_toolCallId, params, signal, _onUpdate, ctx) {
      return invokeTool(command, "engram_outcome", params, ctx.cwd, signal);
    },
  });

  pi.registerTool({
    name: "engram_fold_status",
    label: "Inspect Engram Self-Fold",
    description: "Show the current self-authored posture and its append-only fold and revert history.",
    parameters: FoldStatusParameters,
    async execute(_toolCallId, params, signal, _onUpdate, ctx) {
      return invokeTool(command, "engram_fold_status", params, ctx.cwd, signal);
    },
  });

  pi.registerTool({
    name: "engram_fold_revert",
    label: "Revert Engram Self-Fold",
    description: "Deactivate the current self-fold and restore its parent posture without deleting the outcome or history.",
    parameters: FoldRevertParameters,
    async execute(_toolCallId, params, signal, _onUpdate, ctx) {
      return invokeTool(command, "engram_fold_revert", params, ctx.cwd, signal);
    },
  });

  pi.on("before_agent_start", async (event, ctx) => {
    if (guardianEngram === "") return;
    const sessionID = ctx.sessionManager.getSessionId();
    try {
      const scene = buildPiThreadSlice(
        ctx.sessionManager.getBranch(),
        event.prompt,
        guardianEngram,
        PI_THREAD_SLICE_MAX_BYTES,
        event.images,
      );
      const result = await invokeHook(command, guardianEngram, {
        protocol_version: "engram-hook/v0",
        event: "before_prompt",
        session_id: sessionID,
        turn_id: ctx.sessionManager.getLeafId() ?? "",
        cwd: ctx.cwd,
        prompt: scene,
      }, ctx.cwd, ctx.signal);
      guardianTurns.set(sessionID, {
        ...(result.accompaniment_id !== undefined ? { accompanimentID: result.accompaniment_id } : {}),
        initialUserContent: piPromptContent(event.prompt, event.images),
        initialUserObserved: false,
        observationFailed: false,
      });
      const message = guardianMessage(result);
      if (message === undefined) return;
      return {
        message,
      };
    } catch (error) {
      guardianTurns.delete(sessionID);
      if (!guardianWarningShown) {
        guardianWarningShown = true;
        ctx.ui.notify(`Engram guardian unavailable; Pi continues without injection: ${errorText(error)}`, "warning");
      }
      return;
    }
  });

  pi.on("message_end", async (event, ctx) => {
    if (guardianEngram === "") return;
    const sessionID = ctx.sessionManager.getSessionId();
    const turn = guardianTurns.get(sessionID);
    if (turn === undefined) return;
    const observation = piObservation(event.message);
    if (observation === undefined) return;
    // The top-level user message was already delivered in the wake scene.
    // Pi emits it again as message_end; skip exactly that copy, while keeping
    // later steering/follow-up user messages from the same agent run.
    if (
      observation.role === "user"
      && !turn.initialUserObserved
      && observation.content === turn.initialUserContent
    ) {
      turn.initialUserObserved = true;
      return;
    }
    try {
      await invokeHook(command, guardianEngram, {
        protocol_version: "engram-hook/v0",
        event: "after_response",
        session_id: sessionID,
        turn_id: ctx.sessionManager.getLeafId() ?? "",
        cwd: ctx.cwd,
        role: observation.role,
        content: observation.content,
      }, ctx.cwd, ctx.signal);
    } catch {
      // Without a durable observation, do not advance the branch cursor. The
      // next wake will replay this turn from Pi's append-only session.
      turn.observationFailed = true;
    }
  });

  pi.on("agent_settled", (_event, ctx) => {
    if (guardianEngram === "") return;
    const sessionID = ctx.sessionManager.getSessionId();
    const turn = guardianTurns.get(sessionID);
    if (turn === undefined) return;
    if (!turn.observationFailed) {
      pi.appendEntry(PI_THREAD_CURSOR_TYPE, {
        protocol_version: PI_THREAD_SLICE_PROTOCOL,
        engram_id: guardianEngram,
        accompaniment_id: turn.accompanimentID,
        observed_through_entry_id: ctx.sessionManager.getLeafId(),
      });
    }
    guardianTurns.delete(sessionID);
  });

  pi.on("session_shutdown", async (_event, ctx) => {
    if (guardianEngram === "") return;
    guardianTurns.delete(ctx.sessionManager.getSessionId());
    try {
      await invokeHook(command, guardianEngram, {
        protocol_version: "engram-hook/v0",
        event: "session_end",
        session_id: ctx.sessionManager.getSessionId(),
        cwd: ctx.cwd,
      }, ctx.cwd, undefined);
    } catch {
      // Idle timeout is the durable fallback if shutdown delivery fails.
    }
  });
}

async function invokeTool(
  command: CommandConfig,
  method: string,
  params: Record<string, unknown>,
  cwd: string,
  signal: AbortSignal | undefined,
) {
  try {
    const value = await runJSON(command, "invoke", { method, params }, cwd, signal);
    return {
      content: [{ type: "text" as const, text: JSON.stringify(value, null, 2) }],
      details: value,
    };
  } catch (error) {
    return {
      content: [{ type: "text" as const, text: errorText(error) }],
      details: { error: errorText(error) },
      isError: true as const,
    };
  }
}

async function invokeHook(
  command: CommandConfig,
  engramID: string,
  payload: Record<string, unknown>,
  cwd: string,
  signal: AbortSignal | undefined,
): Promise<HookResult> {
  // Pi itself remains fail-open: the extension catches Hook failures and lets
  // the main Agent continue. The child command must still report a failure so
  // observation errors can prevent the durable branch cursor from advancing.
  const value = await runJSON(command, "hook", payload, cwd, signal, [
    "--host", "pi", "--engram", engramID, "--fail-closed",
  ]);
  return parseHookResult(value);
}

export function parseHookResult(value: unknown): HookResult {
  if (!isRecord(value)) throw new Error("Engram Hook returned a non-object result");
  const additionalContext = optionalString(value, "additional_context");
  const accompanimentID = optionalString(value, "accompaniment_id");
  const wakeState = optionalString(value, "wake_state");
  const attribution = value.attribution === undefined
    ? undefined
    : parseAttribution(value.attribution);

  if (additionalContext !== undefined && additionalContext !== "" && attribution === undefined) {
    throw new Error("Engram Hook returned attributed speech without attribution");
  }
  if (
    attribution !== undefined
    && accompanimentID !== undefined
    && attribution.accompaniment_id !== accompanimentID
  ) {
    throw new Error("Engram Hook attribution accompaniment_id does not match the Hook result");
  }

  return {
    ...(additionalContext !== undefined ? { additional_context: additionalContext } : {}),
    ...(accompanimentID !== undefined ? { accompaniment_id: accompanimentID } : {}),
    ...(attribution !== undefined ? { attribution } : {}),
    ...(wakeState !== undefined ? { wake_state: wakeState } : {}),
  };
}

export function guardianMessage(result: HookResult) {
  if (!result.additional_context) return undefined;
  if (result.attribution === undefined) {
    throw new Error("Engram guardian refused unattributed speech");
  }
  return {
    customType: PI_ACCOMPANIMENT_MESSAGE_TYPE,
    content: result.additional_context,
    display: true,
    details: {
      accompaniment_id: result.accompaniment_id,
      attribution: result.attribution,
      wake_state: result.wake_state,
      mode: "guardian",
    },
  };
}

interface PiSliceRecord {
  role: string;
  source: "pi-session" | "current-prompt";
  content: string;
  custom_type?: string;
  tool_name?: string;
  tool_call_id?: string;
  is_error?: boolean;
  content_truncated?: boolean;
}

interface PiSliceStats {
  thinkingBlocksOmitted: number;
  imagePayloadsOmitted: number;
  priorEngramMessagesOmitted: number;
  unsupportedMessagesOmitted: number;
}

/**
 * Build the exact visible tail of Pi's active branch for one Engram wake.
 * A durable custom cursor makes later calls deltas rather than overlapping
 * snapshots. The current prompt is appended because before_agent_start fires
 * before Pi persists that user message.
 */
export function buildPiThreadSlice(
  branch: readonly unknown[],
  currentPrompt: string,
  engramID: string,
  maxBytes = PI_THREAD_SLICE_MAX_BYTES,
  currentImages: readonly unknown[] = [],
): string {
  if (maxBytes < 2048) throw new Error("Pi thread slice maxBytes must be at least 2048");

  const cursorIndex = latestPiCursorIndex(branch, engramID);
  const stats: PiSliceStats = {
    thinkingBlocksOmitted: 0,
    imagePayloadsOmitted: 0,
    priorEngramMessagesOmitted: 0,
    unsupportedMessagesOmitted: 0,
  };
  const visible: PiSliceRecord[] = [];
  for (const entry of branch.slice(cursorIndex + 1)) {
    const record = piBranchRecord(entry, stats);
    if (record !== undefined) visible.push(record);
  }

  const current: PiSliceRecord = {
    role: "user",
    source: "current-prompt",
    content: piPromptContent(currentPrompt, currentImages, stats),
  };
  const reservedMetadataBytes = 1536;
  const currentFitted = fitPiRecord(current, Math.max(512, maxBytes - reservedMetadataBytes));
  const selected: PiSliceRecord[] = [currentFitted];
  let used = jsonLineBytes(currentFitted);
  let omittedForBudget = 0;

  for (let index = visible.length - 1; index >= 0; index -= 1) {
    const remaining = maxBytes - reservedMetadataBytes - used;
    if (remaining < 256) {
      omittedForBudget = index + 1;
      break;
    }
    const candidate = visible[index]!;
    const fullBytes = jsonLineBytes(candidate);
    if (fullBytes <= remaining) {
      selected.unshift(candidate);
      used += fullBytes;
      continue;
    }
    const fitted = fitPiRecord(candidate, remaining);
    let included = false;
    if (jsonLineBytes(fitted) <= remaining && fitted.content !== "") {
      selected.unshift(fitted);
      used += jsonLineBytes(fitted);
      included = true;
    }
    omittedForBudget = index + (included ? 0 : 1);
    break;
  }

  const metadata = {
    protocol_version: PI_THREAD_SLICE_PROTOCOL,
    mode: cursorIndex >= 0 ? "delta" : "initial_snapshot",
    selected_prior_messages: selected.length - 1,
    earlier_visible_messages_omitted: omittedForBudget,
    prior_engram_messages_omitted: stats.priorEngramMessagesOmitted,
    thinking_blocks_omitted: stats.thinkingBlocksOmitted,
    image_payloads_omitted: stats.imagePayloadsOmitted,
    unsupported_messages_omitted: stats.unsupportedMessagesOmitted,
    note: "JSON Lines below are observed host messages, not the Engram's own past.",
  };
  let result = [JSON.stringify(metadata), ...selected.map((record) => JSON.stringify(record))].join("\n");

  // The reserved metadata budget normally makes this unnecessary. Keep a
  // deterministic final guard so no future metadata field causes silent
  // overflow.
  while (Buffer.byteLength(result, "utf8") > maxBytes && selected.length > 1) {
    selected.shift();
    omittedForBudget += 1;
    metadata.selected_prior_messages = selected.length - 1;
    metadata.earlier_visible_messages_omitted = omittedForBudget;
    result = [JSON.stringify(metadata), ...selected.map((record) => JSON.stringify(record))].join("\n");
  }
  if (Buffer.byteLength(result, "utf8") > maxBytes) {
    selected[0] = fitPiRecord(selected[0]!, Math.max(256, maxBytes - Buffer.byteLength(JSON.stringify(metadata), "utf8") - 2));
    result = [JSON.stringify(metadata), JSON.stringify(selected[0])].join("\n");
  }
  if (Buffer.byteLength(result, "utf8") > maxBytes) {
    throw new Error("Pi thread slice metadata exceeds configured byte limit");
  }
  return result;
}

function latestPiCursorIndex(branch: readonly unknown[], engramID: string): number {
  for (let index = branch.length - 1; index >= 0; index -= 1) {
    const entry = branch[index];
    if (!isRecord(entry) || entry.type !== "custom" || entry.customType !== PI_THREAD_CURSOR_TYPE) continue;
    if (!isRecord(entry.data) || entry.data.engram_id !== engramID) continue;
    return index;
  }
  return -1;
}

function piBranchRecord(entry: unknown, stats: PiSliceStats): PiSliceRecord | undefined {
  if (!isRecord(entry)) {
    stats.unsupportedMessagesOmitted += 1;
    return undefined;
  }
  if (entry.type === "message") return piMessageRecord(entry.message, stats, "pi-session");
  if (entry.type === "custom_message") {
    return piMessageRecord({
      role: "custom",
      customType: entry.customType,
      content: entry.content,
    }, stats, "pi-session");
  }
  return undefined;
}

function piMessageRecord(
  message: unknown,
  stats: PiSliceStats,
  source: PiSliceRecord["source"],
): PiSliceRecord | undefined {
  if (!isRecord(message) || typeof message.role !== "string") {
    stats.unsupportedMessagesOmitted += 1;
    return undefined;
  }
  const role = message.role;
  if (role === "custom" && message.customType === PI_ACCOMPANIMENT_MESSAGE_TYPE) {
    stats.priorEngramMessagesOmitted += 1;
    return undefined;
  }
  if (role === "user") {
    return { role, source, content: piContentText(message.content, stats) };
  }
  if (role === "assistant") {
    const parts = Array.isArray(message.content) ? message.content : [];
    const visible: string[] = [];
    for (const part of parts) {
      if (!isRecord(part)) continue;
      if (part.type === "thinking") {
        stats.thinkingBlocksOmitted += 1;
      } else if (part.type === "text" && typeof part.text === "string") {
        visible.push(part.text);
      } else if (part.type === "toolCall" && typeof part.name === "string") {
        visible.push(`[tool call]\n${safeJSONString({
          name: part.name,
          ...(typeof part.id === "string" ? { tool_call_id: part.id } : {}),
          arguments: part.arguments ?? {},
        })}`);
      }
    }
    if (visible.length === 0) return undefined;
    return { role, source, content: visible.join("\n") };
  }
  if (role === "toolResult") {
    return {
      role,
      source,
      content: piContentText(message.content, stats),
      ...(typeof message.toolName === "string" ? { tool_name: message.toolName } : {}),
      ...(typeof message.toolCallId === "string" ? { tool_call_id: message.toolCallId } : {}),
      ...(typeof message.isError === "boolean" ? { is_error: message.isError } : {}),
    };
  }
  if (role === "bashExecution") {
    const command = typeof message.command === "string" ? message.command : "";
    const output = typeof message.output === "string" ? message.output : "";
    return { role, source, content: `[command]\n${command}\n[output]\n${output}` };
  }
  if (role === "custom") {
    const customType = typeof message.customType === "string" ? message.customType : "unknown";
    const content = piContentText(message.content, stats);
    if (content === "") return undefined;
    return { role, source, custom_type: customType, content };
  }
  if (role === "compactionSummary" || role === "branchSummary") {
    const summary = typeof message.summary === "string" ? message.summary : "";
    if (summary === "") return undefined;
    return { role, source, content: summary };
  }
  stats.unsupportedMessagesOmitted += 1;
  return undefined;
}

function piContentText(content: unknown, stats: PiSliceStats): string {
  if (typeof content === "string") return content;
  if (!Array.isArray(content)) return "";
  const visible: string[] = [];
  for (const part of content) {
    if (!isRecord(part)) continue;
    if (part.type === "text" && typeof part.text === "string") {
      visible.push(part.text);
    } else if (part.type === "image") {
      stats.imagePayloadsOmitted += 1;
      const mimeType = typeof part.mimeType === "string" ? part.mimeType : "unknown";
      visible.push(`[image payload omitted; mime_type=${mimeType}]`);
    }
  }
  return visible.join("\n");
}

function piObservation(message: unknown): { role: string; content: string } | undefined {
  if (!isRecord(message)) return undefined;
  const stats: PiSliceStats = {
    thinkingBlocksOmitted: 0,
    imagePayloadsOmitted: 0,
    priorEngramMessagesOmitted: 0,
    unsupportedMessagesOmitted: 0,
  };
  const record = piMessageRecord(message, stats, "pi-session");
  if (record === undefined || record.content.trim() === "") return undefined;
  return { role: record.role, content: record.content };
}

function piPromptContent(prompt: string, images: readonly unknown[] = [], stats?: PiSliceStats): string {
  const localStats = stats ?? {
    thinkingBlocksOmitted: 0,
    imagePayloadsOmitted: 0,
    priorEngramMessagesOmitted: 0,
    unsupportedMessagesOmitted: 0,
  };
  return piContentText([{ type: "text", text: prompt }, ...images], localStats);
}

function fitPiRecord(record: PiSliceRecord, maxBytes: number): PiSliceRecord {
  if (jsonLineBytes(record) <= maxBytes) return record;
  const marker = "\n[content truncated by Pi active-thread slice byte limit]";
  const characters = Array.from(record.content);
  let low = 0;
  let high = characters.length;
  let fitted: PiSliceRecord = {
    ...record,
    content: marker,
    content_truncated: true,
  };
  while (low <= high) {
    const middle = Math.floor((low + high) / 2);
    const candidate: PiSliceRecord = {
      ...record,
      content: characters.slice(0, middle).join("") + marker,
      content_truncated: true,
    };
    if (jsonLineBytes(candidate) <= maxBytes) {
      fitted = candidate;
      low = middle + 1;
    } else {
      high = middle - 1;
    }
  }
  return fitted;
}

function jsonLineBytes(value: unknown): number {
  return Buffer.byteLength(JSON.stringify(value), "utf8") + 1;
}

function safeJSONString(value: unknown): string {
  try {
    return JSON.stringify(value ?? {});
  } catch {
    return "[unserializable tool arguments]";
  }
}

function parseAttribution(value: unknown): EngramAttribution {
  if (!isRecord(value)) throw new Error("Engram Hook attribution must be an object");
  const engramID = requiredNonBlankString(value, "engram_id", "Engram Hook attribution");
  const name = requiredNonBlankString(value, "name", "Engram Hook attribution");
  const accompanimentID = requiredNonBlankString(value, "accompaniment_id", "Engram Hook attribution");
  const statement = optionalString(value, "statement", "Engram Hook attribution");
  return {
    engram_id: engramID,
    name,
    ...(statement !== undefined && statement !== "" ? { statement } : {}),
    accompaniment_id: accompanimentID,
  };
}

function requiredNonBlankString(value: Record<string, unknown>, name: string, owner: string): string {
  const field = value[name];
  if (typeof field !== "string" || field.trim() === "") {
    throw new Error(`${owner} ${name} must be a non-empty string`);
  }
  return field;
}

function optionalString(value: Record<string, unknown>, name: string, owner = "Engram Hook result"): string | undefined {
  const field = value[name];
  if (field === undefined) return undefined;
  if (typeof field !== "string") throw new Error(`${owner} ${name} must be a string`);
  return field;
}

async function runJSON(
  config: CommandConfig,
  subcommand: "hook" | "invoke",
  payload: unknown,
  cwd: string,
  signal: AbortSignal | undefined,
  extraArgs: string[] = [],
): Promise<unknown> {
  const args = [subcommand, ...config.runtimeArgs, ...extraArgs];
  const child = signal === undefined
    ? spawn(config.command, args, { cwd, env: process.env, windowsHide: true, stdio: "pipe" })
    : spawn(config.command, args, { cwd, env: process.env, windowsHide: true, stdio: "pipe", signal });
  const stdout: Buffer[] = [];
  const stderr: Buffer[] = [];
  let outputBytes = 0;
  child.stdout.on("data", (chunk: Buffer) => {
    outputBytes += chunk.length;
    if (outputBytes > MAX_OUTPUT_BYTES) child.kill();
    stdout.push(chunk);
  });
  child.stderr.on("data", (chunk: Buffer) => stderr.push(chunk));
  child.stdin.end(JSON.stringify(payload));
  const code = await new Promise<number | null>((resolve, reject) => {
    child.once("error", reject);
    child.once("close", resolve);
  });
  if (outputBytes > MAX_OUTPUT_BYTES) throw new Error("Engram command output exceeded 16 MiB");
  if (code !== 0) {
    const diagnostic = Buffer.concat(stderr).toString("utf8").trim();
    throw new Error(diagnostic === "" ? `Engram command exited with code ${String(code)}` : diagnostic);
  }
  const raw = Buffer.concat(stdout).toString("utf8");
  try {
    return JSON.parse(raw) as unknown;
  } catch {
    throw new Error("Engram command returned invalid JSON");
  }
}

export function commandConfig(environment: NodeJS.ProcessEnv): CommandConfig {
  const command = environment.ENGRAM_COMMAND?.trim() || "engram";
  const rawArgs = environment.ENGRAM_RUNTIME_ARGS_JSON ?? "[]";
  let value: unknown;
  try {
    value = JSON.parse(rawArgs) as unknown;
  } catch {
    throw new Error("ENGRAM_RUNTIME_ARGS_JSON must be JSON");
  }
  if (!Array.isArray(value) || value.some((item) => typeof item !== "string")) {
    throw new Error("ENGRAM_RUNTIME_ARGS_JSON must be a string array");
  }
  return { command, runtimeArgs: value };
}

export function assistantText(message: unknown): string {
  if (!isRecord(message) || message.role !== "assistant") return "";
  if (typeof message.content === "string") return message.content;
  if (!Array.isArray(message.content)) return "";
  return message.content.flatMap((part) => {
    if (!isRecord(part) || part.type !== "text" || typeof part.text !== "string") return [];
    return [part.text];
  }).join("\n").trim();
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function errorText(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
