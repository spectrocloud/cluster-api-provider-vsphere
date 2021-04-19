package util

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/coreos/ignition/config/util"
	ignitionTypes "github.com/coreos/ignition/config/v2_3/types"
	"github.com/pkg/errors"
	infrav1 "sigs.k8s.io/cluster-api-provider-vsphere/api/v1alpha3"
)

const (
	hostNamePath   = "/etc/hostname"
	rootFileSystem = "root"
)

func ConverBootstrapDatatoIgnition(data []byte) (*ignitionTypes.Config, error) {
	config := &ignitionTypes.Config{}
	if err := json.Unmarshal(data, config); err != nil {
		return nil, errors.Wrap(err, "unable to unmarshal bootstrap data into ingition type")
	}
	return config, nil
}

func setHostName(hostname string, config *ignitionTypes.Config) *ignitionTypes.Config {
	for _, file := range config.Storage.Files {
		if file.Path == hostNamePath {
			return config
		}
	}

	// if not found we must set the hostname
	config.Storage.Files = append(config.Storage.Files, ignitionTypes.File{
		Node: ignitionTypes.Node{
			Filesystem: rootFileSystem,
			Path:       hostNamePath,
		},
		FileEmbedded1: ignitionTypes.FileEmbedded1{
			Append: false,
			Contents: ignitionTypes.FileContents{
				Source: fmt.Sprintf("data:,%s", hostname),
			},
			Mode: util.IntToPtr(420),
		},
	})
	return config
}

func setNetwork(devices []infrav1.NetworkDeviceSpec, config *ignitionTypes.Config) *ignitionTypes.Config {
	ip4 := ""
	gateway4 := ""
	dns := ""
	searchDomains := ""
	for _, device := range devices {
		if len(device.IPAddrs) > 0 {
			ip4 = device.IPAddrs[0]
			gateway4 = device.Gateway4
			dns = strings.Join(device.Nameservers, " ")
			searchDomains = strings.Join(device.SearchDomains, " ")
			break
		}
	}

	if len(config.Networkd.Units) == 0 {
		config.Networkd.Units = append(config.Networkd.Units, ignitionTypes.Networkdunit{
			Contents: fmt.Sprintf("[Match]\nName=ens192\n\n[Network]\nAddress=%s\nGateway=%s\nDNS=%s\nDomains=%s", ip4, gateway4, dns, searchDomains),
			Name:     "00-ens192.network",
		})
	}

	return config
}
