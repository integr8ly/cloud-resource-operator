package client

import (
	"context"
	croAWS "github.com/integr8ly/cloud-resource-operator/pkg/providers/aws"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"testing"
)

const (
	testNamespace = "testNamespace"
	testTierName  = "production"
)

func buildDefaultConfigMap() *v1.ConfigMap {
	return &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      croAWS.DefaultConfigMapName,
			Namespace: testNamespace,
		},
		Data: map[string]string{
			"blobstorage": "{\"development\": { \"region\": \"\", \"createStrategy\": {}, \"deleteStrategy\": {} }, \"production\": { \"region\": \"\", \"createStrategy\": {}, \"deleteStrategy\": {} }}",
			"redis":       "{\"development\":{\"region\":\"\",\"createStrategy\":{},\"deleteStrategy\":{}},\"production\":{\"region\":\"\",\"createStrategy\":{\"AtRestEncryptionEnabled\":null,\"AuthToken\":null,\"AutoMinorVersionUpgrade\":null,\"AutomaticFailoverEnabled\":null,\"CacheNodeType\":null,\"CacheParameterGroupName\":null,\"CacheSecurityGroupNames\":null,\"CacheSubnetGroupName\":null,\"Engine\":null,\"EngineVersion\":null,\"KmsKeyId\":null,\"NodeGroupConfiguration\":null,\"NotificationTopicArn\":null,\"NumCacheClusters\":null,\"NumNodeGroups\":null,\"Port\":null,\"PreferredCacheClusterAZs\":null,\"PreferredMaintenanceWindow\":\"sun:18:05-sun:19:05\",\"PrimaryClusterId\":null,\"ReplicasPerNodeGroup\":null,\"ReplicationGroupDescription\":null,\"ReplicationGroupId\":null,\"SecurityGroupIds\":null,\"SnapshotArns\":null,\"SnapshotName\":null,\"SnapshotRetentionLimit\":null,\"SnapshotWindow\":\"16:04-17:04\",\"Tags\":null,\"TransitEncryptionEnabled\":null},\"deleteStrategy\":{}}}",
			"postgres":    "{\"development\":{\"region\":\"\",\"createStrategy\":{},\"deleteStrategy\":{}},\"production\":{\"region\":\"\",\"createStrategy\":{\"AllocatedStorage\":null,\"AutoMinorVersionUpgrade\":null,\"AvailabilityZone\":null,\"BackupRetentionPeriod\":null,\"CharacterSetName\":null,\"CopyTagsToSnapshot\":null,\"DBClusterIdentifier\":null,\"DBInstanceClass\":null,\"DBInstanceIdentifier\":null,\"DBName\":null,\"DBParameterGroupName\":null,\"DBSecurityGroups\":null,\"DBSubnetGroupName\":null,\"DeletionProtection\":null,\"Domain\":null,\"DomainIAMRoleName\":null,\"EnableCloudwatchLogsExports\":null,\"EnableIAMDatabaseAuthentication\":null,\"EnablePerformanceInsights\":null,\"Engine\":null,\"EngineVersion\":null,\"Iops\":null,\"KmsKeyId\":null,\"LicenseModel\":null,\"MasterUserPassword\":null,\"MasterUsername\":null,\"MaxAllocatedStorage\":null,\"MonitoringInterval\":null,\"MonitoringRoleArn\":null,\"MultiAZ\":null,\"OptionGroupName\":null,\"PerformanceInsightsKMSKeyId\":null,\"PerformanceInsightsRetentionPeriod\":null,\"Port\":null,\"PreferredBackupWindow\":\"16:04-17:04\",\"PreferredMaintenanceWindow\":\"sun:18:05-sun:19:05\",\"ProcessorFeatures\":null,\"PromotionTier\":null,\"PubliclyAccessible\":null,\"StorageEncrypted\":null,\"StorageType\":null,\"Tags\":null,\"TdeCredentialArn\":null,\"TdeCredentialPassword\":null,\"Timezone\":null,\"VpcSecurityGroupIds\":null},\"deleteStrategy\":{}}}",
		},
	}
}

