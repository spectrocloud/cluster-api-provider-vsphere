/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"regexp"
	"sigs.k8s.io/cluster-api-provider-vsphere/pkg/services/govmomi/bootstrap"
	"text/template"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
	"sigs.k8s.io/controller-runtime/pkg/client"

	infrav1 "sigs.k8s.io/cluster-api-provider-vsphere/api/v1alpha3"
)

// GetMachinesInCluster gets a cluster's Machine resources.
func GetMachinesInCluster(
	ctx context.Context,
	controllerClient client.Client,
	namespace, clusterName string) ([]*clusterv1.Machine, error) {

	labels := map[string]string{clusterv1.ClusterLabelName: clusterName}
	machineList := &clusterv1.MachineList{}

	if err := controllerClient.List(
		ctx, machineList,
		client.InNamespace(namespace),
		client.MatchingLabels(labels)); err != nil {
		return nil, errors.Wrapf(
			err, "error getting machines in cluster %s/%s",
			namespace, clusterName)
	}

	machines := make([]*clusterv1.Machine, len(machineList.Items))
	for i := range machineList.Items {
		machines[i] = &machineList.Items[i]
	}

	return machines, nil
}

// GetVSphereMachinesInCluster gets a cluster's VSphereMachine resources.
func GetVSphereMachinesInCluster(
	ctx context.Context,
	controllerClient client.Client,
	namespace, clusterName string) ([]*infrav1.VSphereMachine, error) {

	labels := map[string]string{clusterv1.ClusterLabelName: clusterName}
	machineList := &infrav1.VSphereMachineList{}

	if err := controllerClient.List(
		ctx, machineList,
		client.InNamespace(namespace),
		client.MatchingLabels(labels)); err != nil {
		return nil, err
	}

	machines := make([]*infrav1.VSphereMachine, len(machineList.Items))
	for i := range machineList.Items {
		machines[i] = &machineList.Items[i]
	}

	return machines, nil
}

// GetVSphereMachine gets a VSphereMachine resource for the given CAPI Machine.
func GetVSphereMachine(
	ctx context.Context,
	controllerClient client.Client,
	namespace, machineName string) (*infrav1.VSphereMachine, error) {

	machine := &infrav1.VSphereMachine{}
	namespacedName := apitypes.NamespacedName{
		Namespace: namespace,
		Name:      machineName,
	}
	if err := controllerClient.Get(ctx, namespacedName, machine); err != nil {
		return nil, err
	}
	return machine, nil
}

// ErrNoMachineIPAddr indicates that no valid IP addresses were found in a machine context
var ErrNoMachineIPAddr = errors.New("no IP addresses found for machine")

// GetMachinePreferredIPAddress returns the preferred IP address for a
// VSphereMachine resource.
func GetMachinePreferredIPAddress(machine *infrav1.VSphereMachine) (string, error) {
	var cidr *net.IPNet
	if cidrString := machine.Spec.Network.PreferredAPIServerCIDR; cidrString != "" {
		var err error
		if _, cidr, err = net.ParseCIDR(cidrString); err != nil {
			return "", errors.New("error parsing preferred API server CIDR")
		}
	}

	for _, machineAddr := range machine.Status.Addresses {
		if machineAddr.Type != clusterv1.MachineExternalIP {
			continue
		}
		if cidr == nil {
			return machineAddr.Address, nil
		}
		if cidr.Contains(net.ParseIP(machineAddr.Address)) {
			return machineAddr.Address, nil
		}
	}

	return "", ErrNoMachineIPAddr
}

// IsControlPlaneMachine returns true if the provided resource is
// a member of the control plane.
func IsControlPlaneMachine(machine metav1.Object) bool {
	_, ok := machine.GetLabels()[clusterv1.MachineControlPlaneLabelName]
	return ok
}

