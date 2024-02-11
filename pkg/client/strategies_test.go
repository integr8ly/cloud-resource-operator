package client

import (
	"context"
	"reflect"
	"testing"
	"time"

	moqClient "github.com/integr8ly/cloud-resource-operator/pkg/client/fake"
	stratType "github.com/integr8ly/cloud-resource-operator/pkg/client/types"
	croAWS "github.com/integr8ly/cloud-resource-operator/pkg/providers/aws"
	croGCP "github.com/integr8ly/cloud-resource-operator/pkg/providers/gcp"
	configv1 "github.com/openshift/api/config/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	testNamespace = "testNamespace"
	testTierName  = "production"
)

func buildDefaultConfigMap(platformType configv1.PlatformType) *v1.ConfigMap {
	switch platformType {
	case configv1.AWSPlatformType:
		return &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      croAWS.DefaultConfigMapName,
				Namespace: testNamespace,
			},
			Data: map[string]string{
				"blobstorage": `{"development": { "region": "", "createStrategy": {}, "deleteStrategy": {} }, "production": { "region": "", "createStrategy": {}, "deleteStrategy": {} }}`,
				"redis":       `{"development":{"region":"","createStrategy":{},"deleteStrategy":{},"serviceUpdates":null},"production":{"region":"","createStrategy":{"AtRestEncryptionEnabled":null,"AuthToken":null,"AutoMinorVersionUpgrade":null,"AutomaticFailoverEnabled":null,"CacheNodeType":null,"CacheParameterGroupName":null,"CacheSecurityGroupNames":null,"CacheSubnetGroupName":null,"DataTieringEnabled":null,"Engine":null,"EngineVersion":null,"KmsKeyId":null,"NodeGroupConfiguration":null,"NotificationTopicArn":null,"NumCacheClusters":null,"NumNodeGroups":null,"Port":null,"PreferredCacheClusterAZs":null,"PreferredMaintenanceWindow":"sun:18:05-sun:19:05","PrimaryClusterId":null,"ReplicasPerNodeGroup":null,"ReplicationGroupDescription":null,"ReplicationGroupId":null,"SecurityGroupIds":null,"SnapshotArns":null,"SnapshotName":null,"SnapshotRetentionLimit":null,"SnapshotWindow":"16:04-17:04","Tags":null,"TransitEncryptionEnabled":null},"deleteStrategy":{},"serviceUpdates":null}}`,
				"postgres":    `{"development":{"region":"","createStrategy":{},"deleteStrategy":{},"serviceUpdates":null},"production":{"region":"","createStrategy":{"AllocatedStorage":null,"AutoMinorVersionUpgrade":null,"AvailabilityZone":null,"BackupRetentionPeriod":null,"BackupTarget":null,"CharacterSetName":null,"CopyTagsToSnapshot":null,"CustomIamInstanceProfile":null,"DBClusterIdentifier":null,"DBInstanceClass":null,"DBInstanceIdentifier":null,"DBName":null,"DBParameterGroupName":null,"DBSecurityGroups":null,"DBSubnetGroupName":null,"DeletionProtection":null,"Domain":null,"DomainIAMRoleName":null,"EnableCloudwatchLogsExports":null,"EnableIAMDatabaseAuthentication":null,"EnablePerformanceInsights":null,"Engine":null,"EngineVersion":null,"Iops":null,"KmsKeyId":null,"LicenseModel":null,"MasterUserPassword":null,"MasterUsername":null,"MaxAllocatedStorage":null,"MonitoringInterval":null,"MonitoringRoleArn":null,"MultiAZ":null,"OptionGroupName":null,"PerformanceInsightsKMSKeyId":null,"PerformanceInsightsRetentionPeriod":null,"Port":null,"PreferredBackupWindow":"16:04-17:04","PreferredMaintenanceWindow":"sun:18:05-sun:19:05","ProcessorFeatures":null,"PromotionTier":null,"PubliclyAccessible":null,"StorageEncrypted":null,"StorageType":null,"Tags":null,"TdeCredentialArn":null,"TdeCredentialPassword":null,"Timezone":null,"VpcSecurityGroupIds":null},"deleteStrategy":{},"serviceUpdates":null}}`,
			},
		}
	case configv1.GCPPlatformType:
		return &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      croGCP.DefaultConfigMapName,
				Namespace: testNamespace,
			},
			Data: map[string]string{
				"blobstorage": `{"development": { "region": "", "createStrategy": {}, "deleteStrategy": {} }, "production": { "region": "", "createStrategy": {}, "deleteStrategy": {} }}`,
				"redis":       `{"development":{"region":"","projectID":"","createStrategy":{},"deleteStrategy":{}},"production":{"region":"","projectID":"","createStrategy":{"instance":{"maintenance_policy":{"weekly_maintenance_window":[{"day":7,"start_time":{"hours":18,"minutes":5}}]}}},"deleteStrategy":{}}}`,
				"postgres":    `{"development":{"region":"","projectID":"","createStrategy":{},"deleteStrategy":{}},"production":{"region":"","projectID":"","createStrategy":{"instance":{"settings":{"backupConfiguration":{"startTime":"16:04"},"maintenanceWindow":{"day":7,"hour":18}}}},"deleteStrategy":{}}}`,
			},
		}
	}
	return nil
}

