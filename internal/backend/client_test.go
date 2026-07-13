package backend

import (
	"encoding/json"
	"testing"
)

func TestToolCallFunction_ArgumentsMap_DecodedObject(t *testing.T) {
	f := ToolCallFunction{Name: "x", Arguments: json.RawMessage(`{"repo_path":"D:/repo"}`)}
	args, err := f.ArgumentsMap()
	if err != nil {
		t.Fatalf("ArgumentsMap: %v", err)
	}
	if args["repo_path"] != "D:/repo" {
		t.Fatalf("unexpected args: %+v", args)
	}
}

func TestToolCallFunction_ArgumentsMap_EncodedString(t *testing.T) {
	// The "proper" OpenAI shape: arguments is a JSON-encoded string.
	encoded, err := json.Marshal(`{"repo_path":"D:/repo"}`)
	if err != nil {
		t.Fatal(err)
	}
	f := ToolCallFunction{Name: "x", Arguments: encoded}
	args, err := f.ArgumentsMap()
	if err != nil {
		t.Fatalf("ArgumentsMap: %v", err)
	}
	if args["repo_path"] != "D:/repo" {
		t.Fatalf("unexpected args: %+v", args)
	}
}

func TestToolCallFunction_ArgumentsMap_Invalid(t *testing.T) {
	f := ToolCallFunction{Name: "x", Arguments: json.RawMessage(`123`)}
	if _, err := f.ArgumentsMap(); err == nil {
		t.Fatal("expected an error for arguments that are neither an object nor a JSON-encoded string")
	}
}
