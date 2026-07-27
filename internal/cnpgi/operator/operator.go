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
	"context"

	cnpgv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cnpg-i-machinery/pkg/pluginhelper/decoder"
	"github.com/cloudnative-pg/cnpg-i-machinery/pkg/pluginhelper/object"
	"github.com/cloudnative-pg/cnpg-i/pkg/operator"
	"github.com/cloudnative-pg/machinery/pkg/log"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	pgbackrestv1 "github.com/operasoftware/cnpg-plugin-pgbackrest/api/v1"
	"github.com/operasoftware/cnpg-plugin-pgbackrest/internal/cnpgi/operator/config"
	"github.com/operasoftware/cnpg-plugin-pgbackrest/internal/cnpgi/operator/specs"
	pgbackrestApi "github.com/operasoftware/cnpg-plugin-pgbackrest/internal/pgbackrest/api"
)

// OperatorImplementation implements the Operator capability. It mutates the
// Cluster to add the resources backup-from-standby needs.
type OperatorImplementation struct {
	Client client.Client
	operator.UnimplementedOperatorServer
}

// GetCapabilities advertises that the plugin mutates the Cluster.
func (o OperatorImplementation) GetCapabilities(
	_ context.Context,
	_ *operator.OperatorCapabilitiesRequest,
) (*operator.OperatorCapabilitiesResult, error) {
	return &operator.OperatorCapabilitiesResult{
		Capabilities: []*operator.OperatorCapability{
			{
				Type: &operator.OperatorCapability_Rpc{
					Rpc: &operator.OperatorCapability_RPC{
						Type: operator.OperatorCapability_RPC_TYPE_MUTATE_CLUSTER,
					},
				},
			},
		},
	}, nil
}

// MutateCluster injects, when backup-from-standby is enabled on the referred
// Archive, a headless service that reaches the current primary's pgBackRest
// server and the matching certificate alternative name. The returned JSON patch
// is empty when the feature is off or the cluster already carries the changes.
func (o OperatorImplementation) MutateCluster(
	ctx context.Context,
	request *operator.OperatorMutateClusterRequest,
) (*operator.OperatorMutateClusterResult, error) {
	contextLogger := log.FromContext(ctx)

	var cluster cnpgv1.Cluster
	if err := decoder.DecodeObjectLenient(request.GetDefinition(), &cluster); err != nil {
		return nil, err
	}

	standby, err := o.resolveStandbyConfiguration(ctx, &cluster)
	if err != nil {
		return nil, err
	}
	if standby == nil {
		return &operator.OperatorMutateClusterResult{}, nil
	}

	mutated := cluster.DeepCopy()
	specs.InjectStandbyBackupTopology(mutated, standby)

	patch, err := object.CreatePatch(mutated, &cluster)
	if err != nil {
		return nil, err
	}
	if len(patch) > 0 {
		contextLogger.Info("Injecting backup-from-standby service and certificate SAN into cluster",
			"name", cluster.Name, "namespace", cluster.Namespace)
	}

	return &operator.OperatorMutateClusterResult{JsonPatch: patch}, nil
}

// resolveStandbyConfiguration returns the backup-from-standby configuration of
// the Archive referred to by the cluster, or nil when the cluster does not use
// the plugin, the Archive is missing, or the feature is disabled.
func (o OperatorImplementation) resolveStandbyConfiguration(
	ctx context.Context,
	cluster *cnpgv1.Cluster,
) (*pgbackrestApi.StandbyBackupConfiguration, error) {
	contextLogger := log.FromContext(ctx)

	pluginConfiguration := config.NewFromCluster(cluster)
	if len(pluginConfiguration.PgbackrestObjectName) == 0 {
		return nil, nil
	}

	var archive pgbackrestv1.Archive
	if err := o.Client.Get(ctx, pluginConfiguration.GetArchiveObjectKey(), &archive); err != nil {
		if apierrs.IsNotFound(err) {
			contextLogger.Debug("archive object not found while mutating cluster",
				"name", pluginConfiguration.PgbackrestObjectName)
			return nil, nil
		}
		return nil, err
	}

	return archive.Spec.Configuration.BackupStandby, nil
}