func buildExpectRedisStrat(platformType configv1.PlatformType) string {
	switch platformType {
	case configv1.AWSPlatformType:
		return `{"development":{"region":"","createStrategy":{},"deleteStrategy":{},"serviceUpdates":null},"production":{"region":"","createStrategy":{"AtRestEncryptionEnabled":null,"AuthToken":null,"AutoMinorVersionUpgrade":null,"AutomaticFailoverEnabled":null,"CacheNodeType":null,"CacheParameterGroupName":null,"CacheSecurityGroupNames":null,"CacheSubnetGroupName":null,"DataTieringEnabled":null,"Engine":null,"EngineVersion":null,"GlobalReplicationGroupId":null,"IpDiscovery":null,"KmsKeyId":null,"LogDeliveryConfigurations":null,"MultiAZEnabled":null,"NetworkType":null,"NodeGroupConfiguration":null,"NotificationTopicArn":null,"NumCacheClusters":null,"NumNodeGroups":null,"Port":null,"PreferredCacheClusterAZs":null,"PreferredMaintenanceWindow":"mon:16:05-mon:17:05","PrimaryClusterId":null,"ReplicasPerNodeGroup":null,"ReplicationGroupDescription":null,"ReplicationGroupId":null,"SecurityGroupIds":null,"SnapshotArns":null,"SnapshotName":null,"SnapshotRetentionLimit":null,"SnapshotWindow":"15:04-16:04","Tags":null,"TransitEncryptionEnabled":null,"TransitEncryptionMode":null,"UserGroupIds":null},"deleteStrategy":{},"serviceUpdates":null}}`
	case configv1.GCPPlatformType:
		return `{"development":{"region":"","projectID":"","createStrategy":{},"deleteStrategy":{}},"production":{"region":"","projectID":"","createStrategy":{"instance":{"maintenance_policy":{"weekly_maintenance_window":[{"day":1,"start_time":{"hours":16,"minutes":5}}]}}},"deleteStrategy":{}}}`
	}
	return ""
}