func buildExpectRedisStrat() string {
	return "{\"development\":{\"region\":\"\",\"createStrategy\":{},\"deleteStrategy\":{}},\"production\":{\"region\":\"\",\"createStrategy\":{\"AtRestEncryptionEnabled\":null,\"AuthToken\":null,\"AutoMinorVersionUpgrade\":null,\"AutomaticFailoverEnabled\":null,\"CacheNodeType\":null,\"CacheParameterGroupName\":null,\"CacheSecurityGroupNames\":null,\"CacheSubnetGroupName\":null,\"Engine\":null,\"EngineVersion\":null,\"KmsKeyId\":null,\"NodeGroupConfiguration\":null,\"NotificationTopicArn\":null,\"NumCacheClusters\":null,\"NumNodeGroups\":null,\"Port\":null,\"PreferredCacheClusterAZs\":null,\"PreferredMaintenanceWindow\":\"mon:16:05-mon:17:05\",\"PrimaryClusterId\":null,\"ReplicasPerNodeGroup\":null,\"ReplicationGroupDescription\":null,\"ReplicationGroupId\":null,\"SecurityGroupIds\":null,\"SnapshotArns\":null,\"SnapshotName\":null,\"SnapshotRetentionLimit\":null,\"SnapshotWindow\":\"15:04-16:04\",\"Tags\":null,\"TransitEncryptionEnabled\":null},\"deleteStrategy\":{}}}"
}

func buildExpectPostgresStrat() string {
	return "{\"development\":{\"region\":\"\",\"createStrategy\":{},\"deleteStrategy\":{}},\"production\":{\"region\":\"\",\"createStrategy\":{\"AllocatedStorage\":null,\"AutoMinorVersionUpgrade\":null,\"AvailabilityZone\":null,\"BackupRetentionPeriod\":null,\"CharacterSetName\":null,\"CopyTagsToSnapshot\":null,\"DBClusterIdentifier\":null,\"DBInstanceClass\":null,\"DBInstanceIdentifier\":null,\"DBName\":null,\"DBParameterGroupName\":null,\"DBSecurityGroups\":null,\"DBSubnetGroupName\":null,\"DeletionProtection\":null,\"Domain\":null,\"DomainIAMRoleName\":null,\"EnableCloudwatchLogsExports\":null,\"EnableIAMDatabaseAuthentication\":null,\"EnablePerformanceInsights\":null,\"Engine\":null,\"EngineVersion\":null,\"Iops\":null,\"KmsKeyId\":null,\"LicenseModel\":null,\"MasterUserPassword\":null,\"MasterUsername\":null,\"MaxAllocatedStorage\":null,\"MonitoringInterval\":null,\"MonitoringRoleArn\":null,\"MultiAZ\":null,\"OptionGroupName\":null,\"PerformanceInsightsKMSKeyId\":null,\"PerformanceInsightsRetentionPeriod\":null,\"Port\":null,\"PreferredBackupWindow\":\"15:04-16:04\",\"PreferredMaintenanceWindow\":\"mon:16:05-mon:17:05\",\"ProcessorFeatures\":null,\"PromotionTier\":null,\"PubliclyAccessible\":null,\"StorageEncrypted\":null,\"StorageType\":null,\"Tags\":null,\"TdeCredentialArn\":null,\"TdeCredentialPassword\":null,\"Timezone\":null,\"VpcSecurityGroupIds\":null},\"deleteStrategy\":{}}}"
}

