package server

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Lightmaze/engram/internal/engram"
)

func TestMCPListsTenV0Tools(t *testing.T) {
	j, _ := engram.OpenJournal(filepath.Join(t.TempDir(), "data"))
	r, _ := engram.NewRuntime(j, engram.RuleProvider{})
	var output bytes.Buffer
	input := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}` + "\n" + `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}` + "\n")
	if err := (MCP{Runtime: r, Journal: j}).Serve(context.Background(), input, &output); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("output = %s", output.String())
	}
	var response struct {
		Result struct {
			Tools []any `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(lines[1]), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Result.Tools) != 10 {
		t.Fatalf("tools = %d", len(response.Result.Tools))
	}
}

func TestMCPRequestSchemasMatchGoJSONContracts(t *testing.T) {
	contracts := map[string]reflect.Type{
		"engram_list":          reflect.TypeOf(struct{}{}),
		"engram_create":        reflect.TypeOf(engram.CreateRequest{}),
		"engram_summon":        reflect.TypeOf(engram.SummonRequest{}),
		"engram_wake":          reflect.TypeOf(engram.WakeRequest{}),
		"engram_observe":       reflect.TypeOf(engram.ObserveRequest{}),
		"engram_release":       reflect.TypeOf(engram.ReleaseRequest{}),
		"engram_outcome":       reflect.TypeOf(engram.OutcomeRequest{}),
		"engram_fold_status":   reflect.TypeOf(engram.FoldStatusRequest{}),
		"engram_fold_revert":   reflect.TypeOf(engram.FoldRevertRequest{}),
		"engram_guardian_take": reflect.TypeOf(engram.GuardianTakeRequest{}),
	}
	byName := map[string]map[string]any{}
	for _, tool := range tools() {
		name, ok := tool["name"].(string)
		if !ok {
			t.Fatalf("tool without string name: %#v", tool)
		}
		byName[name] = tool
	}
	for name, contract := range contracts {
		tool, ok := byName[name]
		if !ok {
			t.Fatalf("missing tool %s", name)
		}
		schema, ok := tool["inputSchema"].(map[string]any)
		if !ok {
			t.Fatalf("%s inputSchema = %#v", name, tool["inputSchema"])
		}
		assertSchemaMatchesJSONType(t, name, schema, contract)
	}
	if len(byName) != len(contracts) {
		t.Fatalf("tool/schema contract count = %d/%d", len(byName), len(contracts))
	}

	create := byName["engram_create"]["inputSchema"].(map[string]any)
	messages := create["properties"].(map[string]any)["messages"].(map[string]any)
	messageSchema := messages["items"].(map[string]any)
	assertSchemaMatchesJSONType(t, "engram_create.messages[]", messageSchema, reflect.TypeOf(engram.Message{}))
}

func assertSchemaMatchesJSONType(t *testing.T, name string, schema map[string]any, contract reflect.Type) {
	t.Helper()
	if schema["additionalProperties"] != false {
		t.Fatalf("%s must reject unknown fields", name)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("%s properties = %#v", name, schema["properties"])
	}
	actualFields := sortedMapKeys(properties)
	expectedFields, expectedRequired := jsonContract(contract)
	if !reflect.DeepEqual(actualFields, expectedFields) {
		t.Fatalf("%s schema fields = %v, Go JSON fields = %v", name, actualFields, expectedFields)
	}
	for index := 0; index < contract.NumField(); index++ {
		field := contract.Field(index)
		jsonName := strings.Split(field.Tag.Get("json"), ",")[0]
		if jsonName == "-" {
			continue
		}
		if jsonName == "" {
			jsonName = field.Name
		}
		assertSchemaTypeMatchesGo(t, name+"."+jsonName, properties[jsonName], field.Type)
	}
	actualRequired, _ := schema["required"].([]string)
	if actualRequired == nil {
		actualRequired = []string{}
	}
	sort.Strings(actualRequired)
	if !reflect.DeepEqual(actualRequired, expectedRequired) {
		t.Fatalf("%s required = %v, Go non-omitempty fields = %v", name, actualRequired, expectedRequired)
	}
}

func assertSchemaTypeMatchesGo(t *testing.T, name string, schema any, contract reflect.Type) {
	t.Helper()
	if contract.Kind() == reflect.Pointer {
		contract = contract.Elem()
	}
	expected := map[reflect.Kind]string{
		reflect.String: "string",
		reflect.Bool:   "boolean",
		reflect.Int:    "integer",
		reflect.Int8:   "integer",
		reflect.Int16:  "integer",
		reflect.Int32:  "integer",
		reflect.Int64:  "integer",
		reflect.Slice:  "array",
		reflect.Struct: "object",
	}[contract.Kind()]
	if contract == reflect.TypeOf(time.Time{}) {
		expected = "string"
	}
	actual, format := schemaTypeAndFormat(schema)
	if expected == "" {
		t.Fatalf("%s has unsupported Go type %s", name, contract)
	}
	if actual != expected {
		t.Fatalf("%s schema type = %q, Go type %s requires %q", name, actual, contract, expected)
	}
	if contract == reflect.TypeOf(time.Time{}) && format != "date-time" {
		t.Fatalf("%s schema format = %q, time.Time requires date-time", name, format)
	}
}

func schemaTypeAndFormat(schema any) (schemaType, format string) {
	switch value := schema.(type) {
	case map[string]any:
		schemaType, _ = value["type"].(string)
		format, _ = value["format"].(string)
	case map[string]string:
		schemaType = value["type"]
		format = value["format"]
	}
	return schemaType, format
}

func jsonContract(contract reflect.Type) (fields []string, required []string) {
	fields = []string{}
	required = []string{}
	for index := 0; index < contract.NumField(); index++ {
		field := contract.Field(index)
		tag := field.Tag.Get("json")
		parts := strings.Split(tag, ",")
		name := parts[0]
		if name == "-" {
			continue
		}
		if name == "" {
			name = field.Name
		}
		fields = append(fields, name)
		optional := false
		for _, option := range parts[1:] {
			optional = optional || option == "omitempty"
		}
		if !optional {
			required = append(required, name)
		}
	}
	sort.Strings(fields)
	sort.Strings(required)
	return fields, required
}

func sortedMapKeys(value map[string]any) []string {
	keys := make([]string, 0, len(value))
	for key := range value {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
