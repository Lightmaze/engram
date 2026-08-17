import assert from "node:assert/strict";
import test from "node:test";
import {
  assistantText,
  buildPiThreadSlice,
  commandConfig,
  guardianMessage,
  parseHookResult,
} from "../src/v0.ts";

function parseThreadSlice(value: string): Array<Record<string, unknown>> {
  return value.split("\n").map((line) => JSON.parse(line) as Record<string, unknown>);
}

test("commandConfig keeps runtime flags after the subcommand boundary", () => {
  const config = commandConfig({
    ENGRAM_COMMAND: "C:/bin/engram.exe",
    ENGRAM_RUNTIME_ARGS_JSON: `["--data","C:/journal","--driver","openai-responses"]`,
  });
  assert.deepEqual(config, {
    command: "C:/bin/engram.exe",
    runtimeArgs: ["--data", "C:/journal", "--driver", "openai-responses"],
  });
});

test("assistantText preserves only assistant text blocks", () => {
  assert.equal(assistantText({
    role: "assistant",
    content: [
      { type: "thinking", thinking: "private" },
      { type: "text", text: "first" },
      { type: "toolCall", name: "read" },
      { type: "text", text: "second" },
    ],
  }), "first\nsecond");
  assert.equal(assistantText({ role: "user", content: "not observed" }), "");
});

test("Pi guardian sends a real multi-message branch slice without echoing Engram speech or private thinking", () => {
  const branch = [
    { type: "message", id: "u1", message: { role: "user", content: "Run moonmatch help." } },
    {
      type: "message",
      id: "a1",
      message: {
        role: "assistant",
        content: [
          { type: "thinking", thinking: "private chain" },
          { type: "text", text: "I will inspect it." },
          { type: "toolCall", id: "call-1", name: "bash", arguments: { command: "moonmatch --help" } },
          { type: "toolCall", id: "call-2", name: "bash", arguments: { command: "moonmatch --version" } },
        ],
      },
    },
    {
      type: "message",
      id: "t2",
      message: {
        role: "toolResult",
        toolName: "bash",
        toolCallId: "call-2",
        isError: false,
        content: [{ type: "text", text: "MoonMatch version 2" }],
      },
    },
    {
      type: "message",
      id: "t1",
      message: {
        role: "toolResult",
        toolName: "bash",
        toolCallId: "call-1",
        isError: false,
        content: [
          { type: "text", text: "MoonMatch usage: ..." },
          { type: "image", mimeType: "image/png", data: "base64-secret" },
        ],
      },
    },
    {
      type: "custom_message",
      id: "e1",
      customType: "engram-accompaniment",
      content: "old Engram steering",
    },
  ];
  const lines = parseThreadSlice(buildPiThreadSlice(
    branch,
    "Can you see the help now?",
    "design-ancestor",
    64 * 1024,
    [{ type: "image", mimeType: "image/jpeg", data: "current-base64-secret" }],
  ));
  const metadata = lines[0];
  assert.ok(metadata);
  const serialized = JSON.stringify(lines);
  assert.equal(metadata.protocol_version, "engram-pi-thread-slice/v1");
  assert.equal(metadata.mode, "initial_snapshot");
  assert.equal(metadata.thinking_blocks_omitted, 1);
  assert.equal(metadata.image_payloads_omitted, 2);
  assert.equal(metadata.prior_engram_messages_omitted, 1);
  assert.match(serialized, /Run moonmatch help/);
  assert.match(serialized, /moonmatch --help/);
  assert.match(serialized, /MoonMatch usage/);
  assert.match(serialized, /MoonMatch version 2/);
  assert.match(serialized, /Can you see the help now/);
  const assistant = lines.find((line) => line.role === "assistant");
  assert.ok(assistant);
  assert.match(String(assistant.content), /"tool_call_id":"call-1"/);
  assert.match(String(assistant.content), /"tool_call_id":"call-2"/);
  assert.deepEqual(
    lines.filter((line) => line.role === "toolResult").map((line) => line.tool_call_id),
    ["call-2", "call-1"],
  );
  assert.match(serialized, /image payload omitted; mime_type=image\/jpeg/);
  assert.doesNotMatch(serialized, /private chain|base64-secret|current-base64-secret|old Engram steering/);
});

