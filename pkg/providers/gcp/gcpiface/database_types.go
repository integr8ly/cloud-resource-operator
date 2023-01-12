package gcpiface

import sqladmin "google.golang.org/api/sqladmin/v1beta4"

type DatabaseInstance struct {
	// ConnectionName: Connection name of the Cloud SQL instance used in
	// connection strings.
	ConnectionName string `json:"connectionName,omitempty"`

	// DatabaseVersion: The database engine type and version. The
	// `databaseVersion` field cannot be changed after instance creation.
	//
	// Possible values:
	//   "SQL_DATABASE_VERSION_UNSPECIFIED" - This is an unknown database
	// version.
	//   "MYSQL_5_1" - The database version is MySQL 5.1.
	//   "MYSQL_5_5" - The database version is MySQL 5.5.
	//   "MYSQL_5_6" - The database version is MySQL 5.6.
	//   "MYSQL_5_7" - The database version is MySQL 5.7.
	//   "SQLSERVER_2017_STANDARD" - The database version is SQL Server 2017
	// Standard.
	//   "SQLSERVER_2017_ENTERPRISE" - The database version is SQL Server
	// 2017 Enterprise.
	//   "SQLSERVER_2017_EXPRESS" - The database version is SQL Server 2017
	// Express.
	//   "SQLSERVER_2017_WEB" - The database version is SQL Server 2017 Web.
	//   "POSTGRES_9_6" - The database version is PostgreSQL 9.6.
	//   "POSTGRES_10" - The database version is PostgreSQL 10.
	//   "POSTGRES_11" - The database version is PostgreSQL 11.
	//   "POSTGRES_12" - The database version is PostgreSQL 12.
	//   "POSTGRES_13" - The database version is PostgreSQL 13.
	//   "POSTGRES_14" - The database version is PostgreSQL 14.
	//   "MYSQL_8_0" - The database version is MySQL 8.
	//   "MYSQL_8_0_18" - The database major version is MySQL 8.0 and the
	// minor version is 18.
	//   "MYSQL_8_0_26" - The database major version is MySQL 8.0 and the
	// minor version is 26.
	//   "MYSQL_8_0_27" - The database major version is MySQL 8.0 and the
	// minor version is 27.
	//   "MYSQL_8_0_28" - The database major version is MySQL 8.0 and the
	// minor version is 28.
	//   "MYSQL_8_0_29" - The database major version is MySQL 8.0 and the
	// minor version is 29.
	//   "MYSQL_8_0_30" - The database major version is MySQL 8.0 and the
	// minor version is 30.
	//   "SQLSERVER_2019_STANDARD" - The database version is SQL Server 2019
	// Standard.
	//   "SQLSERVER_2019_ENTERPRISE" - The database version is SQL Server
	// 2019 Enterprise.
	//   "SQLSERVER_2019_EXPRESS" - The database version is SQL Server 2019
	// Express.
	//   "SQLSERVER_2019_WEB" - The database version is SQL Server 2019 Web.
	DatabaseVersion string `json:"databaseVersion,omitempty"`

	// DiskEncryptionConfiguration: Disk encryption configuration specific
	// to an instance.
	DiskEncryptionConfiguration *sqladmin.DiskEncryptionConfiguration `json:"diskEncryptionConfiguration,omitempty"`

	// FailoverReplica: The name and status of the failover replica.
	FailoverReplica *DatabaseInstanceFailoverReplica `json:"failoverReplica,omitempty"`

	// GceZone: The Compute Engine zone that the instance is currently
	// serving from. This value could be different from the zone that was
	// specified when the instance was created if the instance has failed
	// over to its secondary zone. WARNING: Changing this might restart the
	// instance.
	GceZone string `json:"gceZone,omitempty"`

	// InstanceType: The instance type.
	//
	// Possible values:
	//   "SQL_INSTANCE_TYPE_UNSPECIFIED" - This is an unknown Cloud SQL
	// instance type.
	//   "CLOUD_SQL_INSTANCE" - A regular Cloud SQL instance that is not
	// replicating from a primary instance.
	//   "ON_PREMISES_INSTANCE" - An instance running on the customer's
	// premises that is not managed by Cloud SQL.
	//   "READ_REPLICA_INSTANCE" - A Cloud SQL instance acting as a
	// read-replica.
	InstanceType string `json:"instanceType,omitempty"`

	// IpAddresses: The assigned IP addresses for the instance.
	IpAddresses []*sqladmin.IpMapping `json:"ipAddresses,omitempty"`

	// Kind: This is always `sql#instance`.
	Kind string `json:"kind,omitempty"`

	// MaintenanceVersion: The current software version on the instance.
	MaintenanceVersion string `json:"maintenanceVersion,omitempty"`

	// MasterInstanceName: The name of the instance which will act as
	// primary in the replication setup.
	MasterInstanceName string `json:"masterInstanceName,omitempty"`

	// MaxDiskSize: The maximum disk size of the instance in bytes.
	MaxDiskSize int64 `json:"maxDiskSize,omitempty,string"`

	// Name: Name of the Cloud SQL instance. This does not include the
	// project ID.
	Name string `json:"name,omitempty"`

	// Project: The project ID of the project containing the Cloud SQL
	// instance. The Google apps domain is prefixed if applicable.
	Project string `json:"project,omitempty"`

	// Region: The geographical region. Can be: * `us-central` (`FIRST_GEN`
	// instances only) * `us-central1` (`SECOND_GEN` instances only) *
	// `asia-east1` or `europe-west1`. Defaults to `us-central` or
	// `us-central1` depending on the instance type. The region cannot be
	// changed after instance creation.
	Region string `json:"region,omitempty"`

	// ReplicaNames: The replicas of the instance.
	ReplicaNames []string `json:"replicaNames,omitempty"`

	// RootPassword: Initial root password. Use only on creation. You must
	// set root passwords before you can connect to PostgreSQL instances.
	RootPassword string `json:"rootPassword,omitempty"`

	// SecondaryGceZone: The Compute Engine zone that the failover instance
	// is currently serving from for a regional instance. This value could
	// be different from the zone that was specified when the instance was
	// created if the instance has failed over to its secondary/failover
	// zone.
	SecondaryGceZone string `json:"secondaryGceZone,omitempty"`

	// SelfLink: The URI of this resource.
	SelfLink string `json:"selfLink,omitempty"`

	// ServerCaCert: SSL configuration.
	ServerCaCert *sqladmin.SslCert `json:"serverCaCert,omitempty"`

	// Settings: The user settings.
	Settings *Settings `json:"settings,omitempty"`
}

