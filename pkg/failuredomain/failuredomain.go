/*
 * Temporary implementation of failure domain by specify through annotation
 */

package failuredomain

import (
	"encoding/json"

	"github.com/go-logr/logr"
	infrav1 "sigs.k8s.io/cluster-api-provider-vsphere/api/v1alpha3"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

const (
	// use this key to get seralized string from annotation as failure domain
	FailureDomainAnnotationKey string = "vsphere.infra.cluster.x-k8s.io/failure-domain"

	FailureDomainKeyDatacenter   string = "Datacenter"
	FailureDomainKeyFolder       string = "Folder"
	FailureDomainKeyDatastore    string = "Datastore"
	FailureDomainKeyResourcePool string = "ResourcePool"
)

// ControlPlaneFailureDomain is the placement properties specified to spread
// cp nodes into different compute clusters
type ControlPlaneFailureDomain struct {
	// Datacenter is the datacenter in which VMs are created/located.
	// +optional
	Datacenter string `json:"datacenter,omitempty"`

	// Folder is the folder in which VMs are created/located.
	// +optional
	Folder string `json:"folder,omitempty"`

	// Datastore is the datastore in which VMs are created/located.
	// +optional
	Datastore string `json:"datastore,omitempty"`

	// ResourcePool is the resource pool in which VMs are created/located.
	// +optional
	ResourcePool string `json:"resourcePool,omitempty"`
}

// map key is compute cluster
type ControlPlaneFailureDomains map[string]ControlPlaneFailureDomain

func (c *ControlPlaneFailureDomain) GetFailureDomain() clusterv1.FailureDomainSpec {
	return clusterv1.FailureDomainSpec{
		ControlPlane: true,
		Attributes: map[string]string{
			FailureDomainKeyDatacenter:   c.Datacenter,
			FailureDomainKeyFolder:       c.Folder,
			FailureDomainKeyDatastore:    c.Datastore,
			FailureDomainKeyResourcePool: c.ResourcePool,
		},
	}
}

func (c *ControlPlaneFailureDomain) SetFailureDomain(fd clusterv1.FailureDomainSpec) {
	if fd.Attributes == nil {
		return
	}
	c.Datacenter = fd.Attributes[FailureDomainKeyDatacenter]
	c.Folder = fd.Attributes[FailureDomainKeyFolder]
	c.Datastore = fd.Attributes[FailureDomainKeyDatastore]
	c.ResourcePool = fd.Attributes[FailureDomainKeyResourcePool]
}

func ReconcileFailureDomain(log logr.Logger, vsphereCluster *infrav1.VSphereCluster) {
	if val, ok := vsphereCluster.Annotations[FailureDomainAnnotationKey]; ok && len(val) > 0 {
		failureDomains := ControlPlaneFailureDomains{}
		if err := json.Unmarshal([]byte(val), &failureDomains); err != nil {
			log.Error(err, "faild to parse failure domain", "annotation", val)
			return
		}

		fds := make(clusterv1.FailureDomains)
		for key, fd := range failureDomains {
			spec := fd.GetFailureDomain()
			fds[key] = spec
		}
		vsphereCluster.Status.FailureDomains = fds
	}
}

func UpdateVSphereVMFromFailureDomain(vsphereCluster *infrav1.VSphereCluster, vm *infrav1.VSphereVM, failureDomain string) {
	if spec, ok := vsphereCluster.Status.FailureDomains[failureDomain]; ok {
		cpfd := ControlPlaneFailureDomain{}
		cpfd.SetFailureDomain(spec)
		if cpfd.Datacenter != "" {
			vm.Spec.Datacenter = cpfd.Datacenter
		}
		if cpfd.Datastore != "" {
			vm.Spec.Datastore = cpfd.Datastore
		}
		if cpfd.Folder != "" {
			vm.Spec.Folder = cpfd.Folder
		}
		if cpfd.ResourcePool != "" {
			vm.Spec.ResourcePool = cpfd.ResourcePool
		}
	}
}
