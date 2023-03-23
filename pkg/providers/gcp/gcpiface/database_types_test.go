package gcpiface

import (
	"reflect"
	"testing"

	sqladmin "google.golang.org/api/sqladmin/v1beta4"
	utils "k8s.io/utils/pointer"
)

func TestDatabaseInstance_MapToGcpDatabaseInstance(t *testing.T) {
	type fields struct {
		ConnectionName              string
		DatabaseVersion             string
		DiskEncryptionConfiguration *sqladmin.DiskEncryptionConfiguration
		FailoverReplica             *DatabaseInstanceFailoverReplica
		GceZone                     string
		InstanceType                string
		IpAddresses                 []*sqladmin.IpMapping
		Kind                        string
		MaintenanceVersion          string
		MasterInstanceName          string
		MaxDiskSize                 int64
		Name                        string
		Project                     string
		Region                      string
		ReplicaNames                []string
		RootPassword                string
		SecondaryGceZone            string
		SelfLink                    string
		ServerCaCert                *sqladmin.SslCert
		Settings                    *Settings
	}
	tests := []struct {
		name   string
		fields fields
		want   *sqladmin.DatabaseInstance
	}{
		{
			name: "success converting database struct",
			fields: fields{
				ConnectionName:              "testName",
				DatabaseVersion:             "POSTGRES_13",
				DiskEncryptionConfiguration: &sqladmin.DiskEncryptionConfiguration{},
				FailoverReplica: &DatabaseInstanceFailoverReplica{
					Available: utils.Bool(false),
					Name:      "testName",
				},
				GceZone:      "test",
				InstanceType: "test",
				IpAddresses: []*sqladmin.IpMapping{
					{
						IpAddress: "",
					},
				},
				Kind:               "test",
				MaintenanceVersion: "test",
				MasterInstanceName: "test",
				MaxDiskSize:        100,
				Name:               "testName",
				Project:            "gcp-test-project",
				Region:             "europe-west2",
				ReplicaNames: []string{
					"testName",
				},
				RootPassword:     "password",
				SecondaryGceZone: "test",
				SelfLink:         "test",
				ServerCaCert:     &sqladmin.SslCert{},
				Settings: &Settings{
					ActivationPolicy: "test",
					AvailabilityType: "test",
					BackupConfiguration: &BackupConfiguration{
						BackupRetentionSettings: &BackupRetentionSettings{
							RetentionUnit:   "COUNT",
							RetainedBackups: 30,
						},
						BinaryLogEnabled:               utils.Bool(false),
						Enabled:                        utils.Bool(false),
						Kind:                           "test",
						Location:                       "test",
						PointInTimeRecoveryEnabled:     utils.Bool(true),
						ReplicationLogArchivingEnabled: utils.Bool(false),
						StartTime:                      "test",
						TransactionLogRetentionDays:    1,
					},
					Collation:                   "test",
					ConnectorEnforcement:        "test",
					CrashSafeReplicationEnabled: utils.Bool(false),
					DataDiskSizeGb:              20,
					DataDiskType:                "test",
					DatabaseFlags:               []*sqladmin.DatabaseFlags{},
					DatabaseReplicationEnabled:  utils.Bool(false),
					DeletionProtectionEnabled:   utils.Bool(true),
					DenyMaintenancePeriods:      []*sqladmin.DenyMaintenancePeriod{},
					InsightsConfig:              &sqladmin.InsightsConfig{},
					IpConfiguration: &IpConfiguration{
						AllocatedIpRange:   "test",
						AuthorizedNetworks: []*sqladmin.AclEntry{},
						Ipv4Enabled:        utils.Bool(true),
						PrivateNetwork:     "test",
						RequireSsl:         utils.Bool(true),
					},
					Kind:                     "test",
					LocationPreference:       &sqladmin.LocationPreference{},
					MaintenanceWindow:        &sqladmin.MaintenanceWindow{},
					PasswordValidationPolicy: &sqladmin.PasswordValidationPolicy{},
					PricingPlan:              "test",
					ReplicationType:          "test",
					SettingsVersion:          2,
					StorageAutoResize:        utils.Bool(true),
					StorageAutoResizeLimit:   100,
					Tier:                     "test",
					UserLabels: map[string]string{
						"integreatly-org_clusterid":     "gcp-test-cluster",
						"integreatly-org_resource-name": "testName",
						"integreatly-org_resource-type": "",
						"red-hat-managed":               "true",
					},
				},
			},
			want: &sqladmin.DatabaseInstance{
				ConnectionName:              "testName",
				DatabaseVersion:             "POSTGRES_13",
				DiskEncryptionConfiguration: &sqladmin.DiskEncryptionConfiguration{},
				FailoverReplica: &sqladmin.DatabaseInstanceFailoverReplica{
					Available:       false,
					Name:            "testName",
					ForceSendFields: []string{"Available"},
				},
				GceZone:      "test",
				InstanceType: "test",
				IpAddresses: []*sqladmin.IpMapping{
					{
						IpAddress: "",
					},
				},
				Kind:               "test",
				MaintenanceVersion: "test",
				MasterInstanceName: "test",
				MaxDiskSize:        100,
				Name:               "testName",
				Project:            "gcp-test-project",
				Region:             "europe-west2",
				ReplicaNames: []string{
					"testName",
				},
				RootPassword: "password",
				SelfLink:     "test",
				ServerCaCert: &sqladmin.SslCert{},
				Settings: &sqladmin.Settings{
					ActivationPolicy: "test",
					AvailabilityType: "test",
					BackupConfiguration: &sqladmin.BackupConfiguration{
						BackupRetentionSettings: &sqladmin.BackupRetentionSettings{
							RetainedBackups: 30,
							RetentionUnit:   "COUNT",
						},
						BinaryLogEnabled:               false,
						Enabled:                        false,
						Kind:                           "test",
						Location:                       "test",
						PointInTimeRecoveryEnabled:     true,
						ReplicationLogArchivingEnabled: false,
						StartTime:                      "test",
						TransactionLogRetentionDays:    1,
						ForceSendFields: []string{
							"BinaryLogEnabled",
							"Enabled",
							"ReplicationLogArchivingEnabled",
						},
					},
					Collation:                   "test",
					ConnectorEnforcement:        "test",
					CrashSafeReplicationEnabled: false,
					DataDiskSizeGb:              20,
					DataDiskType:                "test",
					DatabaseFlags:               []*sqladmin.DatabaseFlags{},
					DatabaseReplicationEnabled:  false,
					DeletionProtectionEnabled:   true,
					DenyMaintenancePeriods:      []*sqladmin.DenyMaintenancePeriod{},
					InsightsConfig:              &sqladmin.InsightsConfig{},
					IpConfiguration: &sqladmin.IpConfiguration{
						AllocatedIpRange:   "test",
						AuthorizedNetworks: []*sqladmin.AclEntry{},
						Ipv4Enabled:        true,
						PrivateNetwork:     "test",
						RequireSsl:         true,
					},
					Kind:                     "test",
					LocationPreference:       &sqladmin.LocationPreference{},
					MaintenanceWindow:        &sqladmin.MaintenanceWindow{},
					PasswordValidationPolicy: &sqladmin.PasswordValidationPolicy{},
					PricingPlan:              "test",
					ReplicationType:          "test",
					SettingsVersion:          2,
					StorageAutoResize:        utils.Bool(true),
					StorageAutoResizeLimit:   100,
					Tier:                     "test",
					UserLabels: map[string]string{
						"integreatly-org_clusterid":     "gcp-test-cluster",
						"integreatly-org_resource-name": "testName",
						"integreatly-org_resource-type": "",
						"red-hat-managed":               "true",
					},
					ForceSendFields: []string{
						"CrashSafeReplicationEnabled",
						"DatabaseReplicationEnabled",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dbi := &DatabaseInstance{
				ConnectionName:              tt.fields.ConnectionName,
				DatabaseVersion:             tt.fields.DatabaseVersion,
				DiskEncryptionConfiguration: tt.fields.DiskEncryptionConfiguration,
				FailoverReplica:             tt.fields.FailoverReplica,
				GceZone:                     tt.fields.GceZone,
				InstanceType:                tt.fields.InstanceType,
				IpAddresses:                 tt.fields.IpAddresses,
				Kind:                        tt.fields.Kind,
				MaintenanceVersion:          tt.fields.MaintenanceVersion,
				MasterInstanceName:          tt.fields.MasterInstanceName,
				MaxDiskSize:                 tt.fields.MaxDiskSize,
				Name:                        tt.fields.Name,
				Project:                     tt.fields.Project,
				Region:                      tt.fields.Region,
				ReplicaNames:                tt.fields.ReplicaNames,
				RootPassword:                tt.fields.RootPassword,
				SecondaryGceZone:            tt.fields.SecondaryGceZone,
				SelfLink:                    tt.fields.SelfLink,
				ServerCaCert:                tt.fields.ServerCaCert,
				Settings:                    tt.fields.Settings,
			}
			if got := dbi.MapToGcpDatabaseInstance(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MapToGcpDatabaseInstance() = %v, want %v", got, tt.want)
			}
		})
	}
}