type Settings struct {
	// ActivationPolicy: The activation policy specifies when the instance
	// is activated; it is applicable only when the instance state is
	// RUNNABLE. Valid values: * `ALWAYS`: The instance is on, and remains
	// so even in the absence of connection requests. * `NEVER`: The
	// instance is off; it is not activated, even if a connection request
	// arrives.
	//
	// Possible values:
	//   "SQL_ACTIVATION_POLICY_UNSPECIFIED" - Unknown activation plan.
	//   "ALWAYS" - The instance is always up and running.
	//   "NEVER" - The instance never starts.
	//   "ON_DEMAND" - The instance starts upon receiving requests.
	ActivationPolicy string `json:"activationPolicy,omitempty"`

	// AvailabilityType: Availability type. Potential values: * `ZONAL`: The
	// instance serves data from only one zone. Outages in that zone affect
	// data accessibility. * `REGIONAL`: The instance can serve data from
	// more than one zone in a region (it is highly available)./ For more
	// information, see Overview of the High Availability Configuration
	// (https://cloud.google.com/sql/docs/mysql/high-availability).
	//
	// Possible values:
	//   "SQL_AVAILABILITY_TYPE_UNSPECIFIED" - This is an unknown
	// Availability type.
	//   "ZONAL" - Zonal available instance.
	//   "REGIONAL" - Regional available instance.
	AvailabilityType string `json:"availabilityType,omitempty"`

	// BackupConfiguration: The daily backup configuration for the instance.
	BackupConfiguration *BackupConfiguration `json:"backupConfiguration,omitempty"`

	// Collation: The name of server Instance collation.
	Collation string `json:"collation,omitempty"`

	// ConnectorEnforcement: Specifies if connections must use Cloud SQL
	// connectors. Option values include the following: `NOT_REQUIRED`
	// (Cloud SQL instances can be connected without Cloud SQL Connectors)
	// and `REQUIRED` (Only allow connections that use Cloud SQL Connectors)
	// Note that using REQUIRED disables all existing authorized networks.
	// If this field is not specified when creating a new instance,
	// NOT_REQUIRED is used. If this field is not specified when patching or
	// updating an existing instance, it is left unchanged in the instance.
	//
	// Possible values:
	//   "CONNECTOR_ENFORCEMENT_UNSPECIFIED" - The requirement for Cloud SQL
	// connectors is unknown.
	//   "NOT_REQUIRED" - Do not require Cloud SQL connectors.
	//   "REQUIRED" - Require all connections to use Cloud SQL connectors,
	// including the Cloud SQL Auth Proxy and Cloud SQL Java, Python, and Go
	// connectors. Note: This disables all existing authorized networks.
	ConnectorEnforcement string `json:"connectorEnforcement,omitempty"`

	// CrashSafeReplicationEnabled: Configuration specific to read replica
	// instances. Indicates whether database flags for crash-safe
	// replication are enabled. This property was only applicable to First
	// Generation instances.
	CrashSafeReplicationEnabled *bool `json:"crashSafeReplicationEnabled,omitempty,string"`

	// DataDiskSizeGb: The size of data disk, in GB. The data disk size
	// minimum is 10GB.
	DataDiskSizeGb int64 `json:"dataDiskSizeGb,omitempty,string"`

	// DataDiskType: The type of data disk: `PD_SSD` (default) or `PD_HDD`.
	// Not used for First Generation instances.
	//
	// Possible values:
	//   "SQL_DATA_DISK_TYPE_UNSPECIFIED" - This is an unknown data disk
	// type.
	//   "PD_SSD" - An SSD data disk.
	//   "PD_HDD" - An HDD data disk.
	//   "OBSOLETE_LOCAL_SSD" - This field is deprecated and will be removed
	// from a future version of the API.
	DataDiskType string `json:"dataDiskType,omitempty"`

	// DatabaseFlags: The database flags passed to the instance at startup.
	DatabaseFlags []*sqladmin.DatabaseFlags `json:"databaseFlags,omitempty"`

	// DatabaseReplicationEnabled: Configuration specific to read replica
	// instances. Indicates whether replication is enabled or not. WARNING:
	// Changing this restarts the instance.
	DatabaseReplicationEnabled *bool `json:"databaseReplicationEnabled,omitempty,string"`

	// DeletionProtectionEnabled: Configuration to protect against
	// accidental instance deletion.
	DeletionProtectionEnabled *bool `json:"deletionProtectionEnabled,omitempty,string"`

	// DenyMaintenancePeriods: Deny maintenance periods
	DenyMaintenancePeriods []*sqladmin.DenyMaintenancePeriod `json:"denyMaintenancePeriods,omitempty"`

	// InsightsConfig: Insights configuration, for now relevant only for
	// Postgres.
	InsightsConfig *sqladmin.InsightsConfig `json:"insightsConfig,omitempty"`

	// IpConfiguration: The settings for IP Management. This allows to
	// enable or disable the instance IP and manage which external networks
	// can connect to the instance. The IPv4 address cannot be disabled for
	// Second Generation instances.
	IpConfiguration *IpConfiguration `json:"ipConfiguration,omitempty"`

	// Kind: This is always `sql#settings`.
	Kind string `json:"kind,omitempty"`

	// LocationPreference: The location preference settings. This allows the
	// instance to be located as near as possible to either an App Engine
	// app or Compute Engine zone for better performance. App Engine
	// co-location was only applicable to First Generation instances.
	LocationPreference *sqladmin.LocationPreference `json:"locationPreference,omitempty"`

	// MaintenanceWindow: The maintenance window for this instance. This
	// specifies when the instance can be restarted for maintenance
	// purposes.
	MaintenanceWindow *sqladmin.MaintenanceWindow `json:"maintenanceWindow,omitempty"`

	// PasswordValidationPolicy: The local user password validation policy
	// of the instance.
	PasswordValidationPolicy *sqladmin.PasswordValidationPolicy `json:"passwordValidationPolicy,omitempty"`

	// PricingPlan: The pricing plan for this instance. This can be either
	// `PER_USE` or `PACKAGE`. Only `PER_USE` is supported for Second
	// Generation instances.
	//
	// Possible values:
	//   "SQL_PRICING_PLAN_UNSPECIFIED" - This is an unknown pricing plan
	// for this instance.
	//   "PACKAGE" - The instance is billed at a monthly flat rate.
	//   "PER_USE" - The instance is billed per usage.
	PricingPlan string `json:"pricingPlan,omitempty"`

	// ReplicationType: The type of replication this instance uses. This can
	// be either `ASYNCHRONOUS` or `SYNCHRONOUS`. (Deprecated) This property
	// was only applicable to First Generation instances.
	//
	// Possible values:
	//   "SQL_REPLICATION_TYPE_UNSPECIFIED" - This is an unknown replication
	// type for a Cloud SQL instance.
	//   "SYNCHRONOUS" - The synchronous replication mode for First
	// Generation instances. It is the default value.
	//   "ASYNCHRONOUS" - The asynchronous replication mode for First
	// Generation instances. It provides a slight performance gain, but if
	// an outage occurs while this option is set to asynchronous, you can
	// lose up to a few seconds of updates to your data.
	ReplicationType string `json:"replicationType,omitempty"`

	// SettingsVersion: The version of instance settings. This is a required
	// field for update method to make sure concurrent updates are handled
	// properly. During update, use the most recent settingsVersion value
	// for this instance and do not try to update this value.
	SettingsVersion int64 `json:"settingsVersion,omitempty,string"`

	// StorageAutoResize: Configuration to increase storage size
	// automatically. The default value is true.
	StorageAutoResize *bool `json:"storageAutoResize,omitempty,string"`

	// StorageAutoResizeLimit: The maximum size to which storage capacity
	// can be automatically increased. The default value is 0, which
	// specifies that there is no limit.
	StorageAutoResizeLimit int64 `json:"storageAutoResizeLimit,omitempty"`

	// Tier: The tier (or machine type) for this instance, for example
	// `db-custom-1-3840`. WARNING: Changing this restarts the instance.
	Tier string `json:"tier,omitempty"`

	// UserLabels: User-provided labels, represented as a dictionary where
	// each label is a single key value pair.
	UserLabels map[string]string `json:"userLabels,omitempty"`
}