func buildExpectPostgresStrat(platformType configv1.PlatformType) string {
	switch platformType {
	case configv1.AWSPlatformType:
		return `{"development":{"region":"","createStrategy":{},"deleteStrategy":{},"serviceUpdates":null},"production":{"region":"","createStrategy":{"AllocatedStorage":null,"AutoMinorVersionUpgrade":null,"AvailabilityZone":null,"BackupRetentionPeriod":null,"BackupTarget":null,"CACertificateIdentifier":null,"CharacterSetName":null,"CopyTagsToSnapshot":null,"CustomIamInstanceProfile":null,"DBClusterIdentifier":null,"DBInstanceClass":null,"DBInstanceIdentifier":null,"DBName":null,"DBParameterGroupName":null,"DBSecurityGroups":null,"DBSubnetGroupName":null,"DeletionProtection":null,"Domain":null,"DomainIAMRoleName":null,"EnableCloudwatchLogsExports":null,"EnableCustomerOwnedIp":null,"EnableIAMDatabaseAuthentication":null,"EnablePerformanceInsights":null,"Engine":null,"EngineVersion":null,"Iops":null,"KmsKeyId":null,"LicenseModel":null,"ManageMasterUserPassword":null,"MasterUserPassword":null,"MasterUserSecretKmsKeyId":null,"MasterUsername":null,"MaxAllocatedStorage":null,"MonitoringInterval":null,"MonitoringRoleArn":null,"MultiAZ":null,"NcharCharacterSetName":null,"NetworkType":null,"OptionGroupName":null,"PerformanceInsightsKMSKeyId":null,"PerformanceInsightsRetentionPeriod":null,"Port":null,"PreferredBackupWindow":"15:04-16:04","PreferredMaintenanceWindow":"mon:16:05-mon:17:05","ProcessorFeatures":null,"PromotionTier":null,"PubliclyAccessible":null,"StorageEncrypted":null,"StorageThroughput":null,"StorageType":null,"Tags":null,"TdeCredentialArn":null,"TdeCredentialPassword":null,"Timezone":null,"VpcSecurityGroupIds":null},"deleteStrategy":{},"serviceUpdates":null}}`
	case configv1.GCPPlatformType:
		return `{"development":{"region":"","projectID":"","createStrategy":{},"deleteStrategy":{}},"production":{"region":"","projectID":"","createStrategy":{"instance":{"settings":{"backupConfiguration":{"startTime":"15:04"},"maintenanceWindow":{"day":1,"hour":16}}}},"deleteStrategy":{}}}`
	}
	return ""
}

func buildInfrastructureType(platformType configv1.PlatformType) *configv1.Infrastructure {
	return &configv1.Infrastructure{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cluster",
		},
		Status: configv1.InfrastructureStatus{
			PlatformStatus: &configv1.PlatformStatus{
				Type: platformType,
			},
		},
	}
}

