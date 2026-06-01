package proto_test

import (
	"encoding/json"
	"testing"

	"erasmus/packages/extension/proto"
)

func TestEncodeDecodeFrame(t *testing.T) {
	frame, err := proto.EncodeFrame("hello", "1", proto.Hello{Name: "x", Version: "v"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(frame)
	if err != nil {
		t.Fatal(err)
	}
	var decoded proto.Frame
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	var hello proto.Hello
	if err := proto.DecodeData(decoded, &hello); err != nil {
		t.Fatal(err)
	}
	if hello.Name != "x" || hello.Version != "v" {
		t.Fatalf("hello = %+v", hello)
	}
}
