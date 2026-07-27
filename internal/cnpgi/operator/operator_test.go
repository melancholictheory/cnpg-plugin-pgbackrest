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

package operator

import (
	"encoding/json"

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cnpg-i/pkg/operator"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	pgbackrestv1 "github.com/operasoftware/cnpg-plugin-pgbackrest/api/v1"
	"github.com/operasoftware/cnpg-plugin-pgbackrest/internal/cnpgi/metadata"
	pgbackrestApi "github.com/operasoftware/cnpg-plugin-pgbackrest/internal/pgbackrest/api"
)

var _ = Describe("OperatorImplementation MutateCluster", func() {
	const (
		clusterName = "cluster-a"
		namespace   = "ns-a"
		archiveName = "store-dest"
	)

	buildScheme := func() *runtime.Scheme {
		scheme := runtime.NewScheme()
		Expect(cnpgv1.AddToScheme(scheme)).To(Succeed())
		Expect(pgbackrestv1.AddToScheme(scheme)).To(Succeed())
		return scheme
	}

	newRequest := func() *operator.OperatorMutateClusterRequest {
		cluster := &cnpgv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{Name: clusterName, Namespace: namespace},
			Spec: cnpgv1.ClusterSpec{
				Plugins: []cnpgv1.PluginConfiguration{
					{
						Name:       metadata.PluginName,
						Enabled:    ptr.To(true),
						Parameters: map[string]string{"pgbackrestObjectName": archiveName},
					},
				},
			},
		}
		clusterJSON, err := json.Marshal(cluster)
		Expect(err).ToNot(HaveOccurred())
		return &operator.OperatorMutateClusterRequest{Definition: clusterJSON}
	}

	archiveWith := func(standby *pgbackrestApi.StandbyBackupConfiguration) *pgbackrestv1.Archive {
		return &pgbackrestv1.Archive{
			ObjectMeta: metav1.ObjectMeta{Name: archiveName, Namespace: namespace},
			Spec: pgbackrestv1.ArchiveSpec{
				Configuration: pgbackrestApi.PgbackrestConfiguration{BackupStandby: standby},
			},
		}
	}

	newImpl := func(archive *pgbackrestv1.Archive) OperatorImplementation {
		builder := fake.NewClientBuilder().WithScheme(buildScheme())
		if archive != nil {
			builder = builder.WithObjects(archive)
		}
		return OperatorImplementation{Client: builder.Build()}
	}

	It("advertises only the MutateCluster capability", func(ctx SpecContext) {
		res, err := OperatorImplementation{}.GetCapabilities(ctx, &operator.OperatorCapabilitiesRequest{})
		Expect(err).ToNot(HaveOccurred())
		Expect(res.GetCapabilities()).To(HaveLen(1))
		Expect(res.GetCapabilities()[0].GetRpc().GetType()).
			To(Equal(operator.OperatorCapability_RPC_TYPE_MUTATE_CLUSTER))
	})

	It("patches the cluster with the service and SANs when enabled", func(ctx SpecContext) {
		impl := newImpl(archiveWith(&pgbackrestApi.StandbyBackupConfiguration{Mode: "y"}))
		res, err := impl.MutateCluster(ctx, newRequest())
		Expect(err).ToNot(HaveOccurred())
		patch := string(res.GetJsonPatch())
		Expect(patch).To(ContainSubstring("serverAltDNSNames"))
		Expect(patch).To(ContainSubstring(clusterName + "-pgbackrest"))
	})

	It("returns an empty patch when the feature is disabled", func(ctx SpecContext) {
		impl := newImpl(archiveWith(nil))
		res, err := impl.MutateCluster(ctx, newRequest())
		Expect(err).ToNot(HaveOccurred())
		Expect(res.GetJsonPatch()).To(BeEmpty())
	})

	It("returns an empty patch when the archive is missing", func(ctx SpecContext) {
		impl := newImpl(nil)
		res, err := impl.MutateCluster(ctx, newRequest())
		Expect(err).ToNot(HaveOccurred())
		Expect(res.GetJsonPatch()).To(BeEmpty())
	})
})
