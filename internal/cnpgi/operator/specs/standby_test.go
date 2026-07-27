/*
Copyright The CloudNativePG Contributors
Copyright 2025, Opera Norway AS

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

package specs

import (
	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pgbackrestApi "github.com/operasoftware/cnpg-plugin-pgbackrest/internal/pgbackrest/api"
)

func boolPtr(b bool) *bool { return &b }

var _ = Describe("InjectStandbyBackupTopology", func() {
	const (
		clusterName = "cluster-a"
		namespace   = "ns-a"
		serviceName = "cluster-a-pgbackrest"
	)

	expectedDNSNames := []string{
		serviceName,
		serviceName + ".ns-a",
		serviceName + ".ns-a.svc",
		serviceName + ".ns-a.svc.cluster.local",
	}

	newCluster := func() *cnpgv1.Cluster {
		return &cnpgv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: clusterName, Namespace: namespace},
		}
	}

	It("injects a headless managed service targeting the primary and the SANs", func() {
		cluster := newCluster()
		InjectStandbyBackupTopology(cluster, &pgbackrestApi.StandbyBackupConfiguration{Mode: "y"})

		Expect(cluster.Spec.Managed).ToNot(BeNil())
		Expect(cluster.Spec.Managed.Services).ToNot(BeNil())
		Expect(cluster.Spec.Managed.Services.Additional).To(HaveLen(1))

		svc := cluster.Spec.Managed.Services.Additional[0]
		Expect(svc.SelectorType).To(Equal(cnpgv1.ServiceSelectorTypeRW))
		Expect(svc.ServiceTemplate.ObjectMeta.Name).To(Equal(serviceName))
		Expect(svc.ServiceTemplate.Spec.ClusterIP).To(Equal(corev1.ClusterIPNone))
		Expect(svc.ServiceTemplate.Spec.Ports).To(HaveLen(1))
		Expect(svc.ServiceTemplate.Spec.Ports[0].Port).To(Equal(int32(8432)))
		Expect(svc.ServiceTemplate.Spec.Ports[0].Protocol).To(Equal(corev1.ProtocolTCP))

		Expect(cluster.Spec.Certificates).ToNot(BeNil())
		Expect(cluster.Spec.Certificates.ServerAltDNSNames).To(Equal(expectedDNSNames))
	})

	It("skips the service when ManageService is false", func() {
		cluster := newCluster()
		InjectStandbyBackupTopology(cluster, &pgbackrestApi.StandbyBackupConfiguration{
			Mode:          "y",
			ManageService: boolPtr(false),
		})

		Expect(cluster.Spec.Managed).To(BeNil())
		Expect(cluster.Spec.Certificates.ServerAltDNSNames).To(Equal(expectedDNSNames))
	})

	It("skips the SANs when ManageCertificate is false", func() {
		cluster := newCluster()
		InjectStandbyBackupTopology(cluster, &pgbackrestApi.StandbyBackupConfiguration{
			Mode:              "y",
			ManageCertificate: boolPtr(false),
		})

		Expect(cluster.Spec.Managed.Services.Additional).To(HaveLen(1))
		Expect(cluster.Spec.Certificates).To(BeNil())
	})

	It("is idempotent (running twice adds nothing new)", func() {
		cluster := newCluster()
		InjectStandbyBackupTopology(cluster, &pgbackrestApi.StandbyBackupConfiguration{Mode: "y"})
		InjectStandbyBackupTopology(cluster, &pgbackrestApi.StandbyBackupConfiguration{Mode: "y"})

		Expect(cluster.Spec.Managed.Services.Additional).To(HaveLen(1))
		Expect(cluster.Spec.Certificates.ServerAltDNSNames).To(Equal(expectedDNSNames))
	})

	It("preserves existing services and alt names", func() {
		cluster := newCluster()
		cluster.Spec.Managed = &cnpgv1.ManagedConfiguration{
			Services: &cnpgv1.ManagedServices{
				Additional: []cnpgv1.ManagedService{
					{
						SelectorType: cnpgv1.ServiceSelectorTypeRO,
						ServiceTemplate: cnpgv1.ServiceTemplateSpec{
							ObjectMeta: cnpgv1.Metadata{Name: "existing-svc"},
						},
					},
				},
			},
		}
		cluster.Spec.Certificates = &cnpgv1.CertificatesConfiguration{
			ServerAltDNSNames: []string{"already-there"},
		}

		InjectStandbyBackupTopology(cluster, &pgbackrestApi.StandbyBackupConfiguration{Mode: "prefer"})

		Expect(cluster.Spec.Managed.Services.Additional).To(HaveLen(2))
		Expect(cluster.Spec.Managed.Services.Additional[0].ServiceTemplate.ObjectMeta.Name).To(Equal("existing-svc"))
		Expect(cluster.Spec.Managed.Services.Additional[1].ServiceTemplate.ObjectMeta.Name).To(Equal(serviceName))
		Expect(cluster.Spec.Certificates.ServerAltDNSNames).To(Equal(append([]string{"already-there"}, expectedDNSNames...)))
	})
})

var _ = Describe("StandbyBackupServiceDNSNames", func() {
	It("returns the four in-cluster DNS forms", func() {
		Expect(StandbyBackupServiceDNSNames("svc", "ns")).To(Equal([]string{
			"svc", "svc.ns", "svc.ns.svc", "svc.ns.svc.cluster.local",
		}))
	})
})
