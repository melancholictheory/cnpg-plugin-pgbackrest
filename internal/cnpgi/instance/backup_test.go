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

package instance

import (
	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	pgbackrestApi "github.com/operasoftware/cnpg-plugin-pgbackrest/internal/pgbackrest/api"
	pgbackrestCommand "github.com/operasoftware/cnpg-plugin-pgbackrest/internal/pgbackrest/command"
)

var _ = Describe("resolveStandbyTopology", func() {
	const (
		clusterName = "cluster"
		ns          = "test-ns"
		primary     = "cluster-1"
		standby     = "cluster-2"
		pgData      = "/var/lib/postgresql/data/pgdata"
	)

	newCluster := func(currentPrimary string) *cnpgv1.Cluster {
		c := &cnpgv1.Cluster{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: clusterName}}
		c.Status.CurrentPrimary = currentPrimary
		return c
	}

	newImpl := func(instanceName string) BackupServiceImplementation {
		return BackupServiceImplementation{InstanceName: instanceName, PGDataPath: pgData}
	}

	cfg := func(mode pgbackrestApi.BackupStandbyType) *pgbackrestApi.PgbackrestConfiguration {
		return &pgbackrestApi.PgbackrestConfiguration{
			BackupStandby: &pgbackrestApi.StandbyBackupConfiguration{Mode: mode},
		}
	}

	It("returns nil when the feature is disabled", func(ctx SpecContext) {
		topo := newImpl(standby).resolveStandbyTopology(ctx, newCluster(primary), &pgbackrestApi.PgbackrestConfiguration{})
		Expect(topo).To(BeNil())
	})

	It("returns nil (local backup) when this instance is the primary", func(ctx SpecContext) {
		topo := newImpl(primary).resolveStandbyTopology(ctx, newCluster(primary), cfg(pgbackrestApi.BackupStandbyEnabled))
		Expect(topo).To(BeNil())
	})

	It("returns nil when no primary is known yet", func(ctx SpecContext) {
		topo := newImpl(standby).resolveStandbyTopology(ctx, newCluster(""), cfg(pgbackrestApi.BackupStandbyEnabled))
		Expect(topo).To(BeNil())
	})

	It("builds the topology from the standby-backup service when on a standby", func(ctx SpecContext) {
		topo := newImpl(standby).resolveStandbyTopology(ctx, newCluster(primary), cfg(pgbackrestApi.BackupStandbyPrefer))
		Expect(topo).ToNot(BeNil())
		Expect(*topo).To(Equal(pgbackrestCommand.StandbyBackupTopology{
			PrimaryHost:   "cluster-pgbackrest.test-ns",
			PrimaryPort:   pgbackrestCommand.DefaultServerPort,
			PrimaryPGData: pgData,
			Mode:          string(pgbackrestApi.BackupStandbyPrefer),
			CertFile:      pgbackrestCommand.DefaultTLSCertFile,
			KeyFile:       pgbackrestCommand.DefaultTLSKeyFile,
			CAFile:        pgbackrestCommand.DefaultTLSCAFile,
		}))
	})
})