test("Pi guardian resumes after its durable observation cursor and marks byte-budget omissions", () => {
  const branch = [
    { type: "message", id: "old", message: { role: "user", content: "old context" } },
    {
      type: "custom",
      id: "cursor",
      customType: "engram-thread-observed",
      data: { engram_id: "design-ancestor" },
    },
    { type: "message", id: "new-0", message: { role: "user", content: "earlier delta" } },
    { type: "message", id: "new-1", message: { role: "assistant", content: [{ type: "text", text: "x".repeat(4000) }] } },
    { type: "message", id: "new-2", message: { role: "toolResult", content: [{ type: "text", text: "recent result" }] } },
  ];
  const value = buildPiThreadSlice(branch, "next prompt", "design-ancestor", 2048);
  const lines = parseThreadSlice(value);
  const metadata = lines[0];
  assert.ok(metadata);
  const serialized = JSON.stringify(lines);
  assert.equal(metadata.mode, "delta");
  assert.ok(Number(metadata.earlier_visible_messages_omitted) >= 1);
  assert.ok(Buffer.byteLength(value, "utf8") <= 2048);
  assert.doesNotMatch(serialized, /old context/);
  assert.match(serialized, /recent result/);
  assert.match(serialized, /next prompt/);
});

test("Pi guardian counts a prior record that cannot fit even as a truncated record", () => {
  const branch = [
    {
      type: "custom_message",
      id: "oversized-metadata",
      customType: "x".repeat(4096),
      content: "visible but impossible to fit with its metadata",
    },
  ];
  const value = buildPiThreadSlice(branch, "current", "design-ancestor", 2048);
  const lines = parseThreadSlice(value);
  assert.equal(lines[0]?.selected_prior_messages, 0);
  assert.equal(lines[0]?.earlier_visible_messages_omitted, 1);
  assert.ok(Buffer.byteLength(value, "utf8") <= 2048);
});

test("Pi guardian enforces the byte budget after JSON escaping expansion", () => {
  const value = buildPiThreadSlice([], "\u0001".repeat(20_000), "design-ancestor", 2048);
  const lines = parseThreadSlice(value);
  assert.ok(Buffer.byteLength(value, "utf8") <= 2048);
  assert.equal(lines[1]?.content_truncated, true);
  assert.match(String(lines[1]?.content), /content truncated by Pi active-thread slice byte limit/);
});

test("Pi guardian preserves runtime-owned attribution and envelope", () => {
  const runtimeEnvelope = [
    "[Engram: attributed Engram speech]",
    'Attribution: {"engram_id":"design-ancestor","name":"Design Ancestor","statement":"Keep the original question alive.","accompaniment_id":"acc-123"}',
    "Steering:",
    "Do not flatten the historical disagreement.",
  ].join("\n");
  const result = parseHookResult({
    protocol_version: "engram-hook/v0",
    action: "continue",
    additional_context: runtimeEnvelope,
    accompaniment_id: "acc-123",
    attribution: {
      engram_id: "design-ancestor",
      name: "Design Ancestor",
      statement: "Keep the original question alive.",
      accompaniment_id: "acc-123",
    },
    wake_state: "active",
  });

  const message = guardianMessage(result);
  assert.ok(message);
  assert.equal(message.content, runtimeEnvelope);
  assert.deepEqual(message.details, {
    accompaniment_id: "acc-123",
    attribution: {
      engram_id: "design-ancestor",
      name: "Design Ancestor",
      statement: "Keep the original question alive.",
      accompaniment_id: "acc-123",
    },
    wake_state: "active",
    mode: "guardian",
  });
});

test("Pi guardian refuses speech with missing or incomplete attribution", () => {
  assert.throws(
    () => parseHookResult({ additional_context: "bare steering", accompaniment_id: "acc-123" }),
    /without attribution/,
  );
  assert.throws(
    () => parseHookResult({
      additional_context: "runtime envelope",
      accompaniment_id: "acc-123",
      attribution: {
        engram_id: "design-ancestor",
        accompaniment_id: "acc-123",
      },
    }),
    /name must be a non-empty string/,
  );
});

test("Pi guardian rejects contradictory accompaniment attribution", () => {
  assert.throws(
    () => parseHookResult({
      additional_context: "runtime envelope",
      accompaniment_id: "acc-visible",
      attribution: {
        engram_id: "design-ancestor",
        name: "Design Ancestor",
        accompaniment_id: "acc-other",
      },
    }),
    /does not match/,
  );
});

test("Pi guardian keeps silent and legacy empty Hook results non-injecting", () => {
  assert.equal(guardianMessage(parseHookResult({})), undefined);
  assert.equal(guardianMessage(parseHookResult({
    additional_context: "",
    accompaniment_id: "acc-silent",
    attribution: {
      engram_id: "design-ancestor",
      name: "Design Ancestor",
      accompaniment_id: "acc-silent",
    },
    wake_state: "silent",
  })), undefined);
});
