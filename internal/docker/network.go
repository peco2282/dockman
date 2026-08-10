package docker

import (
	"context"
	"sort"

	"github.com/moby/moby/client"

	"dockman/internal/model"
)

func (m *Manager) ListNetworks(ctx context.Context) ([]model.Network, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.client == nil {
		return nil, ErrNotConnected
	}
	result, err := m.client.NetworkList(ctx, client.NetworkListOptions{})
	if err != nil {
		return nil, err
	}
	items := make([]model.Network, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, model.Network{ID: item.ID, Name: item.Name, Driver: item.Driver, Scope: item.Scope, Created: item.Created, Internal: item.Internal, Attachable: item.Attachable, IPv4: item.EnableIPv4, IPv6: item.EnableIPv6})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}

func (m *Manager) InspectNetwork(ctx context.Context, id string) (model.Network, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.client == nil {
		return model.Network{}, ErrNotConnected
	}
	result, err := m.client.NetworkInspect(ctx, id, client.NetworkInspectOptions{})
	if err != nil {
		return model.Network{}, err
	}
	item := result.Network
	n := model.Network{ID: item.ID, Name: item.Name, Driver: item.Driver, Scope: item.Scope, Created: item.Created, Internal: item.Internal, Attachable: item.Attachable, IPv4: item.EnableIPv4, IPv6: item.EnableIPv6, IPAMDriver: item.IPAM.Driver, Labels: item.Labels, Options: item.Options}
	for _, config := range item.IPAM.Config {
		subnet := model.NetworkSubnet{}
		if config.Subnet.IsValid() {
			subnet.Subnet = config.Subnet.String()
		}
		if config.Gateway.IsValid() {
			subnet.Gateway = config.Gateway.String()
		}
		if config.IPRange.IsValid() {
			subnet.IPRange = config.IPRange.String()
		}
		n.Subnets = append(n.Subnets, subnet)
	}
	for id, endpoint := range item.Containers {
		c := model.NetworkContainer{ID: id, Name: endpoint.Name, MACAddress: endpoint.MacAddress.String()}
		if endpoint.IPv4Address.IsValid() {
			c.IPv4Address = endpoint.IPv4Address.String()
		}
		if endpoint.IPv6Address.IsValid() {
			c.IPv6Address = endpoint.IPv6Address.String()
		}
		n.Containers = append(n.Containers, c)
	}
	sort.Slice(n.Containers, func(i, j int) bool { return n.Containers[i].Name < n.Containers[j].Name })
	return n, nil
}

func (m *Manager) CreateNetwork(ctx context.Context, name, driver string) (model.Network, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.client == nil {
		return model.Network{}, ErrNotConnected
	}
	result, err := m.client.NetworkCreate(ctx, name, client.NetworkCreateOptions{Driver: driver})
	if err != nil {
		return model.Network{}, err
	}
	return model.Network{ID: result.ID, Name: name, Driver: driver}, nil
}

func (m *Manager) RemoveNetwork(ctx context.Context, id string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.client == nil {
		return ErrNotConnected
	}
	_, err := m.client.NetworkRemove(ctx, id, client.NetworkRemoveOptions{})
	return err
}

func (m *Manager) ConnectNetwork(ctx context.Context, networkID, containerID string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.client == nil {
		return ErrNotConnected
	}
	_, err := m.client.NetworkConnect(ctx, networkID, client.NetworkConnectOptions{Container: containerID})
	return err
}

func (m *Manager) DisconnectNetwork(ctx context.Context, networkID, containerID string) error {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.client == nil {
		return ErrNotConnected
	}
	_, err := m.client.NetworkDisconnect(ctx, networkID, client.NetworkDisconnectOptions{Container: containerID})
	return err
}
