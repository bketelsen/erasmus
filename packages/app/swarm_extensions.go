package app

import (
	"context"
	"encoding/json"

	"erasmus/packages/extension"
	"erasmus/packages/swarm"
)

func applyExtensionBackgroundActions(ctx context.Context, s *swarm.Swarm, actions []extension.HostAction) error {
	for _, action := range actions {
		switch action.Type {
		case "background_spawn":
			var req swarm.SpawnRequest
			if err := json.Unmarshal(action.Data, &req); err != nil {
				return err
			}
			if _, err := s.Spawn(ctx, req); err != nil {
				return err
			}
		case "background_send":
			var data struct {
				ID   string `json:"id"`
				Text string `json:"text"`
			}
			if err := json.Unmarshal(action.Data, &data); err != nil {
				return err
			}
			if err := s.Send(ctx, data.ID, data.Text); err != nil {
				return err
			}
		case "background_stop":
			var data struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(action.Data, &data); err != nil {
				return err
			}
			if err := s.Stop(ctx, data.ID); err != nil {
				return err
			}
		}
	}
	return nil
}