func TestReconcileStrategyMaps(t *testing.T) {
	scheme, err := buildTestScheme()
	if err != nil {
		t.Fatal("failed to build scheme", err)
	}
	type args struct {
		ctx        context.Context
		client     client.Client
		timeConfig *StrategyTimeConfig
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
				ctx:    context.TODO(),
				client: fake.NewFakeClientWithScheme(scheme),
				timeConfig: &StrategyTimeConfig{
					BackupStartTime:      "15:04",
					MaintenanceStartTime: "Mon 16:05",
				},
				tier:      testTierName,
				namespace: testNamespace,
			},
			wantErr: false,
			getConfigSpec: func(ctx context.Context, c client.Client) (interface{}, error) {
				config := &v1.ConfigMap{}
				err := c.Get(ctx, types.NamespacedName{Name: croAWS.DefaultConfigMapName, Namespace: testNamespace}, config)
				return config.Data["redis"], err
			},
			want: buildExpectRedisStrat(),
		},
		{
			name: "aws strategy config map redis is updated successfully",
			args: args{
				ctx:    context.TODO(),
				client: fake.NewFakeClientWithScheme(scheme, buildDefaultConfigMap()),
				timeConfig: &StrategyTimeConfig{
					BackupStartTime:      "15:04",
					MaintenanceStartTime: "Mon 16:05",
				},
				tier:      testTierName,
				namespace: testNamespace,
			},
			wantErr: false,
			getConfigSpec: func(ctx context.Context, c client.Client) (interface{}, error) {
				config := &v1.ConfigMap{}
				err := c.Get(ctx, types.NamespacedName{Name: croAWS.DefaultConfigMapName, Namespace: testNamespace}, config)
				return config.Data["redis"], err
			},
			want: buildExpectRedisStrat(),
		},
		{
			name: "aws strategy config map postgres is created successfully",
			args: args{
				ctx:    context.TODO(),
				client: fake.NewFakeClientWithScheme(scheme),
				timeConfig: &StrategyTimeConfig{
					BackupStartTime:      "15:04",
					MaintenanceStartTime: "Mon 16:05",
				},
				tier:      testTierName,
				namespace: testNamespace,
			},
			wantErr: false,
			getConfigSpec: func(ctx context.Context, c client.Client) (interface{}, error) {
				config := &v1.ConfigMap{}
				err := c.Get(ctx, types.NamespacedName{Name: croAWS.DefaultConfigMapName, Namespace: testNamespace}, config)
				return config.Data["postgres"], err
			},
			want: buildExpectPostgresStrat(),
		},
		{
			name: "aws strategy config map postgres is updated successfully",
			args: args{
				ctx:    context.TODO(),
				client: fake.NewFakeClientWithScheme(scheme, buildDefaultConfigMap()),
				timeConfig: &StrategyTimeConfig{
					BackupStartTime:      "15:04",
					MaintenanceStartTime: "Mon 16:05",
				},
				tier:      testTierName,
				namespace: testNamespace,
			},
			wantErr: false,
			getConfigSpec: func(ctx context.Context, c client.Client) (interface{}, error) {
				config := &v1.ConfigMap{}
				err := c.Get(ctx, types.NamespacedName{Name: croAWS.DefaultConfigMapName, Namespace: testNamespace}, config)
				return config.Data["postgres"], err
			},
			want: buildExpectPostgresStrat(),
		},
		{
			name: "aws strategy config map check backup start time parsing fails",
			args: args{
				ctx:    context.TODO(),
				client: fake.NewFakeClientWithScheme(scheme),
				timeConfig: &StrategyTimeConfig{
					BackupStartTime:      "I am the wrong format",
					MaintenanceStartTime: "Mon 16:05",
				},
				tier:      testTierName,
				namespace: testNamespace,
			},
			wantErr: true,
			want:    nil,
			getConfigSpec: func(ctx context.Context, c client.Client) (interface{}, error) {
				return nil, err
			},
		},
		{
			name: "aws strategy config map check maintenance start time parsing fails",
			args: args{
				ctx:    context.TODO(),
				client: fake.NewFakeClientWithScheme(scheme),
				timeConfig: &StrategyTimeConfig{
					BackupStartTime:      "15:04",
					MaintenanceStartTime: "I am the wrong format",
				},
				tier:      testTierName,
				namespace: testNamespace,
			},
			wantErr: true,
			want:    nil,
			getConfigSpec: func(ctx context.Context, c client.Client) (interface{}, error) {
				return nil, err
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ReconcileStrategyMaps(tt.args.ctx, tt.args.client, tt.args.timeConfig, tt.args.tier, tt.args.namespace)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReconcileStrategyMaps() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			got, err := tt.getConfigSpec(tt.args.ctx, tt.args.client)
			if err != nil {
				t.Error("ReconcileStrategyMaps() unexpected error while getting testable config ", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReconcileStrategyMaps() \n got = %+v, \n want = %+v", got, tt.want)
			}
		})
	}
}
