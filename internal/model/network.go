package model

import "time"

type Network struct {
	ID         string
	Name       string
	Driver     string
	Scope      string
	Created    time.Time
	Internal   bool
	Attachable bool
	IPv4       bool
	IPv6       bool
	IPAMDriver string
	Subnets    []NetworkSubnet
	Containers []NetworkContainer
	Labels     map[string]string
	Options    map[string]string
}

type NetworkSubnet struct {
	Subnet  string
	Gateway string
	IPRange string
}

type NetworkContainer struct {
	ID          string
	Name        string
	IPv4Address string
	IPv6Address string
	MACAddress  string
}