func TestReconcileStrategyMaps(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	type args struct {
		ctx        context.Context
		client     client.Client
		timeConfig *stratType.StrategyTimeConfig
		tier       string
		namespace  string
	}
	tests := []struct {
		name          string
		args          args
		getConfigSpec func(ctx context.Context, c client.Client) (interface{}, error)
		want          interface{}
		wantErr       bool
	}{
		{
			name: "aws strategy config map redis is created successfully",
			args: args{
				ctx:        context.TODO(),
				client:     moqClient.NewSigsClientMoqWithScheme(scheme, buildInfrastructureType(configv1.AWSPlatformType)),
				timeConfig: stratType.NewStrategyTimeConfig(15, 04, time.Monday, 16, 05),
				tier:       testTierName,
				namespace:  testNamespace,
			},
			wantErr: false,
			getConfigSpec: func(ctx context.Context, c client.Client) (interface{}, error) {
				config := &v1.ConfigMap{}
				err := c.Get(ctx, types.NamespacedName{Name: croAWS.DefaultConfigMapName, Namespace: testNamespace}, config)
				return config.Data[stratType.RedisStratKey], err
			},
			want: buildExpectRedisStrat(configv1.AWSPlatformType),
		},
		{
			name: "aws strategy config map redis is updated successfully",
			args: args{
				ctx:        context.TODO(),
				client:     moqClient.NewSigsClientMoqWithScheme(scheme, buildDefaultConfigMap(configv1.AWSPlatformType), buildInfrastructureType(configv1.AWSPlatformType)),
				timeConfig: stratType.NewStrategyTimeConfig(15, 04, time.Monday, 16, 05),
				tier:       testTierName,
				namespace:  testNamespace,
			},
			wantErr: false,
			getConfigSpec: func(ctx context.Context, c client.Client) (interface{}, error) {
				config := &v1.ConfigMap{}
				err := c.Get(ctx, types.NamespacedName{Name: croAWS.DefaultConfigMapName, Namespace: testNamespace}, config)
				return config.Data[stratType.RedisStratKey], err
			},
			want: buildExpectRedisStrat(configv1.AWSPlatformType),
		},
		{
			name: "aws strategy config map postgres is created successfully",
			args: args{
				ctx:        context.TODO(),
				client:     moqClient.NewSigsClientMoqWithScheme(scheme, buildInfrastructureType(configv1.AWSPlatformType)),
				timeConfig: stratType.NewStrategyTimeConfig(15, 04, time.Monday, 16, 05),
				tier:       testTierName,
				namespace:  testNamespace,
			},
			wantErr: false,
			getConfigSpec: func(ctx context.Context, c client.Client) (interface{}, error) {
				config := &v1.ConfigMap{}
				err := c.Get(ctx, types.NamespacedName{Name: croAWS.DefaultConfigMapName, Namespace: testNamespace}, config)
				return config.Data[stratType.PostgresStratKey], err
			},
			want: buildExpectPostgresStrat(configv1.AWSPlatformType),
		},
		{
			name: "aws strategy config map postgres is updated successfully",
			args: args{
				ctx:        context.TODO(),
				client:     moqClient.NewSigsClientMoqWithScheme(scheme, buildDefaultConfigMap(configv1.AWSPlatformType), buildInfrastructureType(configv1.AWSPlatformType)),
				timeConfig: stratType.NewStrategyTimeConfig(15, 04, time.Monday, 16, 05),
				tier:       testTierName,
				namespace:  testNamespace,
			},
			wantErr: false,
			getConfigSpec: func(ctx context.Context, c client.Client) (interface{}, error) {
				config := &v1.ConfigMap{}
				err := c.Get(ctx, types.NamespacedName{Name: croAWS.DefaultConfigMapName, Namespace: testNamespace}, config)
				return config.Data[stratType.PostgresStratKey], err
			},
			want: buildExpectPostgresStrat(configv1.AWSPlatformType),
		},
		{
			name: "gcp strategy config map redis is created successfully",
			args: args{
				ctx:        context.TODO(),
				client:     moqClient.NewSigsClientMoqWithScheme(scheme, buildInfrastructureType(configv1.GCPPlatformType)),
				timeConfig: stratType.NewStrategyTimeConfig(15, 04, time.Monday, 16, 05),
				tier:       testTierName,
				namespace:  testNamespace,
			},
			wantErr: false,
			getConfigSpec: func(ctx context.Context, c client.Client) (interface{}, error) {
				config := &v1.ConfigMap{}
				err := c.Get(ctx, types.NamespacedName{Name: croGCP.DefaultConfigMapName, Namespace: testNamespace}, config)
				return config.Data[stratType.RedisStratKey], err
			},
			want: buildExpectRedisStrat(configv1.GCPPlatformType),
		},
		{
			name: "gcp strategy config map redis is updated successfully",
			args: args{
				ctx:        context.TODO(),
				client:     moqClient.NewSigsClientMoqWithScheme(scheme, buildDefaultConfigMap(configv1.GCPPlatformType), buildInfrastructureType(configv1.GCPPlatformType)),
				timeConfig: stratType.NewStrategyTimeConfig(15, 04, time.Monday, 16, 05),
				tier:       testTierName,
				namespace:  testNamespace,
			},
			wantErr: false,
			getConfigSpec: func(ctx context.Context, c client.Client) (interface{}, error) {
				config := &v1.ConfigMap{}
				err := c.Get(ctx, types.NamespacedName{Name: croGCP.DefaultConfigMapName, Namespace: testNamespace}, config)
				return config.Data[stratType.RedisStratKey], err
			},
			want: buildExpectRedisStrat(configv1.GCPPlatformType),
		},
		{
			name: "gcp strategy config map postgres is created successfully",
			args: args{
				ctx:        context.TODO(),
				client:     moqClient.NewSigsClientMoqWithScheme(scheme, buildInfrastructureType(configv1.GCPPlatformType)),
				timeConfig: stratType.NewStrategyTimeConfig(15, 04, time.Monday, 16, 05),
				tier:       testTierName,
				namespace:  testNamespace,
			},
			wantErr: false,
			getConfigSpec: func(ctx context.Context, c client.Client) (interface{}, error) {
				config := &v1.ConfigMap{}
				err := c.Get(ctx, types.NamespacedName{Name: croGCP.DefaultConfigMapName, Namespace: testNamespace}, config)
				return config.Data[stratType.PostgresStratKey], err
			},
			want: buildExpectPostgresStrat(configv1.GCPPlatformType),
		},
		{
			name: "gcp strategy config map postgres is updated successfully",
			args: args{
				ctx:        context.TODO(),
				client:     moqClient.NewSigsClientMoqWithScheme(scheme, buildDefaultConfigMap(configv1.GCPPlatformType), buildInfrastructureType(configv1.GCPPlatformType)),
				timeConfig: stratType.NewStrategyTimeConfig(15, 04, time.Monday, 16, 05),
				tier:       testTierName,
				namespace:  testNamespace,
			},
			wantErr: false,
			getConfigSpec: func(ctx context.Context, c client.Client) (interface{}, error) {
				config := &v1.ConfigMap{}
				err := c.Get(ctx, types.NamespacedName{Name: croGCP.DefaultConfigMapName, Namespace: testNamespace}, config)
				return config.Data[stratType.PostgresStratKey], err
			},
			want: buildExpectPostgresStrat(configv1.GCPPlatformType),
		},
		{
			name: "error retrieving cluster platform type",
			args: args{
				ctx:        context.TODO(),
				client:     moqClient.NewSigsClientMoqWithScheme(scheme),
				timeConfig: stratType.NewStrategyTimeConfig(15, 04, time.Monday, 16, 05),
				tier:       testTierName,
				namespace:  testNamespace,
			},
			wantErr:       true,
			getConfigSpec: nil,
			want:          nil,
		},
		{
			name: "error unsupported platform type",
			args: args{
				ctx:        context.TODO(),
				client:     moqClient.NewSigsClientMoqWithScheme(scheme, buildInfrastructureType(configv1.AzurePlatformType)),
				timeConfig: stratType.NewStrategyTimeConfig(15, 04, time.Monday, 16, 05),
				tier:       testTierName,
				namespace:  testNamespace,
			},
			wantErr:       true,
			getConfigSpec: nil,
			want:          nil,
		},
		{
			name: "error reconciling aws strategy map, overlapping windows",
			args: args{
				ctx:        context.TODO(),
				client:     moqClient.NewSigsClientMoqWithScheme(scheme, buildInfrastructureType(configv1.AWSPlatformType)),
				timeConfig: stratType.NewStrategyTimeConfig(15, 04, time.Monday, 15, 05),
				tier:       testTierName,
				namespace:  testNamespace,
			},
			wantErr: true,
			getConfigSpec: func(ctx context.Context, c client.Client) (interface{}, error) {
				config := &v1.ConfigMap{}
				err := c.Get(ctx, types.NamespacedName{Name: croAWS.DefaultConfigMapName, Namespace: testNamespace}, config)
				return config.Data[stratType.PostgresStratKey], err
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ReconcileStrategyMaps(tt.args.ctx, tt.args.client, tt.args.timeConfig, tt.args.tier, tt.args.namespace)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ReconcileStrategyMaps() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			got, err := tt.getConfigSpec(tt.args.ctx, tt.args.client)
			if err != nil {
				t.Fatal("ReconcileStrategyMaps() unexpected error while getting testable config ", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ReconcileStrategyMaps() \n got = %+v, \n want = %+v", got, tt.want)
			}
		})
	}
}
