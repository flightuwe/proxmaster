package proxmox

import (
	"context"

	"proxmaster/backend/internal/models"
	"proxmaster/backend/internal/store"
)

type Client struct {
	store *store.MemoryStore
}

func NewClient(s *store.MemoryStore) *Client {
	return &Client{store: s}
}

func (c *Client) GetState(_ context.Context) models.ClusterState {
	return c.store.ClusterState()
}

func (c *Client) SetNodeMaintenance(_ context.Context, nodeID string, maintenance bool) (map[string]any, error) {
	ok := c.store.SetNodeMaintenance(nodeID, maintenance)
	return map[string]any{"changed": ok, "node_id": nodeID, "maintenance": maintenance}, nil
}

func (c *Client) MigrateVM(_ context.Context, vmID, targetNode string) (map[string]any, error) {
	ok := c.store.MigrateVM(vmID, targetNode)
	return map[string]any{"changed": ok, "vm_id": vmID, "target_node": targetNode}, nil
}

func (c *Client) ApplyStoragePool(_ context.Context, name, poolType string) (map[string]any, error) {
	c.store.ApplyPool(name, poolType)
	return map[string]any{"changed": true, "pool_name": name, "pool_type": poolType}, nil
}

func (c *Client) ApplyNetwork(_ context.Context, name, kind, cidr string) (map[string]any, error) {
	c.store.ApplyNetwork(name, kind, cidr)
	return map[string]any{"changed": true, "name": name, "kind": kind, "cidr": cidr}, nil
}