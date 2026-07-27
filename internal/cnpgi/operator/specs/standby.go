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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	pgbackrestApi "github.com/operasoftware/cnpg-plugin-pgbackrest/internal/pgbackrest/api"
	pgbackrestCommand "github.com/operasoftware/cnpg-plugin-pgbackrest/internal/pgbackrest/command"
)

// standbyBackupServicePortName is the name of the pgBackRest port on the
// injected headless service.
const standbyBackupServicePortName = "pgbackrest"

// InjectStandbyBackupTopology mutates the cluster in place so a standby can
// reach the current primary's pgBackRest server. It adds a headless managed
// service that targets the primary on the pgBackRest port, and the matching
// certificate alternative names. Each part is opt-outable via the standby
// configuration, and the mutation is idempotent (re-running it produces no
// further change).
func InjectStandbyBackupTopology(
	cluster *cnpgv1.Cluster,
	standby *pgbackrestApi.StandbyBackupConfiguration,
) {
	serviceName := pgbackrestCommand.StandbyBackupServiceName(cluster.Name)

	if standby.ShouldManageService() {
		ensureManagedHeadlessService(cluster, serviceName)
	}
	if standby.ShouldManageCertificate() {
		ensureServerAltDNSNames(cluster, StandbyBackupServiceDNSNames(serviceName, cluster.Namespace))
	}
}

// StandbyBackupServiceDNSNames returns the in-cluster DNS forms of the standby
// backup service that the server certificate must cover.
func StandbyBackupServiceDNSNames(serviceName, namespace string) []string {
	return []string{
		serviceName,
		serviceName + "." + namespace,
		serviceName + "." + namespace + ".svc",
		serviceName + "." + namespace + ".svc.cluster.local",
	}
}

func ensureManagedHeadlessService(cluster *cnpgv1.Cluster, serviceName string) {
	if cluster.Spec.Managed == nil {
		cluster.Spec.Managed = &cnpgv1.ManagedConfiguration{}
	}
	if cluster.Spec.Managed.Services == nil {
		cluster.Spec.Managed.Services = &cnpgv1.ManagedServices{}
	}

	for i := range cluster.Spec.Managed.Services.Additional {
		if cluster.Spec.Managed.Services.Additional[i].ServiceTemplate.ObjectMeta.Name == serviceName {
			// Already injected: keep the mutation idempotent.
			return
		}
	}

	port := int32(pgbackrestCommand.DefaultServerPort)
	cluster.Spec.Managed.Services.Additional = append(
		cluster.Spec.Managed.Services.Additional,
		cnpgv1.ManagedService{
			// Target the primary so a standby reaches the primary's server.
			SelectorType: cnpgv1.ServiceSelectorTypeRW,
			ServiceTemplate: cnpgv1.ServiceTemplateSpec{
				ObjectMeta: cnpgv1.Metadata{Name: serviceName},
				Spec: corev1.ServiceSpec{
					// Headless so the name resolves directly to the primary pod,
					// reachable on a non-Postgres port.
					ClusterIP: corev1.ClusterIPNone,
					Ports: []corev1.ServicePort{
						{
							Name:       standbyBackupServicePortName,
							Protocol:   corev1.ProtocolTCP,
							Port:       port,
							TargetPort: intstr.FromInt32(port),
						},
					},
				},
			},
		},
	)
}

func ensureServerAltDNSNames(cluster *cnpgv1.Cluster, names []string) {
	if cluster.Spec.Certificates == nil {
		cluster.Spec.Certificates = &cnpgv1.CertificatesConfiguration{}
	}

	existing := make(map[string]struct{}, len(cluster.Spec.Certificates.ServerAltDNSNames))
	for _, name := range cluster.Spec.Certificates.ServerAltDNSNames {
		existing[name] = struct{}{}
	}
	for _, name := range names {
		if _, ok := existing[name]; ok {
			continue
		}
		cluster.Spec.Certificates.ServerAltDNSNames = append(cluster.Spec.Certificates.ServerAltDNSNames, name)
		existing[name] = struct{}{}
	}
}
