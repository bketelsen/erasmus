package extension_test

import (
	"testing"

	"erasmus/packages/event"
	"erasmus/packages/extension"
	"erasmus/packages/extension/proto"
)

func TestManagerPublishesExtensionUpdatesForRegistrations(t *testing.T) {
	m := extension.NewManager(nil)
	var updates []event.ExtensionUpdate
	m.Subscribe(func(ev event.Event) {
		if update, ok := ev.(event.ExtensionUpdate); ok {
			updates = append(updates, update)
		}
	})
	m.RegisterTool(proto.RegisterTool{Name: "weather", Description: "get weather"})
	m.RegisterCommand(proto.RegisterCommand{Name: "hello", Description: "say hello"}, nil)
	if len(updates) != 2 {
		t.Fatalf("updates = %+v", updates)
	}
	if updates[0].Action != "register_tool" || len(updates[0].Tools) != 1 || updates[0].Tools[0].Name != "weather" {
		t.Fatalf("tool update = %+v", updates[0])
	}
	if updates[1].Action != "register_command" || len(updates[1].Commands) != 1 || updates[1].Commands[0].Name != "hello" {
		t.Fatalf("command update = %+v", updates[1])
	}
}
