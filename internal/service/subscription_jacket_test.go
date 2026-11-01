package service

import (
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
)

func TestRebaseANXJacket(t *testing.T) {
	anx := func(settings map[string]any) *model.ParsedNode {
		return &model.ParsedNode{Type: string(model.ProtocolANX), Settings: settings}
	}
	t.Run("moves the jacket to the port the entry offers", func(t *testing.T) {
		parsed := anx(map[string]any{"wg_listen_port": float64(11114)})
		rebaseANXJacket(parsed, func(jacket int) (int, bool) { return 15310, jacket == 11114 })
		if got := ANXWGPort(parsed.Settings); got != 15310 {
			t.Fatalf("wg port = %d, want 15310", got)
		}
	})
	t.Run("drops a jacket the entry does not carry", func(t *testing.T) {
		parsed := anx(map[string]any{"wg_listen_port": float64(11115)})
		rebaseANXJacket(parsed, func(jacket int) (int, bool) { return 15310, jacket == 11114 })
		if _, ok := parsed.Settings["wg_listen_port"]; ok {
			t.Fatalf("wg_listen_port kept: %v", parsed.Settings)
		}
	})
	t.Run("leaves nodes without a jacket alone", func(t *testing.T) {
		parsed := anx(map[string]any{})
		rebaseANXJacket(parsed, func(int) (int, bool) { t.Fatal("at called"); return 0, false })
		if len(parsed.Settings) != 0 {
			t.Fatalf("settings changed: %v", parsed.Settings)
		}
	})
	t.Run("ignores other protocols", func(t *testing.T) {
		parsed := &model.ParsedNode{Type: string(model.ProtocolWireGuard), Settings: map[string]any{"wg_listen_port": float64(51820)}}
		rebaseANXJacket(parsed, func(int) (int, bool) { return 0, false })
		if ANXWGPort(parsed.Settings) != 51820 {
			t.Fatalf("settings changed: %v", parsed.Settings)
		}
	})
}