type DatabaseInstanceFailoverReplica struct {
	// Available: The availability status of the failover replica. A false
	// status indicates that the failover replica is out of sync. The
	// primary instance can only failover to the failover replica when the
	// status is true.
	Available *bool `json:"available,omitempty,string"`

	// Name: The name of the failover replica. If specified at instance
	// creation, a failover replica is created for the instance. The name
	// doesn't include the project ID.
	Name string `json:"name,omitempty"`
}

type BackupConfiguration struct {
	// BackupRetentionSettings: Backup retention settings.
	BackupRetentionSettings *BackupRetentionSettings `json:"backupRetentionSettings,omitempty"`

	// BinaryLogEnabled: (MySQL only) Whether binary log is enabled. If
	// backup configuration is disabled, binarylog must be disabled as well.
	BinaryLogEnabled *bool `json:"binaryLogEnabled,omitempty,string"`

	// Enabled: Whether this configuration is enabled.
	Enabled *bool `json:"enabled,omitempty,string"`

	// Kind: This is always `sql#backupConfiguration`.
	Kind string `json:"kind,omitempty"`

	// Location: Location of the backup
	Location string `json:"location,omitempty"`

	// PointInTimeRecoveryEnabled: (Postgres only) Whether point in time
	// recovery is enabled.
	PointInTimeRecoveryEnabled *bool `json:"pointInTimeRecoveryEnabled,omitempty,string"`

	// ReplicationLogArchivingEnabled: Reserved for future use.
	ReplicationLogArchivingEnabled *bool `json:"replicationLogArchivingEnabled,omitempty,string"`

	// StartTime: Start time for the daily backup configuration in UTC
	// timezone in the 24-hour format - `HH:MM`.
	StartTime string `json:"startTime,omitempty"`

	// TransactionLogRetentionDays: The number of days of transaction logs
	// we retain for point in time restore, from 1-7.
	TransactionLogRetentionDays int64 `json:"transactionLogRetentionDays,omitempty"`
}