// GetMachineMetadata returns the cloud-init metadata as a base-64 encoded
// string for a given VSphereMachine.
func GetMachineMetadata(hostname string, machine infrav1.VSphereVM, networkStatus ...infrav1.NetworkStatus) ([]byte, error) {
	// Create a copy of the devices and add their MAC addresses from a network status.
	devices := make([]infrav1.NetworkDeviceSpec, len(machine.Spec.Network.Devices))
	var waitForIPv4, waitForIPv6 bool
	for i := range machine.Spec.Network.Devices {
		machine.Spec.Network.Devices[i].DeepCopyInto(&devices[i])
		if len(networkStatus) > 0 {
			devices[i].MACAddr = networkStatus[i].MACAddr
		}

		if waitForIPv4 && waitForIPv6 {
			// break early as we already wait for ipv4 and ipv6
			continue
		}
		// check static IPs
		for _, ipStr := range machine.Spec.Network.Devices[i].IPAddrs {
			ip := net.ParseIP(ipStr)
			// check the IP family
			if ip != nil {
				if ip.To4() == nil {
					waitForIPv6 = true
				} else {
					waitForIPv4 = true
				}
			}
		}
		// check if DHCP is enabled
		if machine.Spec.Network.Devices[i].DHCP4 {
			waitForIPv4 = true
		}
		if machine.Spec.Network.Devices[i].DHCP6 {
			waitForIPv6 = true
		}
	}

	buf := &bytes.Buffer{}
	tpl := template.Must(template.New("t").Funcs(
		template.FuncMap{
			"nameservers": func(spec infrav1.NetworkDeviceSpec) bool {
				return len(spec.Nameservers) > 0 || len(spec.SearchDomains) > 0
			},
		}).Parse(metadataFormat))
	if err := tpl.Execute(buf, struct {
		Hostname    string
		Devices     []infrav1.NetworkDeviceSpec
		Routes      []infrav1.NetworkRouteSpec
		WaitForIPv4 bool
		WaitForIPv6 bool
	}{
		Hostname:    hostname, // note that hostname determines the Kubernetes node name
		Devices:     devices,
		Routes:      machine.Spec.Network.Routes,
		WaitForIPv4: waitForIPv4,
		WaitForIPv6: waitForIPv6,
	}); err != nil {
		return nil, errors.Wrapf(
			err,
			"error getting cloud init metadata for machine %s/%s/%s",
			machine.Namespace, machine.ClusterName, machine.Name)
	}
	return buf.Bytes(), nil
}


// GetMachineMetadataIgnition returns the ignition metadata
// for a given VSphereMachine, withc network and .
func GetMachineMetadataIgnition(bootstrapData bootstrap.VMBootstrapData, hostname string, machine infrav1.VSphereVM, networkStatus ...infrav1.NetworkStatus) ([]byte, error) {
	// Create a copy of the devices and add their MAC addresses from a network status.
	devices := make([]infrav1.NetworkDeviceSpec, len(machine.Spec.Network.Devices))
	var waitForIPv4, waitForIPv6 bool
	for i := range machine.Spec.Network.Devices {
		machine.Spec.Network.Devices[i].DeepCopyInto(&devices[i])
		if len(networkStatus) > 0 {
			devices[i].MACAddr = networkStatus[i].MACAddr
		}

		if waitForIPv4 && waitForIPv6 {
			// break early as we already wait for ipv4 and ipv6
			continue
		}
		// check static IPs
		for _, ipStr := range machine.Spec.Network.Devices[i].IPAddrs {
			ip := net.ParseIP(ipStr)
			// check the IP family
			if ip != nil {
				if ip.To4() == nil {
					waitForIPv6 = true
				} else {
					waitForIPv4 = true
				}
			}
		}
		// check if DHCP is enabled
		if machine.Spec.Network.Devices[i].DHCP4 {
			waitForIPv4 = true
		}
		if machine.Spec.Network.Devices[i].DHCP6 {
			waitForIPv6 = true
		}
	}

	config, err := ConverBootstrapDatatoIgnition(bootstrapData.GetValue())
	if err != nil {
		return nil, err
	}

	setHostName(hostname, config)

	if !waitForIPv4 && !waitForIPv6 {
		setNetwork(devices, config)
	}



	data, err := json.Marshal(config)
	if err != nil {
		return nil, errors.Wrap(err, "unable to marshal cloudconfig")
	}

	return data, nil
}



const (
	// ProviderIDPrefix is the string data prefixed to a BIOS UUID in order
	// to build a provider ID.
	ProviderIDPrefix = "vsphere://"

	// ProviderIDPattern is a regex pattern and is used by ConvertProviderIDToUUID
	// to convert a providerID into a UUID string.
	ProviderIDPattern = `(?i)^` + ProviderIDPrefix + `([a-f\d]{8}-[a-f\d]{4}-[a-f\d]{4}-[a-f\d]{4}-[a-f\d]{12})$`

	// UUIDPattern is a regex pattern and is used by ConvertUUIDToProviderID
	// to convert a UUID into a providerID string.
	UUIDPattern = `(?i)^[a-f\d]{8}-[a-f\d]{4}-[a-f\d]{4}-[a-f\d]{4}-[a-f\d]{12}$`
)

// ConvertProviderIDToUUID transforms a provider ID into a UUID string.
// If providerID is nil, empty, or invalid, then an empty string is returned.
// A valid providerID should adhere to the format specified by
// ProviderIDPattern.
func ConvertProviderIDToUUID(providerID *string) string {
	if providerID == nil || *providerID == "" {
		return ""
	}
	pattern := regexp.MustCompile(ProviderIDPattern)
	matches := pattern.FindStringSubmatch(*providerID)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}

// ConvertUUIDToProviderID transforms a UUID string into a provider ID.
// If the supplied UUID is empty or invalid then an empty string is returned.
// A valid UUID should adhere to the format specified by UUIDPattern.
func ConvertUUIDToProviderID(uuid string) string {
	if uuid == "" {
		return ""
	}
	pattern := regexp.MustCompile(UUIDPattern)
	if !pattern.MatchString(uuid) {
		return ""
	}
	return ProviderIDPrefix + uuid
}