type IpConfiguration struct {
	// AllocatedIpRange: The name of the allocated ip range for the private
	// ip Cloud SQL instance. For example:
	// "google-managed-services-default". If set, the instance ip will be
	// created in the allocated range. The range name must comply with RFC
	// 1035 (https://tools.ietf.org/html/rfc1035). Specifically, the name
	// must be 1-63 characters long and match the regular expression
	// `[a-z]([-a-z0-9]*[a-z0-9])?.`
	AllocatedIpRange string `json:"allocatedIpRange,omitempty"`

	// AuthorizedNetworks: The list of external networks that are allowed to
	// connect to the instance using the IP. In 'CIDR' notation, also known
	// as 'slash' notation (for example: `157.197.200.0/24`).
	AuthorizedNetworks []*sqladmin.AclEntry `json:"authorizedNetworks,omitempty"`

	// Ipv4Enabled: Whether the instance is assigned a public IP address or
	// not.
	Ipv4Enabled *bool `json:"ipv4Enabled,omitempty,string"`

	// PrivateNetwork: The resource link for the VPC network from which the
	// Cloud SQL instance is accessible for private IP. For example,
	// `/projects/myProject/global/networks/default`. This setting can be
	// updated, but it cannot be removed after it is set.
	PrivateNetwork string `json:"privateNetwork,omitempty"`

	// RequireSsl: Whether SSL connections over IP are enforced or not.
	RequireSsl *bool `json:"requireSsl,omitempty,string"`
}

type BackupRetentionSettings struct {

	// The unit that 'retained_backups' represents.
	RetentionUnit string `json:"retentionUnit,omitempty"`
	// Depending on the value of retention_unit, this is used to determine
	// if a backup needs to be deleted.  If retention_unit is 'COUNT', we will
	// retain this many backups.
	RetainedBackups int64 `json:"retainedBackups,omitempty,string"`
	// contains filtered or unexported fields
}
